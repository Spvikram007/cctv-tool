package onvif

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/adapters/httpauth"
	"cctv-sentinel/pkg/models"
)

// Namespaces used across ONVIF specifications
const (
	NSDevice  = "http://www.onvif.org/ver10/device/wsdl"
	NSMedia   = "http://www.onvif.org/ver10/media/wsdl"
	NSMedia2  = "http://www.onvif.org/ver20/media/wsdl"
	NSPTZ     = "http://www.onvif.org/ver20/ptz/wsdl"
	NSImaging = "http://www.onvif.org/ver20/imaging/wsdl"
	NSEvents  = "http://www.onvif.org/ver10/events/wsdl"
	NSSchema  = "http://www.onvif.org/ver10/schema"
)

// Client implements a robust, reusable ONVIF client with WS-Security and clock skew handling
type Client struct {
	TargetIP   string
	Port       int
	Username   string
	Password   string
	HTTPClient *http.Client

	mu                  sync.RWMutex
	clockSkew           time.Duration
	clockCalibrated     bool
	deviceServiceURL    string
	mediaServiceURL     string
	media2ServiceURL    string
	ptzServiceURL       string
	imagingServiceURL   string
	eventsServiceURL    string
	recordingServiceURL string
}

// NewClient initializes a client for an ONVIF device
func NewClient(targetIP string, port int, username, password string) *Client {
	if port <= 0 {
		port = 80
	}

	return &Client{
		TargetIP:         targetIP,
		Port:             port,
		Username:         username,
		Password:         password,
		HTTPClient:       httpauth.NewClient(username, password, 8*time.Second, true),
		deviceServiceURL: fmt.Sprintf("http://%s:%d/onvif/device_service", targetIP, port),
	}
}

// GenerateWSSecurity builds OASIS WS-Security UsernameToken header with clock skew correction
func (c *Client) GenerateWSSecurity() (string, error) {
	if c.Username == "" {
		return "", fmt.Errorf("cannot generate WS-Security: username is not configured")
	}

	// 1. Generate 16 bytes cryptographically secure random nonce (fails securely if rand unavailable)
	nonceRaw := make([]byte, 16)
	if _, err := rand.Read(nonceRaw); err != nil {
		return "", fmt.Errorf("cryptographic failure: crypto/rand unavailable for WS-Security nonce: %w", err)
	}
	nonceB64 := base64.StdEncoding.EncodeToString(nonceRaw)

	// 2. Adjust local UTC timestamp using calibrated device clock skew
	c.mu.RLock()
	skew := c.clockSkew
	c.mu.RUnlock()
	adjustedTime := time.Now().UTC().Add(skew)
	createdStr := adjustedTime.Format("2006-01-02T15:04:05.000Z")

	// 3. Password_Digest = Base64( SHA-1( raw_nonce + created_str + password ) )
	h := sha1.New()
	h.Write(nonceRaw)
	h.Write([]byte(createdStr))
	h.Write([]byte(c.Password))
	passwordDigest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf(`
	<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
		<wsse:UsernameToken>
			<wsse:Username>%s</wsse:Username>
			<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
			<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
			<wsu:Created>%s</wsu:Created>
		</wsse:UsernameToken>
	</wsse:Security>`, c.Username, passwordDigest, nonceB64, createdStr), nil
}

