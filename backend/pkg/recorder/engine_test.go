package recorder_test

import (
	"strings"
	"testing"
	"time"

	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/recorder"
)

func setupTestRecorderEngine(t *testing.T) (*recorder.Engine, *database.Store) {
	tempDir := t.TempDir()
	store := database.NewStore(tempDir + "/sentinel_store.json")
	logger := audit.NewLogger()
	eng := recorder.NewEngine(store, logger)

	// Seed test devices: 1 NVR, 1 XVR, 1 DVR
	store.UpsertDevice(models.Device{
		ID:           "test-nvr-01",
		Name:         "CP PLUS 4K NVR",
		IP:           "192.168.31.143",
		Port:         80,
		Manufacturer: "CP PLUS",
		Model:        "CP-UNR-4K4162-V2",
		Category:     models.CategoryNVR,
		Status:       "ONLINE",
		ChannelCount: 16,
	})

	store.UpsertDevice(models.Device{
		ID:           "test-xvr-01",
		Name:         "Dahua WizSense XVR",
		IP:           "192.168.31.102",
		Port:         80,
		Manufacturer: "Dahua",
		Model:        "DHI-XVR5108HS-4KL-I3",
		Category:     models.CategoryXVR,
		Status:       "ONLINE",
		ChannelCount: 16,
	})

	store.UpsertDevice(models.Device{
		ID:           "test-dvr-01",
		Name:         "Hikvision TurboHD DVR",
		IP:           "192.168.31.103",
		Port:         80,
		Manufacturer: "Hikvision",
		Model:        "DS-7208HQHI-K1",
		Category:     models.CategoryDVR,
		Status:       "ONLINE",
		ChannelCount: 12,
	})

	return eng, store
}

func TestGetRecorderProfile(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	// 1. Test NVR Profile
	nvrProf, err := eng.GetRecorderProfile("test-nvr-01")
	if err != nil {
		t.Fatalf("GetRecorderProfile NVR failed: %v", err)
	}
	if nvrProf.RecorderType != recorder.TypeNVR {
		t.Errorf("Expected TypeNVR, got %s", nvrProf.RecorderType)
	}
	if nvrProf.PhysicalAnalogChannels != 0 {
		t.Errorf("NVR must have 0 physical analog channels, got %d", nvrProf.PhysicalAnalogChannels)
	}
	if nvrProf.SupportedIPChannels != 16 {
		t.Errorf("Expected 16 IP channels, got %d", nvrProf.SupportedIPChannels)
	}

	// 2. Test XVR Profile
	xvrProf, err := eng.GetRecorderProfile("test-xvr-01")
	if err != nil {
		t.Fatalf("GetRecorderProfile XVR failed: %v", err)
	}
	if xvrProf.RecorderType != recorder.TypeXVR {
		t.Errorf("Expected TypeXVR, got %s", xvrProf.RecorderType)
	}
	if xvrProf.PhysicalAnalogChannels != 8 {
		t.Errorf("Expected 8 analog BNC channels, got %d", xvrProf.PhysicalAnalogChannels)
	}
	if !xvrProf.CoaxialControlSupported {
		t.Errorf("XVR must support Coaxial UTC control")
	}
}

func TestEnumerateChannels(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	// Enumerate XVR Channels (8 Analog + 8 IP = 16 channels)
	channels, err := eng.EnumerateChannels("test-xvr-01")
	if err != nil {
		t.Fatalf("EnumerateChannels failed: %v", err)
	}
	if len(channels) != 16 {
		t.Fatalf("Expected 16 channels, got %d", len(channels))
	}

	// Verify first 8 channels are Analog
	for i := 0; i < 8; i++ {
		if channels[i].ChannelType != recorder.ChannelTypeAnalog {
			t.Errorf("Channel %d should be ANALOG, got %s", i+1, channels[i].ChannelType)
		}
	}

	// Verify next 8 channels are IP
	for i := 8; i < 16; i++ {
		if channels[i].ChannelType != recorder.ChannelTypeIP {
			t.Errorf("Channel %d should be IP, got %s", i+1, channels[i].ChannelType)
		}
	}
}

func TestAnalogFormatSwitching(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	// 1. Switch format on valid analog channel on XVR (Channel 1 -> AHD)
	res, err := eng.SetAnalogFormat("test-xvr-01", 1, recorder.FormatAHD, "Technician")
	if err != nil {
		t.Fatalf("SetAnalogFormat failed: %v", err)
	}
	if res.CurrentFormat != recorder.FormatAHD {
		t.Errorf("Expected format AHD, got %s", res.CurrentFormat)
	}

	// 2. Verify pure IP NVR rejects analog format switching
	_, err = eng.SetAnalogFormat("test-nvr-01", 1, recorder.FormatCVI, "Technician")
	if err == nil {
		t.Errorf("NVR must reject analog format switching, but succeeded")
	}
	if !strings.Contains(err.Error(), "analog signal formats are not supported") {
		t.Errorf("Expected ErrAnalogNotSupported, got %v", err)
	}
}

func TestCoaxialControl(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	err := eng.SendCoaxialControl("test-xvr-01", 1, "MENU_ENTER", "Technician")
	if err != nil {
		t.Fatalf("SendCoaxialControl failed: %v", err)
	}

	// Invalid command must be rejected
	err = eng.SendCoaxialControl("test-xvr-01", 1, "INVALID_CMD", "Technician")
	if err == nil {
		t.Errorf("Invalid command should have failed")
	}
}

