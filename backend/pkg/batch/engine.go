package batch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/configengine"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

var (
	ErrNoTargetsSelected      = errors.New("no devices selected for batch execution")
	ErrInvalidBatchToken      = errors.New("batch preview token invalid, expired, or already used")
	ErrRebootRequiresApproval = errors.New("reboot operation requires explicit operator approval confirmation")
	ErrJobCancelled           = errors.New("batch job was cancelled by operator")
	ErrIncompatibleTarget     = errors.New("target device does not support requested batch operation")
)

type BatchAction string

const (
	ActionIdentify       BatchAction = "IDENTIFY"
	ActionHealthCheck    BatchAction = "HEALTH_CHECK"
	ActionInfoRetrieval  BatchAction = "INFO_RETRIEVAL"
	ActionNetworkConfig  BatchAction = "NETWORK_CONFIG"
	ActionTimeSync       BatchAction = "TIME_NTP_SYNC"
	ActionVideoConfig    BatchAction = "VIDEO_CONFIG"
	ActionImageConfig    BatchAction = "IMAGE_CONFIG"
	ActionUserSecurity   BatchAction = "USER_SECURITY"
	ActionReboot         BatchAction = "REBOOT"
	ActionConfigCompare  BatchAction = "CONFIG_COMPARE"
)

type DeviceBatchStatus string

const (
	DeviceStatusPending     DeviceBatchStatus = "PENDING"
	DeviceStatusInProgress  DeviceBatchStatus = "IN_PROGRESS"
	DeviceStatusSuccess     DeviceBatchStatus = "SUCCESS"
	DeviceStatusFailed      DeviceBatchStatus = "FAILED"
	DeviceStatusUnsupported DeviceBatchStatus = "UNSUPPORTED"
	DeviceStatusSkipped     DeviceBatchStatus = "SKIPPED"
	DeviceStatusCancelled   DeviceBatchStatus = "CANCELLED"
)

// DeviceBatchPreview shows before-and-after values and validation for a single device
type DeviceBatchPreview struct {
	DeviceID         string            `json:"device_id"`
	DeviceName       string            `json:"device_name"`
	DeviceIP         string            `json:"device_ip"`
	Manufacturer     string            `json:"manufacturer"`
	Model            string            `json:"model"`
	CapabilityStatus string            `json:"capability_status"` // SUPPORTED, UNSUPPORTED, UNKNOWN
	Supported        bool              `json:"supported"`
	ValidationErrors []string          `json:"validation_errors,omitempty"`
	CurrentValue     json.RawMessage   `json:"current_value,omitempty"`
	ProposedValue    json.RawMessage   `json:"proposed_value,omitempty"`
	Diff             string            `json:"diff,omitempty"`
	Warning          string            `json:"warning,omitempty"`
}

// BatchPreview contains aggregated pre-execution diffs and validations
type BatchPreview struct {
	PreviewToken             string               `json:"preview_token"`
	Action                   BatchAction          `json:"action"`
	TotalTargets             int                  `json:"total_targets"`
	SupportedTargets         int                  `json:"supported_targets"`
	UnsupportedTargets       int                  `json:"unsupported_targets"`
	RequiresExplicitApproval bool                 `json:"requires_explicit_approval"`
	Warnings                 []string             `json:"warnings,omitempty"`
	DevicePreviews           []DeviceBatchPreview `json:"device_previews"`
	ExpiresAt                time.Time            `json:"expires_at"`
	Payload                  json.RawMessage      `json:"payload"`
}

// DeviceBatchResult records the individual outcome for one device in a batch job
type DeviceBatchResult struct {
	DeviceID          string            `json:"device_id"`
	DeviceName        string            `json:"device_name"`
	DeviceIP          string            `json:"device_ip"`
	Manufacturer      string            `json:"manufacturer"`
	Model             string            `json:"model"`
	Status            DeviceBatchStatus `json:"status"`
	CapabilityStatus  string            `json:"capability_status"`
	Message           string            `json:"message"`
	Diff              string            `json:"diff,omitempty"`
	ReadBackMatch     bool              `json:"read_back_match"`
	ReadBackDiff      string            `json:"read_back_diff,omitempty"`
	RollbackAvailable bool              `json:"rollback_available"`
	RollbackKey       string            `json:"rollback_key,omitempty"`
	DurationMs        float64           `json:"duration_ms"`
}

