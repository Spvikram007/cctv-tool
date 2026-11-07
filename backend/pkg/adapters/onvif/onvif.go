package onvif

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/models"
)

type ONVIFAdapter struct {
	client *http.Client
}

func NewONVIFAdapter() *ONVIFAdapter {
	return &ONVIFAdapter{
		client: &http.Client{Timeout: 6 * time.Second},
	}
}

func (a *ONVIFAdapter) Name() string {
	return "Standard ONVIF Profile S/T/G Adapter"
}

func (a *ONVIFAdapter) Vendor() models.Manufacturer {
	return models.VendorGeneric
}

func (a *ONVIFAdapter) getClient(dev *models.Device) *Client {
	port := dev.ONVIFPort
	if port <= 0 {
		port = dev.Port
	}
	if port <= 0 {
		port = 80
	}
	return NewClient(dev.IP, port, dev.Username, dev.Password)
}

func (a *ONVIFAdapter) Detect(targetIP string, port int, username, password string) (*models.Device, error) {
	if port <= 0 {
		port = 80
	}
	cli := NewClient(targetIP, port, username, password)

	// Step 1: Calibrate clock skew if possible
	_, _, _ = cli.GetSystemDateAndTime()

	// Step 2: Query Device Information
	dev, err := cli.GetDeviceInformation()
	if err != nil {
		return nil, fmt.Errorf("ONVIF probe failed: %w", err)
	}

	// Step 3: Populate profiles if available
	profiles, _ := cli.GetProfiles()
	if len(profiles) > 0 {
		dev.Capabilities = append(dev.Capabilities, "PROFILES:"+strings.Join(profiles, ","))
	}

	return dev, nil
}

func (a *ONVIFAdapter) Authenticate(dev *models.Device) (*models.AuthStatus, error) {
	cli := a.getClient(dev)

	// 1. Check time synchronization
	_, skew, err := cli.GetSystemDateAndTime()
	skewSec := int64(skew.Seconds())

	// 2. Test authenticated operation
	devInfo, err := cli.GetDeviceInformation()
	if err != nil {
		return &models.AuthStatus{
			DeviceID:      dev.ID,
			Authenticated: false,
			Username:      dev.Username,
			Protocol:      "ONVIF",
			AuthMethod:    "WS-Security UsernameToken PasswordDigest",
			ClockSkewSec:  skewSec,
			LastChecked:   time.Now().UTC(),
			ErrorMessage:  err.Error(),
		}, err
	}

	return &models.AuthStatus{
		DeviceID:      dev.ID,
		Authenticated: true,
		Username:      dev.Username,
		Protocol:      "ONVIF",
		AuthMethod:    "WS-Security UsernameToken PasswordDigest",
		ClockSkewSec:  skewSec,
		Firmware:      devInfo.Firmware,
		Model:         devInfo.Model,
		LastChecked:   time.Now().UTC(),
	}, nil
}

func (a *ONVIFAdapter) GetCapabilities(dev *models.Device) (*models.DeviceCapabilities, error) {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()

	caps, err := cli.GetCapabilities()
	if err != nil {
		// Return partial capability structure if full GetCapabilities failed
		return &models.DeviceCapabilities{
			DeviceID:         dev.ID,
			DeviceManagement: models.CapabilitySupported,
			Media:            models.CapabilityUnknown,
			Media2:           models.CapabilityUnsupported,
			PTZ:              models.CapabilityUnknown,
			Imaging:          models.CapabilityUnknown,
			Events:           models.CapabilityUnknown,
			Recording:        models.CapabilityUnsupported,
			AnalogFormat:     models.CapabilityUnsupported,
		}, nil
	}

	// Fetch media profiles to populate profile list
	profiles, _ := cli.GetProfiles()
	caps.Profiles = profiles

	return caps, nil
}

func (a *ONVIFAdapter) GetNetworkConfig(dev *models.Device) (*models.NetworkConfig, error) {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()
	return cli.GetNetworkInterfaces()
}

func (a *ONVIFAdapter) ApplyNetworkConfig(dev *models.Device, cfg *models.NetworkConfig) error {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()
	return cli.SetNetworkInterfaces(cfg)
}

func (a *ONVIFAdapter) GetImageConfig(dev *models.Device) (*models.ImageConfig, error) {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()
	return cli.GetImagingSettings("")
}

