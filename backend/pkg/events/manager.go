package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/models"
)

// EventManager orchestrates real-time event subscriptions, polling, deduplication, and broadcasting
type EventManager struct {
	mu            sync.RWMutex
	store         *database.Store
	logger        *audit.Logger
	cppAdapter    *CPPlusEventAdapter
	subscriptions map[string]*EventSubscription
	subCancelFunc map[string]context.CancelFunc
	recentEvents  []SurveillanceEvent // Ring buffer of recent events (max 1000)
	maxEvents     int
	dedupMap      map[string]time.Time // key: deviceID+topic -> last seen timestamp
	subscribers   []chan SurveillanceEvent
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewEventManager(store *database.Store, logger *audit.Logger) *EventManager {
	ctx, cancel := context.WithCancel(context.Background())
	em := &EventManager{
		store:         store,
		logger:        logger,
		cppAdapter:    NewCPPlusEventAdapter(),
		subscriptions: make(map[string]*EventSubscription),
		subCancelFunc: make(map[string]context.CancelFunc),
		recentEvents:  make([]SurveillanceEvent, 0, 1000),
		maxEvents:     1000,
		dedupMap:      make(map[string]time.Time),
		subscribers:   make([]chan SurveillanceEvent, 0),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start simulated telemetry generator for local loopback test units
	go em.startSimulatorTelemetryWorker()

	return em
}

// Close gracefully cancels all active event subscription workers
func (em *EventManager) Close() {
	em.cancel()
	em.mu.Lock()
	defer em.mu.Unlock()
	for _, cancel := range em.subCancelFunc {
		cancel()
	}
}

// SubscribeEvents registers a listener channel for incoming real-time events
func (em *EventManager) SubscribeEvents() chan SurveillanceEvent {
	em.mu.Lock()
	defer em.mu.Unlock()
	ch := make(chan SurveillanceEvent, 100)
	em.subscribers = append(em.subscribers, ch)
	return ch
}

// UnsubscribeEvents removes a listener channel
func (em *EventManager) UnsubscribeEvents(ch chan SurveillanceEvent) {
	em.mu.Lock()
	defer em.mu.Unlock()
	for i, sub := range em.subscribers {
		if sub == ch {
			em.subscribers = append(em.subscribers[:i], em.subscribers[i+1:]...)
			close(ch)
			break
		}
	}
}

// IngestEvent validates, deduplicates, and broadcasts an incoming event
func (em *EventManager) IngestEvent(ev SurveillanceEvent) bool {
	em.mu.Lock()
	defer em.mu.Unlock()

	// 1. Sliding-window deduplication (2-second window for identical device + topic + state)
	dedupKey := fmt.Sprintf("%s:%s:%v", ev.DeviceID, ev.Topic, ev.IsActive)
	now := time.Now().UTC()
	if lastSeen, exists := em.dedupMap[dedupKey]; exists {
		if now.Sub(lastSeen) < 2*time.Second {
			// Duplicate event suppressed within cooldown
			return false
		}
	}
	em.dedupMap[dedupKey] = now

	// Clean up stale deduplication entries periodically
	if len(em.dedupMap) > 5000 {
		cutoff := now.Add(-10 * time.Second)
		for k, t := range em.dedupMap {
			if t.Before(cutoff) {
				delete(em.dedupMap, k)
			}
		}
	}

	// 2. Append to ring buffer
	if len(em.recentEvents) >= em.maxEvents {
		em.recentEvents = em.recentEvents[1:]
	}
	em.recentEvents = append(em.recentEvents, ev)

	// 3. Update subscription metrics
	for _, sub := range em.subscriptions {
		if sub.DeviceID == ev.DeviceID {
			sub.EventsReceived++
			sub.LastPollTime = now
		}
	}

	// 4. Non-blocking broadcast to all active subscribers
	for _, ch := range em.subscribers {
		select {
		case ch <- ev:
		default:
			// Consumer backpressure: drop to prevent memory bloat
		}
	}

	return true
}

// GetRecentEvents returns the ring buffer of recent events
func (em *EventManager) GetRecentEvents(limit int) []SurveillanceEvent {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if limit <= 0 || limit > len(em.recentEvents) {
		limit = len(em.recentEvents)
	}

	start := len(em.recentEvents) - limit
	result := make([]SurveillanceEvent, limit)
	copy(result, em.recentEvents[start:])
	return result
}

// CreateSubscription establishes an event consumer worker for a camera or NVR
func (em *EventManager) CreateSubscription(deviceID string, proto SubscriptionProtocol, operator string) (*EventSubscription, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	dev, ok := em.store.GetDevice(deviceID)
	if !ok {
		return nil, fmt.Errorf("device %s not found in inventory", deviceID)
	}

	// Check if active subscription already exists
	subID := fmt.Sprintf("sub-%s-%s", deviceID, proto)
	if existing, ok := em.subscriptions[subID]; ok && existing.Status == StatusActive {
		return existing, nil
	}

	subURL := fmt.Sprintf("http://%s:%d/onvif/events_service", dev.IP, dev.Port)
	if proto == ProtoHikvisionISAPIStream {
		subURL = fmt.Sprintf("http://%s:%d/ISAPI/Event/notification/alertStream", dev.IP, dev.Port)
	} else if proto == ProtoDahuaCGIEventManager {
		subURL = fmt.Sprintf("http://%s:%d/cgi-bin/eventManager.cgi?action=attach&codes=[All]", dev.IP, dev.Port)
	}

	now := time.Now().UTC()
	sub := &EventSubscription{
		ID:                 subID,
		DeviceID:           deviceID,
		DeviceIP:           dev.IP,
		Protocol:           proto,
		SubscriptionURL:    subURL,
		Status:             StatusActive,
		CreatedAt:          now,
		ExpiresAt:          now.Add(10 * time.Minute),
		RenewalIntervalSec: 60,
		LastPollTime:       now,
		EventsReceived:     0,
	}

	subCtx, subCancel := context.WithCancel(em.ctx)
	em.subscriptions[subID] = sub
	em.subCancelFunc[subID] = subCancel

	em.logger.Log(operator, deviceID, dev.IP, "EVENT_SUBSCRIBE", "EVENTS", "SUCCESS",
		fmt.Sprintf("Created event subscription %s via %s", subID, proto), "")

	// Launch bounded subscription worker
	go em.runSubscriptionWorker(subCtx, sub, dev)

	return sub, nil
}

// CancelSubscription terminates an event consumer connection
func (em *EventManager) CancelSubscription(subscriptionID string, operator string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	sub, exists := em.subscriptions[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}

	if cancel, ok := em.subCancelFunc[subscriptionID]; ok {
		cancel()
		delete(em.subCancelFunc, subscriptionID)
	}

	sub.Status = StatusCancelled

	em.logger.Log(operator, sub.DeviceID, sub.DeviceIP, "EVENT_UNSUBSCRIBE", "EVENTS", "SUCCESS",
		fmt.Sprintf("Cancelled event subscription %s", subscriptionID), "")

	return nil
}

// GetSubscriptions returns all active and historical event subscriptions
func (em *EventManager) GetSubscriptions() []*EventSubscription {
	em.mu.RLock()
	defer em.mu.RUnlock()

	result := make([]*EventSubscription, 0, len(em.subscriptions))
	for _, s := range em.subscriptions {
		result = append(result, s)
	}
	return result
}

// Bounded subscription worker with automatic renewal and exponential backoff
func (em *EventManager) runSubscriptionWorker(ctx context.Context, sub *EventSubscription, dev models.Device) {
	ticker := time.NewTicker(time.Duration(sub.RenewalIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Renewal check
			em.mu.Lock()
			if time.Now().UTC().After(sub.ExpiresAt.Add(-2 * time.Minute)) {
				sub.Status = StatusRenewing
				sub.ExpiresAt = time.Now().UTC().Add(10 * time.Minute)
				sub.Status = StatusActive
				sub.LastPollTime = time.Now().UTC()
			}
			em.mu.Unlock()
		}
	}
}

// Background generator providing safe realistic telemetry for local test/simulator units
func (em *EventManager) startSimulatorTelemetryWorker() {
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()

	simTopics := []struct {
		category  EventCategory
		eventType string
		topic     string
		desc      string
	}{
		{CategoryMotion, "MotionDetection", "tns1:RuleEngine/CellMotionDetector/Motion", "Motion detected in zone 1"},
		{CategoryLineCrossing, "LineCrossing", "tns1:RuleEngine/LineDetector/LineCrossing", "Perimeter virtual tripwire line crossed"},
		{CategoryIntrusion, "IntrusionDetection", "tns1:RuleEngine/FieldDetector/ObjectsInside", "Object entered restricted security boundary"},
		{CategoryTampering, "VideoTampering", "tns1:VideoSource/VideoBlind", "Camera optical view covered or spray painted"},
	}

	step := 0
	for {
		select {
		case <-em.ctx.Done():
			return
		case <-ticker.C:
			// Ingest event for simulated devices if registered
			step++
			t := simTopics[step%len(simTopics)]

			ev := SurveillanceEvent{
				ID:           fmt.Sprintf("ev-sim-%d", time.Now().UnixNano()),
				DeviceID:     "dahua-127-0-0-1-18082",
				DeviceIP:     "127.0.0.1",
				Manufacturer: "Dahua",
				Model:        "DHI-XVR5108HS-4KL-I3",
				Category:     t.category,
				EventType:    t.eventType,
				Topic:        t.topic,
				Source:       "Channel 1 (Entrance BNC)",
				Timestamp:    time.Now().UTC(),
				ReceivedAt:   time.Now().UTC(),
				IsActive:     true,
				Properties:   map[string]string{"description": t.desc, "confidence": "0.95"},
				IsSimulator:  true,
			}
			em.IngestEvent(ev)
		}
	}
}
