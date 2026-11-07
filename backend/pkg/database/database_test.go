package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cctv-sentinel/pkg/models"
)

func TestStoreConcurrencyAndIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	storeFile := filepath.Join(tmpDir, "test_store.json")

	store := NewStore(storeFile)

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
				store.UpsertDevice(models.Device{
					ID:           devID,
					Name:         fmt.Sprintf("Camera %d-%d", workerID, j),
					IP:           fmt.Sprintf("192.168.1.%d", (workerID*10+j)%250+1),
					Manufacturer: models.VendorHikvision,
					Model:        "DS-2CD2043G2-I",
					Category:     models.CategoryIPCamera,
					Status:       "online",
					LastSeen:     time.Now().UTC(),
				})

				// Read back
				dev, found := store.GetDevice(devID)
				if !found || dev.ID != devID {
					t.Errorf("worker %d: expected device %s to be found", workerID, devID)
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
	newStore := NewStore(storeFile)
	devices := newStore.ListDevices()
	if len(devices) == 0 {
		t.Fatalf("reloaded store contains 0 devices, expected persistent devices")
	}

	t.Logf("Successfully verified store concurrency with %d concurrent operations. Persisted %d devices.", goroutines*iterations, len(devices))
}
