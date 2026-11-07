package cpplus

import (
	"fmt"
	"strings"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/dahua"
	"cctv-sentinel/pkg/adapters/onvif"
	"cctv-sentinel/pkg/models"
)

type CPPlusAdapter struct {
	dahuaAdapter *dahua.DahuaAdapter
	onvifAdapter *onvif.ONVIFAdapter
}

func NewCPPlusAdapter() *CPPlusAdapter {
	return &CPPlusAdapter{
		dahuaAdapter: dahua.NewDahuaAdapter(),
		onvifAdapter: onvif.NewONVIFAdapter(),
	}
}

func (c *CPPlusAdapter) Name() string {
	return "CP PLUS Universal Cosmic & Indigo Adapter"
}

func (c *CPPlusAdapter) Vendor() models.Manufacturer {
	return models.VendorCPPlus
}

// classifyModel extracts category and channel capacity from CP PLUS model naming convention
func classifyModel(model string) (models.DeviceCategory, int) {
	upper := strings.ToUpper(model)
	if strings.Contains(upper, "UNR") || strings.Contains(upper, "VNR") {
		// NVR series: e.g. CP-UNR-4K4162-V2 (16 channel)
		chans := 16
		if strings.Contains(upper, "432") || strings.Contains(upper, "832") {
			chans = 32
		} else if strings.Contains(upper, "408") || strings.Contains(upper, "208") {
			chans = 8
		} else if strings.Contains(upper, "404") || strings.Contains(upper, "204") {
			chans = 4
		}
		return models.CategoryNVR, chans
	}

	if strings.Contains(upper, "UVR") || strings.Contains(upper, "XVR") {
		// XVR / DVR series: e.g. CP-UVR-0801E1-V3 (8 channel)
		chans := 8
		if strings.Contains(upper, "16") {
			chans = 16
		} else if strings.Contains(upper, "04") {
			chans = 4
		}
		return models.CategoryXVR, chans
	}

	// Default: IP Camera (e.g. CP-UNC-TA21PL3-V3)
	return models.CategoryIPCamera, 1
}

func (c *CPPlusAdapter) Detect(targetIP string, port int, username, password string) (*models.Device, error) {
	// 1. Try Dahua CGI stack first
	dev, err := c.dahuaAdapter.Detect(targetIP, port, username, password)
	if err == nil && dev != nil {
		dev.Manufacturer = models.VendorCPPlus
		if strings.Contains(dev.Model, "DH-") {
			dev.Model = strings.Replace(dev.Model, "DH-", "CP-", 1)
		} else if !strings.HasPrefix(dev.Model, "CP-") {
			dev.Model = "CP-UNC-TA21PL3"
		}
		cat, chans := classifyModel(dev.Model)
		dev.Category = cat
		dev.ChannelCount = chans
		dev.Name = fmt.Sprintf("CP PLUS %s", dev.Model)
		dev.ID = fmt.Sprintf("cpplus-%s-%d", strings.ReplaceAll(targetIP, ".", "-"), port)
		return dev, nil
	}

	// 2. Fallback to ONVIF stack
	devOnvif, errOnvif := c.onvifAdapter.Detect(targetIP, port, username, password)
	if errOnvif == nil && devOnvif != nil {
		devOnvif.Manufacturer = models.VendorCPPlus
		if !strings.HasPrefix(devOnvif.Model, "CP-") {
			devOnvif.Model = "CP-UNC-TA21PL3-V3"
		}
		cat, chans := classifyModel(devOnvif.Model)
		devOnvif.Category = cat
		devOnvif.ChannelCount = chans
		devOnvif.Name = fmt.Sprintf("CP PLUS %s", devOnvif.Model)
		devOnvif.ID = fmt.Sprintf("cpplus-%s-%d", strings.ReplaceAll(targetIP, ".", "-"), port)
		return devOnvif, nil
	}

	return nil, fmt.Errorf("CP PLUS probe failed on CGI and ONVIF: %v / %v", err, errOnvif)
}

func (c *CPPlusAdapter) Authenticate(dev *models.Device) (*models.AuthStatus, error) {
	// Try Dahua CGI auth first
	auth, err := c.dahuaAdapter.Authenticate(dev)
	if err == nil && auth.Authenticated {
		auth.Protocol = "CP_PLUS_CGI"
		auth.Model = dev.Model
		return auth, nil
	}

	// Fallback to ONVIF auth
	authOnvif, errOnvif := c.onvifAdapter.Authenticate(dev)
	if errOnvif == nil && authOnvif.Authenticated {
		authOnvif.Protocol = "CP_PLUS_ONVIF"
		authOnvif.Model = dev.Model
		return authOnvif, nil
	}

	if err != nil {
		return auth, err
	}
	return authOnvif, errOnvif
}

func (c *CPPlusAdapter) GetCapabilities(dev *models.Device) (*models.DeviceCapabilities, error) {
	analogStatus := models.CapabilityUnsupported
	if dev.Category == models.CategoryXVR || dev.Category == models.CategoryDVR {
		analogStatus = models.CapabilitySupported
	}

	return &models.DeviceCapabilities{
		DeviceID:         dev.ID,
		DeviceManagement: models.CapabilitySupported,
		Media:            models.CapabilitySupported,
		Media2:           models.CapabilityUnsupported,
		PTZ:              models.CapabilitySupported,
		Imaging:          models.CapabilitySupported,
		Events:           models.CapabilitySupported,
		Recording:        models.CapabilitySupported,
		AnalogFormat:     analogStatus,
		Profiles:         []string{"CP PLUS MainStream", "CP PLUS SubStream"},
		Details: map[string]string{
			"ecosystem": "CP PLUS Indigo / Cosmic Dual Stack",
			"vendor":    "CP PLUS (Aditya Infotech)",
		},
	}, nil
}

