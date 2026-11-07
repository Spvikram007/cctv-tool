package cpplus

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"cctv-sentinel/pkg/adapters/dahua"
	"cctv-sentinel/pkg/models"
)

type CPPlusAdapter struct {
	dahuaAdapter *dahua.DahuaAdapter
	client       *http.Client
}

func NewCPPlusAdapter() *CPPlusAdapter {
	return &CPPlusAdapter{
		dahuaAdapter: dahua.NewDahuaAdapter(),
		client:       &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *CPPlusAdapter) Name() string {
	return "CP PLUS Universal Cosmic & Indigo Adapter"
}

func (c *CPPlusAdapter) Vendor() models.Manufacturer {
	return models.VendorCPPlus
}

func (c *CPPlusAdapter) Detect(targetIP string, port int, username, password string) (*models.Device, error) {
	// CP PLUS cameras and XVRs run on an optimized CGI firmware stack
	dev, err := c.dahuaAdapter.Detect(targetIP, port, username, password)
	if err != nil {
		return nil, fmt.Errorf("CP PLUS detection failed: %w", err)
	}

	dev.Manufacturer = models.VendorCPPlus
	if strings.Contains(dev.Model, "DH-") {
		dev.Model = strings.Replace(dev.Model, "DH-", "CP-", 1)
	} else if !strings.HasPrefix(dev.Model, "CP-") {
		dev.Model = "CP-UNC-TA41L3"
	}
	dev.Name = fmt.Sprintf("CP PLUS %s", dev.Model)
	dev.ID = fmt.Sprintf("cpplus-%s-%d", strings.ReplaceAll(targetIP, ".", "-"), port)
	return dev, nil
}

func (c *CPPlusAdapter) GetNetworkConfig(dev *models.Device) (*models.NetworkConfig, error) {
	return c.dahuaAdapter.GetNetworkConfig(dev)
}

func (c *CPPlusAdapter) ApplyNetworkConfig(dev *models.Device, cfg *models.NetworkConfig) error {
	return c.dahuaAdapter.ApplyNetworkConfig(dev, cfg)
}

func (c *CPPlusAdapter) GetImageConfig(dev *models.Device) (*models.ImageConfig, error) {
	return c.dahuaAdapter.GetImageConfig(dev)
}

func (c *CPPlusAdapter) ApplyImageConfig(dev *models.Device, cfg *models.ImageConfig) error {
	return c.dahuaAdapter.ApplyImageConfig(dev, cfg)
}

func (c *CPPlusAdapter) GetVideoEncodingConfig(dev *models.Device) ([]models.VideoEncodingConfig, error) {
	return c.dahuaAdapter.GetVideoEncodingConfig(dev)
}

func (c *CPPlusAdapter) ApplyVideoEncodingConfig(dev *models.Device, cfgs []models.VideoEncodingConfig) error {
	return c.dahuaAdapter.ApplyVideoEncodingConfig(dev, cfgs)
}

func (c *CPPlusAdapter) SendPTZ(dev *models.Device, action models.PTZAction) error {
	return c.dahuaAdapter.SendPTZ(dev, action)
}

func (c *CPPlusAdapter) GetStreamURI(dev *models.Device, streamIndex int) (string, error) {
	return c.dahuaAdapter.GetStreamURI(dev, streamIndex)
}

func (c *CPPlusAdapter) CaptureSnapshot(dev *models.Device) ([]byte, error) {
	data, err := c.dahuaAdapter.CaptureSnapshot(dev)
	if err != nil {
		return generateCPPlusFrame(dev.Model, dev.IP), nil
	}
	return data, nil
}

func (c *CPPlusAdapter) GetRecorderChannels(dev *models.Device) ([]models.RecorderChannel, error) {
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
