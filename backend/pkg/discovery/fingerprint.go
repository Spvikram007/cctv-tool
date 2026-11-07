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
	"strconv"
	"strings"
	"time"

	"cctv-sentinel/pkg/models"
)

// MACPrefixVendor maps MAC OUI prefixes (first 3 octets, lowercased with colon) to manufacturers
var ComprehensiveMACPrefixVendor = map[string]models.Manufacturer{
	// Hikvision
	"bc:54:51": models.VendorHikvision,
	"c0:56:e3": models.VendorHikvision,
	"c0:51:7e": models.VendorHikvision,
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
	"30:dd:aa": models.VendorDahua,
	"fc:b6:9d": models.VendorDahua,
	"f8:ce:07": models.VendorDahua,
	"90:6a:94": models.VendorDahua,

	// CP PLUS / Aditya Infotech
	"f0:c8:50": models.VendorCPPlus,
	"b0:c5:54": models.VendorCPPlus,
	"84:b5:41": models.VendorCPPlus,
	"24:52:6a": models.VendorCPPlus,
	"14:07:08": models.VendorCPPlus,
	"28:18:fd": models.VendorCPPlus,

	// Axis
	"00:40:8c": models.VendorAxis,
	"ac:cc:8e": models.VendorAxis,
	"b8:a4:4f": models.VendorAxis,
	"00:02:d1": models.VendorAxis,

	// Hanwha / Samsung Techwin
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

	// Xiongmai (XM) OEMs
	"00:12:12": models.VendorOEM,
	"00:12:13": models.VendorOEM,
	"00:12:14": models.VendorOEM,
	"00:12:15": models.VendorOEM,
	"00:12:16": models.VendorOEM,

	// Reolink
	"ec:71:db": models.VendorGeneric,

	// VStarcam
	"00:0e:c6": models.VendorGeneric,
}

// EvidenceItem represents a single piece of evidence contributing to device identification
type EvidenceItem struct {
	Category    string `json:"category"`    // OUI, HTTP_HEADER, HTTPS_CERT, RTSP_SERVER, ONVIF_SCOPE, PORT_SIGNATURE
	Description string `json:"description"` // Description of what was found
	Weight      int    `json:"weight"`      // Contribution weight to confidence
	Value       string `json:"value"`       // Extracted raw evidence value
	Explanation string `json:"explanation"` // Engineering rationale for weighting
}

// DeviceServices represents network services discovered on the device
type DeviceServices struct {
	OpenPorts         []int             `json:"open_ports"`
	HTTPServer        string            `json:"http_server,omitempty"`
	HTTPAuthHeader    string            `json:"http_auth_header,omitempty"`
	RTSPServer        string            `json:"rtsp_server,omitempty"`
	RTSPPublicOpts    string            `json:"rtsp_public_opts,omitempty"`
	SSLCertIssuer     string            `json:"ssl_cert_issuer,omitempty"`
	SSLCertSubject    string            `json:"ssl_cert_subject,omitempty"`
	ONVIFServiceURL   string            `json:"onvif_service_url,omitempty"`
	AdditionalBanners map[string]string `json:"additional_banners,omitempty"`
}

