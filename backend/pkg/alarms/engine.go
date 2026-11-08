package alarms

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"cctv-sentinel/pkg/audit"
	"cctv-sentinel/pkg/database"
	"cctv-sentinel/pkg/events"
	"cctv-sentinel/pkg/media/snapshot"
)

// Engine manages alarm processing, rule evaluation, state transitions, and real-time broadcasting
type Engine struct {
	mu           sync.RWMutex
	store        *database.Store
	logger       *audit.Logger
	snapshotEng  *snapshot.Engine
	eventMgr     *events.EventManager
	alarms       []*Alarm              // Ring buffer of normalized alarms
	alarmMap     map[string]*Alarm     // ID -> Alarm index
	rules        map[string]*AlarmRule // ID -> Rule
	ruleCooldown map[string]time.Time  // ruleID:deviceID -> last triggered timestamp
	subscribers  []chan *Alarm
	maxAlarms    int
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewEngine(store *database.Store, logger *audit.Logger, snapEng *snapshot.Engine, evMgr *events.EventManager) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{
		store:        store,
		logger:       logger,
		snapshotEng:  snapEng,
		eventMgr:     evMgr,
		alarms:       make([]*Alarm, 0, 5000),
		alarmMap:     make(map[string]*Alarm),
		rules:        make(map[string]*AlarmRule),
		ruleCooldown: make(map[string]time.Time),
		subscribers:  make([]chan *Alarm, 0),
		maxAlarms:    5000,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Seed default enterprise alarm rules
	e.seedDefaultRules()

	// Start event listener worker
	go e.startEventConsumer()

	return e
}

// Close terminates event listener and background tasks
func (e *Engine) Close() {
	e.cancel()
}

func (e *Engine) seedDefaultRules() {
	e.rules["rule-tamper-critical"] = &AlarmRule{
		ID:              "rule-tamper-critical",
		Name:            "Camera Tampering Critical Alert",
		Enabled:         true,
		EventCategories: []string{"TAMPERING", "VIDEO_LOSS"},
		Severity:        SeverityCritical,
		CaptureSnapshot: true,
		CooldownSec:     10,
	}

	e.rules["rule-perimeter-intrusion"] = &AlarmRule{
		ID:              "rule-perimeter-intrusion",
		Name:            "Perimeter Line Crossing & Intrusion",
		Enabled:         true,
		EventCategories: []string{"LINE_CROSSING", "INTRUSION"},
		Severity:        SeverityMajor,
		CaptureSnapshot: true,
		CooldownSec:     5,
	}

	e.rules["rule-storage-alert"] = &AlarmRule{
		ID:              "rule-storage-alert",
		Name:            "Storage & HDD Hardware Failure",
		Enabled:         true,
		EventCategories: []string{"STORAGE_ALARM"},
		Severity:        SeverityCritical,
		CaptureSnapshot: false,
		CooldownSec:     60,
	}
}

// SubscribeAlarms returns a listener channel for live alarm notifications
func (e *Engine) SubscribeAlarms() chan *Alarm {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan *Alarm, 100)
	e.subscribers = append(e.subscribers, ch)
	return ch
}

// UnsubscribeAlarms detaches a listener channel
func (e *Engine) UnsubscribeAlarms(ch chan *Alarm) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, sub := range e.subscribers {
		if sub == ch {
			e.subscribers = append(e.subscribers[:i], e.subscribers[i+1:]...)
			close(ch)
			break
		}
	}
}

// startEventConsumer receives events from the EventManager and evaluates alarm rules
func (e *Engine) startEventConsumer() {
	evChan := e.eventMgr.SubscribeEvents()
	defer e.eventMgr.UnsubscribeEvents(evChan)

	for {
		select {
		case <-e.ctx.Done():
			return
		case ev, ok := <-evChan:
			if !ok {
				return
			}
			// Only active/start events create new alarms
			if ev.IsActive {
				e.ProcessEvent(ev)
			}
		}
	}
}

