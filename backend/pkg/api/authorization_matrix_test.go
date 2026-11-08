package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/alarms"
	"cctv-sentinel/pkg/api"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/events"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/recorder"
	"cctv-sentinel/pkg/security"
)

// setupFullMatrixTestServer initializes an isolated Sentinel test server with database seed
func setupFullMatrixTestServer(t *testing.T) (*api.Server, http.Handler, *database.Store, *security.RBACManager) {
	tempDir := t.TempDir()
	store := database.NewStore(tempDir + "/matrix_store.json")
	registry := adapters.NewRegistry()
	logger := audit.NewLogger()
	cfgEngine := configengine.NewEngine(registry, store, logger)
	discEngine := discovery.NewEngine(registry)
	licMgr := licensing.NewManager()
	rbacMgr := security.NewRBACManager()

	srv := api.NewServer(store, registry, cfgEngine, discEngine, licMgr, logger, nil, nil, nil, nil, rbacMgr)
	return srv, srv.Handler(), store, rbacMgr
}

// 1. Missing Token, Bad Token, Expired Token, and Query-String Token tests
func TestAuthMatrix_TokenHandlingAndValidation(t *testing.T) {
	srv, handler, store, rbacMgr := setupFullMatrixTestServer(t)

	// Seed standard device
	store.UpsertDevice(models.Device{
		ID:           "cam-auth-test",
		Name:         "Entrance Camera",
		IP:           "192.168.1.100",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})

	// 1.1 Unauthenticated requests return 401
	unauthRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/devices"},
		{"GET", "/api/v1/devices/cam-auth-test"},
		{"POST", "/api/v1/devices/cam-auth-test/reboot"},
		{"GET", "/api/v1/devices/cam-auth-test/ptz/capabilities"},
		{"POST", "/api/v1/devices/cam-auth-test/ptz/move"},
		{"GET", "/api/v1/recorders"},
		{"GET", "/api/v1/events/live"},
		{"GET", "/api/v1/alarms"},
		{"GET", "/api/v1/security/users"},
		{"GET", "/api/v1/audit/logs"},
		{"GET", "/api/v1/sites"},
	}

	for _, r := range unauthRoutes {
		req := httptest.NewRequest(r.method, r.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for %s %s, got %d", r.method, r.path, rec.Code)
		}
	}

	// 1.2 Invalid Bearer token returns 401
	badTokenReq := httptest.NewRequest("GET", "/api/v1/devices", nil)
	badTokenReq.Header.Set("Authorization", "Bearer invalid-garbage-token-xyz")
	badTokenRec := httptest.NewRecorder()
	handler.ServeHTTP(badTokenRec, badTokenReq)

	if badTokenRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", badTokenRec.Code)
	}

	// 1.3 Query-parameter token ?token=... is strictly rejected (401)
	validToken := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")
	queryParamReq := httptest.NewRequest("GET", "/api/v1/devices?token="+validToken, nil)
	queryParamRec := httptest.NewRecorder()
	handler.ServeHTTP(queryParamRec, queryParamReq)

	if queryParamRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for token passed via query parameter (must not be accepted), got %d", queryParamRec.Code)
	}

	// 1.4 Expired token returns 401 with SESSION_EXPIRED code
	expiredTok := "test-expired-token-999"
	rbacMgr.InjectSessionForTest(expiredTok, "expired-operator", security.RoleAdmin, "default", "hq", -1*time.Minute)
	expiredReq := httptest.NewRequest("GET", "/api/v1/devices", nil)
	expiredReq.Header.Set("Authorization", "Bearer "+expiredTok)
	expiredRec := httptest.NewRecorder()
	handler.ServeHTTP(expiredRec, expiredReq)

	if expiredRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", expiredRec.Code)
	}
	var errResp api.ErrorResponse
	if err := json.Unmarshal(expiredRec.Body.Bytes(), &errResp); err == nil {
		if errResp.Error.Code != "SESSION_EXPIRED" {
			t.Errorf("expected error code 'SESSION_EXPIRED', got %q", errResp.Error.Code)
		}
	}
}

// 2. Cookie authentication and session revocation via logout
func TestAuthMatrix_CookieSessionAndRevocation(t *testing.T) {
	srv, handler, _, rbacMgr := setupFullMatrixTestServer(t)

	// Complete initial setup
	setupToken := rbacMgr.GetSetupToken()
	_ = rbacMgr.CompleteSetup(setupToken, "admin_user", "TestMatrixAdmin#2026!", "Admin User", "admin@sentinel.io")

	loginBody := `{"username":"admin_user","password":"TestMatrixAdmin#2026!"}`

	// 1. Browser login without Origin must fail closed with 403 CSRF_MISSING_ORIGIN
	loginNoOrigin := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginNoOrigin.Header.Set("Content-Type", "application/json")
	recNoOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recNoOrigin, loginNoOrigin)
	if recNoOrigin.Code != http.StatusForbidden || !strings.Contains(recNoOrigin.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Fatalf("expected 403 CSRF_MISSING_ORIGIN for browser login without Origin, got %d: %s", recNoOrigin.Code, recNoOrigin.Body.String())
	}

	// 1a. Browser login with X-Sentinel-Origin alone (no Origin) must fail closed with 403 CSRF_MISSING_ORIGIN
	loginXOrigin := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginXOrigin.Header.Set("Content-Type", "application/json")
	loginXOrigin.Header.Set("X-Sentinel-Origin", "http://localhost:5173")
	recXOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recXOrigin, loginXOrigin)
	if recXOrigin.Code != http.StatusForbidden || !strings.Contains(recXOrigin.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Fatalf("expected 403 CSRF_MISSING_ORIGIN for browser login with X-Sentinel-Origin alone, got %d: %s", recXOrigin.Code, recXOrigin.Body.String())
	}

	// 1b. Browser login with Referer alone (no Origin) must fail closed with 403 CSRF_MISSING_ORIGIN
	loginReferer := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginReferer.Header.Set("Content-Type", "application/json")
	loginReferer.Header.Set("Referer", "http://localhost:5173/dashboard")
	recReferer := httptest.NewRecorder()
	handler.ServeHTTP(recReferer, loginReferer)
	if recReferer.Code != http.StatusForbidden || !strings.Contains(recReferer.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Fatalf("expected 403 CSRF_MISSING_ORIGIN for browser login with Referer alone, got %d: %s", recReferer.Code, recReferer.Body.String())
	}

	// 2. Browser login with untrusted Origin must fail with 403 CSRF_UNTRUSTED_ORIGIN
	loginUntrusted := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginUntrusted.Header.Set("Content-Type", "application/json")
	loginUntrusted.Header.Set("Origin", "http://attacker-site.com")
	recUntrusted := httptest.NewRecorder()
	handler.ServeHTTP(recUntrusted, loginUntrusted)
	if recUntrusted.Code != http.StatusForbidden || !strings.Contains(recUntrusted.Body.String(), "CSRF_UNTRUSTED_ORIGIN") {
		t.Fatalf("expected 403 CSRF_UNTRUSTED_ORIGIN for untrusted Origin, got %d: %s", recUntrusted.Code, recUntrusted.Body.String())
	}

	// 3. Desktop client (X-Client-Type: desktop) without Origin succeeds
	loginDesktop := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginDesktop.Header.Set("Content-Type", "application/json")
	loginDesktop.Header.Set("X-Client-Type", "desktop")
	recDesktop := httptest.NewRecorder()
	handler.ServeHTTP(recDesktop, loginDesktop)
	if recDesktop.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for desktop login without Origin, got %d: %s", recDesktop.Code, recDesktop.Body.String())
	}

	// 4. Authenticate via /api/v1/security/login with trusted Origin
	loginReq := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", "http://localhost:5173")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid login, got %d: %s", loginRec.Code, loginRec.Body.String())
	}

	// Verify Set-Cookie header contains sentinel_session
	cookieHeader := loginRec.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "sentinel_session=") {
		t.Fatalf("expected sentinel_session cookie in login response, got: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "HttpOnly") || !strings.Contains(cookieHeader, "SameSite=Strict") {
		t.Errorf("cookie missing HttpOnly or SameSite=Strict: %s", cookieHeader)
	}

	// Extract cookie
	cookies := loginRec.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sentinel_session" {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("sentinel_session cookie not parsed")
	}

	// 2.1 Authenticated request using cookie succeeds
	cookieReq := httptest.NewRequest("GET", "/api/v1/devices", nil)
	cookieReq.AddCookie(sessCookie)
	cookieRec := httptest.NewRecorder()
	handler.ServeHTTP(cookieRec, cookieReq)

	if cookieRec.Code != http.StatusOK {
		t.Errorf("expected 200 OK using cookie auth, got %d", cookieRec.Code)
	}

	// 2.2 Logout revokes session server-side and clears cookie
	logoutReq := httptest.NewRequest("POST", "/api/v1/security/logout", nil)
	logoutReq.AddCookie(sessCookie)
	logoutReq.Header.Set("Origin", "http://localhost:5173")
	logoutRec := httptest.NewRecorder()
	handler.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for logout, got %d", logoutRec.Code)
	}
	clearedCookie := logoutRec.Header().Get("Set-Cookie")
	if !strings.Contains(clearedCookie, "Max-Age=0") && !strings.Contains(clearedCookie, "sentinel_session=;") && !strings.Contains(clearedCookie, "Expires=") {
		t.Errorf("expected expired cookie in logout response, got: %s", clearedCookie)
	}

	// 2.3 Subsequent request using the revoked token returns 401
	subseqReq := httptest.NewRequest("GET", "/api/v1/devices", nil)
	subseqReq.Header.Set("Authorization", "Bearer "+sessCookie.Value)
	subseqRec := httptest.NewRecorder()
	handler.ServeHTTP(subseqRec, subseqReq)

	if subseqRec.Code != http.StatusUnauthorized {
		t.Errorf("revoked session must return 401 Unauthorized, got %d", subseqRec.Code)
	}

	_ = srv
}

