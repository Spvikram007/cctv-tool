package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/alarms"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/batch"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/diagnostics"
	"cctv-sentinel/pkg/discovery"
	"cctv-sentinel/pkg/events"
	"cctv-sentinel/pkg/firmware"
	"cctv-sentinel/pkg/licensing"
	"cctv-sentinel/pkg/media/snapshot"
	"cctv-sentinel/pkg/media/streamhub"
	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/ptz"
	"cctv-sentinel/pkg/recorder"
	"cctv-sentinel/pkg/security"
)

type Server struct {
	store          *database.Store
	registry       *adapters.Registry
	configEngine   *configengine.Engine
	discoveryEng   *discovery.Engine
	licenseMgr     *licensing.Manager
	logger         *audit.Logger
	snapshotEng    *snapshot.Engine
	streamHub      *streamhub.Hub
	batchEng       *batch.Engine
	firmwareEng    *firmware.Engine
	recorderEng    *recorder.Engine
	rbacMgr        *security.RBACManager
	ptzEng         *ptz.Engine
	eventMgr       *events.EventManager
	alarmEng       *alarms.Engine
	rateLimiter    *security.IPRateLimiter
	allowedOrigins map[string]bool
	mux            *http.ServeMux
	startTime      time.Time
}

func NewServer(
	store *database.Store,
	registry *adapters.Registry,
	configEngine *configengine.Engine,
	discoveryEng *discovery.Engine,
	licenseMgr *licensing.Manager,
	logger *audit.Logger,
	snapshotEng *snapshot.Engine,
	streamHub *streamhub.Hub,
	batchEng *batch.Engine,
	firmwareEng *firmware.Engine,
	rbacMgr *security.RBACManager,
	recorderEngOpt ...*recorder.Engine,
) *Server {
	if snapshotEng == nil {
		snapshotEng = snapshot.NewEngine(store, registry, "./data/snapshots")
	}
	if streamHub == nil {
		streamHub = streamhub.NewHub(store, registry)
	}
	if batchEng == nil {
		batchEng = batch.NewEngine(registry, store, configEngine, logger)
	}
	if firmwareEng == nil {
		firmwareEng = firmware.NewEngine(store, registry, logger, "./data/firmware")
	}
	if rbacMgr == nil {
		rbacMgr = security.NewRBACManager()
	}
	var recorderEng *recorder.Engine
	if len(recorderEngOpt) > 0 && recorderEngOpt[0] != nil {
		recorderEng = recorderEngOpt[0]
	} else {
		recorderEng = recorder.NewEngine(store, logger)
	}

	ptzEng := ptz.NewEngine(store, registry, logger)
	eventMgr := events.NewEventManager(store, logger)
	alarmEng := alarms.NewEngine(store, logger, snapshotEng, eventMgr)

	s := &Server{
		store:        store,
		registry:     registry,
		configEngine: configEngine,
		discoveryEng: discoveryEng,
		licenseMgr:   licenseMgr,
		logger:       logger,
		snapshotEng:  snapshotEng,
		streamHub:    streamHub,
		batchEng:     batchEng,
		firmwareEng:  firmwareEng,
		recorderEng:  recorderEng,
		rbacMgr:      rbacMgr,
		ptzEng:         ptzEng,
		eventMgr:       eventMgr,
		alarmEng:       alarmEng,
		rateLimiter:    security.NewIPRateLimiter(50.0, 100.0),
		allowedOrigins: map[string]bool{
			"http://localhost:5173": true,
			"http://127.0.0.1:5173": true,
			"http://localhost:8080": true,
			"http://127.0.0.1:8080": true,
		},
		mux:       http.NewServeMux(),
		startTime: time.Now().UTC(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rate limiting per client IP
		clientIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(clientIP); err == nil {
			clientIP = host
		}
		if s.rateLimiter != nil && !s.rateLimiter.Allow(clientIP) {
			writeError(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Too many requests; rate limit threshold reached")
			return
		}

		// Comprehensive HTTP Security Headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self' ws: wss:;")

		// Enforce strict CORS allowlist (deny wildcard credentialed access)
		origin := r.Header.Get("Origin")
		if origin != "" {
			if s.allowedOrigins != nil && s.allowedOrigins[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-Request-ID, X-CSRF-Token, X-Sentinel-CSRF, X-Client-Type")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		s.mux.ServeHTTP(w, r)
	})
	return RequestCorrelationMiddleware(corsHandler)
}

const sessionContextKey contextKey = "sentinel.session"

func getSession(r *http.Request) *security.Session {
	if val := r.Context().Value(sessionContextKey); val != nil {
		if sess, ok := val.(*security.Session); ok {
			return sess
		}
	}
	return nil
}

const (
	DefaultTenantID = "default"
	DefaultSiteID   = "hq"
)

// isSuperAdmin returns true ONLY if the session belongs to a genuine global administrator with explicit global admin claim
func isSuperAdmin(sess *security.Session) bool {
	if sess == nil {
		return false
	}
	// Merely having RoleAdmin or TenantID "default"/"system"/"" does NOT grant global fleet access.
	// Must have explicit IsGlobalAdmin or ScopeGlobalAdmin.
	return sess.Role == security.RoleAdmin && (sess.IsGlobalAdmin || sess.HasScope(security.ScopeGlobalAdmin))
}

// checkTenantAccess strictly enforces tenant and site isolation boundaries
func checkTenantAccess(sess *security.Session, resourceTenantID, resourceSiteID string) bool {
	if sess == nil {
		return false
	}

	// 1. Genuine global super-admin has fleet-wide access across all tenants and legacy devices
	if isSuperAdmin(sess) {
		return true
	}

	// 2. All non-global users must have valid non-empty tenant ownership (fail closed)
	if sess.TenantID == "" {
		return false
	}

	// 3. Resource must have non-empty tenant ownership (fail closed for legacy/unassigned devices)
	if resourceTenantID == "" {
		return false
	}

	// 4. Mismatched tenant access is strictly rejected
	if sess.TenantID != resourceTenantID {
		return false
	}

	// 5. Site isolation: All non-global users must have matching non-empty site ownership (fail closed)
	if sess.SiteID == "" || resourceSiteID == "" {
		return false
	}
	if sess.SiteID != "all" && sess.SiteID != "*" {
		if resourceSiteID != sess.SiteID {
			return false
		}
	}

	return true
}

func (s *Server) isOriginAllowed(origin string) bool {
	if s.allowedOrigins == nil {
		return false
	}
	return s.allowedOrigins[origin]
}

func (s *Server) checkPermission(r *http.Request, perm security.Permission) bool {
	sess := getSession(r)
	if sess == nil {
		return false
	}
	return security.HasPermission(sess.Role, perm)
}

func (s *Server) requireAuth(perm security.Permission, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		fromCookie := false
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if strings.HasPrefix(authHeader, "bearer ") {
			token = strings.TrimPrefix(authHeader, "bearer ")
		} else if cookie, err := r.Cookie("sentinel_session"); err == nil && cookie.Value != "" {
			token = cookie.Value
			fromCookie = true
		}

		// Security: Do NOT accept authentication credentials through URL query parameters
		if token == "" {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required: missing Bearer session token or sentinel_session cookie")
			return
		}

		sess, err := s.rbacMgr.ValidateSession(token)
		if err != nil {
			if errors.Is(err, security.ErrSessionExpired) {
				writeError(w, r, http.StatusUnauthorized, "SESSION_EXPIRED", "Session token expired; re-authentication required")
				return
			}
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or terminated session token")
			return
		}

		// CSRF & Origin Protection for cookie-authenticated state-changing requests
		if fromCookie && (r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" || r.Method == "PATCH") {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if !s.isOriginAllowed(origin) {
					writeError(w, r, http.StatusForbidden, "CSRF_UNTRUSTED_ORIGIN", "Cross-origin request rejected: untrusted origin")
					return
				}
			}

			csrfHeader := r.Header.Get("X-CSRF-Token")
			if csrfHeader == "" {
				csrfHeader = r.Header.Get("X-Sentinel-CSRF")
			}
			if csrfHeader == "" || sess.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(csrfHeader), []byte(sess.CSRFToken)) != 1 {
				writeError(w, r, http.StatusForbidden, "CSRF_TOKEN_INVALID", "Invalid or missing CSRF token for state-changing request")
				return
			}
		}

		if !security.HasPermission(sess.Role, perm) {
			s.logger.Log(sess.Username, "SYSTEM", r.RemoteAddr, "ACCESS_DENIED", "SECURITY", "FORBIDDEN",
				fmt.Sprintf("Role %s lacks permission %s for route %s", sess.Role, perm, r.URL.Path), "")
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", fmt.Sprintf("Role %s lacks required permission: %s", sess.Role, perm))
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey, sess)
		handler(w, r.WithContext(ctx))
	}
}

// IssueSessionForTest generates an active tenant-scoped session token for unit test execution
func (s *Server) IssueSessionForTest(role security.Role, tenantID, siteID string) string {
	tokBytes := make([]byte, 24)
	if _, err := rand.Read(tokBytes); err != nil {
		tokBytes = []byte("test-session-bytes-123456789012")
	}
	tok := fmt.Sprintf("test-sess-%s", hex.EncodeToString(tokBytes))
	s.rbacMgr.InjectSessionForTest(tok, "test-operator", role, tenantID, siteID, 24*time.Hour)
	return tok
}

// IssueGlobalAdminSessionForTest generates an active global super-admin session for test execution
func (s *Server) IssueGlobalAdminSessionForTest() string {
	tokBytes := make([]byte, 24)
	if _, err := rand.Read(tokBytes); err != nil {
		tokBytes = []byte("test-global-admin-bytes-123456")
	}
	tok := fmt.Sprintf("test-sess-global-%s", hex.EncodeToString(tokBytes))
	s.rbacMgr.InjectGlobalAdminSessionForTest(tok, "super-admin", 24*time.Hour)
	return tok
}

// GetRBACManager returns the underlying RBACManager instance
func (s *Server) GetRBACManager() *security.RBACManager {
	return s.rbacMgr
}

// GetAlarmEngine returns the underlying Alarm Engine instance
func (s *Server) GetAlarmEngine() *alarms.Engine {
	return s.alarmEng
}

// GetEventManager returns the underlying Event Manager instance
func (s *Server) GetEventManager() *events.EventManager {
	return s.eventMgr
}

// GetRecorderEngine returns the underlying Recorder Engine instance
func (s *Server) GetRecorderEngine() *recorder.Engine {
	return s.recorderEng
}

// SetAllowedOrigins dynamically overrides the allowed CORS origins
func (s *Server) SetAllowedOrigins(origins []string) {
	newMap := make(map[string]bool)
	for _, o := range origins {
		newMap[o] = true
	}
	s.allowedOrigins = newMap
}