// ProcessEvent transforms a surveillance event into a normalized alarm
func (e *Engine) ProcessEvent(ev events.SurveillanceEvent) *Alarm {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Determine severity from default mapping
	severity := e.mapDefaultSeverity(ev.Category)
	shouldSnap := false
	var assignTo string

	now := time.Now().UTC()

	// 2. Evaluate against configured Alarm Rules
	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		// Check device filter
		if len(rule.DeviceIDs) > 0 {
			matchedDev := false
			for _, id := range rule.DeviceIDs {
				if id == ev.DeviceID {
					matchedDev = true
					break
				}
			}
			if !matchedDev {
				continue
			}
		}

		// Check category match
		matchedCat := false
		for _, cat := range rule.EventCategories {
			if strings.EqualFold(cat, string(ev.Category)) {
				matchedCat = true
				break
			}
		}

		if matchedCat {
			// Cooldown check
			cooldownKey := fmt.Sprintf("%s:%s", rule.ID, ev.DeviceID)
			if last, ok := e.ruleCooldown[cooldownKey]; ok {
				if now.Sub(last) < time.Duration(rule.CooldownSec)*time.Second {
					// Rule in cooldown window: suppress duplicate notification loop
					continue
				}
			}
			e.ruleCooldown[cooldownKey] = now

			severity = rule.Severity
			if rule.CaptureSnapshot {
				shouldSnap = true
			}
			if rule.AssignTechnician != "" {
				assignTo = rule.AssignTechnician
			}
			break
		}
	}

	// 3. Lookup device metadata
	siteID := "Default-Site"
	mfg := ev.Manufacturer
	model := ev.Model
	if dev, ok := e.store.GetDevice(ev.DeviceID); ok {
		if dev.SiteID != "" {
			siteID = dev.SiteID
		}
		if mfg == "" {
			mfg = string(dev.Manufacturer)
		}
		if model == "" {
			model = dev.Model
		}
	}

	alarmID := fmt.Sprintf("alm-%s-%d", ev.DeviceID, now.UnixNano())
	desc := fmt.Sprintf("%s event detected on %s", ev.EventType, ev.Source)
	if pDesc, ok := ev.Properties["description"]; ok && pDesc != "" {
		desc = pDesc
	}

	alm := &Alarm{
		ID:                alarmID,
		TenantID:          "tenant-01",
		SiteID:            siteID,
		DeviceID:          ev.DeviceID,
		Manufacturer:      mfg,
		Model:             model,
		EventCategory:     string(ev.Category),
		EventType:         ev.EventType,
		Severity:          severity,
		EventTimestamp:    ev.Timestamp,
		ReceivedTimestamp: now,
		EventSource:       ev.Source,
		Description:       desc,
		State:             StateNew,
		AckStatus:         "UNACKNOWLEDGED",
		RedactedPayload:   ev.RedactedPayload,
		IsSimulator:       ev.IsSimulator,
	}

	if assignTo != "" {
		alm.AssignedTo = assignTo
		alm.AssignedAt = &now
		alm.State = StateInvestigating
	}

	// 4. Trigger asynchronous snapshot evidence capture if requested
	if shouldSnap && e.snapshotEng != nil {
		go func(targetDevID string, targetAlm *Alarm) {
			meta, _, err := e.snapshotEng.CaptureSnapshot(targetDevID)
			if err == nil && meta != nil {
				captureTime := time.Now().UTC()
				e.mu.Lock()
				targetAlm.SnapshotRef = meta.URL
				targetAlm.SnapshotCaptureTime = &captureTime
				e.mu.Unlock()
			}
		}(ev.DeviceID, alm)
	}

	// 5. Append to ring buffer
	if len(e.alarms) >= e.maxAlarms {
		old := e.alarms[0]
		delete(e.alarmMap, old.ID)
		e.alarms = e.alarms[1:]
	}
	e.alarms = append(e.alarms, alm)
	e.alarmMap[alm.ID] = alm

	// 6. Broadcast to real-time subscribers
	for _, ch := range e.subscribers {
		select {
		case ch <- alm:
		default:
		}
	}

	e.logger.Log("System_AlarmEngine", ev.DeviceID, ev.DeviceIP, "ALARM_GENERATED", "ALARMS", string(severity),
		fmt.Sprintf("Alarm %s [%s] generated for %s (%s)", alm.ID, severity, alm.EventType, ev.Source), "")

	return alm
}

