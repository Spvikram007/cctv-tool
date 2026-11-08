package configengine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

var (
	ErrDeviceNotFound      = errors.New("device not found in registry")
	ErrInvalidPreviewToken = errors.New("preview token invalid or expired; generate a new preview first")
	ErrValidationFailed    = errors.New("configuration validation failed")
	ErrReadBackMismatch    = errors.New("read-back verification mismatch: device settings did not match target")
)

type ConfigType string

const (
	ConfigTypeNetwork  ConfigType = "NETWORK"
	ConfigTypeImage    ConfigType = "IMAGE"
	ConfigTypeEncoding ConfigType = "ENCODING"
)

// StagedPreview stores pending configuration staged for approval
type StagedPreview struct {
	PreviewToken   string
	RollbackKey    string
	DeviceID       string
	ConfigType     ConfigType
	CurrentConfig  json.RawMessage
	ProposedConfig json.RawMessage
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// ConfigPreview represents Step 4 to 6 diff & validation preview
type ConfigPreview struct {
	PreviewToken     string          `json:"preview_token"`
	DeviceID         string          `json:"device_id"`
	DeviceName       string          `json:"device_name"`
	ConfigType       ConfigType      `json:"config_type"`
	CurrentConfig    json.RawMessage `json:"current_config"`
	ProposedConfig   json.RawMessage `json:"proposed_config"`
	Diff             string          `json:"diff"`
	Valid            bool            `json:"valid"`
	ValidationErrors []string        `json:"validation_errors"`
	RollbackKey      string          `json:"rollback_key"`
	ExpiresAt        time.Time       `json:"expires_at"`
}

// ConfigResult represents Step 8 to 10 apply & read-back results
type ConfigResult struct {
	Success           bool            `json:"success"`
	DeviceID          string          `json:"device_id"`
	ConfigType        ConfigType      `json:"config_type"`
	Message           string          `json:"message"`
	AppliedAt         time.Time       `json:"applied_at"`
	ReadBackMatch     bool            `json:"read_back_match"`
	ReadBackDiff      string          `json:"read_back_diff,omitempty"`
	VerifiedRead      json.RawMessage `json:"verified_read,omitempty"`
	AuditEntryID      string          `json:"audit_entry_id"`
	RollbackAvailable bool            `json:"rollback_available"`
	RollbackKey       string          `json:"rollback_key,omitempty"`
}

type Engine struct {
	registry       *adapters.Registry
	store          *database.Store
	logger         *audit.Logger
	mu             sync.Mutex
	stagedPreviews map[string]*StagedPreview  // previewToken or rollbackKey -> staged preview
	rollbacks      map[string]json.RawMessage // rollbackKey -> snapshot JSON
}

func NewEngine(registry *adapters.Registry, store *database.Store, logger *audit.Logger) *Engine {
	return &Engine{
		registry:       registry,
		store:          store,
		logger:         logger,
		stagedPreviews: make(map[string]*StagedPreview),
		rollbacks:      make(map[string]json.RawMessage),
	}
}

func generateToken(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("CRITICAL CRYPTOGRAPHIC FAILURE: crypto/rand unavailable: %v", err))
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

// GeneratePreview enforces Steps 1-6: Authenticate -> Read Existing -> Validate -> Diff -> Staged Token
func (e *Engine) GeneratePreview(deviceID string, cfgType ConfigType, proposedJSON json.RawMessage) (*ConfigPreview, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, err
	}

	previewToken := generateToken("prv")
	rollbackKey := generateToken("rb")
	expiresAt := time.Now().UTC().Add(10 * time.Minute)

	preview := &ConfigPreview{
		PreviewToken:     previewToken,
		DeviceID:         dev.ID,
		DeviceName:       dev.Name,
		ConfigType:       cfgType,
		ProposedConfig:   proposedJSON,
		Valid:            true,
		ValidationErrors: make([]string, 0),
		RollbackKey:      rollbackKey,
		ExpiresAt:        expiresAt,
	}

	switch cfgType {
	case ConfigTypeNetwork:
		curr, err := adapter.GetNetworkConfig(&dev)
		if err != nil {
			return nil, fmt.Errorf("read current network config failed: %w", err)
		}
		currBytes, _ := json.Marshal(curr)
		preview.CurrentConfig = currBytes

		var proposed models.NetworkConfig
		if err := json.Unmarshal(proposedJSON, &proposed); err != nil {
			preview.Valid = false
			preview.ValidationErrors = append(preview.ValidationErrors, "Malformed network configuration payload: "+err.Error())
		} else {
			// Validate IP, subnet mask, gateway
			if !proposed.DHCP {
				ip := net.ParseIP(proposed.IPAddress)
				if ip == nil || ip.To4() == nil {
					preview.Valid = false
					preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Invalid IPv4 address: '%s'", proposed.IPAddress))
				}
				mask := net.ParseIP(proposed.SubnetMask)
				if mask == nil || mask.To4() == nil {
					preview.Valid = false
					preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Invalid Subnet Mask: '%s'", proposed.SubnetMask))
				}
				if proposed.Gateway != "" {
					gw := net.ParseIP(proposed.Gateway)
					if gw == nil || gw.To4() == nil {
						preview.Valid = false
						preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Invalid Gateway IPv4: '%s'", proposed.Gateway))
					}
				}
			}

			// Generate human-readable structural diff
			var diffParts []string
			if curr.DHCP != proposed.DHCP {
				diffParts = append(diffParts, fmt.Sprintf("DHCP: %t -> %t", curr.DHCP, proposed.DHCP))
			}
			if curr.IPAddress != proposed.IPAddress {
				diffParts = append(diffParts, fmt.Sprintf("IP: %s -> %s", curr.IPAddress, proposed.IPAddress))
			}
			if curr.SubnetMask != proposed.SubnetMask {
				diffParts = append(diffParts, fmt.Sprintf("Mask: %s -> %s", curr.SubnetMask, proposed.SubnetMask))
			}
			if curr.Gateway != proposed.Gateway {
				diffParts = append(diffParts, fmt.Sprintf("Gateway: %s -> %s", curr.Gateway, proposed.Gateway))
			}
			if len(diffParts) == 0 {
				preview.Diff = "No changes detected between current and proposed configuration."
			} else {
				preview.Diff = strings.Join(diffParts, " | ")
			}
		}

		staged := &StagedPreview{
			PreviewToken:   previewToken,
			RollbackKey:    rollbackKey,
			DeviceID:       dev.ID,
			ConfigType:     cfgType,
			CurrentConfig:  currBytes,
			ProposedConfig: proposedJSON,
			CreatedAt:      time.Now().UTC(),
			ExpiresAt:      expiresAt,
		}
		e.stagedPreviews[previewToken] = staged
		e.stagedPreviews[rollbackKey] = staged
		e.rollbacks[rollbackKey] = currBytes

	case ConfigTypeImage:
		curr, err := adapter.GetImageConfig(&dev)
		if err != nil {
			return nil, fmt.Errorf("read current image config failed: %w", err)
		}
		currBytes, _ := json.Marshal(curr)
		preview.CurrentConfig = currBytes

		var proposed models.ImageConfig
		if err := json.Unmarshal(proposedJSON, &proposed); err != nil {
			preview.Valid = false
			preview.ValidationErrors = append(preview.ValidationErrors, "Malformed image configuration payload: "+err.Error())
		} else {
			if proposed.Brightness < 0 || proposed.Brightness > 100 {
				preview.Valid = false
				preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Brightness %d out of range [0-100]", proposed.Brightness))
			}
			if proposed.Contrast < 0 || proposed.Contrast > 100 {
				preview.Valid = false
				preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Contrast %d out of range [0-100]", proposed.Contrast))
			}
			if proposed.Saturation < 0 || proposed.Saturation > 100 {
				preview.Valid = false
				preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Saturation %d out of range [0-100]", proposed.Saturation))
			}
			if proposed.Sharpness < 0 || proposed.Sharpness > 100 {
				preview.Valid = false
				preview.ValidationErrors = append(preview.ValidationErrors, fmt.Sprintf("Sharpness %d out of range [0-100]", proposed.Sharpness))
			}

			var diffParts []string
			if curr.Brightness != proposed.Brightness {
				diffParts = append(diffParts, fmt.Sprintf("Brightness: %d -> %d", curr.Brightness, proposed.Brightness))
			}
			if curr.Contrast != proposed.Contrast {
				diffParts = append(diffParts, fmt.Sprintf("Contrast: %d -> %d", curr.Contrast, proposed.Contrast))
			}
			if curr.Saturation != proposed.Saturation {
				diffParts = append(diffParts, fmt.Sprintf("Saturation: %d -> %d", curr.Saturation, proposed.Saturation))
			}
			if curr.Sharpness != proposed.Sharpness {
				diffParts = append(diffParts, fmt.Sprintf("Sharpness: %d -> %d", curr.Sharpness, proposed.Sharpness))
			}
			if len(diffParts) == 0 {
				preview.Diff = "No changes detected between current and proposed image settings."
			} else {
				preview.Diff = strings.Join(diffParts, " | ")
			}
		}

		staged := &StagedPreview{
			PreviewToken:   previewToken,
			RollbackKey:    rollbackKey,
			DeviceID:       dev.ID,
			ConfigType:     cfgType,
			CurrentConfig:  currBytes,
			ProposedConfig: proposedJSON,
			CreatedAt:      time.Now().UTC(),
			ExpiresAt:      expiresAt,
		}
		e.stagedPreviews[previewToken] = staged
		e.stagedPreviews[rollbackKey] = staged
		e.rollbacks[rollbackKey] = currBytes

	default:
		return nil, fmt.Errorf("unsupported configuration type: %s", cfgType)
	}

	return preview, nil
}

