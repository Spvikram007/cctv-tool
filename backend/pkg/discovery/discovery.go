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

// MACPrefixVendor maps MAC OUI prefixes to known CCTV manufacturers (backward compatibility)
var MACPrefixVendor = ComprehensiveMACPrefixVendor

type ScanProgress struct {
	TotalIPs   int             `json:"total_ips"`
	ScannedIPs int             `json:"scanned_ips"`
	FoundCount int             `json:"found_count"`
	Percentage int             `json:"percentage"`
	Status     string          `json:"status"` // IDLE, SCANNING, FINISHED, CANCELLED
	Found      []models.Device `json:"found"`
}

// DiscoveryOptions specifies discovery parameters
type DiscoveryOptions struct {
	Method      string   `json:"method"` // ONVIF, TCP, HYBRID
	Interface   string   `json:"interface,omitempty"`
	CIDR        string   `json:"cidr,omitempty"`
	Ports       []int    `json:"ports,omitempty"`
	TimeoutMs   int      `json:"timeout_ms,omitempty"`
	Concurrency int      `json:"concurrency,omitempty"`
	Username    string   `json:"username,omitempty"`
	Password    string   `json:"password,omitempty"`
}

// Engine coordinates network discovery and device identification
type Engine struct {
	registry      *adapters.Registry
	Fingerprinter *Fingerprinter
	JobManager    *JobManager
	mu            sync.RWMutex
	progress      ScanProgress
	history       []models.Device
}

func NewEngine(registry *adapters.Registry) *Engine {
	return &Engine{
		registry:      registry,
		Fingerprinter: NewFingerprinter(),
		JobManager:    NewJobManager(),
		progress:      ScanProgress{Status: "IDLE", Found: make([]models.Device, 0)},
		history:       make([]models.Device, 0),
	}
}

func (e *Engine) GetProgress() ScanProgress {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.progress
}

// LookupManufacturerByMAC checks OUI prefix
func LookupManufacturerByMAC(mac string) models.Manufacturer {
	vendor, ok := LookupMACFromOUI(mac)
	if ok {
		return vendor
	}
	return models.VendorGeneric
}

// StartDiscovery launches an asynchronous discovery job
func (e *Engine) StartDiscovery(ctx context.Context, opts DiscoveryOptions) (*DiscoveryJob, error) {
	method := strings.ToUpper(opts.Method)
	if method == "" {
		method = "HYBRID"
	}
	if method != "ONVIF" && method != "TCP" && method != "HYBRID" {
		method = "HYBRID"
	}

	cidr := opts.CIDR
	if cidr == "" && (method == "TCP" || method == "HYBRID") {
		// Default to local loopback if not provided, or detect from selected interface
		if opts.Interface != "" {
			ifaces, _ := GetAvailableInterfaces()
			for _, iface := range ifaces {
				if iface.Name == opts.Interface && len(iface.Subnets) > 0 {
					cidr = iface.Subnets[0]
					break
				}
			}
		}
		if cidr == "" {
			cidr = "127.0.0.1/32"
		}
	}

	// Calculate target count
	var ips []string
	var err error
	if method == "TCP" || method == "HYBRID" {
		ips, err = parseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR '%s': %w", cidr, err)
		}
	}

	totalTargets := len(ips)
	if method == "ONVIF" {
		totalTargets = 1 // Single multicast probe session
	} else if method == "HYBRID" {
		totalTargets = len(ips) + 1 // Multicast probe + subnet IPs
	}

	jobCtx, cancel := context.WithCancel(ctx)
	job := e.JobManager.CreateJob(method, opts.Interface, cidr, totalTargets, cancel)

	// Update legacy progress structure
	e.mu.Lock()
	e.progress = ScanProgress{
		TotalIPs:   totalTargets,
		ScannedIPs: 0,
		FoundCount: 0,
		Percentage: 0,
		Status:     "SCANNING",
		Found:      make([]models.Device, 0),
	}
	e.mu.Unlock()

	go func() {
		defer cancel()
		var runErr error

		switch method {
		case "ONVIF":
			runErr = e.runONVIFDiscovery(jobCtx, job.ID, opts)
		case "TCP":
			runErr = e.runTCPSubnetDiscovery(jobCtx, job.ID, ips, opts)
		case "HYBRID":
			runErr = e.runHybridDiscovery(jobCtx, job.ID, ips, opts)
		}

		e.JobManager.MarkCompleted(job.ID, runErr)

		// Sync legacy progress
		jobSnap, ok := e.JobManager.GetJob(job.ID)
		if ok {
			e.mu.Lock()
			legacyDevs := make([]models.Device, 0, len(jobSnap.Results))
			for _, r := range jobSnap.Results {
				legacyDevs = append(legacyDevs, r.Device)
			}
			e.progress.Found = legacyDevs
			e.progress.FoundCount = len(legacyDevs)
			e.progress.Percentage = 100
			if jobSnap.Status == JobStatusCancelled {
				e.progress.Status = "CANCELLED"
			} else {
				e.progress.Status = "FINISHED"
			}
			e.history = append(e.history, legacyDevs...)
			e.mu.Unlock()
		}
	}()

	return job, nil
}

