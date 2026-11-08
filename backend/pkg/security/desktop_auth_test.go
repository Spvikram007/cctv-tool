package security_test

import (
	"strings"
	"testing"

	"cctv-sentinel/pkg/security"
)

func TestDesktopAuthManager_Lifecycle(t *testing.T) {
	mgr := security.NewDesktopAuthManager()

	// 1. Unregistered device cannot obtain challenge
	_, err := mgr.IssueChallenge("unknown-device-001")
	if err != security.ErrDeviceNotFound {
		t.Fatalf("expected ErrDeviceNotFound for unregistered device, got %v", err)
	}

	// 2. Register a new device
	dev, devKey, err := mgr.RegisterDevice("workstation-hq-01", "Ops Control Room 1")
	if err != nil {
		t.Fatalf("failed to register device: %v", err)
	}
	if dev.ClientID != "workstation-hq-01" || devKey == "" {
		t.Fatalf("invalid device registration output: %+v", dev)
	}
	if len(devKey) != 64 { // 32 bytes hex encoded = 64 characters
		t.Errorf("expected 64-char hex device key, got length %d", len(devKey))
	}

	// Duplicate registration must fail
	_, _, err = mgr.RegisterDevice("workstation-hq-01", "Duplicate")
	if err != security.ErrDeviceAlreadyExists {
		t.Errorf("expected ErrDeviceAlreadyExists, got %v", err)
	}

	// 3. Issue challenge
	chRec, err := mgr.IssueChallenge("workstation-hq-01")
	if err != nil {
		t.Fatalf("failed to issue challenge: %v", err)
	}
	if !strings.HasPrefix(chRec.Challenge, "ch-") {
		t.Errorf("expected ch- prefix on challenge, got %s", chRec.Challenge)
	}

	// 4. Verification with invalid proof fails
	err = mgr.VerifyProof("workstation-hq-01", chRec.Challenge, "0000111122223333444455556666777788889999aaaabbbbccccddddeeeeffff")
	if err != security.ErrInvalidDeviceProof {
		t.Fatalf("expected ErrInvalidDeviceProof for bad proof, got %v", err)
	}

	// 5. Re-issue challenge after invalid attempt (since challenge was consumed)
	chRec2, err := mgr.IssueChallenge("workstation-hq-01")
	if err != nil {
		t.Fatalf("failed to issue second challenge: %v", err)
	}

	// 6. Valid proof succeeds
	validProof := security.ComputeProofHelper(devKey, chRec2.Challenge)
	if err := mgr.VerifyProof("workstation-hq-01", chRec2.Challenge, validProof); err != nil {
		t.Fatalf("expected valid proof to succeed, got %v", err)
	}

	// 7. Replay attack: reusing the same challenge must be rejected
	err = mgr.VerifyProof("workstation-hq-01", chRec2.Challenge, validProof)
	if err != security.ErrChallengeNotFound {
		t.Fatalf("replayed challenge must return ErrChallengeNotFound, got %v", err)
	}

	// 8. Deactivated device cannot issue challenges
	if err := mgr.DeactivateDevice("workstation-hq-01"); err != nil {
		t.Fatalf("failed to deactivate device: %v", err)
	}
	_, err = mgr.IssueChallenge("workstation-hq-01")
	if err != security.ErrDeviceDeactivated {
		t.Fatalf("deactivated device must return ErrDeviceDeactivated, got %v", err)
	}
}

func TestDesktopAuthManager_DeviceListing(t *testing.T) {
	mgr := security.NewDesktopAuthManager()

	_, _, _ = mgr.RegisterDevice("dev-a", "Device Alpha")
	_, _, _ = mgr.RegisterDevice("dev-b", "Device Beta")

	list := mgr.ListDevices()
	if len(list) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(list))
	}

	// Verify secret keys are NEVER exposed in Device objects
	for _, d := range list {
		if d.DeviceKey != "" {
			t.Errorf("DeviceKey leaked in listed device: %+v", d)
		}
	}
}
