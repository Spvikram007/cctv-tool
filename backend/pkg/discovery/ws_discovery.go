package discovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	WSDiscoveryMulticastIPv4  = "239.255.255.250:3702"
	DefaultWSDiscoveryTimeout = 3 * time.Second
)

// WSProbeMatch represents an ONVIF ProbeMatch item extracted from WS-Discovery response
type WSProbeMatch struct {
	EndpointReference string   `json:"endpoint_reference"`
	Types             []string `json:"types"`
	Scopes            []string `json:"scopes"`
	XAddrs            []string `json:"xaddrs"`
	MetadataVersion   string   `json:"metadata_version"`
	IP                string   `json:"ip"`
	Port              int      `json:"port"`
	Manufacturer      string   `json:"manufacturer"`
	Model             string   `json:"model"`
	Name              string   `json:"name"`
	HardwareProfile   string   `json:"hardware_profile"`
	RawXML            string   `json:"-"`
}

// WSDiscoveryOptions configures WS-Discovery probe
type WSDiscoveryOptions struct {
	InterfaceName string
	Timeout       time.Duration
}

// GenerateMessageID generates a standard UUID-based WS-Addressing MessageID
func GenerateMessageID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("urn:uuid:%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// BuildProbeMessage creates a standards-compliant ONVIF WS-Discovery SOAP Probe XML
func BuildProbeMessage(messageID string) []byte {
	probeTemplate := `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
               xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
               xmlns:dn="http://www.onvif.org/ver10/network/wsdl"
               xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <soap:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action>
    <wsa:MessageID>%s</wsa:MessageID>
    <wsa:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</wsa:To>
  </soap:Header>
  <soap:Body>
    <d:Probe>
      <d:Types>dn:NetworkVideoTransmitter tds:Device</d:Types>
    </d:Probe>
  </soap:Body>
</soap:Envelope>`
	return []byte(fmt.Sprintf(probeTemplate, messageID))
}

// ParseProbeMatches parses raw SOAP response XML into a list of WSProbeMatch
func ParseProbeMatches(data []byte) ([]WSProbeMatch, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var matches []WSProbeMatch

	type ProbeMatchRaw struct {
		EndpointReference struct {
			Address string `xml:"Address"`
		} `xml:"EndpointReference"`
		Types           string `xml:"Types"`
		Scopes          string `xml:"Scopes"`
		XAddrs          string `xml:"XAddrs"`
		MetadataVersion string `xml:"MetadataVersion"`
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			if len(matches) > 0 {
				return matches, nil
			}
			return nil, fmt.Errorf("XML parse error: %w", err)
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local == "ProbeMatch" {
				var raw ProbeMatchRaw
				if err := decoder.DecodeElement(&raw, &elem); err == nil {
					match := parseProbeMatchRaw(raw)
					match.RawXML = string(data)
					matches = append(matches, match)
				}
			}
		}
	}

	return matches, nil
}

func parseProbeMatchRaw(raw struct {
	EndpointReference struct {
		Address string `xml:"Address"`
	} `xml:"EndpointReference"`
	Types           string `xml:"Types"`
	Scopes          string `xml:"Scopes"`
	XAddrs          string `xml:"XAddrs"`
	MetadataVersion string `xml:"MetadataVersion"`
}) WSProbeMatch {
	match := WSProbeMatch{
		EndpointReference: strings.TrimSpace(raw.EndpointReference.Address),
		MetadataVersion:   strings.TrimSpace(raw.MetadataVersion),
	}

	// Split types
	for _, t := range strings.Fields(raw.Types) {
		if strings.TrimSpace(t) != "" {
			match.Types = append(match.Types, strings.TrimSpace(t))
		}
	}

	// Split scopes and parse ONVIF semantics
	for _, s := range strings.Fields(raw.Scopes) {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			match.Scopes = append(match.Scopes, trimmed)
			parseScopeItem(trimmed, &match)
		}
	}

	// Split XAddrs and parse IP / Port
	for _, x := range strings.Fields(raw.XAddrs) {
		trimmed := strings.TrimSpace(x)
		if trimmed != "" {
			match.XAddrs = append(match.XAddrs, trimmed)
			if match.IP == "" {
				if parsedURL, err := url.Parse(trimmed); err == nil {
					host, portStr, err := net.SplitHostPort(parsedURL.Host)
					if err == nil {
						match.IP = host
						p, _ := strconv.Atoi(portStr)
						match.Port = p
					} else {
						match.IP = parsedURL.Host
						if parsedURL.Scheme == "https" {
							match.Port = 443
						} else {
							match.Port = 80
						}
					}
				}
			}
		}
	}

	return match
}

