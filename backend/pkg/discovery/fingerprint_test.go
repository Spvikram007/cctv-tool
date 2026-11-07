package discovery

import (
	"testing"

	"cctv-sentinel/pkg/models"
)

func TestLookupMACFromOUI(t *testing.T) {
	cases := []struct {
		mac      string
		expected models.Manufacturer
		found    bool
	}{
		{"bc:54:51:aa:bb:cc", models.VendorHikvision, true},
		{"BC-54-51-11-22-33", models.VendorHikvision, true},
		{"38:af:29:01:02:03", models.VendorDahua, true},
		{"f0:c8:50:99:88:77", models.VendorCPPlus, true},
		{"00:40:8c:12:34:56", models.VendorAxis, true},
		{"34:bd:fa:aa:bb:cc", models.VendorUniview, true},
		{"ff:ff:ff:ff:ff:ff", models.VendorGeneric, false},
	}

	for _, c := range cases {
		vendor, ok := LookupMACFromOUI(c.mac)
		if ok != c.found {
			t.Errorf("For MAC %s: expected found=%v, got %v", c.mac, c.found, ok)
		}
		if c.found && vendor != c.expected {
			t.Errorf("For MAC %s: expected vendor %s, got %s", c.mac, c.expected, vendor)
		}
	}
}

func TestAnalyzeFingerprint_ConfidenceLevels(t *testing.T) {
	// Case 1: MAC alone should NOT yield 100% confidence
	res1 := AnalyzeFingerprint("192.168.1.50", "bc:54:51:11:22:33", []int{80}, nil, DeviceServices{}, "TCP_SWEEP")
	if res1.Confidence > 40 {
		t.Errorf("MAC alone must not yield high confidence! Got: %d", res1.Confidence)
	}
	if res1.ConfidenceLevel != "LOW" {
		t.Errorf("Expected confidence level LOW, got %s", res1.ConfidenceLevel)
	}

	// Case 2: Open ports + HTTP banner + OUI -> Medium to High confidence
	res2 := AnalyzeFingerprint(
		"192.168.1.64",
		"bc:54:51:11:22:33",
		[]int{80, 8000, 554},
		nil,
		DeviceServices{
			HTTPServer: "Hikvision-Webs",
			RTSPServer: "Hikvision RTSP Server 1.0",
		},
		"TCP_SWEEP",
	)
	if res2.Confidence < 75 {
		t.Errorf("Expected >=75%% confidence with banner + RTSP + port, got %d", res2.Confidence)
	}
	if res2.IdentifiedManufacturer != models.VendorHikvision {
		t.Errorf("Expected Hikvision, got %s", res2.IdentifiedManufacturer)
	}

	// Case 3: ONVIF WS-Discovery + RTSP -> High confidence (85%+)
	res3 := AnalyzeFingerprint(
		"192.168.1.65",
		"",
		[]int{80, 554},
		&WSProbeMatch{
			EndpointReference: "urn:uuid:test-1234",
			Manufacturer:      "Dahua",
			Model:             "DH-SD49225T-HN",
			Scopes:            []string{"onvif://www.onvif.org/type/ptz", "onvif://www.onvif.org/name/Dahua"},
			XAddrs:            []string{"http://192.168.1.65:80/onvif/device_service"},
		},
		DeviceServices{
			RTSPServer: "Dahua RTSP Server",
		},
		"ONVIF",
	)
	if res3.Confidence < 85 {
		t.Errorf("Expected high confidence for ONVIF + RTSP, got %d", res3.Confidence)
	}
	if res3.ConfidenceLevel != "HIGH" {
		t.Errorf("Expected HIGH confidence level, got %s", res3.ConfidenceLevel)
	}
	if res3.Category != models.CategoryPTZCamera {
		t.Errorf("Expected PTZ camera classification, got %s", res3.Category)
	}
}

func TestClassifyDevice(t *testing.T) {
	cases := []struct {
		model        string
		scopes       []string
		ports        []int
		manufacturer models.Manufacturer
		expected     models.DeviceCategory
	}{
		{"DS-2DE4225IW-DE", nil, []int{80, 554}, models.VendorHikvision, models.CategoryPTZCamera},
		{"DS-7608NI-K2 / 8P", nil, []int{80, 8000}, models.VendorHikvision, models.CategoryNVR},
		{"DH-XVR5108HS-4KL-I3", nil, []int{80, 37777}, models.VendorDahua, models.CategoryXVR},
		{"DS-7208HGHI-K1", nil, []int{80, 8000}, models.VendorHikvision, models.CategoryDVR},
		{"DS-6704HFI", nil, []int{80, 8000}, models.VendorHikvision, models.CategoryEncoder},
		{"DS-2CD2042WD-I", nil, []int{80, 554}, models.VendorHikvision, models.CategoryIPCamera},
		{"Unknown Device", nil, []int{9999}, models.VendorGeneric, models.CategoryUnknown},
	}

	for _, c := range cases {
		var match *WSProbeMatch
		if len(c.scopes) > 0 {
			match = &WSProbeMatch{Scopes: c.scopes}
		}
		cat := ClassifyDevice(c.model, match, c.ports, c.manufacturer)
		if cat != c.expected {
			t.Errorf("For model %s: expected category %s, got %s", c.model, c.expected, cat)
		}
	}
}