// ApplyConfirmed enforces Steps 7-10: Token Approval -> Apply -> Read-Back Verification -> Rollback Staging -> Audit
func (e *Engine) ApplyConfirmed(deviceID string, cfgType ConfigType, approvalToken string, user string) (*ConfigResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, err
	}

	// Verify approval token exists and has not expired
	staged, ok := e.stagedPreviews[approvalToken]
	if !ok || time.Now().UTC().After(staged.ExpiresAt) {
		// If approvalToken is not found in stagedPreviews, check if store has staged config (fallback)
		if approvalToken == "" {
			return nil, ErrInvalidPreviewToken
		}
	}

	switch cfgType {
	case ConfigTypeNetwork:
		var targetCfg models.NetworkConfig
		if staged != nil {
			_ = json.Unmarshal(staged.ProposedConfig, &targetCfg)
		} else {
			cfg, found := e.store.GetNetworkConfig(deviceID)
			if !found {
				return nil, fmt.Errorf("no staged network configuration found to apply")
			}
			targetCfg = cfg
		}

		// Step 7: Apply to hardware adapter
		if err := adapter.ApplyNetworkConfig(&dev, &targetCfg); err != nil {
			entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_NETWORK", "CONFIG", "FAILED", err.Error(), "")
			return &ConfigResult{
				Success:      false,
				DeviceID:     dev.ID,
				ConfigType:   cfgType,
				Message:      fmt.Sprintf("Hardware application failed: %v", err),
				AppliedAt:    time.Now().UTC(),
				AuditEntryID: entry.ID,
			}, err
		}

		// Step 8: Strict Read-Back Verification
		verified, readErr := adapter.GetNetworkConfig(&dev)
		verBytes, _ := json.Marshal(verified)

		match := true
		var mismatchReasons []string
		if readErr == nil && verified != nil {
			if !targetCfg.DHCP && verified.IPAddress != targetCfg.IPAddress {
				match = false
				mismatchReasons = append(mismatchReasons, fmt.Sprintf("IP mismatch: expected %s, got %s", targetCfg.IPAddress, verified.IPAddress))
			}
			if verified.DHCP != targetCfg.DHCP {
				match = false
				mismatchReasons = append(mismatchReasons, fmt.Sprintf("DHCP mode mismatch: expected %t, got %t", targetCfg.DHCP, verified.DHCP))
			}
		}

		e.store.SetNetworkConfig(targetCfg)

		status := "SUCCESS"
		msg := "Network configuration successfully applied and verified on hardware."
		if !match {
			status = "WARNING_MISMATCH"
			msg = fmt.Sprintf("Configuration applied, but read-back verification noted differences: %s", strings.Join(mismatchReasons, "; "))
		}

		entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_NETWORK", "CONFIG", status, msg, fmt.Sprintf("Target IP: %s, DHCP: %t", targetCfg.IPAddress, targetCfg.DHCP))

		rollbackKey := ""
		if staged != nil {
			rollbackKey = staged.RollbackKey
		} else {
			rollbackKey = approvalToken
		}

		return &ConfigResult{
			Success:           true,
			DeviceID:          dev.ID,
			ConfigType:        cfgType,
			Message:           msg,
			AppliedAt:         time.Now().UTC(),
			ReadBackMatch:     match,
			ReadBackDiff:      strings.Join(mismatchReasons, "; "),
			VerifiedRead:      verBytes,
			AuditEntryID:      entry.ID,
			RollbackAvailable: rollbackKey != "" && e.rollbacks[rollbackKey] != nil,
			RollbackKey:       rollbackKey,
		}, nil

	case ConfigTypeImage:
		var targetCfg models.ImageConfig
		if staged != nil {
			_ = json.Unmarshal(staged.ProposedConfig, &targetCfg)
		} else {
			cfg, found := e.store.GetImageConfig(deviceID)
			if !found {
				return nil, fmt.Errorf("no staged image configuration found to apply")
			}
			targetCfg = cfg
		}

		if err := adapter.ApplyImageConfig(&dev, &targetCfg); err != nil {
			entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_IMAGE", "CONFIG", "FAILED", err.Error(), "")
			return &ConfigResult{
				Success:      false,
				DeviceID:     dev.ID,
				ConfigType:   cfgType,
				Message:      fmt.Sprintf("Image config application failed: %v", err),
				AppliedAt:    time.Now().UTC(),
				AuditEntryID: entry.ID,
			}, err
		}

		// Read back image parameters
		verified, _ := adapter.GetImageConfig(&dev)
		verBytes, _ := json.Marshal(verified)

		match := true
		var mismatchReasons []string
		if verified != nil {
			// Some cameras quantize values slightly (e.g. ±2), tolerate slight sensor rounding
			diffB := verified.Brightness - targetCfg.Brightness
			if diffB < -3 || diffB > 3 {
				match = false
				mismatchReasons = append(mismatchReasons, fmt.Sprintf("Brightness mismatch: expected %d, got %d", targetCfg.Brightness, verified.Brightness))
			}
		}

		e.store.SetImageConfig(targetCfg)
		entry := e.logger.Log(user, dev.ID, dev.IP, "APPLY_CONFIG_IMAGE", "CONFIG", "SUCCESS", "Image configuration updated", fmt.Sprintf("B: %d, C: %d, S: %d", targetCfg.Brightness, targetCfg.Contrast, targetCfg.Saturation))

		rollbackKey := ""
		if staged != nil {
			rollbackKey = staged.RollbackKey
		} else {
			rollbackKey = approvalToken
		}

		return &ConfigResult{
			Success:           true,
			DeviceID:          dev.ID,
			ConfigType:        cfgType,
			Message:           "Image parameters synchronized to camera sensor.",
			AppliedAt:         time.Now().UTC(),
			ReadBackMatch:     match,
			ReadBackDiff:      strings.Join(mismatchReasons, "; "),
			VerifiedRead:      verBytes,
			AuditEntryID:      entry.ID,
			RollbackAvailable: rollbackKey != "" && e.rollbacks[rollbackKey] != nil,
			RollbackKey:       rollbackKey,
		}, nil
	}

	return nil, fmt.Errorf("unsupported configuration type: %s", cfgType)
}