func TestRegisterIPCamera(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	regReq := recorder.IPRegistrationReq{
		ChannelID:   9, // Digital channel on XVR
		IP:          "192.168.31.107",
		Port:        80,
		Protocol:    "ONVIF",
		Username:    "admin",
		Password:    "TestPass123",
		StreamIndex: 0,
	}

	ch, err := eng.RegisterIPCamera("test-xvr-01", regReq, "Technician")
	if err != nil {
		t.Fatalf("RegisterIPCamera failed: %v", err)
	}
	if ch.ConnectedCameraIP != "192.168.31.107" {
		t.Errorf("Expected connected IP 192.168.31.107, got %s", ch.ConnectedCameraIP)
	}

	// SSRF Protection: Attempt to map cloud metadata IP must fail
	badReq := recorder.IPRegistrationReq{
		ChannelID: 10,
		IP:        "169.254.169.254",
		Port:      80,
	}
	_, err = eng.RegisterIPCamera("test-xvr-01", badReq, "Technician")
	if err == nil {
		t.Errorf("SSRF protection must block registration of 169.254.169.254")
	}
}

func TestRecordingSchedule(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	sched, err := eng.GetRecordingSchedule("test-xvr-01", 1)
	if err != nil {
		t.Fatalf("GetRecordingSchedule failed: %v", err)
	}
	if sched.PreRecordSec != 5 {
		t.Errorf("Expected default PreRecordSec=5, got %d", sched.PreRecordSec)
	}

	// Update schedule
	sched.PreRecordSec = 10
	sched.PostRecordSec = 60
	updated, err := eng.UpdateRecordingSchedule("test-xvr-01", *sched, "Technician")
	if err != nil {
		t.Fatalf("UpdateRecordingSchedule failed: %v", err)
	}
	if updated.PreRecordSec != 10 || updated.PostRecordSec != 60 {
		t.Errorf("Schedule update failed to persist")
	}
}

func TestSearchRecordingsAndPlayback(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	start := time.Now().UTC().Add(-6 * time.Hour)
	end := time.Now().UTC()

	// 1. Search recordings
	segments, err := eng.SearchRecordings("test-xvr-01", 1, start, end, "ALL")
	if err != nil {
		t.Fatalf("SearchRecordings failed: %v", err)
	}
	if len(segments) == 0 {
		t.Errorf("Expected recording segments, got 0")
	}

	// 2. Create Playback Session
	sess, err := eng.CreatePlaybackSession("test-xvr-01", 1, start, end, "Technician")
	if err != nil {
		t.Fatalf("CreatePlaybackSession failed: %v", err)
	}
	if !strings.HasPrefix(sess.PlaybackToken, "rec-play-") {
		t.Errorf("Expected token prefix rec-play-, got %s", sess.PlaybackToken)
	}

	// 3. Export Recording Segment
	tmpDir := t.TempDir()
	outPath, err := eng.ExportRecording("test-xvr-01", 1, segments[0].ID, tmpDir, "Technician")
	if err != nil {
		t.Fatalf("ExportRecording failed: %v", err)
	}
	if !strings.HasSuffix(outPath, ".mp4") {
		t.Errorf("Expected .mp4 extension, got %s", outPath)
	}
}

func TestStorageHealthAndRetentionEstimator(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	report, err := eng.GetStorageHealth("test-nvr-01")
	if err != nil {
		t.Fatalf("GetStorageHealth failed: %v", err)
	}
	if report.TotalDisks != 2 {
		t.Errorf("NVR expected 2 disks, got %d", report.TotalDisks)
	}
	if report.TotalCapacityGB <= 0 {
		t.Errorf("Expected non-zero total storage capacity")
	}
	if report.EstimatedRetentionDays <= 0 {
		t.Errorf("Expected non-zero estimated retention days")
	}

	// Direct Retention Formula Test
	// 8 channels @ 2048 kbps, 24 hours/day, 4000 GB capacity
	ret := eng.EstimateRetention(4000, 8, 2048, 24.0)
	if ret.DailyStorageGB <= 0 || ret.TotalRetentionDays <= 0 {
		t.Errorf("Invalid retention estimate calculation: %+v", ret)
	}
}

func TestDiskFormatPreviewAndApprove(t *testing.T) {
	eng, _ := setupTestRecorderEngine(t)

	// 1. Stage Format Preview
	preview, err := eng.StageFormatDiskPreview("test-nvr-01", 1, "Technician")
	if err != nil {
		t.Fatalf("StageFormatDiskPreview failed: %v", err)
	}
	if !strings.HasPrefix(preview.FormatToken, "fmtprv-") {
		t.Errorf("Expected token prefix fmtprv-, got %s", preview.FormatToken)
	}

	// 2. Approve Format with valid token
	err = eng.ApproveFormatDisk(preview.FormatToken, "Technician")
	if err != nil {
		t.Fatalf("ApproveFormatDisk failed: %v", err)
	}

	// 3. Re-using same token must fail (single-use enforcement)
	err = eng.ApproveFormatDisk(preview.FormatToken, "Technician")
	if err == nil {
		t.Errorf("Token reuse must be rejected")
	}
}
