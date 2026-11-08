package audit

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	Index      int       `json:"index"`
	EntryID    string    `json:"entry_id"`
	EntryHash  string    `json:"entry_hash"`
	Timestamp  time.Time `json:"timestamp"`
	HMACDigest string    `json:"hmac_digest"`
	KeyID      string    `json:"key_id,omitempty"`
}

// Logger provides tamper-evident audit logging with cryptographic hash chaining, secret redaction & signed HMAC checkpoints
type Logger struct {
	mu            sync.RWMutex
	entries       []models.AuditLogEntry
	checkpoints   []AuditCheckpoint
	lastHash      string
	keyFilePath   string
	currentKeyID  string
	checkpointKey []byte
	keyRing       map[string][]byte
}

type persistedKeyStore struct {
	CurrentKeyID string            `json:"current_key_id"`
	Keys         map[string]string `json:"keys"` // keyID -> hex
}

// NewLogger initializes the audit logger with secure OS-backed key generation and persistence
func NewLogger() *Logger {
	l, err := NewLoggerWithKeyFile("sentinel_audit.key")
	if err != nil {
		panic(fmt.Sprintf("CRITICAL CRYPTOGRAPHIC FAILURE: Failed to initialize secure audit logger: %v", err))
	}
	return l
}

// NewLoggerWithKeyFile initializes audit logger with a specific key file path
func NewLoggerWithKeyFile(keyFilePath string) (*Logger, error) {
	l := &Logger{
		entries:      make([]models.AuditLogEntry, 0),
		checkpoints:  make([]AuditCheckpoint, 0),
		lastHash:     "GENESIS_SENTINEL_ROOT",
		keyFilePath:  keyFilePath,
		keyRing:      make(map[string][]byte),
		currentKeyID: "root-key-1",
	}

	if err := l.loadOrGenerateKey(); err != nil {
		return nil, fmt.Errorf("secure audit key initialization failed: %w", err)
	}

	return l, nil
}

func (l *Logger) loadOrGenerateKey() error {
	if _, err := os.Stat(l.keyFilePath); err == nil {
		// Keyfile exists - load persisted keys
		data, err := os.ReadFile(l.keyFilePath)
		if err != nil {
			return fmt.Errorf("failed to read persisted audit keyfile: %w", err)
		}

		var ks persistedKeyStore
		if err := json.Unmarshal(data, &ks); err == nil && len(ks.Keys) > 0 {
			l.currentKeyID = ks.CurrentKeyID
			for kid, khex := range ks.Keys {
				rawKey, err := hex.DecodeString(khex)
				if err == nil && len(rawKey) >= 16 {
					l.keyRing[kid] = rawKey
				}
			}
			if active, ok := l.keyRing[l.currentKeyID]; ok {
				l.checkpointKey = active
				return nil
			}
		}

		// Backward compatibility for raw 32-byte binary keyfile
		if len(data) >= 32 {
			l.checkpointKey = make([]byte, 32)
			copy(l.checkpointKey, data[:32])
			l.keyRing[l.currentKeyID] = l.checkpointKey
			return nil
		}
	}

	// Keyfile does not exist - generate fresh key from OS cryptographic random source
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		// Fail securely: NEVER fallback to deterministic values
		return fmt.Errorf("failed to generate cryptographic key from OS random source: %w", err)
	}

	l.checkpointKey = key
	l.keyRing[l.currentKeyID] = key

	return l.persistKeys()
}

func (l *Logger) persistKeys() error {
	ks := persistedKeyStore{
		CurrentKeyID: l.currentKeyID,
		Keys:         make(map[string]string),
	}
	for kid, kbytes := range l.keyRing {
		ks.Keys[kid] = hex.EncodeToString(kbytes)
	}

	data, err := json.MarshalIndent(ks, "", "  ")
	if err != nil {
		return err
	}

	// Persist with restrictive 0600 file permissions
	return os.WriteFile(l.keyFilePath, data, 0600)
}

