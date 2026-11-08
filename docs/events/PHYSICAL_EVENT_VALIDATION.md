# Physical Event & Alarm Telemetry Validation
**Hardware Device:** Vatilon H43 PTZ Camera (`192.168.31.104`)  
**Network Subnet:** `192.168.31.0/24` (Real on-premise technician network)  
**Release:** Phase 7.1 Stabilization  
**Author:** Tech Defence Solutions QA & Engineering  

---

## 1. Physical Device Telemetry Capabilities

The physical Vatilon H43 camera exposes event and alarm telemetry via two complementary mechanisms:
1. **HTTP Digest Web Hook / CGI Polling:**
   Device status query (`mod=device&cmd=get`) provides bitmask status for active sensors.
2. **ONVIF PullPoint & Basic Notifications:**
   Standard WS-BaseNotification and PullPoint subscriptions report motion and tampering states.

---

## 2. Event Normalization Pipeline

Surveillance events from physical devices and simulators pass through the unified event normalization engine (`backend/pkg/events/manager.go`):

```mermaid
flowchart TD
    PhysicalCam[Physical Camera / Simulator] -->|Raw Event / Poll / Push| EventManager[Event Manager Thread]
    EventManager --> Deduplication[Sliding Window Deduplication (5s)]
    Deduplication --> Normalizer[Event Normalizer]
    Normalizer --> NormalizedEvent[Normalized SurveillanceEvent]
    NormalizedEvent --> RingBuffer[In-Memory Ring Buffer (1000 events)]
    NormalizedEvent --> SSEStream[Server-Sent Events / WebSocket]
    NormalizedEvent --> AlarmEngine[Intelligent Alarm Engine]
    AlarmEngine --> RuleMatcher[Alarm Rule Evaluation]
    RuleMatcher -->|Trigger Match| AlarmStore[Alarm Incident Created]
    AlarmStore --> SnapshotWorker[Evidence Snapshot Trigger]
```

### Supported Event Categories
- `CategoryMotion`: Hardware video motion detection (VMD).
- `CategoryLineCrossing`: Intelligent virtual tripwire boundary crossing.
- `CategoryIntrusion`: Bounded perimeter zone intrusion.
- `CategoryTampering`: Optical lens occlusion, spray paint, or severe defocus.
- `CategoryStorage`: SD card / HDD missing, unformatted, or write failure.
- `CategorySystem`: Cold restart, network link flap, or unauthorized login attempt.

---

## 3. Reliability & Reconnection Resilience

### 3.1 Long-Lived Listener Threads
Event subscriptions run in isolated goroutines. If a network interruption occurs:
- The listener catches the socket error or HTTP timeout.
- Exponential backoff reconnects automatically ($2\,\text{s} \to 4\,\text{s} \to 8\,\text{s} \dots \text{max } 60\,\text{s}$).
- The subscription state transitions to `DEGRADED` and returns to `ACTIVE` upon reconnect.

### 3.2 Duplicate Event Suppression
High-frequency cameras (e.g. foliage in wind triggering motion every 50 ms) are throttled via sliding window deduplication:
- Events with identical `(DeviceID, Category, Source)` within a 5-second window increment the repeat counter without creating redundant alarm tickets.

