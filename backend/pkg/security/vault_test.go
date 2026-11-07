package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultEncryptionAndDecryption(t *testing.T) {
	tmpDir := t.TempDir()
	vault, err := NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	secret := "CCTV@Admin#2026!SecureKey"

	enc, err := vault.Encrypt(secret)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if !vault.IsEncrypted(enc) {
		t.Errorf("expected encrypted string to have prefix %s, got: %s", EncryptedPrefix, enc)
	}

	if strings.Contains(enc, secret) {
		t.Errorf("ciphertext exposes plaintext secret!")
	}

	// Decrypt
	dec, err := vault.Decrypt(enc)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if dec != secret {
		t.Errorf("expected decrypted secret %q, got %q", secret, dec)
	}
}

func TestVaultNoncedRandomness(t *testing.T) {
	tmpDir := t.TempDir()
	vault, err := NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	secret := "SamePassword123"
	enc1, _ := vault.Encrypt(secret)
	enc2, _ := vault.Encrypt(secret)

	if enc1 == enc2 {
		t.Errorf("AES-GCM nonce reuse detected: identical ciphertexts for same plaintext")
	}

	dec1, _ := vault.Decrypt(enc1)
	dec2, _ := vault.Decrypt(enc2)
	if dec1 != secret || dec2 != secret {
		t.Errorf("decryption mismatch")
	}
}

func TestVaultTamperingDetection(t *testing.T) {
	tmpDir := t.TempDir()
	vault, err := NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create vault: %v", err)
	}

	secret := "CriticalPassword"
	enc, _ := vault.Encrypt(secret)

	// Tamper with the ciphertext by flipping a character in the base64 string
	tampered := enc[:len(enc)-4] + "AAAA"

	_, err = vault.Decrypt(tampered)
	if err == nil {
		t.Fatalf("expected decryption error on tampered ciphertext, but succeeded")
	}
}

func TestVaultMissingOrInvalidKey(t *testing.T) {
	// Vault with empty key
	v := &Vault{key: []byte{}}
	_, err := v.Encrypt("test")
	if err != ErrMissingKey {
		t.Errorf("expected ErrMissingKey, got %v", err)
	}

	_, err = v.Decrypt(EncryptedPrefix + "dGVzdA==")
	if err != ErrMissingKey {
		t.Errorf("expected ErrMissingKey, got %v", err)
	}
}

func TestVaultKeyPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, DefaultKeyFile)

	// 1. Initial vault creates key
	v1, err := NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create vault 1: %v", err)
	}

	if _, err := os.Stat(keyFile); err != nil {
		t.Fatalf("expected key file to exist at %s", keyFile)
	}

	secret := "PersistentSecret99"
	enc, err := v1.Encrypt(secret)
	if err != nil {
		t.Fatalf("v1 encryption failed: %v", err)
	}

	// 2. Second vault loads existing key from same directory
	v2, err := NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to create vault 2: %v", err)
	}

	dec, err := v2.Decrypt(enc)
	if err != nil {
		t.Fatalf("v2 decryption failed with reloaded key: %v", err)
	}

	if dec != secret {
		t.Errorf("expected decrypted secret %q, got %q", secret, dec)
	}
}