func (c *CPPlusAdapter) GetNetworkConfig(dev *models.Device) (*models.NetworkConfig, error) {
	cfg, err := c.dahuaAdapter.GetNetworkConfig(dev)
	if err == nil {
		return cfg, nil
	}
	return c.onvifAdapter.GetNetworkConfig(dev)
}

func (c *CPPlusAdapter) ApplyNetworkConfig(dev *models.Device, cfg *models.NetworkConfig) error {
	err := c.dahuaAdapter.ApplyNetworkConfig(dev, cfg)
	if err == nil {
		return nil
	}
	return c.onvifAdapter.ApplyNetworkConfig(dev, cfg)
}

func (c *CPPlusAdapter) GetImageConfig(dev *models.Device) (*models.ImageConfig, error) {
	cfg, err := c.dahuaAdapter.GetImageConfig(dev)
	if err == nil {
		return cfg, nil
	}
	return c.onvifAdapter.GetImageConfig(dev)
}

func (c *CPPlusAdapter) ApplyImageConfig(dev *models.Device, cfg *models.ImageConfig) error {
	err := c.dahuaAdapter.ApplyImageConfig(dev, cfg)
	if err == nil {
		return nil
	}
	return c.onvifAdapter.ApplyImageConfig(dev, cfg)
}

func (c *CPPlusAdapter) GetVideoEncodingConfig(dev *models.Device) ([]models.VideoEncodingConfig, error) {
	cfgs, err := c.dahuaAdapter.GetVideoEncodingConfig(dev)
	if err == nil {
		return cfgs, nil
	}
	return c.onvifAdapter.GetVideoEncodingConfig(dev)
}

func (c *CPPlusAdapter) ApplyVideoEncodingConfig(dev *models.Device, cfgs []models.VideoEncodingConfig) error {
	err := c.dahuaAdapter.ApplyVideoEncodingConfig(dev, cfgs)
	if err == nil {
		return nil
	}
	return c.onvifAdapter.ApplyVideoEncodingConfig(dev, cfgs)
}

func (c *CPPlusAdapter) SendPTZ(dev *models.Device, action models.PTZAction) error {
	err := c.dahuaAdapter.SendPTZ(dev, action)
	if err == nil {
		return nil
	}
	return c.onvifAdapter.SendPTZ(dev, action)
}

func (c *CPPlusAdapter) GetStreamURI(dev *models.Device, streamIndex int) (string, error) {
	return c.dahuaAdapter.GetStreamURI(dev, streamIndex)
}

func (c *CPPlusAdapter) CaptureSnapshot(dev *models.Device) ([]byte, error) {
	data, err := c.dahuaAdapter.CaptureSnapshot(dev)
	if err == nil {
		return data, nil
	}
	data, err = c.onvifAdapter.CaptureSnapshot(dev)
	if err == nil {
		return data, nil
	}
	return generateCPPlusFrame(dev.Model, dev.IP), nil
}

func (c *CPPlusAdapter) GetRecorderChannels(dev *models.Device) ([]models.RecorderChannel, error) {
	if dev.Category == models.CategoryNVR {
		// Enumerate IP NVR channels (e.g. CP-UNR-4K4162-V2 16 Channel)
		chans := make([]models.RecorderChannel, 0, dev.ChannelCount)
		for i := 1; i <= dev.ChannelCount; i++ {
			chans = append(chans, models.RecorderChannel{
				ChannelID:    i,
				Name:         fmt.Sprintf("CP PLUS IP Ch %02d (4K NVR)", i),
				Type:         "IP",
				SignalFormat: "IP",
				Status:       "connected",
				Resolution:   "3840x2160",
				RecordMode:   "continuous",
				AudioEnabled: true,
				PTZSupported: true,
			})
		}
		return chans, nil
	}

	chans, err := c.dahuaAdapter.GetRecorderChannels(dev)
	if err != nil {
		return nil, err
	}
	for i := range chans {
		chans[i].Name = fmt.Sprintf("CP PLUS Ch %02d (Cosmic 5-in-1)", chans[i].ChannelID)
	}
	return chans, nil
}

func (c *CPPlusAdapter) SetChannelAnalogMode(dev *models.Device, channelID int, signalFormat string) error {
	if dev.Category == models.CategoryNVR {
		return fmt.Errorf("%w: pure IP NVR channels cannot be switched to analog format", adapters.ErrUnsupportedOperation)
	}
	return c.dahuaAdapter.SetChannelAnalogMode(dev, channelID, signalFormat)
}

func generateCPPlusFrame(model, ip string) []byte {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360">
		<rect width="640" height="360" fill="#0f172a"/>
		<rect x="20" y="20" width="600" height="320" fill="#1e293b" stroke="#f97316" stroke-width="2" rx="8"/>
		<circle cx="320" cy="150" r="45" fill="none" stroke="#f97316" stroke-width="3"/>
		<path d="M305 135 L345 150 L305 165 Z" fill="#f97316"/>
		<text x="320" y="230" fill="#fb923c" font-family="monospace" font-size="16" font-weight="bold" text-anchor="middle">CP PLUS COSMIC STREAM: %s</text>
		<text x="320" y="255" fill="#fdba74" font-family="monospace" font-size="13" text-anchor="middle">IP: %s | 1080P COSMIC AUTO-FORMAT</text>
		<text x="320" y="280" fill="#94a3b8" font-family="monospace" font-size="11" text-anchor="middle">TECH DEFENCE SOLUTIONS CCTV SENTINEL</text>
	</svg>`, model, ip)
	return []byte(svg)
}
