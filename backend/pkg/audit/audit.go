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

var (
	secretRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|auth)["']?\s*[:=]\s*["']?([^"',\s&]+)`),
		regexp.MustCompile(`(?i)Authorization:\s*Basic\s+([A-Za-z0-9+/=]+)`),
		regexp.MustCompile(`(?i)Authorization:\s*Digest\s+([^\r\n]+)`),
		regexp.MustCompile(`(?i)rtsp://([^:]+):([^@]+)@`),
	}
)

// RedactSecrets removes sensitive data like passwords or tokens from strings
func RedactSecrets(input string) string {
	res := input
	for _, re := range secretRegexes {
		res = re.ReplaceAllString(res, "$1: [REDACTED]")
	}
	return res
}

// Logger provides tamper-evident audit logging with secret redaction
type Logger struct {
	mu      sync.RWMutex
	entries []models.AuditLogEntry
	lastHash string
}

func NewLogger() *Logger {
	return &Logger{
		entries:  make([]models.AuditLogEntry, 0),
		lastHash: "GENESIS_SENTINEL_ROOT",
	}
}

// Log records an audit entry, automatically redacting any credentials
func (l *Logger) Log(user, deviceID, deviceIP, action, category, status, details, diff string) models.AuditLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	cleanedDetails := RedactSecrets(details)
	cleanedDiff := RedactSecrets(diff)

	now := time.Now().UTC()
	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		l.lastHash, now.Format(time.RFC3339Nano), user, deviceID, action, status, cleanedDetails, cleanedDiff)
	h := sha256.Sum256([]byte(hashInput))
	entryID := hex.EncodeToString(h[:16])
	l.lastHash = hex.EncodeToString(h[:])

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
