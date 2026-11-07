package discovery

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"cctv-sentinel/pkg/models"
)

// MACPrefixVendor maps MAC OUI prefixes (first 3 octets, lowercased with colon) to manufacturers
var ComprehensiveMACPrefixVendor = map[string]models.Manufacturer{
	// Hikvision
	"bc:54:51": models.VendorHikvision,
	"c0:56:e3": models.VendorHikvision,
	"44:19:b6": models.VendorHikvision,
	"a4:14:37": models.VendorHikvision,
	"10:12:fb": models.VendorHikvision,
	"28:57:be": models.VendorHikvision,
	"54:c4:15": models.VendorHikvision,
	"c4:2f:90": models.VendorHikvision,
	"e8:ab:fa": models.VendorHikvision,

	// Dahua
	"38:af:29": models.VendorDahua,
	"90:02:a9": models.VendorDahua,
	"4c:11:bf": models.VendorDahua,
	"e0:50:8b": models.VendorDahua,
	"a0:bd:cd": models.VendorDahua,
	"b8:a3:87": models.VendorDahua,
	"f4:b8:5e": models.VendorDahua,
	"70:2c:1f": models.VendorDahua,

	// CP PLUS
	"f0:c8:50": models.VendorCPPlus,
	"b0:c5:54": models.VendorCPPlus,
	"84:b5:41": models.VendorCPPlus,
	"24:52:6a": models.VendorCPPlus,

	// Axis
	"00:40:8c": models.VendorAxis,
	"ac:cc:8e": models.VendorAxis,
	"b8:a4:4f": models.VendorAxis,
	"00:02:d1": models.VendorAxis,

	// Hanwha / Samsung
	"00:16:6c": models.VendorHanwha,
	"00:09:18": models.VendorHanwha,
	"00:24:90": models.VendorHanwha,
	"ec:22:80": models.VendorHanwha,

	// Uniview
	"34:bd:fa": models.VendorUniview,
	"e4:b3:18": models.VendorUniview,
	"c8:02:8f": models.VendorUniview,

	// Tiandy
	"00:1c:df": models.VendorTiandy,
	"70:b3:d5": models.VendorTiandy,

	// TVT
	"68:69:2e": models.VendorTVT,
	"48:ea:63": models.VendorTVT,
}

// EvidenceItem represents a single piece of evidence contributing to device identification
type EvidenceItem struct {
	Category    string `json:"category"`    // OUI, HTTP_HEADER, HTTPS_CERT, RTSP_SERVER, ONVIF_SCOPE, PORT_SIGNATURE
	Description string `json:"description"` // Description of what was found
	Weight      int    `json:"weight"`      // Contribution weight to confidence
	Value       string `json:"value"`       // Extracted raw evidence value
}

// DeviceServices represents network services discovered on the device
type DeviceServices struct {
	OpenPorts        []int             `json:"open_ports"`
	HTTPServer       string            `json:"http_server,omitempty"`
	HTTPAuthHeader   string            `json:"http_auth_header,omitempty"`
	RTSPServer       string            `json:"rtsp_server,omitempty"`
	RTSPPublicOpts   string            `json:"rtsp_public_opts,omitempty"`
	SSLCertIssuer    string            `json:"ssl_cert_issuer,omitempty"`
	SSLCertSubject   string            `json:"ssl_cert_subject,omitempty"`
	ONVIFServiceURL  string            `json:"onvif_service_url,omitempty"`
	AdditionalBanners map[string]string `json:"additional_banners,omitempty"`
}

// FingerprintResult holds the complete result of the fingerprinting and identification process
type FingerprintResult struct {
	IP                       string                `json:"ip"`
	MAC                      string                `json:"mac"`
	IdentifiedManufacturer   models.Manufacturer   `json:"identified_manufacturer"`
	ProbableManufacturer     string                `json:"probable_manufacturer"`
	Model                    string                `json:"model"`
	Firmware                 string                `json:"firmware"`
	SerialNumber             string                `json:"serial_number"`
	Category                 models.DeviceCategory `json:"category"`
	Confidence               int                   `json:"confidence"` // 0 - 100
	ConfidenceLevel          string                `json:"confidence_level"` // HIGH, MEDIUM, LOW
	EvidenceSources          []EvidenceItem        `json:"evidence_sources"`
	UnknownFields            []string              `json:"unknown_fields"`
	Services                 DeviceServices        `json:"services"`
	Protocols                []models.ProtocolType `json:"protocols"`
	Capabilities             []string              `json:"capabilities"`
	ChannelCount             int                   `json:"channel_count"`
	DiscoveredVia            string                `json:"discovered_via"` // ONVIF, TCP_SWEEP, HYBRID
}

