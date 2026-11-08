package events

import (
	"fmt"
	"strings"
	"time"
)

// BuildCreatePullPointRequest builds a standards-compliant ONVIF PullPoint creation SOAP envelope
func BuildCreatePullPointRequest(initialTerminationTime string) string {
	if initialTerminationTime == "" {
		initialTerminationTime = "PT60S" // 60-second default subscription
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://www.w3.org/2005/08/addressing"
               xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
    <soap:Header>
        <wsa:Action>http://www.onvif.org/ver10/events/wsdl/EventPortType/CreatePullPointSubscriptionRequest</wsa:Action>
    </soap:Header>
    <soap:Body>
        <tev:CreatePullPointSubscription>
            <tev:InitialTerminationTime>%s</tev:InitialTerminationTime>
        </tev:CreatePullPointSubscription>
    </soap:Body>
</soap:Envelope>`, initialTerminationTime)
}

// BuildPullMessagesRequest builds a bounded PullMessages poll request
func BuildPullMessagesRequest(timeout string, messageLimit int) string {
	if timeout == "" {
		timeout = "PT5S" // 5-second polling timeout
	}
	if messageLimit <= 0 || messageLimit > 100 {
		messageLimit = 10
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://www.w3.org/2005/08/addressing"
               xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
    <soap:Header>
        <wsa:Action>http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/PullMessagesRequest</wsa:Action>
    </soap:Header>
    <soap:Body>
        <tev:PullMessages>
            <tev:Timeout>%s</tev:Timeout>
            <tev:MessageLimit>%d</tev:MessageLimit>
        </tev:PullMessages>
    </soap:Body>
</soap:Envelope>`, timeout, messageLimit)
}