func (e *Engine) mapDefaultSeverity(cat events.EventCategory) AlarmSeverity {
	switch cat {
	case events.CategoryTampering, events.CategoryVideoLoss, events.CategoryStorageAlarm:
		return SeverityCritical
	case events.CategoryLineCrossing, events.CategoryIntrusion, events.CategoryFaceDetection, events.CategoryAlarmInput:
		return SeverityMajor
	case events.CategoryMotion, events.CategoryHumanDetection, events.CategoryVehicleDetection:
		return SeverityMinor
	default:
		return SeverityInfo
	}
}

// Acknowledge marks an alarm as acknowledged by an operator
func (e *Engine) Acknowledge(alarmID, operator string) (*Alarm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alm, ok := e.alarmMap[alarmID]
	if !ok {
		return nil, fmt.Errorf("alarm %s not found", alarmID)
	}

	now := time.Now().UTC()
	alm.State = StateAcknowledged
	alm.AckStatus = "ACKNOWLEDGED"
	alm.AckBy = operator
	alm.AckAt = &now

	e.logger.Log(operator, alm.DeviceID, "127.0.0.1", "ALARM_ACKNOWLEDGE", "ALARMS", "SUCCESS",
		fmt.Sprintf("Operator %s acknowledged alarm %s", operator, alarmID), "")

	return alm, nil
}

// Assign delegates an alarm to a field technician
func (e *Engine) Assign(alarmID, technician, operator string) (*Alarm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alm, ok := e.alarmMap[alarmID]
	if !ok {
		return nil, fmt.Errorf("alarm %s not found", alarmID)
	}

	now := time.Now().UTC()
	alm.AssignedTo = technician
	alm.AssignedAt = &now
	alm.State = StateInvestigating

	e.logger.Log(operator, alm.DeviceID, "127.0.0.1", "ALARM_ASSIGN", "ALARMS", "SUCCESS",
		fmt.Sprintf("Assigned alarm %s to technician %s", alarmID, technician), "")

	return alm, nil
}

// Resolve concludes an alarm with technician resolution notes
func (e *Engine) Resolve(alarmID, notes, operator string) (*Alarm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alm, ok := e.alarmMap[alarmID]
	if !ok {
		return nil, fmt.Errorf("alarm %s not found", alarmID)
	}

	now := time.Now().UTC()
	alm.State = StateResolved
	alm.ResolutionNotes = notes
	alm.ResolvedBy = operator
	alm.ResolvedAt = &now

	e.logger.Log(operator, alm.DeviceID, "127.0.0.1", "ALARM_RESOLVE", "ALARMS", "SUCCESS",
		fmt.Sprintf("Resolved alarm %s: %s", alarmID, notes), "")

	return alm, nil
}

// Dismiss closes a false positive alarm
func (e *Engine) Dismiss(alarmID, reason, operator string) (*Alarm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alm, ok := e.alarmMap[alarmID]
	if !ok {
		return nil, fmt.Errorf("alarm %s not found", alarmID)
	}

	now := time.Now().UTC()
	alm.State = StateDismissed
	alm.ResolutionNotes = fmt.Sprintf("Dismissed: %s", reason)
	alm.ResolvedBy = operator
	alm.ResolvedAt = &now

	e.logger.Log(operator, alm.DeviceID, "127.0.0.1", "ALARM_DISMISS", "ALARMS", "SUCCESS",
		fmt.Sprintf("Dismissed alarm %s: %s", alarmID, reason), "")

	return alm, nil
}

// GetAlarm returns a single alarm by ID
func (e *Engine) GetAlarm(id string) (*Alarm, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	alm, ok := e.alarmMap[id]
	return alm, ok
}