// Fingerprinter performs active protocol fingerprinting
type Fingerprinter struct {
	HTTPClient *http.Client
}

// NewFingerprinter creates a new Fingerprinter with safe timeouts and custom TLS settings
func NewFingerprinter() *Fingerprinter {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout: 500 * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: 1 * time.Second,
	}
	return &Fingerprinter{
		HTTPClient: &http.Client{
			Transport: tr,
			Timeout:   2 * time.Second,
		},
	}
}

// LookupMACFromOUI checks known OUI tables
func LookupMACFromOUI(mac string) (models.Manufacturer, bool) {
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(mac, "-", ":"), ".", ":"))
	if len(clean) >= 8 {
		prefix := clean[:8]
		if m, ok := ComprehensiveMACPrefixVendor[prefix]; ok {
			return m, true
		}
	}
	return models.VendorGeneric, false
}

// LookupSystemARP queries the local OS ARP cache for a given IP
func LookupSystemARP(ip string) string {
	if ip == "127.0.0.1" || strings.HasPrefix(ip, "127.") {
		return "00:00:00:00:00:00"
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("arp", "-a", ip)
	} else {
		cmd = exec.Command("arp", "-n", ip)
	}

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Regex matching MAC address format xx:xx:xx:xx:xx:xx or xx-xx-xx-xx-xx-xx
	macRe := regexp.MustCompile(`([0-9a-fA-F]{2}[:-]){5}([0-9a-fA-F]{2})`)
	match := macRe.FindString(string(out))
	if match != "" {
		return strings.ToLower(strings.ReplaceAll(match, "-", ":"))
	}
	return ""
}

// ProbeRTSPServer sends an RTSP OPTIONS handshake to retrieve Server banner
func ProbeRTSPServer(ctx context.Context, ip string, port int) (serverBanner, publicOpts string, err error) {
	d := net.Dialer{Timeout: 600 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(1 * time.Second))
	req := fmt.Sprintf("OPTIONS rtsp://%s:%d/ RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: CCTV-Sentinel-Universal/1.0\r\n\r\n", ip, port)
	if _, err := conn.Write([]byte(req)); err != nil {
		return "", "", err
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "server:") {
			serverBanner = strings.TrimSpace(line[7:])
		} else if strings.HasPrefix(strings.ToLower(line), "public:") {
			publicOpts = strings.TrimSpace(line[7:])
		}
	}
	return serverBanner, publicOpts, nil
}

// ProbeSSLCert connects to HTTPS port and inspects certificate
func ProbeSSLCert(ctx context.Context, ip string, port int) (issuer, subject string, err error) {
	d := net.Dialer{Timeout: 600 * time.Millisecond}
	conn, err := tls.DialWithDialer(&d, "tcp", fmt.Sprintf("%s:%d", ip, port), &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) > 0 {
		cert := certs[0]
		issuer = cert.Issuer.String()
		subject = cert.Subject.String()
		return issuer, subject, nil
	}
	return "", "", nil
}

// ProbeHTTPServer sends a lightweight HEAD / GET request to inspect HTTP headers
func (fp *Fingerprinter) ProbeHTTPServer(ctx context.Context, ip string, port int) (server, authHeader, title string) {
	proto := "http"
	if port == 443 || port == 8443 {
		proto = "https"
	}
	reqURL := fmt.Sprintf("%s://%s:%d/", proto, ip, port)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", "", ""
	}

	resp, err := fp.HTTPClient.Do(req)
	if err != nil {
		return "", "", ""
	}
	defer resp.Body.Close()

	server = resp.Header.Get("Server")
	authHeader = resp.Header.Get("WWW-Authenticate")

	// Read small preview of body for HTML title
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	if n > 0 {
		bodyStr := string(buf[:n])
		reTitle := regexp.MustCompile(`(?i)<title>(.*?)</title>`)
		if m := reTitle.FindStringSubmatch(bodyStr); len(m) > 1 {
			title = strings.TrimSpace(m[1])
		}
	}

	return server, authHeader, title
}