// 3. Viewer Role: Strict prohibition against PTZ, recorder, event, alarm, and system changes
func TestAuthMatrix_ViewerForbiddenActions(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	// Seed test camera & recorder
	store.UpsertDevice(models.Device{
		ID:           "cam-viewer-test",
		Name:         "PTZ Dome 1",
		IP:           "192.168.1.101",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})
	store.UpsertDevice(models.Device{
		ID:           "rec-viewer-test",
		Name:         "NVR 1",
		IP:           "192.168.1.102",
		Category:     models.CategoryNVR,
		Manufacturer: models.VendorHikvision,
		ChannelCount: 16,
		TenantID:     "default",
		SiteID:       "hq",
	})

	viewerToken := srv.IssueSessionForTest(security.RoleViewer, "default", "hq")

	forbiddenViewerEndpoints := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		// PTZ Control endpoints
		{"PTZ Move", "POST", "/api/v1/devices/cam-viewer-test/ptz/move", `{"pan":0.5,"tilt":0.0}`},
		{"PTZ Stop", "POST", "/api/v1/devices/cam-viewer-test/ptz/stop", `{}`},
		{"PTZ Focus/Iris", "POST", "/api/v1/devices/cam-viewer-test/ptz/focus-iris", `{"command":"FOCUS_NEAR"}`},
		{"PTZ Goto Preset", "POST", "/api/v1/devices/cam-viewer-test/ptz/presets/1/goto", `{}`},
		{"PTZ Create Preset", "POST", "/api/v1/devices/cam-viewer-test/ptz/presets", `{"id":1,"name":"Gate"}`},
		{"PTZ Rename Preset", "PUT", "/api/v1/devices/cam-viewer-test/ptz/presets/1", `{"name":"Back Gate"}`},
		{"PTZ Delete Preset", "DELETE", "/api/v1/devices/cam-viewer-test/ptz/presets/1", `{}`},
		{"PTZ Run Patrol", "POST", "/api/v1/devices/cam-viewer-test/ptz/patrols/1/run", `{}`},
		{"PTZ Stop Patrol", "POST", "/api/v1/devices/cam-viewer-test/ptz/patrols/1/stop", `{}`},
		{"PTZ Create Patrol", "POST", "/api/v1/devices/cam-viewer-test/ptz/patrols", `{"id":1,"name":"Tour"}`},
		{"PTZ Goto Home", "POST", "/api/v1/devices/cam-viewer-test/ptz/home", `{"action":"goto"}`},
		{"PTZ Set Home", "POST", "/api/v1/devices/cam-viewer-test/ptz/home", `{"action":"set"}`},
		{"PTZ AutoScan", "POST", "/api/v1/devices/cam-viewer-test/ptz/autoscan", `{"enabled":true}`},
		// Recorder Management endpoints
		{"Recorder Analog Format", "POST", "/api/v1/recorders/rec-viewer-test/analog-format", `{"channel_id":1,"format":"AHD"}`},
		{"Recorder Coaxial Control", "POST", "/api/v1/recorders/rec-viewer-test/coaxial-control", `{"channel_id":1,"command":"UP"}`},
		{"Recorder Register IP", "POST", "/api/v1/recorders/rec-viewer-test/ip-registration", `{"channel_id":1,"ip":"192.168.1.150"}`},
		{"Recorder Schedule Update", "PUT", "/api/v1/recorders/rec-viewer-test/schedule", `{"channel_id":1}`},
		{"Recorder Export Recording", "POST", "/api/v1/recorders/rec-viewer-test/export", `{"channel_id":1,"segment_id":"seg-1"}`},
		{"Recorder Disk Format Preview", "POST", "/api/v1/recorders/rec-viewer-test/storage/format-preview", `{"disk_index":1}`},
		{"Recorder Disk Format Approve", "POST", "/api/v1/recorders/rec-viewer-test/storage/format-approve", `{"format_token":"tok","confirmation":"CONFIRM"}`},
		// Events & Alarms endpoints
		{"Create Event Subscription", "POST", "/api/v1/events/subscriptions", `{"device_id":"cam-viewer-test"}`},
		{"Cancel Event Subscription", "DELETE", "/api/v1/events/subscriptions/sub-123", `{}`},
		{"Create Alarm Rule", "POST", "/api/v1/alarms/rules", `{"name":"Motion Rule"}`},
		{"Delete Alarm Rule", "DELETE", "/api/v1/alarms/rules/rule-123", `{}`},
		{"Export Alarms", "GET", "/api/v1/alarms/export", ``},
		// Administration & Device Modification
		{"Reboot Device", "POST", "/api/v1/devices/cam-viewer-test/reboot", `{}`},
		{"Delete Device", "DELETE", "/api/v1/devices/cam-viewer-test", `{}`},
		{"Apply Device Config", "POST", "/api/v1/devices/cam-viewer-test/config/apply", `{"type":"NETWORK","preview_token":"tok"}`},
		{"Batch Preview", "POST", "/api/v1/batch/preview", `{"action":"REBOOT","target_ids":["cam-viewer-test"]}`},
		{"Batch Execute", "POST", "/api/v1/batch/execute", `{"preview_token":"tok"}`},
		{"Firmware Preview", "POST", "/api/v1/firmware/preview", `{"device_id":"cam-viewer-test","package_id":"pkg-1"}`},
		{"Firmware Approve", "POST", "/api/v1/firmware/approve", `{"preview_token":"tok"}`},
		{"List Users", "GET", "/api/v1/security/users", ``},
		{"Create User", "POST", "/api/v1/security/users", `{"username":"new_user"}`},
		{"Rotate Audit Key", "POST", "/api/v1/audit/rotate-key", `{}`},
	}

	for _, ep := range forbiddenViewerEndpoints {
		t.Run("Viewer_Forbidden_"+ep.name, func(t *testing.T) {
			var req *http.Request
			if ep.body != "" {
				req = httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("Viewer must be forbidden (403) on %s %s (%s), got status %d: %s",
					ep.method, ep.path, ep.name, rec.Code, rec.Body.String())
			}
		})
	}

	// Viewer ALLOWED read-only view endpoints:
	allowedViewerEndpoints := []struct {
		name   string
		method string
		path   string
	}{
		{"List Devices", "GET", "/api/v1/devices"},
		{"Get Device", "GET", "/api/v1/devices/cam-viewer-test"},
		{"Get PTZ Capabilities", "GET", "/api/v1/devices/cam-viewer-test/ptz/capabilities"},
		{"List Presets", "GET", "/api/v1/devices/cam-viewer-test/ptz/presets"},
		{"List Patrols", "GET", "/api/v1/devices/cam-viewer-test/ptz/patrols"},
		{"List Recorders", "GET", "/api/v1/recorders"},
		{"Get Recorder Profile", "GET", "/api/v1/recorders/rec-viewer-test"},
		{"Get Recorder Channels", "GET", "/api/v1/recorders/rec-viewer-test/channels"},
		{"Get Recorder Schedule", "GET", "/api/v1/recorders/rec-viewer-test/schedule?channel_id=1"},
		{"Search Recordings", "GET", "/api/v1/recorders/rec-viewer-test/search-recordings?channel_id=1"},
		{"Get Live Events", "GET", "/api/v1/events/live"},
		{"Get Alarms", "GET", "/api/v1/alarms"},
	}

	for _, ep := range allowedViewerEndpoints {
		t.Run("Viewer_Allowed_"+ep.name, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Viewer must be allowed (200 OK) on %s %s (%s), got status %d: %s",
					ep.method, ep.path, ep.name, rec.Code, rec.Body.String())
			}
		})
	}
}

