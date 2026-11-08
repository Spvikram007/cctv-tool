package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrDeviceNotFound      = errors.New("desktop_auth: native client device not registered")
	ErrDeviceDeactivated   = errors.New("desktop_auth: native client device is deactivated")
	ErrDeviceAlreadyExists = errors.New("desktop_auth: native client device ID already registered")
	ErrInvalidDeviceProof  = errors.New("desktop_auth: cryptographic proof failed; invalid device signature")
	ErrChallengeExpired    = errors.New("desktop_auth: authentication challenge has expired")
	ErrChallengeNotFound   = errors.New("desktop_auth: challenge not found or already consumed")
)

// DesktopDevice represents an enrolled workstation allowed to authenticate native desktop clients
type DesktopDevice struct {
	ClientID   string    `json:"client_id"`
	DeviceName string    `json:"device_name"`
	DeviceKey  string    `json:"-"` // Cryptographic secret key provisioned to OS credential store (never sent to browsers)
	EnrolledAt time.Time `json:"enrolled_at"`
	LastAuth   time.Time `json:"last_auth,omitempty"`
	Active     bool      `json:"active"`
}

// ChallengeRecord tracks a short-lived cryptographic nonce issued to a specific native client
type ChallengeRecord struct {
	Challenge string
	ClientID  string
	ExpiresAt time.Time
	Consumed  bool
}

// DesktopAuthManager handles registration, challenge generation, and proof verification for native desktop clients
type DesktopAuthManager struct {
	mu         sync.RWMutex
	devices    map[string]*DesktopDevice
	challenges map[string]*ChallengeRecord
	ttl        time.Duration
}

// NewDesktopAuthManager initializes a new native desktop client authentication manager
func NewDesktopAuthManager() *DesktopAuthManager {
	return &DesktopAuthManager{
		devices:    make(map[string]*DesktopDevice),
		challenges: make(map[string]*ChallengeRecord),
		ttl:        60 * time.Second, // Challenges are short-lived (60 seconds)
	}
}

// RegisterDevice registers a new native desktop client device and generates a unique workstation key
func (m *DesktopAuthManager) RegisterDevice(clientID, deviceName string) (*DesktopDevice, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	deviceName = strings.TrimSpace(deviceName)
	if clientID == "" {
		return nil, "", errors.New("desktop_auth: client_id cannot be empty")
	}
	if deviceName == "" {
		deviceName = clientID
	}
	if _, exists := m.devices[clientID]; exists {
		return nil, "", ErrDeviceAlreadyExists
	}

	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("desktop_auth: failed to generate cryptographic device key: %w", err)
	}
	deviceKey := hex.EncodeToString(keyBytes)

	dev := &DesktopDevice{
		ClientID:   clientID,
		DeviceName: deviceName,
		DeviceKey:  deviceKey,
		EnrolledAt: time.Now().UTC(),
		Active:     true,
	}
	m.devices[clientID] = dev

	return dev, deviceKey, nil
}

// RegisterDeviceWithKey allows enrolling a device with an existing securely generated key (e.g. from tests or secure seeding)
func (m *DesktopAuthManager) RegisterDeviceWithKey(clientID, deviceName, deviceKey string) (*DesktopDevice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("desktop_auth: client_id cannot be empty")
	}
	if _, exists := m.devices[clientID]; exists {
		return nil, ErrDeviceAlreadyExists
	}
	if len(deviceKey) < 32 {
		return nil, errors.New("desktop_auth: device key must be at least 32 characters")
	}

	dev := &DesktopDevice{
		ClientID:   clientID,
		DeviceName: deviceName,
		DeviceKey:  deviceKey,
		EnrolledAt: time.Now().UTC(),
		Active:     true,
	}
	m.devices[clientID] = dev
	return dev, nil
}

// GetDevice retrieves device metadata (excluding secret key)
func (m *DesktopAuthManager) GetDevice(clientID string) (*DesktopDevice, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dev, exists := m.devices[clientID]
	if !exists {
		return nil, ErrDeviceNotFound
	}
	// Return copy without leaking raw pointer
	return &DesktopDevice{
		ClientID:   dev.ClientID,
		DeviceName: dev.DeviceName,
		EnrolledAt: dev.EnrolledAt,
		LastAuth:   dev.LastAuth,
		Active:     dev.Active,
	}, nil
}

// ListDevices returns all registered native client devices
func (m *DesktopAuthManager) ListDevices() []*DesktopDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*DesktopDevice, 0, len(m.devices))
	for _, dev := range m.devices {
		list = append(list, &DesktopDevice{
			ClientID:   dev.ClientID,
			DeviceName: dev.DeviceName,
			EnrolledAt: dev.EnrolledAt,
			LastAuth:   dev.LastAuth,
			Active:     dev.Active,
		})
	}
	return list
}

// DeactivateDevice disables a registered native device
func (m *DesktopAuthManager) DeactivateDevice(clientID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, exists := m.devices[clientID]
	if !exists {
		return ErrDeviceNotFound
	}
	dev.Active = false
	return nil
}

// IssueChallenge generates a single-use cryptographic challenge for a registered native client
func (m *DesktopAuthManager) IssueChallenge(clientID string) (*ChallengeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	dev, exists := m.devices[clientID]
	if !exists {
		return nil, ErrDeviceNotFound
	}
	if !dev.Active {
		return nil, ErrDeviceDeactivated
	}

	// Clean expired challenges
	now := time.Now().UTC()
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
		ClientID:  clientID,
		ExpiresAt: now.Add(m.ttl),
		Consumed:  false,
	}
	m.challenges[challengeStr] = rec

	return rec, nil
}

// VerifyProof verifies the HMAC-SHA256 proof computed by the native client using its registered device key
func (m *DesktopAuthManager) VerifyProof(clientID, challenge, proofHex string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientID = strings.TrimSpace(clientID)
	dev, exists := m.devices[clientID]
	if !exists {
		return ErrDeviceNotFound
	}
	if !dev.Active {
		return ErrDeviceDeactivated
	}

	rec, exists := m.challenges[challenge]
	if !exists || rec.Consumed {
		return ErrChallengeNotFound
	}

	// Challenge must be bound to the claimed clientID
	if subtle.ConstantTimeCompare([]byte(rec.ClientID), []byte(clientID)) != 1 {
		return ErrInvalidDeviceProof
	}

	// Verify expiry
	if time.Now().UTC().After(rec.ExpiresAt) {
		delete(m.challenges, challenge)
		return ErrChallengeExpired
	}

	// Consume challenge immediately (single use protection against replay attacks)
	rec.Consumed = true
	delete(m.challenges, challenge)

	// Compute expected HMAC-SHA256(challenge, deviceKey)
	mac := hmac.New(sha256.New, []byte(dev.DeviceKey))
	mac.Write([]byte(challenge))
	expectedProof := mac.Sum(nil)

	providedProof, err := hex.DecodeString(strings.TrimSpace(proofHex))
	if err != nil {
		return ErrInvalidDeviceProof
	}

	if !hmac.Equal(providedProof, expectedProof) {
		return ErrInvalidDeviceProof
	}

	dev.LastAuth = time.Now().UTC()
	return nil
}

// ComputeProofHelper is a helper for native clients and unit tests to compute valid challenge proofs
func ComputeProofHelper(deviceKey, challenge string) string {
	mac := hmac.New(sha256.New, []byte(deviceKey))
	mac.Write([]byte(challenge))
	return hex.EncodeToString(mac.Sum(nil))
}