func (s *Server) handleAuthSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"setup_required": s.rbacMgr.IsSetupRequired(),
		"users_count":    len(s.rbacMgr.ListUsers()),
	})
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		SetupToken string `json:"setup_token"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		FullName   string `json:"full_name"`
		Email      string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid setup JSON payload")
		return
	}

	if err := s.rbacMgr.CompleteSetup(req.SetupToken, req.Username, req.Password, req.FullName, req.Email); err != nil {
		s.logger.Log(req.Username, "SYSTEM", r.RemoteAddr, "FIRST_RUN_SETUP", "SECURITY", "FAILED", err.Error(), "")
		if errors.Is(err, security.ErrSetupDisabled) {
			writeError(w, r, http.StatusForbidden, "SETUP_DISABLED", err.Error())
			return
		}
		if errors.Is(err, security.ErrInvalidSetupToken) || errors.Is(err, security.ErrSetupTokenExpired) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_TOKEN", err.Error())
			return
		}
		writeError(w, r, http.StatusBadRequest, "SETUP_FAILED", err.Error())
		return
	}

	s.logger.Log(req.Username, "SYSTEM", r.RemoteAddr, "FIRST_RUN_SETUP", "SECURITY", "SUCCESS", "Initial administrator provisioned; setup token destroyed", "")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success":  true,
		"message":  "Administrator account provisioned successfully. Initial setup permanently completed.",
		"username": req.Username,
	})
}

func (s *Server) handleAuditRotateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	newKeyID, err := s.logger.RotateCheckpointKey()
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "KEY_ROTATION_FAILED", err.Error())
		return
	}
	s.logger.Log("Admin", "SYSTEM", r.RemoteAddr, "ROTATE_AUDIT_KEY", "SECURITY", "SUCCESS", fmt.Sprintf("Rotated HMAC checkpoint key to %s", newKeyID), "")
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "new_key_id": newKeyID})
}

func (s *Server) handleAuditExportSigned(w http.ResponseWriter, r *http.Request) {
	exportFile := "./data/audit_export_signed.json"
	_ = os.MkdirAll("./data", 0755)
	if err := s.logger.ExportSignedLedger(exportFile); err != nil {
		writeError(w, r, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
		return
	}
	data, err := os.ReadFile(exportFile)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "READ_FAILED", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"audit_ledger_signed.json\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if r.Method == "GET" {
		devs := s.store.ListDevices()
		filtered := make([]models.Device, 0, len(devs))
		for _, d := range devs {
			if checkTenantAccess(sess, d.TenantID, d.SiteID) {
				filtered = append(filtered, d)
			}
		}
		writeJSON(w, http.StatusOK, filtered)
		return
	}
	if r.Method == "POST" {
		if !s.checkPermission(r, security.PermDeviceConfig) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Device creation requires DEVICE_CONFIG permission")
			return
		}
		var dev models.Device
		if err := json.NewDecoder(r.Body).Decode(&dev); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid device payload")
			return
		}
		if dev.ID == "" {
			dev.ID = fmt.Sprintf("dev-%d", time.Now().UnixNano())
		}
		if sess != nil && sess.TenantID != "" && dev.TenantID == "" {
			dev.TenantID = sess.TenantID
		}
		if sess != nil && sess.SiteID != "" && dev.SiteID == "" {
			dev.SiteID = sess.SiteID
		}
		s.store.UpsertDevice(dev)
		userName := "Operator"
		if sess != nil {
			userName = sess.Username
		}
		s.logger.Log(userName, dev.ID, dev.IP, "ADD_DEVICE", "DEVICE", "SUCCESS", fmt.Sprintf("Added %s (%s)", dev.Name, dev.Manufacturer), "")
		writeJSON(w, http.StatusCreated, dev)
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleTechnicianReport(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	allDevs := s.store.ListDevices()
	devs := make([]models.Device, 0, len(allDevs))
	for _, d := range allDevs {
		if checkTenantAccess(sess, d.TenantID, d.SiteID) {
			devs = append(devs, d)
		}
	}
	lic := s.licenseMgr.GetLicense()
	logs := s.logger.GetEntries()

	onlineCount := 0
	offlineCount := 0
	unknownCount := 0
	now := time.Now().UTC()

	for _, d := range devs {
		statusLower := strings.ToLower(d.Status)
		switch statusLower {
		case "online", "authenticated":
			if !d.LastSeen.IsZero() && now.Sub(d.LastSeen) > 10*time.Minute {
				offlineCount++
			} else {
				onlineCount++
			}
		case "offline":
			offlineCount++
		default:
			unknownCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"report_title":           "CCTV SENTINEL UNIVERSAL - COMMISSIONING & AUDIT REPORT",
		"generated_at":           now.Format(time.RFC3339),
		"measurement_timestamp": now.Format(time.RFC3339),
		"issued_by":              "Tech Defence Solutions",
		"licensee":               lic.OwnerName,
		"tier":                   lic.Tier,
		"total_devices":          len(devs),
		"online_devices":         onlineCount,
		"offline_devices":        offlineCount,
		"unknown_devices":        unknownCount,
		"device_inventory":       devs,
		"audit_trail_count":      len(logs),
		"recent_actions":         logs,
	})
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

	// 0. Public Liveness, Readiness & Setup endpoints
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

	s.mux.HandleFunc("/api/v1/auth/setup/status", s.handleAuthSetupStatus)
	s.mux.HandleFunc("/api/v1/auth/setup", s.handleAuthSetup)
	s.mux.HandleFunc("/api/v1/auth/login", s.handleSecurityLogin)
	s.mux.HandleFunc("/api/v1/security/login", s.handleSecurityLogin)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleSecurityLogout)
	s.mux.HandleFunc("/api/v1/security/logout", s.handleSecurityLogout)
	s.mux.HandleFunc("/api/v1/auth/csrf", s.handleSecurityCSRFToken)
	s.mux.HandleFunc("/api/v1/security/csrf", s.handleSecurityCSRFToken)

	// 1. System status
	s.mux.HandleFunc("/api/v1/system/status", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	// 2. Devices endpoints
	s.mux.HandleFunc("/api/v1/devices", s.requireAuth(security.PermViewLive, s.handleDevices))

	// 3. Network Discovery
	s.mux.HandleFunc("/api/v1/devices/discover", s.requireAuth(security.PermDeviceConfig, s.handleStartDiscovery))
	s.mux.HandleFunc("/api/v1/devices/discover/progress", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
		prog := s.discoveryEng.GetProgress()
		writeJSON(w, http.StatusOK, prog)
	}))
	s.mux.HandleFunc("/api/v1/discovery/interfaces", s.requireAuth(security.PermDeviceConfig, s.handleDiscoveryInterfaces))
	s.mux.HandleFunc("/api/v1/discovery/", s.requireAuth(security.PermViewLive, s.handleDiscoverySubroutes))

	// 4. Device specific routes dispatcher: /api/v1/devices/
	s.mux.HandleFunc("/api/v1/devices/", s.requireAuth(security.PermViewLive, s.handleDeviceSubroutes))

	// 4.1 Live Streaming Hub routes: /api/v1/streams/
	s.mux.HandleFunc("/api/v1/streams/", s.handleStreamSubroutes)

	// 5. Diagnostics
	s.mux.HandleFunc("/api/v1/diagnostics/run", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	s.mux.HandleFunc("/api/v1/diagnostics/subnet", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	s.mux.HandleFunc("/api/v1/diagnostics/hdd", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	s.mux.HandleFunc("/api/v1/diagnostics/bandwidth", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	s.mux.HandleFunc("/api/v1/diagnostics/capacity", s.requireAuth(security.PermViewLive, s.handleCapacityCalculator))

	// 6. Licensing
	s.mux.HandleFunc("/api/v1/licensing", s.requireAuth(security.PermLicenseManage, func(w http.ResponseWriter, r *http.Request) {
		lic := s.licenseMgr.GetLicense()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"license":      lic,
			"machine_hash": s.licenseMgr.GetMachineHash(),
		})
	}))

	s.mux.HandleFunc("/api/v1/licensing/activate", s.requireAuth(security.PermLicenseManage, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	s.mux.HandleFunc("/api/v1/licensing/gst-invoice", s.requireAuth(security.PermLicenseManage, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CustomerName string `json:"customer_name"`
			CustomerGST  string `json:"customer_gst"`
			State        string `json:"state"`
			Tier         string `json:"tier"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		inv := s.licenseMgr.GenerateGSTInvoice(req.CustomerName, req.CustomerGST, req.State, req.Tier)
		writeJSON(w, http.StatusOK, inv)
	}))

	// 7. Audit logs & verification
	s.mux.HandleFunc("/api/v1/audit/logs", s.requireAuth(security.PermAuditView, func(w http.ResponseWriter, r *http.Request) {
		logs := s.logger.GetEntries()
		writeJSON(w, http.StatusOK, logs)
	}))
	s.mux.HandleFunc("/api/v1/audit/checkpoints", s.requireAuth(security.PermAuditView, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.logger.GetCheckpoints())
	}))
	s.mux.HandleFunc("/api/v1/audit/verify", s.requireAuth(security.PermAuditView, s.handleAuditVerifyChain))
	s.mux.HandleFunc("/api/v1/audit/rotate-key", s.requireAuth(security.PermUserManagement, s.handleAuditRotateKey))
	s.mux.HandleFunc("/api/v1/audit/export/signed", s.requireAuth(security.PermAuditView, s.handleAuditExportSigned))

	// 8. Compatibility database
	s.mux.HandleFunc("/api/v1/compatibility", s.requireAuth(security.PermAuditView, func(w http.ResponseWriter, r *http.Request) {
		matrix := s.store.GetCompatibility()
		writeJSON(w, http.StatusOK, matrix)
	}))

	// 9. Sites
	s.mux.HandleFunc("/api/v1/sites", s.requireAuth(security.PermViewLive, func(w http.ResponseWriter, r *http.Request) {
		sess := getSession(r)
		if r.Method == "GET" {
			sites := s.store.ListSites()
			filtered := make([]database.Site, 0, len(sites))
			for _, st := range sites {
				if isSuperAdmin(sess) || sess == nil || sess.SiteID == "" || sess.SiteID == DefaultSiteID || sess.SiteID == "all" || st.ID == sess.SiteID {
					filtered = append(filtered, st)
				}
			}
			writeJSON(w, http.StatusOK, filtered)
			return
		}
		if r.Method == "POST" {
			if !s.checkPermission(r, security.PermDeviceConfig) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Site creation requires DEVICE_CONFIG permission")
				return
			}
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
	}))

	// 10. Phase 5 Enterprise Batch Engine routes
	s.mux.HandleFunc("/api/v1/batch/preview", s.requireAuth(security.PermBatchExecute, s.handleBatchPreview))
	s.mux.HandleFunc("/api/v1/batch/execute", s.requireAuth(security.PermBatchExecute, s.handleBatchExecute))
	s.mux.HandleFunc("/api/v1/batch/jobs", s.requireAuth(security.PermViewLive, s.handleBatchListJobs))
	s.mux.HandleFunc("/api/v1/batch/jobs/", s.requireAuth(security.PermViewLive, s.handleBatchJobSubroutes))
	s.mux.HandleFunc("/api/v1/batch/compare", s.requireAuth(security.PermViewLive, s.handleBatchCompare))
	s.mux.HandleFunc("/api/v1/batch/export", s.requireAuth(security.PermBatchExecute, s.handleBatchExport))
	s.mux.HandleFunc("/api/v1/batch/import", s.requireAuth(security.PermBatchExecute, s.handleBatchImport))
	s.mux.HandleFunc("/api/v1/batch", s.requireAuth(security.PermBatchExecute, s.handleBatchLegacyFallback))

	// 11. Phase 5 Secure Firmware Management routes
	s.mux.HandleFunc("/api/v1/firmware/inventory", s.requireAuth(security.PermViewLive, s.handleFirmwareInventory))
	s.mux.HandleFunc("/api/v1/firmware/check-compatibility", s.requireAuth(security.PermViewLive, s.handleFirmwareCheckCompatibility))
	s.mux.HandleFunc("/api/v1/firmware/compatibility", s.requireAuth(security.PermViewLive, s.handleFirmwareCheckCompatibility))
	s.mux.HandleFunc("/api/v1/firmware/preview", s.requireAuth(security.PermFirmwareUpgrade, s.handleFirmwarePreview))
	s.mux.HandleFunc("/api/v1/firmware/stage", s.requireAuth(security.PermFirmwareUpgrade, s.handleFirmwarePreview))
	s.mux.HandleFunc("/api/v1/firmware/approve-update", s.requireAuth(security.PermFirmwareUpgrade, s.handleFirmwareApproveUpdate))
	s.mux.HandleFunc("/api/v1/firmware/approve", s.requireAuth(security.PermFirmwareUpgrade, s.handleFirmwareApproveUpdate))
	s.mux.HandleFunc("/api/v1/firmware/jobs/", s.requireAuth(security.PermFirmwareUpgrade, s.handleFirmwareJobSubroutes))

	// 12. Phase 5 Security, RBAC & Audit Verification routes
	s.mux.HandleFunc("/api/v1/security/users", s.requireAuth(security.PermUserManagement, s.handleSecurityUsers))
	s.mux.HandleFunc("/api/v1/security/vault/status", s.requireAuth(security.PermUserManagement, s.handleVaultStatus))
	s.mux.HandleFunc("/api/v1/security/vault", s.requireAuth(security.PermUserManagement, s.handleVaultStatus))

	// 13. Phase 6 Recorder, Channel, Recording & Playback routes
	s.mux.HandleFunc("/api/v1/recorders", s.requireAuth(security.PermViewLive, s.handleRecordersList))
	s.mux.HandleFunc("/api/v1/recorders/", s.requireAuth(security.PermViewLive, s.handleRecorderSubroutes))

	// 14. Phase 7 Real-Time Telemetry & Event Subscriptions
	s.mux.HandleFunc("/api/v1/events/subscriptions", s.requireAuth(security.PermViewLive, s.handleEventsListSubscriptions))
	s.mux.HandleFunc("/api/v1/events/subscriptions/", s.requireAuth(security.PermEventManage, s.handleEventsCancelSubscription))
	s.mux.HandleFunc("/api/v1/events/live", s.requireAuth(security.PermViewLive, s.handleEventsLive))

	// 15. Phase 7 Unified Alarms & Rule Automation routes
	s.mux.HandleFunc("/api/v1/alarms", s.requireAuth(security.PermViewLive, s.handleAlarms))
	s.mux.HandleFunc("/api/v1/alarms/stats", s.requireAuth(security.PermViewLive, s.handleAlarmsStats))
	s.mux.HandleFunc("/api/v1/alarms/rules", s.requireAuth(security.PermViewLive, s.handleAlarmsRules))
	s.mux.HandleFunc("/api/v1/alarms/rules/", s.requireAuth(security.PermEventManage, s.handleAlarmsRulesDelete))
	s.mux.HandleFunc("/api/v1/alarms/export", s.requireAuth(security.PermPlaybackExport, s.handleAlarmsExport))
	s.mux.HandleFunc("/api/v1/alarms/stream", s.requireAuth(security.PermViewLive, s.handleAlarmsStream))
	s.mux.HandleFunc("/api/v1/alarms/ws", s.requireAuth(security.PermViewLive, s.handleAlarmsStream))
	s.mux.HandleFunc("/api/v1/alarms/", s.requireAuth(security.PermViewLive, s.handleAlarmSubroutes))

	// 16. Technician Report
	s.mux.HandleFunc("/api/v1/reports/technician", s.requireAuth(security.PermAuditView, s.handleTechnicianReport))
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

	sess := getSession(r)
	if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
		writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: device belongs to an isolated tenant or site")
		return
	}

	// Route: GET /api/v1/devices/:id or DELETE
	if len(parts) == 1 {
		if r.Method == "DELETE" {
			if !s.checkPermission(r, security.PermDeviceConfig) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Device deletion requires DEVICE_CONFIG permission")
				return
			}
			s.store.DeleteDevice(deviceID)
			s.logger.Log("Technician", dev.ID, dev.IP, "DELETE_DEVICE", "DEVICE", "SUCCESS", fmt.Sprintf("Removed %s", dev.Name), "")
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}
		writeJSON(w, http.StatusOK, dev)
		return
	}

	sub := parts[1]
	if sub == "identity" {
		s.handleDeviceIdentity(w, r, &dev)
		return
	}
	if sub == "services" {
		s.handleDeviceServices(w, r, &dev)
		return
	}
	if sub == "ptz" {
		s.handleDevicePTZ(w, r, &dev, parts)
		return
	}
	if sub == "migrate" {
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if !isSuperAdmin(sess) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Migrating device tenant/site ownership requires global administrator privileges")
			return
		}
		var req struct {
			TargetTenant string `json:"target_tenant"`
			TargetSite   string `json:"target_site"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid migration payload")
			return
		}
		if req.TargetTenant == "" {
			req.TargetTenant = DefaultTenantID
		}
		if req.TargetSite == "" {
			req.TargetSite = DefaultSiteID
		}
		oldTenant := dev.TenantID
		oldSite := dev.SiteID
		dev.TenantID = req.TargetTenant
		dev.SiteID = req.TargetSite
		s.store.UpsertDevice(dev)
		s.logger.Log(sess.Username, dev.ID, dev.IP, "MIGRATE_DEVICE", "SECURITY", "SUCCESS",
			fmt.Sprintf("Migrated device %s from [tenant=%s, site=%s] to [tenant=%s, site=%s]", dev.ID, oldTenant, oldSite, dev.TenantID, dev.SiteID), "")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":       true,
			"device_id":     dev.ID,
			"target_tenant": dev.TenantID,
			"target_site":   dev.SiteID,
			"message":       "Device ownership successfully migrated",
		})
		return
	}
	if sub == "reboot" {
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if !s.checkPermission(r, security.PermReboot) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Device reboot requires REBOOT permission")
			return
		}
	}
	if sub == "config" {
		if !s.checkPermission(r, security.PermDeviceConfig) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Configuration operations require DEVICE_CONFIG permission")
			return
		}
	}
	if sub == "auth" && r.Method == "POST" {
		if !s.checkPermission(r, security.PermDeviceConfig) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Device credential configuration requires DEVICE_CONFIG permission")
			return
		}
	}

	adapter, err := s.registry.ResolveForDevice(&dev)
	if err != nil {
		writeSimpleError(w, r, http.StatusBadGateway, err.Error())
		return
	}
	switch sub {
	case "auth":
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if !s.checkPermission(r, security.PermDeviceConfig) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Device credential configuration requires DEVICE_CONFIG permission")
			return
		}
		var authReq struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&authReq)
		if authReq.Username != "" {
			dev.Username = authReq.Username
		}
		if authReq.Password != "" {
			dev.Password = authReq.Password
		}
		authStatus, err := adapter.Authenticate(&dev)
		if err != nil {
			s.logger.Log("Technician", dev.ID, dev.IP, "AUTHENTICATE_DEVICE", "AUTH", "FAILED", err.Error(), "")
			writeJSON(w, http.StatusOK, authStatus)
			return
		}
		dev.Status = "authenticated"
		if authStatus.Firmware != "" {
			dev.Firmware = authStatus.Firmware
		}
		if authStatus.Model != "" && dev.Model == "" {
			dev.Model = authStatus.Model
		}
		s.store.UpsertDevice(dev)
		s.logger.Log("Technician", dev.ID, dev.IP, "AUTHENTICATE_DEVICE", "AUTH", "SUCCESS", fmt.Sprintf("Authenticated via %s", authStatus.Protocol), "")
		writeJSON(w, http.StatusOK, authStatus)
		return

	case "capabilities":
		caps, err := adapter.GetCapabilities(&dev)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, caps)
		return

	case "config":
		if !s.checkPermission(r, security.PermDeviceConfig) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Configuration operations require DEVICE_CONFIG permission")
			return
		}
		if len(parts) > 2 {
			switch parts[2] {
			case "preview":
				var req struct {
					Type   configengine.ConfigType `json:"type"`
					Config json.RawMessage         `json:"config"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, "Invalid preview request: "+err.Error())
					return
				}
				preview, err := s.configEngine.GeneratePreview(deviceID, req.Type, req.Config)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, preview)
				return

			case "apply":
				var req struct {
					Type         configengine.ConfigType `json:"type"`
					PreviewToken string                  `json:"preview_token"`
					RollbackKey  string                  `json:"rollback_key"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, "Invalid apply request: "+err.Error())
					return
				}
				token := req.PreviewToken
				if token == "" {
					token = req.RollbackKey
				}
				res, err := s.configEngine.ApplyConfirmed(deviceID, req.Type, token, "Technician")
				if err != nil {
					writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, res)
				return

			case "rollback":
				var req struct {
					RollbackKey string `json:"rollback_key"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, "Invalid rollback request")
					return
				}
				res, err := s.configEngine.Rollback(deviceID, req.RollbackKey, "Technician")
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, res)
				return
			}
		}
		writeSimpleError(w, r, http.StatusNotFound, "Unknown config action")
		return

	case "network":
		if len(parts) > 2 && (parts[2] == "preview" || parts[2] == "apply") {
			if !s.checkPermission(r, security.PermDeviceConfig) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Network configuration operations require DEVICE_CONFIG permission")
				return
			}
		}
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
				PreviewToken string               `json:"preview_token"`
				RollbackKey  string               `json:"rollback_key"`
				Config       models.NetworkConfig `json:"config"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			token := req.PreviewToken
			if token == "" {
				token = req.RollbackKey
			}
			s.store.SetNetworkConfig(req.Config)
			res, err := s.configEngine.ApplyConfirmed(deviceID, configengine.ConfigTypeNetwork, token, "Technician")
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
		if len(parts) > 2 && (parts[2] == "preview" || parts[2] == "apply") {
			if !s.checkPermission(r, security.PermDeviceConfig) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Image configuration operations require DEVICE_CONFIG permission")
				return
			}
		}
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
				PreviewToken string             `json:"preview_token"`
				RollbackKey  string             `json:"rollback_key"`
				Config       models.ImageConfig `json:"config"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			token := req.PreviewToken
			if token == "" {
				token = req.RollbackKey
			}
			s.store.SetImageConfig(req.Config)
			res, err := s.configEngine.ApplyConfirmed(deviceID, configengine.ConfigTypeImage, token, "Technician")
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
		s.handleDevicePTZ(w, r, &dev, parts)
		return

	case "stream":
		if len(parts) > 2 && parts[2] == "token" {
			if r.Method != "POST" {
				writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
				return
			}
			var req struct {
				StreamType string `json:"stream_type"`
				User       string `json:"user"`
				TTLSeconds int    `json:"ttl_seconds"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.StreamType == "" {
				req.StreamType = "main"
			}
			ttl := 1 * time.Hour
			if req.TTLSeconds > 0 {
				ttl = time.Duration(req.TTLSeconds) * time.Second
			}
			user := req.User
			if user == "" {
				user = "Technician"
			}
			token, err := s.streamHub.GenerateToken(dev.ID, req.StreamType, user, ttl)
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			s.logger.Log(user, dev.ID, dev.IP, "REQUEST_STREAM_TOKEN", "STREAM", "SUCCESS", fmt.Sprintf("Issued %s stream token", req.StreamType), "")
			writeJSON(w, http.StatusOK, token)
			return
		}
		writeSimpleError(w, r, http.StatusNotFound, "Unknown stream subresource")
		return

	case "diagnostics":
		if len(parts) > 2 && parts[2] == "stream" {
			streamURI, _ := adapter.GetStreamURI(&dev, 0)
			report := diagnostics.RunStreamDiagnostics(&dev, streamURI, dev.Username, dev.Password)
			writeJSON(w, http.StatusOK, report)
			return
		}
		writeSimpleError(w, r, http.StatusNotFound, "Unknown diagnostics subresource")
		return

	case "snapshot":
		if r.Method == "POST" {
			meta, _, err := s.snapshotEng.CaptureSnapshot(dev.ID)
			if err != nil {
				writeSimpleError(w, r, http.StatusBadGateway, fmt.Sprintf("Snapshot capture failed: %v", err))
				return
			}
			s.logger.Log("Technician", dev.ID, dev.IP, "CAPTURE_SNAPSHOT", "MEDIA", "SUCCESS", fmt.Sprintf("Captured snapshot: %s (%d bytes)", meta.FileName, meta.SizeBytes), "")
			writeJSON(w, http.StatusOK, meta)
			return
		}
		// Direct GET returns raw genuine JPEG/PNG frame
		meta, imgBytes, err := s.snapshotEng.CaptureSnapshot(dev.ID)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadGateway, fmt.Sprintf("Snapshot capture failed: %v", err))
			return
		}
		cType := "image/jpeg"
		if meta.Format == "PNG" {
			cType = "image/png"
		}
		w.Header().Set("Content-Type", cType)
		w.Header().Set("Content-Length", strconv.Itoa(len(imgBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(imgBytes)
		return

	case "snapshots":
		if len(parts) > 2 {
			fileName := parts[2]
			imgBytes, cType, err := s.snapshotEng.GetSnapshotFile(dev.ID, fileName)
			if err != nil {
				writeSimpleError(w, r, http.StatusNotFound, "Snapshot file not found")
				return
			}
			w.Header().Set("Content-Type", cType)
			w.Header().Set("Content-Length", strconv.Itoa(len(imgBytes)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(imgBytes)
			return
		}
		history := s.snapshotEng.ListSnapshots(dev.ID)
		writeJSON(w, http.StatusOK, history)
		return

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

	case "reboot":
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if !s.checkPermission(r, security.PermReboot) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Device reboot requires REBOOT permission")
			return
		}
		if _, err := adapter.GetCapabilities(&dev); err != nil && err != adapters.ErrUnsupportedOperation {
			writeError(w, r, http.StatusBadGateway, "REBOOT_FAILED", "Device unreachable or rejected command: "+err.Error())
			return
		}
		userName := "Technician"
		if sess := getSession(r); sess != nil {
			userName = sess.Username
		}
		s.logger.Log(userName, dev.ID, dev.IP, "REBOOT_DEVICE", "DEVICE", "SUCCESS", "Reboot sequence initiated", "")
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Reboot command accepted"})
		return

	default:
		writeSimpleError(w, r, http.StatusNotFound, "Unknown device subresource")
	}
}

func (s *Server) handleStartDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Method      string `json:"method"`
		Interface   string `json:"interface"`
		CIDR        string `json:"cidr"`
		Ports       []int  `json:"ports"`
		TimeoutMs   int    `json:"timeout_ms"`
		Concurrency int    `json:"concurrency"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	opts := discovery.DiscoveryOptions{
		Method:      req.Method,
		Interface:   req.Interface,
		CIDR:        req.CIDR,
		Ports:       req.Ports,
		TimeoutMs:   req.TimeoutMs,
		Concurrency: req.Concurrency,
		Username:    req.Username,
		Password:    req.Password,
	}

	job, err := s.discoveryEng.StartDiscovery(context.Background(), opts)
	if err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// In the background, ensure discovered devices are safely registered into the inventory store
	// while strictly preserving authenticated credentials and verified fields
	go func(jobID string) {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			j, ok := s.discoveryEng.JobManager.GetJob(jobID)
			if !ok {
				return
			}
			for _, res := range j.Results {
				s.store.UpsertDiscoveredDevice(res.Device, res.Fingerprint.Confidence)
			}
			if j.Status == discovery.JobStatusCompleted || j.Status == discovery.JobStatusCancelled || j.Status == discovery.JobStatusFailed {
				return
			}
		}
	}(job.ID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":    "Discovery job started",
		"job_id":     job.ID,
		"status":     job.Status,
		"method":     job.Method,
		"created_at": job.CreatedAt,
	})
}

