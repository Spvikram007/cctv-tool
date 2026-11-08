package security_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cctv-sentinel/pkg/security"
)

// faultyUserStore mocks a store that can simulate user persistence failures
type faultyUserStore struct {
	mu        sync.RWMutex
	users     map[string]security.UserAccount
	failSave  bool
	failDel   bool
}

func newFaultyUserStore() *faultyUserStore {
	return &faultyUserStore{
		users: make(map[string]security.UserAccount),
	}
}

func (s *faultyUserStore) SaveUser(u security.UserAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failSave {
		return errors.New("simulated I/O disk failure on SaveUser")
	}
	s.users[u.Username] = u
	return nil
}

func (s *faultyUserStore) GetUser(username string) (security.UserAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	return u, ok
}

func (s *faultyUserStore) ListUsers() []security.UserAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]security.UserAccount, 0, len(s.users))
	for _, u := range s.users {
		list = append(list, u)
	}
	return list
}

func (s *faultyUserStore) DeleteUser(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failDel {
		return errors.New("simulated I/O disk failure on DeleteUser")
	}
	delete(s.users, username)
	return nil
}

// TestUserPersistence_FailedSaveUserPreventsSetupCompletionAndPreservesToken proves that
// when durable persistence fails, setup completion fails closed, the setup token is NOT destroyed,
// and setup mode remains active.
func TestUserPersistence_FailedSaveUserPreventsSetupCompletionAndPreservesToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "test-setup.token")

	store := newFaultyUserStore()
	store.failSave = true // Deliberate failure injection

	mgr := security.NewRBACManager()
	mgr.SetSetupTokenFilePath(tokenFile)
	mgr.SetStore(store)

	token := mgr.GetSetupToken()
	if token == "" {
		t.Fatalf("expected active setup token, got empty")
	}
	// Durably write token to tokenFile for this test
	if err := security.WriteSecureFile(tokenFile, []byte(token+"\n")); err != nil {
		t.Fatalf("failed to write setup token file: %v", err)
	}
	if !mgr.IsSetupRequired() {
		t.Fatalf("setup required must be true before setup completion")
	}

	// Attempt setup with strong password but faulty store
	err := mgr.CompleteSetup(token, "admin_user", "SecureOperator#Pass2026!", "Administrator", "admin@sentinel.internal")
	if err == nil {
		t.Fatalf("expected error from CompleteSetup when persistence fails, got nil")
	}
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed, got %v", err)
	}

	// PROVE: Token must NOT be destroyed on disk or in memory
	if !mgr.IsSetupRequired() {
		t.Errorf("setup mode must remain ACTIVE after persistence failure")
	}
	if curToken := mgr.GetSetupToken(); curToken != token {
		t.Errorf("setup token was unexpectedly modified or cleared in memory: %s", curToken)
	}
	tokenData, fileErr := os.ReadFile(tokenFile)
	if fileErr != nil || strings.TrimSpace(string(tokenData)) != token {
		t.Errorf("setup token file on disk was destroyed or corrupted: %v (content: %s)", fileErr, string(tokenData))
	}

	// PROVE: Phantom user must NOT exist in memory
	if _, exists := mgr.GetUser("admin_user"); exists {
		t.Errorf("phantom user found in memory after failed persistence")
	}
}

// TestUserPersistence_CreateUser_FailClosed proves that CreateUser does not register
// in-memory users if durable persistence fails.
func TestUserPersistence_CreateUser_FailClosed(t *testing.T) {
	store := newFaultyUserStore()
	store.failSave = true

	mgr := security.NewRBACManager()
	mgr.SetStore(store)

	err := mgr.CreateUser("technician_bob", "TechPass#2026!Secure", "Bob Tech", security.RoleTechnician, "bob@cctv.com", "t-1", "s-1")
	if err == nil {
		t.Fatalf("expected error from CreateUser when persistence fails")
	}
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed, got %v", err)
	}

	// Verify no phantom user in memory
	if _, exists := mgr.GetUser("technician_bob"); exists {
		t.Errorf("phantom user technician_bob found in memory after failed CreateUser")
	}
}