// 4. Auditor Role: Read-only & compliance export allowed, motor control & modification forbidden
func TestAuthMatrix_AuditorRoleCapabilities(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	store.UpsertDevice(models.Device{
		ID:           "cam-auditor-test",
		Name:         "Perimeter Camera",
		IP:           "192.168.1.103",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})

	auditorToken := srv.IssueSessionForTest(security.RoleAuditor, "default", "hq")

	// Auditor CAN read audit logs and export signed ledger
	reqAudit := httptest.NewRequest("GET", "/api/v1/audit/logs", nil)
	reqAudit.Header.Set("Authorization", "Bearer "+auditorToken)
	recAudit := httptest.NewRecorder()
	handler.ServeHTTP(recAudit, reqAudit)
	if recAudit.Code != http.StatusOK {
		t.Errorf("Auditor must be able to view audit logs, got %d", recAudit.Code)
	}

	reqSigned := httptest.NewRequest("GET", "/api/v1/audit/export/signed", nil)
	reqSigned.Header.Set("Authorization", "Bearer "+auditorToken)
	recSigned := httptest.NewRecorder()
	handler.ServeHTTP(recSigned, reqSigned)
	if recSigned.Code != http.StatusOK {
		t.Errorf("Auditor must be able to export signed audit ledger, got %d", recSigned.Code)
	}

	// Auditor CAN export alarms
	reqAlarmExp := httptest.NewRequest("GET", "/api/v1/alarms/export?format=json", nil)
	reqAlarmExp.Header.Set("Authorization", "Bearer "+auditorToken)
	recAlarmExp := httptest.NewRecorder()
	handler.ServeHTTP(recAlarmExp, reqAlarmExp)
	if recAlarmExp.Code != http.StatusOK {
		t.Errorf("Auditor must be able to export alarms, got %d", recAlarmExp.Code)
	}

	// Auditor CANNOT move PTZ
	reqPTZ := httptest.NewRequest("POST", "/api/v1/devices/cam-auditor-test/ptz/move", strings.NewReader(`{"pan":0.5}`))
	reqPTZ.Header.Set("Authorization", "Bearer "+auditorToken)
	reqPTZ.Header.Set("Content-Type", "application/json")
	recPTZ := httptest.NewRecorder()
	handler.ServeHTTP(recPTZ, reqPTZ)
	if recPTZ.Code != http.StatusForbidden {
		t.Errorf("Auditor must be forbidden from moving PTZ, got %d", recPTZ.Code)
	}

	// Auditor CANNOT reboot device
	reqReboot := httptest.NewRequest("POST", "/api/v1/devices/cam-auditor-test/reboot", nil)
	reqReboot.Header.Set("Authorization", "Bearer "+auditorToken)
	recReboot := httptest.NewRecorder()
	handler.ServeHTTP(recReboot, reqReboot)
	if recReboot.Code != http.StatusForbidden {
		t.Errorf("Auditor must be forbidden from rebooting devices, got %d", recReboot.Code)
	}

	// Auditor CANNOT rotate HMAC checkpoint key
	reqRot := httptest.NewRequest("POST", "/api/v1/audit/rotate-key", nil)
	reqRot.Header.Set("Authorization", "Bearer "+auditorToken)
	recRot := httptest.NewRecorder()
	handler.ServeHTTP(recRot, reqRot)
	if recRot.Code != http.StatusForbidden {
		t.Errorf("Auditor must be forbidden from rotating HMAC checkpoint keys, got %d", recRot.Code)
	}
}

// 5. Technician Role: Operational authority allowed, user management & audit key rotation forbidden
func TestAuthMatrix_TechnicianRoleCapabilities(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	store.UpsertDevice(models.Device{
		ID:           "cam-tech-test",
		Name:         "PTZ Workshop",
		IP:           "192.168.1.104",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})

	techToken := srv.IssueSessionForTest(security.RoleTechnician, "default", "hq")

	// Technician CAN perform PTZ operations (mocked continuous move)
	reqPTZ := httptest.NewRequest("POST", "/api/v1/devices/cam-tech-test/ptz/move", strings.NewReader(`{"movement_type":"CONTINUOUS","pan":0.5,"speed":0.5}`))
	reqPTZ.Header.Set("Authorization", "Bearer "+techToken)
	reqPTZ.Header.Set("Content-Type", "application/json")
	recPTZ := httptest.NewRecorder()
	handler.ServeHTTP(recPTZ, reqPTZ)
	if recPTZ.Code != http.StatusOK {
		t.Errorf("Technician should be allowed to execute PTZ movement, got %d: %s", recPTZ.Code, recPTZ.Body.String())
	}

	// Technician CANNOT access user management
	reqUsers := httptest.NewRequest("GET", "/api/v1/security/users", nil)
	reqUsers.Header.Set("Authorization", "Bearer "+techToken)
	recUsers := httptest.NewRecorder()
	handler.ServeHTTP(recUsers, reqUsers)
	if recUsers.Code != http.StatusForbidden {
		t.Errorf("Technician must be forbidden from listing user accounts, got %d", recUsers.Code)
	}

	// Technician CANNOT rotate audit checkpoint keys
	reqRot := httptest.NewRequest("POST", "/api/v1/audit/rotate-key", nil)
	reqRot.Header.Set("Authorization", "Bearer "+techToken)
	recRot := httptest.NewRecorder()
	handler.ServeHTTP(recRot, reqRot)
	if recRot.Code != http.StatusForbidden {
		t.Errorf("Technician must be forbidden from rotating audit keys, got %d", recRot.Code)
	}
}

