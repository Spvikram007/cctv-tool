package licensing_test

import (
	"testing"

	"cctv-sentinel/pkg/licensing"
)

func TestLicensingAndGST(t *testing.T) {
	mgr := licensing.NewManager()
	machineHash := mgr.GetMachineHash()
	if machineHash == "" {
		t.Fatalf("Expected non-empty machine hash")
	}

	company := "Tech Security Integrators Ltd"
	// 1. Generate legitimate offline key
	validKey := mgr.GenerateActivationCode(machineHash, licensing.TierTechnicianPro, company, 365)
	
	// 2. Activate
	err := mgr.ActivateLicense(validKey, company, "29AABCT1234F1Z5")
	if err != nil {
		t.Fatalf("Failed to activate valid key: %v", err)
	}

	lic := mgr.GetLicense()
	if lic.Tier != licensing.TierTechnicianPro {
		t.Errorf("Expected tier %s, got %s", licensing.TierTechnicianPro, lic.Tier)
	}
	if lic.MaxDevices != 64 {
		t.Errorf("Expected 64 max devices, got %d", lic.MaxDevices)
	}

	// 3. Test Invalid Key protection
	err = mgr.ActivateLicense("SENTINEL-FAKE-TIER-0000-INVALID", company, "")
	if err == nil {
		t.Errorf("Expected invalid key to be rejected")
	}

	// 4. Test GST Invoicing
	inv := mgr.GenerateGSTInvoice(company, "29AABCT1234F1Z5", "Karnataka", licensing.TierTechnicianPro)
	if inv.BaseAmount != 4999.00 {
		t.Errorf("Expected base amount 4999, got %f", inv.BaseAmount)
	}
	if inv.CGST <= 0 || inv.SGST <= 0 || inv.TotalAmount <= inv.BaseAmount {
		t.Errorf("Invalid GST calculation breakdown: %+v", inv)
	}
}
