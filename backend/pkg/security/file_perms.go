package security

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// RestrictFilePermissions secures a sensitive file (such as sentinel-setup.token) against unauthorized local access.
// On Windows, it invokes icacls to strip inherited permissions (/inheritance:r) and grant Full Control only to
// SYSTEM (*S-1-5-18), Builtin Administrators (*S-1-5-32-544), and the current executing user (%USERNAME%).
// On Unix/POSIX, it sets file permission mode 0600 (read/write only by owner).
func RestrictFilePermissions(path string) error {
	if runtime.GOOS == "windows" {
		username := os.Getenv("USERNAME")
		if username == "" {
			username = "%USERNAME%"
		}
		// Execute cmd /c icacls with inheritance stripping and restricted grants
		cmdStr := fmt.Sprintf(`icacls "%s" /inheritance:r /grant:r "*S-1-5-18:(F)" /grant:r "*S-1-5-32-544:(F)" /grant:r "%s:(F)"`, path, username)
		cmd := exec.Command("cmd", "/c", cmdStr)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("icacls hardening failed: %w: %s", err, string(out))
		}
		return nil
	}
	return os.Chmod(path, 0600)
}
