package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrSSRFIpForbidden       = errors.New("SSRF Protection: destination IP address is blocked, reserved, or unauthorized")
	ErrPathTraversalDetected = errors.New("security: path traversal attempt detected")
	ErrDNSRebindingDetected  = errors.New("SSRF Protection: DNS resolution resolved to forbidden or metadata IP address")
	ErrUnsafeRedirectTarget  = errors.New("SSRF Protection: redirect target violates network security policy")
)

// Blocked Cloud Metadata IPs
var cloudMetadataIPs = []string{
	"169.254.169.254", // AWS, Azure, GCP, DigitalOcean, OpenStack IPv4
	"100.100.100.200", // Alibaba Cloud metadata
	"fd00:ec2::254",   // AWS EC2 IPv6 metadata
}

// ValidateSurveillanceIP ensures an IP address is a valid unicast IPv4 or IPv6 surveillance target
// Blocks cloud metadata IPs, multicast, loopback (unless in dev/test mode), and zero/broadcast addresses
func ValidateSurveillanceIP(ipStr string, allowLoopback bool) (net.IP, error) {
	cleanStr := strings.TrimSpace(ipStr)
	// Strip surrounding brackets for IPv6 if present
	cleanStr = strings.TrimPrefix(cleanStr, "[")
	cleanStr = strings.TrimSuffix(cleanStr, "]")

	ip := net.ParseIP(cleanStr)
	if ip == nil {
		return nil, errors.New("invalid IP address syntax")
	}

	// 1. Explicit Cloud Metadata Protection (IPv4 and IPv6)
	for _, meta := range cloudMetadataIPs {
		metaIP := net.ParseIP(meta)
		if metaIP != nil && ip.Equal(metaIP) {
			return nil, ErrSSRFIpForbidden
		}
	}

	// Handle IPv4 or IPv4-mapped IPv6
	ipv4 := ip.To4()
	if ipv4 != nil {
		// Block AWS/GCP/Azure link-local metadata address range (169.254.0.0/16)
		if ipv4[0] == 169 && ipv4[1] == 254 {
			return nil, ErrSSRFIpForbidden
		}

		// Block Alibaba cloud metadata
		if ipv4[0] == 100 && ipv4[1] == 100 && ipv4[2] == 100 && ipv4[3] == 200 {
			return nil, ErrSSRFIpForbidden
		}

		// Block Multicast (224.0.0.0/4)
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

	// IPv6 Handling
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return nil, ErrSSRFIpForbidden
	}

	if ip.IsMulticast() || ip.IsUnspecified() {
		return nil, ErrSSRFIpForbidden
	}

	if ip.IsLoopback() {
		if !allowLoopback {
			return nil, ErrSSRFIpForbidden
		}
	}

	return ip, nil
}

// ValidateSurveillanceTarget is a helper wrapper for ValidateSurveillanceIP
func ValidateSurveillanceTarget(ipStr string, allowLoopback bool) error {
	_, err := ValidateSurveillanceIP(ipStr, allowLoopback)
	return err
}

// ResolveAndValidateHost resolves a hostname and verifies EVERY returned IP against SSRF policy to prevent DNS rebinding
func ResolveAndValidateHost(host string, allowLoopback bool) (net.IP, error) {
	trimmedHost := strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(trimmedHost); err == nil {
		trimmedHost = h
	}

	// Direct IP check
	if directIP := net.ParseIP(trimmedHost); directIP != nil {
		return ValidateSurveillanceIP(trimmedHost, allowLoopback)
	}

	// Resolve all IPs via DNS
	ips, err := net.LookupIP(trimmedHost)
	if err != nil {
		return nil, fmt.Errorf("SSRF Protection: failed to resolve host '%s': %w", trimmedHost, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("SSRF Protection: no IP addresses returned for host '%s'", trimmedHost)
	}

	// Validate EVERY returned IP. If even one points to cloud metadata or forbidden address, abort
	var safeIP net.IP
	for _, ip := range ips {
		validated, err := ValidateSurveillanceIP(ip.String(), allowLoopback)
		if err != nil {
			return nil, fmt.Errorf("%w: host '%s' resolved to forbidden IP '%s'", ErrDNSRebindingDetected, trimmedHost, ip.String())
		}
		if safeIP == nil {
			safeIP = validated
		}
	}

	return safeIP, nil
}

// NewSafeHTTPClient creates an http.Client with DNS-rebinding protection, post-resolution IP validation, and strict redirect policy
func NewSafeHTTPClient(timeout time.Duration, allowLoopback bool) *http.Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			safeIP, err := ResolveAndValidateHost(host, allowLoopback)
			if err != nil {
				return nil, err
			}

			// Dial the validated IP directly to prevent DNS rebinding Time-Of-Check to Time-Of-Use
			targetAddr := net.JoinHostPort(safeIP.String(), port)
			return dialer.DialContext(ctx, network, targetAddr)
		},
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			// Validate redirect destination against SSRF policy
			_, err := ResolveAndValidateHost(req.URL.Hostname(), allowLoopback)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrUnsafeRedirectTarget, err)
			}
			return nil
		},
	}

	return client
}

// SanitizePath prevents directory traversal attacks by stripping path delimiters
func SanitizePath(filename string) (string, error) {
	cleaned := filepath.Base(filename)
	if cleaned == "." || cleaned == "/" || cleaned == "\\" || strings.Contains(filename, "..") {
		return "", ErrPathTraversalDetected
	}
	return cleaned, nil
}