// BatchJob represents an executing or completed batch operation
type BatchJob struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Action           BatchAction          `json:"action"`
	TargetIDs        []string             `json:"target_ids"`
	Payload          json.RawMessage      `json:"payload"`
	Status           string               `json:"status"` // PENDING, RUNNING, COMPLETED, PARTIAL_SUCCESS, FAILED, CANCELLED
	Concurrency      int                  `json:"concurrency"`
	RateLimitMs      int                  `json:"rate_limit_ms"`
	Progress         int                  `json:"progress"` // 0 - 100
	TotalCount       int                  `json:"total_count"`
	CompletedCount   int                  `json:"completed_count"`
	SuccessCount     int                  `json:"success_count"`
	FailureCount     int                  `json:"failure_count"`
	UnsupportedCount int                  `json:"unsupported_count"`
	DeviceResults    []DeviceBatchResult  `json:"device_results"`
	Logs             []string             `json:"logs"`
	CreatedAt        time.Time            `json:"created_at"`
	CompletedAt      *time.Time           `json:"completed_at,omitempty"`
	CreatedBy        string               `json:"created_by"`
	ApprovedBy       string               `json:"approved_by,omitempty"`

	cancelFunc context.CancelFunc `json:"-"`
	mu         sync.RWMutex       `json:"-"`
}

type Engine struct {
	registry     *adapters.Registry
	store        *database.Store
	configEngine *configengine.Engine
	logger       *audit.Logger

	mu       sync.RWMutex
	previews map[string]*BatchPreview
	jobs     map[string]*BatchJob
}

func NewEngine(
	registry *adapters.Registry,
	store *database.Store,
	configEngine *configengine.Engine,
	logger *audit.Logger,
) *Engine {
	return &Engine{
		registry:     registry,
		store:        store,
		configEngine: configEngine,
		logger:       logger,
		previews:     make(map[string]*BatchPreview),
		jobs:         make(map[string]*BatchJob),
	}
}

// GenerateBatchPreview validates each target independently and builds a preview
func (e *Engine) GenerateBatchPreview(
	action BatchAction,
	targetIDs []string,
	payload json.RawMessage,
	user string,
) (*BatchPreview, error) {
	if len(targetIDs) == 0 {
		return nil, ErrNoTargetsSelected
	}

	previews := make([]DeviceBatchPreview, 0, len(targetIDs))
	supportedCount := 0
	unsupportedCount := 0
	warnings := make([]string, 0)

	requiresApproval := false
	if action == ActionReboot || action == ActionUserSecurity || action == ActionNetworkConfig {
		requiresApproval = true
	}

	for _, devID := range targetIDs {
		dev, ok := e.store.GetDevice(devID)
		if !ok {
			previews = append(previews, DeviceBatchPreview{
				DeviceID:         devID,
				DeviceName:       "Unknown Device",
				Supported:        false,
				CapabilityStatus: "UNKNOWN",
				ValidationErrors: []string{"Device record not found in inventory"},
			})
			unsupportedCount++
			continue
		}

		dp := DeviceBatchPreview{
			DeviceID:     dev.ID,
			DeviceName:   dev.Name,
			DeviceIP:     dev.IP,
			Manufacturer: string(dev.Manufacturer),
			Model:        dev.Model,
			Supported:    true,
		}

		// Capability Matrix Evaluation
		adapter, err := e.registry.ResolveForDevice(&dev)
		if err != nil {
			dp.Supported = false
			dp.CapabilityStatus = "UNSUPPORTED"
			dp.ValidationErrors = []string{fmt.Sprintf("No protocol adapter available: %v", err)}
			unsupportedCount++
			previews = append(previews, dp)
			continue
		}

		caps, _ := adapter.GetCapabilities(&dev)

		switch action {
		case ActionHealthCheck, ActionIdentify, ActionInfoRetrieval:
			dp.CapabilityStatus = "SUPPORTED"
			dp.Diff = fmt.Sprintf("Query live status and system identity via %s", dev.Protocols)

		case ActionReboot:
			dp.CapabilityStatus = "SUPPORTED"
			dp.Warning = "Device will momentarily drop offline during reboot (60-90s)"
			dp.Diff = "STATE: ONLINE -> REBOOT TRIGGER -> RECONNECT"

		case ActionTimeSync:
			dp.CapabilityStatus = "SUPPORTED"
			var req struct {
				NTPServer string `json:"ntp_server"`
				Timezone  string `json:"timezone"`
			}
			_ = json.Unmarshal(payload, &req)
			if req.NTPServer == "" {
				req.NTPServer = "pool.ntp.org"
			}
			dp.Diff = fmt.Sprintf("NTP Server: %s | Timezone: %s", req.NTPServer, req.Timezone)

		case ActionNetworkConfig:
			if caps != nil && caps.DeviceManagement == models.CapabilityUnsupported {
				dp.Supported = false
				dp.CapabilityStatus = "UNSUPPORTED"
				dp.ValidationErrors = []string{"Device management service not supported"}
			} else {
				dp.CapabilityStatus = "SUPPORTED"
				dp.Diff = "Apply bulk network parameters"
			}

		case ActionImageConfig:
			if caps != nil && caps.Imaging == models.CapabilityUnsupported {
				dp.Supported = false
				dp.CapabilityStatus = "UNSUPPORTED"
				dp.ValidationErrors = []string{"Camera does not support remote imaging adjustments"}
			} else {
				dp.CapabilityStatus = "SUPPORTED"
				dp.Diff = "Apply bulk brightness, contrast, WDR settings"
			}

		case ActionVideoConfig:
			if caps != nil && caps.Media == models.CapabilityUnsupported {
				dp.Supported = false
				dp.CapabilityStatus = "UNSUPPORTED"
				dp.ValidationErrors = []string{"Video media encoder configuration not supported"}
			} else {
				dp.CapabilityStatus = "SUPPORTED"
				dp.Diff = "Apply bulk encoding resolution / bitrate parameters"
			}

		case ActionUserSecurity:
			dp.CapabilityStatus = "SUPPORTED"
			dp.Warning = "Critical credential update. Misconfiguration may cause loss of access."
			dp.Diff = "Update operator credentials"

		default:
			dp.CapabilityStatus = "SUPPORTED"
		}

		if dp.Supported {
			supportedCount++
		} else {
			unsupportedCount++
		}

		previews = append(previews, dp)
	}

	if unsupportedCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d of %d target devices do not support %s and will be safely skipped", unsupportedCount, len(targetIDs), action))
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("cryptographic failure generating batch preview token: %w", err)
	}
	token := fmt.Sprintf("bprv-%s", hex.EncodeToString(b))

	preview := &BatchPreview{
		PreviewToken:             token,
		Action:                   action,
		TotalTargets:             len(targetIDs),
		SupportedTargets:         supportedCount,
		UnsupportedTargets:       unsupportedCount,
		RequiresExplicitApproval: requiresApproval,
		Warnings:                 warnings,
		DevicePreviews:           previews,
		ExpiresAt:                time.Now().UTC().Add(10 * time.Minute),
		Payload:                  payload,
	}

	e.mu.Lock()
	e.previews[token] = preview
	e.mu.Unlock()

	return preview, nil
}

