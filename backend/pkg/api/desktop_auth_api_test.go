package api_test

import (
	"bytes"
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
	_, handler, rbacMgr, deskMgr := setupDesktopAPITestServer(t)

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
	bodyUnregChallenge := `{"client_id":"unregistered-workstation-999"}`
	reqUnregCh := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyUnregChallenge))
	reqUnregCh.Header.Set("Content-Type", "application/json")
	recUnregCh := httptest.NewRecorder()
	handler.ServeHTTP(recUnregCh, reqUnregCh)
	if recUnregCh.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unregistered native client challenge, got %d: %s", recUnregCh.Code, recUnregCh.Body.String())
	}

	// Unregistered native client cannot obtain desktop token
	bodyUnregLogin := `{"client_id":"unregistered-workstation-999","challenge":"ch-fake","client_proof":"00","username":"admin_operator","password":"AdminOperator#Pass2026!"}`
	reqUnregLogin := httptest.NewRequest("POST", "/api/v1/auth/desktop/login", strings.NewReader(bodyUnregLogin))
	reqUnregLogin.Header.Set("Content-Type", "application/json")
	recUnregLogin := httptest.NewRecorder()
	handler.ServeHTTP(recUnregLogin, reqUnregLogin)
	if recUnregLogin.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unregistered native client login, got %d: %s", recUnregLogin.Code, recUnregLogin.Body.String())
	}

	// 6. Enrolled native client authenticates via Challenge-Response flow
	dev, devKey, err := deskMgr.RegisterDevice("workstation-ops-01", "Control Room Console")
	if err != nil {
		t.Fatalf("failed to enroll desktop workstation: %v", err)
	}
	if dev.ClientID != "workstation-ops-01" || devKey == "" {
		t.Fatalf("invalid device enrollment")
	}

	// 6a. Request challenge
	bodyChallenge := `{"client_id":"workstation-ops-01"}`
	reqCh := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyChallenge))
	reqCh.Header.Set("Content-Type", "application/json")
	recCh := httptest.NewRecorder()
	handler.ServeHTTP(recCh, reqCh)
	if recCh.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for challenge request, got %d: %s", recCh.Code, recCh.Body.String())
	}

	var chResp struct {
		Challenge string `json:"challenge"`
		ClientID  string `json:"client_id"`
	}
	_ = json.Unmarshal(recCh.Body.Bytes(), &chResp)
	if chResp.Challenge == "" {
		t.Fatalf("expected non-empty challenge string")
	}

	// 6b. Invalid proof fails login
	badLoginPayload := map[string]string{
		"client_id":    "workstation-ops-01",
		"challenge":    chResp.Challenge,
		"client_proof": "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		"username":     "admin_operator",
		"password":     "AdminOperator#Pass2026!",
	}
	bBad, _ := json.Marshal(badLoginPayload)
	reqBadProof := httptest.NewRequest("POST", "/api/v1/auth/desktop/login", bytes.NewReader(bBad))
	reqBadProof.Header.Set("Content-Type", "application/json")
	recBadProof := httptest.NewRecorder()
	handler.ServeHTTP(recBadProof, reqBadProof)
	if recBadProof.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid device proof, got %d: %s", recBadProof.Code, recBadProof.Body.String())
	}

	// 6c. Obtain fresh challenge for valid authentication (single use consumed previous)
	recCh2 := httptest.NewRecorder()
	reqCh2 := httptest.NewRequest("POST", "/api/v1/auth/desktop/challenge", strings.NewReader(bodyChallenge))
	reqCh2.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(recCh2, reqCh2)
	_ = json.Unmarshal(recCh2.Body.Bytes(), &chResp)

	// Valid proof calculation using workstation device key from OS credential vault
	validProof := security.ComputeProofHelper(devKey, chResp.Challenge)

	validLoginPayload := map[string]string{
		"client_id":    "workstation-ops-01",
		"challenge":    chResp.Challenge,
		"client_proof": validProof,
		"username":     "admin_operator",
		"password":     "AdminOperator#Pass2026!",
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
		ClientID  string `json:"client_id"`
	}
	if err := json.Unmarshal(recValidProof.Body.Bytes(), &deskLoginResp); err != nil {
		t.Fatalf("failed to decode desktop login response: %v", err)
	}
	if !deskLoginResp.Success || deskLoginResp.Token == "" || deskLoginResp.TokenType != "Bearer" {
		t.Fatalf("expected valid bearer token for native desktop client, got: %+v", deskLoginResp)
	}
	desktopBearerToken := deskLoginResp.Token

	// 6d. Use the issued desktop bearer token to access protected API endpoints
	reqStatus := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	reqStatus.Header.Set("Authorization", "Bearer "+desktopBearerToken)
	recStatus := httptest.NewRecorder()
	handler.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 OK using desktop bearer token, got %d: %s", recStatus.Code, recStatus.Body.String())
	}

	// 7. Revoked and expired desktop tokens are rejected
	// 7a. Revoke session server-side
	rbacMgr.RevokeSession(desktopBearerToken)

	reqRevoked := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	reqRevoked.Header.Set("Authorization", "Bearer "+desktopBearerToken)
	recRevoked := httptest.NewRecorder()
	handler.ServeHTTP(recRevoked, reqRevoked)
	if recRevoked.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for revoked desktop token, got %d: %s", recRevoked.Code, recRevoked.Body.String())
	}

	// 7b. Expired token
	expiredToken, err := rbacMgr.Authenticate("admin_operator", "AdminOperator#Pass2026!", 1*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create short session: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // Allow token to expire

	reqExpired := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	reqExpired.Header.Set("Authorization", "Bearer "+expiredToken.Token)
	recExpired := httptest.NewRecorder()
	handler.ServeHTTP(recExpired, reqExpired)
	if recExpired.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for expired desktop token, got %d: %s", recExpired.Code, recExpired.Body.String())
	}
	if !strings.Contains(recExpired.Body.String(), "SESSION_EXPIRED") {
		t.Errorf("expected SESSION_EXPIRED code, got %s", recExpired.Body.String())
	}
}

