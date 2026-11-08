package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

var (
	ErrUnauthorized       = errors.New("security: authentication required or invalid credentials")
	ErrForbidden          = errors.New("security: role lacks required permission for this operation")
	ErrUserNotFound       = errors.New("security: user account not found")
	ErrUserDisabled       = errors.New("security: user account is deactivated")
	ErrSessionExpired     = errors.New("security: session token expired; re-authentication required")
	ErrSetupDisabled      = errors.New("security: initial admin setup is disabled; system is already provisioned")
	ErrSetupTokenExpired  = errors.New("security: initial setup token has expired")
	ErrInvalidSetupToken  = errors.New("security: invalid setup token")
	ErrWeakPassword       = errors.New("security: password does not meet enterprise complexity requirements")
	ErrInvalidArgonParams = errors.New("security: stored Argon2id parameters exceed safe execution limits or are malformed")
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
	PermPTZControl      Permission = "PTZ_CONTROL"
	PermPTZManage       Permission = "PTZ_MANAGE"
	PermEventManage     Permission = "EVENT_MANAGE"
	PermAlarmManage     Permission = "ALARM_MANAGE"
	PermAlarmExport     Permission = "ALARM_EXPORT"
)

// Security Scopes for Global Fleet Administration
const (
	ScopeGlobalAdmin = "global:admin"
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
		PermPTZControl,
		PermPTZManage,
		PermEventManage,
		PermAlarmManage,
		PermAlarmExport,
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
		PermPTZControl,
		PermPTZManage,
		PermEventManage,
		PermAlarmManage,
		PermAlarmExport,
	},
	RoleAuditor: {
		PermViewLive,
		PermAuditView,
		PermPlaybackView,
		PermPlaybackExport,
		PermAlarmManage,
		PermAlarmExport,
	},
	RoleViewer: {
		PermViewLive,
		PermPlaybackView,
	},
}

// Argon2 Bounds for DoS and Resource Exhaustion Prevention
const (
	MinArgonMemory      = 16 * 1024  // 16 MB
	MaxArgonMemory      = 512 * 1024 // 512 MB
	MinArgonIterations  = 1
	MaxArgonIterations  = 10
	MinArgonParallelism = 1
	MaxArgonParallelism = 16
	MinSaltLength       = 16
	MaxSaltLength       = 64
	MinHashLength       = 16
	MaxHashLength       = 64
)

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
	Username      string    `json:"username"`
	FullName      string    `json:"full_name"`
	Role          Role      `json:"role"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"-"` // Argon2id or legacy salted hash
	Salt          string    `json:"-"`
	TenantID      string    `json:"tenant_id,omitempty"`
	SiteID        string    `json:"site_id,omitempty"`
	IsGlobalAdmin bool      `json:"is_global_admin,omitempty"`
	Scopes        []string  `json:"scopes,omitempty"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
	LastLogin     time.Time `json:"last_login,omitempty"`
}

// HasScope checks if the user account holds an explicit security scope
func (u *UserAccount) HasScope(scope string) bool {
	if u == nil {
		return false
	}
	if u.IsGlobalAdmin && scope == ScopeGlobalAdmin {
		return true
	}
	for _, s := range u.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Session represents an active authenticated session
type Session struct {
	Token         string    `json:"token,omitempty"`
	Username      string    `json:"username"`
	Role          Role      `json:"role"`
	TenantID      string    `json:"tenant_id,omitempty"`
	SiteID        string    `json:"site_id,omitempty"`
	IsGlobalAdmin bool      `json:"is_global_admin,omitempty"`
	Scopes        []string  `json:"scopes,omitempty"`
	CSRFToken     string    `json:"csrf_token,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// HasScope checks if the session holds an explicit security scope
func (s *Session) HasScope(scope string) bool {
	if s == nil {
		return false
	}
	if s.IsGlobalAdmin && scope == ScopeGlobalAdmin {
		return true
	}
	for _, sc := range s.Scopes {
		if sc == scope {
			return true
		}
	}
	return false
}

// RBACManager handles user authentication, role checks, session tokens, and first-run setup
type RBACManager struct {
	mu                 sync.RWMutex
	users              map[string]*UserAccount
	sessions           map[string]*Session
	argon2Config       *Argon2Params
	setupMode          bool
	setupToken         string
	setupTokenExpires  time.Time
	setupTokenFilePath string
}

// ValidatePasswordStrength enforces enterprise password complexity
func ValidatePasswordStrength(password string) error {
	if len(password) < 10 {
		return fmt.Errorf("%w: password must be at least 10 characters long", ErrWeakPassword)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return fmt.Errorf("%w: password must include uppercase, lowercase, number, and special character", ErrWeakPassword)
	}
	return nil
}

