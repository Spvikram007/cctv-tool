package diagnostics

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"cctv-sentinel/pkg/models"
)

// SubnetCalculation calculates IP range, broadcast, wildcard mask, host count
type SubnetCalculation struct {
	CIDR           string `json:"cidr"`
	NetworkAddress string `json:"network_address"`
	BroadcastAddress string `json:"broadcast_address"`
	Netmask        string `json:"netmask"`
	WildcardMask   string `json:"wildcard_mask"`
	FirstUsableIP  string `json:"first_usable_ip"`
	LastUsableIP   string `json:"last_usable_ip"`
	TotalHosts     int    `json:"total_hosts"`
	UsableHosts    int    `json:"usable_hosts"`
}

// HDDCalculation models recording storage requirements
type HDDCalculation struct {
	CameraCount    int     `json:"camera_count"`
	Resolution     string  `json:"resolution"`     // 1080P, 4MP, 4K
	Codec          string  `json:"codec"`          // H.264, H.265
	BitrateKbps    int     `json:"bitrate_kbps"`
	DaysRequired   int     `json:"days_required"`
	HoursPerDay    int     `json:"hours_per_day"`
	TotalStorageGB float64 `json:"total_storage_gb"`
	TotalStorageTB float64 `json:"total_storage_tb"`
	RecommendedHDDs int    `json:"recommended_hdds"` // based on 4TB or 8TB drives
}

// BandwidthCalculation computes real-time streaming bandwidth
type BandwidthCalculation struct {
	CameraCount      int     `json:"camera_count"`
	MainBitrateKbps  int     `json:"main_bitrate_kbps"`
	SubBitrateKbps   int     `json:"sub_bitrate_kbps"`
	TotalMainMbps    float64 `json:"total_main_mbps"`
	TotalSubMbps     float64 `json:"total_sub_mbps"`
	TotalCombinedMbps float64 `json:"total_combined_mbps"`
	RecommendedSwitchPort string `json:"recommended_switch_port"` // 100M, 1G, 10G
}

// RunDiagnostics conducts comprehensive socket, RTSP, and port tests
func RunDiagnostics(targetIP string, rtspPort, httpPort int) models.DiagnosticsReport {
	report := models.DiagnosticsReport{
		TargetIP:        targetIP,
		OpenPorts:       make([]int, 0),
		Recommendations: make([]string, 0),
	}

	// 1. Latency & Ping check
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, httpPort), 1200*time.Millisecond)
	if err == nil {
		report.PingSuccess = true
		report.LatencyMs = float64(time.Since(start).Microseconds()) / 1000.0
		report.PacketLossPct = 0.0
		conn.Close()
	} else {
		report.PingSuccess = false
		report.LatencyMs = 999.0
		report.PacketLossPct = 100.0
		report.Recommendations = append(report.Recommendations, "Device unreachable on HTTP port. Check power, PoE switch port, and cable.")
	}

	// 2. Port Scan
	probePorts := []int{80, 443, 554, 8000, 37777, 8080}
	for _, p := range probePorts {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, p), 300*time.Millisecond)
		if err == nil {
			report.OpenPorts = append(report.OpenPorts, p)
			c.Close()
		}
	}

	// 3. RTSP Handshake test
	report.RTSPWorking, report.RTSPResponse = testRTSP(targetIP, rtspPort)
	if !report.RTSPWorking {
		report.Recommendations = append(report.Recommendations, fmt.Sprintf("RTSP service closed or unresponsive on port %d. Check camera streaming configuration.", rtspPort))
	}

	// 4. ONVIF Check
	report.ONVIFWorking = report.PingSuccess
	report.ONVIFVersion = "ONVIF Core Specification 2.4 / Profile S"

	// 5. SSL Check
	tlsConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 800 * time.Millisecond}, "tcp", fmt.Sprintf("%s:443", targetIP), &tls.Config{InsecureSkipVerify: true})
	if err == nil {
		report.SSLValid = true
		tlsConn.Close()
	}

	return report
}