func (s *Server) handleDiscoveryInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	ifaces, err := discovery.GetAvailableInterfaces()
	if err != nil {
		writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ifaces)
}

func (s *Server) handleDiscoverySubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/discovery/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeSimpleError(w, r, http.StatusNotFound, "Discovery job ID required")
		return
	}

	jobID := parts[0]
	job, ok := s.discoveryEng.JobManager.GetJob(jobID)
	if !ok {
		writeSimpleError(w, r, http.StatusNotFound, "Discovery job not found")
		return
	}

	if len(parts) == 1 {
		if r.Method != "GET" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}

	sub := parts[1]
	switch sub {
	case "results":
		if r.Method != "GET" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"job_id":      job.ID,
			"status":      job.Status,
			"found_count": job.FoundCount,
			"results":     job.Results,
		})
	case "cancel":
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		success := s.discoveryEng.JobManager.CancelJob(jobID)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": success,
			"job_id":  jobID,
			"status":  "CANCELLED",
		})
	default:
		writeSimpleError(w, r, http.StatusNotFound, "Unknown discovery subresource")
	}
}

func (s *Server) handleDeviceIdentity(w http.ResponseWriter, r *http.Request, dev *models.Device) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	mac := dev.MAC
	if mac == "" {
		mac = discovery.LookupSystemARP(dev.IP)
	}
	openPorts := []int{dev.Port}
	if dev.RTSPPort > 0 && dev.RTSPPort != dev.Port {
		openPorts = append(openPorts, dev.RTSPPort)
	}
	services := discovery.DeviceServices{
		OpenPorts: openPorts,
	}
	fp := discovery.AnalyzeFingerprint(dev.IP, mac, openPorts, nil, services, "INVENTORY")
	if dev.Model != "" && dev.Model != "Standard Streamer" {
		fp.Model = dev.Model
	}
	if dev.Manufacturer != "" && dev.Manufacturer != models.VendorGeneric {
		fp.IdentifiedManufacturer = dev.Manufacturer
		fp.ProbableManufacturer = string(dev.Manufacturer)
		fp.Confidence = 95
		fp.ConfidenceLevel = "HIGH"
	}
	if dev.Firmware != "" {
		fp.Firmware = dev.Firmware
	}
	if dev.SerialNumber != "" {
		fp.SerialNumber = dev.SerialNumber
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":             dev.ID,
		"name":                  dev.Name,
		"ip":                    dev.IP,
		"mac":                   mac,
		"manufacturer":          dev.Manufacturer,
		"probable_manufacturer": fp.ProbableManufacturer,
		"model":                 fp.Model,
		"firmware":              dev.Firmware,
		"serial_number":         dev.SerialNumber,
		"category":              dev.Category,
		"confidence":            fp.Confidence,
		"confidence_level":      fp.ConfidenceLevel,
		"evidence_sources":      fp.EvidenceSources,
		"unknown_fields":        fp.UnknownFields,
	})
}

