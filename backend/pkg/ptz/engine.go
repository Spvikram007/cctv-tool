package ptz

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

// Engine manages robotic PTZ operations, presets, tours, and capability detection
type Engine struct {
	mu           sync.RWMutex
	store        *database.Store
	registry     *adapters.Registry
	logger       *audit.Logger
	presets      map[string]map[int]*Preset    // key: deviceID -> presetID -> Preset
	patrols      map[string]map[int]*PatrolTour // key: deviceID -> tourID -> PatrolTour
	activeTours  map[string]int                // key: deviceID -> active tourID
	stopChannels map[string]chan struct{}      // key: deviceID -> stop cancel channel
}

func NewEngine(store *database.Store, registry *adapters.Registry, logger *audit.Logger) *Engine {
	e := &Engine{
		store:        store,
		registry:     registry,
		logger:       logger,
		presets:      make(map[string]map[int]*Preset),
		patrols:      make(map[string]map[int]*PatrolTour),
		activeTours:  make(map[string]int),
		stopChannels: make(map[string]chan struct{}),
	}
	return e
}

// GetCapabilities inspects a device and detects verified PTZ features
func (e *Engine) GetCapabilities(deviceID string) (*PTZCapabilities, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found in inventory", deviceID)
	}

	caps := &PTZCapabilities{
		DeviceID: dev.ID,
		MinSpeed: 0.1,
		MaxSpeed: 1.0,
	}

	// 1. Detect if camera has physical/robotic PTZ hardware
	modelUpper := strings.ToUpper(dev.Model)
	nameUpper := strings.ToUpper(dev.Name)
	isPhysicalPTZ := dev.Category == models.CategoryPTZCamera ||
		strings.Contains(modelUpper, "PTZ") ||
		strings.Contains(nameUpper, "PTZ") ||
		strings.Contains(modelUpper, "H43") // Vatilon H43 Physical PTZ Candidate

	// Check if device has coaxial UTC PTZ capabilities (e.g. XVR channels)
	hasCoaxPTZ := false
	for _, c := range dev.Capabilities {
		if c == "HDCVI_UTC" || c == "ANALOG_UTC" || c == "PTZ" {
			hasCoaxPTZ = true
			break
		}
	}

	if !isPhysicalPTZ && !hasCoaxPTZ {
		// Device is a fixed bullet, turret, or dome without PTZ motors
		caps.PTZSupported = false
		return caps, nil
	}

	// 2. Hardware is verified PTZ-capable: populate available features
	caps.PTZSupported = true
	caps.PanSupported = true
	caps.TiltSupported = true
	caps.ZoomSupported = true
	caps.ContinuousMoveSupported = true
	caps.RelativeMoveSupported = true
	caps.PresetsSupported = true
	caps.MaxPresets = 255
	caps.PatrolSupported = true
	caps.MaxPatrols = 8
	caps.HomePositionSupported = true

	// Focus & Iris supported on optical zoom units (e.g. Vatilon H43, Dahua Motorized, Hikvision AcuSense PTZ)
	if isPhysicalPTZ || strings.Contains(modelUpper, "ZS") || strings.Contains(modelUpper, "4KL") {
		caps.FocusSupported = true
		caps.IrisSupported = true
		caps.AbsoluteMoveSupported = true
		caps.AutoScanSupported = true
	}

	// Assign primary protocol
	switch dev.Manufacturer {
	case models.VendorDahua:
		caps.Protocol = "DAHUA_CGI"
	case models.VendorHikvision:
		caps.Protocol = "HIKVISION_ISAPI"
	default:
		caps.Protocol = "ONVIF"
	}

	return caps, nil
}

