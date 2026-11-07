package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/database"
)

var (
	ErrInvalidImageFormat = errors.New("image data failed magic byte validation: not genuine JPEG or PNG")
	ErrImageTooSmall      = errors.New("image payload too small to be a valid frame (< 1KB)")
	ErrDeviceNotFound     = errors.New("device not found in store")
)

// SnapshotMetadata represents stored snapshot details
type SnapshotMetadata struct {
	ID        string    `json:"id"`
	DeviceID  string    `json:"device_id"`
	Timestamp time.Time `json:"timestamp"`
	Format    string    `json:"format"` // "JPEG" or "PNG"
	SizeBytes int64     `json:"size_bytes"`
	FileName  string    `json:"file_name"`
	URL       string    `json:"url"`
}

type Engine struct {
	store    *database.Store
	registry *adapters.Registry
	baseDir  string
	mu       sync.RWMutex
	history  map[string][]SnapshotMetadata // deviceID -> snapshots
}

func NewEngine(store *database.Store, registry *adapters.Registry, baseDir string) *Engine {
	if baseDir == "" {
		baseDir = "./data/snapshots"
	}
	_ = os.MkdirAll(baseDir, 0755)

	return &Engine{
		store:    store,
		registry: registry,
		baseDir:  baseDir,
		history:  make(map[string][]SnapshotMetadata),
	}
}

// ValidateImage checks for genuine JPEG or PNG magic bytes and minimum length
func ValidateImage(data []byte) (string, error) {
	if len(data) < 1024 {
		return "", ErrImageTooSmall
	}

	// Reject SVG or HTML error responses
	prefix := string(data[:min(len(data), 64)])
	if strings.Contains(strings.ToLower(prefix), "<svg") ||
		strings.Contains(strings.ToLower(prefix), "<html") ||
		strings.Contains(strings.ToLower(prefix), "<!doctype") {
		return "", ErrInvalidImageFormat
	}

	// JPEG Magic Bytes: 0xFF 0xD8 0xFF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "JPEG", nil
	}

	// PNG Magic Bytes: 0x89 0x50 0x4E 0x47
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "PNG", nil
	}

	return "", ErrInvalidImageFormat
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FindFFmpeg locates ffmpeg binary on the system
func FindFFmpeg() string {
	// 1. Check known Recoverit / local paths
	candidates := []string{
		`C:\Program Files (x86)\Wondershare\Recoverit\ffmpeg.exe`,
		`C:\Program Files\Wondershare\Recoverit\ffmpeg.exe`,
		`C:\ffmpeg\bin\ffmpeg.exe`,
		`./bin/ffmpeg.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// 2. Check PATH
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return ""
}

// CaptureFromRTSP uses ffmpeg to grab a single genuine frame from an RTSP stream
func CaptureFromRTSP(rtspURL, username, password string, timeout time.Duration) ([]byte, error) {
	ffmpegPath := FindFFmpeg()
	if ffmpegPath == "" {
		return nil, errors.New("ffmpeg executable not found on system")
	}

	authenticatedURL := rtspURL
	if username != "" && !strings.Contains(rtspURL, "@") {
		// Inject credentials into rtsp URL for ffmpeg
		authenticatedURL = strings.Replace(rtspURL, "rtsp://", fmt.Sprintf("rtsp://%s:%s@", username, password), 1)
	}

	cmd := exec.Command(ffmpegPath,
		"-rtsp_transport", "tcp",
		"-y",
		"-i", authenticatedURL,
		"-vframes", "1",
		"-f", "image2",
		"-q:v", "2",
		"pipe:1",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("ffmpeg capture timed out after %v", timeout)
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("ffmpeg frame grab failed: %v | %s", err, stderr.String())
		}
	}

	data := stdout.Bytes()
	if _, err := ValidateImage(data); err != nil {
		return nil, fmt.Errorf("frame grabbed was invalid: %w", err)
	}

	return data, nil
}

// CaptureSnapshot coordinates vendor adapter, ONVIF, and RTSP fallback to retrieve a genuine snapshot
func (e *Engine) CaptureSnapshot(deviceID string) (*SnapshotMetadata, []byte, error) {
	dev, ok := e.store.GetDevice(deviceID)
	if !ok {
		return nil, nil, ErrDeviceNotFound
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, nil, err
	}

	var imgBytes []byte
	var format string

	// Step 1: Query vendor adapter
	data, err := adapter.CaptureSnapshot(&dev)
	if err == nil {
		fmtStr, vErr := ValidateImage(data)
		if vErr == nil {
			imgBytes = data
			format = fmtStr
		}
	}

	// Step 2: Fallback to FFmpeg RTSP stream capture if adapter returned SVG or failed
	if len(imgBytes) == 0 {
		streamURI, sErr := adapter.GetStreamURI(&dev, 0)
		if sErr == nil && streamURI != "" {
			ffBytes, ffErr := CaptureFromRTSP(streamURI, dev.Username, dev.Password, 6*time.Second)
			if ffErr == nil {
				fmtStr, _ := ValidateImage(ffBytes)
				imgBytes = ffBytes
				format = fmtStr
			}
		}
	}

	if len(imgBytes) == 0 {
		return nil, nil, fmt.Errorf("failed to capture genuine image frame: %v", err)
	}

	// Step 3: Save to disk
	now := time.Now().UTC()
	ext := ".jpg"
	if format == "PNG" {
		ext = ".png"
	}
	fileName := fmt.Sprintf("%s_%s%s", dev.ID, now.Format("20060102_150405_000"), ext)
	devDir := filepath.Join(e.baseDir, dev.ID)
	_ = os.MkdirAll(devDir, 0755)
	filePath := filepath.Join(devDir, fileName)

	if err := os.WriteFile(filePath, imgBytes, 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to save snapshot file: %w", err)
	}

	meta := SnapshotMetadata{
		ID:        fmt.Sprintf("snap-%s-%d", dev.ID, now.UnixNano()),
		DeviceID:  dev.ID,
		Timestamp: now,
		Format:    format,
		SizeBytes: int64(len(imgBytes)),
		FileName:  fileName,
		URL:       fmt.Sprintf("/api/v1/devices/%s/snapshots/%s", dev.ID, fileName),
	}

	e.mu.Lock()
	e.history[dev.ID] = append([]SnapshotMetadata{meta}, e.history[dev.ID]...)
	if len(e.history[dev.ID]) > 50 {
		e.history[dev.ID] = e.history[dev.ID][:50]
	}
	e.mu.Unlock()

	return &meta, imgBytes, nil
}

// ListSnapshots returns historical snapshots for a device
func (e *Engine) ListSnapshots(deviceID string) []SnapshotMetadata {
	e.mu.RLock()
	defer e.mu.RUnlock()

	items, ok := e.history[deviceID]
	if !ok {
		return []SnapshotMetadata{}
	}
	cp := make([]SnapshotMetadata, len(items))
	copy(cp, items)
	return cp
}

// GetSnapshotFile loads an existing snapshot file from disk
func (e *Engine) GetSnapshotFile(deviceID, fileName string) ([]byte, string, error) {
	// Sanitize fileName to prevent directory traversal
	cleaned := filepath.Base(fileName)
	targetPath := filepath.Join(e.baseDir, deviceID, cleaned)

	f, err := os.Open(targetPath)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", err
	}

	contentType := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(cleaned), ".png") {
		contentType = "image/png"
	}

	return data, contentType, nil
}