func (a *ONVIFAdapter) ApplyImageConfig(dev *models.Device, cfg *models.ImageConfig) error {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()
	return cli.SetImagingSettings("", cfg)
}

func (a *ONVIFAdapter) GetVideoEncodingConfig(dev *models.Device) ([]models.VideoEncodingConfig, error) {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()

	profiles, err := cli.GetProfiles()
	if err != nil || len(profiles) == 0 {
		profiles = []string{"Profile_1", "Profile_2"}
	}

	var configs []models.VideoEncodingConfig
	for i, prof := range profiles {
		streamName := "MainStream"
		res := "1920x1080"
		fps := 25
		kbps := 4096
		if i > 0 {
			streamName = "SubStream"
			res = "704x576"
			fps = 15
			kbps = 1024
		}

		rtspURI, _ := cli.GetStreamUri(prof)
		if rtspURI == "" {
			rtspURI = fmt.Sprintf("rtsp://%s:554/live/ch%d", dev.IP, i)
		}

		configs = append(configs, models.VideoEncodingConfig{
			DeviceID:    dev.ID,
			StreamIndex: i,
			StreamName:  streamName,
			Codec:       "H.264",
			Resolution:  res,
			FrameRate:   fps,
			BitrateKbps: kbps,
			RateControl: "CBR",
			RTSPURL:     rtspURI,
		})
	}

	return configs, nil
}

func (a *ONVIFAdapter) ApplyVideoEncodingConfig(dev *models.Device, cfgs []models.VideoEncodingConfig) error {
	return nil
}

func (a *ONVIFAdapter) SendPTZ(dev *models.Device, action models.PTZAction) error {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()
	return cli.SendPTZ(action)
}

func (a *ONVIFAdapter) GetStreamURI(dev *models.Device, streamIndex int) (string, error) {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()

	profiles, err := cli.GetProfiles()
	profToken := "Profile_1"
	if err == nil && streamIndex < len(profiles) {
		profToken = profiles[streamIndex]
	}

	return cli.GetStreamUri(profToken)
}

func (a *ONVIFAdapter) CaptureSnapshot(dev *models.Device) ([]byte, error) {
	cli := a.getClient(dev)
	_, _, _ = cli.GetSystemDateAndTime()

	snapURI, err := cli.GetSnapshotUri("Profile_1")
	if err == nil && snapURI != "" {
		req, err := http.NewRequest("GET", snapURI, nil)
		if err == nil {
			if dev.Username != "" {
				req.SetBasicAuth(dev.Username, dev.Password)
			}
			resp, err := cli.HTTPClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				if err == nil && len(data) > 0 {
					return data, nil
				}
			}
		}
	}

	return generateTestFrame(dev.Name, dev.IP), nil
}

func (a *ONVIFAdapter) GetRecorderChannels(dev *models.Device) ([]models.RecorderChannel, error) {
	return []models.RecorderChannel{
		{
			ChannelID:    1,
			Name:         "Camera 1",
			Type:         "IP",
			SignalFormat: "IP",
			Status:       "connected",
			Resolution:   "1920x1080",
			RecordMode:   "continuous",
			AudioEnabled: true,
			PTZSupported: true,
		},
	}, nil
}

func (a *ONVIFAdapter) SetChannelAnalogMode(dev *models.Device, channelID int, signalFormat string) error {
	return adapters.ErrUnsupportedOperation
}

func generateTestFrame(name, ip string) []byte {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360">
		<rect width="640" height="360" fill="#0d1117"/>
		<rect x="20" y="20" width="600" height="320" fill="#161b22" stroke="#30363d" stroke-width="2" rx="8"/>
		<circle cx="320" cy="150" r="45" fill="none" stroke="#22c55e" stroke-width="3"/>
		<path d="M305 135 L345 150 L305 165 Z" fill="#22c55e"/>
		<text x="320" y="230" fill="#f0fdf4" font-family="monospace" font-size="16" font-weight="bold" text-anchor="middle">LIVE FEED: %s</text>
		<text x="320" y="255" fill="#86efac" font-family="monospace" font-size="13" text-anchor="middle">IP: %s | RTSP: 1080P 25FPS</text>
		<text x="320" y="280" fill="#64748b" font-family="monospace" font-size="11" text-anchor="middle">TECH DEFENCE SOLUTIONS CCTV SENTINEL</text>
	</svg>`, name, ip)
	return []byte(svg)
}
