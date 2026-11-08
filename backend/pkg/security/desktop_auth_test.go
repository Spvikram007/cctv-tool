package security_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cctv-sentinel/pkg/security"
)

// mockDeviceStore implements security.DesktopDeviceStore in memory for unit testing
type mockDeviceStore struct {
	mu      sync.RWMutex
	devices map[string]security.DesktopDeviceRecord
}

func newMockDeviceStore() *mockDeviceStore {
	return &mockDeviceStore{
		devices: make(map[string]security.DesktopDeviceRecord),
	}
}

func (s *mockDeviceStore) UpsertDesktopDevice(dev security.DesktopDeviceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[dev.ID] = dev
	return nil
}

func (s *mockDeviceStore) GetDesktopDevice(id string) (security.DesktopDeviceRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[id]
	return d, ok
}

func (s *mockDeviceStore) ListDesktopDevices() []security.DesktopDeviceRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]security.DesktopDeviceRecord, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}
	return list
}

func (s *mockDeviceStore) DeleteDesktopDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.devices, id)
	return nil
}

func TestDesktopAuthManager_AsymmetricLifecycle(t *testing.T) {
	store := newMockDeviceStore()
	mgr := security.NewDesktopAuthManager(store)

	// 1. Unknown device cannot obtain a challenge
	_, err := mgr.IssueChallenge("workstation-unknown")
	if err != security.ErrDeviceNotFound {
		t.Fatalf("expected ErrDeviceNotFound for unregistered device, got %v", err)
	}

	// 2. Client generates local Ed25519 keypair
	pubKey, privKey, err := security.GenerateClientKeyPair()
	if err != nil {
		t.Fatalf("failed to generate Ed25519 keypair: %v", err)
	}
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKey)

	// 3. Submit enrollment (Pending status)
	enrolled, err := mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:         "ws-control-01",
		ClientName: "Main Control Workstation",
		OS:         "Windows 11 Enterprise",
		HostName:   "CCTV-OPS-01",
		PublicKey:  pubKeyB64,
	})
	if err != nil {
		t.Fatalf("failed to submit enrollment: %v", err)
	}
	if enrolled.ApprovalStatus != string(security.ApprovalPending) {
		t.Errorf("expected status PENDING, got %s", enrolled.ApprovalStatus)
	}
	if !strings.HasPrefix(enrolled.KeyFingerprint, "SHA256:") {
		t.Errorf("expected SHA256 prefix on fingerprint, got %s", enrolled.KeyFingerprint)
	}

	// 4. Pending device cannot obtain a challenge
	_, err = mgr.IssueChallenge("ws-control-01")
	if err != security.ErrDeviceNotApproved {
		t.Fatalf("expected ErrDeviceNotApproved for pending device, got %v", err)
	}

	// 5. Admin approves device
	approved, err := mgr.ApproveDevice("ws-control-01", "sec_admin")
	if err != nil {
		t.Fatalf("failed to approve device: %v", err)
	}
	if approved.ApprovalStatus != string(security.ApprovalApproved) || !approved.Active {
		t.Fatalf("approved device has invalid state: %+v", approved)
	}

	// 6. Active approved device can obtain challenge
	chRec, err := mgr.IssueChallenge("ws-control-01")
	if err != nil {
		t.Fatalf("failed to issue challenge: %v", err)
	}
	if !strings.HasPrefix(chRec.Challenge, "ch-") {
		t.Errorf("expected challenge to start with ch-, got %s", chRec.Challenge)
	}

	// 7. Verification with bad signature must fail
	badSig := base64.StdEncoding.EncodeToString(make([]byte, 64))
	err = mgr.VerifyProof("ws-control-01", chRec.Challenge, badSig)
	if err != security.ErrInvalidDeviceProof {
		t.Fatalf("expected ErrInvalidDeviceProof for bad signature, got %v", err)
	}

	// 8. Challenge must be atomically consumed EVEN on failed verification attempt
	err = mgr.VerifyProof("ws-control-01", chRec.Challenge, badSig)
	if err != security.ErrChallengeNotFound {
		t.Fatalf("expected ErrChallengeNotFound because challenge was consumed, got %v", err)
	}

	// 9. Re-issue challenge and sign with valid private key
	chRec2, err := mgr.IssueChallenge("ws-control-01")
	if err != nil {
		t.Fatalf("failed to issue challenge 2: %v", err)
	}
	validSig := security.SignChallenge(privKey, chRec2.Challenge)
	if err := mgr.VerifyProof("ws-control-01", chRec2.Challenge, validSig); err != nil {
		t.Fatalf("expected valid proof to succeed, got %v", err)
	}

	// 10. Replay attack: cannot reuse consumed challenge
	err = mgr.VerifyProof("ws-control-01", chRec2.Challenge, validSig)
	if err != security.ErrChallengeNotFound {
		t.Fatalf("expected ErrChallengeNotFound on replay attack, got %v", err)
	}

	// 11. Silent overwrite protection: re-enrollment of active approved device must fail
	_, err = mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-control-01",
		PublicKey: pubKeyB64,
	})
	if err != security.ErrDeviceAlreadyActive {
		t.Fatalf("expected ErrDeviceAlreadyActive on duplicate active enrollment, got %v", err)
	}

	// 12. Key rotation
	newPub, newPriv, _ := security.GenerateClientKeyPair()
	rotated, err := mgr.RotateDeviceKey("ws-control-01", base64.StdEncoding.EncodeToString(newPub), "sec_admin")
	if err != nil {
		t.Fatalf("failed to rotate device key: %v", err)
	}
	if rotated.KeyVersion != 2 {
		t.Errorf("expected KeyVersion 2 after rotation, got %d", rotated.KeyVersion)
	}

	// Old private key signature should now fail
	chRec3, _ := mgr.IssueChallenge("ws-control-01")
	oldSig := security.SignChallenge(privKey, chRec3.Challenge)
	if err := mgr.VerifyProof("ws-control-01", chRec3.Challenge, oldSig); err != security.ErrInvalidDeviceProof {
		t.Fatalf("expected failure when verifying with old private key, got %v", err)
	}

	// New private key signature succeeds
	chRec4, _ := mgr.IssueChallenge("ws-control-01")
	newSig := security.SignChallenge(newPriv, chRec4.Challenge)
	if err := mgr.VerifyProof("ws-control-01", chRec4.Challenge, newSig); err != nil {
		t.Fatalf("expected success with new rotated private key, got %v", err)
	}

	// 13. Revoke device
	revoked, err := mgr.RevokeDevice("ws-control-01", "sec_admin")
	if err != nil {
		t.Fatalf("failed to revoke device: %v", err)
	}
	if revoked.Status != string(security.StatusRevoked) || revoked.Active {
		t.Fatalf("revoked device has invalid state: %+v", revoked)
	}

	// Revoked device cannot obtain challenges
	_, err = mgr.IssueChallenge("ws-control-01")
	if err != security.ErrDeviceDeactivated {
		t.Fatalf("expected ErrDeviceDeactivated for revoked device, got %v", err)
	}

	// 14. Persistence verification: reconstruct manager from store
	mgr2 := security.NewDesktopAuthManager(store)
	loaded, err := mgr2.GetDevice("ws-control-01")
	if err != nil {
		t.Fatalf("failed to reload device from store: %v", err)
	}
	if loaded.Status != string(security.StatusRevoked) {
		t.Errorf("expected reloaded status REVOKED, got %s", loaded.Status)
	}
}