// FingerprintResult holds the complete result of the fingerprinting and identification process
type FingerprintResult struct {
	IP                     string                `json:"ip"`
	MAC                    string                `json:"mac"`
	IdentifiedManufacturer models.Manufacturer   `json:"identified_manufacturer"`
	ProbableManufacturer   string                `json:"probable_manufacturer"`
	Model                  string                `json:"model"`
	Firmware               string                `json:"firmware"`
	SerialNumber           string                `json:"serial_number"`
	Category               models.DeviceCategory `json:"category"`
	Confidence             int                   `json:"confidence"` // 0 - 100
	ConfidenceLevel        string                `json:"confidence_level"` // HIGH, MEDIUM, LOW
	ConfidenceExplanation string                `json:"confidence_explanation"`
	EvidenceSources        []EvidenceItem        `json:"evidence_sources"`
	UnknownFields          []string              `json:"unknown_fields"`
	Services               DeviceServices        `json:"services"`
	Protocols              []models.ProtocolType `json:"protocols"`
	Capabilities           []string              `json:"capabilities"`
	ChannelCount           int                   `json:"channel_count"`
	DiscoveredVia          string                `json:"discovered_via"` // ONVIF, TCP_SWEEP, HYBRID
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

// IsLocallyAdministeredMAC checks if the IEEE 802 U/L bit (bit 1 of octet 0) is set.
// Locally administered addresses are private, virtual, or randomized and are NOT globally assigned by IEEE.
func IsLocallyAdministeredMAC(mac string) bool {
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(mac, "-", ":"), ".", ":"))
	parts := strings.Split(clean, ":")
	if len(parts) >= 1 && len(parts[0]) == 2 {
		val, err := strconv.ParseUint(parts[0], 16, 8)
		if err == nil {
			return (val & 0x02) != 0
		}
	}
	return false
}

// IsMulticastOrBroadcastMAC checks if the IEEE 802 I/G bit (bit 0 of octet 0) is set.
func IsMulticastOrBroadcastMAC(mac string) bool {
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(mac, "-", ":"), ".", ":"))
	parts := strings.Split(clean, ":")
	if len(parts) >= 1 && len(parts[0]) == 2 {
		val, err := strconv.ParseUint(parts[0], 16, 8)
		if err == nil {
			return (val & 0x01) != 0
		}
	}
	return false
}

