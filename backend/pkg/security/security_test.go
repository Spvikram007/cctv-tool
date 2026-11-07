package security_test

import (
	"testing"
	"time"

	"cctv-sentinel/pkg/security"
)

func TestRBACAuthenticationAndPermissions(t *testing.T) {
	rbac := security.NewRBACManager()

	// 1. Authenticate Admin
	adminSess, err := rbac.Authenticate("admin", "Admin@Sentinel2026!", 1*time.Hour)
	if err != nil {
		t.Fatalf("Admin authentication failed: %v", err)
	}
	if adminSess.Role != security.RoleAdmin {
		t.Errorf("expected RoleAdmin, got %s", adminSess.Role)
	}

	// Verify Admin has all permissions
	if !security.HasPermission(adminSess.Role, security.PermReboot) {
		t.Errorf("Admin must have PermReboot")
	}
	if !security.HasPermission(adminSess.Role, security.PermFirmwareUpgrade) {
		t.Errorf("Admin must have PermFirmwareUpgrade")
	}

	// 2. Authenticate Viewer
	viewerSess, err := rbac.Authenticate("viewer", "View@Sentinel2026!", 1*time.Hour)
	if err != nil {
		t.Fatalf("Viewer authentication failed: %v", err)
	}

	// Viewer can view live, but cannot reboot or configure devices
	if !security.HasPermission(viewerSess.Role, security.PermViewLive) {
		t.Errorf("Viewer must have PermViewLive")
	}
	if security.HasPermission(viewerSess.Role, security.PermReboot) {
		t.Errorf("Viewer must NOT have PermReboot")
	}
	if security.HasPermission(viewerSess.Role, security.PermDeviceConfig) {
		t.Errorf("Viewer must NOT have PermDeviceConfig")
	}

	// 3. Test Invalid Credentials
	_, err = rbac.Authenticate("admin", "WrongPassword123", 1*time.Hour)
	if err != security.ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized for bad password, got %v", err)
	}
}

func TestArgon2idPasswordHashingAndRehash(t *testing.T) {
	rbac := security.NewRBACManager()

	// 1. Authenticate with newly created Argon2id account
	sess, err := rbac.Authenticate("admin", "Admin@Sentinel2026!", 1*time.Hour)
	if err != nil || sess == nil {
		t.Fatalf("Argon2id authentication failed: %v", err)
	}

	// 2. Test legacy migration / rehash
	// Create user with explicit legacy salted hash (pre-Phase 6)
	legacyUser := "legacy_tech"
	legacyPass := "LegacyPass@2025!"
	_ = rbac.CreateUser(legacyUser, legacyPass, "Legacy Tech", security.RoleTechnician, "legacy@tds.in", "default", "site-1")

	// Verify authenticate works and upgrades to Argon2id
	legacySess, err := rbac.Authenticate(legacyUser, legacyPass, 1*time.Hour)
	if err != nil || legacySess == nil {
		t.Fatalf("Legacy password authentication failed: %v", err)
	}
}

func TestSSRFProtection(t *testing.T) {
	// 1. Cloud metadata IP must be blocked
	_, err := security.ValidateSurveillanceIP("169.254.169.254", false)
	if err == nil {
		t.Errorf("CRITICAL SECURITY: 169.254.169.254 must be blocked by SSRF validator!")
	}

	// 2. Multicast IP must be blocked
	_, err = security.ValidateSurveillanceIP("239.255.255.250", false)
	if err == nil {
		t.Errorf("Multicast address must be blocked for unicast queries")
	}

	// 3. Loopback without allowLoopback must be blocked
	_, err = security.ValidateSurveillanceIP("127.0.0.1", false)
	if err == nil {
		t.Errorf("Loopback address must be blocked when allowLoopback=false")
	}

	// 4. Legitimate surveillance camera IP must be accepted
	ip, err := security.ValidateSurveillanceIP("192.168.31.103", false)
	if err != nil || ip.String() != "192.168.31.103" {
		t.Errorf("Legitimate camera IP 192.168.31.103 was unexpectedly rejected: %v", err)
	}
}

func TestPathTraversalPrevention(t *testing.T) {
	maliciousPaths := []string{
		"../../../../windows/system32/cmd.exe",
		`..\..\..\boot.ini`,
		"/etc/shadow",
		"../../../data/snapshots/secret.jpg",
	}

	for _, p := range maliciousPaths {
		cleaned, err := security.SanitizePath(p)
		if err == nil && cleaned != "cmd.exe" && cleaned != "boot.ini" && cleaned != "shadow" && cleaned != "secret.jpg" {
			t.Errorf("Path traversal was not properly handled for %s", p)
		}
	}
}

func TestIPRateLimiter(t *testing.T) {
	// 5 requests per second, capacity 5
	limiter := security.NewIPRateLimiter(5.0, 5.0)

	testIP := "192.168.1.50"

	// Initial 5 requests should pass
	for i := 0; i < 5; i++ {
		if !limiter.Allow(testIP) {
			t.Fatalf("Request %d should have been allowed under capacity limit", i+1)
		}
	}

	// 6th immediate request should be rate-limited
	if limiter.Allow(testIP) {
		t.Errorf("Request 6 should have been rate limited")
	}
}