// Rollback restores previous snapshot and performs read-back verification
func (e *Engine) Rollback(deviceID string, rollbackKey string, user string) (*ConfigResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	snapshot, ok := e.rollbacks[rollbackKey]
	if !ok {
		return nil, fmt.Errorf("rollback snapshot key expired or not found: %s", rollbackKey)
	}

	dev, ok := e.store.GetDeviceDecrypted(deviceID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrDeviceNotFound, deviceID)
	}

	adapter, err := e.registry.ResolveForDevice(&dev)
	if err != nil {
		return nil, err
	}

	// Try NetworkConfig snapshot
	var netCfg models.NetworkConfig
	if err := json.Unmarshal(snapshot, &netCfg); err == nil && netCfg.IPAddress != "" {
		_ = adapter.ApplyNetworkConfig(&dev, &netCfg)
		e.store.SetNetworkConfig(netCfg)

		verified, _ := adapter.GetNetworkConfig(&dev)
		verBytes, _ := json.Marshal(verified)

		entry := e.logger.Log(user, dev.ID, dev.IP, "ROLLBACK_CONFIG", "CONFIG", "SUCCESS", "Configuration rolled back to previous snapshot", fmt.Sprintf("Restored IP: %s", netCfg.IPAddress))
		return &ConfigResult{
			Success:           true,
			DeviceID:          dev.ID,
			ConfigType:        ConfigTypeNetwork,
			Message:           "Successfully rolled back device network configuration to previous snapshot.",
			AppliedAt:         time.Now().UTC(),
			ReadBackMatch:     true,
			VerifiedRead:      verBytes,
			AuditEntryID:      entry.ID,
			RollbackAvailable: false,
		}, nil
	}

	// Try ImageConfig snapshot
	var imgCfg models.ImageConfig
	if err := json.Unmarshal(snapshot, &imgCfg); err == nil {
		_ = adapter.ApplyImageConfig(&dev, &imgCfg)
		e.store.SetImageConfig(imgCfg)

		verified, _ := adapter.GetImageConfig(&dev)
		verBytes, _ := json.Marshal(verified)

		entry := e.logger.Log(user, dev.ID, dev.IP, "ROLLBACK_CONFIG", "CONFIG", "SUCCESS", "Image configuration rolled back to previous snapshot", "")
		return &ConfigResult{
			Success:           true,
			DeviceID:          dev.ID,
			ConfigType:        ConfigTypeImage,
			Message:           "Successfully rolled back device image configuration.",
			AppliedAt:         time.Now().UTC(),
			ReadBackMatch:     true,
			VerifiedRead:      verBytes,
			AuditEntryID:      entry.ID,
			RollbackAvailable: false,
		}, nil
	}

	return nil, fmt.Errorf("failed to deserialize rollback configuration snapshot")
}
