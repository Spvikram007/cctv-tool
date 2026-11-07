package licensing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/models"
)

const (
	TierFree             = "FREE"
	TierTechnicianPro    = "TECHNICIAN_PRO"
	TierDealerEnterprise = "DEALER_ENTERPRISE"
	licenseSalt          = "TECH_DEFENCE_SOLUTIONS_CCTV_SENTINEL_2026_SECURE_SALT"
)

// GSTInvoice represents a commercial tax invoice
type GSTInvoice struct {
	InvoiceNumber string    `json:"invoice_number"`
	InvoiceDate   time.Time `json:"invoice_date"`
	CustomerName  string    `json:"customer_name"`
	CustomerGST   string    `json:"customer_gst"`
	SellerName    string    `json:"seller_name"`
	SellerGST     string    `json:"seller_gst"`
	Tier          string    `json:"tier"`
	HSNCode       string    `json:"hsn_code"` // 998313
	BaseAmount    float64   `json:"base_amount"`
	CGST          float64   `json:"cgst"`     // 9%
	SGST          float64   `json:"sgst"`     // 9%
	IGST          float64   `json:"igst"`     // 18% if inter-state
	TotalAmount   float64   `json:"total_amount"`
	Currency      string    `json:"currency"`
}

// Manager handles licensing and activation
type Manager struct {
	mu          sync.RWMutex
	current     models.License
	machineHash string
}

func NewManager() *Manager {
	m := &Manager{}
	m.machineHash = m.generateMachineHash()
	// Default to Free Edition
	m.current = models.License{
		Tier:        TierFree,
		LicenseKey:  "COMMUNITY-FREE-EDITION",
		OwnerName:   "Community Technician",
		Company:     "Independent Operator",
		MaxDevices:  8,
		MaxSites:    1,
		IssuedAt:    time.Now().AddDate(-1, 0, 0),
		ExpiresAt:   time.Now().AddDate(10, 0, 0),
		Active:      true,
		MachineHash: m.machineHash,
	}
	return m
}

func (m *Manager) generateMachineHash() string {
	hostname, _ := os.Hostname()
	macs := make([]string, 0)
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.HardwareAddr != nil && len(iface.HardwareAddr) > 0 {
				macs = append(macs, iface.HardwareAddr.String())
			}
		}
	}
	raw := fmt.Sprintf("%s|%s", hostname, strings.Join(macs, ","))
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:16])
}

func (m *Manager) GetMachineHash() string {
	return m.machineHash
}

func (m *Manager) GetLicense() models.License {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// GenerateActivationCode creates an offline cryptographically signed key for a given machine and tier
func (m *Manager) GenerateActivationCode(machineHash, tier, company string, daysValid int) string {
	expiry := time.Now().AddDate(0, 0, daysValid).Format("20060102")
	payload := fmt.Sprintf("%s|%s|%s|%s", machineHash, tier, company, expiry)
	mac := hmac.New(sha256.New, []byte(licenseSalt))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))[:16]
	return fmt.Sprintf("SENTINEL-%s-%s-%s", tier, expiry, signature)
}

// ActivateLicense validates and installs an offline or online activation key
func (m *Manager) ActivateLicense(key, company, gst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	parts := strings.Split(key, "-")
	if len(parts) < 4 || parts[0] != "SENTINEL" {
		return fmt.Errorf("invalid license key format")
	}

	tier := parts[1]
	expiryStr := parts[2]
	signature := parts[3]

	// Verify expiry
	expiry, err := time.Parse("20060102", expiryStr)
	if err != nil {
		return fmt.Errorf("malformed license expiry date: %w", err)
	}
	if time.Now().After(expiry) {
		return fmt.Errorf("license key has expired on %s", expiryStr)
	}

	// Verify cryptographic signature against current machine hash
	payload := fmt.Sprintf("%s|%s|%s|%s", m.machineHash, tier, company, expiryStr)
	mac := hmac.New(sha256.New, []byte(licenseSalt))
	mac.Write([]byte(payload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))[:16]

	if signature != expectedSig {
		return fmt.Errorf("license verification failed: invalid signature for this machine hardware ID")
	}

	maxDevs := 8
	maxSites := 1
	if tier == TierTechnicianPro {
		maxDevs = 64
		maxSites = 10
	} else if tier == TierDealerEnterprise {
		maxDevs = 999999
		maxSites = 999999
	}

	m.current = models.License{
		Tier:        tier,
		LicenseKey:  key,
		OwnerName:   company,
		Company:     company,
		GSTNumber:   gst,
		MaxDevices:  maxDevs,
		MaxSites:    maxSites,
		IssuedAt:    time.Now(),
		ExpiresAt:   expiry,
		Active:      true,
		MachineHash: m.machineHash,
		OfflineKey:  key,
	}

	return nil
}

// GenerateGSTInvoice creates a compliant invoice for licensing purchases
func (m *Manager) GenerateGSTInvoice(customerName, customerGST, state string, tier string) GSTInvoice {
	basePrice := 0.0
	switch tier {
	case TierTechnicianPro:
		basePrice = 4999.00
	case TierDealerEnterprise:
		basePrice = 14999.00
	default:
		basePrice = 0.00
	}

	inv := GSTInvoice{
		InvoiceNumber: fmt.Sprintf("INV-TDS-%d", time.Now().Unix()),
		InvoiceDate:   time.Now(),
		CustomerName:  customerName,
		CustomerGST:   customerGST,
		SellerName:    "Tech Defence Solutions Pvt Ltd",
		SellerGST:     "29AABCT1337Q1ZS",
		Tier:          tier,
		HSNCode:       "998313", // Information technology software services
		BaseAmount:    basePrice,
		Currency:      "INR",
	}

	// If Karnataka (same state as seller 29), split CGST 9% + SGST 9%, otherwise IGST 18%
	if strings.HasPrefix(customerGST, "29") || strings.EqualFold(state, "Karnataka") {
		inv.CGST = basePrice * 0.09
		inv.SGST = basePrice * 0.09
		inv.IGST = 0.0
	} else {
		inv.CGST = 0.0
		inv.SGST = 0.0
		inv.IGST = basePrice * 0.18
	}
	inv.TotalAmount = inv.BaseAmount + inv.CGST + inv.SGST + inv.IGST
	return inv
}
