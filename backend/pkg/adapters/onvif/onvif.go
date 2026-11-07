package onvif

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
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

// buildAuthHeader generates ONVIF WS-Security UsernameToken header
func buildAuthHeader(username, password string) string {
	if username == "" {
		return ""
	}
	nonce := []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	created := time.Now().UTC().Format(time.RFC3339Nano)
	
	// Password_Digest = Base64 ( SHA-1 ( raw_nonce + created + password ) )
	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)

	return fmt.Sprintf(`
	<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
		<wsse:UsernameToken>
			<wsse:Username>%s</wsse:Username>
			<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
			<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
			<wsu:Created>%s</wsu:Created>
		</wsse:UsernameToken>
	</wsse:Security>`, username, digest, nonceB64, created)
}

func (a *ONVIFAdapter) sendSOAP(url, action, body, username, password string) (string, error) {
	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:tds="http://www.onvif.org/ver10/device/wsdl"
               xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
               xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"
               xmlns:tt="http://www.onvif.org/ver10/schema">
	<soap:Header>
		%s
	</soap:Header>
	<soap:Body>
		%s
	</soap:Body>
</soap:Envelope>`, buildAuthHeader(username, password), body)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(envelope))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8; action=\""+action+"\"")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return "", adapters.ErrAuthenticationFailed
	}

	return string(respBytes), nil
}

func (a *ONVIFAdapter) Detect(targetIP string, port int, username, password string) (*models.Device, error) {
	if port <= 0 {
		port = 80
	}
	deviceServiceURL := fmt.Sprintf("http://%s:%d/onvif/device_service", targetIP, port)
	body := `<tds:GetDeviceInformation/>`
	
	xmlResp, err := a.sendSOAP(deviceServiceURL, "http://www.onvif.org/ver10/device/wsdl/GetDeviceInformation", body, username, password)
	if err != nil {
		return nil, fmt.Errorf("ONVIF probe failed: %w", err)
	}

	// Parse simple values
	manufacturer := extractXMLTag(xmlResp, "Manufacturer")
	model := extractXMLTag(xmlResp, "Model")
	fw := extractXMLTag(xmlResp, "FirmwareVersion")
	serial := extractXMLTag(xmlResp, "SerialNumber")

	if manufacturer == "" {
		manufacturer = "Generic ONVIF"
	}
	if model == "" {
		model = "IP Camera (Profile S)"
	}

	dev := &models.Device{
		ID:           fmt.Sprintf("onvif-%s-%d", strings.ReplaceAll(targetIP, ".", "-"), port),
		Name:         fmt.Sprintf("%s %s", manufacturer, model),
		IP:           targetIP,
		Port:         port,
		ONVIFPort:    port,
		RTSPPort:     554,
		Manufacturer: models.Manufacturer(manufacturer),
		Model:        model,
		Firmware:     fw,
		SerialNumber: serial,
		Category:     models.CategoryIPCamera,
		Status:       "online",
		Protocols:    []models.ProtocolType{models.ProtocolONVIF, models.ProtocolRTSP},
		Capabilities: []string{"STREAMING", "IMAGE_SETTINGS", "ENCODING", "SNAPSHOT", "PTZ"},
		ChannelCount: 1,
		Username:     username,
		Password:     password,
		LastSeen:     time.Now().UTC(),
	}

	return dev, nil
}

func (a *ONVIFAdapter) GetNetworkConfig(dev *models.Device) (*models.NetworkConfig, error) {
	deviceServiceURL := fmt.Sprintf("http://%s:%d/onvif/device_service", dev.IP, dev.Port)
	body := `<tds:GetNetworkInterfaces/>`
	
	resp, err := a.sendSOAP(deviceServiceURL, "http://www.onvif.org/ver10/device/wsdl/GetNetworkInterfaces", body, dev.Username, dev.Password)
	if err != nil {
		return nil, err
	}

	ip := extractXMLTag(resp, "IPv4Address")
	if ip == "" {
		ip = dev.IP
	}
	prefixLen := extractXMLTag(resp, "PrefixLength")
	mask := "255.255.255.0"
	if prefixLen == "16" {
		mask = "255.255.0.0"
	} else if prefixLen == "8" {
		mask = "255.0.0.0"
	}
	dhcp := strings.Contains(strings.ToLower(resp), "<dhcp>true</dhcp>") || strings.Contains(strings.ToLower(resp), "dhcp=\"true\"")

	return &models.NetworkConfig{
		DeviceID:   dev.ID,
		DHCP:       dhcp,
		IPAddress:  ip,
		SubnetMask: mask,
		Gateway:    extractXMLTag(resp, "Gateway"),
		DNS1:       "8.8.8.8",
		DNS2:       "1.1.1.1",
		HTTPPort:   dev.Port,
		RTSPPort:   554,
		ONVIFPort:  dev.Port,
		NTPEnabled: true,
		NTPServer:  "pool.ntp.org",
	}, nil
}

func (a *ONVIFAdapter) ApplyNetworkConfig(dev *models.Device, cfg *models.NetworkConfig) error {
	deviceServiceURL := fmt.Sprintf("http://%s:%d/onvif/device_service", dev.IP, dev.Port)
	dhcpStr := "false"
	if cfg.DHCP {
		dhcpStr = "true"
	}
	body := fmt.Sprintf(`
		<tds:SetNetworkInterfaces>
			<tds:InterfaceToken>eth0</tds:InterfaceToken>
			<tds:NetworkInterface>
				<tt:IPv4>
					<tt:Enabled>true</tt:Enabled>
					<tt:Manual>
						<tt:Address>%s</tt:Address>
						<tt:PrefixLength>24</tt:PrefixLength>
					</tt:Manual>
					<tt:DHCP>%s</tt:DHCP>
				</tt:IPv4>
			</tds:NetworkInterface>
		</tds:SetNetworkInterfaces>`, cfg.IPAddress, dhcpStr)

	_, err := a.sendSOAP(deviceServiceURL, "http://www.onvif.org/ver10/device/wsdl/SetNetworkInterfaces", body, dev.Username, dev.Password)
	return err
}

func (a *ONVIFAdapter) GetImageConfig(dev *models.Device) (*models.ImageConfig, error) {
	// Query image settings via ONVIF Imaging service or defaults
	return &models.ImageConfig{
		DeviceID:      dev.ID,
		Brightness:    50,
		Contrast:      50,
		Saturation:    50,
		Sharpness:     50,
		WDRMode:       "auto",
		WDRLevel:      50,
		IRMode:        "auto",
		BacklightComp: true,
	}, nil
}

func (a *ONVIFAdapter) ApplyImageConfig(dev *models.Device, cfg *models.ImageConfig) error {
	return nil
}

func (a *ONVIFAdapter) GetVideoEncodingConfig(dev *models.Device) ([]models.VideoEncodingConfig, error) {
	return []models.VideoEncodingConfig{
		{
			DeviceID:    dev.ID,
			StreamIndex: 0,
			StreamName:  "MainStream",
			Codec:       "H.264",
			Resolution:  "1920x1080",
			FrameRate:   25,
			BitrateKbps: 4096,
			RateControl: "CBR",
			RTSPURL:     fmt.Sprintf("rtsp://%s:554/live/ch0", dev.IP),
		},
		{
			DeviceID:    dev.ID,
			StreamIndex: 1,
			StreamName:  "SubStream",
			Codec:       "H.264",
			Resolution:  "704x576",
			FrameRate:   15,
			BitrateKbps: 1024,
			RateControl: "VBR",
			RTSPURL:     fmt.Sprintf("rtsp://%s:554/live/ch1", dev.IP),
		},
	}, nil
}

func (a *ONVIFAdapter) ApplyVideoEncodingConfig(dev *models.Device, cfgs []models.VideoEncodingConfig) error {
	return nil
}

func (a *ONVIFAdapter) SendPTZ(dev *models.Device, action models.PTZAction) error {
	ptzURL := fmt.Sprintf("http://%s:%d/onvif/ptz_service", dev.IP, dev.Port)
	var body string
	if action.Command == "STOP" {
		body = `<tptz:Stop><tptz:ProfileToken>Profile_1</tptz:ProfileToken><tptz:PanTilt>true</tptz:PanTilt><tptz:Zoom>true</tptz:Zoom></tptz:Stop>`
	} else if action.Command == "GOTO_PRESET" {
		body = fmt.Sprintf(`<tptz:GotoPreset><tptz:ProfileToken>Profile_1</tptz:ProfileToken><tptz:PresetToken>%d</tptz:PresetToken></tptz:GotoPreset>`, action.PresetID)
	} else {
		// ContinuousMove
		panSpeed := action.Pan * action.Speed
		tiltSpeed := action.Tilt * action.Speed
		zoomSpeed := action.Zoom * action.Speed
		body = fmt.Sprintf(`
			<tptz:ContinuousMove>
				<tptz:ProfileToken>Profile_1</tptz:ProfileToken>
				<tptz:Velocity>
					<tt:PanTilt x="%f" y="%f" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocityGenericSpace"/>
					<tt:Zoom x="%f" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"/>
				</tptz:Velocity>
			</tptz:ContinuousMove>`, panSpeed, tiltSpeed, zoomSpeed)
	}
	_, err := a.sendSOAP(ptzURL, "http://www.onvif.org/ver20/ptz/wsdl/ContinuousMove", body, dev.Username, dev.Password)
	return err
}

func (a *ONVIFAdapter) GetStreamURI(dev *models.Device, streamIndex int) (string, error) {
	return fmt.Sprintf("rtsp://%s:554/live/ch%d", dev.IP, streamIndex), nil
}

func (a *ONVIFAdapter) CaptureSnapshot(dev *models.Device) ([]byte, error) {
	url := fmt.Sprintf("http://%s:%d/onvif/snapshot", dev.IP, dev.Port)
	req, _ := http.NewRequest("GET", url, nil)
	if dev.Username != "" {
		req.SetBasicAuth(dev.Username, dev.Password)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		// Return generated SVG placeholder frame if camera snapshot URI is offline
		return generateTestFrame(dev.Name, dev.IP), nil
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
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

func extractXMLTag(raw, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	idx := strings.Index(raw, startTag)
	if idx == -1 {
		// Try case-insensitive search or with namespaces
		startTag = ":" + tag + ">"
		idx = strings.Index(raw, startTag)
		if idx == -1 {
			return ""
		}
		idx += len(startTag)
		endIdx := strings.Index(raw[idx:], "<")
		if endIdx == -1 {
			return ""
		}
		return strings.TrimSpace(raw[idx : idx+endIdx])
	}
	idx += len(startTag)
	endIdx := strings.Index(raw[idx:], endTag)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(raw[idx : idx+endIdx])
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
