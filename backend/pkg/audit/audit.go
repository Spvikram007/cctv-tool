package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// Logger provides tamper-evident audit logging with cryptographic hash chaining & secret redaction
type Logger struct {
	mu       sync.RWMutex
	entries  []models.AuditLogEntry
	lastHash string
}

func NewLogger() *Logger {
	return &Logger{
		entries:  make([]models.AuditLogEntry, 0),
		lastHash: "GENESIS_SENTINEL_ROOT",
	}
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
	return entry
}

// GetEntries returns all recorded entries
func (l *Logger) GetEntries() []models.AuditLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	res := make([]models.AuditLogEntry, len(l.entries))
	copy(res, l.entries)
	return res
}

// VerifyChainIntegrity validates cryptographic continuity across the entire audit log trail
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

	return true, len(l.entries), nil
}