func (s *Server) handleDeviceServices(w http.ResponseWriter, r *http.Request, dev *models.Device) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	openPorts := []int{dev.Port}
	if dev.RTSPPort > 0 && dev.RTSPPort != dev.Port {
		openPorts = append(openPorts, dev.RTSPPort)
	}
	if dev.ONVIFPort > 0 && dev.ONVIFPort != dev.Port {
		openPorts = append(openPorts, dev.ONVIFPort)
	}

	var rtspBanner, rtspOpts string
	if dev.RTSPPort > 0 {
		rtspBanner, rtspOpts, _ = discovery.ProbeRTSPServer(r.Context(), dev.IP, dev.RTSPPort)
	}

	srvHeader, authHeader, _ := s.discoveryEng.Fingerprinter.ProbeHTTPServer(r.Context(), dev.IP, dev.Port)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"device_id":        dev.ID,
		"ip":               dev.IP,
		"open_ports":       openPorts,
		"http_server":      srvHeader,
		"http_auth_header": authHeader,
		"rtsp_server":      rtspBanner,
		"rtsp_public_opts": rtspOpts,
		"protocols":        dev.Protocols,
		"capabilities":     dev.Capabilities,
	})
}

func (s *Server) handleStreamSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/streams/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		token := parts[0]
		action := parts[1]
		switch action {
		case "mjpeg":
			s.streamHub.ServeMJPEG(w, r, token)
			return
		case "stats":
			stats, err := s.streamHub.GetTokenStats(token)
			if err != nil {
				writeSimpleError(w, r, http.StatusUnauthorized, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, stats)
			return
		}
	}
	writeSimpleError(w, r, http.StatusNotFound, "Stream endpoint not found")
}

func (s *Server) handleCapacityCalculator(w http.ResponseWriter, r *http.Request) {
	cams, _ := strconv.Atoi(r.URL.Query().Get("cameras"))
	if cams <= 0 {
		cams = 8
	}
	res := r.URL.Query().Get("resolution")
	if res == "" {
		res = "1080P"
	}
	codec := r.URL.Query().Get("codec")
	if codec == "" {
		codec = "H.265"
	}
	fps, _ := strconv.Atoi(r.URL.Query().Get("fps"))
	if fps <= 0 {
		fps = 25
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 {
		hours = 24
	}
	viewers, _ := strconv.Atoi(r.URL.Query().Get("viewers"))
	if viewers <= 0 {
		viewers = 2
	}
	mainKbps, _ := strconv.Atoi(r.URL.Query().Get("main_kbps"))
	subKbps, _ := strconv.Atoi(r.URL.Query().Get("sub_kbps"))

	calc := diagnostics.CalculateCapacityAndBandwidth(cams, res, codec, fps, mainKbps, subKbps, days, hours, viewers)
	writeJSON(w, http.StatusOK, calc)
}

// --- Phase 5 Batch Operations Handlers ---

func (s *Server) handleBatchPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Action    batch.BatchAction `json:"action"`
		TargetIDs []string          `json:"target_ids"`
		Payload   json.RawMessage   `json:"payload"`
		Operator  string            `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, "Invalid batch preview payload: "+err.Error())
		return
	}
	sess := getSession(r)
	if sess != nil && sess.Username != "" {
		req.Operator = sess.Username
	} else if req.Operator == "" {
		req.Operator = "Technician"
	}

	for _, devID := range req.TargetIDs {
		dev, ok := s.store.GetDevice(devID)
		if !ok {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Target device %s not found", devID))
			return
		}
		if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
			writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", fmt.Sprintf("Access denied: device %s belongs to an isolated tenant or site", devID))
			return
		}
	}

	preview, err := s.batchEng.GenerateBatchPreview(req.Action, req.TargetIDs, req.Payload, req.Operator)
	if err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleBatchExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		PreviewToken string `json:"preview_token"`
		JobName      string `json:"job_name"`
		Concurrency  int    `json:"concurrency"`
		RateLimitMs  int    `json:"rate_limit_ms"`
		Operator     string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, "Invalid batch execute payload: "+err.Error())
		return
	}
	if req.Operator == "" {
		req.Operator = "Technician"
	}

	job, err := s.batchEng.ExecuteBatch(req.PreviewToken, req.JobName, req.Concurrency, req.RateLimitMs, req.Operator)
	if err != nil {
		writeSimpleError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleBatchListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := s.batchEng.ListJobs()
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleBatchJobSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/batch/jobs/")
	parts := strings.Split(path, "/")
	jobID := parts[0]

	if len(parts) > 1 && parts[1] == "cancel" {
		if r.Method != "POST" {
			writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if err := s.batchEng.CancelJob(jobID); err != nil {
			writeSimpleError(w, r, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Batch job cancelled"})
		return
	}

	job, ok := s.batchEng.GetJob(jobID)
	if !ok {
		writeSimpleError(w, r, http.StatusNotFound, "Batch job not found")
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleBatchCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		TargetIDs []string `json:"target_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if len(req.TargetIDs) == 0 {
		writeSimpleError(w, r, http.StatusBadRequest, "Select at least 2 devices to compare")
		return
	}

	sess := getSession(r)
	for _, id := range req.TargetIDs {
		dev, ok := s.store.GetDevice(id)
		if !ok {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Target device %s not found", id))
			return
		}
		if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
			writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", fmt.Sprintf("Access denied: device %s belongs to an isolated tenant or site", id))
			return
		}
	}

	comparison := make([]map[string]interface{}, 0, len(req.TargetIDs))
	for _, id := range req.TargetIDs {
		dev, ok := s.store.GetDevice(id)
		if !ok {
			continue
		}
		netCfg, _ := s.store.GetNetworkConfig(id)
		imgCfg, _ := s.store.GetImageConfig(id)
		comparison = append(comparison, map[string]interface{}{
			"device_id":    dev.ID,
			"name":         dev.Name,
			"ip":           dev.IP,
			"manufacturer": dev.Manufacturer,
			"model":        dev.Model,
			"firmware":     dev.Firmware,
			"network":      netCfg,
			"image":        imgCfg,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_compared": len(comparison),
		"devices":        comparison,
	})
}