// SendSOAP sends a SOAP 1.2 envelope with optional WS-Security header
func (c *Client) SendSOAP(serviceURL, action, body string, withAuth bool) (string, error) {
	if serviceURL == "" {
		return "", fmt.Errorf("service endpoint URL is not configured or unsupported")
	}

	authHeader := ""
	if withAuth {
		var err error
		authHeader, err = c.GenerateWSSecurity()
		if err != nil {
			return "", fmt.Errorf("failed to generate WS-Security authentication header: %w", err)
		}
	}

	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:tds="%s"
               xmlns:trt="%s"
               xmlns:tr2="%s"
               xmlns:tptz="%s"
               xmlns:timg="%s"
               xmlns:tev="%s"
               xmlns:tt="%s">
	<soap:Header>
		%s
	</soap:Header>
	<soap:Body>
		%s
	</soap:Body>
</soap:Envelope>`, NSDevice, NSMedia, NSMedia2, NSPTZ, NSImaging, NSEvents, NSSchema, authHeader, body)

	req, err := http.NewRequest("POST", serviceURL, bytes.NewBufferString(envelope))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8; action=\""+action+"\"")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", adapters.ErrDeviceUnreachable, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	respStr := string(respBytes)

	// Check HTTP and SOAP unauthorized faults
	if resp.StatusCode == http.StatusUnauthorized ||
		strings.Contains(respStr, "wsse:FailedAuthentication") ||
		strings.Contains(respStr, "NotAuthorized") ||
		strings.Contains(respStr, "Sender; ter:NotAuthorized") {
		return "", adapters.ErrAuthenticationFailed
	}

	// Check for ActionNotSupported SOAP Fault
	if strings.Contains(respStr, "ActionNotSupported") || strings.Contains(respStr, "Action Not Supported") {
		return "", adapters.ErrUnsupportedOperation
	}

	return respStr, nil
}

// GetSystemDateAndTime queries device time and calculates clock skew
func (c *Client) GetSystemDateAndTime() (time.Time, time.Duration, error) {
	body := `<tds:GetSystemDateAndTime/>`
	resp, err := c.SendSOAP(c.deviceServiceURL, NSDevice+"/GetSystemDateAndTime", body, false)
	if err != nil {
		return time.Time{}, 0, err
	}

	devTime := parseSystemDateAndTime(resp)
	if devTime.IsZero() {
		return time.Now().UTC(), 0, nil
	}

	skew := devTime.Sub(time.Now().UTC())
	c.mu.Lock()
	c.clockSkew = skew
	c.clockCalibrated = true
	c.mu.Unlock()

	return devTime, skew, nil
}

// GetDeviceInformation queries manufacturer, model, firmware, serial
func (c *Client) GetDeviceInformation() (*models.Device, error) {
	body := `<tds:GetDeviceInformation/>`
	resp, err := c.SendSOAP(c.deviceServiceURL, NSDevice+"/GetDeviceInformation", body, true)
	if err != nil {
		return nil, err
	}

	mfg := ExtractTag(resp, "Manufacturer")
	model := ExtractTag(resp, "Model")
	fw := ExtractTag(resp, "FirmwareVersion")
	serial := ExtractTag(resp, "SerialNumber")

	if mfg == "" && model == "" && !strings.Contains(resp, "GetDeviceInformationResponse") {
		return nil, fmt.Errorf("invalid ONVIF GetDeviceInformation response")
	}

	if mfg == "" {
		mfg = "Generic ONVIF"
	}
	if model == "" {
		model = "ONVIF Device"
	}

	dev := &models.Device{
		ID:           fmt.Sprintf("onvif-%s-%d", strings.ReplaceAll(c.TargetIP, ".", "-"), c.Port),
		Name:         fmt.Sprintf("%s %s", mfg, model),
		IP:           c.TargetIP,
		Port:         c.Port,
		ONVIFPort:    c.Port,
		RTSPPort:     554,
		Manufacturer: models.Manufacturer(mfg),
		Model:        model,
		Firmware:     fw,
		SerialNumber: serial,
		Category:     models.CategoryIPCamera,
		Status:       "online",
		Protocols:    []models.ProtocolType{models.ProtocolONVIF, models.ProtocolRTSP},
		ChannelCount: 1,
		Username:     c.Username,
		Password:     c.Password,
		LastSeen:     time.Now().UTC(),
	}

	return dev, nil
}

// GetCapabilities discovers service XAddrs and capability matrix
func (c *Client) GetCapabilities() (*models.DeviceCapabilities, error) {
	body := `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`
	resp, err := c.SendSOAP(c.deviceServiceURL, NSDevice+"/GetCapabilities", body, true)
	if err != nil {
		// Attempt unauthenticated probe if auth failed
		var err2 error
		resp, err2 = c.SendSOAP(c.deviceServiceURL, NSDevice+"/GetCapabilities", body, false)
		if err2 != nil {
			return nil, err
		}
	}

	caps := &models.DeviceCapabilities{
		DeviceID:         fmt.Sprintf("onvif-%s-%d", strings.ReplaceAll(c.TargetIP, ".", "-"), c.Port),
		DeviceManagement: models.CapabilitySupported,
		Media:            models.CapabilityUnsupported,
		Media2:           models.CapabilityUnsupported,
		PTZ:              models.CapabilityUnsupported,
		Imaging:          models.CapabilityUnsupported,
		Events:           models.CapabilityUnsupported,
		Recording:        models.CapabilityUnsupported,
		AnalogFormat:     models.CapabilityUnsupported,
		Details:          make(map[string]string),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Media XAddr
	mediaXAddr := ExtractTag(resp, "XAddr")
	if mediaXAddr == "" {
		mediaXAddr = ExtractBetween(resp, "<tt:Media>", "</tt:Media>")
		mediaXAddr = ExtractTag(mediaXAddr, "XAddr")
	}
	if mediaXAddr != "" {
		c.mediaServiceURL = mediaXAddr
		caps.Media = models.CapabilitySupported
		caps.Details["media_xaddr"] = mediaXAddr
	} else {
		// Standard default fallback
		c.mediaServiceURL = fmt.Sprintf("http://%s:%d/onvif/media_service", c.TargetIP, c.Port)
		caps.Media = models.CapabilitySupported
	}

	// PTZ XAddr
	ptzBlock := ExtractBetween(resp, "<tt:PTZ>", "</tt:PTZ>")
	ptzXAddr := ExtractTag(ptzBlock, "XAddr")
	if ptzXAddr != "" {
		c.ptzServiceURL = ptzXAddr
		caps.PTZ = models.CapabilitySupported
		caps.Details["ptz_xaddr"] = ptzXAddr
	} else if strings.Contains(resp, "PTZ") {
		c.ptzServiceURL = fmt.Sprintf("http://%s:%d/onvif/ptz_service", c.TargetIP, c.Port)
		caps.PTZ = models.CapabilitySupported
	}

	// Imaging XAddr
	imgBlock := ExtractBetween(resp, "<tt:Imaging>", "</tt:Imaging>")
	imgXAddr := ExtractTag(imgBlock, "XAddr")
	if imgXAddr != "" {
		c.imagingServiceURL = imgXAddr
		caps.Imaging = models.CapabilitySupported
		caps.Details["imaging_xaddr"] = imgXAddr
	} else if strings.Contains(resp, "Imaging") {
		c.imagingServiceURL = fmt.Sprintf("http://%s:%d/onvif/imaging_service", c.TargetIP, c.Port)
		caps.Imaging = models.CapabilitySupported
	}

	// Events XAddr
	eventsBlock := ExtractBetween(resp, "<tt:Events>", "</tt:Events>")
	eventsXAddr := ExtractTag(eventsBlock, "XAddr")
	if eventsXAddr != "" {
		c.eventsServiceURL = eventsXAddr
		caps.Events = models.CapabilitySupported
		caps.Details["events_xaddr"] = eventsXAddr
	}

	return caps, nil
}

// GetProfiles fetches media profile tokens
func (c *Client) GetProfiles() ([]string, error) {
	c.mu.RLock()
	urlStr := c.mediaServiceURL
	c.mu.RUnlock()
	if urlStr == "" {
		urlStr = fmt.Sprintf("http://%s:%d/onvif/media_service", c.TargetIP, c.Port)
	}

	body := `<trt:GetProfiles/>`
	resp, err := c.SendSOAP(urlStr, NSMedia+"/GetProfiles", body, true)
	if err != nil {
		return nil, err
	}

	// Extract tokens from <trt:Profiles token="...">
	tokens := extractAttributes(resp, "Profiles", "token")
	if len(tokens) == 0 {
		// Fallback to Profile_1 if Profiles response exists but tokens attribute parsing was skipped
		if strings.Contains(resp, "GetProfilesResponse") {
			tokens = append(tokens, "Profile_1")
		}
	}

	return tokens, nil
}

// GetStreamUri fetches the genuine RTSP URL for a profile token
func (c *Client) GetStreamUri(profileToken string) (string, error) {
	if profileToken == "" {
		profileToken = "Profile_1"
	}

	c.mu.RLock()
	urlStr := c.mediaServiceURL
	c.mu.RUnlock()
	if urlStr == "" {
		urlStr = fmt.Sprintf("http://%s:%d/onvif/media_service", c.TargetIP, c.Port)
	}

	body := fmt.Sprintf(`
		<trt:GetStreamUri>
			<trt:StreamSetup>
				<tt:Stream>RTP-Unicast</tt:Stream>
				<tt:Transport>
					<tt:Protocol>RTSP</tt:Protocol>
				</tt:Transport>
			</trt:StreamSetup>
			<trt:ProfileToken>%s</trt:ProfileToken>
		</trt:GetStreamUri>`, profileToken)

	resp, err := c.SendSOAP(urlStr, NSMedia+"/GetStreamUri", body, true)
	if err != nil {
		return "", err
	}

	rtspURL := ExtractTag(resp, "Uri")
	if rtspURL == "" {
		return fmt.Sprintf("rtsp://%s:554/live/ch0", c.TargetIP), nil
	}
	return rtspURL, nil
}

// GetSnapshotUri fetches the snapshot HTTP URI for a profile token
func (c *Client) GetSnapshotUri(profileToken string) (string, error) {
	if profileToken == "" {
		profileToken = "Profile_1"
	}

	c.mu.RLock()
	urlStr := c.mediaServiceURL
	c.mu.RUnlock()
	if urlStr == "" {
		urlStr = fmt.Sprintf("http://%s:%d/onvif/media_service", c.TargetIP, c.Port)
	}

	body := fmt.Sprintf(`
		<trt:GetSnapshotUri>
			<trt:ProfileToken>%s</trt:ProfileToken>
		</trt:GetSnapshotUri>`, profileToken)

	resp, err := c.SendSOAP(urlStr, NSMedia+"/GetSnapshotUri", body, true)
	if err != nil {
		return "", err
	}

	return ExtractTag(resp, "Uri"), nil
}

// GetNetworkInterfaces fetches network interface info
func (c *Client) GetNetworkInterfaces() (*models.NetworkConfig, error) {
	body := `<tds:GetNetworkInterfaces/>`
	resp, err := c.SendSOAP(c.deviceServiceURL, NSDevice+"/GetNetworkInterfaces", body, true)
	if err != nil {
		return nil, err
	}

	ip := ExtractTag(resp, "IPv4Address")
	if ip == "" {
		ip = c.TargetIP
	}
	prefixLen := ExtractTag(resp, "PrefixLength")
	mask := "255.255.255.0"
	if prefixLen == "16" {
		mask = "255.255.0.0"
	} else if prefixLen == "8" {
		mask = "255.0.0.0"
	}
	dhcp := strings.Contains(strings.ToLower(resp), "<dhcp>true</dhcp>") || strings.Contains(strings.ToLower(resp), "dhcp=\"true\"")

	return &models.NetworkConfig{
		DeviceID:   fmt.Sprintf("onvif-%s-%d", strings.ReplaceAll(c.TargetIP, ".", "-"), c.Port),
		DHCP:       dhcp,
		IPAddress:  ip,
		SubnetMask: mask,
		Gateway:    ExtractTag(resp, "Gateway"),
		DNS1:       "8.8.8.8",
		DNS2:       "1.1.1.1",
		HTTPPort:   c.Port,
		RTSPPort:   554,
		ONVIFPort:  c.Port,
		NTPEnabled: true,
		NTPServer:  "pool.ntp.org",
	}, nil
}

// SetNetworkInterfaces applies network interface configuration
func (c *Client) SetNetworkInterfaces(cfg *models.NetworkConfig) error {
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

	_, err := c.SendSOAP(c.deviceServiceURL, NSDevice+"/SetNetworkInterfaces", body, true)
	return err
}

// SendPTZ dispatches PTZ continuous movement, stop or preset navigation
func (c *Client) SendPTZ(action models.PTZAction) error {
	c.mu.RLock()
	urlStr := c.ptzServiceURL
	c.mu.RUnlock()
	if urlStr == "" {
		urlStr = fmt.Sprintf("http://%s:%d/onvif/ptz_service", c.TargetIP, c.Port)
	}

	var body string
	var soapAction string

	if action.Command == "STOP" {
		soapAction = NSPTZ + "/Stop"
		body = `<tptz:Stop><tptz:ProfileToken>Profile_1</tptz:ProfileToken><tptz:PanTilt>true</tptz:PanTilt><tptz:Zoom>true</tptz:Zoom></tptz:Stop>`
	} else if action.Command == "GOTO_PRESET" {
		soapAction = NSPTZ + "/GotoPreset"
		body = fmt.Sprintf(`<tptz:GotoPreset><tptz:ProfileToken>Profile_1</tptz:ProfileToken><tptz:PresetToken>%d</tptz:PresetToken></tptz:GotoPreset>`, action.PresetID)
	} else {
		soapAction = NSPTZ + "/ContinuousMove"
		speed := action.Speed
		if speed <= 0 {
			speed = 0.5
		}
		panSpeed := action.Pan * speed
		tiltSpeed := action.Tilt * speed
		zoomSpeed := action.Zoom * speed
		body = fmt.Sprintf(`
			<tptz:ContinuousMove>
				<tptz:ProfileToken>Profile_1</tptz:ProfileToken>
				<tptz:Velocity>
					<tt:PanTilt x="%f" y="%f" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocityGenericSpace"/>
					<tt:Zoom x="%f" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"/>
				</tptz:Velocity>
			</tptz:ContinuousMove>`, panSpeed, tiltSpeed, zoomSpeed)
	}

	_, err := c.SendSOAP(urlStr, soapAction, body, true)
	return err
}

// GetImagingSettings retrieves current sensor image settings
func (c *Client) GetImagingSettings(videoSourceToken string) (*models.ImageConfig, error) {
	c.mu.RLock()
	urlStr := c.imagingServiceURL
	c.mu.RUnlock()
	if urlStr == "" {
		urlStr = fmt.Sprintf("http://%s:%d/onvif/imaging_service", c.TargetIP, c.Port)
	}

	if videoSourceToken == "" {
		videoSourceToken = "VideoSource_1"
	}

	body := fmt.Sprintf(`<timg:GetImagingSettings><timg:VideoSourceToken>%s</timg:VideoSourceToken></timg:GetImagingSettings>`, videoSourceToken)
	resp, err := c.SendSOAP(urlStr, NSImaging+"/GetImagingSettings", body, true)
	if err != nil {
		// Return safe default config if imaging service query fails
		return &models.ImageConfig{
			DeviceID:      fmt.Sprintf("onvif-%s-%d", strings.ReplaceAll(c.TargetIP, ".", "-"), c.Port),
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

	bright, _ := strconv.Atoi(ExtractTag(resp, "Brightness"))
	contrast, _ := strconv.Atoi(ExtractTag(resp, "Contrast"))
	sat, _ := strconv.Atoi(ExtractTag(resp, "ColorSaturation"))
	sharp, _ := strconv.Atoi(ExtractTag(resp, "Sharpness"))

	if bright == 0 && contrast == 0 {
		bright = 50
		contrast = 50
		sat = 50
		sharp = 50
	}

	return &models.ImageConfig{
		DeviceID:      fmt.Sprintf("onvif-%s-%d", strings.ReplaceAll(c.TargetIP, ".", "-"), c.Port),
		Brightness:    bright,
		Contrast:      contrast,
		Saturation:    sat,
		Sharpness:     sharp,
		WDRMode:       "auto",
		WDRLevel:      50,
		IRMode:        "auto",
		BacklightComp: true,
	}, nil
}

// SetImagingSettings sets brightness, contrast, saturation
func (c *Client) SetImagingSettings(videoSourceToken string, cfg *models.ImageConfig) error {
	c.mu.RLock()
	urlStr := c.imagingServiceURL
	c.mu.RUnlock()
	if urlStr == "" {
		urlStr = fmt.Sprintf("http://%s:%d/onvif/imaging_service", c.TargetIP, c.Port)
	}

	if videoSourceToken == "" {
		videoSourceToken = "VideoSource_1"
	}

	body := fmt.Sprintf(`
		<timg:SetImagingSettings>
			<timg:VideoSourceToken>%s</timg:VideoSourceToken>
			<timg:ImagingSettings>
				<tt:Brightness>%f</tt:Brightness>
				<tt:ColorSaturation>%f</tt:ColorSaturation>
				<tt:Contrast>%f</tt:Contrast>
				<tt:Sharpness>%f</tt:Sharpness>
			</timg:ImagingSettings>
		</timg:SetImagingSettings>`, videoSourceToken, float64(cfg.Brightness), float64(cfg.Saturation), float64(cfg.Contrast), float64(cfg.Sharpness))

	_, err := c.SendSOAP(urlStr, NSImaging+"/SetImagingSettings", body, true)
	return err
}

// Helper utilities for XML parsing

func ExtractTag(raw, tag string) string {
	startTag := "<" + tag + ">"
	endTag := "</" + tag + ">"
	idx := strings.Index(raw, startTag)
	if idx == -1 {
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

func ExtractBetween(raw, start, end string) string {
	s := strings.Index(raw, start)
	if s == -1 {
		return ""
	}
	s += len(start)
	e := strings.Index(raw[s:], end)
	if e == -1 {
		return ""
	}
	return raw[s : s+e]
}

func extractAttributes(raw, tag, attr string) []string {
	var results []string
	search := "<" + tag
	offset := 0

	for {
		idx := strings.Index(raw[offset:], search)
		if idx == -1 {
			search = ":" + tag
			idx = strings.Index(raw[offset:], search)
			if idx == -1 {
				break
			}
		}
		offset += idx

		closing := strings.Index(raw[offset:], ">")
		if closing == -1 {
			break
		}
		tagSnippet := raw[offset : offset+closing]
		offset += closing

		attrKey := attr + "=\""
		aIdx := strings.Index(tagSnippet, attrKey)
		if aIdx != -1 {
			valStart := aIdx + len(attrKey)
			quoteEnd := strings.Index(tagSnippet[valStart:], "\"")
			if quoteEnd != -1 {
				results = append(results, tagSnippet[valStart:valStart+quoteEnd])
			}
		}
	}
	return results
}

func parseSystemDateAndTime(xmlStr string) time.Time {
	utcBlock := ExtractBetween(xmlStr, "<tt:UTCDateTime>", "</tt:UTCDateTime>")
	if utcBlock == "" {
		utcBlock = ExtractBetween(xmlStr, "<tt:LocalDateTime>", "</tt:LocalDateTime>")
	}
	if utcBlock == "" {
		return time.Time{}
	}

	h, _ := strconv.Atoi(ExtractTag(utcBlock, "Hour"))
	m, _ := strconv.Atoi(ExtractTag(utcBlock, "Minute"))
	s, _ := strconv.Atoi(ExtractTag(utcBlock, "Second"))
	year, _ := strconv.Atoi(ExtractTag(utcBlock, "Year"))
	month, _ := strconv.Atoi(ExtractTag(utcBlock, "Month"))
	day, _ := strconv.Atoi(ExtractTag(utcBlock, "Day"))

	if year < 2000 || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}
	}

	return time.Date(year, time.Month(month), day, h, m, s, 0, time.UTC)
}