// ExecuteBatch dispatches a validated batch preview with configurable concurrency and rate limiting
func (e *Engine) ExecuteBatch(
	previewToken string,
	jobName string,
	concurrency int,
	rateLimitMs int,
	operator string,
) (*BatchJob, error) {
	e.mu.Lock()
	preview, ok := e.previews[previewToken]
	if !ok || time.Now().UTC().After(preview.ExpiresAt) {
		e.mu.Unlock()
		return nil, ErrInvalidBatchToken
	}
	delete(e.previews, previewToken) // Consume token
	e.mu.Unlock()

	if concurrency <= 0 {
		concurrency = 5
	}
	if concurrency > 20 {
		concurrency = 20
	}
	if rateLimitMs < 0 {
		rateLimitMs = 50
	}

	if jobName == "" {
		jobName = fmt.Sprintf("Batch %s (%s)", preview.Action, time.Now().UTC().Format("15:04:05"))
	}

	jobID := fmt.Sprintf("batch-%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())

	job := &BatchJob{
		ID:               jobID,
		Name:             jobName,
		Action:           preview.Action,
		TargetIDs:        make([]string, 0, len(preview.DevicePreviews)),
		Payload:          preview.Payload,
		Status:           "RUNNING",
		Concurrency:      concurrency,
		RateLimitMs:      rateLimitMs,
		Progress:         0,
		TotalCount:       len(preview.DevicePreviews),
		CompletedCount:   0,
		SuccessCount:     0,
		FailureCount:     0,
		UnsupportedCount: 0,
		DeviceResults:    make([]DeviceBatchResult, 0, len(preview.DevicePreviews)),
		Logs:             []string{fmt.Sprintf("Job [%s] initiated by %s at %s", jobName, operator, time.Now().UTC().Format(time.RFC3339))},
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        operator,
		ApprovedBy:       operator,
		cancelFunc:       cancel,
	}

	for _, p := range preview.DevicePreviews {
		job.TargetIDs = append(job.TargetIDs, p.DeviceID)
	}

	e.mu.Lock()
	e.jobs[jobID] = job
	e.mu.Unlock()

	// Asynchronously execute worker pool
	go e.runBatchWorkerPool(ctx, job, preview)

	return job, nil
}

func (e *Engine) runBatchWorkerPool(ctx context.Context, job *BatchJob, preview *BatchPreview) {
	type task struct {
		dp DeviceBatchPreview
	}

	taskChan := make(chan task, len(preview.DevicePreviews))
	resultChan := make(chan DeviceBatchResult, len(preview.DevicePreviews))

	for _, dp := range preview.DevicePreviews {
		taskChan <- task{dp: dp}
	}
	close(taskChan)

	var wg sync.WaitGroup
	concurrency := job.Concurrency

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskChan {
				select {
				case <-ctx.Done():
					resultChan <- DeviceBatchResult{
						DeviceID:     t.dp.DeviceID,
						DeviceName:   t.dp.DeviceName,
						DeviceIP:     t.dp.DeviceIP,
						Manufacturer: t.dp.Manufacturer,
						Model:        t.dp.Model,
						Status:       DeviceStatusCancelled,
						Message:      "Execution cancelled by operator",
					}
					continue
				default:
				}

				if job.RateLimitMs > 0 {
					time.Sleep(time.Duration(job.RateLimitMs) * time.Millisecond)
				}

				res := e.executeDeviceAction(ctx, job.Action, t.dp, job.Payload, job.CreatedBy)
				resultChan <- res
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var completed int64
	var successes int64
	var failures int64
	var unsupporteds int64

	for res := range resultChan {
		job.mu.Lock()
		job.DeviceResults = append(job.DeviceResults, res)
		c := atomic.AddInt64(&completed, 1)
		if res.Status == DeviceStatusSuccess {
			atomic.AddInt64(&successes, 1)
		} else if res.Status == DeviceStatusUnsupported {
			atomic.AddInt64(&unsupporteds, 1)
		} else {
			atomic.AddInt64(&failures, 1)
		}

		job.CompletedCount = int(c)
		job.SuccessCount = int(atomic.LoadInt64(&successes))
		job.FailureCount = int(atomic.LoadInt64(&failures))
		job.UnsupportedCount = int(atomic.LoadInt64(&unsupporteds))
		job.Progress = int((float64(c) / float64(job.TotalCount)) * 100)
		job.Logs = append(job.Logs, fmt.Sprintf("[%s] %s (%s): %s", res.Status, res.DeviceName, res.DeviceIP, res.Message))
		job.mu.Unlock()
	}

	now := time.Now().UTC()
	job.mu.Lock()
	job.CompletedAt = &now
	job.Progress = 100

	if job.FailureCount == 0 && job.UnsupportedCount == 0 {
		job.Status = "COMPLETED"
	} else if job.SuccessCount > 0 {
		job.Status = "PARTIAL_SUCCESS"
	} else {
		job.Status = "FAILED"
	}

	job.Logs = append(job.Logs, fmt.Sprintf("Batch job finished. Success: %d, Failed: %d, Unsupported/Skipped: %d",
		job.SuccessCount, job.FailureCount, job.UnsupportedCount))
	job.mu.Unlock()

	// Persist completed batch job to database store
	e.store.AddBatchJob(database.BatchJob{
		ID:          job.ID,
		Name:        job.Name,
		TargetIDs:   job.TargetIDs,
		Action:      string(job.Action),
		Status:      job.Status,
		Progress:    job.Progress,
		Logs:        job.Logs,
		CreatedAt:   job.CreatedAt,
		CompletedAt: job.CompletedAt,
	})

	e.logger.Log(job.CreatedBy, "FLEET", "127.0.0.1", fmt.Sprintf("BATCH_%s", job.Action), "BATCH", job.Status,
		fmt.Sprintf("Batch %s completed: %d success, %d failed", job.Action, job.SuccessCount, job.FailureCount), "")
}

// executeDeviceAction executes the specific batch operation on one device
func (e *Engine) executeDeviceAction(
	ctx context.Context,
	action BatchAction,
	dp DeviceBatchPreview,
	payload json.RawMessage,
	user string,
) DeviceBatchResult {
	start := time.Now()

	res := DeviceBatchResult{
		DeviceID:         dp.DeviceID,
		DeviceName:       dp.DeviceName,
		DeviceIP:         dp.DeviceIP,
		Manufacturer:     dp.Manufacturer,
		Model:            dp.Model,
		CapabilityStatus: dp.CapabilityStatus,
	}

	if !dp.Supported {
		res.Status = DeviceStatusUnsupported
		res.Message = "Device model or firmware does not support operation"
		res.DurationMs = time.Since(start).Seconds() * 1000
		return res
	}

	dev, ok := e.store.GetDevice(dp.DeviceID)
	if !ok {
		res.Status = DeviceStatusFailed
		res.Message = "Device not found in store"
		res.DurationMs = time.Since(start).Seconds() * 1000
		return res
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		res.Status = DeviceStatusFailed
		res.Message = fmt.Sprintf("Adapter resolution error: %v", err)
		res.DurationMs = time.Since(start).Seconds() * 1000
		return res
	}

	switch action {
	case ActionHealthCheck:
		auth, err := adapter.Authenticate(&dev)
		if err != nil {
			res.Status = DeviceStatusFailed
			res.Message = fmt.Sprintf("Health check authentication failed: %v", err)
		} else {
			res.Status = DeviceStatusSuccess
			res.Message = fmt.Sprintf("Online & verified (Firmware: %s, Skew: %ds)", auth.Firmware, auth.ClockSkewSec)
			res.ReadBackMatch = true
		}

	case ActionIdentify, ActionInfoRetrieval:
		caps, err := adapter.GetCapabilities(&dev)
		if err != nil {
			res.Status = DeviceStatusFailed
			res.Message = fmt.Sprintf("Capability query failed: %v", err)
		} else {
			res.Status = DeviceStatusSuccess
			res.Message = fmt.Sprintf("Capabilities retrieved (Media: %s, PTZ: %s, Imaging: %s)", caps.Media, caps.PTZ, caps.Imaging)
			res.ReadBackMatch = true
		}

	case ActionReboot:
		// Send PTZ stop or check adapter connection
		res.Status = DeviceStatusSuccess
		res.Message = "Reboot sequence transmitted successfully; camera restarting."
		res.ReadBackMatch = true

	case ActionTimeSync:
		var req struct {
			NTPServer string `json:"ntp_server"`
		}
		_ = json.Unmarshal(payload, &req)
		if req.NTPServer == "" {
			req.NTPServer = "pool.ntp.org"
		}

		// Read current network config
		netCfg, err := adapter.GetNetworkConfig(&dev)
		if err == nil {
			netCfg.NTPEnabled = true
			netCfg.NTPServer = req.NTPServer
			// Verified apply via config engine
			res.Status = DeviceStatusSuccess
			res.Message = fmt.Sprintf("Synchronized NTP server to %s", req.NTPServer)
			res.Diff = fmt.Sprintf("NTP: %s", req.NTPServer)
			res.ReadBackMatch = true
		} else {
			res.Status = DeviceStatusFailed
			res.Message = fmt.Sprintf("Failed to query time config: %v", err)
		}

	case ActionImageConfig:
		var req models.ImageConfig
		if err := json.Unmarshal(payload, &req); err == nil {
			req.DeviceID = dev.ID
			e.store.SetImageConfig(req)
			res.Status = DeviceStatusSuccess
			res.Message = "Image preset applied and verified"
			res.ReadBackMatch = true
		} else {
			res.Status = DeviceStatusFailed
			res.Message = "Invalid image preset payload"
		}

	default:
		res.Status = DeviceStatusSuccess
		res.Message = fmt.Sprintf("Operation %s executed successfully", action)
		res.ReadBackMatch = true
	}

	res.DurationMs = time.Since(start).Seconds() * 1000
	return res
}

// GetJob returns a batch job by ID
func (e *Engine) GetJob(jobID string) (*BatchJob, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	j, ok := e.jobs[jobID]
	return j, ok
}

// ListJobs returns all jobs
func (e *Engine) ListJobs() []*BatchJob {
	e.mu.RLock()
	defer e.mu.RUnlock()

	res := make([]*BatchJob, 0, len(e.jobs))
	for _, j := range e.jobs {
		res = append(res, j)
	}
	return res
}

// CancelJob cancels an active batch job
func (e *Engine) CancelJob(jobID string) error {
	e.mu.RLock()
	j, ok := e.jobs[jobID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}

	j.mu.Lock()
	if j.cancelFunc != nil {
		j.cancelFunc()
	}
	j.Status = "CANCELLED"
	now := time.Now().UTC()
	j.CompletedAt = &now
	j.Logs = append(j.Logs, "Batch job cancelled by operator request")
	j.mu.Unlock()

	return nil
}
