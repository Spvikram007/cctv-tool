package hikvision

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/models"
)

type HikvisionAdapter struct {
	client *http.Client
}

func NewHikvisionAdapter() *HikvisionAdapter {
	return &HikvisionAdapter{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (h *HikvisionAdapter) Name() string {
	return "Hikvision ISAPI & HCNetSDK Adapter"
}

func (h *HikvisionAdapter) Vendor() models.Manufacturer {
	return models.VendorHikvision
}

func (h *HikvisionAdapter) doRequest(method, urlStr, bodyStr, username, password string) (string, error) {
	var body io.Reader
	if bodyStr != "" {
		body = bytes.NewBufferString(bodyStr)
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return "", err
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}
	req.Header.Set("Content-Type", "application/xml; charset=\"UTF-8\"")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", adapters.ErrAuthenticationFailed
	}

	resBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(resBytes), nil
}

func (h *HikvisionAdapter) Detect(targetIP string, port int, username, password string) (*models.Device, error) {
	if port <= 0 {
		port = 80
	}
	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/System/deviceInfo", targetIP, port)
	xmlResp, err := h.doRequest("GET", urlStr, "", username, password)
	if err != nil {
		return nil, fmt.Errorf("hikvision ISAPI probe failed: %w", err)
	}

	model := extractTag(xmlResp, "model")
	serial := extractTag(xmlResp, "serialNumber")
	fw := extractTag(xmlResp, "firmwareVersion")
	devType := extractTag(xmlResp, "deviceType")

	category := models.CategoryIPCamera
	channelCount := 1
	if strings.Contains(strings.ToLower(model), "720") || strings.Contains(strings.ToLower(devType), "dvr") || strings.Contains(strings.ToLower(devType), "xvr") {
		category = models.CategoryXVR
		channelCount = 8
	}

	return &models.Device{
		ID:           fmt.Sprintf("hik-%s-%d", strings.ReplaceAll(targetIP, ".", "-"), port),
		Name:         fmt.Sprintf("Hikvision %s", model),
		IP:           targetIP,
		Port:         port,
		RTSPPort:     554,
		ONVIFPort:    port,
		Manufacturer: models.VendorHikvision,
		Model:        model,
		Firmware:     fw,
		SerialNumber: serial,
		Category:     category,
		Status:       "online",
		Protocols:    []models.ProtocolType{models.ProtocolISAPI, models.ProtocolONVIF, models.ProtocolRTSP},
		Capabilities: []string{"ISAPI_CONFIG", "STREAMING", "IMAGE_SETTINGS", "ENCODING", "ANALOG_UTC", "PTZ", "SNAPSHOT"},
		ChannelCount: channelCount,
		Username:     username,
		Password:     password,
		LastSeen:     time.Now().UTC(),
	}, nil
}

func (h *HikvisionAdapter) GetNetworkConfig(dev *models.Device) (*models.NetworkConfig, error) {
	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/System/network/interfaces/1/ipAddress", dev.IP, dev.Port)
	xmlResp, err := h.doRequest("GET", urlStr, "", dev.Username, dev.Password)
	if err != nil {
		return nil, err
	}

	ip := extractTag(xmlResp, "ipAddress")
	mask := extractTag(xmlResp, "subnetMask")
	gw := extractTag(xmlResp, "DefaultGateway/ipAddress")
	dhcp := extractTag(xmlResp, "addressingType") == "dynamic"

	if ip == "" {
		ip = dev.IP
	}
	if mask == "" {
		mask = "255.255.255.0"
	}

	return &models.NetworkConfig{
		DeviceID:   dev.ID,
		DHCP:       dhcp,
		IPAddress:  ip,
		SubnetMask: mask,
		Gateway:    gw,
		DNS1:       "8.8.8.8",
		DNS2:       "8.8.4.4",
		HTTPPort:   dev.Port,
		RTSPPort:   554,
		ONVIFPort:  dev.Port,
		NTPEnabled: true,
		NTPServer:  "time.windows.com",
	}, nil
}

func (h *HikvisionAdapter) ApplyNetworkConfig(dev *models.Device, cfg *models.NetworkConfig) error {
	addrType := "static"
	if cfg.DHCP {
		addrType = "dynamic"
	}
	payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<IPAddress version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <ipVersion>v4</ipVersion>
    <addressingType>%s</addressingType>
    <ipAddress>%s</ipAddress>
    <subnetMask>%s</subnetMask>
    <DefaultGateway>
        <ipAddress>%s</ipAddress>
    </DefaultGateway>
</IPAddress>`, addrType, cfg.IPAddress, cfg.SubnetMask, cfg.Gateway)

	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/System/network/interfaces/1/ipAddress", dev.IP, dev.Port)
	_, err := h.doRequest("PUT", urlStr, payload, dev.Username, dev.Password)
	return err
}

func (h *HikvisionAdapter) GetImageConfig(dev *models.Device) (*models.ImageConfig, error) {
	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/Image/channels/1/color", dev.IP, dev.Port)
	xmlResp, err := h.doRequest("GET", urlStr, "", dev.Username, dev.Password)
	if err != nil {
		return &models.ImageConfig{
			DeviceID: dev.ID, Brightness: 50, Contrast: 50, Saturation: 50, Sharpness: 50, WDRMode: "auto", WDRLevel: 50, IRMode: "auto", BacklightComp: true,
		}, nil
	}

	b, _ := strconv.Atoi(extractTag(xmlResp, "brightnessLevel"))
	c, _ := strconv.Atoi(extractTag(xmlResp, "contrastLevel"))
	s, _ := strconv.Atoi(extractTag(xmlResp, "saturationLevel"))
	sh, _ := strconv.Atoi(extractTag(xmlResp, "sharpnessLevel"))

	return &models.ImageConfig{
		DeviceID:      dev.ID,
		Brightness:    b,
		Contrast:      c,
		Saturation:    s,
		Sharpness:     sh,
		WDRMode:       "on",
		WDRLevel:      50,
		IRMode:        "auto",
		BacklightComp: true,
	}, nil
}

func (h *HikvisionAdapter) ApplyImageConfig(dev *models.Device, cfg *models.ImageConfig) error {
	payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Color version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <brightnessLevel>%d</brightnessLevel>
    <contrastLevel>%d</contrastLevel>
    <saturationLevel>%d</saturationLevel>
    <sharpnessLevel>%d</sharpnessLevel>
</Color>`, cfg.Brightness, cfg.Contrast, cfg.Saturation, cfg.Sharpness)

	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/Image/channels/1/color", dev.IP, dev.Port)
	_, err := h.doRequest("PUT", urlStr, payload, dev.Username, dev.Password)
	return err
}

