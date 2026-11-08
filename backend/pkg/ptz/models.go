package ptz

import (
	"errors"
	"time"
)

var (
	ErrPTZNotSupported = errors.New("ptz: device does not support pan/tilt/zoom operations")
	ErrInvalidPreset   = errors.New("ptz: invalid preset index or name")
	ErrPresetNotFound  = errors.New("ptz: preset not found")
	ErrPatrolNotFound  = errors.New("ptz: patrol tour not found")
	ErrUnauthorized    = errors.New("ptz: operator unauthorized for robotic camera control")
)

// MovementType defines the coordinate mode for PTZ operations
type MovementType string

const (
	MovementContinuous MovementType = "CONTINUOUS"
	MovementRelative   MovementType = "RELATIVE"
	MovementAbsolute   MovementType = "ABSOLUTE"
)

// PTZCapabilities outlines verified robotic hardware features for a camera
type PTZCapabilities struct {
	DeviceID               string       `json:"device_id"`
	PTZSupported           bool         `json:"ptz_supported"`
	PanSupported           bool         `json:"pan_supported"`
	TiltSupported          bool         `json:"tilt_supported"`
	ZoomSupported          bool         `json:"zoom_supported"`
	FocusSupported         bool         `json:"focus_supported"`
	IrisSupported          bool         `json:"iris_supported"`
	ContinuousMoveSupported bool         `json:"continuous_move_supported"`
	RelativeMoveSupported   bool         `json:"relative_move_supported"`
	AbsoluteMoveSupported   bool         `json:"absolute_move_supported"`
	PresetsSupported       bool         `json:"presets_supported"`
	MaxPresets             int          `json:"max_presets"`
	PatrolSupported        bool         `json:"patrol_supported"`
	MaxPatrols            int          `json:"max_patrols"`
	HomePositionSupported  bool         `json:"home_position_supported"`
	AutoScanSupported      bool         `json:"auto_scan_supported"`
	MinSpeed               float64      `json:"min_speed"`
	MaxSpeed               float64      `json:"max_speed"`
	Protocol               string       `json:"protocol"` // ONVIF, DAHUA_CGI, HIKVISION_ISAPI, COAXIAL_UTC
}

// Preset represents a memorized robotic pan/tilt/zoom position
type Preset struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token,omitempty"` // ONVIF preset token
	Pan       float64   `json:"pan,omitempty"`
	Tilt      float64   `json:"tilt,omitempty"`
	Zoom      float64   `json:"zoom,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PatrolStep defines a waypoint in an automated patrol tour
type PatrolStep struct {
	StepNumber  int     `json:"step_number"`
	PresetID    int     `json:"preset_id"`
	PresetName  string  `json:"preset_name"`
	DwellTimeSec int    `json:"dwell_time_sec"` // Time to hold position in seconds
	Speed       float64 `json:"speed"`          // Movement speed to waypoint (0.1 - 1.0)
}

// PatrolTour defines a sequence of preset waypoints for automated robotic scanning
type PatrolTour struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	Running     bool         `json:"running"`
	Steps       []PatrolStep `json:"steps"`
	RepeatCount int          `json:"repeat_count"` // 0 = continuous loop
}

// MoveRequest encapsulates a robotic navigation request
type MoveRequest struct {
	MovementType MovementType `json:"movement_type"` // CONTINUOUS, RELATIVE, ABSOLUTE
	Pan          float64      `json:"pan"`           // -1.0 (Left) to +1.0 (Right)
	Tilt         float64      `json:"tilt"`          // -1.0 (Down) to +1.0 (Up)
	Zoom         float64      `json:"zoom"`          // -1.0 (Wide) to +1.0 (Tele)
	Speed        float64      `json:"speed"`         // 0.1 to 1.0 (Normal: 0.5)
	TimeoutSec   int          `json:"timeout_sec"`   // Optional automatic stop timeout
}

// FocusIrisRequest encapsulates optical lens parameter adjustments
type FocusIrisRequest struct {
	Command string  `json:"command"` // FOCUS_NEAR, FOCUS_FAR, FOCUS_STOP, IRIS_OPEN, IRIS_CLOSE, IRIS_RESET, AUTO_FOCUS
	Speed   float64 `json:"speed"`
}