// FilterAlarms searches and filters alarms according to query specifications
func (e *Engine) FilterAlarms(f AlarmFilter) []*Alarm {
	e.mu.RLock()
	defer e.mu.RUnlock()

	results := make([]*Alarm, 0)
	// Iterate from newest to oldest
	for i := len(e.alarms) - 1; i >= 0; i-- {
		alm := e.alarms[i]

		if f.DeviceID != "" && alm.DeviceID != f.DeviceID {
			continue
		}
		if f.SiteID != "" && alm.SiteID != f.SiteID {
			continue
		}
		if f.Manufacturer != "" && !strings.EqualFold(alm.Manufacturer, f.Manufacturer) {
			continue
		}
		if f.EventCategory != "" && !strings.EqualFold(alm.EventCategory, f.EventCategory) {
			continue
		}
		if f.Severity != "" && alm.Severity != f.Severity {
			continue
		}
		if f.State != "" && alm.State != f.State {
			continue
		}
		if f.StartTime != nil && alm.EventTimestamp.Before(*f.StartTime) {
			continue
		}
		if f.EndTime != nil && alm.EventTimestamp.After(*f.EndTime) {
			continue
		}

		results = append(results, alm)
		if f.Limit > 0 && len(results) >= f.Limit {
			break
		}
	}
	return results
}

// GetStats compiles real-time dashboard counts
func (e *Engine) GetStats() AlarmStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := AlarmStats{
		TotalAlarms: len(e.alarms),
	}

	for _, a := range e.alarms {
		switch a.State {
		case StateNew:
			stats.NewAlarms++
		case StateAcknowledged:
			stats.AcknowledgedAlarms++
		case StateInvestigating:
			stats.InvestigatingAlarms++
		case StateResolved:
			stats.ResolvedAlarms++
		}

		switch a.Severity {
		case SeverityCritical:
			stats.CriticalCount++
		case SeverityMajor:
			stats.MajorCount++
		case SeverityMinor:
			stats.MinorCount++
		case SeverityInfo:
			stats.InfoCount++
		}
	}
	return stats
}

// Rules CRUD
func (e *Engine) GetRules() []*AlarmRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make([]*AlarmRule, 0, len(e.rules))
	for _, r := range e.rules {
		res = append(res, r)
	}
	return res
}

func (e *Engine) UpsertRule(rule *AlarmRule, operator string) *AlarmRule {
	e.mu.Lock()
	defer e.mu.Unlock()
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	e.rules[rule.ID] = rule
	e.logger.Log(operator, "SYSTEM", "127.0.0.1", "ALARM_RULE_UPSERT", "ALARMS", "SUCCESS",
		fmt.Sprintf("Configured alarm rule %s [%s]", rule.ID, rule.Name), "")
	return rule
}

func (e *Engine) DeleteRule(id, operator string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.rules[id]; !ok {
		return fmt.Errorf("rule %s not found", id)
	}
	delete(e.rules, id)
	e.logger.Log(operator, "SYSTEM", "127.0.0.1", "ALARM_RULE_DELETE", "ALARMS", "SUCCESS",
		fmt.Sprintf("Deleted alarm rule %s", id), "")
	return nil
}

// ExportJSON exports all alarms matching filter criteria as formatted JSON
func (e *Engine) ExportJSON(f AlarmFilter) ([]byte, error) {
	filtered := e.FilterAlarms(f)
	return json.MarshalIndent(filtered, "", "  ")
}

// ExportCSV exports alarm history as comma-separated values for incident compliance
func (e *Engine) ExportCSV(f AlarmFilter) ([]byte, error) {
	filtered := e.FilterAlarms(f)
	var buf strings.Builder
	w := csv.NewWriter(&buf)

	// Write header
	header := []string{"Alarm ID", "Severity", "State", "Device ID", "Manufacturer", "Category", "Event Type", "Event Time", "Ack Status", "Assigned To", "Resolution"}
	_ = w.Write(header)

	for _, a := range filtered {
		record := []string{
			a.ID,
			string(a.Severity),
			string(a.State),
			a.DeviceID,
			a.Manufacturer,
			a.EventCategory,
			a.EventType,
			a.EventTimestamp.Format(time.RFC3339),
			a.AckStatus,
			a.AssignedTo,
			a.ResolutionNotes,
		}
		_ = w.Write(record)
	}
	w.Flush()
	return []byte(buf.String()), nil
}