// 6. Cross-Tenant and Cross-Site Isolation Enforcement
func TestAuthMatrix_CrossTenantAndCrossSiteIsolation(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	// Seed Tenant-A device & Tenant-B device
	store.UpsertDevice(models.Device{
		ID:           "cam-tenant-a",
		Name:         "Tenant A Cam",
		IP:           "10.10.1.10",
		Manufacturer: models.VendorHikvision,
		TenantID:     "tenant-alpha",
		SiteID:       "site-alpha-1",
	})
	store.UpsertDevice(models.Device{
		ID:           "cam-tenant-b",
		Name:         "Tenant B Cam",
		IP:           "10.10.2.10",
		Manufacturer: models.VendorDahua,
		TenantID:     "tenant-beta",
		SiteID:       "site-beta-1",
	})
	store.UpsertDevice(models.Device{
		ID:           "rec-tenant-b",
		Name:         "Tenant B Recorder",
		IP:           "10.10.2.20",
		Category:     models.CategoryNVR,
		Manufacturer: models.VendorDahua,
		ChannelCount: 8,
		TenantID:     "tenant-beta",
		SiteID:       "site-beta-1",
	})

	// Operator token scoped strictly to Tenant Alpha
	tokenAlpha := srv.IssueSessionForTest(security.RoleAdmin, "tenant-alpha", "site-alpha-1")

	crossTenantAttempts := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"Direct Device Read", "GET", "/api/v1/devices/cam-tenant-b", ""},
		{"Device PTZ Move", "POST", "/api/v1/devices/cam-tenant-b/ptz/move", `{"pan":0.5}`},
		{"Direct Recorder Read", "GET", "/api/v1/recorders/rec-tenant-b", ""},
		{"Recorder Channels", "GET", "/api/v1/recorders/rec-tenant-b/channels", ""},
		{"Batch Preview Targeting Tenant B", "POST", "/api/v1/batch/preview", `{"action":"REBOOT","target_ids":["cam-tenant-b"]}`},
		{"Batch Compare Targeting Tenant B", "POST", "/api/v1/batch/compare", `{"target_ids":["cam-tenant-a","cam-tenant-b"]}`},
		{"Firmware Compatibility Target B", "POST", "/api/v1/firmware/check-compatibility", `{"device_id":"cam-tenant-b","package_id":"pkg-1"}`},
		{"Firmware Preview Target B", "POST", "/api/v1/firmware/preview", `{"device_id":"cam-tenant-b","package_id":"pkg-1"}`},
		{"Create Event Subscription Target B", "POST", "/api/v1/events/subscriptions", `{"device_id":"cam-tenant-b","protocol":"ONVIF_PULLPOINT"}`},
	}

	for _, tc := range crossTenantAttempts {
		t.Run("CrossTenant_"+tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header.Set("Authorization", "Bearer "+tokenAlpha)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden on %s (%s %s), got %d: %s",
					tc.name, tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}

	// 6.2 Cross-Site Isolation: User restricted to Site 1 cannot access Site 2 of the same tenant
	store.UpsertDevice(models.Device{
		ID:           "cam-tenant-a-site2",
		Name:         "Tenant A Site 2 Cam",
		IP:           "10.10.1.20",
		Manufacturer: models.VendorHikvision,
		TenantID:     "tenant-alpha",
		SiteID:       "site-alpha-2",
	})

	siteReq := httptest.NewRequest("GET", "/api/v1/devices/cam-tenant-a-site2", nil)
	siteReq.Header.Set("Authorization", "Bearer "+tokenAlpha) // scoped to site-alpha-1
	siteRec := httptest.NewRecorder()
	handler.ServeHTTP(siteRec, siteReq)

	if siteRec.Code != http.StatusForbidden {
		t.Errorf("Cross-site access attempt within same tenant must return 403 Forbidden, got %d", siteRec.Code)
	}
}

// 7. Legacy Device Migration: Unassigned devices reject tenant users, accept super-admin, and migrate cleanly
func TestAuthMatrix_LegacyDeviceMigration(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	// Seed unassigned legacy device with empty TenantID and SiteID
	store.UpsertDevice(models.Device{
		ID:           "legacy-dev-unassigned",
		Name:         "Unassigned Analog Camera",
		IP:           "192.168.1.250",
		Manufacturer: models.VendorHikvision,
		TenantID:     "",
		SiteID:       "",
	})

	tenantToken := srv.IssueSessionForTest(security.RoleAdmin, "tenant-gamma", "site-gamma-1")
	superAdminToken := srv.IssueGlobalAdminSessionForTest()

	// 7.1 Tenant-scoped user access to unassigned legacy device is strictly rejected (403)
	reqTenant := httptest.NewRequest("GET", "/api/v1/devices/legacy-dev-unassigned", nil)
	reqTenant.Header.Set("Authorization", "Bearer "+tenantToken)
	recTenant := httptest.NewRecorder()
	handler.ServeHTTP(recTenant, reqTenant)

	if recTenant.Code != http.StatusForbidden {
		t.Errorf("Tenant user accessing unassigned legacy device must receive 403 Forbidden, got %d", recTenant.Code)
	}

	// 7.2 Global Super-Admin CAN access unassigned legacy device
	reqAdmin := httptest.NewRequest("GET", "/api/v1/devices/legacy-dev-unassigned", nil)
	reqAdmin.Header.Set("Authorization", "Bearer "+superAdminToken)
	recAdmin := httptest.NewRecorder()
	handler.ServeHTTP(recAdmin, reqAdmin)

	if recAdmin.Code != http.StatusOK {
		t.Errorf("Super-Admin must be able to view unassigned legacy device, got %d: %s", recAdmin.Code, recAdmin.Body.String())
	}

	// 7.3 Non-superadmin cannot invoke migration endpoint (403)
	migratePayload := `{"target_tenant":"tenant-gamma","target_site":"site-gamma-1"}`
	reqNonAdminMigrate := httptest.NewRequest("POST", "/api/v1/devices/legacy-dev-unassigned/migrate", strings.NewReader(migratePayload))
	reqNonAdminMigrate.Header.Set("Authorization", "Bearer "+tenantToken)
	reqNonAdminMigrate.Header.Set("Content-Type", "application/json")
	recNonAdminMigrate := httptest.NewRecorder()
	handler.ServeHTTP(recNonAdminMigrate, reqNonAdminMigrate)

	if recNonAdminMigrate.Code != http.StatusForbidden {
		t.Errorf("Non-superadmin must be forbidden (403) from calling /migrate, got %d", recNonAdminMigrate.Code)
	}

	// 7.4 Global Super-Admin migrates the unassigned device to tenant-gamma
	reqAdminMigrate := httptest.NewRequest("POST", "/api/v1/devices/legacy-dev-unassigned/migrate", strings.NewReader(migratePayload))
	reqAdminMigrate.Header.Set("Authorization", "Bearer "+superAdminToken)
	reqAdminMigrate.Header.Set("Content-Type", "application/json")
	recAdminMigrate := httptest.NewRecorder()
	handler.ServeHTTP(recAdminMigrate, reqAdminMigrate)

	if recAdminMigrate.Code != http.StatusOK {
		t.Fatalf("Super-Admin migration failed, got %d: %s", recAdminMigrate.Code, recAdminMigrate.Body.String())
	}

	// 7.5 After migration, tenant-gamma user CAN access the migrated device
	reqTenantAfter := httptest.NewRequest("GET", "/api/v1/devices/legacy-dev-unassigned", nil)
	reqTenantAfter.Header.Set("Authorization", "Bearer "+tenantToken)
	recTenantAfter := httptest.NewRecorder()
	handler.ServeHTTP(recTenantAfter, reqTenantAfter)

	if recTenantAfter.Code != http.StatusOK {
		t.Errorf("Tenant user must be able to access device after migration to their tenant, got %d: %s",
			recTenantAfter.Code, recTenantAfter.Body.String())
	}

	// Verify migrated metadata in store
	migratedDev, ok := store.GetDevice("legacy-dev-unassigned")
	if !ok || migratedDev.TenantID != "tenant-gamma" || migratedDev.SiteID != "site-gamma-1" {
		t.Errorf("Device metadata not updated properly: %+v", migratedDev)
	}
}