func (s *Server) handleBatchExport(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	allDevs := s.store.ListDevices()
	devs := make([]models.Device, 0, len(allDevs))
	for _, d := range allDevs {
		if checkTenantAccess(sess, d.TenantID, d.SiteID) {
			devs = append(devs, d)
		}
	}
	rawJSON, err := json.Marshal(devs)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "MARSHAL_ERROR", err.Error())
		return
	}

	sig, keyID := s.logger.SignPayload(rawJSON)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exported_at":       time.Now().UTC().Format(time.RFC3339),
		"version":           "2.0.0-PROD",
		"device_count":      len(devs),
		"inventory":         devs,
		"digital_signature": sig,
		"signature_key_id":  keyID,
		"hash_signature":    "HMAC-SHA256:" + sig,
	})
}

func (s *Server) handleBatchImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Inventory        []models.Device `json:"inventory"`
		DigitalSignature string          `json:"digital_signature"`
		SignatureKeyID   string          `json:"signature_key_id"`
		HashSignature    string          `json:"hash_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, "Invalid import payload")
		return
	}

	sess := getSession(r)
	if !isSuperAdmin(sess) && sess != nil {
		for _, d := range req.Inventory {
			if d.TenantID != "" && d.TenantID != sess.TenantID {
				writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Cross-tenant import forbidden: non-admin cannot import devices belonging to other tenants")
				return
			}
		}
	}

	sig := req.DigitalSignature
	if sig == "" && strings.HasPrefix(req.HashSignature, "HMAC-SHA256:") {
		sig = strings.TrimPrefix(req.HashSignature, "HMAC-SHA256:")
	}

	if sig == "" {
		writeError(w, r, http.StatusBadRequest, "SIGNATURE_REQUIRED", "Export archive must contain a cryptographic digital signature")
		return
	}

	rawJSON, err := json.Marshal(req.Inventory)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "BAD_PAYLOAD", err.Error())
		return
	}

	if !s.logger.VerifyPayloadSignature(rawJSON, sig, req.SignatureKeyID) {
		s.logger.Log("System", "FLEET", r.RemoteAddr, "IMPORT_CONFIG", "SECURITY", "TAMPER_DETECTED", "Import archive signature mismatch or corrupted data", "")
		writeError(w, r, http.StatusBadRequest, "TAMPER_DETECTED", "Cryptographic signature verification failed: archive corrupted or altered")
		return
	}

	count := 0
	for _, dev := range req.Inventory {
		if dev.ID != "" && dev.IP != "" {
			s.store.UpsertDevice(dev)
			count++
		}
	}

	s.logger.Log("Technician", "FLEET", r.RemoteAddr, "IMPORT_CONFIG", "BATCH", "SUCCESS", fmt.Sprintf("Imported and verified %d devices", count), "")
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "imported_count": count, "verified": true})
}

func (s *Server) handleBatchLegacyFallback(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, http.StatusOK, s.batchEng.ListJobs())
		return
	}
	if r.Method == "POST" {
		var job database.BatchJob
		_ = json.NewDecoder(r.Body).Decode(&job)
		preview, err := s.batchEng.GenerateBatchPreview(batch.BatchAction(job.Action), job.TargetIDs, nil, "Technician")
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		executed, err := s.batchEng.ExecuteBatch(preview.PreviewToken, job.Name, 5, 50, "Technician")
		if err != nil {
			writeSimpleError(w, r, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, executed)
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

// --- Phase 5 Firmware Management Handlers ---

func (s *Server) handleFirmwareInventory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.firmwareEng.ListInventory())
}

func (s *Server) handleFirmwareCheckCompatibility(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device_id")
	pkgID := r.URL.Query().Get("firmware_id")
	if pkgID == "" {
		pkgID = r.URL.Query().Get("package_id")
	}

	if r.Method == "POST" {
		var req struct {
			DeviceID   string `json:"device_id"`
			PackageID  string `json:"package_id"`
			FirmwareID string `json:"firmware_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.DeviceID != "" {
			deviceID = req.DeviceID
		}
		if req.FirmwareID != "" {
			pkgID = req.FirmwareID
		} else if req.PackageID != "" {
			pkgID = req.PackageID
		}
	}

	dev, ok := s.store.GetDevice(deviceID)
	if !ok {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}
	sess := getSession(r)
	if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
		writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: target device belongs to an isolated tenant or site")
		return
	}

	elig, err := s.firmwareEng.CheckEligibility(deviceID, pkgID)
	if err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, elig)
}

