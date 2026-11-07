package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cctv-sentinel/pkg/models"
)

// JobStatus represents the state of an asynchronous discovery job
type JobStatus string

const (
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusCancelled JobStatus = "CANCELLED"
	JobStatusFailed    JobStatus = "FAILED"
)

// DiscoveredDevice represents an identified CCTV unit resulting from discovery
type DiscoveredDevice struct {
	Device      models.Device     `json:"device"`
	Fingerprint FingerprintResult `json:"fingerprint"`
}

// DiscoveryJob holds the status and results of a discovery operation
type DiscoveryJob struct {
	ID              string             `json:"id"`
	Method          string             `json:"method"` // ONVIF, TCP, HYBRID
	Status          JobStatus          `json:"status"`
	Interface       string             `json:"interface,omitempty"`
	CIDR            string             `json:"cidr,omitempty"`
	ProgressPercent int                `json:"progress_percent"`
	ScannedTargets  int                `json:"scanned_targets"`
	TotalTargets    int                `json:"total_targets"`
	FoundCount      int                `json:"found_count"`
	Results         []DiscoveredDevice `json:"results"`
	CreatedAt       time.Time          `json:"created_at"`
	CompletedAt     *time.Time         `json:"completed_at,omitempty"`
	Error           string             `json:"error,omitempty"`

	cancelFunc context.CancelFunc `json:"-"`
}

// JobManager manages active and completed discovery jobs safely in memory
type JobManager struct {
	mu   sync.RWMutex
	jobs map[string]*DiscoveryJob
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]*DiscoveryJob),
	}
}

// CreateJob creates and registers a new discovery job
func (jm *JobManager) CreateJob(method, iface, cidr string, totalTargets int, cancel context.CancelFunc) *DiscoveryJob {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job := &DiscoveryJob{
		ID:              fmt.Sprintf("job-%d", time.Now().UnixNano()),
		Method:          method,
		Status:          JobStatusRunning,
		Interface:       iface,
		CIDR:            cidr,
		ProgressPercent: 0,
		ScannedTargets:  0,
		TotalTargets:    totalTargets,
		FoundCount:      0,
		Results:         make([]DiscoveredDevice, 0),
		CreatedAt:       time.Now().UTC(),
		cancelFunc:      cancel,
	}

	jm.jobs[job.ID] = job
	return job
}

// GetJob retrieves a job by ID
func (jm *JobManager) GetJob(id string) (*DiscoveryJob, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	job, ok := jm.jobs[id]
	if !ok {
		return nil, false
	}
	// Return shallow copy for thread safety
	cpy := *job
	return &cpy, true
}

// CancelJob cancels a running job
func (jm *JobManager) CancelJob(id string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, ok := jm.jobs[id]
	if !ok {
		return false
	}

	if job.Status == JobStatusRunning {
		if job.cancelFunc != nil {
			job.cancelFunc()
		}
		job.Status = JobStatusCancelled
		now := time.Now().UTC()
		job.CompletedAt = &now
		return true
	}
	return false
}

// UpdateProgress updates job target counters
func (jm *JobManager) UpdateProgress(id string, scanned, total int) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, ok := jm.jobs[id]
	if !ok {
		return
	}
	job.ScannedTargets = scanned
	job.TotalTargets = total
	if total > 0 {
		job.ProgressPercent = (scanned * 100) / total
		if job.ProgressPercent > 99 && job.Status == JobStatusRunning {
			job.ProgressPercent = 99
		}
	}
}

