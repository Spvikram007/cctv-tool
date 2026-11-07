package discovery

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGenerateMessageID(t *testing.T) {
	id1 := GenerateMessageID()
	id2 := GenerateMessageID()

	if !strings.HasPrefix(id1, "urn:uuid:") {
		t.Fatalf("Expected MessageID to start with urn:uuid:, got %s", id1)
	}
	if id1 == id2 {
		t.Fatalf("Expected unique MessageIDs, got identical %s", id1)
	}
}

func TestBuildProbeMessage(t *testing.T) {
	msgID := "urn:uuid:12345678-1234-4234-8234-123456789abc"
	msg := BuildProbeMessage(msgID)

	xmlStr := string(msg)
	if !strings.Contains(xmlStr, msgID) {
		t.Errorf("Probe message missing MessageID: %s", xmlStr)
	}
	if !strings.Contains(xmlStr, "dn:NetworkVideoTransmitter") {
		t.Errorf("Probe message missing NetworkVideoTransmitter: %s", xmlStr)
	}
	if !strings.Contains(xmlStr, "http://schemas.xmlsoap.org/ws/2005/04/discovery") {
		t.Errorf("Probe message missing WS-Discovery namespace: %s", xmlStr)
	}
}

func TestParseProbeMatches_Hikvision(t *testing.T) {
	hikvisionXML := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
               xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery">
  <soap:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/ProbeMatches</wsa:Action>
  </soap:Header>
  <soap:Body>
    <d:ProbeMatches>
      <d:ProbeMatch>
        <wsa:EndpointReference>
          <wsa:Address>urn:uuid:48494b56-3030-3030-3030-bc5451a2b3c4</wsa:Address>
        </wsa:EndpointReference>
        <d:Types>dn:NetworkVideoTransmitter tds:Device</d:Types>
        <d:Scopes>
          onvif://www.onvif.org/type/video_encoder
          onvif://www.onvif.org/type/audio_encoder
          onvif://www.onvif.org/hardware/DS-2CD2042WD-I
          onvif://www.onvif.org/name/Hikvision
          onvif://www.onvif.org/location/country/India
        </d:Scopes>
        <d:XAddrs>http://192.168.1.64:80/onvif/device_service</d:XAddrs>
        <d:MetadataVersion>1</d:MetadataVersion>
      </d:ProbeMatch>
    </d:ProbeMatches>
  </soap:Body>
</soap:Envelope>`

	matches, err := ParseProbeMatches([]byte(hikvisionXML))
	if err != nil {
		t.Fatalf("Failed to parse Hikvision XML: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("Expected 1 match, got %d", len(matches))
	}

	m := matches[0]
	if m.EndpointReference != "urn:uuid:48494b56-3030-3030-3030-bc5451a2b3c4" {
		t.Errorf("Unexpected EndpointReference: %s", m.EndpointReference)
	}
	if m.Manufacturer != "Hikvision" {
		t.Errorf("Expected Hikvision manufacturer, got %s", m.Manufacturer)
	}
	if m.Model != "DS-2CD2042WD-I" {
		t.Errorf("Expected DS-2CD2042WD-I model, got %s", m.Model)
	}
	if m.IP != "192.168.1.64" {
		t.Errorf("Expected IP 192.168.1.64, got %s", m.IP)
	}
	if m.Port != 80 {
		t.Errorf("Expected port 80, got %d", m.Port)
	}
}

func TestParseProbeMatches_MalformedXML(t *testing.T) {
	malformed := `<?xml version="1.0"?><soap:Envelope><unclosed-tag>`
	matches, err := ParseProbeMatches([]byte(malformed))
	if err == nil && len(matches) > 0 {
		t.Fatalf("Expected error or empty matches for malformed XML, got %d matches", len(matches))
	}
}

func TestProbeONVIF_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	start := time.Now()
	matches, err := ProbeONVIF(ctx, WSDiscoveryOptions{Timeout: 5 * time.Second})
	duration := time.Since(start)

	if err != nil {
		t.Logf("ProbeONVIF cancelled with err: %v", err)
	}
	if duration > 1*time.Second {
		t.Errorf("ProbeONVIF took too long to cancel: %v", duration)
	}
	if len(matches) > 0 {
		t.Logf("Found %d matches during immediate cancel test", len(matches))
	}
}
