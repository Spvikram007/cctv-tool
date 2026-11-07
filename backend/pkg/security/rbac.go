package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrUnauthorized   = errors.New("security: authentication required or invalid credentials")
	ErrForbidden      = errors.New("security: role lacks required permission for this operation")
	ErrUserNotFound   = errors.New("security: user account not found")
	ErrUserDisabled   = errors.New("security: user account is deactivated")
	ErrSessionExpired = errors.New("security: session token expired; re-authentication required")
)

type Role string

const (
	RoleAdmin      Role = "ADMIN"
	RoleTechnician Role = "TECHNICIAN"
	RoleViewer     Role = "VIEWER"
	RoleAuditor    Role = "AUDITOR"
)

type Permission string

const (
	PermViewLive        Permission = "VIEW_LIVE"
	PermDeviceConfig    Permission = "DEVICE_CONFIG"
	PermBatchExecute    Permission = "BATCH_EXECUTE"
	PermReboot          Permission = "REBOOT"
	PermFirmwareUpgrade Permission = "FIRMWARE_UPGRADE"
	PermAuditView       Permission = "AUDIT_VIEW"
	PermUserManagement  Permission = "USER_MANAGEMENT"
	PermLicenseManage   Permission = "LICENSE_MANAGE"
	PermRecorderManage  Permission = "RECORDER_MANAGE"
	PermPlaybackView    Permission = "PLAYBACK_VIEW"
	PermPlaybackExport  Permission = "PLAYBACK_EXPORT"
)

// RolePermissions maps roles to granted enterprise capabilities
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermViewLive,
		PermDeviceConfig,
		PermBatchExecute,
		PermReboot,
		PermFirmwareUpgrade,
		PermAuditView,
		PermUserManagement,
		PermLicenseManage,
		PermRecorderManage,
		PermPlaybackView,
		PermPlaybackExport,
	},
	RoleTechnician: {
		PermViewLive,
		PermDeviceConfig,
		PermBatchExecute,
		PermReboot,
		PermAuditView,
		PermRecorderManage,
		PermPlaybackView,
		PermPlaybackExport,
	},
	RoleAuditor: {
		PermViewLive,
		PermAuditView,
		PermPlaybackView,
	},
	RoleViewer: {
		PermViewLive,
		PermPlaybackView,
	},
}

// Argon2Params defines tunable memory-hard password hashing parameters (RFC 9106)
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params configured for enterprise security
var DefaultArgon2Params = &Argon2Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// UserAccount represents an authenticated enterprise operator
type UserAccount struct {
	Username     string    `json:"username"`
	FullName     string    `json:"full_name"`
	Role         Role      `json:"role"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Argon2id or legacy salted hash
	Salt         string    `json:"-"`
	TenantID     string    `json:"tenant_id,omitempty"`
	SiteID       string    `json:"site_id,omitempty"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login,omitempty"`
}

