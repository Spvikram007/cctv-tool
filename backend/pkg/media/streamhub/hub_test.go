package streamhub

import (
	"testing"
	"time"

	"cctv-sentinel/pkg/adapters"
	"cctv-sentinel/pkg/database"
)

func TestStreamTokenLifecycle(t *testing.T) {
	store := database.NewStore("")
	registry := adapters.NewRegistry()
	hub := NewHub(store, registry)

	// 1. Generate token
	token, err := hub.GenerateToken("dev-01", "main", "Technician1", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token.Token == "" {
		t.Fatalf("expected non-empty token")
	}
	if token.DeviceID != "dev-01" {
		t.Errorf("expected device dev-01, got %s", token.DeviceID)
	}

	// 2. Validate valid token
	val, err := hub.ValidateToken(token.Token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if val.DeviceID != "dev-01" {
		t.Errorf("expected dev-01")
	}

	// 3. Test non-existent token
	_, err = hub.ValidateToken("fake-token-123")
	if err != ErrTokenNotFound {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}

	// 4. Test expiration
	time.Sleep(600 * time.Millisecond)
	_, err = hub.ValidateToken(token.Token)
	if err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestStreamStats(t *testing.T) {
	sess := &LiveSession{
		DeviceID:   "dev-02",
		StreamType: "main",
		status:     "LIVE",
		frameCount: 100,
	}

	stats := sess.GetStats()
	if stats.DeviceID != "dev-02" {
		t.Errorf("expected dev-02, got %s", stats.DeviceID)
	}
	if stats.Status != "LIVE" {
		t.Errorf("expected LIVE, got %s", stats.Status)
	}
	if stats.FrameCount != 100 {
		t.Errorf("expected 100 frames, got %d", stats.FrameCount)
	}
}