// AnalyzeFingerprint evaluates all collected evidence, computes confidence score, and classifies device
func AnalyzeFingerprint(
	ip string,
	mac string,
	openPorts []int,
	onvifMatch *WSProbeMatch,
	services DeviceServices,
	discoveredVia string,
) FingerprintResult {
	result := FingerprintResult{
		IP:              ip,
		MAC:             mac,
		EvidenceSources: make([]EvidenceItem, 0),
		UnknownFields:   make([]string, 0),
		Services:        services,
		Protocols:       []models.ProtocolType{models.ProtocolHTTP},
		Capabilities:    []string{"STREAMING"},
		ChannelCount:    1,
		DiscoveredVia:   discoveredVia,
	}

	totalConfidence := 0

	// 1. Evaluate ONVIF WS-Discovery evidence (Definitive: 50 - 60 points)
	if onvifMatch != nil {
		result.Protocols = append(result.Protocols, models.ProtocolONVIF)
		evidenceVal := fmt.Sprintf("Scopes: %s; XAddrs: %s", strings.Join(onvifMatch.Scopes, " "), strings.Join(onvifMatch.XAddrs, " "))
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "ONVIF_WS_DISCOVERY",
			Description: "Active ONVIF WS-Discovery SOAP ProbeMatch response received",
			Weight:      55,
			Value:       evidenceVal,
		})
		totalConfidence += 55

		if onvifMatch.Manufacturer != "" {
			result.IdentifiedManufacturer = models.Manufacturer(onvifMatch.Manufacturer)
			result.ProbableManufacturer = onvifMatch.Manufacturer
		}
		if onvifMatch.Model != "" {
			result.Model = onvifMatch.Model
		}
		if onvifMatch.Name != "" && result.Model == "" {
			result.Model = onvifMatch.Name
		}
		if len(onvifMatch.XAddrs) > 0 {
			result.Services.ONVIFServiceURL = onvifMatch.XAddrs[0]
		}
	}

	// 2. Evaluate MAC OUI evidence (Probable: 25 - 35 points, NEVER 100% on its own)
	if mac != "" {
		if ouiVendor, ok := LookupMACFromOUI(mac); ok {
			result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
				Category:    "MAC_OUI",
				Description: fmt.Sprintf("MAC prefix matches vendor assignment for %s", ouiVendor),
				Weight:      30,
				Value:       mac,
			})
			totalConfidence += 30
			if result.IdentifiedManufacturer == "" || result.IdentifiedManufacturer == models.VendorGeneric {
				result.ProbableManufacturer = string(ouiVendor)
			}
		} else {
			result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
				Category:    "MAC_OUI",
				Description: "MAC OUI prefix does not match known CCTV manufacturer",
				Weight:      5,
				Value:       mac,
			})
		}
	}

	// 3. Evaluate RTSP OPTIONS Server banner evidence (Strong: 20 - 30 points)
	if services.RTSPServer != "" {
		result.Protocols = append(result.Protocols, models.ProtocolRTSP)
		result.Capabilities = append(result.Capabilities, "RTSP_H264", "SNAPSHOT")
		weight := 20
		serverLower := strings.ToLower(services.RTSPServer)

		if strings.Contains(serverLower, "hikvision") {
			weight = 30
			result.IdentifiedManufacturer = models.VendorHikvision
		} else if strings.Contains(serverLower, "dahua") || strings.Contains(serverLower, "gm_rtsp") {
			weight = 30
			result.IdentifiedManufacturer = models.VendorDahua
		} else if strings.Contains(serverLower, "axis") {
			weight = 30
			result.IdentifiedManufacturer = models.VendorAxis
		} else if strings.Contains(serverLower, "live555") {
			weight = 15 // Common generic RTSP streamer library
		}

		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "RTSP_SERVER",
			Description: fmt.Sprintf("RTSP OPTIONS returned Server header: %s", services.RTSPServer),
			Weight:      weight,
			Value:       services.RTSPServer,
		})
		totalConfidence += weight
	}

	// 4. Evaluate HTTP Server header & Auth evidence (15 - 25 points)
	if services.HTTPServer != "" || services.HTTPAuthHeader != "" {
		weight := 15
		srvLower := strings.ToLower(services.HTTPServer)
		authLower := strings.ToLower(services.HTTPAuthHeader)

		if strings.Contains(srvLower, "hikvision") || strings.Contains(authLower, "hikvision") {
			weight = 25
			result.IdentifiedManufacturer = models.VendorHikvision
			result.Protocols = append(result.Protocols, models.ProtocolISAPI)
		} else if strings.Contains(srvLower, "dahua") || strings.Contains(authLower, "dahua") {
			weight = 25
			result.IdentifiedManufacturer = models.VendorDahua
			result.Protocols = append(result.Protocols, models.ProtocolDahuaCGI)
		} else if strings.Contains(srvLower, "app-webs") { // Common Hikvision HTTP server banner
			weight = 20
			if result.ProbableManufacturer == "" {
				result.ProbableManufacturer = string(models.VendorHikvision)
			}
		}

		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "HTTP_BANNER",
			Description: fmt.Sprintf("HTTP Server: %s, Auth: %s", services.HTTPServer, services.HTTPAuthHeader),
			Weight:      weight,
			Value:       services.HTTPServer,
		})
		totalConfidence += weight
	}

	// 5. Evaluate SSL Certificate evidence (15 - 20 points)
	if services.SSLCertIssuer != "" || services.SSLCertSubject != "" {
		weight := 15
		certLower := strings.ToLower(services.SSLCertSubject + " " + services.SSLCertIssuer)
		if strings.Contains(certLower, "hikvision") {
			result.IdentifiedManufacturer = models.VendorHikvision
			weight = 20
		} else if strings.Contains(certLower, "dahua") {
			result.IdentifiedManufacturer = models.VendorDahua
			weight = 20
		} else if strings.Contains(certLower, "axis") {
			result.IdentifiedManufacturer = models.VendorAxis
			weight = 20
		} else if strings.Contains(certLower, "cpplus") || strings.Contains(certLower, "cp plus") {
			result.IdentifiedManufacturer = models.VendorCPPlus
			weight = 20
		}

		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "HTTPS_CERT",
			Description: fmt.Sprintf("TLS Certificate Subject: %s", services.SSLCertSubject),
			Weight:      weight,
			Value:       services.SSLCertSubject,
		})
		totalConfidence += weight
	}

	// 6. Evaluate Open Ports signatures (10 - 20 points)
	hasHikSDK := containsPort(openPorts, 8000)
	hasDahuaSDK := containsPort(openPorts, 37777)
	hasXM := containsPort(openPorts, 34567)

	if hasHikSDK {
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "PORT_SIGNATURE",
			Description: "Port 8000 (Hikvision SDK / SADP service) open",
			Weight:      20,
			Value:       "8000/tcp",
		})
		totalConfidence += 20
		if result.ProbableManufacturer == "" {
			result.ProbableManufacturer = string(models.VendorHikvision)
		}
	}
	if hasDahuaSDK {
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "PORT_SIGNATURE",
			Description: "Port 37777 (Dahua NetSDK) open",
			Weight:      20,
			Value:       "37777/tcp",
		})
		totalConfidence += 20
		if result.ProbableManufacturer == "" {
			result.ProbableManufacturer = string(models.VendorDahua)
		}
	}
	if hasXM {
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "PORT_SIGNATURE",
			Description: "Port 34567 (Xiongmai / XM Xiongmai protocol) open",
			Weight:      15,
			Value:       "34567/tcp",
		})
		totalConfidence += 15
	}

	// Cap confidence at 100%
	if totalConfidence > 100 {
		totalConfidence = 100
	}
	result.Confidence = totalConfidence

	// Assign Confidence Level
	if totalConfidence >= 80 {
		result.ConfidenceLevel = "HIGH"
	} else if totalConfidence >= 50 {
		result.ConfidenceLevel = "MEDIUM"
	} else {
		result.ConfidenceLevel = "LOW"
	}

	// Reconcile Identified vs Probable Manufacturer
	if result.IdentifiedManufacturer == "" {
		if result.ProbableManufacturer != "" {
			result.IdentifiedManufacturer = models.Manufacturer(result.ProbableManufacturer)
		} else {
			result.IdentifiedManufacturer = models.VendorGeneric
			result.ProbableManufacturer = "Generic ONVIF / Unknown"
		}
	}

	// Default model if unestablished
	if result.Model == "" {
		result.Model = "Standard Streamer"
		result.UnknownFields = append(result.UnknownFields, "model")
	}
	if result.Firmware == "" {
		result.UnknownFields = append(result.UnknownFields, "firmware")
	}
	if result.SerialNumber == "" {
		result.UnknownFields = append(result.UnknownFields, "serial_number")
	}
	if result.MAC == "" {
		result.UnknownFields = append(result.UnknownFields, "mac")
	}

	// 7. Perform Device Classification
	result.Category = ClassifyDevice(result.Model, onvifMatch, openPorts, result.IdentifiedManufacturer)

	return result
}