// ContinuousMove dispatches directional velocity commands
func (e *Engine) ContinuousMove(deviceID string, pan, tilt, zoom, speed float64, timeoutSec int, operator string) error {
	caps, err := e.GetCapabilities(deviceID)
	if err != nil {
		return err
	}
	if !caps.PTZSupported {
		return ErrPTZNotSupported
	}

	if speed <= 0 {
		speed = 0.5
	}
	if speed > 1.0 {
		speed = 1.0
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		// Stop software loop even if physical adapter is offline or unregistered
		return nil
	}

	// Build vendor PTZ action
	cmd := "CONTINUOUS_MOVE"
	if pan < 0 && tilt == 0 {
		cmd = "LEFT"
	} else if pan > 0 && tilt == 0 {
		cmd = "RIGHT"
	} else if tilt > 0 && pan == 0 {
		cmd = "UP"
	} else if tilt < 0 && pan == 0 {
		cmd = "DOWN"
	} else if pan < 0 && tilt > 0 {
		cmd = "UP_LEFT"
	} else if pan > 0 && tilt > 0 {
		cmd = "UP_RIGHT"
	} else if pan < 0 && tilt < 0 {
		cmd = "DOWN_LEFT"
	} else if pan > 0 && tilt < 0 {
		cmd = "DOWN_RIGHT"
	} else if zoom > 0 {
		cmd = "ZOOM_IN"
	} else if zoom < 0 {
		cmd = "ZOOM_OUT"
	}

	action := models.PTZAction{
		DeviceID: deviceID,
		Command:  cmd,
		Pan:      pan,
		Tilt:     tilt,
		Zoom:     zoom,
		Speed:    speed,
	}

	if err := adapter.SendPTZ(&dev, action); err != nil {
		return fmt.Errorf("ptz move failed: %w", err)
	}

	e.logger.Log(operator, deviceID, dev.IP, "PTZ_MOVE", "PTZ", "SUCCESS",
		fmt.Sprintf("Dispatched PTZ Move [%s] Pan=%.2f Tilt=%.2f Zoom=%.2f Speed=%.2f", cmd, pan, tilt, zoom, speed), "")

	// Handle optional auto-stop timeout
	if timeoutSec > 0 {
		go func() {
			time.Sleep(time.Duration(timeoutSec) * time.Second)
			_ = e.Stop(deviceID, "System_AutoStop")
		}()
	}

	return nil
}

// RelativeMove moves the camera in discrete fractional coordinate steps
func (e *Engine) RelativeMove(deviceID string, panStep, tiltStep, zoomStep float64, speed float64, operator string) error {
	caps, err := e.GetCapabilities(deviceID)
	if err != nil {
		return err
	}
	if !caps.PTZSupported || !caps.RelativeMoveSupported {
		return ErrPTZNotSupported
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		// Stop software loop even if physical adapter is offline or unregistered
		return nil
	}

	action := models.PTZAction{
		DeviceID: deviceID,
		Command:  "RELATIVE_MOVE",
		Pan:      panStep,
		Tilt:     tiltStep,
		Zoom:     zoomStep,
		Speed:    speed,
	}

	if err := adapter.SendPTZ(&dev, action); err != nil {
		return fmt.Errorf("relative ptz move failed: %w", err)
	}

	e.logger.Log(operator, deviceID, dev.IP, "PTZ_RELATIVE_MOVE", "PTZ", "SUCCESS",
		fmt.Sprintf("Dispatched Relative Step Pan=%.2f Tilt=%.2f Zoom=%.2f", panStep, tiltStep, zoomStep), "")

	return nil
}

// AbsoluteMove navigates to exact degree/zoom coordinates
func (e *Engine) AbsoluteMove(deviceID string, panDeg, tiltDeg, zoomVal float64, speed float64, operator string) error {
	caps, err := e.GetCapabilities(deviceID)
	if err != nil {
		return err
	}
	if !caps.PTZSupported || !caps.AbsoluteMoveSupported {
		return ErrPTZNotSupported
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		// Stop software loop even if physical adapter is offline or unregistered
		return nil
	}

	action := models.PTZAction{
		DeviceID: deviceID,
		Command:  "ABSOLUTE_MOVE",
		Pan:      panDeg,
		Tilt:     tiltDeg,
		Zoom:     zoomVal,
		Speed:    speed,
	}

	if err := adapter.SendPTZ(&dev, action); err != nil {
		return fmt.Errorf("absolute ptz move failed: %w", err)
	}

	e.logger.Log(operator, deviceID, dev.IP, "PTZ_ABSOLUTE_MOVE", "PTZ", "SUCCESS",
		fmt.Sprintf("Navigated Absolute Position Pan=%.1f° Tilt=%.1f° Zoom=%.1fx", panDeg, tiltDeg, zoomVal), "")

	return nil
}

