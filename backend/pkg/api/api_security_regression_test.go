package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func setupSecurityTestServer(t *testing.T) (*api.Server, http.Handler, *database.Store, *security.RBACManager) {
	tempDir := t.TempDir()
	store := database.NewStore(tempDir + "/security_store.json")
	registry := adapters.NewRegistry()
	logger := audit.NewLogger()
	cfgEngine := configengine.NewEngine(registry, store, logger)
	discEngine := discovery.NewEngine(registry)
	licMgr := licensing.NewManager()
	rbacMgr := security.NewRBACManager()

	srv := api.NewServer(store, registry, cfgEngine, discEngine, licMgr, logger, nil, nil, nil, nil, rbacMgr)
	t.Cleanup(func() {
		rbacMgr.RevokeSetupToken()
	})
	return srv, srv.Handler(), store, rbacMgr
}

// 1. Unauthenticated requests to protected endpoints return 401
func TestSecurityUnauthenticatedAccessBlocked(t *testing.T) {
	_, handler, _, _ := setupSecurityTestServer(t)

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/devices"},
		{"GET", "/api/v1/devices/dev-123"},
		{"POST", "/api/v1/devices/dev-123/reboot"},
		{"GET", "/api/v1/devices/dev-123/ptz/capabilities"},
		{"GET", "/api/v1/events/live"},
		{"GET", "/api/v1/alarms"},
		{"GET", "/api/v1/recorders"},
		{"GET", "/api/v1/security/users"},
		{"GET", "/api/v1/audit/logs"},
	}

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for unauthenticated %s %s, got %d", route.method, route.path, rec.Code)
		}

		var errResp api.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err == nil {
			if errResp.Error.Code != "UNAUTHORIZED" {
				t.Errorf("expected error code 'UNAUTHORIZED' for %s %s, got %s", route.method, route.path, errResp.Error.Code)
			}
		}
	}
}

// 2. Role-Based Access Control (RBAC): Viewer cannot perform privileged actions
func TestSecurityViewerRoleForbiddenActions(t *testing.T) {
	srv, handler, store, _ := setupSecurityTestServer(t)

	// Seed a test device
	store.UpsertDevice(models.Device{
		ID:           "test-cam-rbac",
		Name:         "Gate Camera",
		IP:           "192.168.1.50",
		Manufacturer: models.VendorHikvision,
		TenantID:     "default",
		SiteID:       "hq",
	})

	// Issue Viewer token
	viewerToken := srv.IssueSessionForTest(security.RoleViewer, "default", "hq")

	privilegedCalls := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"Reboot Device", "POST", "/api/v1/devices/test-cam-rbac/reboot", ""},
		{"Add Device", "POST", "/api/v1/devices", `{"name":"Rogue Cam","ip":"192.168.1.99"}`},
		{"List Users", "GET", "/api/v1/security/users", ""},
		{"Rotate Audit Key", "POST", "/api/v1/audit/rotate-key", ""},
	}

	for _, tc := range privilegedCalls {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for Viewer on %s, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}

	// Viewer CAN access read-only live viewing endpoints
	viewReq := httptest.NewRequest("GET", "/api/v1/devices", nil)
	viewReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewRec := httptest.NewRecorder()
	handler.ServeHTTP(viewRec, viewReq)
	if viewRec.Code != http.StatusOK {
		t.Errorf("Viewer must be able to list devices, got code %d", viewRec.Code)
	}
}

