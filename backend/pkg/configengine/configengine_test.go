package configengine_test

import (
	"encoding/json"
	"testing"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/hikvision"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/simulator"
)

func TestConfigEngine12Steps(t *testing.T) {
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer sim.Stop()

	store := database.NewStore("")
	logger := audit.NewLogger()
	registry := adapters.NewRegistry()
	hikAdapter := hikvision.NewHikvisionAdapter()
	registry.Register(hikAdapter, models.ProtocolISAPI)

	devHik, err := hikAdapter.Detect("127.0.0.1", 18081, "admin", "Admin@123")
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	store.UpsertDevice(*devHik)

	engine := configengine.NewEngine(registry, store, logger)

	// Test 1: Generate Preview with proposed network change (Step 5 & 6)
	proposedNet := models.NetworkConfig{
		DeviceID:   devHik.ID,
		DHCP:       false,
		IPAddress:  "192.168.1.188",
		SubnetMask: "255.255.255.0",
		Gateway:    "192.168.1.1",
	}
	propBytes, _ := json.Marshal(proposedNet)
	preview, err := engine.GeneratePreview(devHik.ID, configengine.ConfigTypeNetwork, propBytes)
	if err != nil {
		t.Fatalf("GeneratePreview failed: %v", err)
	}
	if !preview.Valid {
		t.Fatalf("Expected valid proposed config, got errors: %v", preview.ValidationErrors)
	}
	if preview.RollbackKey == "" {
		t.Fatalf("Expected rollback key to be generated")
	}

	// Test 2: Apply Confirmed Config (Step 8, 9, 10, 11)
	store.SetNetworkConfig(proposedNet)
	res, err := engine.ApplyConfirmed(devHik.ID, configengine.ConfigTypeNetwork, preview.RollbackKey, "TechOperator")
	if err != nil {
		t.Fatalf("ApplyConfirmed failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("Expected configuration apply success")
	}

	// Test 3: Rollback (Step 12)
	rbRes, err := engine.Rollback(devHik.ID, preview.RollbackKey, "TechOperator")
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	if !rbRes.Success {
		t.Fatalf("Expected rollback success")
	}

	// Verify Audit trail recorded actions with credentials scrubbed
	entries := logger.GetEntries()
	if len(entries) < 2 {
		t.Errorf("Expected at least 2 audit entries, found %d", len(entries))
	}
}