// Session represents an active authenticated session
type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	TenantID  string    `json:"tenant_id,omitempty"`
	SiteID    string    `json:"site_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RBACManager handles user authentication, role checks, and session tokens
type RBACManager struct {
	mu           sync.RWMutex
	users        map[string]*UserAccount
	sessions     map[string]*Session
	argon2Config *Argon2Params
}

func NewRBACManager() *RBACManager {
	mgr := &RBACManager{
		users:        make(map[string]*UserAccount),
		sessions:     make(map[string]*Session),
		argon2Config: DefaultArgon2Params,
	}

	// Seed default enterprise operators with Argon2id
	_ = mgr.CreateUser("admin", "Admin@Sentinel2026!", "System Administrator", RoleAdmin, "admin@techdefence.in", "default", "hq")
	_ = mgr.CreateUser("technician", "Tech@Sentinel2026!", "Field Technician", RoleTechnician, "tech@techdefence.in", "default", "site-1")
	_ = mgr.CreateUser("auditor", "Audit@Sentinel2026!", "Compliance Officer", RoleAuditor, "audit@techdefence.in", "default", "hq")
	_ = mgr.CreateUser("viewer", "View@Sentinel2026!", "Control Room Operator", RoleViewer, "view@techdefence.in", "default", "site-1")

	return mgr
}

// SetArgon2Params updates the hashing parameters dynamically
func (m *RBACManager) SetArgon2Params(params *Argon2Params) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if params != nil {
		m.argon2Config = params
	}
}

// HashPasswordArgon2id produces a standard RFC 9106 Argon2id hash string
func HashPasswordArgon2id(password string, saltBytes []byte, p *Argon2Params) string {
	if p == nil {
		p = DefaultArgon2Params
	}
	hash := argon2.IDKey([]byte(password), saltBytes, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	saltHex := hex.EncodeToString(saltBytes)
	hashHex := hex.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", p.Memory, p.Iterations, p.Parallelism, saltHex, hashHex)
}

// legacyHashPassword preserves support for verification of pre-Phase 6 salted SHA-256 passwords
func legacyHashPassword(password, salt string) string {
	combined := fmt.Sprintf("%s:%s:CCTV_SENTINEL_SALT_PEPPER", salt, password)
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

// CreateUser registers a new operator with Argon2id password hashing
func (m *RBACManager) CreateUser(username, password, fullName string, role Role, email, tenantID, siteID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[username]; exists {
		return fmt.Errorf("user %s already exists", username)
	}

	saltBytes := make([]byte, m.argon2Config.SaltLength)
	if _, err := rand.Read(saltBytes); err != nil {
		return fmt.Errorf("failed to generate cryptographic salt: %w", err)
	}

	argonHash := HashPasswordArgon2id(password, saltBytes, m.argon2Config)

	u := &UserAccount{
		Username:     username,
		FullName:     fullName,
		Role:         role,
		Email:        email,
		PasswordHash: argonHash,
		Salt:         hex.EncodeToString(saltBytes),
		TenantID:     tenantID,
		SiteID:       siteID,
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}

	m.users[username] = u
	return nil
}

// Authenticate verifies credentials with Argon2id and automatically rehashes legacy passwords
func (m *RBACManager) Authenticate(username, password string, ttl time.Duration) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[username]
	if !exists {
		return nil, ErrUnauthorized
	}
	if !user.Active {
		return nil, ErrUserDisabled
	}

	authenticated := false

	// Check if stored hash is Argon2id
	if strings.HasPrefix(user.PasswordHash, "$argon2id$") {
		parts := strings.Split(user.PasswordHash, "$")
		// Format: ["", "argon2id", "v=19", "m=...,t=...,p=...", "saltHex", "hashHex"]
		if len(parts) == 6 {
			var mem, iter uint32
			var par uint64
			for _, param := range strings.Split(parts[3], ",") {
				kv := strings.Split(param, "=")
				if len(kv) == 2 {
					switch kv[0] {
					case "m":
						v, _ := strconv.ParseUint(kv[1], 10, 32)
						mem = uint32(v)
					case "t":
						v, _ := strconv.ParseUint(kv[1], 10, 32)
						iter = uint32(v)
					case "p":
						par, _ = strconv.ParseUint(kv[1], 10, 8)
					}
				}
			}
			saltBytes, err1 := hex.DecodeString(parts[4])
			expectedHashBytes, err2 := hex.DecodeString(parts[5])
			if err1 == nil && err2 == nil {
				calculatedHash := argon2.IDKey([]byte(password), saltBytes, iter, mem, uint8(par), uint32(len(expectedHashBytes)))
				if subtle.ConstantTimeCompare(calculatedHash, expectedHashBytes) == 1 {
					authenticated = true
				}
			}
		}
	} else {
		// Legacy Salted SHA-256 verification
		expectedHash := legacyHashPassword(password, user.Salt)
		if subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(expectedHash)) == 1 {
			authenticated = true
			// Rehash on login to Argon2id
			saltBytes := make([]byte, m.argon2Config.SaltLength)
			_, _ = rand.Read(saltBytes)
			user.PasswordHash = HashPasswordArgon2id(password, saltBytes, m.argon2Config)
			user.Salt = hex.EncodeToString(saltBytes)
		}
	}

	if !authenticated {
		return nil, ErrUnauthorized
	}

	user.LastLogin = time.Now().UTC()

	if ttl <= 0 {
		ttl = 8 * time.Hour
	}

	tokBytes := make([]byte, 24)
	_, _ = rand.Read(tokBytes)
	token := fmt.Sprintf("sent-sess-%s", hex.EncodeToString(tokBytes))

	now := time.Now().UTC()
	sess := &Session{
		Token:     token,
		Username:  user.Username,
		Role:      user.Role,
		TenantID:  user.TenantID,
		SiteID:    user.SiteID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	m.sessions[token] = sess
	return sess, nil
}

// ValidateSession inspects a bearer session token
func (m *RBACManager) ValidateSession(token string) (*Session, error) {
	m.mu.RLock()
	sess, exists := m.sessions[token]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrUnauthorized
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
		return nil, ErrSessionExpired
	}

	return sess, nil
}

// HasPermission checks if a given role is authorized for a specific permission
func HasPermission(role Role, perm Permission) bool {
	perms, ok := RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// ListUsers returns operator accounts without sensitive hashes
func (m *RBACManager) ListUsers() []UserAccount {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]UserAccount, 0, len(m.users))
	for _, u := range m.users {
		safe := *u
		safe.PasswordHash = ""
		safe.Salt = ""
		res = append(res, safe)
	}
	return res
}
