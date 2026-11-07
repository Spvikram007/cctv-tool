package recorder

import (
	"time"
)

type RecorderType string

const (
	TypeDVR            RecorderType = "DVR"
	TypeNVR            RecorderType = "NVR"
	TypeXVR            RecorderType = "XVR"
	TypeHVR            RecorderType = "HVR"
	TypeHybridRecorder RecorderType = "HYBRID_RECORDER"
)

type ChannelType string

const (
	ChannelTypeAnalog ChannelType = "ANALOG"
	ChannelTypeIP     ChannelType = "IP"
)

type AnalogFormat string

const (
	FormatCVI  AnalogFormat = "CVI"
	FormatTVI  AnalogFormat = "TVI"
	FormatAHD  AnalogFormat = "AHD"
	FormatCVBS AnalogFormat = "CVBS"
	FormatAuto AnalogFormat = "AUTO"
	FormatIP   AnalogFormat = "IP"
)

type ChannelStatus string

const (
	StatusOnline    ChannelStatus = "ONLINE"
	StatusOffline   ChannelStatus = "OFFLINE"
	StatusNoSignal  ChannelStatus = "NO_SIGNAL"
	StatusDisabled ChannelStatus = "DISABLED"
)

type SignalStatus string

const (
	SignalLocked       SignalStatus = "SIGNAL_LOCKED"
	SignalLost         SignalStatus = "SIGNAL_LOST"
	SignalUnconfigured SignalStatus = "UNCONFIGURED"
)

type RecordMode string

const (
	RecordModeContinuous RecordMode = "CONTINUOUS"
	RecordModeMotion     RecordMode = "MOTION"
	RecordModeAlarm      RecordMode = "ALARM"
	RecordModeManual     RecordMode = "MANUAL"
	RecordModeDisabled   RecordMode = "DISABLED"
)

// RecorderProfile represents the complete hardware and capability profile of an XVR/DVR/NVR
type RecorderProfile struct {
	DeviceID               string         `json:"device_id"`
	Name                   string         `json:"name"`
	IP                     string         `json:"ip"`
	Port                   int            `json:"port"`
	Manufacturer           string         `json:"manufacturer"`
	Model                  string         `json:"model"`
	HardwareRevision       string         `json:"hardware_revision"`
	FirmwareVersion        string         `json:"firmware_version"`
	SerialNumber           string         `json:"serial_number"`
	RecorderType           RecorderType   `json:"recorder_type"`
	PhysicalAnalogChannels int            `json:"physical_analog_channels"`
	SupportedIPChannels    int            `json:"supported_ip_channels"`
	TotalChannels          int            `json:"total_channels"`
	MaxResolution          string         `json:"max_resolution"` // e.g. "4K (3840x2160)", "5MP (2560x1920)"
	SupportedAnalogFormats []AnalogFormat `json:"supported_analog_formats"`
	SupportedCodecs        []string       `json:"supported_codecs"`
	CoaxialControlSupported bool           `json:"coaxial_control_supported"` // UTC PTZ & OSD over coax
	AudioOverCoaxSupported  bool           `json:"audio_over_coax_supported"`
	PTZOverCoaxSupported    bool           `json:"ptz_over_coax_supported"`
	AIAnalyticsSupported   bool           `json:"ai_analytics_supported"`
	DiskCount              int            `json:"disk_count"`
	TotalStorageGB         int64          `json:"total_storage_gb"`
	UsedStorageGB          int64          `json:"used_storage_gb"`
	NetworkInterfaces      []string       `json:"network_interfaces"`
}

// ChannelInfo models a physical or IP camera channel on the recorder
type ChannelInfo struct {
	RecorderID             string        `json:"recorder_id"`
	ChannelID              int           `json:"channel_id"` // Vendor hardware channel index
	ChannelNumber          int           `json:"channel_number"` // Standard 1-based display number
	Name                   string        `json:"name"`
	ChannelType            ChannelType   `json:"channel_type"` // ANALOG or IP
	ConnectedCameraID      string        `json:"connected_camera_id,omitempty"`
	ConnectedCameraIP      string        `json:"connected_camera_ip,omitempty"`
	ConnectedCameraModel   string        `json:"connected_camera_model,omitempty"`
	ConnectionStatus       ChannelStatus `json:"connection_status"`
	SignalStatus           SignalStatus  `json:"signal_status"`
	SignalFormat           AnalogFormat  `json:"signal_format"`
	Resolution             string        `json:"resolution"`
	FrameRate              int           `json:"frame_rate"`
	BitrateKbps            int           `json:"bitrate_kbps"`
	EncodingFormat         string        `json:"encoding_format"` // H.264, H.265, SmartH.265
	RecordingStatus        RecordMode    `json:"recording_status"`
	AudioEnabled           bool          `json:"audio_enabled"`
	PTZSupported           bool          `json:"ptz_supported"`
	RTSPStreamURL          string        `json:"rtsp_stream_url,omitempty"`
}

