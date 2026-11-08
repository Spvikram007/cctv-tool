package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/alarms"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/events"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/ptz"
	"cctv-sentinel/pkg/security"
)

func setupPhase7TestServer(t *testing.T) (*Server, http.Handler) {
	store := database.NewStore("test_phase7_api_store.json")
	registry := adapters.NewRegistry()
	logger := audit.NewLogger()
	licMgr := licensing.NewManager()
	configEng := configengine.NewEngine(registry, store, logger)
	discEng := discovery.NewEngine(registry)
	rbacMgr := security.NewRBACManager()

	// Seed PTZ camera (Vatilon H43)
	store.UpsertDevice(models.Device{
		ID:           "dev-vatilon-h43",
		Name:         "Vatilon H43",
		IP:           "192.168.31.104",
		Port:         80,
		Category:     models.CategoryPTZCamera,
		Model:        "H43",
		Manufacturer: "Vatilon",
		Capabilities: []string{"PTZ", "STREAMING"},
		Protocols:    []models.ProtocolType{models.ProtocolONVIF},
	})

	// Seed Fixed camera (Hikvision DS-2CD1023G0-IU)
	store.UpsertDevice(models.Device{
		ID:           "dev-hik-fixed",
		Name:         "Hikvision DS-2CD1023G0-IU",
		IP:           "192.168.31.103",
		Port:         80,
		Category:     models.CategoryIPCamera,
		Model:        "DS-2CD1023G0-IU",
		Manufacturer: models.VendorHikvision,
		Capabilities: []string{"STREAMING"},
		Protocols:    []models.ProtocolType{models.ProtocolISAPI},
	})

	srv := NewServer(store, registry, configEng, discEng, licMgr, logger, nil, nil, nil, nil, rbacMgr)
	return srv, srv.Handler()
}

func TestPhase7PTZAPIE2E(t *testing.T) {
	_, handler := setupPhase7TestServer(t)

	// 1. Check PTZ Capabilities on Vatilon H43 (Must be supported)
	req := httptest.NewRequest("GET", "/api/v1/devices/dev-vatilon-h43/ptz/capabilities", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for PTZ capabilities, got %d", w.Code)
	}

	var caps ptz.PTZCapabilities
	if err := json.NewDecoder(w.Body).Decode(&caps); err != nil {
		t.Fatalf("failed to decode capabilities: %v", err)
	}
	if !caps.PTZSupported || !caps.PanSupported || !caps.TiltSupported {
		t.Errorf("expected Vatilon H43 PTZ supported, got: %+v", caps)
	}

	// 2. Check PTZ Capabilities on Fixed Camera (Must NOT be supported)
	reqFixed := httptest.NewRequest("GET", "/api/v1/devices/dev-hik-fixed/ptz/capabilities", nil)
	wFixed := httptest.NewRecorder()
	handler.ServeHTTP(wFixed, reqFixed)

	if wFixed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wFixed.Code)
	}
	var capsFixed ptz.PTZCapabilities
	_ = json.NewDecoder(wFixed.Body).Decode(&capsFixed)
	if capsFixed.PTZSupported {
		t.Errorf("fixed camera DS-2CD1023G0-IU must strictly have PTZSupported=false")
	}

	// 3. Move Fixed Camera -> Must return 400 Bad Request
	moveBody := bytes.NewBufferString(`{"movement_type":"CONTINUOUS","pan":0.5,"tilt":0,"speed":0.5}`)
	reqMoveFixed := httptest.NewRequest("POST", "/api/v1/devices/dev-hik-fixed/ptz/move", moveBody)
	reqMoveFixed.Header.Set("Content-Type", "application/json")
	wMoveFixed := httptest.NewRecorder()
	handler.ServeHTTP(wMoveFixed, reqMoveFixed)

	if wMoveFixed.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request moving fixed camera, got %d", wMoveFixed.Code)
	}

	// 4. Create and Goto Preset on Vatilon H43
	presetBody := bytes.NewBufferString(`{"id":5,"name":"East Gate Observation"}`)
	reqPreset := httptest.NewRequest("POST", "/api/v1/devices/dev-vatilon-h43/ptz/presets", presetBody)
	reqPreset.Header.Set("Content-Type", "application/json")
	wPreset := httptest.NewRecorder()
	handler.ServeHTTP(wPreset, reqPreset)

	if wPreset.Code != http.StatusOK {
		t.Fatalf("expected 200 OK creating preset, got %d (%s)", wPreset.Code, wPreset.Body.String())
	}

	reqGoto := httptest.NewRequest("POST", "/api/v1/devices/dev-vatilon-h43/ptz/presets/5/goto", bytes.NewBufferString(`{"speed":0.8}`))
	reqGoto.Header.Set("Content-Type", "application/json")
	wGoto := httptest.NewRecorder()
	handler.ServeHTTP(wGoto, reqGoto)

	if wGoto.Code != http.StatusOK {
		t.Fatalf("expected 200 OK navigating to preset, got %d", wGoto.Code)
	}

	// 5. Test Stop Command
	reqStop := httptest.NewRequest("POST", "/api/v1/devices/dev-vatilon-h43/ptz/stop", nil)
	wStop := httptest.NewRecorder()
	handler.ServeHTTP(wStop, reqStop)
	if wStop.Code != http.StatusOK {
		t.Errorf("expected 200 OK for ptz stop, got %d", wStop.Code)
	}
}

