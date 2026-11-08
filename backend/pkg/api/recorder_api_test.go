package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/api"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/batch"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/firmware"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/recorder"
	"cctv-sentinel/pkg/security"
)

func setupRecorderAPITestServer(t *testing.T) (*httptest.Server, *database.Store, *recorder.Engine, string) {
	tempDir := t.TempDir()
	store := database.NewStore(tempDir + "/sentinel_store.json")
	logger := audit.NewLogger()
	registry := adapters.NewRegistry()
	cfgEngine := configengine.NewEngine(registry, store, logger)
	discEngine := discovery.NewEngine(registry)
	licMgr := licensing.NewManager()
	batchEng := batch.NewEngine(registry, store, cfgEngine, logger)
	firmwareEng := firmware.NewEngine(store, registry, logger, tempDir+"/firmware")
	rbacMgr := security.NewRBACManager()
	recorderEng := recorder.NewEngine(store, logger)

	// Seed test devices: 1 XVR, 1 NVR
	store.UpsertDevice(models.Device{
		ID:           "test-xvr-5108",
		Name:         "Dahua WizSense XVR",
		IP:           "192.168.31.102",
		Port:         80,
		Manufacturer: models.VendorDahua,
		Model:        "DHI-XVR5108HS-4KL-I3",
		Category:     models.CategoryXVR,
		Status:       "online",
		ChannelCount: 16,
		TenantID:     "default",
		SiteID:       "hq",
	})

	store.UpsertDevice(models.Device{
		ID:           "test-nvr-4162",
		Name:         "CP PLUS 4K NVR",
		IP:           "192.168.31.143",
		Port:         80,
		Manufacturer: models.VendorCPPlus,
		Model:        "CP-UNR-4K4162-V2",
		Category:     models.CategoryNVR,
		Status:       "online",
		ChannelCount: 16,
		TenantID:     "default",
		SiteID:       "hq",
	})

	server := api.NewServer(store, registry, cfgEngine, discEngine, licMgr, logger, nil, nil, batchEng, firmwareEng, rbacMgr, recorderEng)
	ts := httptest.NewServer(server.Handler())
	token := server.IssueSessionForTest(security.RoleAdmin, "default", "hq")
	return ts, store, recorderEng, token
}

