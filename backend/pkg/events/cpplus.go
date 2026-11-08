package events

import (
	"fmt"
	"strings"
	"time"
)

// CPPlusEventAdapter provides model-aware event parsing and API capability detection
type CPPlusEventAdapter struct{}

func NewCPPlusEventAdapter() *CPPlusEventAdapter {
	return &CPPlusEventAdapter{}
}

// NormalizeCPPlusEvent handles incoming telemetry based on exact CP PLUS hardware model
func (a *CPPlusEventAdapter) NormalizeCPPlusEvent(rawPayload string, model string, deviceID, deviceIP string) (*SurveillanceEvent, error) {
	modelUpper := strings.ToUpper(model)

	// 1. CP PLUS 4K NVR series (e.g. CP-UNR-4K4162-V2 on our physical benchmark)
	if strings.Contains(modelUpper, "CP-UNR") {
		// NVR multi-channel storage and channel loss events
		if strings.Contains(rawPayload, "Code=") {
			ev, err := ParseDahuaCGIEventChunk(rawPayload, deviceID, deviceIP)
			if err == nil {
				ev.Manufacturer = "CP PLUS"
				ev.Model = model
				return ev, nil
			}
		}
		if strings.Contains(rawPayload, "<") && strings.Contains(rawPayload, ">") {
			evs, err := ParseONVIFPullMessagesResponse(rawPayload, deviceID, deviceIP)
			if err == nil && len(evs) > 0 {
				ev := evs[0]
				ev.Manufacturer = "CP PLUS"
				ev.Model = model
				return &ev, nil
			}
		}
	}

	// 2. CP PLUS Indigo / Cosmic IP Camera series (e.g. CP-UNC-TA21PL3-V3)
	if strings.Contains(modelUpper, "CP-UNC") || strings.Contains(modelUpper, "INDIGO") || strings.Contains(modelUpper, "COSMIC") {
		if strings.Contains(rawPayload, "Code=") {
			ev, err := ParseDahuaCGIEventChunk(rawPayload, deviceID, deviceIP)
			if err == nil {
				ev.Manufacturer = "CP PLUS"
				ev.Model = model
				return ev, nil
			}
		}
	}

	// 3. Fallback to ONVIF standards parsing if SOAP payload detected
	if strings.Contains(rawPayload, "<wsnt:NotificationMessage") || strings.Contains(rawPayload, "<NotificationMessage") {
		evs, err := ParseONVIFPullMessagesResponse(rawPayload, deviceID, deviceIP)
		if err == nil && len(evs) > 0 {
			ev := evs[0]
			ev.Manufacturer = "CP PLUS"
			ev.Model = model
			return &ev, nil
		}
	}

	// 4. Record unsupported / unknown CP PLUS event without discarding or panicking
	ev := &SurveillanceEvent{
		ID:              fmt.Sprintf("ev-cpp-unknown-%s-%d", deviceID, time.Now().UnixNano()),
		DeviceID:        deviceID,
		DeviceIP:        deviceIP,
		Manufacturer:    "CP PLUS",
		Model:           model,
		Category:        CategoryUnknown,
		EventType:       "CPPlusUnrecognizedTelemetry",
		Topic:           "RawTelemetry",
		Source:          "Device",
		Timestamp:       time.Now().UTC(),
		ReceivedAt:      time.Now().UTC(),
		IsActive:        true,
		Properties:      map[string]string{"raw_preview": rawPayload[:min(len(rawPayload), 256)]},
		IsSimulator:     strings.Contains(deviceIP, "127.0.0.1"),
		RedactedPayload: sanitizeXMLPayload(rawPayload),
	}

	return ev, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
