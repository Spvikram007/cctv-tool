package firmware_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/firmware"
	"cctv-sentinel/pkg/models"
)

func setupFirmwareTestEnv(t *testing.T) (*firmware.Engine, *database.Store) {
	store := database.NewStore("")
	registry := adapters.NewRegistry()
	logger := audit.NewLogger()
	tmpDir := t.TempDir()

	engine := firmware.NewEngine(store, registry, logger, tmpDir)

	// Seed test Hikvision camera
	store.UpsertDevice(models.Device{
		ID:           "test-hik-2043",
		Name:         "Entrance Hikvision Dome",
		IP:           "127.0.0.1",
		Port:         18081,
		Manufacturer: models.VendorHikvision,
		Model:        "DS-2CD2043G2-I",
		Firmware:     "V5.5.0 build 220101",
		Protocols:    []models.ProtocolType{models.ProtocolISAPI},
		Status:       "online",
	})

	// Seed test Dahua camera
	store.UpsertDevice(models.Device{
		ID:           "test-dahua-1230",
		Name:         "Parking Dahua Bullet",
		IP:           "127.0.0.1",
		Port:         18082,
		Manufacturer: models.VendorDahua,
		Model:        "DH-IPC-HFW1230T1P-ZS-S4",
		Firmware:     "2.800.0000000.10.R",
		Protocols:    []models.ProtocolType{models.ProtocolDahuaCGI},
		Status:       "online",
	})

	// Seed test CP PLUS camera
	store.UpsertDevice(models.Device{
		ID:           "test-cpplus-ta21",
		Name:         "Lobby CP PLUS Camera",
		IP:           "127.0.0.1",
		Port:         18083,
		Manufacturer: models.VendorCPPlus,
		Model:        "CP-UNC-TA21PL3-V3",
		Firmware:     "2.800.0000000.12.R",
		Protocols:    []models.ProtocolType{models.ProtocolONVIF},
		Status:       "online",
	})

	return engine, store
}

func TestCheckEligibilityMatchingModel(t *testing.T) {
	engine, _ := setupFirmwareTestEnv(t)

	elig, err := engine.CheckEligibility("test-hik-2043", "fw-hik-2cd2043g2-v571")
	if err != nil {
		t.Fatalf("CheckEligibility failed: %v", err)
	}

	if !elig.Eligible {
		t.Errorf("expected matching model to be eligible, got errors: %v", elig.ValidationErrors)
	}
	if elig.CrossFlashRisk {
		t.Errorf("expected no cross-flash risk for matching manufacturer")
	}
}

func TestCheckEligibilityCrossFlashRejection(t *testing.T) {
	engine, _ := setupFirmwareTestEnv(t)

	// Attempt to flash Dahua firmware onto a CP PLUS device
	elig, err := engine.CheckEligibility("test-cpplus-ta21", "fw-dahua-1230t1p-v2820")
	if err != nil {
		t.Fatalf("CheckEligibility failed: %v", err)
	}

	if elig.Eligible {
		t.Errorf("CRITICAL BUG: Cross-flashing CP PLUS with Dahua firmware must be rejected!")
	}
	if !elig.CrossFlashRisk {
		t.Errorf("expected CrossFlashRisk to be true")
	}
	if len(elig.ValidationErrors) == 0 {
		t.Errorf("expected validation errors explaining cross-flash block")
	}
}

func TestCheckEligibilityModelMismatch(t *testing.T) {
	engine, _ := setupFirmwareTestEnv(t)

	// Attempt to flash Hikvision 2043 firmware onto Dahua 1230
	elig, err := engine.CheckEligibility("test-dahua-1230", "fw-hik-2cd2043g2-v571")
	if err != nil {
		t.Fatalf("CheckEligibility failed: %v", err)
	}

	if elig.Eligible {
		t.Errorf("expected model mismatch to be rejected")
	}
}

func TestFirmwareStagePreviewAndExecute(t *testing.T) {
	engine, store := setupFirmwareTestEnv(t)

	// Stage preview
	preview, err := engine.StageFirmwarePreview("test-hik-2043", "fw-hik-2cd2043g2-v571", "SeniorEngineer")
	if err != nil {
		t.Fatalf("StageFirmwarePreview failed: %v", err)
	}

	if preview.PreviewToken == "" {
		t.Fatalf("expected non-empty preview token")
	}
	if !preview.ExplicitApprovalRequired {
		t.Errorf("expected explicit approval requirement")
	}

	// Approve and dispatch
	job, err := engine.ApproveAndExecute(preview.PreviewToken, "SeniorEngineer")
	if err != nil {
		t.Fatalf("ApproveAndExecute failed: %v", err)
	}

	if job.Status != "UPLOADING" {
		t.Errorf("expected UPLOADING status initially, got %s", job.Status)
	}

	// Wait for background simulated flash & post-update verification
	time.Sleep(450 * time.Millisecond)

	retrieved, ok := engine.GetJob(job.ID)
	if !ok {
		t.Fatalf("job not found in engine")
	}

	if retrieved.Status != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %s", retrieved.Status)
	}
	if !retrieved.VerifiedMatch {
		t.Errorf("expected VerifiedMatch to be true")
	}
	if retrieved.PostUpdateVersion != "V5.7.1 build 230915" {
		t.Errorf("unexpected post update version: %s", retrieved.PostUpdateVersion)
	}

	// Check that device store record was updated
	dev, _ := store.GetDevice("test-hik-2043")
	if dev.Firmware != "V5.7.1 build 230915" {
		t.Errorf("expected device store firmware to be updated, got %s", dev.Firmware)
	}
}

func TestVerifyFileChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	sampleFile := filepath.Join(tmpDir, "test_fw.bin")
	content := []byte("CCTV_FIRMWARE_HEADER_TEST_PAYLOAD_123456789")
	_ = os.WriteFile(sampleFile, content, 0644)

	// Compute known sha256
	match, actual, err := firmware.VerifyFileChecksum(sampleFile, "invalid_checksum")
	if err != nil {
		t.Fatalf("VerifyFileChecksum failed: %v", err)
	}
	if match {
		t.Errorf("expected false for invalid checksum")
	}

	// Test with actual checksum
	matchValid, _, err := firmware.VerifyFileChecksum(sampleFile, actual)
	if err != nil {
		t.Fatalf("VerifyFileChecksum failed: %v", err)
	}
	if !matchValid {
		t.Errorf("expected true for matching checksum")
	}
}
