package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrDeviceNotFound      = errors.New("desktop_auth: native client device not registered")
	ErrDeviceDeactivated   = errors.New("desktop_auth: native client device is revoked or deactivated")
	ErrDeviceAlreadyExists = errors.New("desktop_auth: native client device ID already registered")
	ErrDeviceAlreadyActive = errors.New("desktop_auth: active device already exists; cannot overwrite active device")
	ErrDeviceNotApproved   = errors.New("desktop_auth: device enrollment pending administrator approval")
	ErrEnrollmentExpired   = errors.New("desktop_auth: device enrollment request has expired")
	ErrInvalidDeviceProof  = errors.New("desktop_auth: cryptographic proof failed; invalid Ed25519 device signature")
	ErrInvalidPublicKey    = errors.New("desktop_auth: invalid Ed25519 public key (must be 32 bytes)")
	ErrChallengeExpired    = errors.New("desktop_auth: authentication challenge has expired")
	ErrChallengeNotFound   = errors.New("desktop_auth: challenge not found or already consumed")
)

// DesktopApprovalStatus defines the lifecycle states of a native client device
type DesktopApprovalStatus string

const (
	ApprovalPending  DesktopApprovalStatus = "PENDING"
	ApprovalApproved DesktopApprovalStatus = "APPROVED"
	ApprovalRejected DesktopApprovalStatus = "REJECTED"
)

// DesktopStatus defines active vs revoked state
type DesktopStatus string

const (
	StatusActive  DesktopStatus = "ACTIVE"
	StatusRevoked DesktopStatus = "REVOKED"
)