// 8. Prove Default Admin Cannot Access Other Tenants (Isolation Hardening)
func TestAuthMatrix_DefaultAdminCannotAccessOtherTenants(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	// Seed tenant-beta device
	store.UpsertDevice(models.Device{
		ID:           "cam-tenant-beta",
		Name:         "Beta Branch Camera",
		IP:           "192.168.30.100",
		Manufacturer: models.VendorHikvision,
		TenantID:     "tenant-beta",
		SiteID:       "beta-hq",
	})

	// Seed default tenant device
	store.UpsertDevice(models.Device{
		ID:           "cam-default-hq",
		Name:         "Default HQ Camera",
		IP:           "192.168.1.100",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})

	// Ordinary administrator in "default" tenant (non-global)
	defaultAdminToken := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")
	// Global super-admin with ScopeGlobalAdmin
	globalAdminToken := srv.IssueGlobalAdminSessionForTest()

	// 8.1 Default Admin accessing tenant-beta camera directly must receive 403 CROSS_TENANT_ACCESS_DENIED
	reqGet := httptest.NewRequest("GET", "/api/v1/devices/cam-tenant-beta", nil)
	reqGet.Header.Set("Authorization", "Bearer "+defaultAdminToken)
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusForbidden {
		t.Errorf("Default Admin must be forbidden (403) from accessing tenant-beta device, got %d: %s", recGet.Code, recGet.Body.String())
	}
	if !strings.Contains(recGet.Body.String(), "CROSS_TENANT_ACCESS_DENIED") {
		t.Errorf("Expected CROSS_TENANT_ACCESS_DENIED error code, got %s", recGet.Body.String())
	}

	// 8.2 Default Admin attempting state-change on tenant-beta device must be rejected
	reqReboot := httptest.NewRequest("POST", "/api/v1/devices/cam-tenant-beta/reboot", nil)
	reqReboot.Header.Set("Authorization", "Bearer "+defaultAdminToken)
	recReboot := httptest.NewRecorder()
	handler.ServeHTTP(recReboot, reqReboot)

	if recReboot.Code != http.StatusForbidden {
		t.Errorf("Default Admin must be forbidden from rebooting tenant-beta device, got %d", recReboot.Code)
	}

	// 8.3 Default Admin listing devices must only see default tenant devices (tenant-beta is hidden)
	reqList := httptest.NewRequest("GET", "/api/v1/devices", nil)
	reqList.Header.Set("Authorization", "Bearer "+defaultAdminToken)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("Device list failed: %d", recList.Code)
	}
	var listedDevs []models.Device
	_ = json.NewDecoder(recList.Body).Decode(&listedDevs)
	if len(listedDevs) != 1 || listedDevs[0].ID != "cam-default-hq" {
		t.Errorf("Default Admin device list must contain only 'cam-default-hq', got %+v", listedDevs)
	}

	// 8.4 Global Super-Admin CAN access tenant-beta device
	reqGlobal := httptest.NewRequest("GET", "/api/v1/devices/cam-tenant-beta", nil)
	reqGlobal.Header.Set("Authorization", "Bearer "+globalAdminToken)
	recGlobal := httptest.NewRecorder()
	handler.ServeHTTP(recGlobal, reqGlobal)

	if recGlobal.Code != http.StatusOK {
		t.Errorf("Global Super-Admin must be able to view tenant-beta device, got %d: %s", recGlobal.Code, recGlobal.Body.String())
	}

	// 8.5 Global Super-Admin sees all devices in fleet
	reqGlobalList := httptest.NewRequest("GET", "/api/v1/devices", nil)
	reqGlobalList.Header.Set("Authorization", "Bearer "+globalAdminToken)
	recGlobalList := httptest.NewRecorder()
	handler.ServeHTTP(recGlobalList, reqGlobalList)

	if recGlobalList.Code != http.StatusOK {
		t.Fatalf("Global Admin device list failed: %d", recGlobalList.Code)
	}
	var allDevs []models.Device
	_ = json.NewDecoder(recGlobalList.Body).Decode(&allDevs)
	if len(allDevs) != 2 {
		t.Errorf("Global Admin must see all 2 devices, got %d", len(allDevs))
	}
}

// 9. CSRF & Origin Validation Protection Suite
func TestAuthMatrix_CSRFProtection(t *testing.T) {
	srv, handler, store, rbacMgr := setupFullMatrixTestServer(t)

	store.UpsertDevice(models.Device{
		ID:           "cam-csrf-device",
		Name:         "Perimeter Camera",
		IP:           "192.168.1.110",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})

	token := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")
	sess, err := rbacMgr.ValidateSession(token)
	if err != nil {
		t.Fatalf("Failed to retrieve session: %v", err)
	}
	csrfToken := sess.CSRFToken
	if csrfToken == "" {
		t.Fatalf("Expected non-empty CSRFToken on session")
	}

	bodyRule := `{"name":"CSRF Test Rule","event_type":"MotionDetection","action":"LOG"}`

	// 9.1 Cookie auth with state-changing POST and missing Origin header returns 403 CSRF_MISSING_ORIGIN
	reqMissingOrigin := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqMissingOrigin.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqMissingOrigin.Header.Set("X-CSRF-Token", csrfToken)
	recMissingOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recMissingOrigin, reqMissingOrigin)

	if recMissingOrigin.Code != http.StatusForbidden {
		t.Errorf("Cookie POST without Origin must return 403 Forbidden, got %d", recMissingOrigin.Code)
	}
	if !strings.Contains(recMissingOrigin.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Errorf("Expected CSRF_MISSING_ORIGIN, got %s", recMissingOrigin.Body.String())
	}

	// 9.2 Cookie auth with untrusted Origin returns 403 CSRF_UNTRUSTED_ORIGIN
	reqBadOrigin := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqBadOrigin.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqBadOrigin.Header.Set("Origin", "https://attacker-origin.evil.com")
	reqBadOrigin.Header.Set("X-CSRF-Token", csrfToken)
	recBadOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recBadOrigin, reqBadOrigin)

	if recBadOrigin.Code != http.StatusForbidden {
		t.Errorf("Cookie POST with untrusted Origin must return 403 Forbidden, got %d", recBadOrigin.Code)
	}
	if !strings.Contains(recBadOrigin.Body.String(), "CSRF_UNTRUSTED_ORIGIN") {
		t.Errorf("Expected CSRF_UNTRUSTED_ORIGIN, got %s", recBadOrigin.Body.String())
	}

	// 9.3 Cookie auth with trusted Origin and missing CSRF header returns 403 CSRF_TOKEN_INVALID
	reqNoCSRF := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqNoCSRF.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqNoCSRF.Header.Set("Origin", "http://localhost:5173")
	recNoCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recNoCSRF, reqNoCSRF)

	if recNoCSRF.Code != http.StatusForbidden {
		t.Errorf("Cookie POST without CSRF token must return 403 Forbidden, got %d", recNoCSRF.Code)
	}
	if !strings.Contains(recNoCSRF.Body.String(), "CSRF_TOKEN_INVALID") {
		t.Errorf("Expected CSRF_TOKEN_INVALID, got %s", recNoCSRF.Body.String())
	}

	// 9.4 Cookie auth with trusted Origin and invalid CSRF header returns 403 CSRF_TOKEN_INVALID
	reqBadCSRF := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqBadCSRF.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqBadCSRF.Header.Set("Origin", "http://localhost:5173")
	reqBadCSRF.Header.Set("X-CSRF-Token", "forged-csrf-token-123456")
	recBadCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recBadCSRF, reqBadCSRF)

	if recBadCSRF.Code != http.StatusForbidden {
		t.Errorf("Cookie POST with invalid CSRF token must return 403 Forbidden, got %d", recBadCSRF.Code)
	}
	if !strings.Contains(recBadCSRF.Body.String(), "CSRF_TOKEN_INVALID") {
		t.Errorf("Expected CSRF_TOKEN_INVALID, got %s", recBadCSRF.Body.String())
	}

	// 9.5 Cookie auth with trusted Origin and valid CSRF token succeeds (200 OK)
	reqValid := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqValid.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqValid.Header.Set("Origin", "http://localhost:5173")
	reqValid.Header.Set("X-CSRF-Token", csrfToken)
	recValid := httptest.NewRecorder()
	handler.ServeHTTP(recValid, reqValid)

	if recValid.Code != http.StatusOK {
		t.Errorf("Cookie POST with valid CSRF token and trusted Origin must succeed (200), got %d: %s", recValid.Code, recValid.Body.String())
	}

	// 9.6 Alternative header X-Sentinel-CSRF is also accepted
	reqAlt := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqAlt.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqAlt.Header.Set("Origin", "http://localhost:5173")
	reqAlt.Header.Set("X-Sentinel-CSRF", csrfToken)
	recAlt := httptest.NewRecorder()
	handler.ServeHTTP(recAlt, reqAlt)

	if recAlt.Code != http.StatusOK {
		t.Errorf("Cookie POST with X-Sentinel-CSRF must succeed (200), got %d: %s", recAlt.Code, recAlt.Body.String())
	}

	// 9.6a Missing Origin + X-Sentinel-Origin supplied must be REJECTED (403 CSRF_MISSING_ORIGIN)
	reqXOrigin := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqXOrigin.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqXOrigin.Header.Set("X-Sentinel-Origin", "http://localhost:5173")
	reqXOrigin.Header.Set("X-CSRF-Token", csrfToken)
	recXOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recXOrigin, reqXOrigin)
	if recXOrigin.Code != http.StatusForbidden || !strings.Contains(recXOrigin.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Errorf("Missing Origin with X-Sentinel-Origin supplied must return 403 CSRF_MISSING_ORIGIN, got %d: %s", recXOrigin.Code, recXOrigin.Body.String())
	}

	// 9.6b Missing Origin + Referer supplied must be REJECTED (403 CSRF_MISSING_ORIGIN)
	reqReferer := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqReferer.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	reqReferer.Header.Set("Referer", "http://localhost:5173/dashboard")
	reqReferer.Header.Set("X-CSRF-Token", csrfToken)
	recReferer := httptest.NewRecorder()
	handler.ServeHTTP(recReferer, reqReferer)
	if recReferer.Code != http.StatusForbidden || !strings.Contains(recReferer.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Errorf("Missing Origin with Referer supplied must return 403 CSRF_MISSING_ORIGIN, got %d: %s", recReferer.Code, recReferer.Body.String())
	}

	// 9.7 Comprehensive Method Matrix: POST, PUT, PATCH, DELETE require Origin and CSRF
	stateMethods := []struct {
		method string
		target string
	}{
		{"POST", "/api/v1/alarms/rules"},
		{"PUT", "/api/v1/devices/cam-csrf-device"},
		{"PATCH", "/api/v1/devices/cam-csrf-device"},
		{"DELETE", "/api/v1/alarms/rules?id=nonexistent"},
	}

	for _, sm := range stateMethods {
		// Missing Origin must be rejected
		mReq := httptest.NewRequest(sm.method, sm.target, strings.NewReader(`{}`))
		mReq.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
		mReq.Header.Set("X-CSRF-Token", csrfToken)
		mRec := httptest.NewRecorder()
		handler.ServeHTTP(mRec, mReq)
		if mRec.Code != http.StatusForbidden || !strings.Contains(mRec.Body.String(), "CSRF_MISSING_ORIGIN") {
			t.Errorf("Method %s without Origin must return 403 CSRF_MISSING_ORIGIN, got %d: %s", sm.method, mRec.Code, mRec.Body.String())
		}

		// Untrusted Origin must be rejected
		uReq := httptest.NewRequest(sm.method, sm.target, strings.NewReader(`{}`))
		uReq.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
		uReq.Header.Set("Origin", "http://evil-attacker.com")
		uReq.Header.Set("X-CSRF-Token", csrfToken)
		uRec := httptest.NewRecorder()
		handler.ServeHTTP(uRec, uReq)
		if uRec.Code != http.StatusForbidden || !strings.Contains(uRec.Body.String(), "CSRF_UNTRUSTED_ORIGIN") {
			t.Errorf("Method %s with untrusted Origin must return 403 CSRF_UNTRUSTED_ORIGIN, got %d: %s", sm.method, uRec.Code, uRec.Body.String())
		}
	}

	// 9.8 Native Desktop Client via Bearer token (no cookie) does not require CSRF or Origin
	reqDesktop := httptest.NewRequest("POST", "/api/v1/alarms/rules", strings.NewReader(bodyRule))
	reqDesktop.Header.Set("Authorization", "Bearer "+token)
	reqDesktop.Header.Set("X-Client-Type", "desktop")
	recDesktop := httptest.NewRecorder()
	handler.ServeHTTP(recDesktop, reqDesktop)

	if recDesktop.Code != http.StatusOK {
		t.Errorf("Desktop client with Bearer token must succeed without CSRF header, got %d: %s", recDesktop.Code, recDesktop.Body.String())
	}

	// 9.9 Cookie-authenticated Logout enforces Origin check
	// Missing Origin on cookie logout
	logoutNoOrigin := httptest.NewRequest("POST", "/api/v1/security/logout", nil)
	logoutNoOrigin.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	recLogoutNoOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recLogoutNoOrigin, logoutNoOrigin)
	if recLogoutNoOrigin.Code != http.StatusForbidden || !strings.Contains(recLogoutNoOrigin.Body.String(), "CSRF_MISSING_ORIGIN") {
		t.Errorf("Logout without Origin must return 403 CSRF_MISSING_ORIGIN, got %d: %s", recLogoutNoOrigin.Code, recLogoutNoOrigin.Body.String())
	}

	// Untrusted Origin on cookie logout
	logoutBadOrigin := httptest.NewRequest("POST", "/api/v1/security/logout", nil)
	logoutBadOrigin.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	logoutBadOrigin.Header.Set("Origin", "http://malicious.org")
	recLogoutBadOrigin := httptest.NewRecorder()
	handler.ServeHTTP(recLogoutBadOrigin, logoutBadOrigin)
	if recLogoutBadOrigin.Code != http.StatusForbidden || !strings.Contains(recLogoutBadOrigin.Body.String(), "CSRF_UNTRUSTED_ORIGIN") {
		t.Errorf("Logout with untrusted Origin must return 403 CSRF_UNTRUSTED_ORIGIN, got %d: %s", recLogoutBadOrigin.Code, recLogoutBadOrigin.Body.String())
	}

	// Trusted Origin on cookie logout succeeds and clears cookies
	logoutValid := httptest.NewRequest("POST", "/api/v1/security/logout", nil)
	logoutValid.AddCookie(&http.Cookie{Name: "sentinel_session", Value: token})
	logoutValid.Header.Set("Origin", "http://localhost:5173")
	recLogoutValid := httptest.NewRecorder()
	handler.ServeHTTP(recLogoutValid, logoutValid)
	if recLogoutValid.Code != http.StatusOK {
		t.Errorf("Logout with trusted Origin must return 200 OK, got %d: %s", recLogoutValid.Code, recLogoutValid.Body.String())
	}
	// Check that Set-Cookie headers clear the session
	cookies := recLogoutValid.Result().Cookies()
	sessionCleared := false
	for _, c := range cookies {
		if c.Name == "sentinel_session" && (c.MaxAge < 0 || c.Value == "") {
			sessionCleared = true
			break
		}
	}
	if !sessionCleared {
		t.Errorf("Expected sentinel_session cookie to be cleared upon logout, cookies: %+v", cookies)
	}
}