// parseScopeItem extracts hardware model, manufacturer and name from an ONVIF scope URL
func parseScopeItem(scope string, match *WSProbeMatch) {
	decoded, err := url.QueryUnescape(scope)
	if err != nil {
		decoded = scope
	}

	lower := strings.ToLower(decoded)

	if strings.Contains(lower, "onvif://www.onvif.org/hardware/") {
		parts := strings.SplitN(decoded, "hardware/", 2)
		if len(parts) == 2 && match.Model == "" {
			match.Model = strings.TrimSpace(parts[1])
		}
	} else if strings.Contains(lower, "onvif://www.onvif.org/name/") {
		parts := strings.SplitN(decoded, "name/", 2)
		if len(parts) == 2 && match.Name == "" {
			match.Name = strings.TrimSpace(parts[1])
		}
	} else if strings.Contains(lower, "onvif://www.onvif.org/profile/") {
		parts := strings.SplitN(decoded, "profile/", 2)
		if len(parts) == 2 {
			match.HardwareProfile = strings.TrimSpace(parts[1])
		}
	} else if strings.Contains(lower, "onvif://www.onvif.org/manufacturer/") {
		parts := strings.SplitN(decoded, "manufacturer/", 2)
		if len(parts) == 2 && match.Manufacturer == "" {
			match.Manufacturer = strings.TrimSpace(parts[1])
		}
	}

	vendors := []struct {
		substr string
		vendor string
	}{
		{"hikvision", "Hikvision"},
		{"dahua", "Dahua"},
		{"cpplus", "CP PLUS"},
		{"cp plus", "CP PLUS"},
		{"uniview", "Uniview"},
		{"axis", "Axis"},
		{"hanwha", "Hanwha"},
		{"tiandy", "Tiandy"},
		{"tvt", "TVT"},
		{"tp-link", "TP-Link"},
		{"vstarcam", "VStarcam"},
		{"xiongmai", "Xiongmai (XM)"},
	}

	for _, v := range vendors {
		if strings.Contains(lower, v.substr) && match.Manufacturer == "" {
			match.Manufacturer = v.vendor
		}
	}

	// Corroborate vendor from model prefix if scope vendor was unestablished
	if match.Manufacturer == "" || match.Manufacturer == "Generic ONVIF" {
		modelUpper := strings.ToUpper(match.Model)
		if strings.HasPrefix(modelUpper, "DH-") || strings.HasPrefix(modelUpper, "DHI-") {
			match.Manufacturer = "Dahua"
		} else if strings.HasPrefix(modelUpper, "CP-") || strings.HasPrefix(modelUpper, "CP ") {
			match.Manufacturer = "CP PLUS"
		} else if strings.HasPrefix(modelUpper, "DS-") || strings.HasPrefix(modelUpper, "HIK") {
			match.Manufacturer = "Hikvision"
		}
	}
}

type ifaceTarget struct {
	iface *net.Interface
	ip    net.IP
}

