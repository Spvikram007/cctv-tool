# CCTV SENTINEL UNIVERSAL
## Recording Schedule & Storage Plan Engine

**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  
**Module:** `backend/pkg/recorder`  
**Classification:** System Engineering Guide

---

## 1. 24x7 Weekly Schedule Matrix Architecture

Recorders manage storage allocation by enforcing time-based recording rules per channel. CCTV Sentinel Universal models this as a $7 \times 24$ hourly matrix where each day (Sunday through Saturday) contains 24 one-hour blocks:

```go
type RecordingSchedule struct {
    RecorderID     string             `json:"recorder_id"`
    ChannelID      int                `json:"channel_id"`
    ChannelName    string             `json:"channel_name"`
    PreRecordSec   int                `json:"pre_record_sec"`  // 0 - 30 seconds
    PostRecordSec  int                `json:"post_record_sec"` // 10 - 300 seconds
    WeeklySchedule map[int][24]string `json:"weekly_schedule"` // Day (0=Sun..6=Sat) -> 24 hours of RecordMode
    HolidayEnabled bool               `json:"holiday_enabled"`
}
```

---

## 2. Supported Recording Modes

| Recording Mode | Color Code | Trigger Condition | Storage Impact |
| :--- | :--- | :--- | :--- |
| `CONTINUOUS` | **Emerald (Green)** | Always recording at designated frame rate & bitrate | High (Constant storage generation) |
| `MOTION` | **Amber (Yellow)** | Video motion detection (VMD) or AI SMD Plus trigger | Low to Medium (50%–80% storage savings) |
| `ALARM` | **Rose (Red)** | External sensor input (PIR, beam sensor, magnetic contact) | Low (Only active during security alerts) |
| `MANUAL` | **Blue** | Operator manually initiates forced recording from console | Ad-hoc (Incident logging) |
| `DISABLED` | **Slate (Dark)** | Recording suspended for this hour | Zero storage consumption |

---

## 3. Pre-Record and Post-Record Buffers

- **Pre-Record Buffer (0 – 30 seconds)**: Recorders continuously buffer video in volatile RAM. When motion or an alarm event occurs, the pre-record buffer is prepended to the recording segment. This captures the critical seconds *before* the intruder entered the frame.
- **Post-Record Buffer (10 – 300 seconds)**: After motion ceases, the recorder continues writing video to disk for the designated hold period to ensure all secondary activity is captured without segment fragmentation.