// 10. Multi-Tenant Telemetry Stream and Event Isolation Suite
func TestAuthMatrix_StreamTenantIsolation(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	// Seed devices across tenants
	store.UpsertDevice(models.Device{
		ID:           "cam-telemetry-alpha",
		Name:         "Alpha Camera",
		IP:           "192.168.10.50",
		Manufacturer: models.VendorHikvision,
		TenantID:     "tenant-alpha",
		SiteID:       "site-alpha",
	})
	store.UpsertDevice(models.Device{
		ID:           "cam-telemetry-beta",
		Name:         "Beta Camera",
		IP:           "192.168.20.50",
		Manufacturer: models.VendorDahua,
		TenantID:     "tenant-beta",
		SiteID:       "site-beta",
	})
	store.UpsertDevice(models.Device{
		ID:           "cam-telemetry-unassigned",
		Name:         "Unassigned Camera",
		IP:           "192.168.30.50",
		Manufacturer: models.VendorCPPlus,
		TenantID:     "",
		SiteID:       "",
	})

	tokenAlpha := srv.IssueSessionForTest(security.RoleAdmin, "tenant-alpha", "site-alpha")
	tokenBeta := srv.IssueSessionForTest(security.RoleAdmin, "tenant-beta", "site-beta")
	tokenGlobal := srv.IssueGlobalAdminSessionForTest()

	// Ingest Events & Alarms into both event manager and alarm engine
	evAlpha := events.SurveillanceEvent{
		ID:           "ev-alpha-001",
		DeviceID:     "cam-telemetry-alpha",
		DeviceIP:     "192.168.10.50",
		Manufacturer: "Hikvision",
		EventType:    "MotionDetection",
		Source:       "Channel 1",
		Timestamp:    time.Now().UTC(),
		ReceivedAt:   time.Now().UTC(),
		IsActive:     true,
	}
	evBeta := events.SurveillanceEvent{
		ID:           "ev-beta-001",
		DeviceID:     "cam-telemetry-beta",
		DeviceIP:     "192.168.20.50",
		Manufacturer: "Dahua",
		EventType:    "LineCrossing",
		Source:       "Channel 1",
		Timestamp:    time.Now().UTC(),
		ReceivedAt:   time.Now().UTC(),
		IsActive:     true,
	}
	evUnassigned := events.SurveillanceEvent{
		ID:           "ev-unassigned-001",
		DeviceID:     "cam-telemetry-unassigned",
		DeviceIP:     "192.168.30.50",
		Manufacturer: "CP PLUS",
		EventType:    "Intrusion",
		Source:       "Channel 1",
		Timestamp:    time.Now().UTC(),
		ReceivedAt:   time.Now().UTC(),
		IsActive:     true,
	}

	srv.GetEventManager().IngestEvent(evAlpha)
	srv.GetEventManager().IngestEvent(evBeta)
	srv.GetEventManager().IngestEvent(evUnassigned)

	srv.GetAlarmEngine().ProcessEvent(evAlpha)
	srv.GetAlarmEngine().ProcessEvent(evBeta)
	srv.GetAlarmEngine().ProcessEvent(evUnassigned)

	// 10.1 Live Events endpoint (/api/v1/events/live) isolation:
	// Tenant Alpha must only see Alpha events
	reqAlphaEv := httptest.NewRequest("GET", "/api/v1/events/live", nil)
	reqAlphaEv.Header.Set("Authorization", "Bearer "+tokenAlpha)
	recAlphaEv := httptest.NewRecorder()
	handler.ServeHTTP(recAlphaEv, reqAlphaEv)

	if recAlphaEv.Code != http.StatusOK {
		t.Fatalf("Tenant Alpha events request failed: %d", recAlphaEv.Code)
	}
	var evsAlpha []events.SurveillanceEvent
	_ = json.NewDecoder(recAlphaEv.Body).Decode(&evsAlpha)
	for _, ev := range evsAlpha {
		if ev.DeviceID != "cam-telemetry-alpha" {
			t.Errorf("Tenant Alpha received cross-tenant event: %+v", ev)
		}
	}

	// Tenant Beta must only see Beta events
	reqBetaEv := httptest.NewRequest("GET", "/api/v1/events/live", nil)
	reqBetaEv.Header.Set("Authorization", "Bearer "+tokenBeta)
	recBetaEv := httptest.NewRecorder()
	handler.ServeHTTP(recBetaEv, reqBetaEv)

	if recBetaEv.Code != http.StatusOK {
		t.Fatalf("Tenant Beta events request failed: %d", recBetaEv.Code)
	}
	var evsBeta []events.SurveillanceEvent
	_ = json.NewDecoder(recBetaEv.Body).Decode(&evsBeta)
	for _, ev := range evsBeta {
		if ev.DeviceID != "cam-telemetry-beta" {
			t.Errorf("Tenant Beta received cross-tenant event: %+v", ev)
		}
	}

	// Global Super-Admin sees all events including unassigned
	reqGlobalEv := httptest.NewRequest("GET", "/api/v1/events/live", nil)
	reqGlobalEv.Header.Set("Authorization", "Bearer "+tokenGlobal)
	recGlobalEv := httptest.NewRecorder()
	handler.ServeHTTP(recGlobalEv, reqGlobalEv)

	if recGlobalEv.Code != http.StatusOK {
		t.Fatalf("Global Admin events request failed: %d", recGlobalEv.Code)
	}
	var evsGlobal []events.SurveillanceEvent
	_ = json.NewDecoder(recGlobalEv.Body).Decode(&evsGlobal)
	if len(evsGlobal) < 3 {
		t.Errorf("Global Admin must see all 3 events, got %d", len(evsGlobal))
	}

	// 10.2 Alarms endpoint (/api/v1/alarms) isolation:
	// Tenant Alpha only receives Alpha alarms
	reqAlphaAlm := httptest.NewRequest("GET", "/api/v1/alarms", nil)
	reqAlphaAlm.Header.Set("Authorization", "Bearer "+tokenAlpha)
	recAlphaAlm := httptest.NewRecorder()
	handler.ServeHTTP(recAlphaAlm, reqAlphaAlm)

	if recAlphaAlm.Code != http.StatusOK {
		t.Fatalf("Tenant Alpha alarms request failed: %d", recAlphaAlm.Code)
	}
	var almsAlpha []alarms.Alarm
	_ = json.NewDecoder(recAlphaAlm.Body).Decode(&almsAlpha)
	for _, alm := range almsAlpha {
		if alm.DeviceID != "cam-telemetry-alpha" {
			t.Errorf("Tenant Alpha received cross-tenant alarm: %+v", alm)
		}
	}

	// Tenant Beta only receives Beta alarms
	reqBetaAlm := httptest.NewRequest("GET", "/api/v1/alarms", nil)
	reqBetaAlm.Header.Set("Authorization", "Bearer "+tokenBeta)
	recBetaAlm := httptest.NewRecorder()
	handler.ServeHTTP(recBetaAlm, reqBetaAlm)

	if recBetaAlm.Code != http.StatusOK {
		t.Fatalf("Tenant Beta alarms request failed: %d", recBetaAlm.Code)
	}
	var almsBeta []alarms.Alarm
	_ = json.NewDecoder(recBetaAlm.Body).Decode(&almsBeta)
	for _, alm := range almsBeta {
		if alm.DeviceID != "cam-telemetry-beta" {
			t.Errorf("Tenant Beta received cross-tenant alarm: %+v", alm)
		}
	}

	// Global Admin receives all alarms
	reqGlobalAlm := httptest.NewRequest("GET", "/api/v1/alarms", nil)
	reqGlobalAlm.Header.Set("Authorization", "Bearer "+tokenGlobal)
	recGlobalAlm := httptest.NewRecorder()
	handler.ServeHTTP(recGlobalAlm, reqGlobalAlm)

	if recGlobalAlm.Code != http.StatusOK {
		t.Fatalf("Global Admin alarms request failed: %d", recGlobalAlm.Code)
	}
	var almsGlobal []alarms.Alarm
	_ = json.NewDecoder(recGlobalAlm.Body).Decode(&almsGlobal)
	if len(almsGlobal) < 3 {
		t.Errorf("Global Admin must see all 3 alarms, got %d", len(almsGlobal))
	}
}