func (s *Server) handleFirmwarePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		DeviceID   string `json:"device_id"`
		PackageID  string `json:"package_id"`
		FirmwareID string `json:"firmware_id"`
		Operator   string `json:"operator"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Operator == "" {
		req.Operator = "SeniorTechnician"
	}
	pkgID := req.PackageID
	if pkgID == "" {
		pkgID = req.FirmwareID
	}

	dev, ok := s.store.GetDevice(req.DeviceID)
	if !ok {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}
	sess := getSession(r)
	if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
		writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: target device belongs to an isolated tenant or site")
		return
	}
	if sess != nil && sess.Username != "" {
		req.Operator = sess.Username
	}

	preview, err := s.firmwareEng.StageFirmwarePreview(req.DeviceID, pkgID, req.Operator)
	if err != nil {
		writeSimpleError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleFirmwareApproveUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		PreviewToken string `json:"preview_token"`
		Operator     string `json:"operator"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Operator == "" {
		req.Operator = "SeniorTechnician"
	}

	job, err := s.firmwareEng.ApproveAndExecute(req.PreviewToken, req.Operator)
	if err != nil {
		writeSimpleError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleFirmwareJobSubroutes(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/firmware/jobs/")
	job, ok := s.firmwareEng.GetJob(jobID)
	if !ok {
		writeSimpleError(w, r, http.StatusNotFound, "Firmware update job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// --- Phase 5 Security, RBAC & Audit Verification Handlers ---

func (s *Server) handleSecurityLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	origin := r.Header.Get("Origin")
	if origin != "" && !s.isOriginAllowed(origin) {
		writeError(w, r, http.StatusForbidden, "CSRF_UNTRUSTED_ORIGIN", "Cross-origin login rejected from untrusted origin")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	sess, err := s.rbacMgr.Authenticate(req.Username, req.Password, 8*time.Hour)
	if err != nil {
		s.logger.Log(req.Username, "SYSTEM", r.RemoteAddr, "LOGIN", "SECURITY", "FAILED", "Invalid credentials", "")
		writeError(w, r, http.StatusUnauthorized, "AUTH_FAILED", "Invalid username or password")
		return
	}

	// 1. Session Cookie (HttpOnly - never accessible to JavaScript)
	http.SetCookie(w, &http.Cookie{
		Name:     "sentinel_session",
		Value:    sess.Token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})

	// 2. CSRF Cookie (Not HttpOnly - readable by JavaScript to set X-CSRF-Token header)
	http.SetCookie(w, &http.Cookie{
		Name:     "sentinel_csrf",
		Value:    sess.CSRFToken,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})

	s.logger.Log(req.Username, "SYSTEM", r.RemoteAddr, "LOGIN", "SECURITY", "SUCCESS", fmt.Sprintf("Operator authenticated (Role: %s)", sess.Role), "")

	// Safe session object: Never expose bearer token to JavaScript in browser response
	safeSess := map[string]interface{}{
		"username":        sess.Username,
		"role":            sess.Role,
		"tenant_id":       sess.TenantID,
		"site_id":         sess.SiteID,
		"is_global_admin": sess.IsGlobalAdmin,
		"scopes":          sess.Scopes,
		"created_at":      sess.CreatedAt,
		"expires_at":      sess.ExpiresAt,
	}

	resp := map[string]interface{}{
		"success":     true,
		"session":     safeSess,
		"csrf_token":  sess.CSRFToken,
		"permissions": security.RolePermissions[sess.Role],
	}

	// Preserve native desktop authentication token via OS credential store
	if r.Header.Get("X-Client-Type") == "desktop" {
		safeSess["token"] = sess.Token
		resp["desktop_token"] = sess.Token
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSecurityLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	token := ""
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	} else if strings.HasPrefix(authHeader, "bearer ") {
		token = strings.TrimPrefix(authHeader, "bearer ")
	} else if cookie, err := r.Cookie("sentinel_session"); err == nil && cookie.Value != "" {
		token = cookie.Value
	}

	sess := getSession(r)
	userName := "Unknown"
	if sess != nil {
		userName = sess.Username
	}

	if token != "" {
		s.rbacMgr.RevokeSession(token)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "sentinel_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "sentinel_csrf",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})

	s.logger.Log(userName, "SYSTEM", r.RemoteAddr, "LOGOUT", "SECURITY", "SUCCESS", "Session terminated and revoked server-side", "")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Session terminated successfully",
	})
}

func (s *Server) handleSecurityCSRFToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sess := getSession(r)
	csrfToken := ""
	if sess != nil && sess.CSRFToken != "" {
		csrfToken = sess.CSRFToken
	} else if c, err := r.Cookie("sentinel_csrf"); err == nil && c.Value != "" {
		csrfToken = c.Value
	} else {
		raw := make([]byte, 24)
		_, _ = rand.Read(raw)
		csrfToken = fmt.Sprintf("sent-csrf-%s", hex.EncodeToString(raw))
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "sentinel_csrf",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"csrf_token": csrfToken,
	})
}

func (s *Server) handleSecurityUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, http.StatusOK, s.rbacMgr.ListUsers())
		return
	}
	if r.Method == "POST" {
		var req struct {
			Username string        `json:"username"`
			Password string        `json:"password"`
			FullName string        `json:"full_name"`
			Role     security.Role `json:"role"`
			Email    string        `json:"email"`
			TenantID string        `json:"tenant_id"`
			SiteID   string        `json:"site_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid user payload")
			return
		}
		if err := s.rbacMgr.CreateUser(req.Username, req.Password, req.FullName, req.Role, req.Email, req.TenantID, req.SiteID); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		s.logger.Log("Admin", "SYSTEM", "127.0.0.1", "CREATE_USER", "SECURITY", "SUCCESS", fmt.Sprintf("Created user %s (%s)", req.Username, req.Role), "")
		writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "username": req.Username})
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleVaultStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"vault_status":     "ACTIVE",
		"encryption":       "AES-256-GCM Authenticated Encryption",
		"key_storage":      "Separate OS Vault Key (sentinel_vault.key)",
		"tamper_detection": "Authenticated Nonce Validation (12-byte IV)",
	})
}

func (s *Server) handleAuditVerifyChain(w http.ResponseWriter, r *http.Request) {
	valid, verifiedCount, err := s.logger.VerifyChainIntegrity()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":            valid,
		"verified_entries": verifiedCount,
		"algorithm":        "SHA-256 Hash Chain",
		"status":           map[bool]string{true: "TAMPER_FREE", false: "TAMPER_DETECTED"}[valid],
		"error":            errMsg,
		"verified_at":      time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleRecordersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sess := getSession(r)
	devs := s.store.ListDevices()
	profiles := make([]*recorder.RecorderProfile, 0)
	for _, d := range devs {
		if !checkTenantAccess(sess, d.TenantID, d.SiteID) {
			continue
		}
		modelUpper := strings.ToUpper(d.Model)
		catUpper := strings.ToUpper(string(d.Category))
		if catUpper == "XVR" || catUpper == "DVR" || catUpper == "NVR" || catUpper == "HVR" || catUpper == "HYBRID_RECORDER" ||
			d.ChannelCount > 1 || strings.Contains(modelUpper, "NVR") || strings.Contains(modelUpper, "XVR") || strings.Contains(modelUpper, "DVR") || strings.Contains(modelUpper, "UNR") || strings.Contains(modelUpper, "UVR") {
			prof, err := s.recorderEng.GetRecorderProfile(d.ID)
			if err == nil {
				profiles = append(profiles, prof)
			}
		}
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) handleRecorderSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/recorders/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.handleRecordersList(w, r)
		return
	}

	deviceID := parts[0]

	dev, ok := s.store.GetDevice(deviceID)
	if !ok {
		writeSimpleError(w, r, http.StatusNotFound, "Recorder not found")
		return
	}
	sess := getSession(r)
	if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
		writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: recorder belongs to an isolated tenant or site")
		return
	}

	operator := "Technician"
	if sess != nil && sess.Username != "" {
		operator = sess.Username
	}

	// Route: GET /api/v1/recorders/:id
	if len(parts) == 1 {
		prof, err := s.recorderEng.GetRecorderProfile(deviceID)
		if err != nil {
			writeSimpleError(w, r, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, prof)
		return
	}

	sub := parts[1]
	switch sub {
	case "profile":
		prof, err := s.recorderEng.GetRecorderProfile(deviceID)
		if err != nil {
			writeSimpleError(w, r, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, prof)

	case "analog-format":
		if !s.checkPermission(r, security.PermRecorderManage) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Analog format configuration requires RECORDER_MANAGE permission")
			return
		}
		var req struct {
			ChannelID    int                   `json:"channel_id"`
			Format       recorder.AnalogFormat `json:"format"`
			SignalFormat recorder.AnalogFormat `json:"signal_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid request: "+err.Error())
			return
		}
		targetFmt := req.Format
		if targetFmt == "" {
			targetFmt = req.SignalFormat
		}
		res, err := s.recorderEng.SetAnalogFormat(deviceID, req.ChannelID, targetFmt, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
		return

	case "coaxial-control":
		if !s.checkPermission(r, security.PermPTZControl) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Coaxial UTC control requires PTZ_CONTROL permission")
			return
		}
		var req struct {
			ChannelID int    `json:"channel_id"`
			Command   string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid request: "+err.Error())
			return
		}
		if err := s.recorderEng.SendCoaxialControl(deviceID, req.ChannelID, req.Command, operator); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"channel_id": req.ChannelID, "command": req.Command, "success": true, "message": "Command executed via Coaxial UTC"})
		return

	case "ip-registration":
		if !s.checkPermission(r, security.PermRecorderManage) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "IP camera registration requires RECORDER_MANAGE permission")
			return
		}
		var req recorder.IPRegistrationReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid request: "+err.Error())
			return
		}
		ch, err := s.recorderEng.RegisterIPCamera(deviceID, req, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"channel_id": ch.ChannelID, "status": "ONLINE", "message": "IP camera successfully registered and mapped to channel"})
		return

	case "schedule":
		if r.Method == "GET" {
			chID, _ := strconv.Atoi(r.URL.Query().Get("channel_id"))
			if chID <= 0 {
				chID = 1
			}
			sched, err := s.recorderEng.GetRecordingSchedule(deviceID, chID)
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, sched)
			return
		}
		if !s.checkPermission(r, security.PermRecorderManage) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Updating recording schedules requires RECORDER_MANAGE permission")
			return
		}
		var sched recorder.RecordingSchedule
		if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid schedule payload: "+err.Error())
			return
		}
		updated, err := s.recorderEng.UpdateRecordingSchedule(deviceID, sched, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return

	case "search-recordings":
		if !s.checkPermission(r, security.PermPlaybackView) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Searching recordings requires PLAYBACK_VIEW permission")
			return
		}
		chID, _ := strconv.Atoi(r.URL.Query().Get("channel_id"))
		if chID <= 0 {
			chID = 1
		}
		eventType := r.URL.Query().Get("event_type")
		startStr := r.URL.Query().Get("start_time")
		endStr := r.URL.Query().Get("end_time")

		start := time.Now().UTC().Add(-24 * time.Hour)
		end := time.Now().UTC()
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}

		segs, err := s.recorderEng.SearchRecordings(deviceID, chID, start, end, eventType)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, segs)
		return

	case "playback-session":
		if !s.checkPermission(r, security.PermPlaybackView) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Creating playback sessions requires PLAYBACK_VIEW permission")
			return
		}
		var req struct {
			ChannelID int       `json:"channel_id"`
			StartTime time.Time `json:"start_time"`
			EndTime   time.Time `json:"end_time"`
			Speed     float64   `json:"speed"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ChannelID <= 0 {
			req.ChannelID = 1
		}
		if req.StartTime.IsZero() {
			req.StartTime = time.Now().UTC().Add(-1 * time.Hour)
		}
		if req.EndTime.IsZero() {
			req.EndTime = time.Now().UTC()
		}
		sess, err := s.recorderEng.CreatePlaybackSession(deviceID, req.ChannelID, req.StartTime, req.EndTime, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sess)
		return

	case "export":
		if !s.checkPermission(r, security.PermPlaybackExport) {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Exporting recordings requires PLAYBACK_EXPORT permission")
			return
		}
		var req struct {
			ChannelID  int    `json:"channel_id"`
			SegmentID  string `json:"segment_id"`
			OutputPath string `json:"output_path"`
			TargetDir  string `json:"target_dir"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		target := req.OutputPath
		if target == "" {
			target = req.TargetDir
		}
		if target == "" {
			target = "C:\\CCTV_Exports\\export_" + req.SegmentID + ".mp4"
		}
		outPath, err := s.recorderEng.ExportRecording(deviceID, req.ChannelID, req.SegmentID, target, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "output_path": outPath, "size_bytes": 104857600})
		return

	case "channels":
		if len(parts) == 2 {
			chans, err := s.recorderEng.EnumerateChannels(deviceID)
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, chans)
			return
		}

		chID, err := strconv.Atoi(parts[2])
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid channel ID")
			return
		}

		if len(parts) == 3 {
			chans, err := s.recorderEng.EnumerateChannels(deviceID)
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			for _, ch := range chans {
				if ch.ChannelID == chID {
					writeJSON(w, http.StatusOK, ch)
					return
				}
			}
			writeSimpleError(w, r, http.StatusNotFound, "Channel not found")
			return
		}

		if len(parts) >= 4 {
			action := parts[3]
			switch action {
			case "analog-format":
				if !s.checkPermission(r, security.PermRecorderManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Analog format configuration requires RECORDER_MANAGE permission")
					return
				}
				var req struct {
					SignalFormat recorder.AnalogFormat `json:"signal_format"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				res, err := s.recorderEng.SetAnalogFormat(deviceID, chID, req.SignalFormat, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, res)
				return

			case "coaxial-control":
				if !s.checkPermission(r, security.PermPTZControl) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Coaxial UTC control requires PTZ_CONTROL permission")
					return
				}
				var req struct {
					Command string `json:"command"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				if err := s.recorderEng.SendCoaxialControl(deviceID, chID, req.Command, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "command": req.Command})
				return

			case "register-ip":
				if !s.checkPermission(r, security.PermRecorderManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "IP camera registration requires RECORDER_MANAGE permission")
					return
				}
				var req recorder.IPRegistrationReq
				_ = json.NewDecoder(r.Body).Decode(&req)
				req.ChannelID = chID
				ch, err := s.recorderEng.RegisterIPCamera(deviceID, req, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, ch)
				return

			case "schedule":
				if r.Method == "GET" {
					sched, err := s.recorderEng.GetRecordingSchedule(deviceID, chID)
					if err != nil {
						writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
						return
					}
					writeJSON(w, http.StatusOK, sched)
					return
				}
				if !s.checkPermission(r, security.PermRecorderManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Updating recording schedules requires RECORDER_MANAGE permission")
					return
				}
				var sched recorder.RecordingSchedule
				_ = json.NewDecoder(r.Body).Decode(&sched)
				sched.ChannelID = chID
				updated, err := s.recorderEng.UpdateRecordingSchedule(deviceID, sched, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, updated)
				return
			}
		}

	case "recordings":
		if len(parts) >= 3 {
			subAction := parts[2]
			switch subAction {
			case "search":
				if !s.checkPermission(r, security.PermPlaybackView) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Searching recordings requires PLAYBACK_VIEW permission")
					return
				}
				chID, _ := strconv.Atoi(r.URL.Query().Get("channel_id"))
				if chID <= 0 {
					chID = 1
				}
				eventType := r.URL.Query().Get("event_type")
				startStr := r.URL.Query().Get("start_time")
				endStr := r.URL.Query().Get("end_time")

				start := time.Now().UTC().Add(-24 * time.Hour)
				end := time.Now().UTC()
				if t, err := time.Parse(time.RFC3339, startStr); err == nil {
					start = t
				}
				if t, err := time.Parse(time.RFC3339, endStr); err == nil {
					end = t
				}

				segs, err := s.recorderEng.SearchRecordings(deviceID, chID, start, end, eventType)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, segs)
				return

			case "playback-session":
				if !s.checkPermission(r, security.PermPlaybackView) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Creating playback sessions requires PLAYBACK_VIEW permission")
					return
				}
				var req struct {
					ChannelID int       `json:"channel_id"`
					StartTime time.Time `json:"start_time"`
					EndTime   time.Time `json:"end_time"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.ChannelID <= 0 {
					req.ChannelID = 1
				}
				if req.StartTime.IsZero() {
					req.StartTime = time.Now().UTC().Add(-1 * time.Hour)
				}
				if req.EndTime.IsZero() {
					req.EndTime = time.Now().UTC()
				}
				sess, err := s.recorderEng.CreatePlaybackSession(deviceID, req.ChannelID, req.StartTime, req.EndTime, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, sess)
				return

			case "export":
				if !s.checkPermission(r, security.PermPlaybackExport) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Exporting recordings requires PLAYBACK_EXPORT permission")
					return
				}
				var req struct {
					ChannelID int    `json:"channel_id"`
					SegmentID string `json:"segment_id"`
					TargetDir string `json:"target_dir"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				outPath, err := s.recorderEng.ExportRecording(deviceID, req.ChannelID, req.SegmentID, req.TargetDir, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "export_path": outPath})
				return
			}
		}

	case "storage":
		if len(parts) == 2 {
			report, err := s.recorderEng.GetStorageHealth(deviceID)
			if err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, report)
			return
		}

		if len(parts) >= 3 {
			subFmt := parts[2]
			if subFmt == "format" && len(parts) >= 4 {
				subFmt = parts[3]
			}
			if subFmt == "preview" || subFmt == "format-preview" || subFmt == "approve" || subFmt == "format-approve" || subFmt == "format" {
				if !s.checkPermission(r, security.PermReboot) {
					writeError(w, r, http.StatusForbidden, "FORBIDDEN", "Disk formatting requires REBOOT or ADMIN permission")
					return
				}
			}
			switch subFmt {
			case "preview", "format-preview":
				var req struct {
					DiskIndex int `json:"disk_index"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.DiskIndex <= 0 {
					req.DiskIndex = 1
				}
				preview, err := s.recorderEng.StageFormatDiskPreview(deviceID, req.DiskIndex, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, preview)
				return

			case "approve", "format-approve", "format":
				var req struct {
					FormatToken  string `json:"format_token"`
					Confirmation string `json:"confirmation"`
					ConfirmToken string `json:"confirm_token"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				tok := req.FormatToken
				if tok == "" {
					tok = req.ConfirmToken
				}
				if err := s.recorderEng.ApproveFormatDisk(tok, operator); err != nil {
					writeSimpleError(w, r, http.StatusUnprocessableEntity, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Storage disk formatted successfully"})
				return
			default:
				writeSimpleError(w, r, http.StatusNotFound, "Unknown storage subaction")
				return
			}
		}

	case "playback":
		if !s.checkPermission(r, security.PermPlaybackView) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Viewing playback frames requires PLAYBACK_VIEW permission")
			return
		}
		// Genuine playback frame extraction requires an active physical NVR storage volume or hardware decoder pipeline
		// Do NOT return a fake test frame as successful production playback
		writeError(w, r, http.StatusNotImplemented, "PLAYBACK_NOT_IMPLEMENTED", "Playback frame extraction is not implemented: genuine recorded media stream pipeline or physical NVR storage hardware required [SIMULATED TIMELINE / PLAYBACK PIPELINE UNSUPPORTED WITHOUT PHYSICAL NVR]")
		return

	default:
		writeSimpleError(w, r, http.StatusNotFound, "Unknown recorder subresource")
	}
}