// ClassifyDevice categorizes hardware into IP_CAMERA, PTZ_CAMERA, DVR, NVR, XVR, HYBRID_RECORDER, ENCODER, NETWORK_VIDEO_SERVER, or UNKNOWN
func ClassifyDevice(
	model string,
	onvifMatch *WSProbeMatch,
	openPorts []int,
	manufacturer models.Manufacturer,
) models.DeviceCategory {
	modelUpper := strings.ToUpper(model)

	// 1. PTZ Camera Check
	if strings.Contains(modelUpper, "PTZ") ||
		strings.Contains(modelUpper, "SPEED DOME") ||
		strings.Contains(modelUpper, "DS-2DE") ||
		strings.Contains(modelUpper, "DS-2DF") ||
		strings.Contains(modelUpper, "DH-SD") ||
		strings.Contains(modelUpper, "CP-VNP") {
		return models.CategoryPTZCamera
	}
	if onvifMatch != nil {
		for _, s := range onvifMatch.Scopes {
			sLower := strings.ToLower(s)
			if strings.Contains(sLower, "ptz") {
				return models.CategoryPTZCamera
			}
		}
	}

	// 2. NVR Check (Network Video Recorder)
	if strings.HasPrefix(modelUpper, "NVR") ||
		strings.Contains(modelUpper, "-NVR") ||
		strings.Contains(modelUpper, "DS-76") ||
		strings.Contains(modelUpper, "DS-77") ||
		strings.Contains(modelUpper, "DS-96") ||
		strings.Contains(modelUpper, "DH-NVR") ||
		strings.Contains(modelUpper, "CP-UNR") ||
		strings.Contains(modelUpper, "NVR-") {
		return models.CategoryNVR
	}

	// 3. XVR / Hybrid Recorder Check
	if strings.Contains(modelUpper, "XVR") ||
		strings.Contains(modelUpper, "DH-XVR") ||
		strings.Contains(modelUpper, "CP-UVR") ||
		strings.Contains(modelUpper, "HVR") {
		return models.CategoryXVR
	}

	// 4. DVR Check (Digital Video Recorder)
	if strings.HasPrefix(modelUpper, "DVR") ||
		strings.Contains(modelUpper, "-DVR") ||
		strings.Contains(modelUpper, "DS-71") ||
		strings.Contains(modelUpper, "DS-72") ||
		strings.Contains(modelUpper, "DS-73") ||
		strings.Contains(modelUpper, "DH-DVR") ||
		strings.Contains(modelUpper, "CP-UVR") {
		return models.CategoryDVR
	}

	// 5. Video Encoder / Video Server Check
	if strings.Contains(modelUpper, "ENCODER") ||
		strings.Contains(modelUpper, "DS-67") ||
		strings.Contains(modelUpper, "NVS") {
		return models.CategoryEncoder
	}

	if onvifMatch != nil {
		hasTransmitter := false
		hasStorage := false
		for _, s := range onvifMatch.Scopes {
			sLower := strings.ToLower(s)
			if strings.Contains(sLower, "network_video_storage") {
				hasStorage = true
			}
			if strings.Contains(sLower, "network_video_transmitter") || strings.Contains(sLower, "video_encoder") {
				hasTransmitter = true
			}
		}
		if hasStorage {
			return models.CategoryNVR
		}
		if hasTransmitter {
			return models.CategoryIPCamera
		}
	}

	// Default fallback: IP Camera if streaming port (RTSP/HTTP) is open
	if containsPort(openPorts, 554) || containsPort(openPorts, 80) || containsPort(openPorts, 8080) {
		return models.CategoryIPCamera
	}

	return models.CategoryUnknown
}

func containsPort(ports []int, target int) bool {
	for _, p := range ports {
		if p == target {
			return true
		}
	}
	return false
}
