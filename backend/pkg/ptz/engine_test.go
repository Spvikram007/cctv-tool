package ptz

import (
	"testing"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

func setupTestPTZEngine(t *testing.T) (*Engine, *database.Store) {
	store := database.NewStore("test_ptz_store.json")
	t.Cleanup(func() {
		// cleanup test store
	})

	registry := adapters.NewRegistry()
	logger := audit.NewLogger()
	engine := NewEngine(store, registry, logger)

	// Seed PTZ device (Vatilon H43)
	store.UpsertDevice(models.Device{
		ID:           "vatilon-h43",
		Name:         "Vatilon H43 PTZ Camera",
		IP:           "192.168.31.104",
		Port:         80,
		Category:     models.CategoryPTZCamera,
		Model:        "H43",
		Manufacturer: "Vatilon",
		Capabilities: []string{"PTZ", "STREAMING", "SNAPSHOT"},
		Protocols:    []models.ProtocolType{models.ProtocolONVIF},
	})

	// Seed Fixed camera (Hikvision DS-2CD1023G0-IU)
	store.UpsertDevice(models.Device{
		ID:           "hik-fixed",
		Name:         "Hikvision Fixed Bullet",
		IP:           "192.168.31.103",
		Port:         80,
		Category:     models.CategoryIPCamera,
		Model:        "DS-2CD1023G0-IU",
		Manufacturer: models.VendorHikvision,
		Capabilities: []string{"STREAMING", "SNAPSHOT"},
		Protocols:    []models.ProtocolType{models.ProtocolISAPI},
	})

	return engine, store
}

func TestPTZCapabilityDetection(t *testing.T) {
	engine, _ := setupTestPTZEngine(t)

	// 1. PTZ camera should have full capabilities
	caps, err := engine.GetCapabilities("vatilon-h43")
	if err != nil {
		t.Fatalf("failed to get capabilities for PTZ: %v", err)
	}
	if !caps.PTZSupported {
		t.Errorf("expected PTZSupported=true for Vatilon H43")
	}
	if !caps.PanSupported || !caps.TiltSupported || !caps.ZoomSupported {
		t.Errorf("expected Pan/Tilt/Zoom supported")
	}

	// 2. Fixed camera must strictly return PTZSupported=false
	fixedCaps, err := engine.GetCapabilities("hik-fixed")
	if err != nil {
		t.Fatalf("failed to get capabilities for fixed: %v", err)
	}
	if fixedCaps.PTZSupported {
		t.Errorf("expected PTZSupported=false for fixed camera DS-2CD1023G0-IU")
	}
}

func TestFixedCameraRejectsPTZMove(t *testing.T) {
	engine, _ := setupTestPTZEngine(t)

	// Moving a fixed camera must return ErrPTZNotSupported
	err := engine.ContinuousMove("hik-fixed", 0.5, 0, 0, 0.5, 0, "Tester")
	if err != ErrPTZNotSupported {
		t.Errorf("expected ErrPTZNotSupported for fixed camera, got: %v", err)
	}
}

func TestPresetsManagement(t *testing.T) {
	engine, _ := setupTestPTZEngine(t)

	// 1. Initial presets list
	presets, err := engine.GetPresets("vatilon-h43")
	if err != nil {
		t.Fatalf("failed to get presets: %v", err)
	}
	if len(presets) < 4 {
		t.Errorf("expected at least 4 default presets, got %d", len(presets))
	}

	// 2. Create Preset
	created, err := engine.CreatePreset("vatilon-h43", 5, "North Gate Observation", "Tester")
	if err != nil {
		t.Fatalf("failed to create preset: %v", err)
	}
	if created.ID != 5 || created.Name != "North Gate Observation" {
		t.Errorf("unexpected preset values: %+v", created)
	}

	// 3. Rename Preset
	renamed, err := engine.RenamePreset("vatilon-h43", 5, "Main Gate 1", "Tester")
	if err != nil {
		t.Fatalf("failed to rename preset: %v", err)
	}
	if renamed.Name != "Main Gate 1" {
		t.Errorf("expected renamed name 'Main Gate 1', got %s", renamed.Name)
	}

	// 4. Delete Preset
	if err := engine.DeletePreset("vatilon-h43", 5, "Tester"); err != nil {
		t.Fatalf("failed to delete preset: %v", err)
	}

	// 5. Verify deletion
	_, err = engine.RenamePreset("vatilon-h43", 5, "Should Fail", "Tester")
	if err != ErrPresetNotFound {
		t.Errorf("expected ErrPresetNotFound after deletion, got %v", err)
	}
}

func TestPatrolToursManagement(t *testing.T) {
	engine, _ := setupTestPTZEngine(t)

	// 1. Get default patrols
	patrols, err := engine.GetPatrols("vatilon-h43")
	if err != nil {
		t.Fatalf("failed to get patrols: %v", err)
	}
	if len(patrols) == 0 {
		t.Fatalf("expected seeded patrol tours")
	}

	// 2. Create custom patrol tour
	tour := PatrolTour{
		ID:          2,
		Name:        "Night Perimeter Scan",
		RepeatCount: 5,
		Steps: []PatrolStep{
			{StepNumber: 1, PresetID: 1, PresetName: "North Gate", DwellTimeSec: 2, Speed: 0.8},
			{StepNumber: 2, PresetID: 2, PresetName: "East Wall", DwellTimeSec: 2, Speed: 0.8},
		},
	}
	created, err := engine.CreatePatrol("vatilon-h43", tour, "Tester")
	if err != nil {
		t.Fatalf("failed to create patrol: %v", err)
	}
	if created.ID != 2 || len(created.Steps) != 2 {
		t.Errorf("unexpected patrol values: %+v", created)
	}

	// 3. Run Patrol & Stop Patrol
	if err := engine.RunPatrol("vatilon-h43", 2, "Tester"); err != nil {
		t.Fatalf("failed to run patrol: %v", err)
	}
	if err := engine.StopPatrol("vatilon-h43", 2, "Tester"); err != nil {
		t.Fatalf("failed to stop patrol: %v", err)
	}
}