func TestPhase6RecorderAPIE2E(t *testing.T) {
	ts, _, _, token := setupRecorderAPITestServer(t)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	authReq := func(method, urlStr string, body string) *http.Request {
		var req *http.Request
		if body != "" {
			req, _ = http.NewRequest(method, urlStr, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, _ = http.NewRequest(method, urlStr, nil)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	// 1. GET /api/v1/recorders
	res, err := client.Do(authReq("GET", ts.URL+"/api/v1/recorders", ""))
	if err != nil {
		t.Fatalf("GET /recorders failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var profiles []recorder.RecorderProfile
	_ = json.NewDecoder(res.Body).Decode(&profiles)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 recorder profiles, got %d", len(profiles))
	}

	// 2. GET /api/v1/recorders/test-xvr-5108/profile
	res, err = client.Do(authReq("GET", ts.URL+"/api/v1/recorders/test-xvr-5108/profile", ""))
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	defer res.Body.Close()
	var xvrProf recorder.RecorderProfile
	_ = json.NewDecoder(res.Body).Decode(&xvrProf)
	if xvrProf.RecorderType != recorder.TypeXVR {
		t.Errorf("expected TypeXVR, got %s", xvrProf.RecorderType)
	}

	// 3. GET /api/v1/recorders/test-xvr-5108/channels
	res, err = client.Do(authReq("GET", ts.URL+"/api/v1/recorders/test-xvr-5108/channels", ""))
	if err != nil {
		t.Fatalf("GET /channels failed: %v", err)
	}
	defer res.Body.Close()
	var chans []recorder.ChannelInfo
	_ = json.NewDecoder(res.Body).Decode(&chans)
	if len(chans) != 16 {
		t.Fatalf("expected 16 channels, got %d", len(chans))
	}

	// 4. POST /api/v1/recorders/test-xvr-5108/channels/1/analog-format
	formatReq := `{"signal_format":"AHD"}`
	res, err = client.Do(authReq("POST", ts.URL+"/api/v1/recorders/test-xvr-5108/channels/1/analog-format", formatReq))
	if err != nil {
		t.Fatalf("POST /analog-format failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for format switch, got %d", res.StatusCode)
	}

	// 5. POST /api/v1/recorders/test-xvr-5108/channels/1/coaxial-control
	coaxReq := `{"command":"MENU_ENTER"}`
	res, err = client.Do(authReq("POST", ts.URL+"/api/v1/recorders/test-xvr-5108/channels/1/coaxial-control", coaxReq))
	if err != nil {
		t.Fatalf("POST /coaxial-control failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for coaxial control, got %d", res.StatusCode)
	}

	// 6. POST /api/v1/recorders/test-xvr-5108/channels/9/register-ip
	ipReq := `{"ip":"192.168.31.107","port":80,"protocol":"ONVIF","username":"admin","password":"password"}`
	res, err = client.Do(authReq("POST", ts.URL+"/api/v1/recorders/test-xvr-5108/channels/9/register-ip", ipReq))
	if err != nil {
		t.Fatalf("POST /register-ip failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for register-ip, got %d", res.StatusCode)
	}

	// 7. GET /api/v1/recorders/test-xvr-5108/channels/1/schedule
	res, err = client.Do(authReq("GET", ts.URL+"/api/v1/recorders/test-xvr-5108/channels/1/schedule", ""))
	if err != nil {
		t.Fatalf("GET /schedule failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for schedule, got %d", res.StatusCode)
	}

	// 8. GET /api/v1/recorders/test-xvr-5108/recordings/search?channel_id=1
	res, err = client.Do(authReq("GET", ts.URL+"/api/v1/recorders/test-xvr-5108/recordings/search?channel_id=1", ""))
	if err != nil {
		t.Fatalf("GET /recordings/search failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for search, got %d", res.StatusCode)
	}

	// 9. POST /api/v1/recorders/test-xvr-5108/recordings/playback-session
	playReq := `{"channel_id":1}`
	res, err = client.Do(authReq("POST", ts.URL+"/api/v1/recorders/test-xvr-5108/recordings/playback-session", playReq))
	if err != nil {
		t.Fatalf("POST /playback-session failed: %v", err)
	}
	defer res.Body.Close()
	var sess recorder.PlaybackSession
	_ = json.NewDecoder(res.Body).Decode(&sess)
	if !strings.HasPrefix(sess.PlaybackToken, "rec-play-") {
		t.Errorf("expected token prefix rec-play-, got %s", sess.PlaybackToken)
	}

	// 10. GET /api/v1/recorders/test-xvr-5108/storage
	res, err = client.Do(authReq("GET", ts.URL+"/api/v1/recorders/test-xvr-5108/storage", ""))
	if err != nil {
		t.Fatalf("GET /storage failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for storage, got %d", res.StatusCode)
	}

	// 11. POST /api/v1/recorders/test-xvr-5108/storage/format/preview & approve
	fmtReq := `{"disk_index":1}`
	res, err = client.Do(authReq("POST", ts.URL+"/api/v1/recorders/test-xvr-5108/storage/format/preview", fmtReq))
	if err != nil {
		t.Fatalf("POST /format/preview failed: %v", err)
	}
	defer res.Body.Close()
	var fmtPrev recorder.DiskFormatPreview
	_ = json.NewDecoder(res.Body).Decode(&fmtPrev)

	appReq := `{"format_token":"` + fmtPrev.FormatToken + `"}`
	res, err = client.Do(authReq("POST", ts.URL+"/api/v1/recorders/test-xvr-5108/storage/format/approve", appReq))
	if err != nil {
		t.Fatalf("POST /format/approve failed: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for format approve, got %d", res.StatusCode)
	}
}
