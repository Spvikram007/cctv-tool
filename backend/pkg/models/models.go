package models

import "time"

// DeviceCategory represents supported hardware categories
type DeviceCategory string

const (
	CategoryIPCamera           DeviceCategory = "IP_CAMERA"
	CategoryAnalogCamera       DeviceCategory = "ANALOG_CAMERA"
	CategoryDVR                DeviceCategory = "DVR"
	CategoryNVR                DeviceCategory = "NVR"
	CategoryXVR                DeviceCategory = "XVR"
	CategoryHVR                DeviceCategory = "HVR"
	CategoryHybridRecorder     DeviceCategory = "HYBRID_RECORDER"
	CategoryPTZCamera          DeviceCategory = "PTZ_CAMERA"
	CategoryEncoder            DeviceCategory = "ENCODER"
	CategoryNetworkVideoServer DeviceCategory = "NETWORK_VIDEO_SERVER"
	CategoryUnknown            DeviceCategory = "UNKNOWN"
)

// Manufacturer represents detected or manual manufacturer
type Manufacturer string

const (
	VendorHikvision Manufacturer = "Hikvision"
	VendorDahua     Manufacturer = "Dahua"
	VendorCPPlus    Manufacturer = "CP PLUS"
	VendorUniview   Manufacturer = "Uniview"
	VendorAxis      Manufacturer = "Axis"
	VendorHanwha    Manufacturer = "Hanwha"
	VendorTiandy    Manufacturer = "Tiandy"
	VendorTVT       Manufacturer = "TVT"
	VendorGeneric   Manufacturer = "Generic ONVIF"
	VendorOEM       Manufacturer = "OEM Rebranded"
)

// ProtocolType represents protocol supported by device
type ProtocolType string

const (
	ProtocolONVIF    ProtocolType = "ONVIF"
	ProtocolRTSP     ProtocolType = "RTSP"
	ProtocolISAPI    ProtocolType = "ISAPI"
	ProtocolDahuaCGI ProtocolType = "DAHUA_CGI"
	ProtocolAxisVAPIX ProtocolType = "AXIS_VAPIX"
	ProtocolHTTP     ProtocolType = "HTTP"
	ProtocolHTTPS    ProtocolType = "HTTPS"
)

// Device represents an identified CCTV unit
type Device struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	IP             string         `json:"ip"`
	MAC            string         `json:"mac"`
	Manufacturer   Manufacturer   `json:"manufacturer"`
	Model          string         `json:"model"`
	Firmware       string         `json:"firmware"`
	SerialNumber   string         `json:"serial_number"`
	Category       DeviceCategory `json:"category"`
	Port           int            `json:"port"`
	RTSPPort       int            `json:"rtsp_port"`
	ONVIFPort      int            `json:"onvif_port"`
	Status         string         `json:"status"` // online, offline, unauthorized
	Protocols      []ProtocolType `json:"protocols"`
	Capabilities   []string       `json:"capabilities"`
	ChannelCount   int            `json:"channel_count"`
	Username       string         `json:"username,omitempty"`
	Password       string         `json:"-"` // Never serialized in JSON
	LastSeen       time.Time      `json:"last_seen"`
	SiteID         string         `json:"site_id,omitempty"`
	TenantID       string         `json:"tenant_id,omitempty"`
}

// NetworkConfig models device network configuration
type NetworkConfig struct {
	DeviceID   string   `json:"device_id"`
	DHCP       bool     `json:"dhcp"`
	IPAddress  string   `json:"ip_address"`
	SubnetMask string   `json:"subnet_mask"`
	Gateway    string   `json:"gateway"`
	DNS1       string   `json:"dns1"`
	DNS2       string   `json:"dns2"`
	HTTPPort   int      `json:"http_port"`
	RTSPPort   int      `json:"rtsp_port"`
	ONVIFPort  int      `json:"onvif_port"`
	NTPEnabled bool     `json:"ntp_enabled"`
	NTPServer  string   `json:"ntp_server"`
}

// ImageConfig models color and sensor adjustments
type ImageConfig struct {
	DeviceID        string `json:"device_id"`
	Brightness      int    `json:"brightness"`       // 0-100
	Contrast        int    `json:"contrast"`         // 0-100
	Saturation      int    `json:"saturation"`       // 0-100
	Sharpness       int    `json:"sharpness"`        // 0-100
	WDRMode         string `json:"wdr_mode"`         // off, on, auto
	WDRLevel        int    `json:"wdr_level"`        // 0-100
	IRMode          string `json:"ir_mode"`          // auto, day, night
	BacklightComp   bool   `json:"backlight_comp"`
}

// VideoEncodingConfig represents stream resolution, codec, bitrate
type VideoEncodingConfig struct {
	DeviceID    string `json:"device_id"`
	StreamIndex int    `json:"stream_index"` // 0 = main, 1 = sub
	StreamName  string `json:"stream_name"`  // MainStream, SubStream
	Codec       string `json:"codec"`        // H.264, H.265, MJPEG
	Resolution  string `json:"resolution"`   // 3840x2160, 2560x1440, 1920x1080, 1280x720, 704x576
	FrameRate   int    `json:"frame_rate"`   // 1-30 fps
	BitrateKbps int    `json:"bitrate_kbps"` // e.g. 2048, 4096, 8192
	RateControl string `json:"rate_control"` // CBR, VBR
	RTSPURL     string `json:"rtsp_url"`
}