// faultyDeviceStore simulates durable persistence failures
type faultyDeviceStore struct {
	mu          sync.RWMutex
	devices     map[string]security.DesktopDeviceRecord
	failUpsert  bool
	failDelete  bool
}

func newFaultyDeviceStore() *faultyDeviceStore {
	return &faultyDeviceStore{
		devices: make(map[string]security.DesktopDeviceRecord),
	}
}

func (s *faultyDeviceStore) UpsertDesktopDevice(dev security.DesktopDeviceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failUpsert {
		return errors.New("simulated disk I/O persistence failure: write error")
	}
	s.devices[dev.ID] = dev
	return nil
}

func (s *faultyDeviceStore) GetDesktopDevice(id string) (security.DesktopDeviceRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[id]
	return d, ok
}

func (s *faultyDeviceStore) ListDesktopDevices() []security.DesktopDeviceRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]security.DesktopDeviceRecord, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}
	return list
}

func (s *faultyDeviceStore) DeleteDesktopDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failDelete {
		return errors.New("simulated disk I/O persistence failure: delete error")
	}
	delete(s.devices, id)
	return nil
}

// TestDesktopAuthManager_FaultInjection_AllMutationsFailClosed tests that every mutation method
// fails closed without mutating in-memory state when durable persistence fails.
func TestDesktopAuthManager_FaultInjection_AllMutationsFailClosed(t *testing.T) {
	store := newFaultyDeviceStore()
	mgr := security.NewDesktopAuthManager(store)

	pub, priv, err := security.GenerateClientKeyPair()
	if err != nil {
		t.Fatalf("failed keypair gen: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	// 1. SubmitEnrollment fail-closed on persistence failure
	store.failUpsert = true
	_, err = mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-fault-1",
		PublicKey: pubB64,
	})
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on SubmitEnrollment, got %v", err)
	}
	// Verify memory state: device must NOT exist in memory
	if _, err := mgr.GetDevice("ws-fault-1"); err != security.ErrDeviceNotFound {
		t.Fatalf("phantom device found in memory after failed SubmitEnrollment: %v", err)
	}

	// Now allow enrollment to succeed
	store.failUpsert = false
	rec, err := mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-fault-1",
		PublicKey: pubB64,
	})
	if err != nil {
		t.Fatalf("enrollment failed: %v", err)
	}
	if rec.ApprovalStatus != string(security.ApprovalPending) {
		t.Fatalf("expected status PENDING, got %s", rec.ApprovalStatus)
	}

	// 2. ApproveDevice fail-closed on persistence failure
	store.failUpsert = true
	_, err = mgr.ApproveDevice("ws-fault-1", "admin")
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on ApproveDevice, got %v", err)
	}
	// Memory state must still be PENDING (not approved, not active)
	cur, _ := mgr.GetDevice("ws-fault-1")
	if cur.ApprovalStatus != string(security.ApprovalPending) || cur.Active {
		t.Fatalf("in-memory state corrupted after failed ApproveDevice: %+v", cur)
	}

	// Now allow approval to succeed
	store.failUpsert = false
	_, err = mgr.ApproveDevice("ws-fault-1", "admin")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	// 3. RotateDeviceKey fail-closed on persistence failure
	newPub, _, _ := security.GenerateClientKeyPair()
	newPubB64 := base64.StdEncoding.EncodeToString(newPub)
	store.failUpsert = true
	_, err = mgr.RotateDeviceKey("ws-fault-1", newPubB64, "admin")
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on RotateDeviceKey, got %v", err)
	}
	cur, _ = mgr.GetDevice("ws-fault-1")
	if cur.KeyVersion != 1 || cur.PublicKey != pubB64 {
		t.Fatalf("in-memory key mutated after failed RotateDeviceKey: %+v", cur)
	}

	// 4. VerifyProof fail-closed on LastSeenAt persistence failure
	chRec, err := mgr.IssueChallenge("ws-fault-1")
	if err != nil {
		t.Fatalf("failed to issue challenge: %v", err)
	}
	sig := security.SignChallenge(priv, chRec.Challenge)
	// Persistence fails when writing LastSeenAt
	store.failUpsert = true
	err = mgr.VerifyProof("ws-fault-1", chRec.Challenge, sig)
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on VerifyProof when store fails, got %v", err)
	}
	// Challenge must still have been consumed atomically
	store.failUpsert = false
	err = mgr.VerifyProof("ws-fault-1", chRec.Challenge, sig)
	if err != security.ErrChallengeNotFound {
		t.Fatalf("expected ErrChallengeNotFound on replay of consumed challenge, got %v", err)
	}

	// 5. RejectDevice fail-closed on persistence failure
	store.failUpsert = true
	_, err = mgr.RejectDevice("ws-fault-1", "admin")
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on RejectDevice, got %v", err)
	}
	cur, _ = mgr.GetDevice("ws-fault-1")
	if cur.ApprovalStatus != string(security.ApprovalApproved) {
		t.Fatalf("in-memory state mutated after failed RejectDevice: %+v", cur)
	}

	// 6. RevokeDevice fail-closed on persistence failure
	store.failUpsert = true
	_, err = mgr.RevokeDevice("ws-fault-1", "admin")
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on RevokeDevice, got %v", err)
	}
	cur, _ = mgr.GetDevice("ws-fault-1")
	if cur.Status != string(security.StatusActive) || !cur.Active {
		t.Fatalf("in-memory state mutated after failed RevokeDevice: %+v", cur)
	}

	// 7. CancelPendingEnrollment fail-closed on persistence failure
	store.failUpsert = false
	pubPending, _, _ := security.GenerateClientKeyPair()
	_, _ = mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-fault-pending",
		PublicKey: base64.StdEncoding.EncodeToString(pubPending),
	})
	store.failDelete = true
	err = mgr.CancelPendingEnrollment("ws-fault-pending")
	if !errors.Is(err, security.ErrPersistenceFailed) {
		t.Fatalf("expected ErrPersistenceFailed on CancelPendingEnrollment, got %v", err)
	}
	if _, err := mgr.GetDevice("ws-fault-pending"); err != nil {
		t.Fatalf("pending enrollment deleted from memory despite persistence failure")
	}
}