// runONVIFDiscovery executes multicast WS-Discovery and fingerprints all responders
func (e *Engine) runONVIFDiscovery(ctx context.Context, jobID string, opts DiscoveryOptions) error {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultWSDiscoveryTimeout
	}

	matches, err := ProbeONVIF(ctx, WSDiscoveryOptions{
		InterfaceName: opts.Interface,
		Timeout:       timeout,
	})
	if err != nil {
		return err
	}

	for _, m := range matches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		mac := LookupSystemARP(m.IP)
		devServices := DeviceServices{
			OpenPorts: []int{m.Port},
		}

		// Quick RTSP probe if standard port is reachable
		if rtspBanner, pub, err := ProbeRTSPServer(ctx, m.IP, 554); err == nil {
			devServices.RTSPServer = rtspBanner
			devServices.RTSPPublicOpts = pub
			devServices.OpenPorts = append(devServices.OpenPorts, 554)
		}

		fp := AnalyzeFingerprint(m.IP, mac, devServices.OpenPorts, &m, devServices, "ONVIF")

		dev := models.Device{
			ID:           fmt.Sprintf("dev-%s", strings.ReplaceAll(m.IP, ".", "-")),
			Name:         fmt.Sprintf("%s %s", fp.IdentifiedManufacturer, fp.Model),
			IP:           m.IP,
			MAC:          mac,
			Manufacturer: fp.IdentifiedManufacturer,
			Model:        fp.Model,
			Firmware:     fp.Firmware,
			SerialNumber: fp.SerialNumber,
			Category:     fp.Category,
			Port:         m.Port,
			RTSPPort:     554,
			ONVIFPort:    m.Port,
			Status:       "online",
			Protocols:    fp.Protocols,
			Capabilities: fp.Capabilities,
			ChannelCount: fp.ChannelCount,
			LastSeen:     time.Now().UTC(),
		}

		e.JobManager.AddResult(jobID, DiscoveredDevice{Device: dev, Fingerprint: fp})
	}

	e.JobManager.UpdateProgress(jobID, 1, 1)
	return nil
}

// runTCPSubnetDiscovery executes a multi-port TCP sweep across the target CIDR
func (e *Engine) runTCPSubnetDiscovery(ctx context.Context, jobID string, ips []string, opts DiscoveryOptions) error {
	ports := opts.Ports
	if len(ports) == 0 {
		ports = []int{80, 554, 8000, 37777, 8080, 8001, 8899, 34567}
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 || concurrency > 128 {
		concurrency = 32
	}

	probeTimeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if probeTimeout <= 0 {
		probeTimeout = 300 * time.Millisecond
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)
	var scannedCount int
	var scMu sync.Mutex

	for _, ip := range ips {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case semaphore <- struct{}{}:
		}

		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			select {
			case <-ctx.Done():
				return
			default:
			}

			devItem := e.probeHost(ctx, targetIP, ports, probeTimeout, opts.Username, opts.Password)
			if devItem != nil {
				e.JobManager.AddResult(jobID, *devItem)
			}

			scMu.Lock()
			scannedCount++
			current := scannedCount
			scMu.Unlock()

			e.JobManager.UpdateProgress(jobID, current, len(ips))

			e.mu.Lock()
			e.progress.ScannedIPs = current
			if len(ips) > 0 {
				e.progress.Percentage = (current * 100) / len(ips)
			}
			e.mu.Unlock()
		}(ip)
	}

	wg.Wait()
	return nil
}

// runHybridDiscovery runs WS-Discovery multicast followed by TCP Subnet sweep
func (e *Engine) runHybridDiscovery(ctx context.Context, jobID string, ips []string, opts DiscoveryOptions) error {
	// Phase 1: ONVIF WS-Discovery probe
	_ = e.runONVIFDiscovery(ctx, jobID, opts)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Phase 2: TCP sweep
	return e.runTCPSubnetDiscovery(ctx, jobID, ips, opts)
}