// NewRBACManager initializes the RBAC manager with zero hardcoded default credentials
func NewRBACManager() *RBACManager {
	mgr := &RBACManager{
		users:              make(map[string]*UserAccount),
		sessions:           make(map[string]*Session),
		argon2Config:       DefaultArgon2Params,
		setupTokenFilePath: "sentinel-setup.token",
	}

	// First-run setup initialization: NO HARDCODED DEFAULT CREDENTIALS
	mgr.initSetupToken()

	return mgr
}

// initSetupToken creates a cryptographically secure one-time token for first-run admin provisioning
func (m *RBACManager) initSetupToken() {
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		// Fail securely: panic rather than proceeding without genuine randomness
		panic(fmt.Sprintf("CRITICAL CRYPTOGRAPHIC FAILURE: crypto/rand unavailable: %v", err))
	}

	m.setupMode = true
	m.setupToken = fmt.Sprintf("sent-setup-%s", hex.EncodeToString(tokBytes))
	m.setupTokenExpires = time.Now().UTC().Add(24 * time.Hour)

	// Securely persist to local token file with restricted permissions (0600)
	if err := os.WriteFile(m.setupTokenFilePath, []byte(m.setupToken+"\n"), 0600); err != nil {
		fmt.Printf("[SECURITY] Notice: could not write setup token file (%v)\n", err)
	}
}

// RevokeSetupToken immediately invalidates any active setup token and purges the token file
func (m *RBACManager) RevokeSetupToken() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setupToken = ""
	m.setupTokenExpires = time.Time{}
	if m.setupTokenFilePath != "" {
		_ = os.Remove(m.setupTokenFilePath)
	}
}

// RegenerateSetupToken invalidates any existing setup token and issues a new cryptographically random single-use token
func (m *RBACManager) RegenerateSetupToken() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.setupMode || len(m.users) > 0 {
		return "", ErrSetupDisabled
	}

	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		return "", fmt.Errorf("cryptographic failure: crypto/rand unavailable: %w", err)
	}

	m.setupToken = fmt.Sprintf("sent-setup-%s", hex.EncodeToString(tokBytes))
	m.setupTokenExpires = time.Now().UTC().Add(24 * time.Hour)

	if err := os.WriteFile(m.setupTokenFilePath, []byte(m.setupToken+"\n"), 0600); err != nil {
		return "", fmt.Errorf("failed to write setup token file: %w", err)
	}

	return m.setupToken, nil
}

// SetSetupTokenFilePath allows configuring the token file destination for isolated tests
func (m *RBACManager) SetSetupTokenFilePath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setupTokenFilePath = path
}

// IsSetupRequired checks if initial administrator setup is still pending
func (m *RBACManager) IsSetupRequired() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setupMode && len(m.users) == 0
}

// GetSetupToken returns the current setup token (accessible only on local console/tests)
func (m *RBACManager) GetSetupToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.setupMode {
		return ""
	}
	return m.setupToken
}

// CompleteSetup provisions the initial system administrator and permanently destroys the setup token
func (m *RBACManager) CompleteSetup(token, username, password, fullName, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.setupMode || len(m.users) > 0 {
		return ErrSetupDisabled
	}

	if m.setupToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(m.setupToken)) != 1 {
		return ErrInvalidSetupToken
	}

	if time.Now().UTC().After(m.setupTokenExpires) {
		return ErrSetupTokenExpired
	}

	if strings.TrimSpace(username) == "" {
		return errors.New("username cannot be empty")
	}

	if err := ValidatePasswordStrength(password); err != nil {
		return err
	}

	saltBytes := make([]byte, m.argon2Config.SaltLength)
	if _, err := rand.Read(saltBytes); err != nil {
		return fmt.Errorf("failed to generate cryptographic salt: %w", err)
	}

	argonHash := HashPasswordArgon2id(password, saltBytes, m.argon2Config)

	u := &UserAccount{
		Username:      username,
		FullName:      fullName,
		Role:          RoleAdmin,
		Email:         email,
		PasswordHash:  argonHash,
		Salt:          hex.EncodeToString(saltBytes),
		TenantID:      "system",
		SiteID:        "hq",
		IsGlobalAdmin: true,
		Scopes:        []string{ScopeGlobalAdmin},
		Active:        true,
		CreatedAt:     time.Now().UTC(),
	}

	m.users[username] = u

	// Permanently disable setup mode and erase token
	m.setupMode = false
	m.setupToken = ""
	_ = os.Remove(m.setupTokenFilePath)

	return nil
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
	if err := ValidatePasswordStrength(password); err != nil {
		return err
	}

	m.mu.Lock()
	if _, exists := m.users[username]; exists {
		m.mu.Unlock()
		return fmt.Errorf("user %s already exists", username)
	}

	saltLength := m.argon2Config.SaltLength
	argonConfig := *m.argon2Config
	m.mu.Unlock()

	saltBytes := make([]byte, saltLength)
	if _, err := rand.Read(saltBytes); err != nil {
		return fmt.Errorf("failed to generate cryptographic salt: %w", err)
	}

	argonHash := HashPasswordArgon2id(password, saltBytes, &argonConfig)

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

	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[username] = u

	// Disable setup mode if an admin was created
	if role == RoleAdmin {
		m.setupMode = false
		m.setupToken = ""
		_ = os.Remove(m.setupTokenFilePath)
	}

	return nil
}