// TestUserPersistence_SuccessfulSetupAndRestartPersistence tests setup completion with durable store,
// token invalidation, and server restart recovery.
func TestUserPersistence_SuccessfulSetupAndRestartPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "sentinel-setup.token")

	store := newFaultyUserStore()
	store.failSave = false

	mgr := security.NewRBACManager()
	mgr.SetSetupTokenFilePath(tokenFile)
	mgr.SetStore(store)

	token := mgr.GetSetupToken()
	adminPass := "MasterAdmin#2026!Key"

	// 1. Complete setup successfully
	err := mgr.CompleteSetup(token, "sys_admin", adminPass, "Chief Admin", "admin@sentinel.local")
	if err != nil {
		t.Fatalf("CompleteSetup failed: %v", err)
	}

	// Token must be invalidated
	if mgr.IsSetupRequired() {
		t.Errorf("setup required must be false after completion")
	}
	if mgr.GetSetupToken() != "" {
		t.Errorf("setup token in memory must be cleared")
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Errorf("setup token file must be deleted from disk")
	}

	// 2. Authenticate user
	sess, err := mgr.Authenticate("sys_admin", adminPass, 1*time.Hour)
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	if sess.Role != security.RoleAdmin || !sess.IsGlobalAdmin {
		t.Fatalf("unexpected session role: %+v", sess)
	}

	// 3. Simulate Server Restart: Reconstruct new RBACManager from same store
	mgrAfterRestart := security.NewRBACManager()
	mgrAfterRestart.SetSetupTokenFilePath(tokenFile)
	mgrAfterRestart.SetStore(store)

	// User must exist and authenticate cleanly after restart
	if mgrAfterRestart.IsSetupRequired() {
		t.Errorf("setup required must remain FALSE after server restart")
	}
	sessAfterRestart, err := mgrAfterRestart.Authenticate("sys_admin", adminPass, 1*time.Hour)
	if err != nil {
		t.Fatalf("authentication after restart failed: %v", err)
	}
	if sessAfterRestart.Username != "sys_admin" {
		t.Errorf("expected username sys_admin, got %s", sessAfterRestart.Username)
	}

	// Old setup token must remain permanently invalid
	err = mgrAfterRestart.CompleteSetup(token, "hacker", "HackerPass#2026!X", "Hacker", "h@h.com")
	if err != security.ErrSetupDisabled {
		t.Errorf("expected ErrSetupDisabled on reuse after restart, got %v", err)
	}
}

// TestUserPersistence_PasswordHashNeverRenderedInListUsers verifies zero credential leakage in API projections
func TestUserPersistence_PasswordHashNeverRenderedInListUsers(t *testing.T) {
	store := newFaultyUserStore()
	mgr := security.NewRBACManager()
	mgr.SetStore(store)

	_ = mgr.CreateUser("user_alice", "AlicePass#2026!Str", "Alice Analyst", security.RoleViewer, "alice@cctv.com", "t-1", "s-1")
	_ = mgr.CreateUser("user_bob", "BobPass#2026!Secure", "Bob Tech", security.RoleTechnician, "bob@cctv.com", "t-1", "s-1")

	users := mgr.ListUsers()
	if len(users) < 2 {
		t.Fatalf("expected at least 2 users, got %d", len(users))
	}

	for _, u := range users {
		if u.PasswordHash != "" {
			t.Errorf("SECURITY LEAK: user %s has non-empty PasswordHash in ListUsers()", u.Username)
		}
		if u.Salt != "" {
			t.Errorf("SECURITY LEAK: user %s has non-empty Salt in ListUsers()", u.Username)
		}
	}
}

// TestUserPersistence_ConcurrentUserCreationRejected verifies atomicity under concurrent creation
func TestUserPersistence_ConcurrentUserCreationRejected(t *testing.T) {
	store := newFaultyUserStore()
	mgr := security.NewRBACManager()
	mgr.SetStore(store)

	var wg sync.WaitGroup
	successCount := 0
	conflictCount := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := mgr.CreateUser("same_operator", "ConcurrentPass#2026!", "Same Operator", security.RoleViewer, "same@cctv.com", "t-1", "s-1")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, security.ErrUserExists) {
				conflictCount++
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful user creation, got %d", successCount)
	}
	if conflictCount != 4 {
		t.Errorf("expected 4 conflict rejections, got %d", conflictCount)
	}
}
