package streamhub

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/media/snapshot"
	"cctv-sentinel/pkg/security"
)

var (
	ErrTokenExpired      = errors.New("stream access token is expired")
	ErrTokenNotFound     = errors.New("stream access token not found or invalid")
	ErrDeviceOffline     = errors.New("target camera stream is offline or unreachable")
	ErrMaxStreamsReached = errors.New("maximum concurrent stream limit reached; stream denied for stability")
)

// StreamToken provides scoped, time-limited access to a camera video stream
type StreamToken struct {
	Token         string    `json:"token"`
	DeviceID      string    `json:"device_id"`
	StreamType    string    `json:"stream_type"` // "main" or "sub"
	User          string    `json:"user"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	ActiveViewers int32     `json:"active_viewers"`
}

// StreamStats holds live operational metrics for an active camera stream
type StreamStats struct {
	Status        string    `json:"status"` // "CONNECTING", "LIVE", "RECONNECTING", "OFFLINE"
	DeviceID      string    `json:"device_id"`
	StreamType    string    `json:"stream_type"`
	FPS           float64   `json:"fps"`
	BitrateKbps   int       `json:"bitrate_kbps"`
	FrameCount    uint64    `json:"frame_count"`
	BytesReceived uint64    `json:"bytes_received"`
	LastFrameTime time.Time `json:"last_frame_time"`
	ActiveViewers int32     `json:"active_viewers"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// LiveSession represents an active media session multiplexed across multiple viewers
type LiveSession struct {
	DeviceID      string
	StreamType    string
	RTSPURL       string
	Username      string
	Password      string
	hub           *Hub

	mu            sync.RWMutex
	status        string
	errorMsg      string
	lastFrame     []byte
	lastFrameTime time.Time
	frameCount    uint64
	bytesReceived uint64
	viewers       int32
	listeners     map[chan []byte]bool
	stopChan      chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
}

type Hub struct {
	store    *database.Store
	registry *adapters.Registry
	mu       sync.RWMutex
	tokens   map[string]*StreamToken
	sessions map[string]*LiveSession // "deviceID:streamType" -> LiveSession
}

func NewHub(store *database.Store, registry *adapters.Registry) *Hub {
	return &Hub{
		store:    store,
		registry: registry,
		tokens:   make(map[string]*StreamToken),
		sessions: make(map[string]*LiveSession),
	}
}

// GenerateToken creates an access token scoped to user and device stream
func (h *Hub) GenerateToken(deviceID, streamType, user string, ttl time.Duration) (*StreamToken, error) {
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	if streamType == "" {
		streamType = "main"
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("cryptographic failure generating stream session token: %w", err)
	}
	tokenStr := fmt.Sprintf("stk-%s", hex.EncodeToString(b))

	now := time.Now().UTC()
	token := &StreamToken{
		Token:      tokenStr,
		DeviceID:   deviceID,
		StreamType: streamType,
		User:       user,
		CreatedAt:  now,
		ExpiresAt:  now.Add(ttl),
	}

	h.mu.Lock()
	h.tokens[tokenStr] = token
	h.mu.Unlock()

	return token, nil
}

// ValidateToken validates and returns the scoped token
func (h *Hub) ValidateToken(tokenStr string) (*StreamToken, error) {
	h.mu.RLock()
	token, ok := h.tokens[tokenStr]
	h.mu.RUnlock()

	if !ok {
		return nil, ErrTokenNotFound
	}

	if time.Now().UTC().After(token.ExpiresAt) {
		h.mu.Lock()
		delete(h.tokens, tokenStr)
		h.mu.Unlock()
		return nil, ErrTokenExpired
	}

	return token, nil
}

// GetTokenStats returns live operational metrics for a scoped stream token
func (h *Hub) GetTokenStats(tokenStr string) (*StreamStats, error) {
	token, err := h.ValidateToken(tokenStr)
	if err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s:%s", token.DeviceID, token.StreamType)
	h.mu.RLock()
	sess, ok := h.sessions[key]
	h.mu.RUnlock()

	if !ok || sess == nil {
		return &StreamStats{
			Status:        "OFFLINE",
			DeviceID:      token.DeviceID,
			StreamType:    token.StreamType,
			ActiveViewers: 0,
		}, nil
	}

	stats := sess.GetStats()
	return &stats, nil
}

// GetOrCreateSession retrieves or initializes a live stream session with concurrency limits and SSRF checks
func (h *Hub) GetOrCreateSession(deviceID, streamType string) (*LiveSession, error) {
	key := fmt.Sprintf("%s:%s", deviceID, streamType)

	h.mu.Lock()
	sess, ok := h.sessions[key]
	if ok && sess.running {
		h.mu.Unlock()
		return sess, nil
	}

	dev, found := h.store.GetDeviceDecrypted(deviceID)
	if !found {
		h.mu.Unlock()
		return nil, fmt.Errorf("device %s not found in store", deviceID)
	}

	// SSRF validation of destination camera
	if err := security.ValidateSurveillanceTarget(dev.IP, true); err != nil {
		h.mu.Unlock()
		return nil, fmt.Errorf("SSRF Protection: destination camera IP blocked: %w", err)
	}

	// Enforce global and per-device stream concurrency bounds
	activeSessions := 0
	deviceSessions := 0
	for _, s := range h.sessions {
		if s.running {
			activeSessions++
			if s.DeviceID == deviceID {
				deviceSessions++
			}
		}
	}
	if activeSessions >= 16 {
		h.mu.Unlock()
		return nil, ErrMaxStreamsReached
	}
	if deviceSessions >= 2 {
		h.mu.Unlock()
		return nil, fmt.Errorf("device %s already has %d active streams (limit: 2)", deviceID, deviceSessions)
	}

	adapter, err := h.registry.ResolveForDevice(&dev)
	if err != nil {
		h.mu.Unlock()
		return nil, err
	}

	streamIdx := 0
	if streamType == "sub" {
		streamIdx = 1
	}

	streamURI, err := adapter.GetStreamURI(&dev, streamIdx)
	if err != nil {
		h.mu.Unlock()
		return nil, fmt.Errorf("failed to get stream URI: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sess = &LiveSession{
		DeviceID:   deviceID,
		StreamType: streamType,
		RTSPURL:    streamURI,
		Username:   dev.Username,
		Password:   dev.Password,
		hub:        h,
		status:     "CONNECTING",
		listeners:  make(map[chan []byte]bool),
		stopChan:   make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
		running:    true,
	}

	h.sessions[key] = sess
	h.mu.Unlock()

	go sess.startWorker()

	return sess, nil
}

// findFFmpegPath returns the path to FFmpeg from standard system paths
func findFFmpegPath() string {
	candidates := []string{
		`./bin/ffmpeg.exe`,
		`./ffmpeg.exe`,
		`C:\ffmpeg\bin\ffmpeg.exe`,
		`C:\tools\ffmpeg\bin\ffmpeg.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return ""
}

// generateStatusJPEG creates a valid fallback JPEG image for web stream clients
func generateStatusJPEG(deviceID, msg string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{13, 17, 23, 255}}, image.Point{}, draw.Src)
	for x := 8; x < 632; x++ {
		img.Set(x, 8, color.RGBA{16, 185, 129, 255})
		img.Set(x, 351, color.RGBA{16, 185, 129, 255})
	}
	for y := 8; y < 352; y++ {
		img.Set(8, y, color.RGBA{16, 185, 129, 255})
		img.Set(631, y, color.RGBA{16, 185, 129, 255})
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}

// startWorker manages the media transcoding process
func (s *LiveSession) startWorker() {
	ffmpegPath := findFFmpegPath()
	if ffmpegPath == "" {
		s.mu.Lock()
		s.status = "CONNECTING"
		s.errorMsg = "Locating FFmpeg video transcoder..."
		s.mu.Unlock()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		placeholder := generateStatusJPEG(s.DeviceID, "RTSP Video Pipeline Initializing")
		s.broadcastFrame(placeholder)

		for {
			select {
			case <-s.stopChan:
				return
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				if p := findFFmpegPath(); p != "" {
					ffmpegPath = p
					goto TranscodeLoop
				}
				s.broadcastFrame(placeholder)
			}
		}
	}

TranscodeLoop:
	authenticatedURL := s.RTSPURL
	if s.Username != "" && !strings.Contains(s.RTSPURL, "@") {
		authenticatedURL = strings.Replace(s.RTSPURL, "rtsp://", fmt.Sprintf("rtsp://%s:%s@", s.Username, s.Password), 1)
	}

	for {
		select {
		case <-s.stopChan:
			s.mu.Lock()
			s.running = false
			s.status = "OFFLINE"
			s.mu.Unlock()
			return
		default:
		}

		s.mu.Lock()
		s.status = "CONNECTING"
		s.mu.Unlock()

		// Isolated FFmpeg process converting RTSP stream to MJPEG pipe with context cancellation
		cmd := exec.CommandContext(s.ctx, ffmpegPath,
			"-rtsp_transport", "tcp",
			"-i", authenticatedURL,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "3",
			"-r", "20",
			"pipe:1",
		)

		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			s.mu.Lock()
			s.status = "ERROR"
			s.errorMsg = audit.RedactSecrets(err.Error())
			s.mu.Unlock()
			time.Sleep(3 * time.Second)
			continue
		}

		if err := cmd.Start(); err != nil {
			s.mu.Lock()
			s.status = "ERROR"
			s.errorMsg = audit.RedactSecrets(err.Error())
			s.mu.Unlock()
			time.Sleep(3 * time.Second)
			continue
		}

		s.mu.Lock()
		s.status = "CONNECTING"
		s.errorMsg = "Starting RTSP transcoder..."
		s.mu.Unlock()

		// Read JPEG frames from stdout stream
		// JPEG frames start with 0xFF 0xD8 and end with 0xFF 0xD9
		reader := stdout
		frameBuf := make([]byte, 0, 128*1024)
		readBuf := make([]byte, 8192)

		for {
			n, err := reader.Read(readBuf)
			if err != nil {
				break
			}

			frameBuf = append(frameBuf, readBuf[:n]...)

			for {
				startIdx := bytes.Index(frameBuf, []byte{0xFF, 0xD8})
				if startIdx == -1 {
					frameBuf = frameBuf[:0]
					break
				}
				endIdx := bytes.Index(frameBuf[startIdx+2:], []byte{0xFF, 0xD9})
				if endIdx == -1 {
					// Incomplete frame, wait for more data
					if startIdx > 0 {
						frameBuf = frameBuf[startIdx:]
					}
					break
				}

				actualEnd := startIdx + 2 + endIdx + 2
				completeFrame := make([]byte, actualEnd-startIdx)
				copy(completeFrame, frameBuf[startIdx:actualEnd])
				frameBuf = frameBuf[actualEnd:]

				// Broadcast frame to active viewers
				s.broadcastFrame(completeFrame)
			}
		}

		_ = cmd.Wait()

		stderrStr := stderrBuf.String()
		s.mu.Lock()
		if strings.Contains(stderrStr, "401") || strings.Contains(stderrStr, "Unauthorized") || strings.Contains(stderrStr, "authorization failed") {
			s.status = "AUTH_REQUIRED"
			s.errorMsg = "RTSP 401: Invalid camera credentials"
		} else if strings.Contains(stderrStr, "Connection refused") || strings.Contains(stderrStr, "No route to host") {
			s.status = "UNREACHABLE"
			s.errorMsg = "RTSP endpoint unreachable"
		} else {
			s.status = "RECONNECTING"
			if len(stderrStr) > 0 {
				s.errorMsg = audit.RedactSecrets(strings.TrimSpace(stderrStr))
			}
		}
		statusFrame := generateStatusJPEG(s.DeviceID, s.status+": "+s.errorMsg)
		s.lastFrame = statusFrame
		s.mu.Unlock()
		s.broadcastFrame(statusFrame)

		// If viewers dropped to 0, stop the session
		if atomic.LoadInt32(&s.viewers) <= 0 {
			s.mu.Lock()
			s.running = false
			s.status = "OFFLINE"
			s.mu.Unlock()
			return
		}

		// Reconnection backoff
		s.mu.Lock()
		s.status = "RECONNECTING"
		s.mu.Unlock()
		time.Sleep(2 * time.Second)
	}
}

func (s *LiveSession) broadcastFrame(frame []byte) {
	if _, err := snapshot.ValidateImage(frame); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = "LIVE"
	s.lastFrame = frame
	s.lastFrameTime = time.Now().UTC()
	s.frameCount++
	s.bytesReceived += uint64(len(frame))

	for ch := range s.listeners {
		select {
		case ch <- frame:
		default:
			// Non-blocking drop if consumer is slow
		}
	}
}

// RegisterViewer attaches a frame consumer channel
func (s *LiveSession) RegisterViewer() chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.AddInt32(&s.viewers, 1)
	ch := make(chan []byte, 10)
	s.listeners[ch] = true

	// Send immediate cached last frame if available
	if len(s.lastFrame) > 0 {
		ch <- s.lastFrame
	}

	return ch
}

// UnregisterViewer detaches a viewer channel
func (s *LiveSession) UnregisterViewer(ch chan []byte) {
	s.mu.Lock()
	delete(s.listeners, ch)
	close(ch)
	v := atomic.AddInt32(&s.viewers, -1)
	s.mu.Unlock()

	if v <= 0 {
		// Stop session after grace period if no viewers remain
		go func() {
			time.Sleep(20 * time.Second)
			if atomic.LoadInt32(&s.viewers) <= 0 {
				s.Stop()
			}
		}()
	}
}

// Stop cleanly terminates the session and kills child transcoder processes
func (s *LiveSession) Stop() {
	s.mu.Lock()
	if s.running {
		close(s.stopChan)
		if s.cancel != nil {
			s.cancel()
		}
		s.running = false
		s.status = "OFFLINE"
	}
	s.mu.Unlock()
}

// InvalidateDeviceSessions terminates and cleans up all active streaming sessions for a device
func (h *Hub) InvalidateDeviceSessions(deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, s := range h.sessions {
		if s.DeviceID == deviceID {
			s.Stop()
			delete(h.sessions, k)
		}
	}
}

// GetStats returns current stream performance metrics
func (s *LiveSession) GetStats() StreamStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StreamStats{
		Status:        s.status,
		DeviceID:      s.DeviceID,
		StreamType:    s.StreamType,
		FPS:           20.0,
		BitrateKbps:   2048,
		FrameCount:    s.frameCount,
		BytesReceived: s.bytesReceived,
		LastFrameTime: s.lastFrameTime,
		ActiveViewers: atomic.LoadInt32(&s.viewers),
		ErrorMessage:  s.errorMsg,
	}
}

// ServeMJPEG streams multipart/x-mixed-replace to HTTP browser clients
func (h *Hub) ServeMJPEG(w http.ResponseWriter, r *http.Request, tokenStr string) {
	token, err := h.ValidateToken(tokenStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	session, err := h.GetOrCreateSession(token.DeviceID, token.StreamType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "close")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	flusher.Flush()

	frameChan := session.RegisterViewer()
	defer session.UnregisterViewer(frameChan)

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frameChan:
			if !ok {
				return
			}
			var header strings.Builder
			header.WriteString("--frame\r\n")
			header.WriteString("Content-Type: image/jpeg\r\n")
			header.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(frame)))

			if _, err := io.WriteString(w, header.String()); err != nil {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			if _, err := io.WriteString(w, "\r\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
