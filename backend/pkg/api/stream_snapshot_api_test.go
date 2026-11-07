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
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/media/snapshot"
	"cctv-sentinel/pkg/media/streamhub"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/simulator"
)

func TestStreamAndSnapshotAPIE2E(t *testing.T) {
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		t.Fatalf("failed to start simulator: %v", err)
	}
	defer sim.Stop()

	tmpDir := t.TempDir()
	store := database.NewStore("")
	registry := adapters.NewRegistry()
	hikAdapter := hikvision.NewHikvisionAdapter()
	registry.Register(hikAdapter, models.ProtocolISAPI)

	logger := audit.NewLogger()
	cfgEngine := configengine.NewEngine(registry, store, logger)
	discEngine := discovery.NewEngine(registry)
	licMgr := licensing.NewManager()
	snapEngine := snapshot.NewEngine(store, registry, tmpDir)
	streamHub := streamhub.NewHub(store, registry)

	server := NewServer(store, registry, cfgEngine, discEngine, licMgr, logger, snapEngine, streamHub, nil, nil, nil)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 1. Seed simulated camera device
	dev := models.Device{
		ID:           "test-cam-stream-01",
		Name:         "Lab Camera 1",
		IP:           "127.0.0.1",
		Port:         18081,
		RTSPPort:     18554,
		Manufacturer: models.VendorHikvision,
		Category:     models.CategoryIPCamera,
		Status:       "online",
		Protocols:    []models.ProtocolType{models.ProtocolISAPI, models.ProtocolRTSP},
		Username:     "admin",
		Password:     "Admin@123",
	}
	store.UpsertDevice(dev)

	client := &http.Client{Timeout: 5 * time.Second}

	// 2. Request scoped Stream Token: POST /api/v1/devices/{id}/stream/token
	tokenReq := map[string]interface{}{
		"stream_type": "main",
		"user":        "SeniorTechnician",
		"ttl_seconds": 3600,
	}
	reqBytes, _ := json.Marshal(tokenReq)
	resp, err := client.Post(ts.URL+"/api/v1/devices/"+dev.ID+"/stream/token", "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("failed to request stream token: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for stream token, got %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token      string `json:"token"`
		DeviceID   string `json:"device_id"`
		StreamType string `json:"stream_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		t.Fatalf("failed to parse stream token response: %v", err)
	}
	resp.Body.Close()

	if tokenResp.Token == "" || tokenResp.DeviceID != dev.ID {
		t.Fatalf("invalid token response: %+v", tokenResp)
	}

	// 3. Query stream stats: GET /api/v1/streams/{token}/stats
	statsResp, err := client.Get(ts.URL + "/api/v1/streams/" + tokenResp.Token + "/stats")
	if err != nil {
		t.Fatalf("failed to query stream stats: %v", err)
	}
	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for stream stats, got %d", statsResp.StatusCode)
	}
	var stats map[string]interface{}
	_ = json.NewDecoder(statsResp.Body).Decode(&stats)
	statsResp.Body.Close()

	if stats["device_id"] != dev.ID {
		t.Fatalf("unexpected device_id in stream stats: %v", stats)
	}

	// 4. Test Stream Diagnostics endpoint: GET /api/v1/devices/{id}/diagnostics/stream
	diagResp, err := client.Get(ts.URL + "/api/v1/devices/" + dev.ID + "/diagnostics/stream")
	if err != nil {
		t.Fatalf("failed to run stream diagnostics: %v", err)
	}
	if diagResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for stream diagnostics, got %d", diagResp.StatusCode)
	}
	var diagData map[string]interface{}
	_ = json.NewDecoder(diagResp.Body).Decode(&diagData)
	diagResp.Body.Close()

	if diagData["device_id"] != dev.ID {
		t.Fatalf("expected device_id in diag, got %v", diagData)
	}

	// 5. Test Capacity Calculator endpoint: GET /api/v1/diagnostics/capacity
	capResp, err := client.Get(ts.URL + "/api/v1/diagnostics/capacity?cameras=16&resolution=4K&codec=H.265&days=30&viewers=4")
	if err != nil {
		t.Fatalf("failed to call capacity calculator: %v", err)
	}
	if capResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for capacity calculator, got %d", capResp.StatusCode)
	}
	var capData map[string]interface{}
	_ = json.NewDecoder(capResp.Body).Decode(&capData)
	capResp.Body.Close()

	if capData["camera_count"] != float64(16) {
		t.Fatalf("expected 16 cameras in calculation result, got %v", capData["camera_count"])
	}
	if capData["recommended_switch_port"] == "" {
		t.Fatalf("expected recommended_switch_port in calculation result, got empty")
	}
}