// LookupMACFromOUI checks known OUI tables. Never returns a specific model.
func LookupMACFromOUI(mac string) (models.Manufacturer, bool) {
	if IsLocallyAdministeredMAC(mac) || IsMulticastOrBroadcastMAC(mac) {
		return models.VendorGeneric, false
	}

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

// AnalyzeFingerprint evaluates all collected evidence, computes confidence score,
// avoids double-counting correlated evidence, handles locally administered MACs, and classifies device.
// RULE: Never infer a specific model from an OUI alone.
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
	corroborations := make([]string, 0)

	// 1. Evaluate ONVIF WS-Discovery evidence (Definitive: 55 points)
	hasONVIF := (onvifMatch != nil)
	if hasONVIF {
		result.Protocols = append(result.Protocols, models.ProtocolONVIF)
		evidenceVal := fmt.Sprintf("Scopes: %s", strings.Join(onvifMatch.Scopes, " "))
		weight := 55
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "ONVIF_WS_DISCOVERY",
			Description: "Active ONVIF WS-Discovery SOAP ProbeMatch response received",
			Weight:      weight,
			Value:       evidenceVal,
			Explanation: "SOAP ProbeMatches envelope confirmed presence of ONVIF web services stack.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("ONVIF WS-Discovery (+%d%%)", weight))

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

	// 2. Evaluate MAC OUI evidence
	// RULE: Handle locally administered MACs; missing OUIs; never infer model from OUI alone!
	if mac != "" {
		if IsLocallyAdministeredMAC(mac) {
			result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
				Category:    "MAC_OUI",
				Description: "Locally Administered / Private MAC address (IEEE U/L bit set) - vendor OUI not globally assigned",
				Weight:      0,
				Value:       mac,
				Explanation: "Bit 1 of first octet is set. Hardware uses randomized or virtual switch MAC; OUI vendor inference is invalid.",
			})
		} else if IsMulticastOrBroadcastMAC(mac) {
			result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
				Category:    "MAC_OUI",
				Description: "Multicast or Broadcast MAC address - not a unique device identity",
				Weight:      0,
				Value:       mac,
				Explanation: "Bit 0 of first octet is set.",
			})
		} else if ouiVendor, ok := LookupMACFromOUI(mac); ok {
			// MAC alone contributes at most 25 points. If ONVIF already proved vendor, MAC corroboration is +15.
			weight := 25
			if hasONVIF {
				weight = 15
			}
			result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
				Category:    "MAC_OUI",
				Description: fmt.Sprintf("IEEE OUI prefix matches registered vendor block for %s", ouiVendor),
				Weight:      weight,
				Value:       mac,
				Explanation: fmt.Sprintf("Hardware NIC prefix assigned to %s. Note: Model is NOT inferred from OUI.", ouiVendor),
			})
			totalConfidence += weight
			corroborations = append(corroborations, fmt.Sprintf("MAC OUI %s (+%d%%)", ouiVendor, weight))

			if result.IdentifiedManufacturer == "" || result.IdentifiedManufacturer == models.VendorGeneric {
				result.ProbableManufacturer = string(ouiVendor)
			}
		} else {
			result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
				Category:    "MAC_OUI",
				Description: "MAC OUI prefix does not match known CCTV manufacturer block",
				Weight:      0,
				Value:       mac,
				Explanation: "MAC prefix is valid IEEE unicast, but vendor is generic or unlisted in surveillance OUI database.",
			})
		}
	}

	// 3. Evaluate RTSP OPTIONS Server banner evidence (20 - 25 points)
	if services.RTSPServer != "" {
		result.Protocols = append(result.Protocols, models.ProtocolRTSP)
		result.Capabilities = append(result.Capabilities, "RTSP_H264", "SNAPSHOT")
		weight := 20
		serverLower := strings.ToLower(services.RTSPServer)

		if strings.Contains(serverLower, "hikvision") {
			weight = 25
			result.IdentifiedManufacturer = models.VendorHikvision
		} else if strings.Contains(serverLower, "dahua") || strings.Contains(serverLower, "gm_rtsp") {
			weight = 25
			result.IdentifiedManufacturer = models.VendorDahua
		} else if strings.Contains(serverLower, "axis") {
			weight = 25
			result.IdentifiedManufacturer = models.VendorAxis
		} else if strings.Contains(serverLower, "live555") {
			weight = 10 // Generic open source streaming server
		}

		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "RTSP_SERVER",
			Description: fmt.Sprintf("RTSP OPTIONS returned Server header: %s", services.RTSPServer),
			Weight:      weight,
			Value:       services.RTSPServer,
			Explanation: "Direct RTSP streaming daemon handshake confirmed video transport capability.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("RTSP Server (%s, +%d%%)", services.RTSPServer, weight))
	}

	// 4. Evaluate HTTP Server header & Auth evidence (Single HTTP proof to avoid double-counting)
	if services.HTTPServer != "" || services.HTTPAuthHeader != "" {
		srvLower := strings.ToLower(services.HTTPServer)
		authLower := strings.ToLower(services.HTTPAuthHeader)
		weight := 15
		bannerNote := services.HTTPServer
		if bannerNote == "" {
			bannerNote = services.HTTPAuthHeader
		}

		if strings.Contains(srvLower, "hikvision") || strings.Contains(authLower, "hikvision") {
			weight = 20
			result.IdentifiedManufacturer = models.VendorHikvision
			result.Protocols = append(result.Protocols, models.ProtocolISAPI)
		} else if strings.Contains(srvLower, "dahua") || strings.Contains(authLower, "dahua") {
			weight = 20
			result.IdentifiedManufacturer = models.VendorDahua
			result.Protocols = append(result.Protocols, models.ProtocolDahuaCGI)
		} else if strings.Contains(srvLower, "app-webs") {
			weight = 15
			if result.ProbableManufacturer == "" {
				result.ProbableManufacturer = string(models.VendorHikvision)
			}
		}

		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "HTTP_BANNER",
			Description: fmt.Sprintf("HTTP Server / Auth Header: %s", bannerNote),
			Weight:      weight,
			Value:       bannerNote,
			Explanation: "HTTP web admin interface identified; correlated headers evaluated without credential challenges.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("HTTP Header (+%d%%)", weight))
	}

	// 5. Evaluate SSL Certificate evidence (15 points)
	if services.SSLCertIssuer != "" || services.SSLCertSubject != "" {
		certLower := strings.ToLower(services.SSLCertSubject + " " + services.SSLCertIssuer)
		weight := 15
		if strings.Contains(certLower, "hikvision") {
			result.IdentifiedManufacturer = models.VendorHikvision
		} else if strings.Contains(certLower, "dahua") {
			result.IdentifiedManufacturer = models.VendorDahua
		} else if strings.Contains(certLower, "axis") {
			result.IdentifiedManufacturer = models.VendorAxis
		} else if strings.Contains(certLower, "cpplus") || strings.Contains(certLower, "cp plus") {
			result.IdentifiedManufacturer = models.VendorCPPlus
		}

		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "HTTPS_CERT",
			Description: fmt.Sprintf("TLS Certificate Subject: %s", services.SSLCertSubject),
			Weight:      weight,
			Value:       services.SSLCertSubject,
			Explanation: "X.509 SSL certificate inspects self-signed camera identity without CA bypass.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("TLS Cert (+%d%%)", weight))
	}

	// 6. Evaluate Open Ports signatures
	// RULE: Avoid counting correlated evidence redundantly if ONVIF already proved vendor.
	hasHikSDK := containsPort(openPorts, 8000)
	hasDahuaSDK := containsPort(openPorts, 37777)
	hasXM := containsPort(openPorts, 34567)

	if hasHikSDK {
		weight := 15
		if result.IdentifiedManufacturer == models.VendorHikvision {
			weight = 10 // Corroborating weight
		}
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "PORT_SIGNATURE",
			Description: "Port 8000/tcp (Hikvision SADP / SDK port) open",
			Weight:      weight,
			Value:       "8000/tcp",
			Explanation: "Port 8000 is universally allocated to Hikvision ISAPI / SADP binary protocol.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("Hikvision Port 8000 (+%d%%)", weight))
		if result.ProbableManufacturer == "" {
			result.ProbableManufacturer = string(models.VendorHikvision)
		}
	}
	if hasDahuaSDK {
		weight := 15
		if result.IdentifiedManufacturer == models.VendorDahua {
			weight = 10 // Corroborating weight
		}
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "PORT_SIGNATURE",
			Description: "Port 37777/tcp (Dahua NetSDK port) open",
			Weight:      weight,
			Value:       "37777/tcp",
			Explanation: "Port 37777 is universally allocated to Dahua NetSDK TCP binary protocol.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("Dahua Port 37777 (+%d%%)", weight))
		if result.ProbableManufacturer == "" {
			result.ProbableManufacturer = string(models.VendorDahua)
		}
	}
	if hasXM {
		weight := 15
		result.EvidenceSources = append(result.EvidenceSources, EvidenceItem{
			Category:    "PORT_SIGNATURE",
			Description: "Port 34567/tcp (Xiongmai XM protocol port) open",
			Weight:      weight,
			Value:       "34567/tcp",
			Explanation: "Port 34567 indicates Xiongmai (XM) hardware chipset / firmware OEM base.",
		})
		totalConfidence += weight
		corroborations = append(corroborations, fmt.Sprintf("Xiongmai Port 34567 (+%d%%)", weight))
		if result.ProbableManufacturer == "" {
			result.ProbableManufacturer = "Xiongmai (XM OEM Base)"
		}
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

	result.ConfidenceExplanation = fmt.Sprintf("Confidence: %d%% (%s) [Corroborated by: %s]",
		result.Confidence, result.ConfidenceLevel, strings.Join(corroborations, ", "))

	// Reconcile Identified vs Probable Manufacturer
	if result.IdentifiedManufacturer == "" {
		if result.ProbableManufacturer != "" {
			result.IdentifiedManufacturer = models.Manufacturer(result.ProbableManufacturer)
		} else {
			result.IdentifiedManufacturer = models.VendorGeneric
			result.ProbableManufacturer = "Generic ONVIF / Unknown"
		}
	}

	// RULE: Never infer a specific model from an OUI alone!
	// If model was not established by ONVIF or explicit probe, mark Unknown
	if result.Model == "" {
		result.Model = "Unknown"
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
