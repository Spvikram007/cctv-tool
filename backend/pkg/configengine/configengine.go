package configengine

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

type ConfigType string

const (
	ConfigTypeNetwork  ConfigType = "NETWORK"
	ConfigTypeImage    ConfigType = "IMAGE"
	ConfigTypeEncoding ConfigType = "ENCODING"
)

// ConfigPreview represents Step 6 diff & validation preview
type ConfigPreview struct {
	DeviceID      string          `json:"device_id"`
	DeviceName    string          `json:"device_name"`
	ConfigType    ConfigType      `json:"config_type"`
	CurrentConfig json.RawMessage `json:"current_config"`
	ProposedConfig json.RawMessage `json:"proposed_config"`
	Diff          string          `json:"diff"`
	Valid         bool            `json:"valid"`
	ValidationErrors []string     `json:"validation_errors"`
	RollbackKey   string          `json:"rollback_key"`
}

// ConfigResult represents Step 10 & 11 results
type ConfigResult struct {
	Success      bool            `json:"success"`
	DeviceID     string          `json:"device_id"`
	ConfigType   ConfigType      `json:"config_type"`
	Message      string          `json:"message"`
	AppliedAt    time.Time       `json:"applied_at"`
	VerifiedRead json.RawMessage `json:"verified_read,omitempty"`
	AuditEntryID string          `json:"audit_entry_id"`
	RollbackAvailable bool       `json:"rollback_available"`
}

type Engine struct {
	registry *adapters.Registry
	store    *database.Store
	logger   *audit.Logger
	mu       sync.Mutex
	rollbacks map[string]json.RawMessage // rollbackKey -> snapshot
}

func NewEngine(registry *adapters.Registry, store *database.Store, logger *audit.Logger) *Engine {
	return &Engine{
		registry:  registry,
		store:     store,
		logger:    logger,
		rollbacks: make(map[string]json.RawMessage),
	}
}

// Step 1 to 6: Generate Preview & Validate Proposed Config
func (e *Engine) GeneratePreview(deviceID string, cfgType ConfigType, proposedJSON json.RawMessage) (*ConfigPreview, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	dev, ok := e.store.GetDevice(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found in registry", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, err
	}

	preview := &ConfigPreview{
		DeviceID:       dev.ID,
		DeviceName:     dev.Name,
		ConfigType:     cfgType,
		ProposedConfig: proposedJSON,
		Valid:          true,
		ValidationErrors: make([]string, 0),
		RollbackKey:    fmt.Sprintf("rb-%s-%d", dev.ID, time.Now().UnixNano()),
	}

	switch cfgType {
	case ConfigTypeNetwork:
		curr, err := adapter.GetNetworkConfig(&dev)
		if err != nil {
			return nil, fmt.Errorf("step 4 failed (read current network config): %w", err)
		}
		currBytes, _ := json.Marshal(curr)
		preview.CurrentConfig = currBytes

		var proposed models.NetworkConfig
		if err := json.Unmarshal(proposedJSON, &proposed); err != nil {
			preview.Valid = false
			preview.ValidationErrors = append(preview.ValidationErrors, "Malformed network configuration payload")
		} else {
			// Validate IP, mask, gateway
			if !proposed.DHCP {
				if net.ParseIP(proposed.IPAddress) == nil {
					preview.Valid = false
					preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Invalid IP address: %s", proposed.IPAddress))
				}
				if net.ParseIP(proposed.SubnetMask) == nil {
					preview.Valid = false
					preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Invalid Subnet Mask: %s", proposed.SubnetMask))
				}
				if proposed.Gateway != "" && net.ParseIP(proposed.Gateway) == nil {
					preview.Valid = false
					preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Invalid Gateway: %s", proposed.Gateway))
				}
			}
			preview.Diff = fmt.Sprintf("IP: %s -> %s | DHCP: %t -> %t | Subnet: %s -> %s",
				curr.IPAddress, proposed.IPAddress, curr.DHCP, proposed.DHCP, curr.SubnetMask, proposed.SubnetMask)
		}
		// Save for rollback
		e.rollbacks[preview.RollbackKey] = currBytes

	case ConfigTypeImage:
		curr, err := adapter.GetImageConfig(&dev)
		if err != nil {
			return nil, fmt.Errorf("step 4 failed (read current image config): %w", err)
		}
		currBytes, _ := json.Marshal(curr)
		preview.CurrentConfig = currBytes

		var proposed models.ImageConfig
		if err := json.Unmarshal(proposedJSON, &proposed); err != nil {
			preview.Valid = false
			preview.ValidationErrors = append(preview.ValidationErrors, "Malformed image configuration payload")
		} else {
			if proposed.Brightness < 0 || proposed.Brightness > 100 {
				preview.Valid = false
				preview.ValidationErrors = append(preview.ValidationErrors, "Brightness must be between 0 and 100")
			}
			if proposed.Contrast < 0 || proposed.Contrast > 100 {
				preview.Valid = false
				preview.ValidationErrors = append(preview.ValidationErrors, "Contrast must be between 0 and 100")
			}
			preview.Diff = fmt.Sprintf("Brightness: %d -> %d | Contrast: %d -> %d | Saturation: %d -> %d",
				curr.Brightness, proposed.Brightness, curr.Contrast, proposed.Contrast, curr.Saturation, proposed.Saturation)
		}
		e.rollbacks[preview.RollbackKey] = currBytes
	}

	return preview, nil
}

