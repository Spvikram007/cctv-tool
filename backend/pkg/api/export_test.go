package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"cctv-sentinel/pkg/security"
)

// IssueSessionForTest generates an active tenant-scoped session token for unit test execution.
// Fails securely: never substitutes deterministic fallback bytes.
func (s *Server) IssueSessionForTest(role security.Role, tenantID, siteID string) string {
	tokBytes := make([]byte, 24)
	if _, err := rand.Read(tokBytes); err != nil {
		panic(fmt.Sprintf("CRITICAL TEST SECURITY ERROR: crypto/rand unavailable: %v", err))
	}
	tok := fmt.Sprintf("test-sess-%s", hex.EncodeToString(tokBytes))
	s.rbacMgr.InjectSessionForTest(tok, "test-operator", role, tenantID, siteID, 24*time.Hour)
	return tok
}

// IssueGlobalAdminSessionForTest generates an active global super-admin session for test execution.
// Fails securely: never substitutes deterministic fallback bytes.
func (s *Server) IssueGlobalAdminSessionForTest() string {
	tokBytes := make([]byte, 24)
	if _, err := rand.Read(tokBytes); err != nil {
		panic(fmt.Sprintf("CRITICAL TEST SECURITY ERROR: crypto/rand unavailable: %v", err))
	}
	tok := fmt.Sprintf("test-sess-global-%s", hex.EncodeToString(tokBytes))
	s.rbacMgr.InjectGlobalAdminSessionForTest(tok, "super-admin", 24*time.Hour)
	return tok
}