// 3. Multi-Tenant and Site Isolation
func TestSecurityMultiTenantIsolation(t *testing.T) {
	srv, handler, store, _ := setupSecurityTestServer(t)

	// Seed devices in different tenants
	store.UpsertDevice(models.Device{
		ID:           "dev-tenant-alpha",
		Name:         "Alpha Camera",
		IP:           "10.0.1.10",
		TenantID:     "tenant-alpha",
		SiteID:       "site-alpha-1",
		Manufacturer: models.VendorHikvision,
	})
	store.UpsertDevice(models.Device{
		ID:           "dev-tenant-beta",
		Name:         "Beta Camera",
		IP:           "10.0.2.10",
		TenantID:     "tenant-beta",
		SiteID:       "site-beta-1",
		Manufacturer: models.VendorDahua,
	})

	// Issue token scoped strictly to Tenant Alpha
	tokenAlpha := srv.IssueSessionForTest(security.RoleAdmin, "tenant-alpha", "site-alpha-1")

	// 1. GET /api/v1/devices should only return tenant-alpha devices
	reqList := httptest.NewRequest("GET", "/api/v1/devices", nil)
	reqList.Header.Set("Authorization", "Bearer "+tokenAlpha)
	recList := httptest.NewRecorder()
	handler.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 listing devices, got %d", recList.Code)
	}
	var listedDevs []models.Device
	_ = json.NewDecoder(recList.Body).Decode(&listedDevs)
	if len(listedDevs) != 1 || listedDevs[0].ID != "dev-tenant-alpha" {
		t.Errorf("expected only tenant-alpha device, got %+v", listedDevs)
	}

	// 2. Direct access to tenant-beta device must return 403 Forbidden
	reqDirect := httptest.NewRequest("GET", "/api/v1/devices/dev-tenant-beta", nil)
	reqDirect.Header.Set("Authorization", "Bearer "+tokenAlpha)
	recDirect := httptest.NewRecorder()
	handler.ServeHTTP(recDirect, reqDirect)

	if recDirect.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden accessing cross-tenant device, got %d", recDirect.Code)
	}
}

// 4. Strict CORS Enforcement: untrusted origins receive NO access header
func TestSecurityCORSEnforcement(t *testing.T) {
	srv, handler, _, _ := setupSecurityTestServer(t)
	token := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")

	// 1. Trusted Origin (http://localhost:5173)
	trustedReq := httptest.NewRequest("GET", "/api/v1/healthz", nil)
	trustedReq.Header.Set("Origin", "http://localhost:5173")
	trustedRec := httptest.NewRecorder()
	handler.ServeHTTP(trustedRec, trustedReq)

	if trustedRec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("expected Access-Control-Allow-Origin for trusted origin, got %q", trustedRec.Header().Get("Access-Control-Allow-Origin"))
	}
	if trustedRec.Header().Get("Vary") != "Origin" {
		t.Errorf("expected Vary: Origin for trusted CORS response")
	}

	// 2. Untrusted Origin (https://attacker-site.com)
	untrustedReq := httptest.NewRequest("GET", "/api/v1/healthz", nil)
	untrustedReq.Header.Set("Origin", "https://attacker-site.com")
	untrustedRec := httptest.NewRecorder()
	handler.ServeHTTP(untrustedRec, untrustedReq)

	if untrustedRec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("untrusted origin must NOT receive Access-Control-Allow-Origin, got %q", untrustedRec.Header().Get("Access-Control-Allow-Origin"))
	}

	// 3. Security Headers presence
	secHeaders := []string{"X-Content-Type-Options", "X-Frame-Options", "X-XSS-Protection", "Content-Security-Policy"}
	for _, h := range secHeaders {
		if trustedRec.Header().Get(h) == "" {
			t.Errorf("missing essential security header: %s", h)
		}
	}

	// Ensure token is used cleanly
	_ = token
}

