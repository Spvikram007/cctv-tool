package events

import (
	"time"
)

type SubscriptionProtocol string

const (
	ProtoONVIFPullPoint       SubscriptionProtocol = "ONVIF_PULLPOINT"
	ProtoHikvisionISAPIStream SubscriptionProtocol = "HIKVISION_ISAPI_STREAM"
	ProtoDahuaCGIEventManager SubscriptionProtocol = "DAHUA_CGI_EVENTMANAGER"
	ProtoCPPlusModelAware     SubscriptionProtocol = "CPPLUS_MODEL_AWARE"
)

type SubscriptionStatus string

const (
	StatusActive     SubscriptionStatus = "ACTIVE"
	StatusRenewing   SubscriptionStatus = "RENEWING"
	StatusExpired    SubscriptionStatus = "EXPIRED"
	StatusFailed     SubscriptionStatus = "FAILED"
	StatusCancelled  SubscriptionStatus = "CANCELLED"
	StatusReconnecting SubscriptionStatus = "RECONNECTING"
)

type EventCategory string

const (
	CategoryMotion          EventCategory = "MOTION"
	CategoryTampering       EventCategory = "TAMPERING"
	CategoryLineCrossing    EventCategory = "LINE_CROSSING"
	CategoryIntrusion       EventCategory = "INTRUSION"
	CategoryRegionExit      EventCategory = "REGION_EXIT"
	CategoryFaceDetection   EventCategory = "FACE_DETECTION"
	CategoryHumanDetection  EventCategory = "HUMAN_DETECTION"
	CategoryVehicleDetection EventCategory = "VEHICLE_DETECTION"
	CategoryAlarmInput      EventCategory = "ALARM_INPUT"
	CategoryVideoLoss       EventCategory = "VIDEO_LOSS"
	CategoryStorageAlarm    EventCategory = "STORAGE_ALARM"
	CategoryDeviceStatus    EventCategory = "DEVICE_STATUS"
	CategoryUnknown         EventCategory = "UNKNOWN"
)

// EventSubscription represents an authenticated event consumer connection
type EventSubscription struct {
	ID                 string               `json:"id"`
	DeviceID           string               `json:"device_id"`
	DeviceIP           string               `json:"device_ip"`
	Protocol           SubscriptionProtocol `json:"protocol"`
	SubscriptionURL    string               `json:"subscription_url"`
	Status             SubscriptionStatus   `json:"status"`
	CreatedAt          time.Time            `json:"created_at"`
	ExpiresAt          time.Time            `json:"expires_at"`
	RenewalIntervalSec int                  `json:"renewal_interval_sec"`
	LastPollTime       time.Time            `json:"last_poll_time"`
	EventsReceived     int64                `json:"events_received"`
	LastError          string               `json:"last_error,omitempty"`
}

// SurveillanceEvent represents a normalized real-time telemetry event
type SurveillanceEvent struct {
	ID              string            `json:"id"`
	DeviceID        string            `json:"device_id"`
	DeviceIP        string            `json:"device_ip"`
	Manufacturer    string            `json:"manufacturer"`
	Model           string            `json:"model"`
	Category        EventCategory     `json:"category"`
	EventType       string            `json:"event_type"` // e.g., "MotionAlarm", "LineCrossing", "VideoBlind"
	Topic           string            `json:"topic"`      // Raw vendor topic / code
	Source          string            `json:"source"`     // e.g., "Channel 1", "VideoSourceToken"
	Timestamp       time.Time         `json:"timestamp"`  // Camera hardware timestamp
	ReceivedAt      time.Time         `json:"received_at"`// Ingestion timestamp
	IsActive        bool              `json:"is_active"`  // true = event active/start, false = clear/stop
	Properties      map[string]string `json:"properties"`
	Data            map[string]string `json:"data,omitempty"`
	IsSimulator     bool              `json:"is_simulator"`
	RedactedPayload string            `json:"redacted_payload,omitempty"`
}
