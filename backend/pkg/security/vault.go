package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	EncryptedPrefix = "$enc$gcm$"
	DefaultKeyFile  = "sentinel_vault.key"
)

var (
	ErrTamperedCiphertext = errors.New("security: ciphertext authentication failed (tampering detected)")
	ErrMissingKey         = errors.New("security: encryption master key is missing or invalid")
	ErrPlaintextPassword  = errors.New("security: password is unencrypted plaintext")
)

// Vault manages AES-256-GCM encryption and decryption for sensitive device credentials
type Vault struct {
	mu      sync.RWMutex
	key     []byte
	keyPath string
}

// NewVault initializes or loads an AES-256 master key
// The key is stored separately from the database store file to ensure separation of secrets.
func NewVault(keyDirectory string) (*Vault, error) {
	if keyDirectory == "" {
		keyDirectory = "."
	}
	keyPath := filepath.Join(keyDirectory, DefaultKeyFile)

	v := &Vault{
		keyPath: keyPath,
	}

	// 1. Check if environment variable CCTV_VAULT_KEY is provided
	if envKey := os.Getenv("CCTV_VAULT_KEY"); envKey != "" {
		hash := sha256.Sum256([]byte(envKey))
		v.key = hash[:]
		return v, nil
	}

	// 2. Load existing key from protected key file
	if data, err := os.ReadFile(keyPath); err == nil && len(data) >= 32 {
		v.key = data[:32]
		return v, nil
	}

	// 3. Generate a new secure 256-bit random key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	// Save key with restricted permissions (0600)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("failed to write vault key: %w", err)
	}

	v.key = key
	return v, nil
}

// NewVaultWithKey initializes a vault with a specific 32-byte key (useful for tests)
func NewVaultWithKey(key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, ErrMissingKey
	}
	return &Vault{
		key: key,
	}, nil
}

// IsEncrypted checks if a string is already encrypted with the GCM prefix
func (v *Vault) IsEncrypted(val string) bool {
	return strings.HasPrefix(val, EncryptedPrefix)
}

// Encrypt encrypts a plaintext secret using AES-256-GCM with a random 12-byte nonce
func (v *Vault) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if v.IsEncrypted(plaintext) {
		return plaintext, nil // already encrypted
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(v.key) != 32 {
		return "", ErrMissingKey
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// 12-byte standard GCM nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return EncryptedPrefix + encoded, nil
}

// Decrypt decrypts an authenticated AES-256-GCM ciphertext
func (v *Vault) Decrypt(encryptedVal string) (string, error) {
	if encryptedVal == "" {
		return "", nil
	}
	if !v.IsEncrypted(encryptedVal) {
		return encryptedVal, nil // fallback if plaintext during migration
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(v.key) != 32 {
		return "", ErrMissingKey
	}

	raw := strings.TrimPrefix(encryptedVal, EncryptedPrefix)
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrTamperedCiphertext
	}

	return string(plaintext), nil
}
