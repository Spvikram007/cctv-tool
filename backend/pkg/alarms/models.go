package alarms

import (
	"time"
)

type AlarmSeverity string

const (
	SeverityCritical AlarmSeverity = "CRITICAL"
	SeverityMajor    AlarmSeverity = "MAJOR"
	SeverityMinor    AlarmSeverity = "MINOR"
	SeverityInfo     AlarmSeverity = "INFO"
)

type AlarmState string

const (
	StateNew           AlarmState = "NEW"
	StateAcknowledged  AlarmState = "ACKNOWLEDGED"
	StateInvestigating AlarmState = "INVESTIGATING"
	StateResolved      AlarmState = "RESOLVED"
	StateDismissed     AlarmState = "DISMISSED"
)

// Alarm represents a normalized, actionable security alert
type Alarm struct {
	ID                  string        `json:"id"`
	TenantID            string        `json:"tenant_id"`
	SiteID              string        `json:"site_id"`
	DeviceID            string        `json:"device_id"`
	RecorderID          string        `json:"recorder_id,omitempty"`
	ChannelID           int           `json:"channel_id,omitempty"`
	Manufacturer        string        `json:"manufacturer"`
	Model               string        `json:"model"`
	EventCategory       string        `json:"event_category"`
	EventType           string        `json:"event_type"`
	Severity            AlarmSeverity `json:"severity"`
	EventTimestamp      time.Time     `json:"event_timestamp"`
	ReceivedTimestamp   time.Time     `json:"received_timestamp"`
	EventSource         string        `json:"event_source"`
	Description         string        `json:"description"`
	SnapshotRef         string        `json:"snapshot_ref,omitempty"`
	SnapshotCaptureTime *time.Time    `json:"snapshot_capture_time,omitempty"`
	State               AlarmState    `json:"state"`
	AckStatus           string        `json:"ack_status"` // UNACKNOWLEDGED, ACKNOWLEDGED
	AckBy               string        `json:"ack_by,omitempty"`
	AckAt               *time.Time    `json:"ack_at,omitempty"`
	AssignedTo          string        `json:"assigned_to,omitempty"`
	AssignedAt          *time.Time    `json:"assigned_at,omitempty"`
	ResolutionNotes     string        `json:"resolution_notes,omitempty"`
	ResolvedBy          string        `json:"resolved_by,omitempty"`
	ResolvedAt          *time.Time    `json:"resolved_at,omitempty"`
	RedactedPayload     string        `json:"redacted_payload,omitempty"`
	IsSimulator         bool          `json:"is_simulator"`
}

// AlarmRule models configurable trigger-to-action automation
type AlarmRule struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Enabled           bool          `json:"enabled"`
	EventCategories   []string      `json:"event_categories"` // e.g. ["TAMPERING", "LINE_CROSSING", "INTRUSION"]
	Severity          AlarmSeverity `json:"severity"`
	DeviceIDs         []string      `json:"device_ids"` // Empty = all devices
	CaptureSnapshot   bool          `json:"capture_snapshot"`
	AssignTechnician  string        `json:"assign_technician,omitempty"`
	WebhookURL        string        `json:"webhook_url,omitempty"`
	EmailNotification string        `json:"email_notification,omitempty"`
	CooldownSec       int           `json:"cooldown_sec"`
}

// AlarmFilter models query criteria for alarm searches
type AlarmFilter struct {
	DeviceID      string        `json:"device_id,omitempty"`
	SiteID        string        `json:"site_id,omitempty"`
	Manufacturer  string        `json:"manufacturer,omitempty"`
	EventCategory string        `json:"event_category,omitempty"`
	Severity      AlarmSeverity `json:"severity,omitempty"`
	State         AlarmState    `json:"state,omitempty"`
	StartTime     *time.Time    `json:"start_time,omitempty"`
	EndTime       *time.Time    `json:"end_time,omitempty"`
	Limit         int           `json:"limit,omitempty"`
}

// AlarmStats provides real-time counts for technician dashboards
type AlarmStats struct {
	TotalAlarms       int `json:"total_alarms"`
	NewAlarms         int `json:"new_alarms"`
	AcknowledgedAlarms int `json:"acknowledged_alarms"`
	InvestigatingAlarms int `json:"investigating_alarms"`
	ResolvedAlarms    int `json:"resolved_alarms"`
	CriticalCount     int `json:"critical_count"`
	MajorCount        int `json:"major_count"`
	MinorCount        int `json:"minor_count"`
	InfoCount         int `json:"info_count"`
}