// AnalogFormatResult represents the outcome of a coaxial UTC format change
type AnalogFormatResult struct {
	ChannelID      int          `json:"channel_id"`
	PreviousFormat AnalogFormat `json:"previous_format"`
	CurrentFormat  AnalogFormat `json:"current_format"`
	SwitchMethod   string       `json:"switch_method"` // COAXIAL_UTC_CGI, COAXIAL_UTC_ISAPI, MANUAL
	Success        bool         `json:"success"`
	Message        string       `json:"message"`
	Timestamp      time.Time    `json:"timestamp"`
}

// IPRegistrationReq represents the payload for mapping an IP camera to an NVR/XVR channel
type IPRegistrationReq struct {
	ChannelID   int    `json:"channel_id"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"` // ONVIF, DAHUA_PRIVATE, HIKVISION_ISAPI, RTSP
	Username    string `json:"username"`
	Password    string `json:"password"`
	StreamIndex int    `json:"stream_index"`
}

// RecordingSchedule models 24x7 time-block recording configurations per channel
type RecordingSchedule struct {
	RecorderID     string            `json:"recorder_id"`
	ChannelID      int               `json:"channel_id"`
	ChannelName    string            `json:"channel_name"`
	PreRecordSec   int               `json:"pre_record_sec"`  // 0 - 30 seconds
	PostRecordSec  int               `json:"post_record_sec"` // 10 - 300 seconds
	WeeklySchedule map[int][24]string `json:"weekly_schedule"` // Day (0=Sun, 1=Mon, ..., 6=Sat) -> 24 hours of RecordMode
	HolidayEnabled bool              `json:"holiday_enabled"`
}

// RecordingSegment represents an indexed video clip on the recorder
type RecordingSegment struct {
	ID          string    `json:"id"`
	RecorderID  string    `json:"recorder_id"`
	ChannelID   int       `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	DurationSec int       `json:"duration_sec"`
	EventType   string    `json:"event_type"` // CONTINUOUS, MOTION, ALARM, MANUAL
	SizeBytes   int64     `json:"size_bytes"`
	FileFormat  string    `json:"file_format"` // MP4, DAV, TS
	PlaybackURI string    `json:"playback_uri"`
}

// PlaybackSession represents an active, authenticated RTSP/HTTP playback stream
type PlaybackSession struct {
	SessionID       string    `json:"session_id"`
	PlaybackToken   string    `json:"playback_token"`
	RecorderID      string    `json:"recorder_id"`
	ChannelID       int       `json:"channel_id"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	CurrentPosition time.Time `json:"current_position"`
	Speed           float64   `json:"speed"` // 0.5, 1.0, 2.0, 4.0, 8.0, 16.0
	State           string    `json:"state"` // PLAYING, PAUSED, STOPPED
	StreamURL       string    `json:"stream_url"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// StorageDisk models an internal hard drive inside a DVR/NVR
type StorageDisk struct {
	DiskIndex    int     `json:"disk_index"`
	Name         string  `json:"name"`
	Model        string  `json:"model"`
	Serial       string  `json:"serial"`
	CapacityGB   int64   `json:"capacity_gb"`
	UsedGB       int64   `json:"used_gb"`
	FreeGB       int64   `json:"free_gb"`
	UsagePercent float64 `json:"usage_percent"`
	Status       string  `json:"status"`       // NORMAL, ERROR, UNFORMATTED, REBUILDING
	SMARTStatus  string  `json:"smart_status"` // HEALTHY, WARNING, FAILED
	TemperatureC int     `json:"temperature_c"`
	BadSectors   int     `json:"bad_sectors"`
	OverwriteMode string  `json:"overwrite_mode"` // OVERWRITE, STOP_RECORDING
}

// StorageHealthReport aggregates physical disk health and capacity analytics
type StorageHealthReport struct {
	RecorderID                  string            `json:"recorder_id"`
	TotalDisks                  int               `json:"total_disks"`
	TotalCapacityGB             int64             `json:"total_capacity_gb"`
	TotalUsedGB                 int64             `json:"total_used_gb"`
	TotalFreeGB                 int64             `json:"total_free_gb"`
	Disks                       []StorageDisk     `json:"disks"`
	OverallHealth               string            `json:"overall_health"` // GOOD, WARNING, CRITICAL
	RecordingInterruptionWarning bool              `json:"recording_interruption_warning"`
	EstimatedRetentionDays      float64           `json:"estimated_retention_days"`
	MeasuredRetentionBasis      string            `json:"measured_retention_basis"`
	RetentionEstimate           RetentionEstimate `json:"retention_estimate"`
}

// RetentionEstimate models storage calculation formulas based on bitrates
type RetentionEstimate struct {
	ChannelCount        int     `json:"channel_count"`
	AvgBitrateKbps      int     `json:"avg_bitrate_kbps"`
	DailyRecordingHours float64 `json:"daily_recording_hours"`
	DailyStorageGB      float64 `json:"daily_storage_gb"`
	TotalRetentionDays  float64 `json:"total_retention_days"`
}

// DiskFormatPreview models the high-impact disk format authorization workflow
type DiskFormatPreview struct {
	FormatToken string    `json:"format_token"`
	RecorderID  string    `json:"recorder_id"`
	DiskIndex   int       `json:"disk_index"`
	Model       string    `json:"model"`
	CapacityGB  int64     `json:"capacity_gb"`
	Warning     string    `json:"warning"`
	ExpiresAt   time.Time `json:"expires_at"`
}