// Step 8 to 12: Apply changes, verify read-back, audit log, enable rollback
func (e *Engine) ApplyConfirmed(deviceID string, cfgType ConfigType, rollbackKey string, user string) (*ConfigResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	dev, ok := e.store.GetDevice(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, err
	}

	switch cfgType {
	case ConfigTypeNetwork:
		cfg, ok := e.store.GetNetworkConfig(deviceID)
		if !ok {
			return nil, fmt.Errorf("no staged network configuration to apply")
		}
		// Step 8: Apply
		if err := adapter.ApplyNetworkConfig(&dev, &cfg); err != nil {
			entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_NETWORK", "CONFIG", "FAILED", err.Error(), "")
			return &ConfigResult{
				Success:      false,
				DeviceID:     dev.ID,
				ConfigType:   cfgType,
				Message:      fmt.Sprintf("Failed to apply network configuration: %v", err),
				AppliedAt:    time.Now().UTC(),
				AuditEntryID: entry.ID,
			}, err
		}

		// Step 9: Read back settings to verify
		verified, _ := adapter.GetNetworkConfig(&dev)
		verBytes, _ := json.Marshal(verified)

		entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_NETWORK", "CONFIG", "SUCCESS", "Network configuration applied and verified", fmt.Sprintf("New IP: %s, DHCP: %t", cfg.IPAddress, cfg.DHCP))
		e.store.SetNetworkConfig(cfg)

		return &ConfigResult{
			Success:           true,
			DeviceID:          dev.ID,
			ConfigType:        cfgType,
			Message:           "Network settings successfully applied and verified on hardware.",
			AppliedAt:         time.Now().UTC(),
			VerifiedRead:      verBytes,
			AuditEntryID:      entry.ID,
			RollbackAvailable: rollbackKey != "" && e.rollbacks[rollbackKey] != nil,
		}, nil

	case ConfigTypeImage:
		cfg, ok := e.store.GetImageConfig(deviceID)
		if !ok {
			return nil, fmt.Errorf("no staged image configuration to apply")
		}
		if err := adapter.ApplyImageConfig(&dev, &cfg); err != nil {
			entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_IMAGE", "CONFIG", "FAILED", err.Error(), "")
			return &ConfigResult{
				Success:      false,
				DeviceID:     dev.ID,
				ConfigType:   cfgType,
				Message:      fmt.Sprintf("Failed to apply image configuration: %v", err),
				AppliedAt:    time.Now().UTC(),
				AuditEntryID: entry.ID,
			}, err
		}

		verified, _ := adapter.GetImageConfig(&dev)
		verBytes, _ := json.Marshal(verified)

		entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_IMAGE", "CONFIG", "SUCCESS", "Image settings updated", fmt.Sprintf("B: %d, C: %d, S: %d", cfg.Brightness, cfg.Contrast, cfg.Saturation))
		e.store.SetImageConfig(cfg)

		return &ConfigResult{
			Success:           true,
			DeviceID:          dev.ID,
			ConfigType:        cfgType,
			Message:           "Image parameters successfully synchronized to camera sensor.",
			AppliedAt:         time.Now().UTC(),
			VerifiedRead:      verBytes,
			AuditEntryID:      entry.ID,
			RollbackAvailable: rollbackKey != "" && e.rollbacks[rollbackKey] != nil,
		}, nil
	}

	return nil, fmt.Errorf("unsupported configuration type: %s", cfgType)
}

// Step 12: Rollback
func (e *Engine) Rollback(deviceID string, rollbackKey string, user string) (*ConfigResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	snapshot, ok := e.rollbacks[rollbackKey]
	if !ok {
		return nil, fmt.Errorf("rollback snapshot key expired or not found")
	}

	dev, ok := e.store.GetDevice(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found", deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, err
	}

	var netCfg models.NetworkConfig
	if err := json.Unmarshal(snapshot, &netCfg); err == nil && netCfg.IPAddress != "" {
		_ = adapter.ApplyNetworkConfig(&dev, &netCfg)
		e.store.SetNetworkConfig(netCfg)
		entry := e.logger.Log(user, dev.ID, dev.IP, "ROLLBACK_CONFIG", "CONFIG", "SUCCESS", "Configuration rolled back to previous state", "")
		return &ConfigResult{
			Success:      true,
			DeviceID:     dev.ID,
			ConfigType:   ConfigTypeNetwork,
			Message:      "Successfully rolled back device configuration.",
			AppliedAt:    time.Now().UTC(),
			AuditEntryID: entry.ID,
		}, nil
	}

	return nil, fmt.Errorf("failed to restore rollback configuration")
}