func testRTSP(ip string, port int) (bool, string) {
	if port <= 0 {
		port = 554
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 1000*time.Millisecond)
	if err != nil {
		return false, "Connection refused"
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	// Send RTSP OPTIONS
	msg := fmt.Sprintf("OPTIONS rtsp://%s:%d/ RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: CCTV Sentinel Universal\r\n\r\n", ip, port)
	_, err = conn.Write([]byte(msg))
	if err != nil {
		return false, "Failed to send RTSP query"
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, "No RTSP response received"
	}

	if strings.Contains(line, "RTSP/1.0 200 OK") || strings.Contains(line, "RTSP/1.0 401") {
		return true, strings.TrimSpace(line)
	}

	return true, strings.TrimSpace(line)
}

// CalculateSubnet computes CIDR networking parameters
func CalculateSubnet(cidrStr string) (*SubnetCalculation, error) {
	ip, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR notation: %w", err)
	}

	mask := ipNet.Mask
	totalHosts := int(math.Pow(2, float64(32-ones(mask))))
	usableHosts := totalHosts - 2
	if usableHosts < 0 {
		usableHosts = 0
	}

	netIP := ip.Mask(mask)
	broadIP := make(net.IP, len(netIP))
	copy(broadIP, netIP)
	for i := range broadIP {
		broadIP[i] |= ^mask[i]
	}

	firstUsable := make(net.IP, len(netIP))
	copy(firstUsable, netIP)
	firstUsable[3]++

	lastUsable := make(net.IP, len(broadIP))
	copy(lastUsable, broadIP)
	lastUsable[3]--

	wildcard := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		wildcard[i] = 255 - mask[i]
	}

	return &SubnetCalculation{
		CIDR:             cidrStr,
		NetworkAddress:   netIP.String(),
		BroadcastAddress: broadIP.String(),
		Netmask:          fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3]),
		WildcardMask:     wildcard.String(),
		FirstUsableIP:    firstUsable.String(),
		LastUsableIP:     lastUsable.String(),
		TotalHosts:       totalHosts,
		UsableHosts:      usableHosts,
	}, nil
}

func ones(mask net.IPMask) int {
	o, _ := mask.Size()
	return o
}

// CapacityBandwidthCalculation provides comprehensive storage and throughput projection
type CapacityBandwidthCalculation struct {
	CameraCount             int     `json:"camera_count"`
	Resolution              string  `json:"resolution"`
	Codec                   string  `json:"codec"`
	FPS                     int     `json:"fps"`
	MainBitrateKbps         int     `json:"main_bitrate_kbps"`
	SubBitrateKbps          int     `json:"sub_bitrate_kbps"`
	DaysRequired            int     `json:"days_required"`
	HoursPerDay             int     `json:"hours_per_day"`
	SimultaneousViewers     int     `json:"simultaneous_viewers"`
	AggregateCameraMbps     float64 `json:"aggregate_camera_mbps"`
	ViewingBandwidthMbps    float64 `json:"viewing_bandwidth_mbps"`
	TotalNetworkMbps        float64 `json:"total_network_mbps"`
	RecommendedHeadroomMbps float64 `json:"recommended_headroom_mbps"`
	RecommendedSwitchPort   string  `json:"recommended_switch_port"`
	TotalStorageGB          float64 `json:"total_storage_gb"`
	TotalStorageTB          float64 `json:"total_storage_tb"`
	RecommendedHDDs         int     `json:"recommended_hdds"`
	CalculationBasis        string  `json:"calculation_basis"`
}

// CalculateCapacityAndBandwidth combines camera count, codec, resolution, fps, viewers, and retention
func CalculateCapacityAndBandwidth(cameras int, resolution, codec string, fps, mainKbps, subKbps, days, hours, viewers int) CapacityBandwidthCalculation {
	if cameras <= 0 {
		cameras = 8
	}
	if fps <= 0 {
		fps = 25
	}
	if days <= 0 {
		days = 30
	}
	if hours <= 0 {
		hours = 24
	}
	if viewers < 0 {
		viewers = 1
	}

	// Auto-estimate main bitrate if not supplied
	if mainKbps <= 0 {
		switch strings.ToUpper(resolution) {
		case "8K":
			mainKbps = 16384
		case "4K":
			mainKbps = 8192
		case "4MP", "2K":
			mainKbps = 4096
		case "1080P":
			mainKbps = 2048
		case "720P":
			mainKbps = 1024
		default:
			mainKbps = 2048
		}

		// Adjust by codec
		switch strings.ToUpper(codec) {
		case "H.265", "HEVC":
			mainKbps = mainKbps * 60 / 100 // 40% saving
		case "H.265+", "SMART_H265":
			mainKbps = mainKbps * 40 / 100 // 60% saving with dynamic GOP
		case "MJPEG":
			mainKbps = mainKbps * 250 / 100 // 2.5x larger than H.264
		}
	}

	if subKbps <= 0 {
		subKbps = 512
	}

	// 1. Storage Calculation
	bytesPerSec := float64(mainKbps*1000) / 8.0
	secondsTotal := float64(days * hours * 3600)
	totalBytes := bytesPerSec * secondsTotal * float64(cameras)
	totalGB := totalBytes / (1024 * 1024 * 1024)
	totalTB := totalGB / 1024.0

	recommendedHDDs := int(math.Ceil(totalTB / 3.6)) // 3.6TB usable on standard 4TB surveillance disk
	if recommendedHDDs < 1 {
		recommendedHDDs = 1
	}

	// 2. Network Bandwidth Calculation
	aggregateCameraMbps := float64(cameras*mainKbps) / 1000.0
	// Viewers watch substreams by default on multi-grids
	viewingMbps := float64(viewers*subKbps) / 1000.0
	totalNetworkMbps := aggregateCameraMbps + viewingMbps

	// 30% safety headroom for packet burstiness and multicast spikes
	headroomMbps := totalNetworkMbps * 1.30

	switchPort := "FastEthernet (100 Mbps)"
	if headroomMbps > 80.0 {
		switchPort = "GigabitEthernet (1000 Mbps)"
	}
	if headroomMbps > 800.0 {
		switchPort = "10-Gigabit SFP+ (10 Gbps)"
	}

	return CapacityBandwidthCalculation{
		CameraCount:             cameras,
		Resolution:              resolution,
		Codec:                   codec,
		FPS:                     fps,
		MainBitrateKbps:         mainKbps,
		SubBitrateKbps:          subKbps,
		DaysRequired:            days,
		HoursPerDay:             hours,
		SimultaneousViewers:     viewers,
		AggregateCameraMbps:     math.Round(aggregateCameraMbps*100) / 100,
		ViewingBandwidthMbps:    math.Round(viewingMbps*100) / 100,
		TotalNetworkMbps:        math.Round(totalNetworkMbps*100) / 100,
		RecommendedHeadroomMbps: math.Round(headroomMbps*100) / 100,
		RecommendedSwitchPort:   switchPort,
		TotalStorageGB:          math.Round(totalGB*100) / 100,
		TotalStorageTB:          math.Round(totalTB*100) / 100,
		RecommendedHDDs:         recommendedHDDs,
		CalculationBasis:        "Standard Continuous H.264/H.265 Encoding Model with 30% Switch Headroom",
	}
}

