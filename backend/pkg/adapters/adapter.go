package adapters

import (
	"errors"
	"fmt"
	"sync"

	"cctv-sentinel/pkg/models"
)

var (
	ErrUnsupportedOperation = errors.New("operation not supported by this device adapter")
	ErrAuthenticationFailed = errors.New("authentication failed: invalid username or password")
	ErrDeviceUnreachable    = errors.New("device unreachable on specified IP and port")
)

// ProtocolAdapter defines the universal contract all CCTV vendor adapters must implement
type ProtocolAdapter interface {
	Name() string
	Vendor() models.Manufacturer
	Detect(targetIP string, port int, username, password string) (*models.Device, error)
	GetNetworkConfig(dev *models.Device) (*models.NetworkConfig, error)
	ApplyNetworkConfig(dev *models.Device, cfg *models.NetworkConfig) error
	GetImageConfig(dev *models.Device) (*models.ImageConfig, error)
	ApplyImageConfig(dev *models.Device, cfg *models.ImageConfig) error
	GetVideoEncodingConfig(dev *models.Device) ([]models.VideoEncodingConfig, error)
	ApplyVideoEncodingConfig(dev *models.Device, cfgs []models.VideoEncodingConfig) error
	SendPTZ(dev *models.Device, action models.PTZAction) error
	GetStreamURI(dev *models.Device, streamIndex int) (string, error)
	CaptureSnapshot(dev *models.Device) ([]byte, error)
	GetRecorderChannels(dev *models.Device) ([]models.RecorderChannel, error)
	SetChannelAnalogMode(dev *models.Device, channelID int, signalFormat string) error
}

// Registry manages registered vendor protocol adapters
type Registry struct {
	mu       sync.RWMutex
	adapters map[models.Manufacturer]ProtocolAdapter
	byProto  map[models.ProtocolType]ProtocolAdapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[models.Manufacturer]ProtocolAdapter),
		byProto:  make(map[models.ProtocolType]ProtocolAdapter),
	}
}

func (r *Registry) Register(adapter ProtocolAdapter, proto models.ProtocolType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[adapter.Vendor()] = adapter
	r.byProto[proto] = adapter
}

func (r *Registry) GetByVendor(vendor models.Manufacturer) (ProtocolAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[vendor]
	return a, ok
}

func (r *Registry) GetByProtocol(proto models.ProtocolType) (ProtocolAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.byProto[proto]
	return a, ok
}

func (r *Registry) ResolveForDevice(dev *models.Device) (ProtocolAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try vendor specific adapter first
	if a, ok := r.adapters[dev.Manufacturer]; ok {
		return a, nil
	}

	// Fallback to ONVIF adapter if device supports it
	for _, p := range dev.Protocols {
		if a, ok := r.byProto[p]; ok {
			return a, nil
		}
	}

	// Try generic ONVIF
	if a, ok := r.adapters[models.VendorGeneric]; ok {
		return a, nil
	}

	return nil, fmt.Errorf("no compatible protocol adapter registered for device %s (%s)", dev.ID, dev.Manufacturer)
}