// DesktopDeviceRecord represents a persistently stored workstation device identity
type DesktopDeviceRecord struct {
	ID             string     `json:"id"`
	ClientName     string     `json:"client_name"`
	OS             string     `json:"os"`
	HostName       string     `json:"hostname"`
	PublicKey      string     `json:"public_key"`      // Base64-encoded Ed25519 32-byte public key
	KeyFingerprint string     `json:"key_fingerprint"` // SHA-256 fingerprint formatted as "SHA256:<hex>"
	KeyVersion     int        `json:"key_version"`
	ApprovalStatus string     `json:"approval_status"` // PENDING, APPROVED, REJECTED
	Status         string     `json:"status"`          // ACTIVE, REVOKED
	ApprovedBy     string     `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	Active         bool       `json:"active"`
}

// DesktopDeviceStore persists desktop client device registrations
type DesktopDeviceStore interface {
	UpsertDesktopDevice(dev DesktopDeviceRecord) error
	GetDesktopDevice(id string) (DesktopDeviceRecord, bool)
	ListDesktopDevices() []DesktopDeviceRecord
	DeleteDesktopDevice(id string) error
}

// ChallengeRecord tracks a short-lived single-use cryptographic challenge nonce
type ChallengeRecord struct {
	Challenge string
	DeviceID  string
	ExpiresAt time.Time
	Consumed  bool
}

// DesktopEnrollmentRequest defines the payload submitted by a native client during local keypair enrollment
type DesktopEnrollmentRequest struct {
	ID         string `json:"id"`
	DeviceID   string `json:"device_id"` // Alias for ID
	ClientID   string `json:"client_id"` // Alias for ID
	ClientName string `json:"client_name"`
	OS         string `json:"os"`
	HostName   string `json:"hostname"`
	PublicKey  string `json:"public_key"` // Base64 or Hex encoded Ed25519 public key
}

// DesktopAuthManager manages asymmetric Ed25519 device authentication and persistent registry
type DesktopAuthManager struct {
	mu         sync.RWMutex
	store      DesktopDeviceStore
	devices    map[string]*DesktopDeviceRecord
	challenges map[string]*ChallengeRecord
	ttl        time.Duration
}

// NewDesktopAuthManager creates an asymmetric device authentication manager backed by a persistent store
func NewDesktopAuthManager(store DesktopDeviceStore) *DesktopAuthManager {
	mgr := &DesktopAuthManager{
		store:      store,
		devices:    make(map[string]*DesktopDeviceRecord),
		challenges: make(map[string]*ChallengeRecord),
		ttl:        60 * time.Second, // Challenges are short-lived (60 seconds)
	}

	// Reload existing registered devices from persistent store
	if store != nil {
		for _, dev := range store.ListDesktopDevices() {
			devCopy := dev
			mgr.devices[dev.ID] = &devCopy
		}
	}

	return mgr
}

// DecodeEd25519PublicKey decodes a public key from base64 or hex
func DecodeEd25519PublicKey(pubKeyStr string) (ed25519.PublicKey, error) {
	pubKeyStr = strings.TrimSpace(pubKeyStr)
	// Try base64 standard
	b, err := base64.StdEncoding.DecodeString(pubKeyStr)
	if err == nil && len(b) == ed25519.PublicKeySize {
		return ed25519.PublicKey(b), nil
	}
	// Try base64 URL
	b, err = base64.URLEncoding.DecodeString(pubKeyStr)
	if err == nil && len(b) == ed25519.PublicKeySize {
		return ed25519.PublicKey(b), nil
	}
	// Try hex
	b, err = hex.DecodeString(pubKeyStr)
	if err == nil && len(b) == ed25519.PublicKeySize {
		return ed25519.PublicKey(b), nil
	}
	return nil, ErrInvalidPublicKey
}

// ComputeFingerprint computes "SHA256:<hex>" for an Ed25519 public key
func ComputeFingerprint(pubKey ed25519.PublicKey) string {
	sum := sha256.Sum256(pubKey)
	return fmt.Sprintf("SHA256:%s", hex.EncodeToString(sum[:]))
}

// SubmitEnrollment submits a native workstation's locally generated public key for administrator approval
func (m *DesktopAuthManager) SubmitEnrollment(req DesktopEnrollmentRequest) (*DesktopDeviceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = strings.TrimSpace(req.DeviceID)
	}
	if id == "" {
		id = strings.TrimSpace(req.ClientID)
	}
	if id == "" {
		return nil, errors.New("desktop_auth: device ID cannot be empty")
	}

	pubKeyBytes, err := DecodeEd25519PublicKey(req.PublicKey)
	if err != nil {
		return nil, err
	}

	fingerprint := ComputeFingerprint(pubKeyBytes)
	pubKeyBase64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	// Check if already registered
	if existing, exists := m.devices[id]; exists {
		// Active, approved devices cannot be silently overwritten
		if existing.Status == string(StatusActive) && existing.ApprovalStatus == string(ApprovalApproved) {
			return nil, ErrDeviceAlreadyActive
		}
	}

	clientName := strings.TrimSpace(req.ClientName)
	if clientName == "" {
		clientName = id
	}

	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour) // Pending enrollment expires in 24 hours

	rec := &DesktopDeviceRecord{
		ID:             id,
		ClientName:     clientName,
		OS:             req.OS,
		HostName:       req.HostName,
		PublicKey:      pubKeyBase64,
		KeyFingerprint: fingerprint,
		KeyVersion:     1,
		ApprovalStatus: string(ApprovalPending),
		Status:         string(StatusActive),
		ExpiresAt:      &expiresAt,
		CreatedAt:      now,
		LastSeenAt:     now,
		Active:         false, // Not active until approved
	}

	m.devices[id] = rec
	if m.store != nil {
		_ = m.store.UpsertDesktopDevice(*rec)
	}

	return rec, nil
}

// ApproveDevice approves a pending workstation enrollment (Administrator only)
func (m *DesktopAuthManager) ApproveDevice(id, adminUser string) (*DesktopDeviceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, exists := m.devices[id]
	if !exists {
		return nil, ErrDeviceNotFound
	}

	now := time.Now().UTC()
	if dev.ExpiresAt != nil && now.After(*dev.ExpiresAt) {
		return nil, ErrEnrollmentExpired
	}

	dev.ApprovalStatus = string(ApprovalApproved)
	dev.Status = string(StatusActive)
	dev.Active = true
	dev.ApprovedBy = adminUser
	dev.ApprovedAt = &now

	if m.store != nil {
		_ = m.store.UpsertDesktopDevice(*dev)
	}

	return dev, nil
}

// RejectDevice rejects a pending workstation enrollment
func (m *DesktopAuthManager) RejectDevice(id, adminUser string) (*DesktopDeviceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, exists := m.devices[id]
	if !exists {
		return nil, ErrDeviceNotFound
	}

	dev.ApprovalStatus = string(ApprovalRejected)
	dev.Status = string(StatusRevoked)
	dev.Active = false

	if m.store != nil {
		_ = m.store.UpsertDesktopDevice(*dev)
	}

	return dev, nil
}

// RevokeDevice permanently revokes an active workstation device identity
func (m *DesktopAuthManager) RevokeDevice(id, adminUser string) (*DesktopDeviceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, exists := m.devices[id]
	if !exists {
		return nil, ErrDeviceNotFound
	}

	dev.Status = string(StatusRevoked)
	dev.ApprovalStatus = string(ApprovalRejected)
	dev.Active = false

	if m.store != nil {
		_ = m.store.UpsertDesktopDevice(*dev)
	}

	return dev, nil
}

// RotateDeviceKey rotates the public key of an approved device
func (m *DesktopAuthManager) RotateDeviceKey(id, newPublicKeyStr, adminUser string) (*DesktopDeviceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, exists := m.devices[id]
	if !exists {
		return nil, ErrDeviceNotFound
	}
	if dev.Status != string(StatusActive) || dev.ApprovalStatus != string(ApprovalApproved) {
		return nil, ErrDeviceDeactivated
	}

	pubKeyBytes, err := DecodeEd25519PublicKey(newPublicKeyStr)
	if err != nil {
		return nil, err
	}

	dev.PublicKey = base64.StdEncoding.EncodeToString(pubKeyBytes)
	dev.KeyFingerprint = ComputeFingerprint(pubKeyBytes)
	dev.KeyVersion++
	dev.LastSeenAt = time.Now().UTC()

	if m.store != nil {
		_ = m.store.UpsertDesktopDevice(*dev)
	}

	return dev, nil
}

// GetDevice retrieves device metadata
func (m *DesktopAuthManager) GetDevice(id string) (*DesktopDeviceRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dev, exists := m.devices[id]
	if !exists {
		return nil, ErrDeviceNotFound
	}
	devCopy := *dev
	return &devCopy, nil
}

// ListDevices returns all registered workstation devices
func (m *DesktopAuthManager) ListDevices() []DesktopDeviceRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]DesktopDeviceRecord, 0, len(m.devices))
	for _, dev := range m.devices {
		list = append(list, *dev)
	}
	return list
}

// IssueChallenge issues a single-use, 60-second cryptographic challenge nonce for an approved device
func (m *DesktopAuthManager) IssueChallenge(id string) (*ChallengeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	dev, exists := m.devices[id]
	if !exists {
		return nil, ErrDeviceNotFound
	}
	if dev.Status == string(StatusRevoked) || dev.ApprovalStatus == string(ApprovalRejected) {
		return nil, ErrDeviceDeactivated
	}
	if dev.ApprovalStatus != string(ApprovalApproved) {
		return nil, ErrDeviceNotApproved
	}
	if dev.Status != string(StatusActive) || !dev.Active {
		return nil, ErrDeviceDeactivated
	}

	now := time.Now().UTC()
	// Clean expired challenges
	for k, c := range m.challenges {
		if now.After(c.ExpiresAt) || c.Consumed {
			delete(m.challenges, k)
		}
	}

	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("desktop_auth: failed to generate challenge nonce: %w", err)
	}

	challengeStr := fmt.Sprintf("ch-%s", hex.EncodeToString(nonceBytes))
	rec := &ChallengeRecord{
		Challenge: challengeStr,
		DeviceID:  id,
		ExpiresAt: now.Add(m.ttl),
		Consumed:  false,
	}
	m.challenges[challengeStr] = rec

	return rec, nil
}

// VerifyProof verifies the Ed25519 signature of the challenge.
// CRITICAL: The challenge is ATOMICALLY CONSUMED on any verification attempt (whether it succeeds or fails).
func (m *DesktopAuthManager) VerifyProof(id, challenge, signatureStr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	dev, exists := m.devices[id]
	if !exists {
		return ErrDeviceNotFound
	}
	if dev.Status == string(StatusRevoked) || dev.ApprovalStatus == string(ApprovalRejected) {
		return ErrDeviceDeactivated
	}
	if dev.ApprovalStatus != string(ApprovalApproved) {
		return ErrDeviceNotApproved
	}
	if dev.Status != string(StatusActive) || !dev.Active {
		return ErrDeviceDeactivated
	}

	rec, exists := m.challenges[challenge]
	if !exists || rec.Consumed {
		return ErrChallengeNotFound
	}

	// ATOMIC CONSUMPTION: Delete challenge immediately to prevent any replay
	rec.Consumed = true
	delete(m.challenges, challenge)

	// Constant-time check: challenge must be bound to the claimed device ID
	if subtle.ConstantTimeCompare([]byte(rec.DeviceID), []byte(id)) != 1 {
		return ErrInvalidDeviceProof
	}

	// Verify expiry
	if time.Now().UTC().After(rec.ExpiresAt) {
		return ErrChallengeExpired
	}

	// Decode signature (support base64 and hex)
	sigBytes, err := decodeSignature(signatureStr)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return ErrInvalidDeviceProof
	}

	// Decode device's registered Ed25519 public key
	pubKeyBytes, err := DecodeEd25519PublicKey(dev.PublicKey)
	if err != nil {
		return ErrInvalidDeviceProof
	}

	// Verify asymmetric signature: ed25519.Verify(pubKey, message, signature)
	if !ed25519.Verify(pubKeyBytes, []byte(challenge), sigBytes) {
		return ErrInvalidDeviceProof
	}

	dev.LastSeenAt = time.Now().UTC()
	if m.store != nil {
		_ = m.store.UpsertDesktopDevice(*dev)
	}

	return nil
}

func decodeSignature(sigStr string) ([]byte, error) {
	sigStr = strings.TrimSpace(sigStr)
	if b, err := base64.StdEncoding.DecodeString(sigStr); err == nil && len(b) == ed25519.SignatureSize {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(sigStr); err == nil && len(b) == ed25519.SignatureSize {
		return b, nil
	}
	if b, err := hex.DecodeString(sigStr); err == nil && len(b) == ed25519.SignatureSize {
		return b, nil
	}
	return nil, errors.New("invalid signature format")
}

// GenerateClientKeyPair is a helper for native desktop clients to locally generate an Ed25519 keypair
func GenerateClientKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// SignChallenge is a helper for native desktop clients to sign a challenge with their local private key
func SignChallenge(privKey ed25519.PrivateKey, challenge string) string {
	sig := ed25519.Sign(privKey, []byte(challenge))
	return base64.StdEncoding.EncodeToString(sig)
}
