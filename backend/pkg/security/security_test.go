package security_test

import (
	"testing"
	"time"

	"cctv-sentinel/pkg/security"
)

func TestNoDefaultCredentials(t *testing.T) {
	rbac := security.NewRBACManager()

	// 1. None of the default accounts should authenticate
	defaults := []struct {
		user string
		pass string
	}{
		{"admin", "Admin@Sentinel2026!"},
		{"technician", "Tech@Sentinel2026!"},
		{"auditor", "Audit@Sentinel2026!"},
		{"viewer", "View@Sentinel2026!"},
	}

	for _, d := range defaults {
		_, err := rbac.Authenticate(d.user, d.pass, 1*time.Hour)
		if err != security.ErrUnauthorized {
			t.Fatalf("CRITICAL SECURITY VIOLATION: Default credentials for user %s still active!", d.user)
		}
	}
}

func TestFirstRunSetupLifecycle(t *testing.T) {
	rbac := security.NewRBACManager()

	// 1. System must be in SetupMode
	if !rbac.IsSetupRequired() {
		t.Fatalf("fresh RBACManager must require setup")
	}

	setupTok := rbac.GetSetupToken()
	if setupTok == "" {
		t.Fatalf("expected non-empty setup token")
	}

	// 2. Reject invalid setup token
	err := rbac.CompleteSetup("sent-setup-invalid-token", "admin", "TestOperator#Pass2026!", "Admin", "admin@tds.in")
	if err != security.ErrInvalidSetupToken {
		t.Fatalf("expected ErrInvalidSetupToken, got %v", err)
	}

	// 3. Reject weak password
	err = rbac.CompleteSetup(setupTok, "admin", "weakpass", "Admin", "admin@tds.in")
	if err == nil {
		t.Fatalf("expected error for weak password, got nil")
	}

	// 4. Accept valid administrator creation
	validAdminPass := "TestOperator#Pass2026!"
	err = rbac.CompleteSetup(setupTok, "admin", validAdminPass, "Lead Architect", "admin@tds.in")
	if err != nil {
		t.Fatalf("valid admin setup failed: %v", err)
	}

	// 5. Setup mode must be permanently disabled and token erased
	if rbac.IsSetupRequired() {
		t.Errorf("setup must be disabled after admin creation")
	}
	if rbac.GetSetupToken() != "" {
		t.Errorf("setup token must be erased after setup completion")
	}

	// 6. Second setup attempt must be rejected
	err = rbac.CompleteSetup(setupTok, "intruder", "Hacker@2026!", "Intruder", "evil@evil.com")
	if err != security.ErrSetupDisabled {
		t.Fatalf("expected ErrSetupDisabled on second attempt, got %v", err)
	}

	// 7. Administrator authenticates successfully
	sess, err := rbac.Authenticate("admin", validAdminPass, 1*time.Hour)
	if err != nil || sess == nil {
		t.Fatalf("admin authentication failed: %v", err)
	}
	if sess.Role != security.RoleAdmin {
		t.Errorf("expected RoleAdmin, got %s", sess.Role)
	}

	// Admin permissions verified
	if !security.HasPermission(sess.Role, security.PermReboot) {
		t.Errorf("Admin must have PermReboot")
	}
	if !security.HasPermission(sess.Role, security.PermFirmwareUpgrade) {
		t.Errorf("Admin must have PermFirmwareUpgrade")
	}
	if !security.HasPermission(sess.Role, security.PermPTZControl) {
		t.Errorf("Admin must have PermPTZControl")
	}
}

func TestArgon2idParameterLimitsAndDoSResistance(t *testing.T) {
	rbac := security.NewRBACManager()
	tok := rbac.GetSetupToken()
	_ = rbac.CompleteSetup(tok, "admin", "TestOperator#Pass2026!", "Admin", "admin@tds.in")

	// 1. Create a user with legacy SHA-256 hash and verify rehash
	legacyUser := "legacy_user"
	legacyPass := "LegacyPass@2026!"
	_ = rbac.CreateUser(legacyUser, legacyPass, "Legacy Operator", security.RoleTechnician, "legacy@tds.in", "default", "site-1")

	sess, err := rbac.Authenticate(legacyUser, legacyPass, 1*time.Hour)
	if err != nil || sess == nil {
		t.Fatalf("legacy authenticate and rehash failed: %v", err)
	}

	// 2. Test malformed and excessive Argon2 parameters rejection
	// We inject an account with excessive memory parameter: m=1048576 (1GB, exceeds 512MB limit)
	excessiveHash := "$argon2id$v=19$m=1048576,t=3,p=2$0102030405060708090a0b0c0d0e0f10$0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	rbac.InjectUserWithHashForTest("attacker", excessiveHash, "0102030405060708090a0b0c0d0e0f10", security.RoleViewer)

	_, err = rbac.Authenticate("attacker", "anyPassword123!", 1*time.Hour)
	if err != security.ErrInvalidArgonParams {
		t.Fatalf("expected ErrInvalidArgonParams for excessive memory, got %v", err)
	}

	// 3. Test wrong version: v=18
	wrongVersionHash := "$argon2id$v=18$m=65536,t=3,p=2$0102030405060708090a0b0c0d0e0f10$0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	rbac.InjectUserWithHashForTest("attacker2", wrongVersionHash, "0102030405060708090a0b0c0d0e0f10", security.RoleViewer)
	_, err = rbac.Authenticate("attacker2", "anyPassword123!", 1*time.Hour)
	if err != security.ErrInvalidArgonParams {
		t.Fatalf("expected ErrInvalidArgonParams for wrong argon version, got %v", err)
	}
}