// BuildRenewSubscriptionRequest builds a subscription renewal envelope
func BuildRenewSubscriptionRequest(terminationTime string) string {
	if terminationTime == "" {
		terminationTime = "PT60S"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://www.w3.org/2005/08/addressing"
               xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
    <soap:Header>
        <wsa:Action>http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/RenewRequest</wsa:Action>
    </soap:Header>
    <soap:Body>
        <wsnt:Renew>
            <wsnt:TerminationTime>%s</wsnt:TerminationTime>
        </wsnt:Renew>
    </soap:Body>
</soap:Envelope>`, terminationTime)
}

// BuildUnsubscribeRequest builds a subscription cancellation envelope
func BuildUnsubscribeRequest() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://www.w3.org/2005/08/addressing"
               xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
    <soap:Header>
        <wsa:Action>http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest</wsa:Action>
    </soap:Header>
    <soap:Body>
        <wsnt:Unsubscribe/>
    </soap:Body>
</soap:Envelope>`
}

// ParseONVIFPullMessagesResponse extracts parsed events from a SOAP PullMessagesResponse
func ParseONVIFPullMessagesResponse(xmlStr string, deviceID, deviceIP string) ([]SurveillanceEvent, error) {
	events := make([]SurveillanceEvent, 0)
	now := time.Now().UTC()

	// Locate each <wsnt:NotificationMessage> block
	chunks := strings.Split(xmlStr, "<wsnt:NotificationMessage")
	if len(chunks) <= 1 {
		chunks = strings.Split(xmlStr, "<NotificationMessage")
	}

	for idx, chunk := range chunks {
		if idx == 0 {
			continue // preamble before first message
		}

		topic := extractTagContent(chunk, "wsnt:Topic")
		if topic == "" {
			topic = extractTagContent(chunk, "Topic")
		}

		utcTimeStr := extractAttribute(chunk, "UtcTime")
		eventTime := now
		if t, err := time.Parse(time.RFC3339, utcTimeStr); err == nil {
			eventTime = t
		}

		// Source and Properties
		properties := make(map[string]string)
		source := "Default VideoSource"
		if strings.Contains(chunk, "VideoSourceConfigurationToken") {
			source = extractAttribute(chunk, "Value")
		}

		// State evaluation (IsActive)
		isActive := true
		if strings.Contains(chunk, `Name="State"`) {
			val := extractStateValue(chunk)
			if val == "false" || val == "0" {
				isActive = false
			}
		}

		category, eventType := ClassifyONVIFTopic(topic)

		ev := SurveillanceEvent{
			ID:              fmt.Sprintf("ev-onvif-%s-%d", deviceID, time.Now().UnixNano()+int64(idx)),
			DeviceID:        deviceID,
			DeviceIP:        deviceIP,
			Category:        category,
			EventType:       eventType,
			Topic:           topic,
			Source:          source,
			Timestamp:       eventTime,
			ReceivedAt:      now,
			IsActive:        isActive,
			Properties:      properties,
			IsSimulator:     strings.Contains(deviceIP, "127.0.0.1"),
			RedactedPayload: sanitizeXMLPayload(chunk),
		}
		events = append(events, ev)
	}

	return events, nil
}

// ClassifyONVIFTopic maps ONVIF WS-Notification topic paths into normalized categories
func ClassifyONVIFTopic(topic string) (EventCategory, string) {
	tLower := strings.ToLower(topic)

	if strings.Contains(tLower, "cellmotiondetector") || strings.Contains(tLower, "motion") {
		return CategoryMotion, "MotionAlarm"
	}
	if strings.Contains(tLower, "tamper") || strings.Contains(tLower, "tampering") {
		return CategoryTampering, "VideoTampering"
	}
	if strings.Contains(tLower, "linecrossing") || strings.Contains(tLower, "crossline") {
		return CategoryLineCrossing, "LineCrossing"
	}
	if strings.Contains(tLower, "fielddetection") || strings.Contains(tLower, "intrusion") {
		return CategoryIntrusion, "IntrusionDetection"
	}
	if strings.Contains(tLower, "regionexit") || strings.Contains(tLower, "regionentrance") {
		return CategoryRegionExit, "RegionExit"
	}
	if strings.Contains(tLower, "facedetection") || strings.Contains(tLower, "face") {
		return CategoryFaceDetection, "FaceDetection"
	}
	if strings.Contains(tLower, "humandetection") || strings.Contains(tLower, "people") {
		return CategoryHumanDetection, "HumanDetection"
	}
	if strings.Contains(tLower, "vehicledetection") || strings.Contains(tLower, "vehicle") {
		return CategoryVehicleDetection, "VehicleDetection"
	}
	if strings.Contains(tLower, "alarminput") || strings.Contains(tLower, "digitalinput") {
		return CategoryAlarmInput, "AlarmInput"
	}
	if strings.Contains(tLower, "videoloss") || strings.Contains(tLower, "signalloss") {
		return CategoryVideoLoss, "VideoLoss"
	}
	if strings.Contains(tLower, "storage") || strings.Contains(tLower, "disk") {
		return CategoryStorageAlarm, "StorageAlarm"
	}
	if strings.Contains(tLower, "devicestatus") || strings.Contains(tLower, "operatingstatus") {
		return CategoryDeviceStatus, "DeviceStatus"
	}

	return CategoryUnknown, "GenericONVIFEvent"
}

// XML helper functions
func extractTagContent(xml, tag string) string {
	startTag := "<" + tag
	startIdx := strings.Index(xml, startTag)
	if startIdx == -1 {
		return ""
	}
	gtIdx := strings.Index(xml[startIdx:], ">")
	if gtIdx == -1 {
		return ""
	}
	contentStart := startIdx + gtIdx + 1

	endTag := "</" + tag + ">"
	endIdx := strings.Index(xml[contentStart:], endTag)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(xml[contentStart : contentStart+endIdx])
}

func extractAttribute(xml, attr string) string {
	pattern := attr + `="`
	idx := strings.Index(xml, pattern)
	if idx == -1 {
		return ""
	}
	start := idx + len(pattern)
	end := strings.Index(xml[start:], `"`)
	if end == -1 {
		return ""
	}
	return xml[start : start+end]
}

func extractStateValue(chunk string) string {
	stateIdx := strings.Index(chunk, `Name="State"`)
	if stateIdx == -1 {
		return "true"
	}
	valIdx := strings.Index(chunk[stateIdx:], `Value="`)
	if valIdx == -1 {
		return "true"
	}
	start := stateIdx + valIdx + len(`Value="`)
	end := strings.Index(chunk[start:], `"`)
	if end == -1 {
		return "true"
	}
	return strings.ToLower(chunk[start : start+end])
}

func sanitizeXMLPayload(payload string) string {
	redacted := payload
	// Redact any password or credential elements
	for _, secret := range []string{"Password", "Auth", "Token", "Nonce"} {
		if strings.Contains(redacted, secret) {
			redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
		}
	}
	if len(redacted) > 1024 {
		redacted = redacted[:1024] + "... [truncated]"
	}
	return strings.TrimSpace(redacted)
}
