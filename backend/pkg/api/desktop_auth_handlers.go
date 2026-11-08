package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cctv-sentinel/pkg/security"
)

// handleDesktopEnroll allows native workstation clients to submit their locally-generated Ed25519 public key
func (s *Server) handleDesktopEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req security.DesktopEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Malformed desktop enrollment payload")
		return
	}

	rec, err := s.desktopAuthMgr.SubmitEnrollment(req)
	if err != nil {
		if err == security.ErrDeviceAlreadyActive {
			writeError(w, r, http.StatusConflict, "DEVICE_ALREADY_ACTIVE", err.Error())
			return
		}
		writeError(w, r, http.StatusBadRequest, "ENROLLMENT_FAILED", err.Error())
		return
	}

	s.logger.Log("DesktopClient", "SYSTEM", r.RemoteAddr, "DESKTOP_ENROLL", "SECURITY", "SUCCESS",
		fmt.Sprintf("Workstation enrollment submitted: id=%s name=%s fingerprint=%s", rec.ID, rec.ClientName, rec.KeyFingerprint), "")

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success":         true,
		"device_id":       rec.ID,
		"client_name":     rec.ClientName,
		"approval_status": rec.ApprovalStatus,
		"fingerprint":     rec.KeyFingerprint,
		"key_version":     rec.KeyVersion,
		"expires_at":      rec.ExpiresAt,
		"message":         "Workstation enrollment submitted successfully. Awaiting administrator review and approval.",
	})
}

// handleDesktopChallenge issues a single-use cryptographic challenge for an approved native desktop client
func (s *Server) handleDesktopChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		DeviceID string `json:"device_id"`
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Malformed challenge request payload")
		return
	}

	id := strings.TrimSpace(req.DeviceID)
	if id == "" {
		id = strings.TrimSpace(req.ClientID)
	}
	if id == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "device_id is required in JSON body")
		return
	}

	chRec, err := s.desktopAuthMgr.IssueChallenge(id)
	if err != nil {
		s.logger.Log("DesktopClient", "SYSTEM", r.RemoteAddr, "DESKTOP_CHALLENGE", "SECURITY", "FAILED",
			fmt.Sprintf("Challenge request rejected for device_id=%s: %v", id, err), "")
		if err == security.ErrDeviceNotApproved {
			writeError(w, r, http.StatusForbidden, "DEVICE_NOT_APPROVED", err.Error())
			return
		}
		writeError(w, r, http.StatusUnauthorized, "DESKTOP_CHALLENGE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"device_id":  chRec.DeviceID,
		"client_id":  chRec.DeviceID,
		"challenge":  chRec.Challenge,
		"expires_at": chRec.ExpiresAt,
	})
}

// handleDesktopLogin authenticates a native desktop client via asymmetric Ed25519 signature + operator credentials
func (s *Server) handleDesktopLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		DeviceID    string `json:"device_id"`
		ClientID    string `json:"client_id"`
		Challenge   string `json:"challenge"`
		Signature   string `json:"signature"`
		ClientProof string `json:"client_proof"` // Fallback alias for signature
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Malformed desktop login payload")
		return
	}

	id := strings.TrimSpace(req.DeviceID)
	if id == "" {
		id = strings.TrimSpace(req.ClientID)
	}

	sig := strings.TrimSpace(req.Signature)
	if sig == "" {
		sig = strings.TrimSpace(req.ClientProof)
	}

	// 1. Verify asymmetric Ed25519 proof of workstation identity
	// Challenge is atomically consumed inside VerifyProof regardless of pass/fail
	if err := s.desktopAuthMgr.VerifyProof(id, req.Challenge, sig); err != nil {
		s.logger.Log(req.Username, "DESKTOP", r.RemoteAddr, "LOGIN_DESKTOP", "SECURITY", "FAILED",
			fmt.Sprintf("Native client device proof failed for device_id=%s: %v", id, err), "")
		writeError(w, r, http.StatusUnauthorized, "DESKTOP_AUTH_FAILED", "Invalid native client device signature or challenge proof")
		return
	}

	// 2. Authenticate enterprise operator credentials and bind session to DeviceID
	sess, err := s.rbacMgr.AuthenticateDesktop(req.Username, req.Password, id, 8*time.Hour)
	if err != nil {
		s.logger.Log(req.Username, "DESKTOP", r.RemoteAddr, "LOGIN_DESKTOP", "SECURITY", "FAILED",
			fmt.Sprintf("Operator credential check failed for native device_id=%s", id), "")
		writeError(w, r, http.StatusUnauthorized, "AUTH_FAILED", "Invalid operator credentials")
		return
	}

	s.logger.Log(req.Username, "DESKTOP", r.RemoteAddr, "LOGIN_DESKTOP", "SECURITY", "SUCCESS",
		fmt.Sprintf("Native desktop client authenticated (Device: %s, Role: %s)", id, sess.Role), "")

	// 3. Issue bearer token exclusively for native desktop client OS credential vault storage
	resp := map[string]interface{}{
		"success":    true,
		"token":      sess.Token,
		"token_type": "Bearer",
		"expires_at": sess.ExpiresAt,
		"device_id":  id,
		"client_id":  id,
		"session": map[string]interface{}{
			"username":        sess.Username,
			"role":            sess.Role,
			"tenant_id":       sess.TenantID,
			"site_id":         sess.SiteID,
			"is_global_admin": sess.IsGlobalAdmin,
			"scopes":          sess.Scopes,
			"device_id":       id,
			"created_at":      sess.CreatedAt,
			"expires_at":      sess.ExpiresAt,
		},
		"permissions": security.RolePermissions[sess.Role],
	}

	writeJSON(w, http.StatusOK, resp)
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