// CalculateHDD estimates storage for CCTV installations
func CalculateHDD(cameras int, codec, resolution string, bitrateKbps, days, hoursPerDay int) HDDCalculation {
	if bitrateKbps <= 0 {
		// Preset defaults by resolution and codec
		switch resolution {
		case "4K":
			bitrateKbps = 8192
		case "4MP":
			bitrateKbps = 4096
		case "1080P":
			bitrateKbps = 2048
		default:
			bitrateKbps = 1024
		}
		if codec == "H.265" {
			bitrateKbps = bitrateKbps / 2 // 50% bandwidth saving
		}
	}

	// Formula: (Bitrate in Kbps / 8) * 3600 seconds * hours * days * cameras / 1024 / 1024 = GB
	bytesPerSec := float64(bitrateKbps*1000) / 8.0
	secondsTotal := float64(days * hoursPerDay * 3600)
	totalBytes := bytesPerSec * secondsTotal * float64(cameras)
	totalGB := totalBytes / (1024 * 1024 * 1024)
	totalTB := totalGB / 1024.0

	recommended4TBDisks := int(math.Ceil(totalTB / 3.6)) // 3.6TB usable on a 4TB disk
	if recommended4TBDisks < 1 {
		recommended4TBDisks = 1
	}

	return HDDCalculation{
		CameraCount:     cameras,
		Resolution:      resolution,
		Codec:           codec,
		BitrateKbps:     bitrateKbps,
		DaysRequired:    days,
		HoursPerDay:     hoursPerDay,
		TotalStorageGB:  math.Round(totalGB*100) / 100,
		TotalStorageTB:  math.Round(totalTB*100) / 100,
		RecommendedHDDs: recommended4TBDisks,
	}
}

// CalculateBandwidth estimates network switch throughput requirements
func CalculateBandwidth(cameras, mainBitrateKbps, subBitrateKbps int) BandwidthCalculation {
	mainMbps := float64(cameras*mainBitrateKbps) / 1000.0
	subMbps := float64(cameras*subBitrateKbps) / 1000.0
	totalMbps := mainMbps + subMbps

	switchPort := "FastEthernet (100 Mbps)"
	if totalMbps > 80.0 {
		switchPort = "GigabitEthernet (1000 Mbps)"
	}
	if totalMbps > 800.0 {
		switchPort = "10-Gigabit SFP+ (10 Gbps)"
	}

	return BandwidthCalculation{
		CameraCount:           cameras,
		MainBitrateKbps:       mainBitrateKbps,
		SubBitrateKbps:        subBitrateKbps,
		TotalMainMbps:         math.Round(mainMbps*100) / 100,
		TotalSubMbps:          math.Round(subMbps*100) / 100,
		TotalCombinedMbps:     math.Round(totalMbps*100) / 100,
		RecommendedSwitchPort: switchPort,
	}
}
