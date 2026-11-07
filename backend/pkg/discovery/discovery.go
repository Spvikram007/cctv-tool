package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/models"
)

// MACPrefixVendor maps MAC OUI prefixes to known CCTV manufacturers
var MACPrefixVendor = map[string]models.Manufacturer{
	"bc:54:51": models.VendorHikvision,
	"c0:56:e3": models.VendorHikvision,
	"44:19:b6": models.VendorHikvision,
	"a4:14:37": models.VendorHikvision,
	"38:af:29": models.VendorDahua,
	"90:02:a9": models.VendorDahua,
	"4c:11:bf": models.VendorDahua,
	"e0:50:8b": models.VendorDahua,
	"00:40:8c": models.VendorAxis,
	"ac:cc:8e": models.VendorAxis,
	"00:16:6c": models.VendorHanwha,
	"34:bd:fa": models.VendorUniview,
	"f0:c8:50": models.VendorCPPlus,
}

type ScanProgress struct {
	TotalIPs   int             `json:"total_ips"`
	ScannedIPs int             `json:"scanned_ips"`
	FoundCount int             `json:"found_count"`
	Percentage int             `json:"percentage"`
	Status     string          `json:"status"` // IDLE, SCANNING, FINISHED
	Found      []models.Device `json:"found"`
}

type Engine struct {
	registry *adapters.Registry
	mu       sync.RWMutex
	progress ScanProgress
	history  []models.Device
}

func NewEngine(registry *adapters.Registry) *Engine {
	return &Engine{
		registry: registry,
		progress: ScanProgress{Status: "IDLE", Found: make([]models.Device, 0)},
		history:  make([]models.Device, 0),
	}
}

func (e *Engine) GetProgress() ScanProgress {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.progress
}

// LookupManufacturerByMAC checks OUI prefix
func LookupManufacturerByMAC(mac string) models.Manufacturer {
	clean := strings.ToLower(strings.ReplaceAll(mac, "-", ":"))
	if len(clean) >= 8 {
		prefix := clean[:8]
		if m, ok := MACPrefixVendor[prefix]; ok {
			return m
		}
	}
	return models.VendorGeneric
}

// ScanSubnet scans an IP range or CIDR (e.g., "192.168.1.0/24" or "127.0.0.1/32")
func (e *Engine) ScanSubnet(ctx context.Context, cidr string, username, password string) ([]models.Device, error) {
	ips, err := parseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR or IP range: %w", err)
	}

	e.mu.Lock()
	e.progress = ScanProgress{
		TotalIPs:   len(ips),
		ScannedIPs: 0,
		FoundCount: 0,
		Percentage: 0,
		Status:     "SCANNING",
		Found:      make([]models.Device, 0),
	}
	e.mu.Unlock()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 32) // max 32 concurrent probes
	resultsChan := make(chan *models.Device, len(ips))

	for _, ip := range ips {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
			}
			defer func() { <-semaphore }()

			dev := e.probeIP(targetIP, username, password)
			if dev != nil {
				resultsChan <- dev
			}

			e.mu.Lock()
			e.progress.ScannedIPs++
			if e.progress.TotalIPs > 0 {
				e.progress.Percentage = (e.progress.ScannedIPs * 100) / e.progress.TotalIPs
			}
			e.mu.Unlock()
		}(ip)
	}

	wg.Wait()
	close(resultsChan)

	found := make([]models.Device, 0)
	for dev := range resultsChan {
		found = append(found, *dev)
	}

	e.mu.Lock()
	e.progress.Found = found
	e.progress.FoundCount = len(found)
	e.progress.Status = "FINISHED"
	e.progress.Percentage = 100
	e.history = append(e.history, found...)
	e.mu.Unlock()

	return found, nil
}

func (e *Engine) probeIP(ip string, username, password string) *models.Device {
	// Probe ports: 80 (HTTP/ONVIF), 554 (RTSP), 8000 (Hikvision SDK), 37777 (Dahua NetSDK), 8080 (Alt HTTP)
	commonPorts := []int{80, 554, 8000, 37777, 8080}
	openPorts := make([]int, 0)
	for _, p := range commonPorts {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, p), 300*time.Millisecond)
		if err == nil {
			openPorts = append(openPorts, p)
			conn.Close()
		}
	}

	if len(openPorts) == 0 {
		return nil
	}

	// Try probing with registered adapters
	for _, m := range []models.Manufacturer{models.VendorHikvision, models.VendorDahua, models.VendorCPPlus, models.VendorGeneric} {
		if adapter, ok := e.registry.GetByVendor(m); ok {
			for _, p := range openPorts {
				if p == 554 {
					continue
				}
				dev, err := adapter.Detect(ip, p, username, password)
				if err == nil && dev != nil {
					return dev
				}
			}
		}
	}

	// Fallback generic identified device
	return &models.Device{
		ID:           fmt.Sprintf("dev-%s", strings.ReplaceAll(ip, ".", "-")),
		Name:         fmt.Sprintf("Network Video Unit (%s)", ip),
		IP:           ip,
		Port:         openPorts[0],
		RTSPPort:     554,
		ONVIFPort:    openPorts[0],
		Manufacturer: models.VendorGeneric,
		Model:        "Standard Streamer",
		Category:     models.CategoryIPCamera,
		Status:       "online",
		Protocols:    []models.ProtocolType{models.ProtocolRTSP, models.ProtocolHTTP},
		Capabilities: []string{"STREAMING", "SNAPSHOT"},
		ChannelCount: 1,
		LastSeen:     time.Now().UTC(),
	}
}

// parseCIDR handles single IP or CIDR notation
func parseCIDR(cidr string) ([]string, error) {
	if !strings.Contains(cidr, "/") {
		// Single IP
		if ip := net.ParseIP(cidr); ip != nil {
			return []string{cidr}, nil
		}
		return nil, fmt.Errorf("invalid IP address")
	}

	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for current := ip.Mask(ipnet.Mask); ipnet.Contains(current); inc(current) {
		ips = append(ips, current.String())
	}

	// Remove network and broadcast address if /24 or smaller
	if len(ips) > 2 {
		return ips[1 : len(ips)-1], nil
	}
	return ips, nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