// ProbeONVIF sends WS-Discovery probes to the network and collects responses
// Executes interface probing in parallel with guaranteed socket cleanup and deadline cancellation
func ProbeONVIF(ctx context.Context, opts WSDiscoveryOptions) ([]WSProbeMatch, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultWSDiscoveryTimeout
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	multicastDst, err := net.ResolveUDPAddr("udp4", WSDiscoveryMulticastIPv4)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve WS-Discovery multicast address: %w", err)
	}

	// Determine source interfaces and IPs to probe from
	targets := make([]ifaceTarget, 0)

	if opts.InterfaceName != "" {
		iface, err := net.InterfaceByName(opts.InterfaceName)
		if err == nil {
			addrs, err := iface.Addrs()
			if err == nil {
				for _, a := range addrs {
					if ipnet, ok := a.(*net.IPNet); ok {
						if ip4 := ipnet.IP.To4(); ip4 != nil && !ip4.IsLoopback() {
							targets = append(targets, ifaceTarget{iface: iface, ip: ip4})
						}
					}
				}
			}
		}
	}

	// If no interface selected or specified interface had no valid IPv4, discover across all active multicast-capable UP interfaces
	if len(targets) == 0 {
		ifaces, _ := net.Interfaces()
		for i := range ifaces {
			iface := &ifaces[i]
			if (iface.Flags&net.FlagUp) != 0 && (iface.Flags&net.FlagMulticast) != 0 {
				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}
				for _, a := range addrs {
					if ipnet, ok := a.(*net.IPNet); ok {
						if ip4 := ipnet.IP.To4(); ip4 != nil && !ip4.IsLoopback() {
							targets = append(targets, ifaceTarget{iface: iface, ip: ip4})
						}
					}
				}
			}
		}
	}

	// Fallback to default wild-card 0.0.0.0:0 if no specific local address found
	if len(targets) == 0 {
		targets = append(targets, ifaceTarget{iface: nil, ip: net.IPv4zero})
	}

	messageID := GenerateMessageID()
	probeMsg := BuildProbeMessage(messageID)

	var mu sync.Mutex
	matchMap := make(map[string]WSProbeMatch)
	var wg sync.WaitGroup

	deadline, ok := probeCtx.Deadline()
	if !ok {
		deadline = time.Now().Add(timeout)
	}

	// Probe all target interfaces in parallel
	for _, target := range targets {
		wg.Add(1)
		go func(tgt ifaceTarget) {
			defer wg.Done()

			localAddr := &net.UDPAddr{IP: tgt.ip, Port: 0}
			conn, err := net.ListenUDP("udp4", localAddr)
			if err != nil {
				return
			}
			defer conn.Close()

			_ = conn.SetDeadline(deadline)

			// Immediate unblock on context cancellation
			stopCh := make(chan struct{})
			go func() {
				select {
				case <-probeCtx.Done():
					_ = conn.SetDeadline(time.Now())
					_ = conn.Close()
				case <-stopCh:
				}
			}()
			defer close(stopCh)

			// Transmit probe multicast envelope
			_, err = conn.WriteToUDP(probeMsg, multicastDst)
			if err != nil {
				return
			}

			// Read incoming replies until timeout or context cancellation
			buf := make([]byte, 65535)
			for {
				select {
				case <-probeCtx.Done():
					return
				default:
				}

				n, remoteAddr, err := conn.ReadFrom(buf)
				if err != nil {
					return
				}

				if n > 0 {
					matches, parseErr := ParseProbeMatches(buf[:n])
					if parseErr == nil {
						mu.Lock()
						for _, m := range matches {
							if m.IP == "" && remoteAddr != nil {
								if udpAddr, ok := remoteAddr.(*net.UDPAddr); ok {
									m.IP = udpAddr.IP.String()
								}
							}
							dedupKey := m.EndpointReference
							if dedupKey == "" {
								dedupKey = fmt.Sprintf("%s:%d", m.IP, m.Port)
							}
							if dedupKey != "" {
								matchMap[dedupKey] = m
							}
						}
						mu.Unlock()
					}
				}
			}
		}(target)
	}

	// Wait for all interface listeners to terminate cleanly
	wg.Wait()

	result := make([]WSProbeMatch, 0, len(matchMap))
	for _, m := range matchMap {
		result = append(result, m)
	}

	return result, nil
}
