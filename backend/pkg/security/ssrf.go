package security

import (
	"errors"
	"net"
	"path/filepath"
	"strings"
)

var (
	ErrSSRFIpForbidden       = errors.New("SSRF Protection: destination IP address is blocked or reserved")
	ErrPathTraversalDetected = errors.New("security: path traversal attempt detected")
)

// ValidateSurveillanceIP ensures an IP address is a valid unicast IPv4 surveillance target
// Blocks cloud metadata IPs (169.254.169.254), multicast, loopback (unless in dev mode), and zero addresses
func ValidateSurveillanceIP(ipStr string, allowLoopback bool) (net.IP, error) {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return nil, errors.New("invalid IP address syntax")
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return nil, errors.New("only IPv4 surveillance targets are currently permitted")
	}

	// Block AWS/GCP/Azure link-local metadata address (169.254.169.254 or any 169.254.0.0/16)
	if ipv4[0] == 169 && ipv4[1] == 254 {
		return nil, ErrSSRFIpForbidden
	}

	// Block Multicast (224.0.0.0 - 239.255.255.255) from unicast API queries
	if ipv4.IsMulticast() {
		return nil, ErrSSRFIpForbidden
	}

	// Block Broadcast (255.255.255.255) or Zero (0.0.0.0)
	if ipv4.Equal(net.IPv4bcast) || ipv4.Equal(net.IPv4zero) {
		return nil, ErrSSRFIpForbidden
	}

	// Loopback (127.0.0.0/8)
	if ipv4.IsLoopback() {
		if !allowLoopback {
			return nil, ErrSSRFIpForbidden
		}
	}

	return ipv4, nil
}

// SanitizePath prevents directory traversal attacks by stripping path delimiters
func SanitizePath(filename string) (string, error) {
	cleaned := filepath.Base(filename)
	if cleaned == "." || cleaned == "/" || cleaned == "\\" || strings.Contains(filename, "..") {
		return "", ErrPathTraversalDetected
	}
	return cleaned, nil
}
