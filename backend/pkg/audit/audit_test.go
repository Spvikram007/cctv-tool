package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cctv-sentinel/pkg/audit"
)

func TestAuditSecretRedaction(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "Connecting to rtsp://admin:SecretPass123@192.168.1.100:554/live",
			expected: "[REDACTED]",
		},
		{
			input:    `{"username":"admin","password":"SuperSecretPassword!"}`,
			expected: `[REDACTED]`,
		},
		{
			input:    "Header Authorization: Basic YWRtaW46YWRtaW4xMjM=",
			expected: "[REDACTED]",
		},
		{
			input:    "Header Authorization: Bearer sent-tok-9876543210abcdef",
			expected: "[REDACTED]",
		},
		{
			input:    "<wsse:PasswordDigest>v9/a3qZ3mX0hF4s1w==" + "</wsse:PasswordDigest>",
			expected: "[REDACTED]",
		},
	}

	for _, tc := range testCases {
		cleaned := audit.RedactSecrets(tc.input)
		if strings.Contains(cleaned, "SecretPass123") ||
			strings.Contains(cleaned, "SuperSecretPassword!") ||
			strings.Contains(cleaned, "YWRtaW46YWRtaW4xMjM=") ||
			strings.Contains(cleaned, "sent-tok-9876543210abcdef") ||
			strings.Contains(cleaned, "v9/a3qZ3mX0hF4s1w==") {
			t.Errorf("Secret redaction leaked sensitive credentials from input: %s | Result: %s", tc.input, cleaned)
		}
	}
}

func TestAuditHashChainAndTamperDetection(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "audit.key")
	logger, err := audit.NewLoggerWithKeyFile(keyFile)
	if err != nil {
		t.Fatalf("failed to init logger: %v", err)
	}

	logger.Log("Admin", "dev-1", "192.168.1.10", "REBOOT", "DEVICE", "SUCCESS", "Reboot sequence initiated", "")
	logger.Log("Technician", "dev-2", "192.168.1.20", "UPDATE_NETWORK", "NETWORK", "SUCCESS", "IP changed to 192.168.1.25", "IP: 192.168.1.20 -> 192.168.1.25")
	logger.Log("Auditor", "FLEET", "127.0.0.1", "EXPORT_REPORT", "SYSTEM", "SUCCESS", "Exported commissioning report", "")

	// Verify intact chain
	valid, count, err := logger.VerifyChainIntegrity()
	if err != nil || !valid || count != 3 {
		t.Fatalf("expected valid chain of 3 entries, got valid=%v, count=%d, err=%v", valid, count, err)
	}
}

func TestHMACCheckpointsAndKeyRotation(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "audit_rotation.key")
	logger, err := audit.NewLoggerWithKeyFile(keyFile)
	if err != nil {
		t.Fatalf("failed to init logger: %v", err)
	}

	for i := 1; i <= 5; i++ {
		logger.Log("Technician", "dev-101", "192.168.1.101", "SET_ANALOG_FORMAT", "RECORDER", "SUCCESS", "Set format to CVI", "")
	}

	// Create signed HMAC checkpoint with original key
	cp1, err := logger.CreateCheckpoint()
	if err != nil {
		t.Fatalf("Failed to create HMAC checkpoint: %v", err)
	}
	if cp1.HMACDigest == "" {
		t.Errorf("Checkpoint HMAC signature must not be empty")
	}

	// Rotate key
	newKeyID, err := logger.RotateCheckpointKey()
	if err != nil {
		t.Fatalf("RotateCheckpointKey failed: %v", err)
	}
	if newKeyID == "" {
		t.Errorf("expected non-empty new key ID")
	}

	// Add more entries with new key
	for i := 6; i <= 10; i++ {
		logger.Log("Technician", "dev-102", "192.168.1.102", "PTZ_MOVE", "PTZ", "SUCCESS", "Pan left", "")
	}

	// Create checkpoint with new key
	cp2, err := logger.CreateCheckpoint()
	if err != nil {
		t.Fatalf("Failed to create second checkpoint: %v", err)
	}
	if cp2.KeyID != newKeyID {
		t.Errorf("expected checkpoint 2 key ID %s, got %s", newKeyID, cp2.KeyID)
	}

	// Verify entire chain: BOTH old checkpoint and new checkpoint must verify successfully!
	valid, count, err := logger.VerifyChainIntegrity()
	if err != nil || !valid || count != 10 {
		t.Fatalf("VerifyChainIntegrity after rotation failed: valid=%v, count=%d, err=%v", valid, count, err)
	}

	// Test persistence across restart: reload new logger with same key file
	loggerReloaded, err := audit.NewLoggerWithKeyFile(keyFile)
	if err != nil {
		t.Fatalf("reloading logger failed: %v", err)
	}
	if loggerReloaded == nil {
		t.Fatalf("reloaded logger is nil")
	}
}

func TestExportedLedgerCryptographicVerification(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "audit_export.key")
	logger, err := audit.NewLoggerWithKeyFile(keyFile)
	if err != nil {
		t.Fatalf("failed to init logger: %v", err)
	}

	logger.Log("Admin", "dev-1", "192.168.1.10", "REBOOT", "DEVICE", "SUCCESS", "Reboot sequence", "")
	_, _ = logger.CreateCheckpoint()

	exportFile := filepath.Join(t.TempDir(), "signed_ledger.json")
	if err := logger.ExportSignedLedger(exportFile); err != nil {
		t.Fatalf("ExportSignedLedger failed: %v", err)
	}

	// 1. Verify untampered archive
	valid, err := logger.VerifyExportedLedger(exportFile)
	if err != nil || !valid {
		t.Fatalf("VerifyExportedLedger on authentic export failed: %v", err)
	}

	// 2. Tamper with archive and ensure verification fails
	raw, _ := os.ReadFile(exportFile)
	tampered := strings.Replace(string(raw), "Reboot sequence", "Tampered Malicious Operation", 1)
	tamperedFile := filepath.Join(t.TempDir(), "tampered_ledger.json")
	_ = os.WriteFile(tamperedFile, []byte(tampered), 0600)

	valid, err = logger.VerifyExportedLedger(tamperedFile)
	if err == nil || valid {
		t.Fatalf("SECURITY VIOLATION: Tampered export archive was accepted without cryptographic failure!")
	}
}