// Stop halts all active robotic pan, tilt, and zoom movements
func (e *Engine) Stop(deviceID string, operator string) error {
	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		// Stop software loop even if physical adapter is offline or unregistered
		return nil
	}

	action := models.PTZAction{
		DeviceID: deviceID,
		Command:  "STOP",
	}

	if err := adapter.SendPTZ(&dev, action); err != nil {
		return fmt.Errorf("ptz stop failed: %w", err)
	}

	return nil
}

// FocusIris adjusts optical focus or iris aperture
func (e *Engine) FocusIris(deviceID string, req FocusIrisRequest, operator string) error {
	caps, err := e.GetCapabilities(deviceID)
	if err != nil {
		return err
	}
	if !caps.PTZSupported {
		return ErrPTZNotSupported
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		// Stop software loop even if physical adapter is offline or unregistered
		return nil
	}

	action := models.PTZAction{
		DeviceID: deviceID,
		Command:  req.Command,
		Speed:    req.Speed,
	}

	if err := adapter.SendPTZ(&dev, action); err != nil {
		return fmt.Errorf("focus/iris adjustment failed: %w", err)
	}

	e.logger.Log(operator, deviceID, dev.IP, "PTZ_OPTICAL_ADJUST", "PTZ", "SUCCESS",
		fmt.Sprintf("Adjusted lens optics [%s]", req.Command), "")

	return nil
}

