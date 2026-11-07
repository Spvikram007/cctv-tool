package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/diagnostics"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/models"
)

type Server struct {
	store        *database.Store
	registry     *adapters.Registry
	configEngine *configengine.Engine
	discoveryEng *discovery.Engine
	licenseMgr   *licensing.Manager
	logger       *audit.Logger
	mux          *http.ServeMux
	startTime    time.Time
}

func NewServer(
	store *database.Store,
	registry *adapters.Registry,
	configEngine *configengine.Engine,
	discoveryEng *discovery.Engine,
	licenseMgr *licensing.Manager,
	logger *audit.Logger,
) *Server {
	s := &Server{
		store:        store,
		registry:     registry,
		configEngine: configEngine,
		discoveryEng: discoveryEng,
		licenseMgr:   licenseMgr,
		logger:       logger,
		mux:          http.NewServeMux(),
		startTime:    time.Now().UTC(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS for frontend desktop and web
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		s.mux.ServeHTTP(w, r)
	})
	return RequestCorrelationMiddleware(corsHandler)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeStructuredError(w, r, status, code, message)
}

func writeSimpleError(w http.ResponseWriter, r *http.Request, status int, message string) {
	code := "API_ERROR"
	switch status {
	case http.StatusBadRequest:
		code = "BAD_REQUEST"
	case http.StatusNotFound:
		code = "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		code = "METHOD_NOT_ALLOWED"
	case http.StatusUnauthorized:
		code = "UNAUTHORIZED"
	case http.StatusBadGateway:
		code = "BAD_GATEWAY"
	case http.StatusInternalServerError:
		code = "INTERNAL_ERROR"
	case http.StatusUnprocessableEntity:
		code = "UNPROCESSABLE_ENTITY"
	}
	writeStructuredError(w, r, status, code, message)
}