// RotateCheckpointKey generates a new active HMAC key while preserving prior keys for checkpoint verification
func (l *Logger) RotateCheckpointKey() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return "", fmt.Errorf("failed to generate cryptographic key for rotation: %w", err)
	}

	newKeyID := fmt.Sprintf("key-%d", time.Now().UTC().UnixNano())
	l.keyRing[newKeyID] = newKey
	l.currentKeyID = newKeyID
	l.checkpointKey = newKey

	if err := l.persistKeys(); err != nil {
		return "", fmt.Errorf("failed to persist rotated audit keys: %w", err)
	}

	return newKeyID, nil
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
	l.keyRing[l.currentKeyID] = l.checkpointKey
	_ = l.persistKeys()
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
		KeyID:      l.currentKeyID,
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

	// Verify signed HMAC checkpoints across key ring history
	for _, cp := range l.checkpoints {
		if cp.Index >= len(l.entries) {
			return false, cp.Index, fmt.Errorf("checkpoint index %d exceeds ledger size", cp.Index)
		}
		entry := l.entries[cp.Index]
		if entry.EntryHash != cp.EntryHash {
			return false, cp.Index, fmt.Errorf("checkpoint hash mismatch at entry %d", cp.Index)
		}

		// Select key from key ring corresponding to checkpoint key_id
		verKey := l.checkpointKey
		if cp.KeyID != "" {
			if ringKey, exists := l.keyRing[cp.KeyID]; exists {
				verKey = ringKey
			} else {
				return false, cp.Index, fmt.Errorf("checkpoint key ID %s not found in keyring", cp.KeyID)
			}
		}

		mac := hmac.New(sha256.New, verKey)
		macInput := fmt.Sprintf("%d|%s|%s|%s", cp.Index, entry.ID, entry.EntryHash, entry.Timestamp.Format(time.RFC3339Nano))
		mac.Write([]byte(macInput))
		expectedDigest := hex.EncodeToString(mac.Sum(nil))

		if subtle.ConstantTimeCompare([]byte(expectedDigest), []byte(cp.HMACDigest)) != 1 {
			return false, cp.Index, fmt.Errorf("checkpoint HMAC signature forged or corrupted at index %d", cp.Index)
		}
	}

	return true, len(l.entries), nil
}

// ExportSignedLedger writes an authenticated, signed audit snapshot with HMAC integrity signature
func (l *Logger) ExportSignedLedger(exportPath string) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	cleanPath := filepath.Clean(exportPath)
	content := map[string]interface{}{
		"version":      "2.0",
		"exported_at":  time.Now().UTC().Format(time.RFC3339),
		"entry_count":  len(l.entries),
		"entries":      l.entries,
		"checkpoints":  l.checkpoints,
		"head_hash":    l.lastHash,
		"key_id":       l.currentKeyID,
	}

	rawJSON, err := json.MarshalIndent(content, "  ", "  ")
	if err != nil {
		return err
	}

	// Compute digital HMAC-SHA256 signature across entire ledger payload
	mac := hmac.New(sha256.New, l.checkpointKey)
	mac.Write(rawJSON)
	signature := hex.EncodeToString(mac.Sum(nil))

	exportPackage := struct {
		Payload   json.RawMessage `json:"payload"`
		Signature string          `json:"signature"`
		KeyID     string          `json:"key_id"`
	}{
		Payload:   json.RawMessage(rawJSON),
		Signature: signature,
		KeyID:     l.currentKeyID,
	}

	data, err := json.MarshalIndent(exportPackage, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cleanPath, data, 0600)
}

// VerifyExportedLedger cryptographically verifies an exported ledger archive
func (l *Logger) VerifyExportedLedger(exportPath string) (bool, error) {
	data, err := os.ReadFile(filepath.Clean(exportPath))
	if err != nil {
		return false, err
	}

	var pkg struct {
		Payload   json.RawMessage `json:"payload"`
		Signature string          `json:"signature"`
		KeyID     string          `json:"key_id"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, errors.New("invalid export archive structure")
	}

	l.mu.RLock()
	verKey, exists := l.keyRing[pkg.KeyID]
	if !exists {
		verKey = l.checkpointKey
	}
	l.mu.RUnlock()

	mac := hmac.New(sha256.New, verKey)
	mac.Write(pkg.Payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expectedSig), []byte(pkg.Signature)) != 1 {
		return false, errors.New("cryptographic signature mismatch: exported ledger has been tampered with")
	}

	return true, nil
}

// SignPayload creates an HMAC-SHA256 signature for arbitrary data using the active checkpoint key
func (l *Logger) SignPayload(data []byte) (string, string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	mac := hmac.New(sha256.New, l.checkpointKey)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), l.currentKeyID
}

// VerifyPayloadSignature verifies an HMAC-SHA256 signature against the keyring
func (l *Logger) VerifyPayloadSignature(data []byte, sigHex, keyID string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	verKey, exists := l.keyRing[keyID]
	if !exists {
		verKey = l.checkpointKey
	}
	mac := hmac.New(sha256.New, verKey)
	mac.Write(data)
	expected := hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sigHex)) == 1
}

