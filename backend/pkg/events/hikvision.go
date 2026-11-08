package events

import (
	"fmt"
	"strings"
	"time"
)

// ParseHikvisionISAPIEvent parses an XML alert block from Hikvision's /ISAPI/Event/notification/alertStream
func ParseHikvisionISAPIEvent(xmlStr string, deviceID, deviceIP string) (*SurveillanceEvent, error) {
	if !strings.Contains(xmlStr, "EventNotificationAlert") {
		return nil, fmt.Errorf("hikvision: payload is not an EventNotificationAlert")
	}

	eventTypeRaw := extractTagContent(xmlStr, "eventType")
	channelID := extractTagContent(xmlStr, "channelID")
	if channelID == "" {
		channelID = "1"
	}
	desc := extractTagContent(xmlStr, "eventDescription")
	dateTimeStr := extractTagContent(xmlStr, "dateTime")

	eventTime := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, dateTimeStr); err == nil {
		eventTime = t
	}

	category, normalizedType := ClassifyHikvisionEvent(eventTypeRaw)

	// State check
	isActive := true
	if strings.Contains(xmlStr, "<eventState>inactive</eventState>") || strings.Contains(xmlStr, "<activePostCount>0</activePostCount>") {
		isActive = false
	}

	props := make(map[string]string)
	props["channel_id"] = channelID
	if desc != "" {
		props["description"] = desc
	}
	if target := extractTagContent(xmlStr, "detectionTarget"); target != "" {
		props["detection_target"] = target
	}

	ev := &SurveillanceEvent{
		ID:              fmt.Sprintf("ev-hik-%s-%d", deviceID, time.Now().UnixNano()),
		DeviceID:        deviceID,
		DeviceIP:        deviceIP,
		Manufacturer:    "Hikvision",
		Category:        category,
		EventType:       normalizedType,
		Topic:           eventTypeRaw,
		Source:          fmt.Sprintf("Channel %s", channelID),
		Timestamp:       eventTime,
		ReceivedAt:      time.Now().UTC(),
		IsActive:        isActive,
		Properties:      props,
		IsSimulator:     strings.Contains(deviceIP, "127.0.0.1"),
		RedactedPayload: sanitizeXMLPayload(xmlStr),
	}

	return ev, nil
}

// ClassifyHikvisionEvent maps Hikvision eventType identifiers to universal categories
func ClassifyHikvisionEvent(rawType string) (EventCategory, string) {
	tLower := strings.ToLower(rawType)

	switch {
	case strings.Contains(tLower, "vmd") || strings.Contains(tLower, "motion"):
		return CategoryMotion, "MotionDetection"
	case strings.Contains(tLower, "tamper") || strings.Contains(tLower, "shelteralarm"):
		return CategoryTampering, "VideoTampering"
	case strings.Contains(tLower, "line") || strings.Contains(tLower, "linedetection"):
		return CategoryLineCrossing, "LineCrossing"
	case strings.Contains(tLower, "field") || strings.Contains(tLower, "intrusion"):
		return CategoryIntrusion, "IntrusionDetection"
	case strings.Contains(tLower, "regionentrance") || strings.Contains(tLower, "regionexit"):
		return CategoryRegionExit, "RegionEntranceExit"
	case strings.Contains(tLower, "facedetection") || strings.Contains(tLower, "face"):
		return CategoryFaceDetection, "FaceDetection"
	case strings.Contains(tLower, "io") || strings.Contains(tLower, "alarminput"):
		return CategoryAlarmInput, "AlarmInput"
	case strings.Contains(tLower, "videoloss"):
		return CategoryVideoLoss, "VideoLoss"
	case strings.Contains(tLower, "disk") || strings.Contains(tLower, "storage"):
		return CategoryStorageAlarm, "StorageAlarm"
	case strings.Contains(tLower, "devicestatus"):
		return CategoryDeviceStatus, "DeviceStatus"
	default:
		return CategoryUnknown, rawType
	}
}