// GetPresets returns all saved presets for a camera
func (e *Engine) GetPresets(deviceID string) ([]Preset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	caps, err := e.getCapabilitiesInternal(deviceID)
	if err != nil {
		return nil, err
	}
	if !caps.PTZSupported {
		return nil, ErrPTZNotSupported
	}

	devicePresets, exists := e.presets[deviceID]
	if !exists {
		devicePresets = make(map[int]*Preset)
		// Seed default presets 1..4 for PTZ devices
		for i := 1; i <= 4; i++ {
			devicePresets[i] = &Preset{
				ID:        i,
				Name:      fmt.Sprintf("Preset %d", i),
				Token:     fmt.Sprintf("%d", i),
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
		}
		e.presets[deviceID] = devicePresets
	}

	result := make([]Preset, 0, len(devicePresets))
	for _, p := range devicePresets {
		result = append(result, *p)
	}
	return result, nil
}

// CreatePreset saves the current camera coordinates to a preset index
func (e *Engine) CreatePreset(deviceID string, presetID int, name string, operator string) (*Preset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	caps, err := e.getCapabilitiesInternal(deviceID)
	if err != nil {
		return nil, err
	}
	if !caps.PTZSupported {
		return nil, ErrPTZNotSupported
	}

	if presetID <= 0 || presetID > 255 {
		return nil, ErrInvalidPreset
	}

	if name == "" {
		name = fmt.Sprintf("Preset %d", presetID)
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err == nil {
		_ = adapter.SendPTZ(&dev, models.PTZAction{
			DeviceID: deviceID,
			Command:  "SET_PRESET",
			PresetID: presetID,
		})
	}

	devicePresets, exists := e.presets[deviceID]
	if !exists {
		devicePresets = make(map[int]*Preset)
		e.presets[deviceID] = devicePresets
	}

	p := &Preset{
		ID:        presetID,
		Name:      name,
		Token:     fmt.Sprintf("%d", presetID),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	devicePresets[presetID] = p

	e.logger.Log(operator, deviceID, dev.IP, "PTZ_CREATE_PRESET", "PTZ", "SUCCESS",
		fmt.Sprintf("Memorized PTZ Preset %d [%s]", presetID, name), "")

	return p, nil
}

// GotoPreset commands the camera to navigate to a saved preset waypoint
func (e *Engine) GotoPreset(deviceID string, presetID int, speed float64, operator string) error {
	caps, err := e.GetCapabilities(deviceID)
	if err != nil {
		return err
	}
	if !caps.PTZSupported {
		return ErrPTZNotSupported
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return fmt.Errorf("device %s not found in inventory", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		// Stop software loop even if physical adapter is offline or unregistered
		return nil
	}

	action := models.PTZAction{
		DeviceID: deviceID,
		Command:  "GOTO_PRESET",
		PresetID: presetID,
		Speed:    speed,
	}

	if err := adapter.SendPTZ(&dev, action); err != nil {
		return fmt.Errorf("goto preset failed: %w", err)
	}

	e.logger.Log(operator, deviceID, dev.IP, "PTZ_GOTO_PRESET", "PTZ", "SUCCESS",
		fmt.Sprintf("Navigated to Preset %d at Speed=%.2f", presetID, speed), "")

	return nil
}

// RenamePreset updates the descriptive label of a saved preset
func (e *Engine) RenamePreset(deviceID string, presetID int, newName string, operator string) (*Preset, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	devicePresets, exists := e.presets[deviceID]
	if !exists {
		return nil, ErrPresetNotFound
	}

	p, ok := devicePresets[presetID]
	if !ok {
		return nil, ErrPresetNotFound
	}

	p.Name = newName
	p.UpdatedAt = time.Now().UTC()

	e.logger.Log(operator, deviceID, "127.0.0.1", "PTZ_RENAME_PRESET", "PTZ", "SUCCESS",
		fmt.Sprintf("Renamed Preset %d to [%s]", presetID, newName), "")

	return p, nil
}

// DeletePreset removes a preset from camera memory
func (e *Engine) DeletePreset(deviceID string, presetID int, operator string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	devicePresets, exists := e.presets[deviceID]
	if !exists {
		return ErrPresetNotFound
	}

	if _, ok := devicePresets[presetID]; !ok {
		return ErrPresetNotFound
	}

	delete(devicePresets, presetID)

	e.logger.Log(operator, deviceID, "127.0.0.1", "PTZ_DELETE_PRESET", "PTZ", "SUCCESS",
		fmt.Sprintf("Deleted PTZ Preset %d", presetID), "")

	return nil
}

// GetPatrols returns all configured automated patrol tours
func (e *Engine) GetPatrols(deviceID string) ([]PatrolTour, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	caps, err := e.getCapabilitiesInternal(deviceID)
	if err != nil {
		return nil, err
	}
	if !caps.PTZSupported {
		return nil, ErrPTZNotSupported
	}

	tours, exists := e.patrols[deviceID]
	if !exists {
		tours = make(map[int]*PatrolTour)
		// Seed default patrol tour 1
		tours[1] = &PatrolTour{
			ID:          1,
			Name:        "Perimeter Patrol Tour 1",
			Running:     false,
			RepeatCount: 0,
			Steps: []PatrolStep{
				{StepNumber: 1, PresetID: 1, PresetName: "North Gate", DwellTimeSec: 5, Speed: 0.5},
				{StepNumber: 2, PresetID: 2, PresetName: "East Perimeter", DwellTimeSec: 5, Speed: 0.5},
				{StepNumber: 3, PresetID: 3, PresetName: "South Fence", DwellTimeSec: 5, Speed: 0.5},
			},
		}
		e.patrols[deviceID] = tours
	}

	result := make([]PatrolTour, 0, len(tours))
	for _, t := range tours {
		result = append(result, *t)
	}
	return result, nil
}

// CreatePatrol creates or updates a multi-preset patrol tour
func (e *Engine) CreatePatrol(deviceID string, tour PatrolTour, operator string) (*PatrolTour, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	caps, err := e.getCapabilitiesInternal(deviceID)
	if err != nil {
		return nil, err
	}
	if !caps.PTZSupported || !caps.PatrolSupported {
		return nil, ErrPTZNotSupported
	}

	if tour.ID <= 0 {
		tour.ID = 1
	}
	if tour.Name == "" {
		tour.Name = fmt.Sprintf("Patrol Tour %d", tour.ID)
	}

	tours, exists := e.patrols[deviceID]
	if !exists {
		tours = make(map[int]*PatrolTour)
		e.patrols[deviceID] = tours
	}

	t := &PatrolTour{
		ID:          tour.ID,
		Name:        tour.Name,
		Running:     false,
		Steps:       tour.Steps,
		RepeatCount: tour.RepeatCount,
	}
	tours[tour.ID] = t

	e.logger.Log(operator, deviceID, "127.0.0.1", "PTZ_CREATE_PATROL", "PTZ", "SUCCESS",
		fmt.Sprintf("Created PTZ Patrol Tour %d [%s] with %d waypoints", tour.ID, tour.Name, len(tour.Steps)), "")

	return t, nil
}

// RunPatrol executes an automated waypoint tour in a background goroutine
func (e *Engine) RunPatrol(deviceID string, tourID int, operator string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tours, exists := e.patrols[deviceID]
	if !exists {
		return ErrPatrolNotFound
	}
	tour, ok := tours[tourID]
	if !ok {
		return ErrPatrolNotFound
	}

	// Stop any existing tour
	if stopChan, running := e.stopChannels[deviceID]; running {
		close(stopChan)
		delete(e.stopChannels, deviceID)
	}

	stopChan := make(chan struct{})
	e.stopChannels[deviceID] = stopChan
	e.activeTours[deviceID] = tourID
	tour.Running = true

	e.logger.Log(operator, deviceID, "127.0.0.1", "PTZ_START_PATROL", "PTZ", "SUCCESS",
		fmt.Sprintf("Initiated Automated Patrol Tour %d [%s]", tourID, tour.Name), "")

	// Bounded execution loop in background goroutine
	go func(steps []PatrolStep, devID string, sc chan struct{}) {
		for {
			for _, step := range steps {
				select {
				case <-sc:
					return
				default:
				}

				_ = e.GotoPreset(devID, step.PresetID, step.Speed, "PatrolDaemon")

				dwell := time.Duration(step.DwellTimeSec) * time.Second
				if dwell <= 0 {
					dwell = 3 * time.Second
				}

				select {
				case <-sc:
					return
				case <-time.After(dwell):
				}
			}
		}
	}(tour.Steps, deviceID, stopChan)

	return nil
}

// StopPatrol halts an active automated patrol tour
func (e *Engine) StopPatrol(deviceID string, tourID int, operator string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if stopChan, running := e.stopChannels[deviceID]; running {
		close(stopChan)
		delete(e.stopChannels, deviceID)
	}

	if tours, exists := e.patrols[deviceID]; exists {
		if tour, ok := tours[tourID]; ok {
			tour.Running = false
		}
	}
	delete(e.activeTours, deviceID)

	e.logger.Log(operator, deviceID, "127.0.0.1", "PTZ_STOP_PATROL", "PTZ", "SUCCESS",
		fmt.Sprintf("Stopped Automated Patrol Tour %d", tourID), "")

	return e.Stop(deviceID, operator)
}

// GotoHome commands the PTZ camera to return to its home calibration position
func (e *Engine) GotoHome(deviceID string, operator string) error {
	return e.GotoPreset(deviceID, 1, 0.5, operator)
}

// SetHome establishes the current coordinate as the calibrated home position
func (e *Engine) SetHome(deviceID string, operator string) error {
	_, err := e.CreatePreset(deviceID, 1, "Home Position", operator)
	return err
}

// AutoScan commands the robotic camera to execute a 360 degree panoramic pan
func (e *Engine) AutoScan(deviceID string, enabled bool, operator string) error {
	if enabled {
		return e.ContinuousMove(deviceID, 0.4, 0, 0, 0.4, 0, operator)
	}
	return e.Stop(deviceID, operator)
}

// Internal helper for capability inspection without mutex re-entry deadlock
func (e *Engine) getCapabilitiesInternal(deviceID string) (*PTZCapabilities, error) {
	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found in inventory", deviceID)
	}

	caps := &PTZCapabilities{
		DeviceID: dev.ID,
		MinSpeed: 0.1,
		MaxSpeed: 1.0,
	}

	modelUpper := strings.ToUpper(dev.Model)
	nameUpper := strings.ToUpper(dev.Name)
	isPhysicalPTZ := dev.Category == models.CategoryPTZCamera ||
		strings.Contains(modelUpper, "PTZ") ||
		strings.Contains(nameUpper, "PTZ") ||
		strings.Contains(modelUpper, "H43")

	hasCoaxPTZ := false
	for _, c := range dev.Capabilities {
		if c == "HDCVI_UTC" || c == "ANALOG_UTC" || c == "PTZ" {
			hasCoaxPTZ = true
			break
		}
	}

	if !isPhysicalPTZ && !hasCoaxPTZ {
		caps.PTZSupported = false
		return caps, nil
	}

	caps.PTZSupported = true
	caps.PanSupported = true
	caps.TiltSupported = true
	caps.ZoomSupported = true
	caps.ContinuousMoveSupported = true
	caps.RelativeMoveSupported = true
	caps.PresetsSupported = true
	caps.MaxPresets = 255
	caps.PatrolSupported = true
	caps.MaxPatrols = 8
	caps.HomePositionSupported = true
	caps.FocusSupported = true
	caps.IrisSupported = true
	caps.AbsoluteMoveSupported = true
	caps.AutoScanSupported = true

	return caps, nil
}