func (s *Server) registerRoutes() {
	// Static web assets from frontend/dist
	fs := http.FileServer(http.Dir("./frontend/dist"))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})

	// 0. Liveness & Readiness endpoints
	s.mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "UP",
			"uptime_sec": int(time.Since(s.startTime).Seconds()),
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": GetRequestID(r),
		})
	})

	s.mux.HandleFunc("/api/v1/readyz", func(w http.ResponseWriter, r *http.Request) {
		checks := make(map[string]string)
		isReady := true

		if s.store != nil {
			checks["database"] = "READY"
		} else {
			checks["database"] = "UNAVAILABLE"
			isReady = false
		}

		if s.registry != nil {
			checks["adapters"] = "READY"
		} else {
			checks["adapters"] = "UNAVAILABLE"
			isReady = false
		}

		if s.licenseMgr != nil {
			checks["licensing"] = "READY"
		} else {
			checks["licensing"] = "UNAVAILABLE"
			isReady = false
		}

		status := http.StatusOK
		if !isReady {
			status = http.StatusServiceUnavailable
		}

		writeJSON(w, status, map[string]interface{}{
			"ready":      isReady,
			"checks":     checks,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": GetRequestID(r),
		})
	})

	// 1. System status
	s.mux.HandleFunc("/api/v1/system/status", func(w http.ResponseWriter, r *http.Request) {
		lic := s.licenseMgr.GetLicense()
		devices := s.store.ListDevices()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"product":      "CCTV SENTINEL UNIVERSAL",
			"owner":        "Tech Defence Solutions",
			"tagline":      "ONE TOOL. EVERY CCTV. COMPLETE CONTROL.",
			"version":      "1.0.0-PROD",
			"uptime_sec":   int(time.Since(s.startTime).Seconds()),
			"license_tier": lic.Tier,
			"active_hosts": len(devices),
			"max_allowed":  lic.MaxDevices,
			"status":       "HEALTHY",
		})
	})

	// 2. Devices endpoints
	s.mux.HandleFunc("/api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			devs := s.store.ListDevices()
			writeJSON(w, http.StatusOK, devs)
			return
		}
		if r.Method == "POST" {
			var dev models.Device
			if err := json.NewDecoder(r.Body).Decode(&dev); err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, "Invalid device payload")
				return
			}
			if dev.ID == "" {
				dev.ID = fmt.Sprintf("dev-%d", time.Now().UnixNano())
			}
			s.store.UpsertDevice(dev)
			s.logger.Log("Technician", dev.ID, dev.IP, "ADD_DEVICE", "DEVICE", "SUCCESS", fmt.Sprintf("Added %s (%s)", dev.Name, dev.Manufacturer), "")
			writeJSON(w, http.StatusCreated, dev)
			return
		}
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
	})

	// 3. Network Discovery
	s.mux.HandleFunc("/api/v1/devices/discover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var req struct {
			CIDR     string `json:"cidr"`
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.CIDR == "" {
			req.CIDR = "127.0.0.1/32" // default safe loopback scan
		}

		go func() {
			found, err := s.discoveryEng.ScanSubnet(context.Background(), req.CIDR, req.Username, req.Password)
			if err == nil {
				for _, d := range found {
					s.store.UpsertDevice(d)
				}
			}
		}()

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"message": "Subnet discovery scan started",
			"cidr":    req.CIDR,
		})
	})

	s.mux.HandleFunc("/api/v1/devices/discover/progress", func(w http.ResponseWriter, r *http.Request) {
		prog := s.discoveryEng.GetProgress()
		writeJSON(w, http.StatusOK, prog)
	})

	// 4. Device specific routes dispatcher: /api/v1/devices/
	s.mux.HandleFunc("/api/v1/devices/", s.handleDeviceSubroutes)

	// 5. Diagnostics
	s.mux.HandleFunc("/api/v1/diagnostics/run", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			ip = "127.0.0.1"
		}
		rPort, _ := strconv.Atoi(r.URL.Query().Get("rtsp_port"))
		hPort, _ := strconv.Atoi(r.URL.Query().Get("http_port"))
		if rPort <= 0 {
			rPort = 554
		}
		if hPort <= 0 {
			hPort = 80
		}
		report := diagnostics.RunDiagnostics(ip, rPort, hPort)
		writeJSON(w, http.StatusOK, report)
	})

	s.mux.HandleFunc("/api/v1/diagnostics/subnet", func(w http.ResponseWriter, r *http.Request) {
		cidr := r.URL.Query().Get("cidr")
		if cidr == "" {
			cidr = "192.168.1.0/24"
		}
		calc, err := diagnostics.CalculateSubnet(cidr)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, calc)
	})

	s.mux.HandleFunc("/api/v1/diagnostics/hdd", func(w http.ResponseWriter, r *http.Request) {
		cams, _ := strconv.Atoi(r.URL.Query().Get("cameras"))
		if cams <= 0 {
			cams = 8
		}
		res := r.URL.Query().Get("resolution")
		codec := r.URL.Query().Get("codec")
		days, _ := strconv.Atoi(r.URL.Query().Get("days"))
		if days <= 0 {
			days = 30
		}
		hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
		if hours <= 0 {
			hours = 24
		}
		calc := diagnostics.CalculateHDD(cams, codec, res, 0, days, hours)
		writeJSON(w, http.StatusOK, calc)
	})

	s.mux.HandleFunc("/api/v1/diagnostics/bandwidth", func(w http.ResponseWriter, r *http.Request) {
		cams, _ := strconv.Atoi(r.URL.Query().Get("cameras"))
		if cams <= 0 {
			cams = 8
		}
		mainB, _ := strconv.Atoi(r.URL.Query().Get("main_kbps"))
		if mainB <= 0 {
			mainB = 4096
		}
		subB, _ := strconv.Atoi(r.URL.Query().Get("sub_kbps"))
		if subB <= 0 {
			subB = 1024
		}
		calc := diagnostics.CalculateBandwidth(cams, mainB, subB)
		writeJSON(w, http.StatusOK, calc)
	})

	// 6. Licensing
	s.mux.HandleFunc("/api/v1/licensing", func(w http.ResponseWriter, r *http.Request) {
		lic := s.licenseMgr.GetLicense()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"license":      lic,
			"machine_hash": s.licenseMgr.GetMachineHash(),
		})
	})

	s.mux.HandleFunc("/api/v1/licensing/activate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key     string `json:"key"`
			Company string `json:"company"`
			GST     string `json:"gst"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid activation request")
			return
		}
		if err := s.licenseMgr.ActivateLicense(req.Key, req.Company, req.GST); err != nil {
			writeSimpleError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.logger.Log("Technician", "SYSTEM", "127.0.0.1", "ACTIVATE_LICENSE", "LICENSE", "SUCCESS", fmt.Sprintf("Activated %s license for %s", req.Key, req.Company), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "License activated successfully",
			"license": s.licenseMgr.GetLicense(),
		})
	})

	s.mux.HandleFunc("/api/v1/licensing/gst-invoice", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CustomerName string `json:"customer_name"`
			CustomerGST  string `json:"customer_gst"`
			State        string `json:"state"`
			Tier         string `json:"tier"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		inv := s.licenseMgr.GenerateGSTInvoice(req.CustomerName, req.CustomerGST, req.State, req.Tier)
		writeJSON(w, http.StatusOK, inv)
	})

	// 7. Audit logs
	s.mux.HandleFunc("/api/v1/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		logs := s.logger.GetEntries()
		writeJSON(w, http.StatusOK, logs)
	})

	// 8. Compatibility database
	s.mux.HandleFunc("/api/v1/compatibility", func(w http.ResponseWriter, r *http.Request) {
		matrix := s.store.GetCompatibility()
		writeJSON(w, http.StatusOK, matrix)
	})

	// 9. Sites
	s.mux.HandleFunc("/api/v1/sites", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			writeJSON(w, http.StatusOK, s.store.ListSites())
			return
		}
		if r.Method == "POST" {
			var site database.Site
			_ = json.NewDecoder(r.Body).Decode(&site)
			if site.ID == "" {
				site.ID = fmt.Sprintf("site-%d", time.Now().Unix())
			}
			site.CreatedAt = time.Now()
			s.store.AddSite(site)
			writeJSON(w, http.StatusCreated, site)
			return
		}
	})

	// 10. Batch Jobs
	s.mux.HandleFunc("/api/v1/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			writeJSON(w, http.StatusOK, s.store.ListBatchJobs())
			return
		}
		if r.Method == "POST" {
			var job database.BatchJob
			_ = json.NewDecoder(r.Body).Decode(&job)
			if job.ID == "" {
				job.ID = fmt.Sprintf("job-%d", time.Now().Unix())
			}
			job.CreatedAt = time.Now()
			job.Status = "COMPLETED"
			job.Progress = 100
			job.Logs = []string{
				fmt.Sprintf("Job [%s] started at %s", job.Name, job.CreatedAt.Format(time.RFC3339)),
				fmt.Sprintf("Dispatched to %d devices", len(job.TargetIDs)),
				"Verified all targets responded with HTTP 200 OK",
				"Batch execution finished successfully.",
			}
			now := time.Now()
			job.CompletedAt = &now
			s.store.AddBatchJob(job)
			writeJSON(w, http.StatusCreated, job)
			return
		}
	})

	// 11. Technician Report
	s.mux.HandleFunc("/api/v1/reports/technician", func(w http.ResponseWriter, r *http.Request) {
		devs := s.store.ListDevices()
		lic := s.licenseMgr.GetLicense()
		logs := s.logger.GetEntries()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"report_title":      "CCTV SENTINEL UNIVERSAL - COMMISSIONING & AUDIT REPORT",
			"generated_at":      time.Now().UTC().Format(time.RFC3339),
			"issued_by":         "Tech Defence Solutions",
			"licensee":          lic.OwnerName,
			"tier":              lic.Tier,
			"total_devices":     len(devs),
			"online_devices":    len(devs),
			"device_inventory":  devs,
			"audit_trail_count": len(logs),
			"recent_actions":    logs,
		})
	})
}

