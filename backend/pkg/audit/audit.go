package audit

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"cctv-sentinel/pkg/models"
)

type secretReplacer struct {
	re   *regexp.Regexp
	repl string
}

var secretReplacers = []secretReplacer{
	{
		re:   regexp.MustCompile(`(?i)(password|passwd|pwd|secret)["']?\s*[:=]\s*["']?([^"',\s&]+)["']?`),
		repl: `$1: [REDACTED]`,
	},
	{
		re:   regexp.MustCompile(`(?i)(Authorization:\s*Basic\s+)[A-Za-z0-9+/=]+`),
		repl: `${1}[REDACTED]`,
	},
	{
		re:   regexp.MustCompile(`(?i)(Authorization:\s*Digest\s+)[^\r\n]+`),
		repl: `${1}[REDACTED]`,
	},
	{
		re:   regexp.MustCompile(`(?i)(Authorization:\s*Bearer\s+)[A-Za-z0-9\-_\.]+`),
		repl: `${1}[REDACTED]`,
	},
	{
		re:   regexp.MustCompile(`(?i)rtsp://([^:]+):([^@]+)@`),
		repl: `rtsp://${1}:[REDACTED]@`,
	},
	{
		re:   regexp.MustCompile(`(?i)(<(?:wsse:)?PasswordDigest[^>]*>)[^<]+(<\/(?:wsse:)?PasswordDigest>)`),
		repl: `${1}[REDACTED]${2}`,
	},
	{
		re:   regexp.MustCompile(`(?i)((?:api_key|token|auth_token)["']?\s*[:=]\s*["']?)[^"',\s&]+(["']?)`),
		repl: `${1}[REDACTED]${2}`,
	},
}

// RedactSecrets removes sensitive data like passwords or tokens from strings
func RedactSecrets(input string) string {
	res := input
	for _, sr := range secretReplacers {
		res = sr.re.ReplaceAllString(res, sr.repl)
	}
	return res
}

// AuditCheckpoint represents a signed cryptographic snapshot in the linear audit chain
type AuditCheckpoint struct {
	Index       int       `json:"index"`
	EntryID     string    `json:"entry_id"`
	EntryHash   string    `json:"entry_hash"`
	Timestamp   time.Time `json:"timestamp"`
	HMACDigest  string    `json:"hmac_digest"`
}

// Logger provides tamper-evident audit logging with cryptographic hash chaining, secret redaction & signed HMAC checkpoints
type Logger struct {
	mu            sync.RWMutex
	entries       []models.AuditLogEntry
	checkpoints   []AuditCheckpoint
	lastHash      string
	checkpointKey []byte
}

func NewLogger() *Logger {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// Fallback deterministic key derivation if crypto/rand fails
		h := sha256.Sum256([]byte("CCTV_SENTINEL_DEFAULT_HMAC_KEY_ROOT"))
		key = h[:]
	}
	return &Logger{
		entries:       make([]models.AuditLogEntry, 0),
		checkpoints:   make([]AuditCheckpoint, 0),
		lastHash:      "GENESIS_SENTINEL_ROOT",
		checkpointKey: key,
	}
}

// SetCheckpointKey allows injecting an isolated, hardware-protected HMAC key
func (l *Logger) SetCheckpointKey(key []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(key) < 16 {
		return fmt.Errorf("HMAC key length must be at least 16 bytes")
	}
	l.checkpointKey = make([]byte, len(key))
	copy(l.checkpointKey, key)
	return nil
}

// Log records an audit entry with cryptographic hash chaining
func (l *Logger) Log(user, deviceID, deviceIP, action, category, status, details, diff string) models.AuditLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	cleanedDetails := RedactSecrets(details)
	cleanedDiff := RedactSecrets(diff)

	now := time.Now().UTC()
	prev := l.lastHash

	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		prev, now.Format(time.RFC3339Nano), user, deviceID, action, status, cleanedDetails, cleanedDiff)
	h := sha256.Sum256([]byte(hashInput))
	entryHash := hex.EncodeToString(h[:])
	entryID := hex.EncodeToString(h[:16])
	l.lastHash = entryHash

	entry := models.AuditLogEntry{
		ID:        entryID,
		Timestamp: now,
		User:      user,
		DeviceID:  deviceID,
		DeviceIP:  deviceIP,
		Action:    action,
		Category:  category,
		Status:    status,
		Details:   cleanedDetails,
		Diff:      cleanedDiff,
		PrevHash:  prev,
		EntryHash: entryHash,
	}

	l.entries = append(l.entries, entry)

	// Automatically create signed HMAC checkpoint every 50 entries
	if len(l.entries)%50 == 0 {
		l.createCheckpointInternal()
	}

	return entry
}