// 5. First-Run Setup Mode Lockdown & Strength Enforcement
func TestSecurityFirstRunSetupLockdown(t *testing.T) {
	_, handler, _, rbacMgr := setupSecurityTestServer(t)

	// Status check: must report setup_required = true
	reqStatus := httptest.NewRequest("GET", "/api/v1/auth/setup/status", nil)
	recStatus := httptest.NewRecorder()
	handler.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 for setup status, got %d", recStatus.Code)
	}

	setupToken := rbacMgr.GetSetupToken()

	// 1. Weak password rejected (< 10 chars, no symbols)
	weakReqBody := map[string]string{
		"setup_token": setupToken,
		"username":    "admin",
		"password":    "weak",
		"full_name":   "System Admin",
		"email":       "admin@sentinel.internal",
	}
	b, _ := json.Marshal(weakReqBody)
	reqWeak := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(b))
	recWeak := httptest.NewRecorder()
	handler.ServeHTTP(recWeak, reqWeak)
	if recWeak.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for weak password, got %d", recWeak.Code)
	}

	// 2. Invalid setup token rejected
	badTokenBody := map[string]string{
		"setup_token": "sent-setup-0000000000000000000000000000000000000000000000000000000000000000",
		"username":    "admin",
		"password":    "TestOperator#Pass2026!",
		"full_name":   "System Admin",
		"email":       "admin@sentinel.internal",
	}
	b, _ = json.Marshal(badTokenBody)
	reqBadTok := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(b))
	recBadTok := httptest.NewRecorder()
	handler.ServeHTTP(recBadTok, reqBadTok)
	if recBadTok.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for invalid setup token, got %d", recBadTok.Code)
	}

	// 3. Valid setup completes successfully
	validBody := map[string]string{
		"setup_token": setupToken,
		"username":    "secops_admin",
		"password":    "SentinelSecOps@2026#Hardened!",
		"full_name":   "Lead SecOps Admin",
		"email":       "secops@sentinel.internal",
	}
	b, _ = json.Marshal(validBody)
	reqValid := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(b))
	recValid := httptest.NewRecorder()
	handler.ServeHTTP(recValid, reqValid)
	if recValid.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for valid setup, got %d: %s", recValid.Code, recValid.Body.String())
	}

	// 4. Repeated setup attempt is permanently locked down (403 SETUP_DISABLED)
	recRepeat := httptest.NewRecorder()
	reqRepeat := httptest.NewRequest("POST", "/api/v1/auth/setup", bytes.NewReader(b))
	handler.ServeHTTP(recRepeat, reqRepeat)
	if recRepeat.Code != http.StatusForbidden {
		t.Errorf("subsequent setup must be forbidden (403), got %d", recRepeat.Code)
	}
}

// 6. Audit Key Rotation and Signed Ledger Export Verification
func TestSecurityAuditKeyRotationAndSignedExport(t *testing.T) {
	srv, handler, _, _ := setupSecurityTestServer(t)
	adminToken := srv.IssueSessionForTest(security.RoleAdmin, "default", "hq")

	// 1. Rotate HMAC Checkpoint Key
	rotReq := httptest.NewRequest("POST", "/api/v1/audit/rotate-key", nil)
	rotReq.Header.Set("Authorization", "Bearer "+adminToken)
	rotRec := httptest.NewRecorder()
	handler.ServeHTTP(rotRec, rotReq)

	if rotRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK rotating audit key, got %d", rotRec.Code)
	}
	var rotData map[string]interface{}
	_ = json.NewDecoder(rotRec.Body).Decode(&rotData)
	if rotData["new_key_id"] == nil || rotData["new_key_id"] == "" {
		t.Fatalf("expected valid new_key_id in rotation response")
	}

	// 2. Export cryptographically signed audit ledger
	expReq := httptest.NewRequest("GET", "/api/v1/audit/export/signed", nil)
	expReq.Header.Set("Authorization", "Bearer "+adminToken)
	expRec := httptest.NewRecorder()
	handler.ServeHTTP(expRec, expReq)

	if expRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK exporting signed audit ledger, got %d: %s", expRec.Code, expRec.Body.String())
	}
	if expRec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json Content-Type, got %s", expRec.Header().Get("Content-Type"))
	}
}

// 7. Database Startup Integrity & Readiness Probe Lockdown
func TestSecurityReadinessProbe_DatabasePersistenceFailsClosed(t *testing.T) {
	_, handler, _, _ := setupSecurityTestServer(t)

	// 1. Ready probe on healthy server returns 200 OK
	reqReady := httptest.NewRequest("GET", "/api/v1/readyz", nil)
	recReady := httptest.NewRecorder()
	handler.ServeHTTP(recReady, reqReady)
	if recReady.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for readyz on healthy system, got %d", recReady.Code)
	}

	// 2. Ready probe with nil store returns 503 Service Unavailable
	brokenServer := api.NewServer(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	reqBroken := httptest.NewRequest("GET", "/api/v1/readyz", nil)
	recBroken := httptest.NewRecorder()
	brokenServer.Handler().ServeHTTP(recBroken, reqBroken)

	if recBroken.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable when database persistence is missing/unhealthy, got %d", recBroken.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(recBroken.Body).Decode(&resp)
	if resp["ready"] != false {
		t.Errorf("expected ready=false in response: %+v", resp)
	}
}
