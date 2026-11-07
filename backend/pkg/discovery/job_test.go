package discovery

import (
	"context"
	"testing"

	"cctv-sentinel/pkg/models"
)

func TestJobManager_Lifecycle(t *testing.T) {
	jm := NewJobManager()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := jm.CreateJob("HYBRID", "eth0", "192.168.1.0/24", 254, cancel)
	if job == nil || job.ID == "" {
		t.Fatalf("Failed to create job")
	}

	if job.Status != JobStatusRunning {
		t.Errorf("Expected status RUNNING, got %s", job.Status)
	}

	jm.UpdateProgress(job.ID, 50, 254)
	snap, ok := jm.GetJob(job.ID)
	if !ok {
		t.Fatalf("Failed to retrieve job")
	}
	if snap.ScannedTargets != 50 {
		t.Errorf("Expected 50 scanned targets, got %d", snap.ScannedTargets)
	}

	// Add discovered device
	dev1 := DiscoveredDevice{
		Device: models.Device{
			ID:           "dev-1",
			IP:           "192.168.1.50",
			Port:         80,
			MAC:          "bc:54:51:00:11:22",
			Manufacturer: models.VendorHikvision,
			Model:        "DS-2CD2042WD-I",
		},
		Fingerprint: FingerprintResult{
			Confidence:      90,
			ConfidenceLevel: "HIGH",
		},
	}
	jm.AddResult(job.ID, dev1)

	// Add duplicate device with lower confidence - should NOT replace higher confidence
	dev1Dup := DiscoveredDevice{
		Device: models.Device{
			ID:           "dev-1-dup",
			IP:           "192.168.1.50",
			Port:         80,
			MAC:          "bc:54:51:00:11:22",
			Manufacturer: models.VendorGeneric,
			Model:        "Standard Streamer",
		},
		Fingerprint: FingerprintResult{
			Confidence:      30,
			ConfidenceLevel: "LOW",
		},
	}
	jm.AddResult(job.ID, dev1Dup)

	snap2, _ := jm.GetJob(job.ID)
	if snap2.FoundCount != 1 {
		t.Errorf("Expected 1 deduplicated device, got %d", snap2.FoundCount)
	}
	if snap2.Results[0].Fingerprint.Confidence != 90 {
		t.Errorf("Expected confidence 90 to be retained, got %d", snap2.Results[0].Fingerprint.Confidence)
	}

	// Cancel job
	ok = jm.CancelJob(job.ID)
	if !ok {
		t.Errorf("Expected CancelJob to return true")
	}
	snap3, _ := jm.GetJob(job.ID)
	if snap3.Status != JobStatusCancelled {
		t.Errorf("Expected status CANCELLED, got %s", snap3.Status)
	}
}

func TestGetAvailableInterfaces(t *testing.T) {
	ifaces, err := GetAvailableInterfaces()
	if err != nil {
		t.Fatalf("Failed to get available interfaces: %v", err)
	}

	t.Logf("Found %d interfaces on host", len(ifaces))
	for _, iface := range ifaces {
		t.Logf("Interface: %s, Up: %v, Multicast: %v, IPs: %v", iface.Name, iface.IsUp, iface.SupportsMulticast, iface.IPv4Addresses)
	}
}