func TestPhase7EventsAndAlarmsAPIE2E(t *testing.T) {
	srv, handler := setupPhase7TestServer(t)

	// 1. Create Event Subscription
	subBody := bytes.NewBufferString(`{"device_id":"dev-vatilon-h43","protocol":"ONVIF_PULLPOINT"}`)
	reqSub := httptest.NewRequest("POST", "/api/v1/events/subscriptions", subBody)
	reqSub.Header.Set("Content-Type", "application/json")
	wSub := httptest.NewRecorder()
	handler.ServeHTTP(wSub, reqSub)

	if wSub.Code != http.StatusOK {
		t.Fatalf("expected 200 OK creating event subscription, got %d", wSub.Code)
	}

	// 2. Query Live Events
	reqLive := httptest.NewRequest("GET", "/api/v1/events/live?limit=10", nil)
	wLive := httptest.NewRecorder()
	handler.ServeHTTP(wLive, reqLive)

	if wLive.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching live events, got %d", wLive.Code)
	}

	// 3. Ingest an Alarm directly via engine
	alm := srv.alarmEng.ProcessEvent(events.SurveillanceEvent{
		ID:           "test-ev-line",
		DeviceID:     "dev-vatilon-h43",
		DeviceIP:     "192.168.31.104",
		Manufacturer: "Vatilon",
		Model:        "H43",
		Category:     events.CategoryLineCrossing,
		EventType:    "LineCrossing",
		Source:       "Channel 1",
		Timestamp:    time.Now().UTC(),
		ReceivedAt:   time.Now().UTC(),
		IsActive:     true,
	})

	if alm == nil {
		t.Fatalf("expected alarm to be generated")
	}

	// 4. Query Alarms List
	reqAlms := httptest.NewRequest("GET", "/api/v1/alarms?device_id=dev-vatilon-h43", nil)
	wAlms := httptest.NewRecorder()
	handler.ServeHTTP(wAlms, reqAlms)

	if wAlms.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching alarms, got %d", wAlms.Code)
	}

	var alarmList []*alarms.Alarm
	if err := json.NewDecoder(wAlms.Body).Decode(&alarmList); err != nil {
		t.Fatalf("failed to decode alarms: %v", err)
	}
	if len(alarmList) == 0 {
		t.Fatalf("expected at least 1 alarm for dev-vatilon-h43")
	}

	targetAlmID := alarmList[0].ID

	// 5. Acknowledge Alarm
	reqAck := httptest.NewRequest("POST", "/api/v1/alarms/"+targetAlmID+"/acknowledge", nil)
	wAck := httptest.NewRecorder()
	handler.ServeHTTP(wAck, reqAck)

	if wAck.Code != http.StatusOK {
		t.Fatalf("expected 200 OK acknowledging alarm, got %d", wAck.Code)
	}

	// 6. Assign Alarm
	reqAssign := httptest.NewRequest("POST", "/api/v1/alarms/"+targetAlmID+"/assign", bytes.NewBufferString(`{"technician":"SurveillanceSpecialist"}`))
	reqAssign.Header.Set("Content-Type", "application/json")
	wAssign := httptest.NewRecorder()
	handler.ServeHTTP(wAssign, reqAssign)

	if wAssign.Code != http.StatusOK {
		t.Fatalf("expected 200 OK assigning alarm, got %d", wAssign.Code)
	}

	// 7. Resolve Alarm
	reqResolve := httptest.NewRequest("POST", "/api/v1/alarms/"+targetAlmID+"/resolve", bytes.NewBufferString(`{"notes":"Investigated perimeter: false trigger from foliage"}`))
	reqResolve.Header.Set("Content-Type", "application/json")
	wResolve := httptest.NewRecorder()
	handler.ServeHTTP(wResolve, reqResolve)

	if wResolve.Code != http.StatusOK {
		t.Fatalf("expected 200 OK resolving alarm, got %d", wResolve.Code)
	}

	// 8. Alarm Stats
	reqStats := httptest.NewRequest("GET", "/api/v1/alarms/stats", nil)
	wStats := httptest.NewRecorder()
	handler.ServeHTTP(wStats, reqStats)

	if wStats.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching alarm stats, got %d", wStats.Code)
	}

	// 9. Export Alarms CSV
	reqExport := httptest.NewRequest("GET", "/api/v1/alarms/export?format=csv", nil)
	wExport := httptest.NewRecorder()
	handler.ServeHTTP(wExport, reqExport)

	if wExport.Code != http.StatusOK {
		t.Fatalf("expected 200 OK exporting alarms CSV, got %d", wExport.Code)
	}
	if wExport.Header().Get("Content-Type") != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %s", wExport.Header().Get("Content-Type"))
	}
}
