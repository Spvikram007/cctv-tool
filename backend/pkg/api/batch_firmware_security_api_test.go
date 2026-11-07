package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/hikvision"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/batch"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/firmware"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/security"
	"cctv-sentinel/pkg/simulator"
)

func TestPhase5BatchFirmwareSecurityAPIE2E(t *testing.T) {
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		t.Fatalf("failed to start simulator: %v", err)
	}
	defer sim.Stop()

	store := database.NewStore("")
	registry := adapters.NewRegistry()
	hikAdapter := hikvision.NewHikvisionAdapter()
	registry.Register(hikAdapter, models.ProtocolISAPI)

	logger := audit.NewLogger()
	cfgEngine := configengine.NewEngine(registry, store, logger)
	discEngine := discovery.NewEngine(registry)
	licMgr := licensing.NewManager()
	batchEng := batch.NewEngine(registry, store, cfgEngine, logger)
	firmwareEng := firmware.NewEngine(store, registry, logger, t.TempDir())
	rbacMgr := security.NewRBACManager()

	server := NewServer(store, registry, cfgEngine, discEngine, licMgr, logger, nil, nil, batchEng, firmwareEng, rbacMgr)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Seed test camera
	store.UpsertDevice(models.Device{
		ID:           "test-cam-p5",
		Name:         "Perimeter Camera 1",
		IP:           "127.0.0.1",
		Port:         18081,
		Manufacturer: models.VendorHikvision,
		Model:        "DS-2CD2043G2-I",
		Firmware:     "V5.5.0 build 220101",
		Protocols:    []models.ProtocolType{models.ProtocolISAPI},
		Status:       "online",
		Username:     "admin",
		Password:     "Admin@123",
	})

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Test Security Login (RBAC)
	loginReq := map[string]string{
		"username": "admin",
		"password": "Admin@Sentinel2026!",
	}
	b, _ := json.Marshal(loginReq)
	resp, err := client.Post(ts.URL+"/api/v1/security/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("failed to call security login: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for login, got %d", resp.StatusCode)
	}
	var loginResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&loginResp)
	resp.Body.Close()

	if loginResp["success"] != true {
		t.Fatalf("expected login success, got %+v", loginResp)
	}

	// 2. Test Vault Status
	vaultResp, err := client.Get(ts.URL + "/api/v1/security/vault/status")
	if err != nil || vaultResp.StatusCode != http.StatusOK {
		t.Fatalf("vault status endpoint failed")
	}
	var vaultData map[string]interface{}
	_ = json.NewDecoder(vaultResp.Body).Decode(&vaultData)
	vaultResp.Body.Close()
	if vaultData["vault_status"] != "ACTIVE" {
		t.Fatalf("expected ACTIVE vault status, got %v", vaultData)
	}

	// 3. Test Audit Hash Chain Integrity Verification
	auditResp, err := client.Get(ts.URL + "/api/v1/audit/verify")
	if err != nil || auditResp.StatusCode != http.StatusOK {
		t.Fatalf("audit verify endpoint failed")
	}
	var auditData map[string]interface{}
	_ = json.NewDecoder(auditResp.Body).Decode(&auditData)
	auditResp.Body.Close()
	if auditData["valid"] != true || auditData["status"] != "TAMPER_FREE" {
		t.Fatalf("expected TAMPER_FREE audit status, got %+v", auditData)
	}

	// 4. Test Batch Preview & Execute
	previewReq := map[string]interface{}{
		"action":     "HEALTH_CHECK",
		"target_ids": []string{"test-cam-p5"},
		"operator":   "Admin",
	}
	b, _ = json.Marshal(previewReq)
	prevResp, err := client.Post(ts.URL+"/api/v1/batch/preview", "application/json", bytes.NewReader(b))
	if err != nil || prevResp.StatusCode != http.StatusOK {
		t.Fatalf("batch preview failed: code %d, err %v", prevResp.StatusCode, err)
	}
	var previewData map[string]interface{}
	_ = json.NewDecoder(prevResp.Body).Decode(&previewData)
	prevResp.Body.Close()

	previewToken := previewData["preview_token"].(string)
	if previewToken == "" {
		t.Fatalf("expected preview token")
	}

	// Execute batch job
	execReq := map[string]interface{}{
		"preview_token": previewToken,
		"job_name":      "Unit Test Fleet Check",
		"concurrency":   2,
		"rate_limit_ms": 10,
		"operator":      "Admin",
	}
	b, _ = json.Marshal(execReq)
	execResp, err := client.Post(ts.URL+"/api/v1/batch/execute", "application/json", bytes.NewReader(b))
	if err != nil || execResp.StatusCode != http.StatusAccepted {
		t.Fatalf("batch execute failed: code %d, err %v", execResp.StatusCode, err)
	}
	execResp.Body.Close()

	// 5. Test Firmware Inventory & Compatibility Check
	fwInvResp, err := client.Get(ts.URL + "/api/v1/firmware/inventory")
	if err != nil || fwInvResp.StatusCode != http.StatusOK {
		t.Fatalf("firmware inventory failed")
	}
	var fwList []map[string]interface{}
	_ = json.NewDecoder(fwInvResp.Body).Decode(&fwList)
	fwInvResp.Body.Close()
	if len(fwList) == 0 {
		t.Fatalf("expected non-empty firmware inventory")
	}

	// Check eligibility
	checkReq := map[string]string{
		"device_id":  "test-cam-p5",
		"package_id": fwList[0]["id"].(string),
	}
	b, _ = json.Marshal(checkReq)
	checkResp, err := client.Post(ts.URL+"/api/v1/firmware/check-compatibility", "application/json", bytes.NewReader(b))
	if err != nil || checkResp.StatusCode != http.StatusOK {
		t.Fatalf("firmware compatibility check failed")
	}
	var checkData map[string]interface{}
	_ = json.NewDecoder(checkResp.Body).Decode(&checkData)
	checkResp.Body.Close()
	if checkData["current_model"] != "DS-2CD2043G2-I" {
		t.Fatalf("expected current model in eligibility check, got %+v", checkData)
	}
}
