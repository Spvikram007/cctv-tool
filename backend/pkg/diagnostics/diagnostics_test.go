package diagnostics_test

import (
	"testing"

	"cctv-sentinel/pkg/diagnostics"
)

func TestCalculatorsAndDiagnostics(t *testing.T) {
	// 1. Subnet Calculator test
	sub, err := diagnostics.CalculateSubnet("192.168.10.0/24")
	if err != nil {
		t.Fatalf("CalculateSubnet failed: %v", err)
	}
	if sub.TotalHosts != 256 || sub.UsableHosts != 254 {
		t.Errorf("Unexpected host calculation: total %d, usable %d", sub.TotalHosts, sub.UsableHosts)
	}
	if sub.BroadcastAddress != "192.168.10.255" {
		t.Errorf("Unexpected broadcast: %s", sub.BroadcastAddress)
	}

	// 2. HDD Storage Calculator test
	hdd := diagnostics.CalculateHDD(16, "H.265", "4MP", 2048, 30, 24)
	if hdd.TotalStorageGB <= 0 || hdd.TotalStorageTB <= 0 {
		t.Errorf("Invalid HDD calculation result: %+v", hdd)
	}
	if hdd.RecommendedHDDs < 1 {
		t.Errorf("Expected at least 1 recommended disk")
	}

	// 3. Bandwidth Calculator test
	bw := diagnostics.CalculateBandwidth(16, 4096, 1024)
	if bw.TotalMainMbps != 65.54 && bw.TotalMainMbps != 65.536 && bw.TotalCombinedMbps <= 0 {
		t.Errorf("Invalid Bandwidth calculation: %+v", bw)
	}
}