// handleDesktopDeviceActions dispatches /api/v1/auth/desktop/devices/{id} and subroutes
func (s *Server) handleDesktopDeviceActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/auth/desktop/devices/")
	path = strings.TrimPrefix(path, "/api/v1/security/desktop/devices/")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		if r.Method == "GET" {
			s.handleDesktopListDevices(w, r)
			return
		}
		writeSimpleError(w, r, http.StatusBadRequest, "Device ID required")
		return
	}

	deviceID := parts[0]

	// GET /api/v1/auth/desktop/devices/{id}
	if len(parts) == 1 {
		if r.Method == "GET" {
			dev, err := s.desktopAuthMgr.GetDevice(deviceID)
			if err != nil {
				writeError(w, r, http.StatusNotFound, "DEVICE_NOT_FOUND", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"success": true,
				"device":  dev,
			})
			return
		}
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	action := parts[1]
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sess := getSession(r)
	adminUser := "Admin"
	if sess != nil {
		adminUser = sess.Username
	}

	switch action {
	case "approve":
		dev, err := s.desktopAuthMgr.ApproveDevice(deviceID, adminUser)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "APPROVE_FAILED", err.Error())
			return
		}
		s.logger.Log(adminUser, "SYSTEM", r.RemoteAddr, "DESKTOP_APPROVE", "SECURITY", "SUCCESS",
			fmt.Sprintf("Approved desktop workstation device_id=%s", deviceID), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"device":  dev,
			"message": "Device approved successfully",
		})

	case "reject":
		dev, err := s.desktopAuthMgr.RejectDevice(deviceID, adminUser)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "REJECT_FAILED", err.Error())
			return
		}
		s.logger.Log(adminUser, "SYSTEM", r.RemoteAddr, "DESKTOP_REJECT", "SECURITY", "SUCCESS",
			fmt.Sprintf("Rejected desktop workstation device_id=%s", deviceID), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"device":  dev,
			"message": "Device enrollment rejected",
		})

	case "revoke":
		dev, err := s.desktopAuthMgr.RevokeDevice(deviceID, adminUser)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "REVOKE_FAILED", err.Error())
			return
		}
		// Immediately terminate all active sessions issued to this device
		revokedSessions := s.rbacMgr.RevokeDeviceSessions(deviceID)
		s.logger.Log(adminUser, "SYSTEM", r.RemoteAddr, "DESKTOP_REVOKE", "SECURITY", "SUCCESS",
			fmt.Sprintf("Revoked desktop workstation device_id=%s (terminated %d active sessions)", deviceID, revokedSessions), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":          true,
			"device":           dev,
			"revoked_sessions": revokedSessions,
			"message":          "Device revoked and active sessions invalidated",
		})

	case "rotate-key":
		var rotReq struct {
			PublicKey string `json:"public_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rotReq); err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Malformed rotate-key payload")
			return
		}
		dev, err := s.desktopAuthMgr.RotateDeviceKey(deviceID, rotReq.PublicKey, adminUser)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "ROTATE_KEY_FAILED", err.Error())
			return
		}
		s.logger.Log(adminUser, "SYSTEM", r.RemoteAddr, "DESKTOP_ROTATE_KEY", "SECURITY", "SUCCESS",
			fmt.Sprintf("Rotated key for device_id=%s new_version=%d fingerprint=%s", deviceID, dev.KeyVersion, dev.KeyFingerprint), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"device":  dev,
			"message": "Device public key rotated successfully",
		})

	default:
		writeSimpleError(w, r, http.StatusNotFound, "Unknown device action")
	}
}
