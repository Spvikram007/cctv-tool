package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/models"
	"cctv-sentinel/pkg/security"
)

type modelsDevice = models.Device

type Site struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Client    string    `json:"client"`
	Address   string    `json:"address"`
	Contact   string    `json:"contact"`
	CreatedAt time.Time `json:"created_at"`
}

type BatchJob struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	TargetIDs   []string               `json:"target_ids"`
	Action      string                 `json:"action"` // NETWORK_CONFIG, IMAGE_CONFIG, ENCODING_CONFIG, PASSWORD_CHANGE
	Payload     map[string]interface{} `json:"payload"`
	Status      string                 `json:"status"` // PENDING, RUNNING, COMPLETED, FAILED
	Progress    int                    `json:"progress"`
	Logs        []string               `json:"logs"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

type CompatibilityEntry struct {
	Manufacturer Manufacturer `json:"manufacturer"`
	Model        string       `json:"model"`
	Category     string       `json:"category"`
	Protocols    []string     `json:"protocols"`
	PTZ          bool         `json:"ptz"`
	Audio        bool         `json:"audio"`
	AnalogFormat string       `json:"analog_format,omitempty"` // TVI/CVI/AHD/CVBS
	TestedFW     string       `json:"tested_fw"`
	Notes        string       `json:"notes"`
	Verified     bool         `json:"verified"`
}

type Manufacturer = models.Manufacturer

type Store struct {
	mu                   sync.RWMutex
	filePath             string
	vault                *security.Vault
	SchemaVersion        int                                     `json:"schema_version"`
	Devices              map[string]models.Device                `json:"devices"`
	EncryptedCredentials map[string]string                       `json:"encrypted_credentials"` // device_id -> AES-256-GCM ciphertext
	NetworkCfgs          map[string]models.NetworkConfig         `json:"network_configs"`
	ImageCfgs            map[string]models.ImageConfig           `json:"image_configs"`
	VideoCfgs            map[string][]models.VideoEncodingConfig `json:"video_configs"`
	Channels             map[string][]models.RecorderChannel     `json:"channels"`
	StorageDisks         map[string][]models.StorageDevice       `json:"storage_disks"`
	Sites                map[string]Site                         `json:"sites"`
	BatchJobs            map[string]BatchJob                     `json:"batch_jobs"`
	Compatibility        []CompatibilityEntry                    `json:"compatibility"`
}

func NewStore(filePath string) *Store {
	return NewStoreWithVault(filePath, nil)
}

func NewStoreWithVault(filePath string, vault *security.Vault) *Store {
	if vault == nil {
		var keyDir string
		if filePath != "" {
			keyDir = "."
		}
		vault, _ = security.NewVault(keyDir)
	}

	s := &Store{
		filePath:             filePath,
		vault:                vault,
		SchemaVersion:        1,
		Devices:              make(map[string]models.Device),
		EncryptedCredentials: make(map[string]string),
		NetworkCfgs:          make(map[string]models.NetworkConfig),
		ImageCfgs:            make(map[string]models.ImageConfig),
		VideoCfgs:            make(map[string][]models.VideoEncodingConfig),
		Channels:             make(map[string][]models.RecorderChannel),
		StorageDisks:         make(map[string][]models.StorageDevice),
		Sites:                make(map[string]Site),
		BatchJobs:            make(map[string]BatchJob),
		Compatibility:        make([]CompatibilityEntry, 0),
	}

	_ = s.load()
	s.seedCompatibilityDatabase()
	return s
}

func (s *Store) SetVault(vault *security.Vault) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vault = vault
}

func (s *Store) seedCompatibilityDatabase() {
	if len(s.Compatibility) > 0 {
		return
	}
	s.Compatibility = []CompatibilityEntry{
		{Manufacturer: models.VendorHikvision, Model: "DS-2CD2043G2-I", Category: "IP_CAMERA", Protocols: []string{"ISAPI", "ONVIF (Profile S/G/T)", "RTSP"}, PTZ: false, Audio: true, TestedFW: "V5.7.1", Notes: "Full ISAPI network, image, encoding & motion control", Verified: true},
		{Manufacturer: models.VendorHikvision, Model: "DS-7208HUHI-K1", Category: "XVR", Protocols: []string{"ISAPI", "ONVIF", "RTSP"}, PTZ: true, Audio: true, AnalogFormat: "TVI / AHD / CVI / CVBS Hybrid", TestedFW: "V4.25.001", Notes: "Supports 8 analog hybrid + 4 IP channels. UTC coaxial control.", Verified: true},
		{Manufacturer: models.VendorDahua, Model: "DH-IPC-HFW2431S-S-S2", Category: "IP_CAMERA", Protocols: []string{"DAHUA_CGI", "ONVIF (Profile S/T)", "RTSP"}, PTZ: false, Audio: true, TestedFW: "V2.820.0000000.38.R", Notes: "Full configManager.cgi integration, smart H.265+ support", Verified: true},
		{Manufacturer: models.VendorDahua, Model: "DHI-XVR5108HS-4KL-I3", Category: "XVR", Protocols: []string{"DAHUA_CGI", "NetSDK", "RTSP"}, PTZ: true, Audio: true, AnalogFormat: "HDCVI / AHD / TVI / CVBS", TestedFW: "V4.001.0000001.0", Notes: "WizSense 4K hybrid recorder. Coaxial audio & alarm over coax.", Verified: true},
		{Manufacturer: models.VendorCPPlus, Model: "CP-UNC-TA41L3-V2", Category: "IP_CAMERA", Protocols: []string{"ONVIF", "RTSP", "HTTP_CGI"}, PTZ: false, Audio: true, TestedFW: "V2.800.0000000.12.R", Notes: "Indigo series 4MP IP IR bullet camera", Verified: true},
		{Manufacturer: models.VendorCPPlus, Model: "CP-UVR-0801E1-CS", Category: "XVR", Protocols: []string{"ONVIF", "RTSP", "DAHUA_CGI"}, PTZ: true, Audio: true, AnalogFormat: "Cosmic 5-in-1 Auto-detect", TestedFW: "V3.218.0000002.0", Notes: "8 Ch 1080P Cosmic series universal recorder", Verified: true},
		{Manufacturer: models.VendorUniview, Model: "IPC2122LR3-PF40-D", Category: "IP_CAMERA", Protocols: []string{"ONVIF (Profile S/T)", "RTSP", "UNIVIEW_API"}, PTZ: false, Audio: false, TestedFW: "B3107P35", Notes: "LightHunter Ultra 265 network camera", Verified: true},
		{Manufacturer: models.VendorAxis, Model: "M3065-V", Category: "IP_CAMERA", Protocols: []string{"AXIS_VAPIX", "ONVIF", "RTSP"}, PTZ: true, Audio: false, TestedFW: "10.12.204", Notes: "VAPIX HTTP API with Zipstream technology", Verified: true},
		{Manufacturer: models.VendorGeneric, Model: "ONVIF Profile S Compliance", Category: "IP_CAMERA", Protocols: []string{"ONVIF", "RTSP"}, PTZ: true, Audio: true, TestedFW: "Universal", Notes: "Verified cross-vendor WS-Discovery & DeviceManagement", Verified: true},
	}
}

func (s *Store) load() error {
	if s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	unmarshalErr := json.Unmarshal(data, s)
	if unmarshalErr != nil {
		// Corruption detected in primary file!
		log.Printf("[DATABASE] Warning: Corrupted database detected at %s: %v", s.filePath, unmarshalErr)

		// 1. Preserve corrupted copy for investigation (never delete user data)
		corruptCopy := fmt.Sprintf("%s.corrupted.%d", s.filePath, time.Now().Unix())
		_ = os.WriteFile(corruptCopy, data, 0644)
		log.Printf("[DATABASE] Preserved forensic copy of corrupted database at %s", corruptCopy)

		// 2. Recover from safe .bak file if available
		bakFile := s.filePath + ".bak"
		bakData, bakErr := os.ReadFile(bakFile)
		if bakErr == nil {
			if err := json.Unmarshal(bakData, s); err == nil {
				log.Printf("[DATABASE] Successfully recovered database from backup: %s", bakFile)
				// Restore valid backup as primary file
				_ = os.WriteFile(s.filePath, bakData, 0644)
				_ = s.RunMigrations()
				return nil
			}
		}

		return fmt.Errorf("database corrupted and no valid backup available: %w", unmarshalErr)
	}

	// Run schema migrations if needed
	_ = s.RunMigrations()
	return nil
}

func (s *Store) Save() error {
	if s.filePath == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.filePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary database file: %w", err)
	}

	// Update .bak before replacing primary file
	bakFile := s.filePath + ".bak"
	if _, err := os.Stat(s.filePath); err == nil {
		if origData, err := os.ReadFile(s.filePath); err == nil && len(origData) > 0 {
			_ = os.WriteFile(bakFile, origData, 0644)
		}
	}

	// Atomic replace
	_ = os.Remove(s.filePath)
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("atomic database rename failed: %w", err)
	}

	return nil
}

// Device methods
func (s *Store) UpsertDevice(dev models.Device) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dev.LastSeen.IsZero() {
		dev.LastSeen = time.Now().UTC()
	}

	// Securely encrypt credentials into vault
	if dev.Password != "" && s.vault != nil {
		if enc, err := s.vault.Encrypt(dev.Password); err == nil {
			s.EncryptedCredentials[dev.ID] = enc
			dev.Password = enc
		}
	} else if existingEnc, ok := s.EncryptedCredentials[dev.ID]; ok {
		dev.Password = existingEnc
	}

	s.Devices[dev.ID] = dev
	_ = s.saveLocked()
}

func (s *Store) GetDevice(id string) (models.Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.Devices[id]
	return dev, ok
}

// FindDeviceByMAC locates an enrolled device by hardware MAC address
func (s *Store) FindDeviceByMAC(mac string) (models.Device, bool) {
	if mac == "" {
		return models.Device{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cleanMAC := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(mac, "-", ":"), ".", ":"))
	for _, dev := range s.Devices {
		if strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(dev.MAC, "-", ":"), ".", ":")) == cleanMAC {
			return dev, true
		}
	}
	return models.Device{}, false
}

// FindDeviceByIP locates an enrolled device by current IP address
func (s *Store) FindDeviceByIP(ip string) (models.Device, bool) {
	if ip == "" {
		return models.Device{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, dev := range s.Devices {
		if dev.IP == ip {
			return dev, true
		}
	}
	return models.Device{}, false
}

// UpsertDiscoveredDevice registers discovery results with strict preservation guarantees
func (s *Store) UpsertDiscoveredDevice(dev models.Device, discoveryConfidence int) models.Device {
	s.mu.Lock()
	defer s.mu.Unlock()

	var existingKey string
	var existing models.Device
	var foundExisting bool

	// 1. Check by ID
	if d, ok := s.Devices[dev.ID]; ok {
		existingKey = dev.ID
		existing = d
		foundExisting = true
	}

	// 2. Check by unique MAC (ignore generic / zero / locally administered)
	if !foundExisting && dev.MAC != "" && dev.MAC != "00:00:00:00:00:00" {
		cleanMAC := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(dev.MAC, "-", ":"), ".", ":"))
		for k, d := range s.Devices {
			if strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(d.MAC, "-", ":"), ".", ":")) == cleanMAC {
				existingKey = k
				existing = d
				foundExisting = true
				break
			}
		}
	}

	// 3. Check by IP
	if !foundExisting && dev.IP != "" {
		for k, d := range s.Devices {
			if d.IP == dev.IP {
				existingKey = k
				existing = d
				foundExisting = true
				break
			}
		}
	}

	targetDev := dev
	if foundExisting {
		targetDev.ID = existingKey

		// PRESERVATION RULE 1: Never overwrite authenticated credentials
		if targetDev.Username == "" && existing.Username != "" {
			targetDev.Username = existing.Username
			targetDev.Password = existing.Password
		}
		// PRESERVATION RULE 2: Never overwrite verified SerialNumber with empty discovery data
		if existing.SerialNumber != "" && (targetDev.SerialNumber == "" || strings.HasPrefix(targetDev.SerialNumber, "urn:")) {
			targetDev.SerialNumber = existing.SerialNumber
		}
		// PRESERVATION RULE 3: Never overwrite verified Firmware with empty discovery data
		if existing.Firmware != "" && targetDev.Firmware == "" {
			targetDev.Firmware = existing.Firmware
		}
		// PRESERVATION RULE 4: Weaker discovery evidence must not downgrade verified manufacturer/model
		if existing.Manufacturer != models.VendorGeneric && targetDev.Manufacturer == models.VendorGeneric {
			targetDev.Manufacturer = existing.Manufacturer
			targetDev.Model = existing.Model
			targetDev.Category = existing.Category
		}
		// PRESERVATION RULE 5: Handle changed IP address (DHCP renewal)
		if existing.IP != dev.IP && dev.IP != "" {
			targetDev.IP = dev.IP
			targetDev.Port = dev.Port
		}
		// Preserve SiteID if previously assigned
		if existing.SiteID != "" && targetDev.SiteID == "" {
			targetDev.SiteID = existing.SiteID
		}
	}

	if targetDev.LastSeen.IsZero() {
		targetDev.LastSeen = time.Now().UTC()
	}

	// Encrypt credentials if newly provided
	if targetDev.Password != "" && s.vault != nil {
		if enc, err := s.vault.Encrypt(targetDev.Password); err == nil {
			s.EncryptedCredentials[targetDev.ID] = enc
			targetDev.Password = enc
		}
	} else if existingEnc, ok := s.EncryptedCredentials[targetDev.ID]; ok {
		targetDev.Password = existingEnc
	}

	s.Devices[targetDev.ID] = targetDev
	_ = s.saveLocked()
	return targetDev
}

// GetDeviceDecrypted returns device with password decrypted via vault for authorized protocol adapters
func (s *Store) GetDeviceDecrypted(id string) (models.Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dev, ok := s.Devices[id]
	if !ok {
		return dev, false
	}
	if encPass, has := s.EncryptedCredentials[id]; has && s.vault != nil {
		if dec, err := s.vault.Decrypt(encPass); err == nil {
			dev.Password = dec
		}
	}
	return dev, true
}

func (s *Store) ListDevices() []models.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]models.Device, 0, len(s.Devices))
	for _, dev := range s.Devices {
		res = append(res, dev)
	}
	return res
}

func (s *Store) DeleteDevice(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Devices, id)
	delete(s.EncryptedCredentials, id)
	delete(s.NetworkCfgs, id)
	delete(s.ImageCfgs, id)
	delete(s.VideoCfgs, id)
	delete(s.Channels, id)
	delete(s.StorageDisks, id)
	_ = s.saveLocked()
}

// NetworkConfig methods
func (s *Store) SetNetworkConfig(cfg models.NetworkConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.NetworkCfgs[cfg.DeviceID] = cfg
	_ = s.saveLocked()
}

func (s *Store) GetNetworkConfig(deviceID string) (models.NetworkConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.NetworkCfgs[deviceID]
	return cfg, ok
}

// ImageConfig methods
func (s *Store) SetImageConfig(cfg models.ImageConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ImageCfgs[cfg.DeviceID] = cfg
	_ = s.saveLocked()
}

func (s *Store) GetImageConfig(deviceID string) (models.ImageConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.ImageCfgs[deviceID]
	return cfg, ok
}

// VideoEncodingConfig methods
func (s *Store) SetVideoConfigs(deviceID string, cfgs []models.VideoEncodingConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VideoCfgs[deviceID] = cfgs
	_ = s.saveLocked()
}

func (s *Store) GetVideoConfigs(deviceID string) ([]models.VideoEncodingConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfgs, ok := s.VideoCfgs[deviceID]
	return cfgs, ok
}

// Channels methods
func (s *Store) SetChannels(deviceID string, channels []models.RecorderChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Channels[deviceID] = channels
	_ = s.saveLocked()
}

func (s *Store) GetChannels(deviceID string) ([]models.RecorderChannel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chans, ok := s.Channels[deviceID]
	return chans, ok
}

// StorageDisks methods
func (s *Store) SetStorageDisks(deviceID string, disks []models.StorageDevice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StorageDisks[deviceID] = disks
	_ = s.saveLocked()
}

func (s *Store) GetStorageDisks(deviceID string) ([]models.StorageDevice, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	disks, ok := s.StorageDisks[deviceID]
	return disks, ok
}

// Sites
func (s *Store) AddSite(site Site) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sites[site.ID] = site
	_ = s.saveLocked()
}

func (s *Store) ListSites() []Site {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]Site, 0, len(s.Sites))
	for _, site := range s.Sites {
		res = append(res, site)
	}
	return res
}

// Batch jobs
func (s *Store) AddBatchJob(job BatchJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchJobs[job.ID] = job
	_ = s.saveLocked()
}

func (s *Store) UpdateBatchJob(job BatchJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BatchJobs[job.ID] = job
	_ = s.saveLocked()
}

func (s *Store) ListBatchJobs() []BatchJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]BatchJob, 0, len(s.BatchJobs))
	for _, job := range s.BatchJobs {
		res = append(res, job)
	}
	return res
}

// Compatibility
func (s *Store) GetCompatibility() []CompatibilityEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]CompatibilityEntry, len(s.Compatibility))
	copy(res, s.Compatibility)
	return res
}