// AddResult appends a discovered device to the job
func (jm *JobManager) AddResult(id string, dev DiscoveredDevice) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, ok := jm.jobs[id]
	if !ok {
		return
	}

	// Deduplicate and intelligently merge by IP or MAC
	for i, r := range job.Results {
		isSameHost := (dev.Device.IP == r.Device.IP) ||
			(dev.Device.MAC != "" && dev.Device.MAC != "00:00:00:00:00:00" && r.Device.MAC == dev.Device.MAC)

		if isSameHost {
			merged := r

			// 1. Keep higher confidence manufacturer and model
			if r.Device.Model == "" || r.Device.Model == "Unknown" {
				if dev.Device.Model != "" && dev.Device.Model != "Unknown" {
					merged.Device.Model = dev.Device.Model
					merged.Fingerprint.Model = dev.Fingerprint.Model
				}
			}
			if r.Device.Manufacturer == models.VendorGeneric && dev.Device.Manufacturer != models.VendorGeneric {
				merged.Device.Manufacturer = dev.Device.Manufacturer
				merged.Fingerprint.IdentifiedManufacturer = dev.Fingerprint.IdentifiedManufacturer
			}
			if merged.Device.Category == models.CategoryUnknown || merged.Device.Category == models.CategoryIPCamera {
				if dev.Device.Category != models.CategoryUnknown {
					merged.Device.Category = dev.Device.Category
					merged.Fingerprint.Category = dev.Fingerprint.Category
				}
			}

			// 2. Merge MAC if previously missing
			if merged.Device.MAC == "" && dev.Device.MAC != "" {
				merged.Device.MAC = dev.Device.MAC
				merged.Fingerprint.MAC = dev.Fingerprint.MAC
			}

			// 3. Merge open ports and services
			for _, p := range dev.Fingerprint.Services.OpenPorts {
				if !containsPort(merged.Fingerprint.Services.OpenPorts, p) {
					merged.Fingerprint.Services.OpenPorts = append(merged.Fingerprint.Services.OpenPorts, p)
				}
			}
			if merged.Fingerprint.Services.RTSPServer == "" && dev.Fingerprint.Services.RTSPServer != "" {
				merged.Fingerprint.Services.RTSPServer = dev.Fingerprint.Services.RTSPServer
			}
			if merged.Fingerprint.Services.RTSPPublicOpts == "" && dev.Fingerprint.Services.RTSPPublicOpts != "" {
				merged.Fingerprint.Services.RTSPPublicOpts = dev.Fingerprint.Services.RTSPPublicOpts
			}
			if merged.Fingerprint.Services.HTTPServer == "" && dev.Fingerprint.Services.HTTPServer != "" {
				merged.Fingerprint.Services.HTTPServer = dev.Fingerprint.Services.HTTPServer
			}
			if merged.Fingerprint.Services.ONVIFServiceURL == "" && dev.Fingerprint.Services.ONVIFServiceURL != "" {
				merged.Fingerprint.Services.ONVIFServiceURL = dev.Fingerprint.Services.ONVIFServiceURL
			}

			// 4. Merge protocols
			for _, proto := range dev.Device.Protocols {
				hasProto := false
				for _, existingP := range merged.Device.Protocols {
					if existingP == proto {
						hasProto = true
						break
					}
				}
				if !hasProto {
					merged.Device.Protocols = append(merged.Device.Protocols, proto)
					merged.Fingerprint.Protocols = append(merged.Fingerprint.Protocols, proto)
				}
			}

			// 5. Update confidence score to highest level
			if dev.Fingerprint.Confidence > merged.Fingerprint.Confidence {
				merged.Fingerprint.Confidence = dev.Fingerprint.Confidence
				merged.Fingerprint.ConfidenceLevel = dev.Fingerprint.ConfidenceLevel
			}

			// 6. Append non-duplicate evidence sources
			for _, newEv := range dev.Fingerprint.EvidenceSources {
				hasEv := false
				for _, oldEv := range merged.Fingerprint.EvidenceSources {
					if oldEv.Category == newEv.Category {
						hasEv = true
						break
					}
				}
				if !hasEv {
					merged.Fingerprint.EvidenceSources = append(merged.Fingerprint.EvidenceSources, newEv)
				}
			}

			job.Results[i] = merged
			return
		}
	}

	job.Results = append(job.Results, dev)
	job.FoundCount = len(job.Results)
}

// MarkCompleted marks a job as completed
func (jm *JobManager) MarkCompleted(id string, err error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, ok := jm.jobs[id]
	if !ok {
		return
	}

	now := time.Now().UTC()
	job.CompletedAt = &now
	job.ProgressPercent = 100
	if err != nil {
		if job.Status != JobStatusCancelled {
			job.Status = JobStatusFailed
			job.Error = err.Error()
		}
	} else if job.Status == JobStatusRunning {
		job.Status = JobStatusCompleted
	}
}

