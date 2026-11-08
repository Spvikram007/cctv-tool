package alarms

import (
	"strings"
	"testing"
	"time"

	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/events"
	"cctv-sentinel/pkg/models"
)

func setupTestAlarmEngine(t *testing.T) (*Engine, *database.Store) {
	store := database.NewStore("test_alarms_store.json")
	store.UpsertDevice(models.Device{
		ID:           "test-cam-01",
		Name:         "Entrance Dahua 4K",
		IP:           "192.168.31.101",
		Port:         80,
		Manufacturer: models.VendorDahua,
		Model:        "DH-P3B-PV",
		SiteID:       "Branch-Headquarters",
	})

	logger := audit.NewLogger()
	evMgr := events.NewEventManager(store, logger)
	t.Cleanup(func() {
		evMgr.Close()
	})

	engine := NewEngine(store, logger, nil, evMgr)
	t.Cleanup(func() {
		engine.Close()
	})

	return engine, store
}

func TestAlarmGenerationAndSeverityMapping(t *testing.T) {
	engine, _ := setupTestAlarmEngine(t)

	// Ingest tampering event -> Rule should mark as CRITICAL
	ev := events.SurveillanceEvent{
		ID:           "ev-tamper-1",
		DeviceID:     "test-cam-01",
		DeviceIP:     "192.168.31.101",
		Manufacturer: "Dahua",
		Model:        "DH-P3B-PV",
		Category:     events.CategoryTampering,
		EventType:    "VideoTampering",
		Topic:        "VideoBlind",
		Source:       "Channel 1",
		Timestamp:    time.Now().UTC(),
		ReceivedAt:   time.Now().UTC(),
		IsActive:     true,
	}

	alm := engine.ProcessEvent(ev)
	if alm == nil {
		t.Fatalf("expected alarm to be generated")
	}

	if alm.Severity != SeverityCritical {
		t.Errorf("expected SeverityCritical for tampering, got %s", alm.Severity)
	}
	if alm.State != StateNew {
		t.Errorf("expected initial StateNew, got %s", alm.State)
	}
	if alm.SiteID != "Branch-Headquarters" {
		t.Errorf("expected SiteID 'Branch-Headquarters', got %s", alm.SiteID)
	}
}

func TestAlarmStateTransitions(t *testing.T) {
	engine, _ := setupTestAlarmEngine(t)

	ev := events.SurveillanceEvent{
		ID:           "ev-line-1",
		DeviceID:     "test-cam-01",
		Category:     events.CategoryLineCrossing,
		EventType:    "LineCrossing",
		Source:       "Channel 1",
		Timestamp:    time.Now().UTC(),
		ReceivedAt:   time.Now().UTC(),
		IsActive:     true,
	}
	alm := engine.ProcessEvent(ev)

	// 1. Acknowledge
	ack, err := engine.Acknowledge(alm.ID, "Operator_Alice")
	if err != nil {
		t.Fatalf("failed to acknowledge: %v", err)
	}
	if ack.State != StateAcknowledged || ack.AckBy != "Operator_Alice" {
		t.Errorf("unexpected acknowledge state: %+v", ack)
	}

	// 2. Assign
	assigned, err := engine.Assign(alm.ID, "Tech_Bob", "Operator_Alice")
	if err != nil {
		t.Fatalf("failed to assign: %v", err)
	}
	if assigned.State != StateInvestigating || assigned.AssignedTo != "Tech_Bob" {
		t.Errorf("unexpected assign state: %+v", assigned)
	}

	// 3. Resolve
	resolved, err := engine.Resolve(alm.ID, "Perimeter checked, authorized maintenance crew verified", "Tech_Bob")
	if err != nil {
		t.Fatalf("failed to resolve: %v", err)
	}
	if resolved.State != StateResolved || resolved.ResolvedBy != "Tech_Bob" {
		t.Errorf("unexpected resolve state: %+v", resolved)
	}
}

func TestAlarmFilteringAndExport(t *testing.T) {
	engine, _ := setupTestAlarmEngine(t)

	// Generate 3 alarms
	engine.ProcessEvent(events.SurveillanceEvent{
		ID: "ev-a", DeviceID: "test-cam-01", Category: events.CategoryMotion, EventType: "Motion", Source: "Ch 1", IsActive: true,
	})
	engine.ProcessEvent(events.SurveillanceEvent{
		ID: "ev-b", DeviceID: "test-cam-01", Category: events.CategoryTampering, EventType: "Tamper", Source: "Ch 1", IsActive: true,
	})

	// Filter Critical only
	criticals := engine.FilterAlarms(AlarmFilter{Severity: SeverityCritical})
	if len(criticals) != 1 {
		t.Errorf("expected 1 critical alarm, got %d", len(criticals))
	}

	// Stats check
	stats := engine.GetStats()
	if stats.TotalAlarms < 2 {
		t.Errorf("expected at least 2 alarms in stats, got %d", stats.TotalAlarms)
	}

	// CSV Export check
	csvData, err := engine.ExportCSV(AlarmFilter{})
	if err != nil {
		t.Fatalf("failed to export CSV: %v", err)
	}
	if !strings.Contains(string(csvData), "Alarm ID") || !strings.Contains(string(csvData), "test-cam-01") {
		t.Errorf("unexpected CSV content: %s", string(csvData))
	}
}
