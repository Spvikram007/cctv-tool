package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/hikvision"
	"cctv-sentinel/pkg/api"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/security"
	"cctv-sentinel/pkg/simulator"
)

func setupTestAPIServer(t *testing.T) (*httptest.Server, *database.Store, *simulator.MultiDeviceSimulator, string) {
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		t.Fatalf("failed to start simulator: %v", err)
	}

	store := database.NewStore("")
	registry := adapters.NewRegistry()
	hikAdapter := hikvision.NewHikvisionAdapter()
	registry.Register(hikAdapter, models.ProtocolISAPI)

	logger := audit.NewLogger()
	cfgEngine := configengine.NewEngine(registry, store, logger)
	discEngine := discovery.NewEngine(registry)
	licMgr := licensing.NewManager()

	apiServer := api.NewServer(store, registry, cfgEngine, discEngine, licMgr, logger, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(apiServer.Handler())
	token := apiServer.IssueSessionForTest(security.RoleAdmin, "default", "hq")

	return ts, store, sim, token
}

func TestDeviceAuthAndCapabilitiesAPI(t *testing.T) {
	ts, store, sim, token := setupTestAPIServer(t)
	defer ts.Close()
	defer sim.Stop()

	authDo := func(method, urlStr string, body []byte) (*http.Response, error) {
		var req *http.Request
		var err error
		if len(body) > 0 {
			req, err = http.NewRequest(method, urlStr, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest(method, urlStr, nil)
		}
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return http.DefaultClient.Do(req)
	}

	// Seed test Hikvision device
	dev := models.Device{
		ID:           "test-hik-dev",
		Name:         "Hikvision AcuSense",
		IP:           "127.0.0.1",
		Port:         18081,
		Manufacturer: models.VendorHikvision,
		Category:     models.CategoryIPCamera,
		Status:       "discovered",
		Username:     "admin",
		Password:     "Admin@123",
		TenantID:     "default",
		SiteID:       "hq",
	}
	store.UpsertDevice(dev)

	// 1. Test POST /api/v1/devices/{id}/auth
	authPayload := []byte(`{"username":"admin","password":"Admin@123"}`)
	resp, err := authDo("POST", ts.URL+"/api/v1/devices/test-hik-dev/auth", authPayload)
	if err != nil {
		t.Fatalf("auth request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var authRes models.AuthStatus
	if err := json.NewDecoder(resp.Body).Decode(&authRes); err != nil {
		t.Fatalf("failed to decode auth response: %v", err)
	}
	if !authRes.Authenticated {
		t.Errorf("expected authenticated=true, error: %s", authRes.ErrorMessage)
	}

	// 2. Test GET /api/v1/devices/{id}/capabilities
	respCap, err := authDo("GET", ts.URL+"/api/v1/devices/test-hik-dev/capabilities", nil)
	if err != nil {
		t.Fatalf("capabilities request failed: %v", err)
	}
	defer respCap.Body.Close()

	if respCap.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respCap.StatusCode)
	}

	var caps models.DeviceCapabilities
	if err := json.NewDecoder(respCap.Body).Decode(&caps); err != nil {
		t.Fatalf("failed to decode capabilities response: %v", err)
	}
	if caps.DeviceManagement != models.CapabilitySupported {
		t.Errorf("expected DeviceManagement supported")
	}
	if caps.Media != models.CapabilitySupported {
		t.Errorf("expected Media supported")
	}

	// 3. Test POST /api/v1/devices/{id}/config/preview
	previewPayload := []byte(`{
		"type": "NETWORK",
		"config": {
			"dhcp": false,
			"ip_address": "192.168.1.200",
			"subnet_mask": "255.255.255.0",
			"gateway": "192.168.1.1"
		}
	}`)
	respPrv, err := authDo("POST", ts.URL+"/api/v1/devices/test-hik-dev/config/preview", previewPayload)
	if err != nil {
		t.Fatalf("config preview request failed: %v", err)
	}
	defer respPrv.Body.Close()

	var previewRes configengine.ConfigPreview
	if err := json.NewDecoder(respPrv.Body).Decode(&previewRes); err != nil {
		t.Fatalf("failed to decode preview response: %v", err)
	}
	if !previewRes.Valid {
		t.Fatalf("expected preview to be valid, errors: %v", previewRes.ValidationErrors)
	}
	if previewRes.PreviewToken == "" {
		t.Fatalf("expected non-empty preview token")
	}

	// 4. Test POST /api/v1/devices/{id}/config/apply with preview token
	applyPayload, _ := json.Marshal(map[string]interface{}{
		"type":          "NETWORK",
		"preview_token": previewRes.PreviewToken,
		"rollback_key":  previewRes.RollbackKey,
	})
	respApply, err := authDo("POST", ts.URL+"/api/v1/devices/test-hik-dev/config/apply", applyPayload)
	if err != nil {
		t.Fatalf("config apply request failed: %v", err)
	}
	defer respApply.Body.Close()

	if respApply.StatusCode != http.StatusOK {
		t.Fatalf("expected apply status 200, got %d", respApply.StatusCode)
	}

	var applyRes configengine.ConfigResult
	if err := json.NewDecoder(respApply.Body).Decode(&applyRes); err != nil {
		t.Fatalf("failed to decode apply result: %v", err)
	}
	if !applyRes.Success {
		t.Errorf("expected apply success=true")
	}
	if !applyRes.RollbackAvailable {
		t.Errorf("expected rollback to be available")
	}

	// 5. Test POST /api/v1/devices/{id}/config/rollback
	rbPayload, _ := json.Marshal(map[string]string{
		"rollback_key": applyRes.RollbackKey,
	})
	respRb, err := authDo("POST", ts.URL+"/api/v1/devices/test-hik-dev/config/rollback", rbPayload)
	if err != nil {
		t.Fatalf("rollback request failed: %v", err)
	}
	defer respRb.Body.Close()

	if respRb.StatusCode != http.StatusOK {
		t.Fatalf("expected rollback status 200, got %d", respRb.StatusCode)
	}
	var rbRes configengine.ConfigResult
	if err := json.NewDecoder(respRb.Body).Decode(&rbRes); err != nil {
		t.Fatalf("failed to decode rollback response: %v", err)
	}
	if !rbRes.Success {
		t.Errorf("expected rollback success=true")
	}
}