func (l *Logger) createCheckpointInternal() AuditCheckpoint {
	idx := len(l.entries) - 1
	entry := l.entries[idx]

	mac := hmac.New(sha256.New, l.checkpointKey)
	macInput := fmt.Sprintf("%d|%s|%s|%s", idx, entry.ID, entry.EntryHash, entry.Timestamp.Format(time.RFC3339Nano))
	mac.Write([]byte(macInput))
	digest := hex.EncodeToString(mac.Sum(nil))

	cp := AuditCheckpoint{
		Index:      idx,
		EntryID:    entry.ID,
		EntryHash:  entry.EntryHash,
		Timestamp:  time.Now().UTC(),
		HMACDigest: digest,
	}
	l.checkpoints = append(l.checkpoints, cp)
	return cp
}

// CreateCheckpoint explicitly freezes and signs an HMAC checkpoint at the current chain head
func (l *Logger) CreateCheckpoint() (AuditCheckpoint, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		return AuditCheckpoint{}, fmt.Errorf("cannot create checkpoint on empty audit log")
	}
	return l.createCheckpointInternal(), nil
}

// GetEntries returns all recorded entries
func (l *Logger) GetEntries() []models.AuditLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	res := make([]models.AuditLogEntry, len(l.entries))
	copy(res, l.entries)
	return res
}

// GetCheckpoints returns all signed HMAC checkpoints
func (l *Logger) GetCheckpoints() []AuditCheckpoint {
	l.mu.RLock()
	defer l.mu.RUnlock()
	res := make([]AuditCheckpoint, len(l.checkpoints))
	copy(res, l.checkpoints)
	return res
}

// VerifyChainIntegrity validates cryptographic continuity across the entire audit log trail AND validates all HMAC checkpoints
func (l *Logger) VerifyChainIntegrity() (bool, int, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	expectedPrev := "GENESIS_SENTINEL_ROOT"
	for idx, entry := range l.entries {
		if entry.PrevHash != "" && entry.PrevHash != expectedPrev {
			return false, idx, fmt.Errorf("tamper detected at index %d: expected prev hash %s, got %s", idx, expectedPrev, entry.PrevHash)
		}
		hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
			expectedPrev, entry.Timestamp.Format(time.RFC3339Nano), entry.User, entry.DeviceID, entry.Action, entry.Status, entry.Details, entry.Diff)
		h := sha256.Sum256([]byte(hashInput))
		calcHash := hex.EncodeToString(h[:])
		if entry.EntryHash != "" && entry.EntryHash != calcHash {
			return false, idx, fmt.Errorf("tamper detected at index %d: entry hash mismatch", idx)
		}
		expectedPrev = calcHash
	}

	// Verify signed HMAC checkpoints
	for _, cp := range l.checkpoints {
		if cp.Index >= len(l.entries) {
			return false, cp.Index, fmt.Errorf("checkpoint index %d exceeds ledger size", cp.Index)
		}
		entry := l.entries[cp.Index]
		if entry.EntryHash != cp.EntryHash {
			return false, cp.Index, fmt.Errorf("checkpoint hash mismatch at entry %d", cp.Index)
		}

		mac := hmac.New(sha256.New, l.checkpointKey)
		macInput := fmt.Sprintf("%d|%s|%s|%s", cp.Index, entry.ID, entry.EntryHash, entry.Timestamp.Format(time.RFC3339Nano))
		mac.Write([]byte(macInput))
		expectedDigest := hex.EncodeToString(mac.Sum(nil))
		if subtleConstantTimeCompare(expectedDigest, cp.HMACDigest) != 1 {
			return false, cp.Index, fmt.Errorf("checkpoint HMAC signature forged or corrupted at index %d", cp.Index)
		}
	}

	return true, len(l.entries), nil
}

func subtleConstantTimeCompare(a, b string) int {
	if len(a) != len(b) {
		return 0
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	if v == 0 {
		return 1
	}
	return 0
}

// ExportSignedLedger writes an authenticated, signed audit snapshot to an external file
func (l *Logger) ExportSignedLedger(exportPath string) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	cleanPath := filepath.Clean(exportPath)
	payload := map[string]interface{}{
		"version":      "2.0",
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
		"entry_count":  len(l.entries),
		"entries":      l.entries,
		"checkpoints":  l.checkpoints,
		"head_hash":    l.lastHash,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cleanPath, data, 0600)
}
