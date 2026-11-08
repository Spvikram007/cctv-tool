package events

import (
	"testing"
	"time"

	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

func TestONVIFPullMessagesParsing(t *testing.T) {
	xmlSample := `<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"
               xmlns:tt="http://www.onvif.org/ver10/schema">
    <soap:Body>
        <tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
            <wsnt:NotificationMessage>
                <wsnt:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">tns1:RuleEngine/CellMotionDetector/Motion</wsnt:Topic>
                <wsnt:Message UtcTime="2026-11-08T02:00:00Z">
                    <tt:Message UtcTime="2026-11-08T02:00:00Z">
                        <tt:Source>
                            <tt:SimpleItem Name="VideoSourceConfigurationToken" Value="VideoSourceToken_1"/>
                        </tt:Source>
                        <tt:Data>
                            <tt:SimpleItem Name="State" Value="true"/>
                        </tt:Data>
                    </tt:Message>
                </wsnt:Message>
            </wsnt:NotificationMessage>
        </tev:PullMessagesResponse>
    </soap:Body>
</soap:Envelope>`

	events, err := ParseONVIFPullMessagesResponse(xmlSample, "cam-01", "192.168.31.104")
	if err != nil {
		t.Fatalf("unexpected error parsing ONVIF message: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	ev := events[0]
	if ev.Category != CategoryMotion {
		t.Errorf("expected CategoryMotion, got %s", ev.Category)
	}
	if !ev.IsActive {
		t.Errorf("expected IsActive=true")
	}
	if ev.DeviceID != "cam-01" {
		t.Errorf("expected DeviceID=cam-01, got %s", ev.DeviceID)
	}
}

func TestHikvisionISAPIParsing(t *testing.T) {
	xmlSample := `<EventNotificationAlert version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <ipAddress>192.168.31.103</ipAddress>
    <portNo>80</portNo>
    <protocol>HTTP</protocol>
    <channelID>1</channelID>
    <dateTime>2026-11-08T02:05:00+00:00</dateTime>
    <activePostCount>1</activePostCount>
    <eventType>VMD</eventType>
    <eventState>active</eventState>
    <eventDescription>Motion detection started</eventDescription>
</EventNotificationAlert>`

	ev, err := ParseHikvisionISAPIEvent(xmlSample, "hik-103", "192.168.31.103")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Category != CategoryMotion {
		t.Errorf("expected CategoryMotion, got %s", ev.Category)
	}
	if ev.EventType != "MotionDetection" {
		t.Errorf("expected EventType MotionDetection, got %s", ev.EventType)
	}
	if !ev.IsActive {
		t.Errorf("expected IsActive=true")
	}
}

func TestDahuaCGIEventChunkParsing(t *testing.T) {
	chunk := "Code=CrossLineDetection;action=Start;index=0;data={\"Direction\":1,\"Class\":\"Human\"}"
	ev, err := ParseDahuaCGIEventChunk(chunk, "dahua-110", "192.168.31.110")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ev.Category != CategoryLineCrossing {
		t.Errorf("expected CategoryLineCrossing, got %s", ev.Category)
	}
	if ev.Source != "Channel 1" {
		t.Errorf("expected Source 'Channel 1', got %s", ev.Source)
	}
	if !ev.IsActive {
		t.Errorf("expected IsActive=true")
	}
}

func TestCPPlusModelAwareAdapter(t *testing.T) {
	adapter := NewCPPlusEventAdapter()

	// Test with NVR chunk
	chunk := "Code=StorageFailure;action=Start;index=0;data={\"Disk\":1}"
	ev, err := adapter.NormalizeCPPlusEvent(chunk, "CP-UNR-4K4162-V2", "cp-nvr-143", "192.168.31.143")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Category != CategoryStorageAlarm {
		t.Errorf("expected CategoryStorageAlarm, got %s", ev.Category)
	}
	if ev.Manufacturer != "CP PLUS" {
		t.Errorf("expected Manufacturer CP PLUS, got %s", ev.Manufacturer)
	}
}

func TestEventManagerDeduplicationAndSubscriptions(t *testing.T) {
	store := database.NewStore("test_events_store.json")
	store.UpsertDevice(models.Device{
		ID:           "test-cam",
		IP:           "192.168.31.101",
		Port:         80,
		Manufacturer: models.VendorDahua,
		Model:        "DH-P3B-PV",
	})
	logger := audit.NewLogger()
	em := NewEventManager(store, logger)
	defer em.Close()

	// 1. Create Subscription
	sub, err := em.CreateSubscription("test-cam", ProtoDahuaCGIEventManager, "Tester")
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	if sub.Status != StatusActive {
		t.Errorf("expected StatusActive, got %s", sub.Status)
	}

	// 2. Ingest Event
	ev := SurveillanceEvent{
		ID:         "ev-1",
		DeviceID:   "test-cam",
		DeviceIP:   "192.168.31.101",
		Category:   CategoryMotion,
		Topic:      "VideoMotion",
		IsActive:   true,
		Timestamp:  time.Now().UTC(),
		ReceivedAt: time.Now().UTC(),
	}

	ingested := em.IngestEvent(ev)
	if !ingested {
		t.Errorf("expected first event to be ingested")
	}

	// 3. Ingest duplicate event within 2s -> must be suppressed!
	dupIngested := em.IngestEvent(ev)
	if dupIngested {
		t.Errorf("expected duplicate event to be suppressed by sliding window")
	}

	// 4. Cancel Subscription
	if err := em.CancelSubscription(sub.ID, "Tester"); err != nil {
		t.Fatalf("failed to cancel subscription: %v", err)
	}
	if sub.Status != StatusCancelled {
		t.Errorf("expected StatusCancelled, got %s", sub.Status)
	}
}