func TestSessionValidationAndRevocation(t *testing.T) {
	rbac := security.NewRBACManager()
	tok := rbac.GetSetupToken()
	_ = rbac.CompleteSetup(tok, "admin", "TestOperator#Pass2026!", "Admin", "admin@tds.in")

	sess, err := rbac.Authenticate("admin", "TestOperator#Pass2026!", 1*time.Second)
	if err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	// Immediate validation succeeds
	vSess, err := rbac.ValidateSession(sess.Token)
	if err != nil || vSess.Username != "admin" {
		t.Fatalf("expected valid session, got %v", err)
	}

	// Explicit revocation
	rbac.RevokeSession(sess.Token)
	_, err = rbac.ValidateSession(sess.Token)
	if err != security.ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized after revocation, got %v", err)
	}
}

func TestSSRFProtection(t *testing.T) {
	// 1. Cloud metadata IPv4 must be blocked
	_, err := security.ValidateSurveillanceIP("169.254.169.254", false)
	if err == nil {
		t.Errorf("CRITICAL SECURITY: 169.254.169.254 must be blocked by SSRF validator!")
	}

	// 2. Alibaba cloud metadata IPv4 must be blocked
	_, err = security.ValidateSurveillanceIP("100.100.100.200", false)
	if err == nil {
		t.Errorf("CRITICAL SECURITY: 100.100.100.200 must be blocked by SSRF validator!")
	}

	// 3. AWS EC2 IPv6 metadata must be blocked
	_, err = security.ValidateSurveillanceIP("fd00:ec2::254", false)
	if err == nil {
		t.Errorf("CRITICAL SECURITY: fd00:ec2::254 must be blocked by SSRF validator!")
	}

	// 4. Multicast IP must be blocked
	_, err = security.ValidateSurveillanceIP("239.255.255.250", false)
	if err == nil {
		t.Errorf("Multicast address must be blocked for unicast queries")
	}

	// 5. Loopback without allowLoopback must be blocked
	_, err = security.ValidateSurveillanceIP("127.0.0.1", false)
	if err == nil {
		t.Errorf("Loopback address must be blocked when allowLoopback=false")
	}

	// 6. Legitimate surveillance camera IP must be accepted
	ip, err := security.ValidateSurveillanceIP("192.168.31.103", false)
	if err != nil || ip.String() != "192.168.31.103" {
		t.Errorf("Legitimate camera IP 192.168.31.103 was unexpectedly rejected: %v", err)
	}
}

func TestRolePermissionMatrix(t *testing.T) {
	// Viewer role cannot reboot, upgrade firmware, configure devices, or manage PTZ
	viewerRole := security.RoleViewer
	if security.HasPermission(viewerRole, security.PermReboot) {
		t.Errorf("Viewer must NOT have PermReboot")
	}
	if security.HasPermission(viewerRole, security.PermFirmwareUpgrade) {
		t.Errorf("Viewer must NOT have PermFirmwareUpgrade")
	}
	if security.HasPermission(viewerRole, security.PermDeviceConfig) {
		t.Errorf("Viewer must NOT have PermDeviceConfig")
	}
	if security.HasPermission(viewerRole, security.PermPTZManage) {
		t.Errorf("Viewer must NOT have PermPTZManage")
	}
	if !security.HasPermission(viewerRole, security.PermViewLive) {
		t.Errorf("Viewer must have PermViewLive")
	}

	// Auditor can view audit logs and live feeds, but cannot control PTZ or upgrade firmware
	auditorRole := security.RoleAuditor
	if !security.HasPermission(auditorRole, security.PermAuditView) {
		t.Errorf("Auditor must have PermAuditView")
	}
	if security.HasPermission(auditorRole, security.PermFirmwareUpgrade) {
		t.Errorf("Auditor must NOT have PermFirmwareUpgrade")
	}
	if security.HasPermission(auditorRole, security.PermPTZControl) {
		t.Errorf("Auditor must NOT have PermPTZControl")
	}
}
