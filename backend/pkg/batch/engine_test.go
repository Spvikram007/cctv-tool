package batch_test

import (
	"encoding/json"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/hikvision"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/batch"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/simulator"
)

func setupBatchTestEnv(t *testing.T) (*batch.Engine, *database.Store, *simulator.MultiDeviceSimulator) {
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
	engine := batch.NewEngine(registry, store, cfgEngine, logger)

	// Seed test devices: 2 Hikvision cameras + 1 unsupported dummy device
	store.UpsertDevice(models.Device{
		ID:           "test-hik-1",
		Name:         "North Gate Camera",
		IP:           "127.0.0.1",
		Port:         18081,
		Manufacturer: models.VendorHikvision,
		Model:        "DS-2CD2043G2-I",
		Protocols:    []models.ProtocolType{models.ProtocolISAPI},
		Status:       "online",
		Username:     "admin",
		Password:     "Admin@123",
	})

	store.UpsertDevice(models.Device{
		ID:           "test-hik-2",
		Name:         "South Gate Camera",
		IP:           "127.0.0.1",
		Port:         18081,
		Manufacturer: models.VendorHikvision,
		Model:        "DS-2CD2043G2-I",
		Protocols:    []models.ProtocolType{models.ProtocolISAPI},
		Status:       "online",
		Username:     "admin",
		Password:     "Admin@123",
	})

	store.UpsertDevice(models.Device{
		ID:           "unsupported-dev-3",
		Name:         "Generic Old Analogue Converter",
		IP:           "192.168.1.99",
		Port:         80,
		Manufacturer: models.VendorGeneric,
		Model:        "Legacy Converter",
		Protocols:    []models.ProtocolType{models.ProtocolHTTP},
		Status:       "offline",
	})

	return engine, store, sim
}

func TestBatchPreviewAndCapabilityCheck(t *testing.T) {
	engine, _, sim := setupBatchTestEnv(t)
	defer sim.Stop()

	targetIDs := []string{"test-hik-1", "test-hik-2", "unsupported-dev-3"}

	// 1. Generate preview for NTP Sync
	preview, err := engine.GenerateBatchPreview(
		batch.ActionTimeSync,
		targetIDs,
		json.RawMessage(`{"ntp_server":"time.google.com"}`),
		"AdminUser",
	)
	if err != nil {
		t.Fatalf("GenerateBatchPreview failed: %v", err)
	}

	if preview.TotalTargets != 3 {
		t.Fatalf("expected 3 total targets, got %d", preview.TotalTargets)
	}
	if preview.SupportedTargets != 2 {
		t.Errorf("expected 2 supported targets, got %d", preview.SupportedTargets)
	}
	if preview.UnsupportedTargets != 1 {
		t.Errorf("expected 1 unsupported target, got %d", preview.UnsupportedTargets)
	}
	if len(preview.Warnings) == 0 {
		t.Errorf("expected warning for unsupported target")
	}
}

func TestBatchRebootRequiresApproval(t *testing.T) {
	engine, _, sim := setupBatchTestEnv(t)
	defer sim.Stop()

	targetIDs := []string{"test-hik-1"}

	preview, err := engine.GenerateBatchPreview(
		batch.ActionReboot,
		targetIDs,
		nil,
		"AdminUser",
	)
	if err != nil {
		t.Fatalf("GenerateBatchPreview failed: %v", err)
	}

	if !preview.RequiresExplicitApproval {
		t.Errorf("reboot preview must require explicit operator approval")
	}
}

func TestBatchExecutionPartialSuccess(t *testing.T) {
	engine, _, sim := setupBatchTestEnv(t)
	defer sim.Stop()

	targetIDs := []string{"test-hik-1", "test-hik-2", "unsupported-dev-3"}

	preview, err := engine.GenerateBatchPreview(
		batch.ActionHealthCheck,
		targetIDs,
		nil,
		"Technician",
	)
	if err != nil {
		t.Fatalf("GenerateBatchPreview failed: %v", err)
	}

	// Execute batch job
	job, err := engine.ExecuteBatch(preview.PreviewToken, "Fleet Health Check", 2, 10, "Technician")
	if err != nil {
		t.Fatalf("ExecuteBatch failed: %v", err)
	}

	// Wait for worker pool completion
	time.Sleep(300 * time.Millisecond)

	retrievedJob, ok := engine.GetJob(job.ID)
	if !ok {
		t.Fatalf("job not found in engine")
	}

	if retrievedJob.Progress != 100 {
		t.Errorf("expected 100 progress, got %d", retrievedJob.Progress)
	}
	if retrievedJob.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", retrievedJob.SuccessCount)
	}
	if retrievedJob.UnsupportedCount != 1 {
		t.Errorf("expected 1 unsupported device, got %d", retrievedJob.UnsupportedCount)
	}
	if retrievedJob.Status != "PARTIAL_SUCCESS" {
		t.Errorf("expected PARTIAL_SUCCESS, got %s", retrievedJob.Status)
	}
}

func TestBatchCancellation(t *testing.T) {
	engine, _, sim := setupBatchTestEnv(t)
	defer sim.Stop()

	targetIDs := []string{"test-hik-1", "test-hik-2"}

	preview, err := engine.GenerateBatchPreview(
		batch.ActionTimeSync,
		targetIDs,
		json.RawMessage(`{"ntp_server":"time.cloudflare.com"}`),
		"Technician",
	)
	if err != nil {
		t.Fatalf("GenerateBatchPreview failed: %v", err)
	}

	job, err := engine.ExecuteBatch(preview.PreviewToken, "Slow Sync Job", 1, 500, "Technician")
	if err != nil {
		t.Fatalf("ExecuteBatch failed: %v", err)
	}

	// Cancel immediately
	if err := engine.CancelJob(job.ID); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	retrieved, _ := engine.GetJob(job.ID)
	if retrieved.Status != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %s", retrieved.Status)
	}
}