// Authenticate verifies credentials with Argon2id and rehashes legacy passwords without holding global locks during computation
func (m *RBACManager) Authenticate(username, password string, ttl time.Duration) (*Session, error) {
	// 1. Read user record under Read Lock
	m.mu.RLock()
	user, exists := m.users[username]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrUnauthorized
	}
	if !user.Active {
		m.mu.RUnlock()
		return nil, ErrUserDisabled
	}

	storedHash := user.PasswordHash
	storedSalt := user.Salt
	userRole := user.Role
	tenantID := user.TenantID
	siteID := user.SiteID
	m.mu.RUnlock()

	// 2. Perform expensive cryptographic hash calculation OUTSIDE the lock
	authenticated := false
	needsRehash := false
	isArgon := strings.HasPrefix(storedHash, "$argon2id$")

	if isArgon {
		parts := strings.Split(storedHash, "$")
		// Standard format: ["", "argon2id", "v=19", "m=...,t=...,p=...", "saltHex", "hashHex"]
		if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
			return nil, ErrInvalidArgonParams
		}

		var mem, iter uint32
		var par uint64
		for _, param := range strings.Split(parts[3], ",") {
			kv := strings.Split(param, "=")
			if len(kv) == 2 {
				switch kv[0] {
				case "m":
					v, err := strconv.ParseUint(kv[1], 10, 32)
					if err != nil {
						return nil, ErrInvalidArgonParams
					}
					mem = uint32(v)
				case "t":
					v, err := strconv.ParseUint(kv[1], 10, 32)
					if err != nil {
						return nil, ErrInvalidArgonParams
					}
					iter = uint32(v)
				case "p":
					var err error
					par, err = strconv.ParseUint(kv[1], 10, 8)
					if err != nil {
						return nil, ErrInvalidArgonParams
					}
				}
			}
		}

		// Enforce strict parameter bounds to prevent DoS / Memory exhaustion
		if mem < MinArgonMemory || mem > MaxArgonMemory ||
			iter < MinArgonIterations || iter > MaxArgonIterations ||
			par < MinArgonParallelism || par > MaxArgonParallelism {
			return nil, ErrInvalidArgonParams
		}

		saltBytes, err1 := hex.DecodeString(parts[4])
		expectedHashBytes, err2 := hex.DecodeString(parts[5])
		if err1 != nil || err2 != nil ||
			len(saltBytes) < MinSaltLength || len(saltBytes) > MaxSaltLength ||
			len(expectedHashBytes) < MinHashLength || len(expectedHashBytes) > MaxHashLength {
			return nil, ErrInvalidArgonParams
		}

		calculatedHash := argon2.IDKey([]byte(password), saltBytes, iter, mem, uint8(par), uint32(len(expectedHashBytes)))
		if subtle.ConstantTimeCompare(calculatedHash, expectedHashBytes) == 1 {
			authenticated = true
		}
	} else {
		// Legacy Salted SHA-256 verification
		expectedHash := legacyHashPassword(password, storedSalt)
		if subtle.ConstantTimeCompare([]byte(storedHash), []byte(expectedHash)) == 1 {
			authenticated = true
			needsRehash = true
		}
	}

	if !authenticated {
		return nil, ErrUnauthorized
	}

	// 3. Acquire Write Lock briefly only to issue session and rehash if needed
	m.mu.Lock()
	defer m.mu.Unlock()

	currentUser, stillExists := m.users[username]
	if !stillExists || !currentUser.Active {
		return nil, ErrUnauthorized
	}

	if needsRehash {
		saltBytes := make([]byte, m.argon2Config.SaltLength)
		if _, err := rand.Read(saltBytes); err == nil {
			currentUser.PasswordHash = HashPasswordArgon2id(password, saltBytes, m.argon2Config)
			currentUser.Salt = hex.EncodeToString(saltBytes)
		}
	}

	currentUser.LastLogin = time.Now().UTC()

	if ttl <= 0 {
		ttl = 8 * time.Hour
	}

	tokBytes := make([]byte, 24)
	if _, err := rand.Read(tokBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure session token: %w", err)
	}
	token := fmt.Sprintf("sent-sess-%s", hex.EncodeToString(tokBytes))

	csrfBytes := make([]byte, 24)
	if _, err := rand.Read(csrfBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure csrf token: %w", err)
	}
	csrfToken := fmt.Sprintf("sent-csrf-%s", hex.EncodeToString(csrfBytes))

	now := time.Now().UTC()
	sess := &Session{
		Token:         token,
		Username:      username,
		Role:          userRole,
		TenantID:      tenantID,
		SiteID:        siteID,
		IsGlobalAdmin: currentUser.IsGlobalAdmin,
		Scopes:        append([]string(nil), currentUser.Scopes...),
		CSRFToken:     csrfToken,
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttl),
	}

	m.sessions[token] = sess
	return sess, nil
}

