package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cctv-sentinel/pkg/api"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/security"
)

func setupDesktopAPITestServer(t *testing.T) (*api.Server, http.Handler, *security.RBACManager, *security.DesktopAuthManager) {
	t.Helper()
	store := database.NewStore("")
	rbacMgr := security.NewRBACManager()
	srv := api.NewServer(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, rbacMgr)

	// Clean up any setup token file created during test
	t.Cleanup(func() {
		rbacMgr.RevokeSetupToken()
	})

	return srv, srv.Handler(), rbacMgr, srv.DesktopAuthManager()
}

func TestDesktopAuth_CompleteIsolationMatrix(t *testing.T) {
	_, handler, rbacMgr, _ := setupDesktopAPITestServer(t)

	// Provision initial system administrator
	setupTok := rbacMgr.GetSetupToken()
	err := rbacMgr.CompleteSetup(setupTok, "admin_operator", "AdminOperator#Pass2026!", "Operations Admin", "admin@sentinel.internal")
	if err != nil {
		t.Fatalf("failed to complete setup: %v", err)
	}

	loginJSON := `{"username":"admin_operator","password":"AdminOperator#Pass2026!"}`

	// 1. Browser login with no Origin is rejected (HTTP 403 CSRF_MISSING_ORIGIN)
	reqNoOrigin := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginJSON))
	reqNoOrigin.Header.Set("Content-Type", "application/json")
	recNoOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recNoOrigin, reqNoOrigin)
	if recNoOrigin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for browser login with no Origin, got %d: %s", recNoOrigin.Code, recNoOrigin.Body.String())
	}
	if !strings.Contains(recNoOrigin.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Errorf("expected CSRF_MISSING_ORIGIN code, got %s", recNoOrigin.Body.String())
	}

	// 2. Browser login attempting to bypass Origin via X-Client-Type: desktop MUST STILL BE REJECTED
	reqSpoofedDesktop := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginJSON))
	reqSpoofedDesktop.Header.Set("Content-Type", "application/json")
	reqSpoofedDesktop.Header.Set("X-Client-Type", "desktop")
	recSpoofedDesktop := httptest.NewRecorder()
	handler.ServeHTTP(recSpoofedDesktop, reqSpoofedDesktop)
	if recSpoofedDesktop.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for spoofed desktop header without Origin, got %d: %s", recSpoofedDesktop.Code, recSpoofedDesktop.Body.String())
	}
	if !strings.Contains(recSpoofedDesktop.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Errorf("expected CSRF_MISSING_ORIGIN code, got %s", recSpoofedDesktop.Body.String())
	}

	// 3 & 4. Valid browser login (with valid Origin) NEVER returns desktop_token, token, or session.token in JSON
	reqBrowserLogin := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginJSON))
	reqBrowserLogin.Header.Set("Content-Type", "application/json")
	reqBrowserLogin.Header.Set("Origin", "http://localhost:5173")
	reqBrowserLogin.Header.Set("X-Client-Type", "desktop") // Even if attacker sets this header!
	recBrowserLogin := httptest.NewRecorder()
	handler.ServeHTTP(recBrowserLogin, reqBrowserLogin)
	if recBrowserLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid browser login, got %d: %s", recBrowserLogin.Code, recBrowserLogin.Body.String())
	}

	var browserResp map[string]interface{}
	if err := json.Unmarshal(recBrowserLogin.Body.Bytes(), &browserResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Browser response must NEVER contain desktop_token or top-level token
	if tok, exists := browserResp["token"]; exists && tok != nil && tok != "" {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Browser login response exposed bearer token: %v", tok)
	}
	if dTok, exists := browserResp["desktop_token"]; exists && dTok != nil && dTok != "" {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Browser login response exposed desktop_token: %v", dTok)
	}

	// session object must NEVER contain token
	sessMap, ok := browserResp["session"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected session object in browser response")
	}
	if sTok, exists := sessMap["token"]; exists && sTok != nil && sTok != "" {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Browser login response session exposed bearer token: %v", sTok)
	}

	// 5. Unregistered native client cannot obtain challenge
	bodyUnregChallenge := `{"device_id":"unregistered-workstation-999"}`
	reqUnregCh := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyUnregChallenge))
	reqUnregCh.Header.Set("Content-Type", "application/json")
	recUnregCh := httptest.NewRecorder()
	handler.ServeHTTP(recUnregCh, reqUnregCh)
	if recUnregCh.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unregistered native client challenge, got %d: %s", recUnregCh.Code, recUnregCh.Body.String())
	}

	// Unregistered native client cannot obtain desktop token
	bodyUnregLogin := `{"device_id":"unregistered-workstation-999","challenge":"ch-fake","signature":"00","username":"admin_operator","password":"AdminOperator#Pass2026!"}`
	reqUnregLogin := httptest.NewRequest("POST", "/api/v1/auth/desktop/login", strings.NewReader(bodyUnregLogin))
	reqUnregLogin.Header.Set("Content-Type", "application/json")
	recUnregLogin := httptest.NewRecorder()
	handler.ServeHTTP(recUnregLogin, reqUnregLogin)
	if recUnregLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unregistered native client login, got %d: %s", recUnregLogin.Code, recUnregLogin.Body.String())
	}

	// 6. Native workstation generates local Ed25519 keypair and submits enrollment
	pubKey, privKey, err := security.GenerateClientKeyPair()
	if err != nil {
		t.Fatalf("failed to generate Ed25519 keypair: %v", err)
	}
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKey)

	enrollPayload := map[string]string{
		"device_id":   "workstation-ops-01",
		"client_name": "Control Room Console",
		"os":          "Windows 11",
		"hostname":    "OPS-PC-01",
		"public_key":  pubKeyB64,
	}
	bEnroll, _ := json.Marshal(enrollPayload)
	reqEnroll := httptest.NewRequest("POST", "/api/v1/auth/desktop/enroll", bytes.NewReader(bEnroll))
	reqEnroll.Header.Set("Content-Type", "application/json")
	recEnroll := httptest.NewRecorder()
	handler.ServeHTTP(recEnroll, reqEnroll)

	if recEnroll.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for desktop enrollment, got %d: %s", recEnroll.Code, recEnroll.Body.String())
	}

	var enrollResp struct {
		Success        bool   `json:"success"`
		DeviceID       string `json:"device_id"`
		ApprovalStatus string `json:"approval_status"`
		Fingerprint    string `json:"fingerprint"`
	}
	_ = json.Unmarshal(recEnroll.Body.Bytes(), &enrollResp)
	if enrollResp.ApprovalStatus != "PENDING" {
		t.Fatalf("expected PENDING approval status, got %s", enrollResp.ApprovalStatus)
	}

	// 6a. Pending device cannot obtain challenge (HTTP 403)
	bodyChallenge := `{"device_id":"workstation-ops-01"}`
	reqChPending := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyChallenge))
	reqChPending.Header.Set("Content-Type", "application/json")
	recChPending := httptest.NewRecorder()
	handler.ServeHTTP(recChPending, reqChPending)
	if recChPending.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for pending device challenge, got %d: %s", recChPending.Code, recChPending.Body.String())
	}

	// 6b. Admin approves the workstation
	adminAuthReq := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginJSON))
	adminAuthReq.Header.Set("Origin", "http://localhost:5173")
	adminAuthReq.Header.Set("Content-Type", "application/json")
	recAdminAuth := httptest.NewRecorder()
	handler.ServeHTTP(recAdminAuth, adminAuthReq)
	var adminAuthResp map[string]interface{}
	_ = json.Unmarshal(recAdminAuth.Body.Bytes(), &adminAuthResp)
	csrfTok := adminAuthResp["csrf_token"].(string)

	// Create admin session token directly for API calls
	adminSess, _ := rbacMgr.Authenticate("admin_operator", "AdminOperator#Pass2026!", 1*time.Hour)

	reqApprove := httptest.NewRequest("POST", "/api/v1/auth/desktop/devices/workstation-ops-01/approve", nil)
	reqApprove.Header.Set("Authorization", "Bearer "+adminSess.Token)
	reqApprove.Header.Set("X-CSRF-Token", csrfTok)
	recApprove := httptest.NewRecorder()
	handler.ServeHTTP(recApprove, reqApprove)
	if recApprove.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for device approval, got %d: %s", recApprove.Code, recApprove.Body.String())
	}

	// 6c. Approved device requests challenge
	reqCh := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyChallenge))
	reqCh.Header.Set("Content-Type", "application/json")
	recCh := httptest.NewRecorder()
	handler.ServeHTTP(recCh, reqCh)
	if recCh.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for challenge request, got %d: %s", recCh.Code, recCh.Body.String())
	}

	var chResp struct {
		Challenge string `json:"challenge"`
		DeviceID  string `json:"device_id"`
	}
	_ = json.Unmarshal(recCh.Body.Bytes(), &chResp)
	if chResp.Challenge == "" {
		t.Fatalf("expected non-empty challenge string")
	}

	// 6d. Invalid proof fails login
	badLoginPayload := map[string]string{
		"device_id": "workstation-ops-01",
		"challenge": chResp.Challenge,
		"signature": base64.StdEncoding.EncodeToString(make([]byte, 64)),
		"username":  "admin_operator",
		"password":  "AdminOperator#Pass2026!",
	}
	bBad, _ := json.Marshal(badLoginPayload)
	reqBadProof := httptest.NewRequest("POST", "/api/v1/auth/desktop/login", bytes.NewReader(bBad))
	reqBadProof.Header.Set("Content-Type", "application/json")
	recBadProof := httptest.NewRecorder()
	handler.ServeHTTP(recBadProof, reqBadProof)
	if recBadProof.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid device signature, got %d: %s", recBadProof.Code, recBadProof.Body.String())
	}

	// 6e. Obtain fresh challenge (because previous challenge was atomically consumed on failed attempt)
	recCh2 := httptest.NewRecorder()
	reqCh2 := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyChallenge))
	reqCh2.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recCh2, reqCh2)
	_ = json.Unmarshal(recCh2.Body.Bytes(), &chResp)

	// Valid Ed25519 signature of challenge using private key stored in workstation vault
	validSig := security.SignChallenge(privKey, chResp.Challenge)

	validLoginPayload := map[string]string{
		"device_id": "workstation-ops-01",
		"challenge": chResp.Challenge,
		"signature": validSig,
		"username":  "admin_operator",
		"password":  "AdminOperator#Pass2026!",
	}
	bValid, _ := json.Marshal(validLoginPayload)
	reqValidProof := httptest.NewRequest("POST", "/api/v1/auth/desktop/login", bytes.NewReader(bValid))
	reqValidProof.Header.Set("Content-Type", "application/json")
	recValidProof := httptest.NewRecorder()
	handler.ServeHTTP(recValidProof, reqValidProof)

	if recValidProof.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid native client authentication, got %d: %s", recValidProof.Code, recValidProof.Body.String())
	}

	var deskLoginResp struct {
		Success   bool   `json:"success"`
		Token     string `json:"token"`
		TokenType string `json:"token_type"`
		DeviceID  string `json:"device_id"`
	}
	if err := json.Unmarshal(recValidProof.Body.Bytes(), &deskLoginResp); err != nil {
		t.Fatalf("failed to decode desktop login response: %v", err)
	}
	if !deskLoginResp.Success || deskLoginResp.Token == "" || deskLoginResp.TokenType != "Bearer" {
		t.Fatalf("expected valid bearer token for native desktop client, got: %+v", deskLoginResp)
	}
	desktopBearerToken := deskLoginResp.Token

	// 6f. Access protected endpoint with bearer token
	reqStatus := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	reqStatus.Header.Set("Authorization", "Bearer "+desktopBearerToken)
	recStatus := httptest.NewRecorder()
	handler.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 OK using desktop bearer token, got %d: %s", recStatus.Code, recStatus.Body.String())
	}

	// 7. Device Revocation & Immediate Session Invalidation
	// Revoking workstation device must immediately terminate its active bearer sessions!
	reqRevoke := httptest.NewRequest("POST", "/api/v1/auth/desktop/devices/workstation-ops-01/revoke", nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+adminSess.Token)
	recRevoke := httptest.NewRecorder()
	handler.ServeHTTP(recRevoke, reqRevoke)
	if recRevoke.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for device revocation, got %d: %s", recRevoke.Code, recRevoke.Body.String())
	}

	// Subsequent request with the desktop bearer token MUST FAIL (HTTP 401 Unauthorized)
	reqRevokedCheck := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	reqRevokedCheck.Header.Set("Authorization", "Bearer "+desktopBearerToken)
	recRevokedCheck := httptest.NewRecorder()
	handler.ServeHTTP(recRevokedCheck, reqRevokedCheck)
	if recRevokedCheck.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for token after device revocation, got %d: %s", recRevokedCheck.Code, recRevokedCheck.Body.String())
	}
}