// 11. Recorder Subroutes and Channel Aliases Authorization Matrix Audit
func TestAuthMatrix_RecorderSubroutesAndAliasesAudit(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	recorderID := "nvr-audit-matrix-01"
	store.UpsertDevice(models.Device{
		ID:           recorderID,
		Name:         "Matrix Audit XVR",
		IP:           "192.168.1.150",
		Manufacturer: models.VendorDahua,
		Category:     models.CategoryXVR,
		ChannelCount: 8,
		TenantID:     "default",
		SiteID:       "hq",
	})

	viewerToken := srv.IssueSessionForTest(security.RoleViewer, "default", "hq")
	auditorToken := srv.IssueSessionForTest(security.RoleAuditor, "default", "hq")
	techToken := srv.IssueSessionForTest(security.RoleTechnician, "default", "hq")
	adminToken := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")

	// 11.1 Channel alias GET /api/v1/recorders/:id/channels/:ch
	reqCh1 := httptest.NewRequest("GET", "/api/v1/recorders/"+recorderID+"/channels/1", nil)
	reqCh1.Header.Set("Authorization", "Bearer "+viewerToken)
	recCh1 := httptest.NewRecorder()
	handler.ServeHTTP(recCh1, reqCh1)

	if recCh1.Code != http.StatusOK {
		t.Errorf("Channel alias GET /channels/1 failed: %d: %s", recCh1.Code, recCh1.Body.String())
	}
	var chInfo recorder.ChannelInfo
	_ = json.NewDecoder(recCh1.Body).Decode(&chInfo)
	if chInfo.ChannelNumber != 1 {
		t.Errorf("Expected ChannelNumber 1, got %d", chInfo.ChannelNumber)
	}

	// Out of bounds channel alias returns 404
	reqCh99 := httptest.NewRequest("GET", "/api/v1/recorders/"+recorderID+"/channels/99", nil)
	reqCh99.Header.Set("Authorization", "Bearer "+viewerToken)
	recCh99 := httptest.NewRecorder()
	handler.ServeHTTP(recCh99, reqCh99)

	if recCh99.Code != http.StatusNotFound {
		t.Errorf("Non-existent channel must return 404, got %d", recCh99.Code)
	}

	// 11.2 Analog format: POST /api/v1/recorders/:id/analog-format
	bodyAnalog := `{"channel_id": 1, "format": "AHD"}`
	// Viewer -> 403 Forbidden
	reqVAnalog := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/analog-format", strings.NewReader(bodyAnalog))
	reqVAnalog.Header.Set("Authorization", "Bearer "+viewerToken)
	recVAnalog := httptest.NewRecorder()
	handler.ServeHTTP(recVAnalog, reqVAnalog)
	if recVAnalog.Code != http.StatusForbidden {
		t.Errorf("Viewer must be forbidden from configuring analog format, got %d", recVAnalog.Code)
	}
	// Auditor -> 403 Forbidden
	reqAAnalog := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/analog-format", strings.NewReader(bodyAnalog))
	reqAAnalog.Header.Set("Authorization", "Bearer "+auditorToken)
	recAAnalog := httptest.NewRecorder()
	handler.ServeHTTP(recAAnalog, reqAAnalog)
	if recAAnalog.Code != http.StatusForbidden {
		t.Errorf("Auditor must be forbidden from configuring analog format, got %d", recAAnalog.Code)
	}
	// Technician -> 200 OK
	reqTAnalog := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/analog-format", strings.NewReader(bodyAnalog))
	reqTAnalog.Header.Set("Authorization", "Bearer "+techToken)
	recTAnalog := httptest.NewRecorder()
	handler.ServeHTTP(recTAnalog, reqTAnalog)
	if recTAnalog.Code != http.StatusOK {
		t.Errorf("Technician must be permitted to configure analog format, got %d: %s", recTAnalog.Code, recTAnalog.Body.String())
	}

	// 11.3 Coaxial control: POST /api/v1/recorders/:id/coaxial-control
	bodyCoax := `{"channel_id": 1, "command": "MENU_ENTER"}`
	reqVCoax := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/coaxial-control", strings.NewReader(bodyCoax))
	reqVCoax.Header.Set("Authorization", "Bearer "+viewerToken)
	recVCoax := httptest.NewRecorder()
	handler.ServeHTTP(recVCoax, reqVCoax)
	if recVCoax.Code != http.StatusForbidden {
		t.Errorf("Viewer must be forbidden from coaxial control, got %d", recVCoax.Code)
	}
	reqTCoax := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/coaxial-control", strings.NewReader(bodyCoax))
	reqTCoax.Header.Set("Authorization", "Bearer "+techToken)
	recTCoax := httptest.NewRecorder()
	handler.ServeHTTP(recTCoax, reqTCoax)
	if recTCoax.Code != http.StatusOK {
		t.Errorf("Technician must be permitted for coaxial control, got %d: %s", recTCoax.Code, recTCoax.Body.String())
	}

	// 11.4 IP Registration: POST /api/v1/recorders/:id/ip-registration
	bodyIPReg := `{"channel_id": 9, "ip": "192.168.1.200", "port": 80, "protocol": "ONVIF"}`
	reqVIP := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/ip-registration", strings.NewReader(bodyIPReg))
	reqVIP.Header.Set("Authorization", "Bearer "+viewerToken)
	recVIP := httptest.NewRecorder()
	handler.ServeHTTP(recVIP, reqVIP)
	if recVIP.Code != http.StatusForbidden {
		t.Errorf("Viewer must be forbidden from IP registration, got %d", recVIP.Code)
	}
	reqTIP := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/ip-registration", strings.NewReader(bodyIPReg))
	reqTIP.Header.Set("Authorization", "Bearer "+techToken)
	recTIP := httptest.NewRecorder()
	handler.ServeHTTP(recTIP, reqTIP)
	if recTIP.Code != http.StatusOK {
		t.Errorf("Technician must be permitted for IP registration, got %d: %s", recTIP.Code, recTIP.Body.String())
	}

	// 11.5 Recording Schedule: GET allowed for Viewer, POST forbidden for Viewer
	reqGetSched := httptest.NewRequest("GET", "/api/v1/recorders/"+recorderID+"/schedule", nil)
	reqGetSched.Header.Set("Authorization", "Bearer "+viewerToken)
	recGetSched := httptest.NewRecorder()
	handler.ServeHTTP(recGetSched, reqGetSched)
	if recGetSched.Code != http.StatusOK {
		t.Errorf("Viewer must be allowed to read schedule, got %d", recGetSched.Code)
	}

	bodySched := `{"channel_id": 1, "weekday": 1, "recording_type": "CONTINUOUS"}`
	reqVSched := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/schedule", strings.NewReader(bodySched))
	reqVSched.Header.Set("Authorization", "Bearer "+viewerToken)
	recVSched := httptest.NewRecorder()
	handler.ServeHTTP(recVSched, reqVSched)
	if recVSched.Code != http.StatusForbidden {
		t.Errorf("Viewer must be forbidden from updating schedule, got %d", recVSched.Code)
	}

	reqTSched := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/schedule", strings.NewReader(bodySched))
	reqTSched.Header.Set("Authorization", "Bearer "+techToken)
	recTSched := httptest.NewRecorder()
	handler.ServeHTTP(recTSched, reqTSched)
	if recTSched.Code != http.StatusOK {
		t.Errorf("Technician must be permitted to update schedule, got %d", recTSched.Code)
	}

	// 11.6 Storage Format: Preview & Format require technician or admin
	bodyStoragePreview := `{"disk_index": 0}`
	reqVStorePrev := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/storage/preview", strings.NewReader(bodyStoragePreview))
	reqVStorePrev.Header.Set("Authorization", "Bearer "+viewerToken)
	recVStorePrev := httptest.NewRecorder()
	handler.ServeHTTP(recVStorePrev, reqVStorePrev)
	if recVStorePrev.Code != http.StatusForbidden {
		t.Errorf("Viewer must be forbidden from storage format preview, got %d", recVStorePrev.Code)
	}

	bodyStorageFmt := `{"disk_index": 0, "confirm_token": "FMT-TEST-CONFIRM"}`
	reqVStoreFmt := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/storage/format", strings.NewReader(bodyStorageFmt))
	reqVStoreFmt.Header.Set("Authorization", "Bearer "+viewerToken)
	recVStoreFmt := httptest.NewRecorder()
	handler.ServeHTTP(recVStoreFmt, reqVStoreFmt)
	if recVStoreFmt.Code != http.StatusForbidden {
		t.Errorf("Viewer must be forbidden from storage formatting, got %d", recVStoreFmt.Code)
	}

	reqAdminStoreFmt := httptest.NewRequest("POST", "/api/v1/recorders/"+recorderID+"/storage/format", strings.NewReader(bodyStorageFmt))
	reqAdminStoreFmt.Header.Set("Authorization", "Bearer "+adminToken)
	recAdminStoreFmt := httptest.NewRecorder()
	handler.ServeHTTP(recAdminStoreFmt, reqAdminStoreFmt)
	if recAdminStoreFmt.Code == http.StatusForbidden {
		t.Errorf("Admin must not be forbidden from storage formatting, got %d: %s", recAdminStoreFmt.Code, recAdminStoreFmt.Body.String())
	}
}