// ValidateSession inspects a bearer session token
func (m *RBACManager) ValidateSession(token string) (*Session, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrUnauthorized
	}

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

// RevokeSession explicitly invalidates an active session token
func (m *RBACManager) RevokeSession(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
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

// LoadExistingUsers loads pre-existing accounts from store during migration
func (m *RBACManager) LoadExistingUsers(users []UserAccount) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hasAdmin := false
	for i := range users {
		u := users[i]
		m.users[u.Username] = &u
		if u.Role == RoleAdmin && u.Active {
			hasAdmin = true
		}
	}
	if hasAdmin {
		m.setupMode = false
		m.setupToken = ""
		_ = os.Remove(m.setupTokenFilePath)
	}
}

// InjectUserWithHashForTest injects a user with a raw hash string for testing parameter validation
func (m *RBACManager) InjectUserWithHashForTest(username, hash, salt string, role Role) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[username] = &UserAccount{
		Username:     username,
		FullName:     "Test User",
		Role:         role,
		Email:        username + "@test.local",
		PasswordHash: hash,
		Salt:         salt,
		TenantID:     "default",
		SiteID:       "hq",
		Active:       true,
		CreatedAt:    time.Now().UTC(),
	}
}

// CreateGlobalAdminUser registers an explicit global super-administrator
func (m *RBACManager) CreateGlobalAdminUser(username, password, fullName, email string) error {
	if err := ValidatePasswordStrength(password); err != nil {
		return err
	}

	m.mu.Lock()
	if _, exists := m.users[username]; exists {
		m.mu.Unlock()
		return fmt.Errorf("user %s already exists", username)
	}

	saltLength := m.argon2Config.SaltLength
	argonConfig := *m.argon2Config
	m.mu.Unlock()

	saltBytes := make([]byte, saltLength)
	if _, err := rand.Read(saltBytes); err != nil {
		return fmt.Errorf("failed to generate cryptographic salt: %w", err)
	}

	argonHash := HashPasswordArgon2id(password, saltBytes, &argonConfig)

	u := &UserAccount{
		Username:      username,
		FullName:      fullName,
		Role:          RoleAdmin,
		Email:         email,
		PasswordHash:  argonHash,
		Salt:          hex.EncodeToString(saltBytes),
		TenantID:      "system",
		SiteID:        "global",
		IsGlobalAdmin: true,
		Scopes:        []string{ScopeGlobalAdmin},
		Active:        true,
		CreatedAt:     time.Now().UTC(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[username] = u
	return nil
}

// InjectSessionForTest creates an active session directly for test cases.
// Fails securely: never substitutes deterministic or predictable randomness.
func (m *RBACManager) InjectSessionForTest(token, username string, role Role, tenantID, siteID string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	csrfBytes := make([]byte, 16)
	if _, err := rand.Read(csrfBytes); err != nil {
		panic(fmt.Sprintf("CRITICAL CRYPTOGRAPHIC FAILURE: crypto/rand unavailable in InjectSessionForTest: %v", err))
	}
	m.sessions[token] = &Session{
		Token:         token,
		Username:      username,
		Role:          role,
		TenantID:      tenantID,
		SiteID:        siteID,
		IsGlobalAdmin: false,
		CSRFToken:     fmt.Sprintf("test-csrf-%s", hex.EncodeToString(csrfBytes)),
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(ttl),
	}
}

// InjectGlobalAdminSessionForTest creates an active global super-admin session directly for tests.
// Fails securely: never substitutes deterministic or predictable randomness.
func (m *RBACManager) InjectGlobalAdminSessionForTest(token, username string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	csrfBytes := make([]byte, 16)
	if _, err := rand.Read(csrfBytes); err != nil {
		panic(fmt.Sprintf("CRITICAL CRYPTOGRAPHIC FAILURE: crypto/rand unavailable in InjectGlobalAdminSessionForTest: %v", err))
	}
	m.sessions[token] = &Session{
		Token:         token,
		Username:      username,
		Role:          RoleAdmin,
		TenantID:      "system",
		SiteID:        "global",
		IsGlobalAdmin: true,
		Scopes:        []string{ScopeGlobalAdmin},
		CSRFToken:     fmt.Sprintf("test-csrf-%s", hex.EncodeToString(csrfBytes)),
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(ttl),
	}
}

