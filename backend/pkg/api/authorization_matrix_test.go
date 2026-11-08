package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/api"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
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
	_ = rbacMgr.CompleteSetup(setupToken, "admin_user", "AdminSec@Sentinel2026!", "Admin User", "admin@sentinel.io")

	// Authenticate via /api/v1/security/login
	loginBody := `{"username":"admin_user","password":"AdminSec@Sentinel2026!"}`
	loginReq := httptest.NewRequest("POST", "/api/v1/security/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
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
	superAdminToken := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")

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