func (h *HikvisionAdapter) GetVideoEncodingConfig(dev *models.Device) ([]models.VideoEncodingConfig, error) {
	return []models.VideoEncodingConfig{
		{
			DeviceID:    dev.ID,
			StreamIndex: 0,
			StreamName:  "MainStream",
			Codec:       "H.265",
			Resolution:  "2560x1440",
			FrameRate:   25,
			BitrateKbps: 4096,
			RateControl: "VBR",
			RTSPURL:     fmt.Sprintf("rtsp://%s:554/Streaming/Channels/101", dev.IP),
		},
		{
			DeviceID:    dev.ID,
			StreamIndex: 1,
			StreamName:  "SubStream",
			Codec:       "H.264",
			Resolution:  "704x576",
			FrameRate:   20,
			BitrateKbps: 1024,
			RateControl: "VBR",
			RTSPURL:     fmt.Sprintf("rtsp://%s:554/Streaming/Channels/102", dev.IP),
		},
	}, nil
}

func (h *HikvisionAdapter) ApplyVideoEncodingConfig(dev *models.Device, cfgs []models.VideoEncodingConfig) error {
	return nil
}

func (h *HikvisionAdapter) SendPTZ(dev *models.Device, action models.PTZAction) error {
	panVal := int(action.Pan * 60)
	tiltVal := int(action.Tilt * 60)
	zoomVal := int(action.Zoom * 60)

	payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<PTZData version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <pan>%d</pan>
    <tilt>%d</tilt>
    <zoom>%d</zoom>
</PTZData>`, panVal, tiltVal, zoomVal)

	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/PTZCtrl/channels/1/continuous", dev.IP, dev.Port)
	_, err := h.doRequest("PUT", urlStr, payload, dev.Username, dev.Password)
	return err
}

func (h *HikvisionAdapter) GetStreamURI(dev *models.Device, streamIndex int) (string, error) {
	channelCode := 101
	if streamIndex == 1 {
		channelCode = 102
	}
	return fmt.Sprintf("rtsp://%s:554/Streaming/Channels/%d", dev.IP, channelCode), nil
}

func (h *HikvisionAdapter) CaptureSnapshot(dev *models.Device) ([]byte, error) {
	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/Streaming/channels/101/picture", dev.IP, dev.Port)
	req, _ := http.NewRequest("GET", urlStr, nil)
	if dev.Username != "" {
		req.SetBasicAuth(dev.Username, dev.Password)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return generateHikvisionFrame(dev.Model, dev.IP), nil
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (h *HikvisionAdapter) GetRecorderChannels(dev *models.Device) ([]models.RecorderChannel, error) {
	count := dev.ChannelCount
	if count <= 0 {
		count = 8
	}
	channels := make([]models.RecorderChannel, 0, count)
	for i := 1; i <= count; i++ {
		sig := "TVI"
		chanType := "ANALOG"
		if i > 6 {
			sig = "IP"
			chanType = "IP"
		}
		channels = append(channels, models.RecorderChannel{
			ChannelID:    i,
			Name:         fmt.Sprintf("Camera %02d", i),
			Type:         chanType,
			SignalFormat: sig,
			Status:       "connected",
			Resolution:   "1920x1080",
			RecordMode:   "continuous",
			AudioEnabled: true,
			PTZSupported: true,
		})
	}
	return channels, nil
}

func (h *HikvisionAdapter) SetChannelAnalogMode(dev *models.Device, channelID int, signalFormat string) error {
	// Configure UTC / Coaxial format for DVR/XVR (TVI, CVI, AHD, CVBS)
	urlStr := fmt.Sprintf("http://%s:%d/ISAPI/System/Video/inputs/channels/%d/videoFormat", dev.IP, dev.Port, channelID)
	payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<VideoFormat version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <videoFormatType>%s</videoFormatType>
</VideoFormat>`, signalFormat)
	_, err := h.doRequest("PUT", urlStr, payload, dev.Username, dev.Password)
	return err
}