// 12. Deception Remediation: Playback Frame Extraction Returns HTTP 501
func TestAuthMatrix_PlaybackNotImplemented(t *testing.T) {
	srv, handler, store, _ := setupFullMatrixTestServer(t)

	recorderID := "nvr-playback-501"
	store.UpsertDevice(models.Device{
		ID:           recorderID,
		Name:         "Playback Test NVR",
		IP:           "192.168.1.180",
		Manufacturer: models.VendorHikvision,
		Category:     models.CategoryNVR,
		TenantID:     "default",
		SiteID:       "hq",
	})

	adminToken := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")

	reqPB := httptest.NewRequest("GET", "/api/v1/recorders/"+recorderID+"/playback/dummy-session-token/stream.mjpeg", nil)
	reqPB.Header.Set("Authorization", "Bearer "+adminToken)
	recPB := httptest.NewRecorder()
	handler.ServeHTTP(recPB, reqPB)

	if recPB.Code != http.StatusNotImplemented {
		t.Fatalf("Expected HTTP 501 Not Implemented for unintegrated playback stream, got %d: %s", recPB.Code, recPB.Body.String())
	}

	bodyStr := recPB.Body.String()
	if !strings.Contains(bodyStr, "PLAYBACK_NOT_IMPLEMENTED") {
		t.Errorf("Expected PLAYBACK_NOT_IMPLEMENTED error code in response body, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "physical NVR storage") {
		t.Errorf("Expected explicit disclaimer in body about physical NVR storage hardware, got %s", bodyStr)
	}
}
