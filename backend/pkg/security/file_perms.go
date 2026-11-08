package security

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

var (
	ErrPermissionHardeningFailed = errors.New("file_perms: failed to restrict file permissions")
	ErrPermissionVerifyFailed    = errors.New("file_perms: verification failed; file permissions are not properly restricted")
)

// RestrictFilePermissions secures a sensitive file against unauthorized local access.
// On Windows, it directly executes icacls.exe without shell interpolation:
// - Removes inheritance (/inheritance:r)
// - Grants Full Control only to SYSTEM (*S-1-5-18), Builtin Administrators (*S-1-5-32-544), and executing user.
// - Strictly verifies the resulting ACL to guarantee no broad read permissions exist.
// On Unix/POSIX, it sets file permission mode 0600 (owner read/write only).
func RestrictFilePermissions(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("file_perms: empty file path")
	}

	if runtime.GOOS == "windows" {
		username := os.Getenv("USERNAME")
		if username == "" {
			username = "Administrator"
		}

		// 1. Execute icacls without shell command interpolation
		args := []string{
			path,
			"/inheritance:r",
			"/grant:r", "*S-1-5-18:(F)",
			"/grant:r", "*S-1-5-32-544:(F)",
			"/grant:r", username + ":(F)",
		}

		cmd := exec.Command("icacls.exe", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: icacls failed: %v (output: %s)", ErrPermissionHardeningFailed, err, string(out))
		}

		// 2. Verify resulting ACL
		if err := VerifyFilePermissions(path); err != nil {
			return err
		}

		return nil
	}

	// Unix / macOS: owner read/write only (0600)
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("%w: chmod 0600 failed: %v", ErrPermissionHardeningFailed, err)
	}

	return VerifyFilePermissions(path)
}

// VerifyFilePermissions verifies that a file is strictly restricted to authorized entities.
func VerifyFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: stat failed: %v", ErrPermissionVerifyFailed, err)
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command("icacls.exe", path)
		outBytes, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: icacls query failed: %v", ErrPermissionVerifyFailed, err)
		}
		out := string(outBytes)

		// Verification criteria:
		// 1. Must show Full Control (F)
		if !strings.Contains(out, ":(F)") {
			return fmt.Errorf("%w: file lacks Full Control grant in ACL output", ErrPermissionVerifyFailed)
		}

		// 2. Must NOT contain broad permissions for Everyone, Authenticated Users, or standard Users
		upperOut := strings.ToUpper(out)
		if strings.Contains(upperOut, "BUILTIN\\USERS:(R") ||
			strings.Contains(upperOut, "BUILTIN\\USERS:(M") ||
			strings.Contains(upperOut, "BUILTIN\\USERS:(F") ||
			strings.Contains(upperOut, "NT AUTHORITY\\AUTHENTICATED USERS") ||
			strings.Contains(upperOut, "\\EVERYONE") {
			return fmt.Errorf("%w: file has overly permissive grants for Users or Everyone", ErrPermissionVerifyFailed)
		}

		// 3. Must indicate successful processing
		if !strings.Contains(out, "Successfully processed 1 files") {
			return fmt.Errorf("%w: icacls did not confirm successful processing", ErrPermissionVerifyFailed)
		}

		return nil
	}

	// Unix / macOS: mode must not have any group or others permissions (0077 mask must be 0)
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("%w: file has group/other permissions: %04o (expected 0600)", ErrPermissionVerifyFailed, perm)
	}

	return nil
}

// WriteSecureFile writes data to path with owner-only permissions and applies platform ACL hardening
func WriteSecureFile(path string, data []byte) error {
	// Create or truncate with restrictive mode 0600
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write secure file: %w", err)
	}

	// Apply and verify platform ACLs
	if err := RestrictFilePermissions(path); err != nil {
		_ = os.Remove(path) // Fail closed: clean up unhardened file
		return fmt.Errorf("failed to harden secure file permissions: %w", err)
	}

	return nil
}