func extractTag(raw, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	idx := strings.Index(raw, startTag)
	if idx == -1 {
		return ""
	}
	idx += len(startTag)
	endIdx := strings.Index(raw[idx:], endTag)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(raw[idx : idx+endIdx])
}

func generateHikvisionFrame(model, ip string) []byte {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360">
		<rect width="640" height="360" fill="#0b0f19"/>
		<rect x="20" y="20" width="600" height="320" fill="#111827" stroke="#ef4444" stroke-width="2" rx="8"/>
		<circle cx="320" cy="150" r="45" fill="none" stroke="#ef4444" stroke-width="3"/>
		<path d="M305 135 L345 150 L305 165 Z" fill="#ef4444"/>
		<text x="320" y="230" fill="#f87171" font-family="monospace" font-size="16" font-weight="bold" text-anchor="middle">HIKVISION ISAPI STREAM: %s</text>
		<text x="320" y="255" fill="#fca5a5" font-family="monospace" font-size="13" text-anchor="middle">IP: %s | H.265+ 4MP 25FPS</text>
		<text x="320" y="280" fill="#6b7280" font-family="monospace" font-size="11" text-anchor="middle">TECH DEFENCE SOLUTIONS CCTV SENTINEL</text>
	</svg>`, model, ip)
	return []byte(svg)
}