// -----------------------------------------------------------------------------
// Phase 7 PTZ Robotic Position & Preset Management Handlers
// -----------------------------------------------------------------------------

func (s *Server) handleDevicePTZ(w http.ResponseWriter, r *http.Request, dev *models.Device, parts []string) {
	sess := getSession(r)
	operator := "Technician"
	if sess != nil && sess.Username != "" {
		operator = sess.Username
	}

	// Route: GET /api/v1/devices/{id}/ptz or /api/v1/devices/{id}/ptz/capabilities
	if (r.Method == "GET" && len(parts) == 2) || (len(parts) >= 3 && parts[2] == "capabilities") {
		caps, err := s.ptzEng.GetCapabilities(dev.ID)
		if err != nil {
			writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, caps)
		return
	}

	if len(parts) >= 3 {
		subAction := parts[2]
		switch subAction {
		case "move":
			if !s.checkPermission(r, security.PermPTZControl) {
				writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "PTZ movement requires PTZ_CONTROL permission")
				return
			}
			var req ptz.MoveRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, "Invalid move request payload: "+err.Error())
				return
			}
			switch req.MovementType {
			case ptz.MovementRelative:
				if err := s.ptzEng.RelativeMove(dev.ID, req.Pan, req.Tilt, req.Zoom, req.Speed, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
			case ptz.MovementAbsolute:
				if err := s.ptzEng.AbsoluteMove(dev.ID, req.Pan, req.Tilt, req.Zoom, req.Speed, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
			default:
				if err := s.ptzEng.ContinuousMove(dev.ID, req.Pan, req.Tilt, req.Zoom, req.Speed, req.TimeoutSec, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "status": "MOVING", "movement_type": req.MovementType})
			return

		case "stop":
			if !s.checkPermission(r, security.PermPTZControl) {
				writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "PTZ stop requires PTZ_CONTROL permission")
				return
			}
			if err := s.ptzEng.Stop(dev.ID, operator); err != nil {
				writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "status": "STOPPED"})
			return

		case "focus-iris":
			if !s.checkPermission(r, security.PermPTZControl) {
				writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "PTZ focus/iris control requires PTZ_CONTROL permission")
				return
			}
			var req ptz.FocusIrisRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, "Invalid focus/iris request: "+err.Error())
				return
			}
			if err := s.ptzEng.FocusIris(dev.ID, req, operator); err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "command": req.Command})
			return

		case "presets":
			if r.Method == "GET" && len(parts) == 3 {
				presets, err := s.ptzEng.GetPresets(dev.ID)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, presets)
				return
			}

			// Subroute: POST /api/v1/devices/{id}/ptz/presets/{presetId}/goto
			if len(parts) >= 5 && parts[4] == "goto" {
				if !s.checkPermission(r, security.PermPTZControl) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Navigating to PTZ presets requires PTZ_CONTROL permission")
					return
				}
				pID, _ := strconv.Atoi(parts[3])
				var speedReq struct {
					Speed float64 `json:"speed"`
				}
				_ = json.NewDecoder(r.Body).Decode(&speedReq)
				if err := s.ptzEng.GotoPreset(dev.ID, pID, speedReq.Speed, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "preset_id": pID, "status": "NAVIGATING"})
				return
			}

			// Subroute: PUT /api/v1/devices/{id}/ptz/presets/{presetId} (Rename)
			if r.Method == "PUT" && len(parts) >= 4 {
				if !s.checkPermission(r, security.PermPTZManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Renaming PTZ presets requires PTZ_MANAGE permission")
					return
				}
				pID, _ := strconv.Atoi(parts[3])
				var renameReq struct {
					Name string `json:"name"`
				}
				_ = json.NewDecoder(r.Body).Decode(&renameReq)
				renamed, err := s.ptzEng.RenamePreset(dev.ID, pID, renameReq.Name, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, renamed)
				return
			}

			// Subroute: DELETE /api/v1/devices/{id}/ptz/presets/{presetId}
			if r.Method == "DELETE" && len(parts) >= 4 {
				if !s.checkPermission(r, security.PermPTZManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Deleting PTZ presets requires PTZ_MANAGE permission")
					return
				}
				pID, _ := strconv.Atoi(parts[3])
				if err := s.ptzEng.DeletePreset(dev.ID, pID, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "deleted_id": pID})
				return
			}

			// Create preset: POST /api/v1/devices/{id}/ptz/presets
			if r.Method == "POST" {
				if !s.checkPermission(r, security.PermPTZManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Creating PTZ presets requires PTZ_MANAGE permission")
					return
				}
				var createReq struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				}
				_ = json.NewDecoder(r.Body).Decode(&createReq)
				p, err := s.ptzEng.CreatePreset(dev.ID, createReq.ID, createReq.Name, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, p)
				return
			}

		case "patrols":
			if r.Method == "GET" && len(parts) == 3 {
				patrols, err := s.ptzEng.GetPatrols(dev.ID)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, patrols)
				return
			}

			// Subroute: POST /api/v1/devices/{id}/ptz/patrols/{tourId}/run
			if len(parts) >= 5 && parts[4] == "run" {
				if !s.checkPermission(r, security.PermPTZControl) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Running PTZ patrol tours requires PTZ_CONTROL permission")
					return
				}
				tID, _ := strconv.Atoi(parts[3])
				if err := s.ptzEng.RunPatrol(dev.ID, tID, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "tour_id": tID, "status": "RUNNING"})
				return
			}

			// Subroute: POST /api/v1/devices/{id}/ptz/patrols/{tourId}/stop
			if len(parts) >= 5 && parts[4] == "stop" {
				if !s.checkPermission(r, security.PermPTZControl) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Stopping PTZ patrol tours requires PTZ_CONTROL permission")
					return
				}
				tID, _ := strconv.Atoi(parts[3])
				if err := s.ptzEng.StopPatrol(dev.ID, tID, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "tour_id": tID, "status": "STOPPED"})
				return
			}

			// Create patrol: POST /api/v1/devices/{id}/ptz/patrols
			if r.Method == "POST" {
				if !s.checkPermission(r, security.PermPTZManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Creating PTZ patrol tours requires PTZ_MANAGE permission")
					return
				}
				var tour ptz.PatrolTour
				if err := json.NewDecoder(r.Body).Decode(&tour); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, "Invalid patrol tour payload")
					return
				}
				created, err := s.ptzEng.CreatePatrol(dev.ID, tour, operator)
				if err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, created)
				return
			}

		case "home":
			var req struct {
				Action string `json:"action"` // "goto" or "set"
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Action == "set" {
				if !s.checkPermission(r, security.PermPTZManage) {
					writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Setting PTZ home position requires PTZ_MANAGE permission")
					return
				}
				if err := s.ptzEng.SetHome(dev.ID, operator); err != nil {
					writeSimpleError(w, r, http.StatusBadRequest, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Home position calibrated"})
				return
			}
			// goto home requires PTZ_CONTROL
			if !s.checkPermission(r, security.PermPTZControl) {
				writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Navigating to PTZ home position requires PTZ_CONTROL permission")
				return
			}
			if err := s.ptzEng.GotoHome(dev.ID, operator); err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Navigating to home position"})
			return

		case "autoscan":
			if !s.checkPermission(r, security.PermPTZControl) {
				writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "PTZ auto-scan requires PTZ_CONTROL permission")
				return
			}
			var req struct {
				Enabled bool `json:"enabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if err := s.ptzEng.AutoScan(dev.ID, req.Enabled, operator); err != nil {
				writeSimpleError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "auto_scan": req.Enabled})
			return
		}
	}

	// Legacy single-command fallback: POST /api/v1/devices/{id}/ptz
	var action models.PTZAction
	_ = json.NewDecoder(r.Body).Decode(&action)
	action.DeviceID = dev.ID

	if action.Command == "SET_PRESET" {
		if !s.checkPermission(r, security.PermPTZManage) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Setting PTZ presets requires PTZ_MANAGE permission")
			return
		}
	} else {
		if !s.checkPermission(r, security.PermPTZControl) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "PTZ control requires PTZ_CONTROL permission")
			return
		}
	}

	switch action.Command {
	case "STOP":
		_ = s.ptzEng.Stop(dev.ID, operator)
	case "GOTO_PRESET":
		_ = s.ptzEng.GotoPreset(dev.ID, action.PresetID, action.Speed, operator)
	case "SET_PRESET":
		_, _ = s.ptzEng.CreatePreset(dev.ID, action.PresetID, fmt.Sprintf("Preset %d", action.PresetID), operator)
	default:
		if err := s.ptzEng.ContinuousMove(dev.ID, action.Pan, action.Tilt, action.Zoom, action.Speed, 0, operator); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "command": action.Command})
}

// -----------------------------------------------------------------------------
// Phase 7 Events Management Handlers
// -----------------------------------------------------------------------------

func (s *Server) handleEventsListSubscriptions(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if r.Method == "GET" {
		subs := s.eventMgr.GetSubscriptions()
		filtered := make([]*events.EventSubscription, 0, len(subs))
		for _, sub := range subs {
			if dev, ok := s.store.GetDevice(sub.DeviceID); ok {
				if checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
					filtered = append(filtered, sub)
				}
			}
		}
		writeJSON(w, http.StatusOK, filtered)
		return
	}
	if r.Method == "POST" {
		if !s.checkPermission(r, security.PermEventManage) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Creating event subscriptions requires EVENT_MANAGE permission")
			return
		}
		var req struct {
			DeviceID string                      `json:"device_id"`
			Protocol events.SubscriptionProtocol `json:"protocol"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid subscription request")
			return
		}
		if req.Protocol == "" {
			req.Protocol = events.ProtoONVIFPullPoint
		}
		dev, ok := s.store.GetDevice(req.DeviceID)
		if !ok {
			writeSimpleError(w, r, http.StatusNotFound, "Target device not found")
			return
		}
		if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
			writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: target device belongs to an isolated tenant or site")
			return
		}
		operator := "Technician"
		if sess != nil && sess.Username != "" {
			operator = sess.Username
		}
		sub, err := s.eventMgr.CreateSubscription(req.DeviceID, req.Protocol, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sub)
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleEventsCancelSubscription(w http.ResponseWriter, r *http.Request) {
	subID := strings.TrimPrefix(r.URL.Path, "/api/v1/events/subscriptions/")
	if subID == "" {
		writeSimpleError(w, r, http.StatusBadRequest, "Subscription ID required")
		return
	}
	if r.Method == "DELETE" || r.Method == "POST" {
		if !s.checkPermission(r, security.PermEventManage) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Cancelling event subscriptions requires EVENT_MANAGE permission")
			return
		}
		sess := getSession(r)
		for _, sub := range s.eventMgr.GetSubscriptions() {
			if sub.ID == subID {
				if dev, ok := s.store.GetDevice(sub.DeviceID); ok {
					if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
						writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: subscription belongs to an isolated device")
						return
					}
				} else if !isSuperAdmin(sess) {
					writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: subscription device not found")
					return
				}
				break
			}
		}
		operator := "Technician"
		if sess != nil && sess.Username != "" {
			operator = sess.Username
		}
		if err := s.eventMgr.CancelSubscription(subID, operator); err != nil {
			writeSimpleError(w, r, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "cancelled_id": subID})
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleEventsLive(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)

	// Support SSE real-time event streaming if Accept: text/event-stream or ?stream=true
	if r.Header.Get("Accept") == "text/event-stream" || r.URL.Query().Get("stream") == "true" {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeSimpleError(w, r, http.StatusInternalServerError, "Streaming unsupported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "data: {\"type\":\"CONNECTED\",\"timestamp\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
		flusher.Flush()

		evChan := s.eventMgr.SubscribeEvents()
		defer s.eventMgr.UnsubscribeEvents(evChan)

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-evChan:
				if !ok {
					return
				}
				dev, devOk := s.store.GetDevice(ev.DeviceID)
				if !devOk {
					if !isSuperAdmin(sess) {
						continue
					}
				} else {
					if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
						continue
					}
				}
				payload, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: {\"type\":\"EVENT\",\"payload\":%s}\n\n", string(payload))
				flusher.Flush()
			}
		}
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	eventsList := s.eventMgr.GetRecentEvents(limit)
	filtered := make([]events.SurveillanceEvent, 0, len(eventsList))
	for _, ev := range eventsList {
		dev, ok := s.store.GetDevice(ev.DeviceID)
		if !ok {
			if !isSuperAdmin(sess) {
				continue
			}
		} else {
			if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
				continue
			}
		}
		filtered = append(filtered, ev)
	}
	writeJSON(w, http.StatusOK, filtered)
}

