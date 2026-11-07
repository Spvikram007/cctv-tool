package discovery

import (
	"strings"
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

func TestLocallyAdministeredMAC(t *testing.T) {
	// Locally administered MACs have bit 1 of the first octet set (e.g. 02:xx, 06:xx, 0a:xx, 0e:xx, 12:xx...)
	localMACs := []string{
		"02:00:00:00:00:01",
		"06:11:22:33:44:55",
		"0a:bb:cc:dd:ee:ff",
		"12:34:56:78:9a:bc",
		"ee:50:8b:11:22:33",
	}

	for _, mac := range localMACs {
		if !IsLocallyAdministeredMAC(mac) {
			t.Errorf("Expected MAC %s to be recognized as locally administered", mac)
		}
		_, found := LookupMACFromOUI(mac)
		if found {
			t.Errorf("Locally administered MAC %s must NOT resolve to a vendor OUI!", mac)
		}
	}

	// Globally unique physical MACs
	globalMACs := []string{
		"bc:54:51:11:22:33", // Hikvision (0xbc = 10111100, bit 1 is 0)
		"38:af:29:11:22:33", // Dahua (0x38 = 00111000, bit 1 is 0)
		"f0:c8:50:11:22:33", // CP PLUS (0xf0 = 11110000, bit 1 is 0)
	}

	for _, mac := range globalMACs {
		if IsLocallyAdministeredMAC(mac) {
			t.Errorf("Physical MAC %s must NOT be flagged as locally administered", mac)
		}
	}

	// Verify that locally administered MAC gets 0 weight in fingerprinting
	fp := AnalyzeFingerprint("192.168.1.99", "02:11:22:33:44:55", []int{80}, nil, DeviceServices{}, "TCP_SWEEP")
	for _, ev := range fp.EvidenceSources {
		if ev.Category == "MAC_OUI" {
			if ev.Weight != 0 {
				t.Errorf("Locally administered MAC should get 0 weight, got %d", ev.Weight)
			}
			if !strings.Contains(ev.Description, "Locally Administered") {
				t.Errorf("Expected Locally Administered explanation, got %s", ev.Description)
			}
		}
	}
}

func TestNeverInferModelFromOUIAlone(t *testing.T) {
	// A MAC OUI alone must NEVER infer a specific model
	res := AnalyzeFingerprint("192.168.1.50", "bc:54:51:11:22:33", []int{80}, nil, DeviceServices{}, "TCP_SWEEP")

	if res.Model != "Unknown" {
		t.Errorf("Model must remain 'Unknown' when only OUI is available! Got: %s", res.Model)
	}

	foundModelUnknown := false
	for _, f := range res.UnknownFields {
		if f == "model" {
			foundModelUnknown = true
			break
		}
	}
	if !foundModelUnknown {
		t.Errorf("Expected 'model' in UnknownFields list")
	}

	if res.Confidence > 30 {
		t.Errorf("OUI alone must stay <= 30%% confidence, got %d", res.Confidence)
	}
	if res.ConfidenceLevel != "LOW" {
		t.Errorf("Expected LOW confidence level for OUI alone, got %s", res.ConfidenceLevel)
	}
}

func TestAvoidDoubleCountingCorrelatedEvidence(t *testing.T) {
	// Probing device with both HTTP Server and WWW-Authenticate pointing to Hikvision
	services := DeviceServices{
		HTTPServer:     "Hikvision-Webs",
		HTTPAuthHeader: "Digest realm=\"Hikvision-DS2CD\"",
	}

	fp := AnalyzeFingerprint("192.168.1.64", "", []int{80}, nil, services, "TCP_SWEEP")

	// Verify that HTTP evidence is counted as a single consolidated proof
	httpEvidenceCount := 0
	for _, ev := range fp.EvidenceSources {
		if ev.Category == "HTTP_BANNER" {
			httpEvidenceCount++
			if ev.Weight > 20 {
				t.Errorf("HTTP evidence should be capped at 20, got %d", ev.Weight)
			}
		}
	}
	if httpEvidenceCount != 1 {
		t.Errorf("Expected exactly 1 consolidated HTTP evidence item, got %d", httpEvidenceCount)
	}

	if fp.ConfidenceExplanation == "" {
		t.Errorf("Expected non-empty ConfidenceExplanation")
	}
}

func TestOEMRecognition(t *testing.T) {
	// Port 34567 is Xiongmai (XM) NetSDK port
	fp := AnalyzeFingerprint("192.168.1.80", "", []int{80, 34567}, nil, DeviceServices{}, "TCP_SWEEP")

	foundXM := false
	for _, ev := range fp.EvidenceSources {
		if ev.Category == "PORT_SIGNATURE" && strings.Contains(ev.Description, "34567") {
			foundXM = true
		}
	}
	if !foundXM {
		t.Errorf("Expected XM port 34567 evidence signature")
	}
	if !strings.Contains(fp.ProbableManufacturer, "Xiongmai") {
		t.Errorf("Expected Xiongmai OEM probable manufacturer, got %s", fp.ProbableManufacturer)
	}
}

func TestAnalyzeFingerprint_ConfidenceLevels(t *testing.T) {
	// Case 1: MAC alone should NOT yield 100% confidence
	res1 := AnalyzeFingerprint("192.168.1.50", "bc:54:51:11:22:33", []int{80}, nil, DeviceServices{}, "TCP_SWEEP")
	if res1.Confidence > 35 {
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
	if res2.Confidence < 70 {
		t.Errorf("Expected >=70%% confidence with banner + RTSP + port, got %d", res2.Confidence)
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
	if res3.Confidence < 75 {
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
