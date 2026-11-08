package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cctv-sentinel/pkg/security"
)

// handleDesktopChallenge issues a single-use cryptographic challenge for a registered native desktop client
func (s *Server) handleDesktopChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ClientID) == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "client_id is required in JSON body")
		return
	}

	chRec, err := s.desktopAuthMgr.IssueChallenge(req.ClientID)
	if err != nil {
		s.logger.Log("DesktopClient", "SYSTEM", r.RemoteAddr, "DESKTOP_CHALLENGE", "SECURITY", "FAILED",
			fmt.Sprintf("Challenge request rejected for client_id=%s: %v", req.ClientID, err), "")
		writeError(w, r, http.StatusUnauthorized, "DESKTOP_CHALLENGE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"client_id":  chRec.ClientID,
		"challenge":  chRec.Challenge,
		"expires_at": chRec.ExpiresAt,
	})
}

// handleDesktopLogin authenticates a native desktop client via challenge-response proof + operator credentials
func (s *Server) handleDesktopLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ClientID    string `json:"client_id"`
		Challenge   string `json:"challenge"`
		ClientProof string `json:"client_proof"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Malformed desktop login payload")
		return
	}

	// 1. Verify native client cryptographic challenge-response proof
	if err := s.desktopAuthMgr.VerifyProof(req.ClientID, req.Challenge, req.ClientProof); err != nil {
		s.logger.Log(req.Username, "DESKTOP", r.RemoteAddr, "LOGIN_DESKTOP", "SECURITY", "FAILED",
			fmt.Sprintf("Native client device proof failed for client_id=%s: %v", req.ClientID, err), "")
		writeError(w, r, http.StatusUnauthorized, "DESKTOP_AUTH_FAILED", "Invalid native client identity or challenge proof")
		return
	}

	// 2. Authenticate enterprise operator credentials
	sess, err := s.rbacMgr.Authenticate(req.Username, req.Password, 8*time.Hour)
	if err != nil {
		s.logger.Log(req.Username, "DESKTOP", r.RemoteAddr, "LOGIN_DESKTOP", "SECURITY", "FAILED",
			fmt.Sprintf("Operator credential check failed for native client_id=%s", req.ClientID), "")
		writeError(w, r, http.StatusUnauthorized, "AUTH_FAILED", "Invalid operator credentials")
		return
	}

	s.logger.Log(req.Username, "DESKTOP", r.RemoteAddr, "LOGIN_DESKTOP", "SECURITY", "SUCCESS",
		fmt.Sprintf("Native desktop client authenticated (Client: %s, Role: %s)", req.ClientID, sess.Role), "")

	// 3. Issue bearer token exclusively for native desktop client OS credential vault storage
	resp := map[string]interface{}{
		"success":    true,
		"token":      sess.Token,
		"token_type": "Bearer",
		"expires_at": sess.ExpiresAt,
		"client_id":  req.ClientID,
		"session": map[string]interface{}{
			"username":        sess.Username,
			"role":            sess.Role,
			"tenant_id":       sess.TenantID,
			"site_id":         sess.SiteID,
			"is_global_admin": sess.IsGlobalAdmin,
			"scopes":          sess.Scopes,
			"created_at":      sess.CreatedAt,
			"expires_at":      sess.ExpiresAt,
		},
		"permissions": security.RolePermissions[sess.Role],
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDesktopRegisterDevice allows administrators to provision and enroll a new native client workstation
func (s *Server) handleDesktopRegisterDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ClientID   string `json:"client_id"`
		DeviceName string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ClientID) == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "client_id is required")
		return
	}

	dev, devKey, err := s.desktopAuthMgr.RegisterDevice(req.ClientID, req.DeviceName)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "DEVICE_REGISTRATION_FAILED", err.Error())
		return
	}

	sess := getSession(r)
	adminUser := "Admin"
	if sess != nil {
		adminUser = sess.Username
	}
	s.logger.Log(adminUser, "SYSTEM", r.RemoteAddr, "ENROLL_DESKTOP_DEVICE", "SECURITY", "SUCCESS",
		fmt.Sprintf("Enrolled desktop workstation client_id=%s name=%s", dev.ClientID, dev.DeviceName), "")

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success":     true,
		"client_id":   dev.ClientID,
		"device_name": dev.DeviceName,
		"device_key":  devKey,
		"enrolled_at": dev.EnrolledAt,
		"message":     "Workstation enrolled successfully. Store device_key in local OS Credential Vault.",
	})
}

// handleDesktopListDevices allows administrators to view all registered native desktop client workstations
func (s *Server) handleDesktopListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	devices := s.desktopAuthMgr.ListDevices()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"devices": devices,
		"count":   len(devices),
	})
}
