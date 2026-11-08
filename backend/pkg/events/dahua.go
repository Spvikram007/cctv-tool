package events

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDahuaCGIEventChunk parses a single key-value telemetry line from /cgi-bin/eventManager.cgi?action=attach
func ParseDahuaCGIEventChunk(line string, deviceID, deviceIP string) (*SurveillanceEvent, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "Code=") && !strings.Contains(line, "Code=") {
		return nil, fmt.Errorf("dahua: line is not an event code notification")
	}

	parts := strings.Split(line, ";")
	kvs := make(map[string]string)
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			kvs[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	code := kvs["Code"]
	action := kvs["action"] // Start, Stop, Pulse
	indexStr := kvs["index"]
	dataJSON := kvs["data"]

	chNum := 1
	if idx, err := strconv.Atoi(indexStr); err == nil {
		chNum = idx + 1 // Dahua 0-indexed to 1-based channel display
	}

	isActive := true
	if strings.EqualFold(action, "Stop") {
		isActive = false
	}

	category, eventType := ClassifyDahuaCode(code)

	props := make(map[string]string)
	props["channel_number"] = strconv.Itoa(chNum)
	props["action"] = action
	if dataJSON != "" {
		props["data"] = dataJSON
	}

	ev := &SurveillanceEvent{
		ID:              fmt.Sprintf("ev-dh-%s-%d", deviceID, time.Now().UnixNano()),
		DeviceID:        deviceID,
		DeviceIP:        deviceIP,
		Manufacturer:    "Dahua",
		Category:        category,
		EventType:       eventType,
		Topic:           code,
		Source:          fmt.Sprintf("Channel %d", chNum),
		Timestamp:       time.Now().UTC(),
		ReceivedAt:      time.Now().UTC(),
		IsActive:        isActive,
		Properties:      props,
		IsSimulator:     strings.Contains(deviceIP, "127.0.0.1"),
		RedactedPayload: sanitizeXMLPayload(line),
	}

	return ev, nil
}

// ClassifyDahuaCode maps Dahua eventManager codes to universal categories
func ClassifyDahuaCode(code string) (EventCategory, string) {
	cLower := strings.ToLower(code)

	switch {
	case strings.Contains(cLower, "videomotion"):
		return CategoryMotion, "MotionDetection"
	case strings.Contains(cLower, "videoblind"):
		return CategoryTampering, "VideoTampering"
	case strings.Contains(cLower, "videoloss"):
		return CategoryVideoLoss, "VideoLoss"
	case strings.Contains(cLower, "crosslinedetection"):
		return CategoryLineCrossing, "TripwireLineCrossing"
	case strings.Contains(cLower, "crossregiondetection"):
		return CategoryIntrusion, "IntrusionZoneAlert"
	case strings.Contains(cLower, "humandetection"):
		return CategoryHumanDetection, "HumanSMDAlert"
	case strings.Contains(cLower, "vehicledetection"):
		return CategoryVehicleDetection, "VehicleSMDAlert"
	case strings.Contains(cLower, "alarmlocal"):
		return CategoryAlarmInput, "HardwareAlarmInput"
	case strings.Contains(cLower, "storagefailure"):
		return CategoryStorageAlarm, "StorageFailureAlarm"
	case strings.Contains(cLower, "disconnection"):
		return CategoryDeviceStatus, "CameraDisconnection"
	default:
		return CategoryUnknown, code
	}
}