// probeHost investigates a single IP for CCTV ports, headers, and banners
func (e *Engine) probeHost(ctx context.Context, ip string, ports []int, timeout time.Duration, username, password string) *DiscoveredDevice {
	openPorts := make([]int, 0)
	for _, p := range ports {
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ip, p))
		if err == nil {
			openPorts = append(openPorts, p)
			conn.Close()
		}
	}

	if len(openPorts) == 0 {
		return nil
	}

	// Collect services details
	services := DeviceServices{
		OpenPorts: openPorts,
	}

	// Probe RTSP if port open
	if containsPort(openPorts, 554) {
		rtspBanner, pub, err := ProbeRTSPServer(ctx, ip, 554)
		if err == nil {
			services.RTSPServer = rtspBanner
			services.RTSPPublicOpts = pub
		}
	}

	// Probe HTTP / HTTPS headers
	for _, p := range openPorts {
		if p == 80 || p == 8080 || p == 8000 || p == 8899 {
			srv, auth, _ := e.Fingerprinter.ProbeHTTPServer(ctx, ip, p)
			if srv != "" || auth != "" {
				services.HTTPServer = srv
				services.HTTPAuthHeader = auth
				break
			}
		}
	}

	// Probe SSL cert if 443 / 8443 open
	if containsPort(openPorts, 443) || containsPort(openPorts, 8443) {
		sslPort := 443
		if !containsPort(openPorts, 443) {
			sslPort = 8443
		}
		issuer, subject, err := ProbeSSLCert(ctx, ip, sslPort)
		if err == nil {
			services.SSLCertIssuer = issuer
			services.SSLCertSubject = subject
		}
	}

	mac := LookupSystemARP(ip)
	fp := AnalyzeFingerprint(ip, mac, openPorts, nil, services, "TCP_SWEEP")

	// Check if registered adapters can detect more details
	if e.registry != nil {
		for _, m := range []models.Manufacturer{models.VendorHikvision, models.VendorDahua, models.VendorCPPlus, models.VendorGeneric} {
			if adapter, ok := e.registry.GetByVendor(m); ok {
				for _, p := range openPorts {
					if p == 554 {
						continue
					}
					dev, err := adapter.Detect(ip, p, username, password)
					if err == nil && dev != nil {
						fp.Model = dev.Model
						fp.IdentifiedManufacturer = dev.Manufacturer
						fp.Confidence = 95
						fp.ConfidenceLevel = "HIGH"
						break
					}
				}
			}
		}
	}

	primaryPort := openPorts[0]
	onvifPort := 80
	if containsPort(openPorts, 80) {
		primaryPort = 80
		onvifPort = 80
	} else if containsPort(openPorts, 8080) {
		onvifPort = 8080
	} else if containsPort(openPorts, 8899) {
		onvifPort = 8899
	}

	dev := models.Device{
		ID:           fmt.Sprintf("dev-%s", strings.ReplaceAll(ip, ".", "-")),
		Name:         fmt.Sprintf("%s %s (%s)", fp.IdentifiedManufacturer, fp.Model, ip),
		IP:           ip,
		MAC:          mac,
		Manufacturer: fp.IdentifiedManufacturer,
		Model:        fp.Model,
		Firmware:     fp.Firmware,
		SerialNumber: fp.SerialNumber,
		Category:     fp.Category,
		Port:         primaryPort,
		RTSPPort:     554,
		ONVIFPort:    onvifPort,
		Status:       "online",
		Protocols:    fp.Protocols,
		Capabilities: fp.Capabilities,
		ChannelCount: fp.ChannelCount,
		LastSeen:     time.Now().UTC(),
	}

	return &DiscoveredDevice{
		Device:      dev,
		Fingerprint: fp,
	}
}

// ScanSubnet preserves legacy interface
func (e *Engine) ScanSubnet(ctx context.Context, cidr string, username, password string) ([]models.Device, error) {
	job, err := e.StartDiscovery(ctx, DiscoveryOptions{
		Method:   "TCP",
		CIDR:     cidr,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	// Poll until completion or context cancellation
	for {
		select {
		case <-ctx.Done():
			e.JobManager.CancelJob(job.ID)
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			jobSnap, ok := e.JobManager.GetJob(job.ID)
			if !ok {
				return nil, fmt.Errorf("job not found")
			}
			if jobSnap.Status == JobStatusCompleted || jobSnap.Status == JobStatusCancelled || jobSnap.Status == JobStatusFailed {
				devs := make([]models.Device, 0, len(jobSnap.Results))
				for _, r := range jobSnap.Results {
					devs = append(devs, r.Device)
				}
				return devs, nil
			}
		}
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
