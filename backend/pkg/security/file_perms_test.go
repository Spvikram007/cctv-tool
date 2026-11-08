package security_test

import (
	"os"
	"path/filepath"
	"testing"

	"cctv-sentinel/pkg/security"
)

func TestFilePerms_WriteSecureFileAndVerify(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_secure.txt")

	data := []byte("confidential-security-data-12345")
	if err := security.WriteSecureFile(filePath, data); err != nil {
		t.Fatalf("failed to write secure file: %v", err)
	}

	readBack, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read back secure file: %v", err)
	}
	if string(readBack) != string(data) {
		t.Fatalf("content mismatch")
	}

	// Verify permissions
	if err := security.VerifyFilePermissions(filePath); err != nil {
		t.Fatalf("expected file permissions to be verified, got: %v", err)
	}
}

func TestFilePerms_EmptyPath(t *testing.T) {
	if err := security.RestrictFilePermissions(""); err == nil {
		t.Errorf("expected error on empty path")
	}
	if err := security.VerifyFilePermissions(""); err == nil {
		t.Errorf("expected error on empty path")
	}
}

func TestFilePerms_InvalidPathPreventsSensitiveFileCreation(t *testing.T) {
	// Attempting to write secure file to an invalid/non-existent directory fails closed
	invalidPath := filepath.Join(string(filepath.Separator), "nonexistent_dir_00000000", "test.key")
	err := security.WriteSecureFile(invalidPath, []byte("sensitive-key-data"))
	if err == nil {
		t.Fatalf("expected error when writing sensitive file to invalid path, got nil")
	}
}

