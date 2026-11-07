package hikvision

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"cctv-sentinel/pkg/models"
)

func TestHikvisionAuthenticateSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ISAPI/System/deviceInfo" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<DeviceInfo version="2.0">
	<model>DS-2CD2043G2-I</model>
	<serialNumber>DS-2CD2043G2-I20231015AAWR123456789</serialNumber>
	<firmwareVersion>V5.7.1 build 230915</firmwareVersion>
	<deviceType>IPCamera</deviceType>
</DeviceInfo>`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	parts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	ip := parts[0]
	port, _ := strconv.Atoi(parts[1])

	dev := &models.Device{
		ID:           "test-hik",
		IP:           ip,
		Port:         port,
		Username:     "admin",
		Password:     "Secret123",
		Manufacturer: models.VendorHikvision,
		Category:     models.CategoryIPCamera,
	}

	adapter := NewHikvisionAdapter()
	authStatus, err := adapter.Authenticate(dev)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if !authStatus.Authenticated {
		t.Errorf("expected Authenticated=true")
	}
	if authStatus.Model != "DS-2CD2043G2-I" {
		t.Errorf("expected model DS-2CD2043G2-I, got %s", authStatus.Model)
	}
	if authStatus.Firmware != "V5.7.1 build 230915" {
		t.Errorf("expected firmware V5.7.1, got %s", authStatus.Firmware)
	}
}

func TestHikvisionAuthenticate401Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="Hikvision", nonce="123456", qop="auth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	parts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	ip := parts[0]
	port, _ := strconv.Atoi(parts[1])

	dev := &models.Device{
		ID:           "test-hik-fail",
		IP:           ip,
		Port:         port,
		Username:     "wronguser",
		Password:     "wrongpass",
		Manufacturer: models.VendorHikvision,
	}

	adapter := NewHikvisionAdapter()
	authStatus, err := adapter.Authenticate(dev)
	if err == nil {
		t.Fatalf("expected error on 401 unauthorized")
	}
	if authStatus.Authenticated {
		t.Errorf("expected Authenticated=false")
	}
}

func TestHikvisionGetCapabilities(t *testing.T) {
	dev := &models.Device{
		ID:           "test-hik-caps",
		Category:     models.CategoryIPCamera,
		Manufacturer: models.VendorHikvision,
	}
	adapter := NewHikvisionAdapter()
	caps, err := adapter.GetCapabilities(dev)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if caps.DeviceManagement != models.CapabilitySupported {
		t.Errorf("expected DeviceManagement supported")
	}
	if caps.Media != models.CapabilitySupported {
		t.Errorf("expected Media supported")
	}
	if caps.AnalogFormat != models.CapabilityUnsupported {
		t.Errorf("expected AnalogFormat unsupported for IP camera")
	}
}