func TestDesktopAuth_AdminDeviceEnrollment(t *testing.T) {
	_, handler, rbacMgr, _ := setupDesktopAPITestServer(t)

	// Complete initial admin setup
	setupTok := rbacMgr.GetSetupToken()
	_ = rbacMgr.CompleteSetup(setupTok, "admin_user", "AdminOperator#Pass2026!", "Administrator", "admin@sentinel.io")

	// Authenticate admin session
	sess, err := rbacMgr.Authenticate("admin_user", "AdminOperator#Pass2026!", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to authenticate admin: %v", err)
	}
	adminToken := sess.Token

	// 1. Unauthenticated request to register device is rejected (401)
	reqUnauth := httptest.NewRequest("POST", "/api/v1/auth/desktop/devices", strings.NewReader(`{"client_id":"dev-1"}`))
	recUnauth := httptest.NewRecorder()
	handler.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated device registration must return 401, got %d", recUnauth.Code)
	}

	// 2. Admin enrolls new device
	enrollPayload := `{"client_id":"workstation-ops-99","device_name":"Surveillance Console 99"}`
	reqEnroll := httptest.NewRequest("POST", "/api/v1/auth/desktop/devices", strings.NewReader(enrollPayload))
	reqEnroll.Header.Set("Authorization", "Bearer "+adminToken)
	reqEnroll.Header.Set("Content-Type", "application/json")
	recEnroll := httptest.NewRecorder()
	handler.ServeHTTP(recEnroll, reqEnroll)

	if recEnroll.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for device enrollment, got %d: %s", recEnroll.Code, recEnroll.Body.String())
	}

	var enrollResp struct {
		Success   bool   `json:"success"`
		ClientID  string `json:"client_id"`
		DeviceKey string `json:"device_key"`
	}
	_ = json.Unmarshal(recEnroll.Body.Bytes(), &enrollResp)
	if !enrollResp.Success || enrollResp.ClientID != "workstation-ops-99" || enrollResp.DeviceKey == "" {
		t.Fatalf("expected device enrollment response with client_id and device_key, got: %+v", enrollResp)
	}

	// 3. Admin lists devices and verifies device_key is NOT leaked in list
	reqList := httptest.NewRequest("GET", "/api/v1/auth/desktop/devices", nil)
	reqList.Header.Set("Authorization", "Bearer "+adminToken)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for device list, got %d: %s", recList.Code, recList.Body.String())
	}

	listBody := recList.Body.String()
	if strings.Contains(listBody, enrollResp.DeviceKey) {
		t.Fatalf("CRITICAL SECURITY VIOLATION: device_key leaked in device list JSON response!")
	}
	if !strings.Contains(listBody, "workstation-ops-99") {
		t.Errorf("expected enrolled device in list response")
	}
}
