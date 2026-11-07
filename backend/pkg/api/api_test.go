package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/security"
)

func setupTestServer(t *testing.T) (*Server, http.Handler) {
	tmpDir := t.TempDir()
	vault, err := security.NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	store := database.NewStoreWithVault("", vault)
	registry := adapters.NewRegistry()
	logger := audit.NewLogger()
	licenseMgr := licensing.NewManager()
	configEng := configengine.NewEngine(registry, store, logger)
	discoveryEng := discovery.NewEngine(registry)

	server := NewServer(store, registry, configEng, discoveryEng, licenseMgr, logger)
	return server, server.Handler()
}

func TestHealthzEndpoint(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse /healthz response: %v", err)
	}

	if resp["status"] != "UP" {
		t.Errorf("expected status 'UP', got %v", resp["status"])
	}

	reqID := rec.Header().Get("X-Request-ID")
	if reqID == "" || !strings.HasPrefix(reqID, "req-") {
		t.Errorf("expected valid X-Request-ID header, got %q", reqID)
	}
}

func TestReadyzEndpoint(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /readyz, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse /readyz response: %v", err)
	}

	if resp["ready"] != true {
		t.Errorf("expected ready true, got %v", resp["ready"])
	}

	checks, ok := resp["checks"].(map[string]interface{})
	if !ok || checks["database"] != "READY" || checks["adapters"] != "READY" {
		t.Errorf("readiness dependency checks failed: %v", checks)
	}
}

func TestRequestCorrelationPropagation(t *testing.T) {
	_, handler := setupTestServer(t)

	customID := "test-audit-req-12345678"
	req := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	req.Header.Set("X-Request-ID", customID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	resID := rec.Header().Get("X-Request-ID")
	if resID != customID {
		t.Errorf("expected propagated X-Request-ID %q, got %q", customID, resID)
	}
}

func TestStructuredErrorHandling(t *testing.T) {
	_, handler := setupTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/devices/non-existent-device-id-99", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse structured error: %v", err)
	}

	if errResp.Success != false {
		t.Errorf("expected success false in error response")
	}

	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected error code 'NOT_FOUND', got %q", errResp.Error.Code)
	}

	if errResp.Error.Message != "Device not found" {
		t.Errorf("expected user-friendly message 'Device not found', got %q", errResp.Error.Message)
	}

	if errResp.Error.RequestID == "" {
		t.Errorf("expected non-empty request_id in structured error payload")
	}

	// Security check: verify no stack trace, path, or internal leak in body
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "panic") || strings.Contains(bodyStr, "goroutine") {
		t.Errorf("internal runtime details leaked in error response: %s", bodyStr)
	}
}

func TestDiscoveryEndpoints(t *testing.T) {
	server, handler := setupTestServer(t)

	// 1. Test GET /api/v1/discovery/interfaces
	reqIf := httptest.NewRequest("GET", "/api/v1/discovery/interfaces", nil)
	recIf := httptest.NewRecorder()
	handler.ServeHTTP(recIf, reqIf)
	if recIf.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /discovery/interfaces, got %d", recIf.Code)
	}

	// 2. Test POST /api/v1/devices/discover
	body := strings.NewReader(`{"method":"TCP","cidr":"127.0.0.1/32","timeout_ms":50,"concurrency":4}`)
	reqDisc := httptest.NewRequest("POST", "/api/v1/devices/discover", body)
	recDisc := httptest.NewRecorder()
	handler.ServeHTTP(recDisc, reqDisc)

	if recDisc.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted from /devices/discover, got %d: %s", recDisc.Code, recDisc.Body.String())
	}

	var discResp map[string]interface{}
	if err := json.Unmarshal(recDisc.Body.Bytes(), &discResp); err != nil {
		t.Fatalf("failed to decode discover response: %v", err)
	}
	jobID, ok := discResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("missing or invalid job_id in response: %v", discResp)
	}

	// 3. Test GET /api/v1/discovery/{id}
	reqJob := httptest.NewRequest("GET", "/api/v1/discovery/"+jobID, nil)
	recJob := httptest.NewRecorder()
	handler.ServeHTTP(recJob, reqJob)
	if recJob.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /discovery/%s, got %d", jobID, recJob.Code)
	}

	// 4. Test GET /api/v1/discovery/{id}/results
	reqRes := httptest.NewRequest("GET", "/api/v1/discovery/"+jobID+"/results", nil)
	recRes := httptest.NewRecorder()
	handler.ServeHTTP(recRes, reqRes)
	if recRes.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /discovery/%s/results, got %d", jobID, recRes.Code)
	}

	// 5. Test POST /api/v1/discovery/{id}/cancel
	reqCancel := httptest.NewRequest("POST", "/api/v1/discovery/"+jobID+"/cancel", nil)
	recCancel := httptest.NewRecorder()
	handler.ServeHTTP(recCancel, reqCancel)
	if recCancel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /discovery/%s/cancel, got %d", jobID, recCancel.Code)
	}

	// 6. Test GET /api/v1/devices/{id}/identity and /services on an enrolled device
	devList := server.store.ListDevices()
	if len(devList) > 0 {
		testDev := devList[0]

		// Identity endpoint
		reqIden := httptest.NewRequest("GET", "/api/v1/devices/"+testDev.ID+"/identity", nil)
		recIden := httptest.NewRecorder()
		handler.ServeHTTP(recIden, reqIden)
		if recIden.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /devices/%s/identity, got %d", testDev.ID, recIden.Code)
		}

		var idenResp map[string]interface{}
		_ = json.Unmarshal(recIden.Body.Bytes(), &idenResp)
		if idenResp["confidence"] == nil {
			t.Errorf("expected confidence score in identity response")
		}

		// Services endpoint
		reqSvc := httptest.NewRequest("GET", "/api/v1/devices/"+testDev.ID+"/services", nil)
		recSvc := httptest.NewRecorder()
		handler.ServeHTTP(recSvc, reqSvc)
		if recSvc.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /devices/%s/services, got %d", testDev.ID, recSvc.Code)
		}
	}
}