// TestDesktopAuthManager_PendingEnrollmentOverwriteProtectionAndCancel tests pending device lifecycle
func TestDesktopAuthManager_PendingEnrollmentOverwriteProtectionAndCancel(t *testing.T) {
	store := newMockDeviceStore()
	mgr := security.NewDesktopAuthManager(store)

	pub1, _, _ := security.GenerateClientKeyPair()
	pub2, _, _ := security.GenerateClientKeyPair()

	// 1. Submit pending enrollment
	_, err := mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-pending-1",
		PublicKey: base64.StdEncoding.EncodeToString(pub1),
	})
	if err != nil {
		t.Fatalf("initial enrollment failed: %v", err)
	}

	// 2. Resubmission of pending device must be rejected with ErrDevicePendingApproval
	_, err = mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-pending-1",
		PublicKey: base64.StdEncoding.EncodeToString(pub2),
	})
	if err != security.ErrDevicePendingApproval {
		t.Fatalf("expected ErrDevicePendingApproval on duplicate pending enrollment, got %v", err)
	}

	// 3. Cancel pending enrollment
	if err := mgr.CancelPendingEnrollment("ws-pending-1"); err != nil {
		t.Fatalf("cancel pending enrollment failed: %v", err)
	}

	// 4. After cancellation, re-enrollment succeeds
	rec, err := mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-pending-1",
		PublicKey: base64.StdEncoding.EncodeToString(pub2),
	})
	if err != nil {
		t.Fatalf("re-enrollment failed after cancellation: %v", err)
	}
	if rec.KeyFingerprint != security.ComputeFingerprint(pub2) {
		t.Fatalf("re-enrolled device does not match new key fingerprint")
	}
}