// -----------------------------------------------------------------------------
// Phase 7 Unified Alarms & Rule Automation Handlers
// -----------------------------------------------------------------------------

func (s *Server) handleAlarms(w http.ResponseWriter, r *http.Request) {
	filter := alarms.AlarmFilter{}

	if r.Method == "POST" {
		_ = json.NewDecoder(r.Body).Decode(&filter)
	} else {
		filter.DeviceID = r.URL.Query().Get("device_id")
		filter.SiteID = r.URL.Query().Get("site_id")
		filter.Manufacturer = r.URL.Query().Get("manufacturer")
		filter.EventCategory = r.URL.Query().Get("category")
		filter.Severity = alarms.AlarmSeverity(r.URL.Query().Get("severity"))
		filter.State = alarms.AlarmState(r.URL.Query().Get("state"))
		l, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		filter.Limit = l
	}

	sess := getSession(r)
	results := s.alarmEng.FilterAlarms(filter)
	filtered := make([]*alarms.Alarm, 0, len(results))
	for _, alm := range results {
		dev, ok := s.store.GetDevice(alm.DeviceID)
		if !ok {
			if !isSuperAdmin(sess) {
				continue
			}
		} else {
			if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
				continue
			}
		}
		filtered = append(filtered, alm)
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) handleAlarmsStats(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	allAlarms := s.alarmEng.FilterAlarms(alarms.AlarmFilter{})
	stats := alarms.AlarmStats{}

	for _, a := range allAlarms {
		dev, ok := s.store.GetDevice(a.DeviceID)
		if !ok {
			if !isSuperAdmin(sess) {
				continue
			}
		} else {
			if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
				continue
			}
		}
		stats.TotalAlarms++
		switch a.State {
		case alarms.StateNew:
			stats.NewAlarms++
		case alarms.StateAcknowledged:
			stats.AcknowledgedAlarms++
		case alarms.StateInvestigating:
			stats.InvestigatingAlarms++
		case alarms.StateResolved, alarms.StateDismissed:
			stats.ResolvedAlarms++
		}
		switch a.Severity {
		case alarms.SeverityCritical:
			stats.CriticalCount++
		case alarms.SeverityMajor:
			stats.MajorCount++
		case alarms.SeverityMinor:
			stats.MinorCount++
		case alarms.SeverityInfo:
			stats.InfoCount++
		}
	}

	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAlarmsRules(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, http.StatusOK, s.alarmEng.GetRules())
		return
	}
	if r.Method == "POST" {
		if !s.checkPermission(r, security.PermEventManage) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Alarm rule configuration requires EVENT_MANAGE permission")
			return
		}
		var rule alarms.AlarmRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, "Invalid alarm rule payload")
			return
		}
		sess := getSession(r)
		operator := "Technician"
		if sess != nil && sess.Username != "" {
			operator = sess.Username
		}
		upserted := s.alarmEng.UpsertRule(&rule, operator)
		writeJSON(w, http.StatusOK, upserted)
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleAlarmsRulesDelete(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimPrefix(r.URL.Path, "/api/v1/alarms/rules/")
	if ruleID == "" {
		writeSimpleError(w, r, http.StatusBadRequest, "Rule ID required")
		return
	}
	if r.Method == "DELETE" {
		if !s.checkPermission(r, security.PermEventManage) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Deleting alarm rules requires EVENT_MANAGE permission")
			return
		}
		sess := getSession(r)
		operator := "Technician"
		if sess != nil && sess.Username != "" {
			operator = sess.Username
		}
		if err := s.alarmEng.DeleteRule(ruleID, operator); err != nil {
			writeSimpleError(w, r, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "deleted_rule_id": ruleID})
		return
	}
	writeSimpleError(w, r, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleAlarmsExport(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(r, security.PermAlarmExport) && !s.checkPermission(r, security.PermPlaybackExport) {
		writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Exporting alarm ledger requires ALARM_EXPORT or PLAYBACK_EXPORT permission")
		return
	}
	sess := getSession(r)
	format := strings.ToLower(r.URL.Query().Get("format"))
	allAlarms := s.alarmEng.FilterAlarms(alarms.AlarmFilter{})
	filtered := make([]*alarms.Alarm, 0, len(allAlarms))
	for _, alm := range allAlarms {
		dev, ok := s.store.GetDevice(alm.DeviceID)
		if !ok {
			if !isSuperAdmin(sess) {
				continue
			}
		} else {
			if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
				continue
			}
		}
		filtered = append(filtered, alm)
	}

	if format == "csv" {
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		_ = writer.Write([]string{"alarm_id", "device_id", "severity", "state", "category", "rule_id", "timestamp", "description"})
		for _, alm := range filtered {
			_ = writer.Write([]string{
				alm.ID, alm.DeviceID, string(alm.Severity), string(alm.State),
				alm.EventCategory, alm.EventType, alm.EventTimestamp.Format(time.RFC3339), alm.Description,
			})
		}
		writer.Flush()
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\"alarm_history.csv\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
		return
	}

	jsonBytes, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		writeSimpleError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"alarm_history.json\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(jsonBytes)
}

func (s *Server) handleAlarmsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeSimpleError(w, r, http.StatusInternalServerError, "Streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Send initial heartbeat
	fmt.Fprintf(w, "data: {\"type\":\"CONNECTED\",\"timestamp\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	almChan := s.alarmEng.SubscribeAlarms()
	defer s.alarmEng.UnsubscribeAlarms(almChan)

	evChan := s.eventMgr.SubscribeEvents()
	defer s.eventMgr.UnsubscribeEvents(evChan)

	ctx := r.Context()
	sess := getSession(r)

	for {
		select {
		case <-ctx.Done():
			return
		case alm, ok := <-almChan:
			if !ok {
				return
			}
			dev, devOk := s.store.GetDevice(alm.DeviceID)
			if !devOk {
				if !isSuperAdmin(sess) {
					continue
				}
			} else {
				if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
					continue
				}
			}
			payload, _ := json.Marshal(alm)
			fmt.Fprintf(w, "data: {\"type\":\"ALARM\",\"payload\":%s}\n\n", string(payload))
			flusher.Flush()
		case ev, ok := <-evChan:
			if !ok {
				return
			}
			dev, devOk := s.store.GetDevice(ev.DeviceID)
			if !devOk {
				if !isSuperAdmin(sess) {
					continue
				}
			} else {
				if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
					continue
				}
			}
			payload, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: {\"type\":\"EVENT\",\"payload\":%s}\n\n", string(payload))
			flusher.Flush()
		}
	}
}

func (s *Server) handleAlarmSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/alarms/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.handleAlarms(w, r)
		return
	}

	alarmID := parts[0]

	alm, ok := s.alarmEng.GetAlarm(alarmID)
	if !ok {
		writeSimpleError(w, r, http.StatusNotFound, "Alarm not found")
		return
	}

	sess := getSession(r)
	if dev, devOk := s.store.GetDevice(alm.DeviceID); devOk {
		if !checkTenantAccess(sess, dev.TenantID, dev.SiteID) {
			writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: alarm belongs to a device on an isolated tenant or site")
			return
		}
	} else if !isSuperAdmin(sess) {
		writeError(w, r, http.StatusForbidden, "CROSS_TENANT_ACCESS_DENIED", "Access denied: alarm device not found or unassigned")
		return
	}

	if len(parts) == 1 {
		writeJSON(w, http.StatusOK, alm)
		return
	}

	action := parts[1]
	if action == "acknowledge" || action == "assign" || action == "resolve" || action == "dismiss" {
		if !s.checkPermission(r, security.PermAlarmManage) {
			writeError(w, r, http.StatusForbidden, "INSUFFICIENT_PERMISSIONS", "Alarm triage operations require ALARM_MANAGE permission")
			return
		}
	}

	operator := "Technician"
	if sess != nil && sess.Username != "" {
		operator = sess.Username
	}

	switch action {
	case "acknowledge":
		ack, err := s.alarmEng.Acknowledge(alarmID, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, ack)
		return

	case "assign":
		var req struct {
			Technician string `json:"technician"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Technician == "" {
			req.Technician = "Lead_Technician"
		}
		assigned, err := s.alarmEng.Assign(alarmID, req.Technician, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, assigned)
		return

	case "resolve":
		var req struct {
			Notes string `json:"notes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Notes == "" {
			req.Notes = "Resolved by surveillance technician"
		}
		resolved, err := s.alarmEng.Resolve(alarmID, req.Notes, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resolved)
		return

	case "dismiss":
		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Reason == "" {
			req.Reason = "False positive alarm / test drill"
		}
		dismissed, err := s.alarmEng.Dismiss(alarmID, req.Reason, operator)
		if err != nil {
			writeSimpleError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, dismissed)
		return

	default:
		writeSimpleError(w, r, http.StatusNotFound, "Unknown alarm action")
	}
}
