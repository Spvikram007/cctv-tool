package adapters_test

import (
	"testing"

	"cctv-sentinel/pkg/adapters/dahua"
	"cctv-sentinel/pkg/adapters/hikvision"
	"cctv-sentinel/pkg/adapters/onvif"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/simulator"
)

func TestAdaptersWithSimulator(t *testing.T) {
	// 1. Launch simulator
	sim := simulator.NewSimulator()
	if err := sim.Start(); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer sim.Stop()

	// 2. Test Hikvision Adapter against simulated ISAPI endpoint (18081)
	hikAdapter := hikvision.NewHikvisionAdapter()
	devHik, err := hikAdapter.Detect("127.0.0.1", 18081, "admin", "Admin@123")
	if err != nil {
		t.Fatalf("Hikvision Detect failed: %v", err)
	}
	if devHik.Manufacturer != models.VendorHikvision {
		t.Errorf("Expected Hikvision manufacturer, got %s", devHik.Manufacturer)
	}

	netHik, err := hikAdapter.GetNetworkConfig(devHik)
	if err != nil {
		t.Fatalf("Hikvision GetNetworkConfig failed: %v", err)
	}
	if netHik.IPAddress != "127.0.0.1" {
		t.Errorf("Expected 127.0.0.1, got %s", netHik.IPAddress)
	}

	imgHik, err := hikAdapter.GetImageConfig(devHik)
	if err != nil {
		t.Fatalf("Hikvision GetImageConfig failed: %v", err)
	}
	if imgHik.Brightness != 55 {
		t.Errorf("Expected brightness 55, got %d", imgHik.Brightness)
	}

	// 3. Test Dahua Adapter against simulated CGI endpoint (18082)
	dahuaAdapter := dahua.NewDahuaAdapter()
	devDahua, err := dahuaAdapter.Detect("127.0.0.1", 18082, "admin", "Admin@123")
	if err != nil {
		t.Fatalf("Dahua Detect failed: %v", err)
	}
	if devDahua.Manufacturer != models.VendorDahua {
		t.Errorf("Expected Dahua manufacturer, got %s", devDahua.Manufacturer)
	}

	// Test TVI/CVI/AHD/CVBS analog channel format switch
	err = dahuaAdapter.SetChannelAnalogMode(devDahua, 1, "TVI")
	if err != nil {
		t.Fatalf("Dahua SetChannelAnalogMode failed: %v", err)
	}

	// 4. Test ONVIF Adapter against simulated ONVIF endpoint (18083)
	onvifAdapter := onvif.NewONVIFAdapter()
	devOnvif, err := onvifAdapter.Detect("127.0.0.1", 18083, "admin", "Admin@123")
	if err != nil {
		t.Fatalf("ONVIF Detect failed: %v", err)
	}
	if devOnvif.Manufacturer != "CP PLUS" {
		t.Errorf("Expected CP PLUS in simulated ONVIF SOAP info, got %s", devOnvif.Manufacturer)
	}

	// 5. Test PTZ commands
	err = onvifAdapter.SendPTZ(devOnvif, models.PTZAction{Command: "UP", Speed: 0.5})
	if err != nil {
		t.Fatalf("ONVIF SendPTZ failed: %v", err)
	}
}