// TestDesktopAuthManager_ApproveFingerprintVerification verifies strict fingerprint binding during approval
func TestDesktopAuthManager_ApproveFingerprintVerification(t *testing.T) {
	mgr := security.NewDesktopAuthManager(newMockDeviceStore())
	pub, _, _ := security.GenerateClientKeyPair()
	fp := security.ComputeFingerprint(pub)

	_, err := mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-fp-check",
		PublicKey: base64.StdEncoding.EncodeToString(pub),
	})
	if err != nil {
		t.Fatalf("failed enrollment: %v", err)
	}

	// Approval with mismatched fingerprint must fail
	_, err = mgr.ApproveDevice("ws-fp-check", "admin", "SHA256:0000000000000000000000000000000000000000000000000000000000000000")
	if err != security.ErrFingerprintMismatch {
		t.Fatalf("expected ErrFingerprintMismatch, got %v", err)
	}

	// Approval with matching fingerprint succeeds
	approved, err := mgr.ApproveDevice("ws-fp-check", "admin", fp)
	if err != nil {
		t.Fatalf("expected approval to succeed with matching fingerprint, got %v", err)
	}
	if approved.KeyFingerprint != fp || approved.ApprovalStatus != string(security.ApprovalApproved) {
		t.Fatalf("unexpected state after approval: %+v", approved)
	}
}

func TestDesktopAuthManager_ExpiredEnrollmentRejected(t *testing.T) {
	mgr := security.NewDesktopAuthManager(nil)
	pub, _, _ := security.GenerateClientKeyPair()

	enrolled, err := mgr.SubmitEnrollment(security.DesktopEnrollmentRequest{
		ID:        "ws-expired",
		PublicKey: base64.StdEncoding.EncodeToString(pub),
	})
	if err != nil {
		t.Fatalf("failed enrollment: %v", err)
	}

	// Manually backdate expiry to simulate expired enrollment
	past := time.Now().UTC().Add(-1 * time.Hour)
	enrolled.ExpiresAt = &past

	_, err = mgr.ApproveDevice("ws-expired", "admin")
	if err != security.ErrEnrollmentExpired {
		t.Fatalf("expected ErrEnrollmentExpired, got %v", err)
	}
}