func TestDesktopAuth_AdminDeviceList(t *testing.T) {
	_, handler, rbacMgr, _ := setupDesktopAPITestServer(t)

	// Complete initial admin setup
	setupTok := rbacMgr.GetSetupToken()
	_ = rbacMgr.CompleteSetup(setupTok, "admin_user", "AdminOperator#Pass2026!", "Administrator", "admin@sentinel.io")

	// Authenticate admin session
	adminSess, err := rbacMgr.Authenticate("admin_user", "AdminOperator#Pass2026!", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to authenticate admin: %v", err)
	}

	// 1. Unauthenticated request to list devices is rejected (401)
	reqUnauth := httptest.NewRequest("GET", "/api/v1/auth/desktop/devices", nil)
	recUnauth := httptest.NewRecorder()
	handler.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated device listing must return 401, got %d", recUnauth.Code)
	}

	// 2. Client submits enrollment
	pub, _, _ := security.GenerateClientKeyPair()
	enrollPayload, _ := json.Marshal(map[string]string{
		"device_id":   "ws-list-test",
		"client_name": "List Test PC",
		"public_key":  base64.StdEncoding.EncodeToString(pub),
	})
	reqEnroll := httptest.NewRequest("POST", "/api/v1/auth/desktop/enroll", bytes.NewReader(enrollPayload))
	reqEnroll.Header.Set("Content-Type", "application/json")
	recEnroll := httptest.NewRecorder()
	handler.ServeHTTP(recEnroll, reqEnroll)
	if recEnroll.Code != http.StatusCreated {
		t.Fatalf("enrollment failed: %d: %s", recEnroll.Code, recEnroll.Body.String())
	}

	// 3. Admin lists devices and verifies metadata is present
	reqList := httptest.NewRequest("GET", "/api/v1/auth/desktop/devices", nil)
	reqList.Header.Set("Authorization", "Bearer "+adminSess.Token)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for device list, got %d: %s", recList.Code, recList.Body.String())
	}

	listBody := recList.Body.String()
	if !strings.Contains(listBody, "ws-list-test") {
		t.Errorf("expected enrolled device in list response: %s", listBody)
	}
	if !strings.Contains(listBody, "SHA256:") {
		t.Errorf("expected fingerprint in list response: %s", listBody)
	}
}
