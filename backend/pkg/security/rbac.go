package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
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
	},
	RoleTechnician: {
		PermViewLive,
		PermDeviceConfig,
		PermBatchExecute,
		PermReboot,
		PermAuditView,
	},
	RoleAuditor: {
		PermViewLive,
		PermAuditView,
	},
	RoleViewer: {
		PermViewLive,
	},
}

// UserAccount represents an authenticated enterprise operator
type UserAccount struct {
	Username     string    `json:"username"`
	FullName     string    `json:"full_name"`
	Role         Role      `json:"role"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Salted SHA-256 hash
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
	mu       sync.RWMutex
	users    map[string]*UserAccount
	sessions map[string]*Session
}

func NewRBACManager() *RBACManager {
	mgr := &RBACManager{
		users:    make(map[string]*UserAccount),
		sessions: make(map[string]*Session),
	}

	// Seed default enterprise operators
	_ = mgr.CreateUser("admin", "Admin@Sentinel2026!", "System Administrator", RoleAdmin, "admin@techdefence.in", "default", "hq")
	_ = mgr.CreateUser("technician", "Tech@Sentinel2026!", "Field Technician", RoleTechnician, "tech@techdefence.in", "default", "site-1")
	_ = mgr.CreateUser("auditor", "Audit@Sentinel2026!", "Compliance Officer", RoleAuditor, "audit@techdefence.in", "default", "hq")
	_ = mgr.CreateUser("viewer", "View@Sentinel2026!", "Control Room Operator", RoleViewer, "view@techdefence.in", "default", "site-1")

	return mgr
}

func hashPassword(password, salt string) string {
	combined := fmt.Sprintf("%s:%s:CCTV_SENTINEL_SALT_PEPPER", salt, password)
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

// CreateUser registers a new operator with salted password hashing
func (m *RBACManager) CreateUser(username, password, fullName string, role Role, email, tenantID, siteID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.users[username]; exists {
		return fmt.Errorf("user %s already exists", username)
	}

	saltBytes := make([]byte, 16)
	_, _ = rand.Read(saltBytes)
	salt := hex.EncodeToString(saltBytes)

	u := &UserAccount{
		Username:     username,
		FullName:     fullName,
		Role:         role,
		Email:        email,
		PasswordHash: hashPassword(password, salt),
		Salt:         salt,
		TenantID:     tenantID,
		SiteID:       siteID,
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}

	m.users[username] = u
	return nil
}

// Authenticate verifies username and password, returning an active session
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

	expectedHash := hashPassword(password, user.Salt)
	if subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(expectedHash)) != 1 {
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