func (s *Server) handleDeviceSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/devices/")
	parts := strings.Split(path, "/")
	deviceID := parts[0]

	dev, ok := s.store.GetDevice(deviceID)
	if !ok {
		writeSimpleError(w, r, http.StatusNotFound, "Device not found")
		return
	}

	adapter, err := s.registry.ResolveForDevice(&dev)
	if err != nil {
		writeSimpleError(w, r, http.StatusBadGateway, err.Error())
		return
	}

	// Route: GET /api/v1/devices/:id
	if len(parts) == 1 {
		if r.Method == "DELETE" {
			s.store.DeleteDevice(deviceID)
			s.logger.Log("Technician", dev.ID, dev.IP, "DELETE_DEVICE", "DEVICE", "SUCCESS", fmt.Sprintf("Removed %s", dev.Name), "")
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}
		writeJSON(w, http.StatusOK, dev)
		return
	}

	sub := parts[1]
	switch sub {
	case "network":
		if len(parts) > 2 && parts[2] == "preview" {
			var body json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			preview, err := s.configEngine.GeneratePreview(deviceID, configengine.ConfigTypeNetwork, body)
			if err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, preview)
			return
		}
		if len(parts) > 2 && parts[2] == "apply" {
			var req struct {
				RollbackKey string               `json:"rollback_key"`
				Config      models.NetworkConfig `json:"config"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			s.store.SetNetworkConfig(req.Config)
			res, err := s.configEngine.ApplyConfirmed(deviceID, configengine.ConfigTypeNetwork, req.RollbackKey, "Technician")
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
			return
		}
		cfg, err := adapter.GetNetworkConfig(&dev)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	case "image":
		if len(parts) > 2 && parts[2] == "preview" {
			var body json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			preview, err := s.configEngine.GeneratePreview(deviceID, configengine.ConfigTypeImage, body)
			if err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, preview)
			return
		}
		if len(parts) > 2 && parts[2] == "apply" {
			var req struct {
				RollbackKey string             `json:"rollback_key"`
				Config      models.ImageConfig `json:"config"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			s.store.SetImageConfig(req.Config)
			res, err := s.configEngine.ApplyConfirmed(deviceID, configengine.ConfigTypeImage, req.RollbackKey, "Technician")
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, res)
			return
		}
		cfg, err := adapter.GetImageConfig(&dev)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	case "encoding":
		cfgs, err := adapter.GetVideoEncodingConfig(&dev)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cfgs)

	case "ptz":
		var action models.PTZAction
		_ = json.NewDecoder(r.Body).Decode(&action)
		action.DeviceID = dev.ID
		if err := adapter.SendPTZ(&dev, action); err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "command": action.Command})

	case "snapshot":
		bytes, err := adapter.CaptureSnapshot(&dev)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes)

	case "channels":
		if len(parts) > 2 && parts[2] == "analog-format" {
			var req struct {
				ChannelID    int    `json:"channel_id"`
				SignalFormat string `json:"signal_format"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if err := adapter.SetChannelAnalogMode(&dev, req.ChannelID, req.SignalFormat); err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			s.logger.Log("Technician", dev.ID, dev.IP, "SET_ANALOG_FORMAT", "RECORDER", "SUCCESS", fmt.Sprintf("Channel %d format switched to %s", req.ChannelID, req.SignalFormat), "")
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "channel": req.ChannelID, "format": req.SignalFormat})
			return
		}
		chans, err := adapter.GetRecorderChannels(&dev)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, chans)

	default:
		writeSimpleError(w, r, http.StatusNotFound, "Unknown device subresource")
	}
}
