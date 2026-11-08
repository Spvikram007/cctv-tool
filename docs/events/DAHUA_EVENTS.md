# DAHUA CGI EVENT STREAMING
## eventManager.cgi Chunk Parser, WizSense AI Analytics & Event Mapping

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** `backend/pkg/events`  
**Phase:** 7  

---

## 1. Dahua CGI Event Stream Architecture

Dahua IP cameras, XVRs, and NVRs emit real-time event telemetry via an HTTP long-polling multipart chunked stream:
- **Endpoint:** `GET /cgi-bin/eventManager.cgi?action=attach&codes=[All]`
- **Alternative Specific Codes:** `&codes=[VideoMotion,CrossLineDetection,CrossRegionDetection,AlarmLocal]`
- **Content-Type:** `multipart/x-mixed-replace;boundary=myboundary`
- **Protocol:** HTTP 1.1 with HTTP Digest or Basic Authentication

The connection remains attached indefinitely. Each event is delimited by `--myboundary` and followed by key-value text pairs.

---

## 2. Event Chunk Stream Format

```
--myboundary
Content-Type: text/plain
Content-Length: 180

Code=CrossLineDetection;action=Start;index=0;data={
   "Direction" : "LeftToRight",
   "EventID" : 42,
   "PTS" : 3829104.0,
   "TrackID" : 101,
   "UTC" : 1794082080
}

--myboundary
Content-Type: text/plain
Content-Length: 140

Code=VideoBlind;action=Start;index=0;data={
   "EventID" : 43,
   "PTS" : 3829110.0,
   "UTC" : 1794082085
}
```

---

## 3. Dahua Event Code Mapping

The Dahua CGI parser converts raw Dahua codes into the standardized CCTV SENTINEL event taxonomy:

| Dahua `Code` | Event Description | Normalized Category | Default Severity | State Mapping |
|---|---|---|---|---|
| `VideoMotion` | Video Motion Detection | `MOTION` | `MINOR` | `action=Start` $\to$ `START`<br>`action=Stop` $\to$ `STOP` |
| `VideoBlind` | Camera occlusion / lens blocked | `TAMPER` | `CRITICAL` | `action=Start` $\to$ `START` |
| `VideoLoss` | Video signal absent / cable cut | `DEVICE_FAULT` | `CRITICAL` | `action=Start` $\to$ `START` |
| `CrossLineDetection` | Tripwire / Virtual line crossed | `LINE_CROSSING` | `MAJOR` | `action=Start` $\to$ `START` |
| `CrossRegionDetection` | Perimeter intrusion detection | `INTRUSION` | `MAJOR` | `action=Start` $\to$ `START` |
| `HumanDetection` | WizSense AI Human classification | `AI_ANALYTICS` | `MAJOR` | `action=Start` $\to$ `PULSE` |
| `VehicleDetection` | WizSense AI Motor vehicle classification | `AI_ANALYTICS` | `MAJOR` | `action=Start` $\to$ `PULSE` |
| `AlarmLocal` | Physical dry-contact alarm input | `IO_ALARM` | `INFO` | `action=Start` $\to$ `START` |
| `StorageFailure` | Hard drive failed or disconnected | `DEVICE_FAULT` | `CRITICAL` | `action=Start` $\to$ `CRITICAL` |
| `StorageLowSpace` | Hard disk free capacity < threshold | `DEVICE_FAULT` | `MAJOR` | `action=Start` $\to$ `MAJOR` |

---

## 4. WizSense Object Filtering

On Dahua WizSense series devices (e.g. `XVR5108HS-4KL-I3`), events carry JSON metadata within the `data={...}` block:
- `Object.ObjectType`: `"Human"` or `"Vehicle"`
- `BoundingBox`: Coordinate boundaries $[x, y, w, h]$
- `Speed`: Estimated target velocity

The parser extracts these attributes into the event's `details` map, allowing automated alarm rules to filter specifically on human intrusion vs vehicle parking violations.
