package dahua

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"cctv-sentinel/pkg/models"
)

func TestDahuaAuthenticateSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "magicBox.cgi") {
			fmt.Fprintf(w, "deviceType=DHI-XVR5108HS-4KL-I3\nserialNumber=9A038E8PAG10082\nappVersion=4.001.0000001.0\n")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	parts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	ip := parts[0]
	port, _ := strconv.Atoi(parts[1])

	dev := &models.Device{
		ID:           "test-dahua",
		IP:           ip,
		Port:         port,
		Username:     "admin",
		Password:     "Secret123",
		Manufacturer: models.VendorDahua,
		Category:     models.CategoryXVR,
	}

	adapter := NewDahuaAdapter()
	authStatus, err := adapter.Authenticate(dev)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if !authStatus.Authenticated {
		t.Errorf("expected Authenticated=true")
	}
	if authStatus.Model != "DHI-XVR5108HS-4KL-I3" {
		t.Errorf("expected model DHI-XVR5108HS-4KL-I3, got %s", authStatus.Model)
	}
	if authStatus.Firmware != "4.001.0000001.0" {
		t.Errorf("expected firmware 4.001.0000001.0, got %s", authStatus.Firmware)
	}
}

func TestDahuaAuthenticate401(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="Login to Dahua", nonce="abcdef", qop="auth"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	parts := strings.Split(strings.TrimPrefix(ts.URL, "http://"), ":")
	ip := parts[0]
	port, _ := strconv.Atoi(parts[1])

	dev := &models.Device{
		ID:           "test-dahua-fail",
		IP:           ip,
		Port:         port,
		Username:     "wronguser",
		Password:     "wrongpass",
		Manufacturer: models.VendorDahua,
	}

	adapter := NewDahuaAdapter()
	authStatus, err := adapter.Authenticate(dev)
	if err == nil {
		t.Fatalf("expected error on 401 unauthorized")
	}
	if authStatus.Authenticated {
		t.Errorf("expected Authenticated=false")
	}
}

func TestDahuaGetCapabilities(t *testing.T) {
	dev := &models.Device{
		ID:           "test-dahua-caps",
		Category:     models.CategoryXVR,
		Manufacturer: models.VendorDahua,
	}
	adapter := NewDahuaAdapter()
	caps, err := adapter.GetCapabilities(dev)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if caps.AnalogFormat != models.CapabilitySupported {
		t.Errorf("expected AnalogFormat supported for XVR")
	}
}
