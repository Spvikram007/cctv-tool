package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/security"
)

func TestStoreConcurrencyAndIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "test_store.json")

	vault, err := security.NewVault(tmpDir)
	if err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}

	store := NewStoreWithVault(storeFile, vault)

	const goroutines = 25
	const iterations = 40

	var wg sync.WaitGroup

	// 1. Concurrent Device Upserts & Reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				devID := fmt.Sprintf("dev-%d-%d", workerID, j)
				pass := fmt.Sprintf("SecretPass-%d-%d", workerID, j)
				store.UpsertDevice(models.Device{
					ID:           devID,
					Name:         fmt.Sprintf("Camera %d-%d", workerID, j),
					IP:           fmt.Sprintf("192.168.1.%d", (workerID*10+j)%250+1),
					Manufacturer: models.VendorHikvision,
					Model:        "DS-2CD2043G2-I",
					Category:     models.CategoryIPCamera,
					Status:       "online",
					Username:     "admin",
					Password:     pass,
					LastSeen:     time.Now().UTC(),
				})

				// Read back via GetDeviceDecrypted
				dev, found := store.GetDeviceDecrypted(devID)
				if !found || dev.ID != devID {
					t.Errorf("worker %d: expected device %s to be found", workerID, devID)
				}
				if dev.Password != pass {
					t.Errorf("worker %d: expected decrypted password %q, got %q", workerID, pass, dev.Password)
				}
			}
		}(i)
	}

	// 2. Concurrent Fleet Listing & Config Updates
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = store.ListDevices()
				targetID := fmt.Sprintf("dev-0-%d", j)
				store.SetNetworkConfig(models.NetworkConfig{
					DeviceID:  targetID,
					IPAddress: "192.168.1.100",
					DHCP:      false,
					Gateway:   "192.168.1.1",
				})
				store.SetImageConfig(models.ImageConfig{
					DeviceID:   targetID,
					Brightness: 60,
					Contrast:   55,
				})
			}
		}(i)
	}

	wg.Wait()

	// Verify persistence file exists and is valid JSON
	if err := store.Save(); err != nil {
		t.Fatalf("failed to save store: %v", err)
	}

	info, err := os.Stat(storeFile)
	if err != nil || info.Size() == 0 {
		t.Fatalf("store file was not written or is empty: %v", err)
	}

	// Test reload from disk
	newStore := NewStoreWithVault(storeFile, vault)
	devices := newStore.ListDevices()
	if len(devices) == 0 {
		t.Fatalf("reloaded store contains 0 devices, expected persistent devices")
	}

	t.Logf("Successfully verified store concurrency with %d concurrent operations. Persisted %d devices.", goroutines*iterations, len(devices))
}

func TestStoreCorruptionDetectionAndRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "sentinel_store.json")

	vault, _ := security.NewVault(tmpDir)
	store := NewStoreWithVault(storeFile, vault)

	// Seed with a verified device
	store.UpsertDevice(models.Device{
		ID:           "camera-01",
		Name:         "North Entrance Hikvision",
		IP:           "192.168.1.50",
		Manufacturer: models.VendorHikvision,
		Model:        "DS-2CD2043G2-I",
		Username:     "admin",
		Password:     "Admin@123",
	})

	if err := store.Save(); err != nil {
		t.Fatalf("failed to save initial store: %v", err)
	}

	// Verify .bak exists after atomic save
	bakFile := storeFile + ".bak"
	if _, err := os.Stat(bakFile); err != nil {
		t.Fatalf("expected .bak file to be created: %v", err)
	}

	// Simulate catastrophic corruption on primary file (e.g. truncated JSON or zero-fill)
	corruptedData := []byte(`{"devices": {"camera-01": {"id": "camera-01", "name": "INCOMPLETE_TRUNCATED_JSON...`)
	if err := os.WriteFile(storeFile, corruptedData, 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	// Reload store — it must detect corruption, recover from .bak, and preserve corrupted copy
	recoveredStore := NewStoreWithVault(storeFile, vault)
	dev, found := recoveredStore.GetDevice("camera-01")
	if !found {
		t.Fatalf("failed to recover 'camera-01' from backup file!")
	}
	if dev.Name != "North Entrance Hikvision" {
		t.Errorf("expected recovered name 'North Entrance Hikvision', got %q", dev.Name)
	}

	// Verify forensic corrupted file was preserved
	matches, _ := filepath.Glob(filepath.Join(tmpDir, "sentinel_store.json.corrupted.*"))
	if len(matches) == 0 {
		t.Errorf("expected forensic copy of corrupted database to be preserved")
	}
}

func TestStoreMigrationsAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "migration_store.json")

	vault, _ := security.NewVault(tmpDir)

	// 1. Manually write a v1 database with a plaintext password
	v1JSON := `{
		"schema_version": 1,
		"devices": {
			"dev-v1": {
				"id": "dev-v1",
				"name": "Legacy V1 Camera",
				"ip": "192.168.1.10",
				"manufacturer": "Dahua",
				"model": "DH-IPC-HFW2431S",
				"category": "IP_CAMERA",
				"status": "online",
				"username": "admin"
			}
		},
		"encrypted_credentials": {}
	}`
	if err := os.WriteFile(storeFile, []byte(v1JSON), 0644); err != nil {
		t.Fatalf("failed to write v1 store: %v", err)
	}

	// 2. Open store with vault; RunMigrations should upgrade to CurrentSchemaVersion (2)
	s := NewStoreWithVault(storeFile, vault)
	if s.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("expected schema version %d after migration, got %d", CurrentSchemaVersion, s.SchemaVersion)
	}

	// Add password and verify migration v2 encrypts it
	s.UpsertDevice(models.Device{
		ID:       "dev-v1",
		Password: "PlaintextPasswordToMigrate",
	})

	dev, _ := s.GetDevice("dev-v1")
	if dev.Password == "PlaintextPasswordToMigrate" {
		t.Errorf("device password in store was not encrypted!")
	}
	if !vault.IsEncrypted(dev.Password) {
		t.Errorf("expected password to have encrypted GCM prefix, got %s", dev.Password)
	}

	// Verify decrypted accessor works
	decDev, _ := s.GetDeviceDecrypted("dev-v1")
	if decDev.Password != "PlaintextPasswordToMigrate" {
		t.Errorf("decrypted password mismatch: got %q", decDev.Password)
	}

	// 3. Test Rollback to v1
	err := s.RollbackMigration(1)
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if s.SchemaVersion != 1 {
		t.Errorf("expected schema version 1 after rollback, got %d", s.SchemaVersion)
	}
}
