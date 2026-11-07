package cpplus

import (
	"testing"

	"cctv-sentinel/pkg/models"
)

func TestClassifyModel(t *testing.T) {
	tests := []struct {
		model        string
		expectedCat  models.DeviceCategory
		expectedChan int
	}{
		{"CP-UNC-TA21PL3-V3", models.CategoryIPCamera, 1},
		{"CP-UNR-4K4162-V2", models.CategoryNVR, 16},
		{"CP-UNR-4322-V2", models.CategoryNVR, 32},
		{"CP-UVR-0801E1-V3", models.CategoryXVR, 8},
		{"CP-UVR-1601E1", models.CategoryXVR, 16},
	}

	for _, tc := range tests {
		cat, chans := classifyModel(tc.model)
		if cat != tc.expectedCat {
			t.Errorf("model %s: expected category %s, got %s", tc.model, tc.expectedCat, cat)
		}
		if chans != tc.expectedChan {
			t.Errorf("model %s: expected %d channels, got %d", tc.model, tc.expectedChan, chans)
		}
	}
}

func TestCPPlusNVRRejectsAnalogFormat(t *testing.T) {
	adapter := NewCPPlusAdapter()
	dev := &models.Device{
		ID:           "test-nvr",
		Model:        "CP-UNR-4K4162-V2",
		Category:     models.CategoryNVR,
		Manufacturer: models.VendorCPPlus,
		ChannelCount: 16,
	}

	err := adapter.SetChannelAnalogMode(dev, 1, "TVI")
	if err == nil {
		t.Fatalf("expected error when setting analog format on pure IP NVR")
	}
}

func TestCPPlusGetCapabilities(t *testing.T) {
	adapter := NewCPPlusAdapter()
	devNVR := &models.Device{
		ID:           "test-nvr",
		Model:        "CP-UNR-4K4162-V2",
		Category:     models.CategoryNVR,
		Manufacturer: models.VendorCPPlus,
	}

	caps, err := adapter.GetCapabilities(devNVR)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if caps.AnalogFormat != models.CapabilityUnsupported {
		t.Errorf("expected AnalogFormat unsupported for NVR")
	}
	if caps.Recording != models.CapabilitySupported {
		t.Errorf("expected Recording supported for NVR")
	}
}
