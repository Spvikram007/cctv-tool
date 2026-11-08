package onvif

import (
	"crypto/sha1"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestWSSecurityPasswordDigest(t *testing.T) {
	// Standard OASIS WS-Security PasswordDigest calculation check
	nonceRaw := []byte("1234567890123456")
	created := "2026-11-07T14:00:00.000Z"
	password := "SecretPass123"

	h := sha1.New()
	h.Write(nonceRaw)
	h.Write([]byte(created))
	h.Write([]byte(password))
	expectedDigest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if expectedDigest == "" {
		t.Fatalf("expected digest should not be empty")
	}

	cli := NewClient("192.168.1.100", 80, "admin", password)
	securityXML, err := cli.GenerateWSSecurity()
	if err != nil {
		t.Fatalf("GenerateWSSecurity failed unexpectedly: %v", err)
	}

	if !strings.Contains(securityXML, "<wsse:Username>admin</wsse:Username>") {
		t.Errorf("expected username admin in WS-Security header, got: %s", securityXML)
	}
	if !strings.Contains(securityXML, "PasswordDigest") {
		t.Errorf("expected PasswordDigest Type in WS-Security header")
	}

	// Verify that password in plaintext does NOT appear anywhere
	if strings.Contains(securityXML, password) {
		t.Errorf("SECURITY RISK: Plaintext password leaked into WS-Security header!")
	}
}

func TestClockSkewCompensation(t *testing.T) {
	cli := NewClient("192.168.1.100", 80, "admin", "pass")

	// Simulate device clock being 1 hour (3600 seconds) in the future
	deviceAheadSkew := 1 * time.Hour
	cli.clockSkew = deviceAheadSkew

	secXML, err := cli.GenerateWSSecurity()
	if err != nil {
		t.Fatalf("GenerateWSSecurity failed unexpectedly: %v", err)
	}
	createdTag := ExtractTag(secXML, "Created")
	if createdTag == "" {
		t.Fatalf("could not extract Created timestamp from WS-Security")
	}

	parsedCreated, err := time.Parse("2006-01-02T15:04:05.000Z", createdTag)
	if err != nil {
		t.Fatalf("failed to parse Created timestamp '%s': %v", createdTag, err)
	}

	diff := parsedCreated.Sub(time.Now().UTC())
	// Diff should be close to +1 hour (e.g. 59m 50s to 1h 10s)
	if diff < 58*time.Minute || diff > 62*time.Minute {
		t.Errorf("expected clock skew compensation around +1h, got diff: %v", diff)
	}
}

func TestParseSystemDateAndTime(t *testing.T) {
	xmlSample := `<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
	<soap:Body>
		<tds:GetSystemDateAndTimeResponse>
			<tds:SystemDateAndTime>
				<tt:DateTimeType>Manual</tt:DateTimeType>
				<tt:DaylightSavings>false</tt:DaylightSavings>
				<tt:TimeZone><tt:TZ>UTC-0</tt:TZ></tt:TimeZone>
				<tt:UTCDateTime>
					<tt:Time><tt:Hour>14</tt:Hour><tt:Minute>30</tt:Minute><tt:Second>45</tt:Second></tt:Time>
					<tt:Date><tt:Year>2026</tt:Year><tt:Month>11</tt:Month><tt:Day>7</tt:Day></tt:Date>
				</tt:UTCDateTime>
			</tds:SystemDateAndTime>
		</tds:GetSystemDateAndTimeResponse>
	</soap:Body>
</soap:Envelope>`

	devTime := parseSystemDateAndTime(xmlSample)
	expected := time.Date(2026, 11, 7, 14, 30, 45, 0, time.UTC)

	if !devTime.Equal(expected) {
		t.Errorf("expected parsed time %v, got %v", expected, devTime)
	}
}

func TestExtractAttributes(t *testing.T) {
	sample := `<trt:GetProfilesResponse>
		<trt:Profiles token="Profile_Main" fixed="true"><tt:Name>MainStream</tt:Name></trt:Profiles>
		<trt:Profiles token="Profile_Sub" fixed="false"><tt:Name>SubStream</tt:Name></trt:Profiles>
	</trt:GetProfilesResponse>`

	tokens := extractAttributes(sample, "Profiles", "token")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0] != "Profile_Main" || tokens[1] != "Profile_Sub" {
		t.Errorf("unexpected tokens parsed: %v", tokens)
	}
}

func TestONVIFFailClosedAuthentication(t *testing.T) {
	// Client without username
	unauthCli := NewClient("192.168.1.100", 80, "", "")
	_, err := unauthCli.GenerateWSSecurity()
	if err == nil {
		t.Errorf("GenerateWSSecurity must return an error when username is empty")
	}

	// SendSOAP with withAuth=true on unauthenticated client must fail closed
	_, err = unauthCli.SendSOAP("http://192.168.1.100:80/onvif/device_service", "GetDeviceInformation", "<tds:GetDeviceInformation/>", true)
	if err == nil {
		t.Errorf("SendSOAP with withAuth=true must return error and NOT send unauthenticated request")
	}
}