// RecorderChannel represents an XVR/DVR/NVR channel
type RecorderChannel struct {
	ChannelID     int    `json:"channel_id"`
	Name          string `json:"name"`
	Type          string `json:"type"`          // ANALOG or IP
	SignalFormat  string `json:"signal_format"` // TVI, CVI, AHD, CVBS, IP
	Status        string `json:"status"`        // connected, no_video
	Resolution    string `json:"resolution"`
	RecordMode    string `json:"record_mode"`   // continuous, motion, schedule, none
	AudioEnabled  bool   `json:"audio_enabled"`
	PTZSupported  bool   `json:"ptz_supported"`
}

// StorageDevice represents an internal HDD or SD card
type StorageDevice struct {
	ID          string `json:"id"`
	DiskIndex   int    `json:"disk_index"`
	Model       string `json:"model"`
	CapacityGB  int64  `json:"capacity_gb"`
	FreeGB      int64  `json:"free_gb"`
	Status      string `json:"status"` // normal, error, formatting
	SMARTHealth string `json:"smart_health"`
}

// PTZAction represents a pan/tilt/zoom operation
type PTZAction struct {
	DeviceID string  `json:"device_id"`
	Command  string  `json:"command"` // UP, DOWN, LEFT, RIGHT, ZOOM_IN, ZOOM_OUT, STOP, GOTO_PRESET, SET_PRESET
	Pan      float64 `json:"pan"`
	Tilt     float64 `json:"tilt"`
	Zoom     float64 `json:"zoom"`
	Speed    float64 `json:"speed"`
	PresetID int     `json:"preset_id,omitempty"`
}

// DiagnosticsReport details comprehensive network and protocol checks
type DiagnosticsReport struct {
	TargetIP       string   `json:"target_ip"`
	PingSuccess    bool     `json:"ping_success"`
	LatencyMs      float64  `json:"latency_ms"`
	PacketLossPct  float64  `json:"packet_loss_pct"`
	OpenPorts      []int    `json:"open_ports"`
	RTSPWorking    bool     `json:"rtsp_working"`
	RTSPResponse   string   `json:"rtsp_response"`
	ONVIFWorking   bool     `json:"onvif_working"`
	ONVIFVersion   string   `json:"onvif_version"`
	HTTPStatus     int      `json:"http_status"`
	SSLValid       bool     `json:"ssl_valid"`
	Recommendations []string `json:"recommendations"`
}

// License models the commercial tier
type License struct {
	Tier         string    `json:"tier"` // FREE, TECHNICIAN_PRO, DEALER_ENTERPRISE
	LicenseKey   string    `json:"license_key"`
	OwnerName    string    `json:"owner_name"`
	Company      string    `json:"company"`
	GSTNumber    string    `json:"gst_number"`
	MaxDevices   int       `json:"max_devices"`
	MaxSites     int       `json:"max_sites"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Active       bool      `json:"active"`
	MachineHash  string    `json:"machine_hash"`
	OfflineKey   string    `json:"offline_key,omitempty"`
}

// CapabilityStatus represents whether a device capability is confirmed supported, unsupported or unknown
type CapabilityStatus string

const (
	CapabilitySupported   CapabilityStatus = "SUPPORTED"
	CapabilityUnsupported CapabilityStatus = "UNSUPPORTED"
	CapabilityUnknown     CapabilityStatus = "UNKNOWN"
)

// DeviceCapabilities holds the authenticated operation capability matrix for a device
type DeviceCapabilities struct {
	DeviceID         string            `json:"device_id"`
	DeviceManagement CapabilityStatus  `json:"device_management"`
	Media            CapabilityStatus  `json:"media"`
	Media2           CapabilityStatus  `json:"media2"`
	PTZ              CapabilityStatus  `json:"ptz"`
	Imaging          CapabilityStatus  `json:"imaging"`
	Events           CapabilityStatus  `json:"events"`
	Recording        CapabilityStatus  `json:"recording"`
	AnalogFormat     CapabilityStatus  `json:"analog_format"`
	Profiles         []string          `json:"profiles,omitempty"`
	Details          map[string]string `json:"details,omitempty"`
}

// AuthStatus records the live authentication verification result
type AuthStatus struct {
	DeviceID      string    `json:"device_id"`
	Authenticated bool      `json:"authenticated"`
	Username      string    `json:"username"`
	Protocol      string    `json:"protocol"`
	AuthMethod    string    `json:"auth_method"` // WS-Security, HTTP Digest, Basic
	ClockSkewSec  int64     `json:"clock_skew_sec"`
	Firmware      string    `json:"firmware"`
	Model         string    `json:"model"`
	LastChecked   time.Time `json:"last_checked"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// AuditLogEntry ensures immutable logging with zero secret leakage
type AuditLogEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	User      string    `json:"user"`
	DeviceID  string    `json:"device_id"`
	DeviceIP  string    `json:"device_ip"`
	Action    string    `json:"action"`
	Category  string    `json:"category"`
	Status    string    `json:"status"` // SUCCESS, FAILED, PREVIEW
	Details   string    `json:"details"`
	Diff      string    `json:"diff,omitempty"`
	PrevHash  string    `json:"prev_hash,omitempty"`
	EntryHash string    `json:"entry_hash,omitempty"`
	Signature string    `json:"signature,omitempty"`
	TenantID  string    `json:"tenant_id,omitempty"`
	SiteID    string    `json:"site_id,omitempty"`
}
