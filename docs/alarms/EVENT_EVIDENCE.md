# EVENT EVIDENCE & THE CCTV SENTINEL TRUTH RULE
## Independent Timestamps, Latency Audit, SHA-256 Hashing & Legal Admissibility

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** `backend/pkg/alarms`  
**Phase:** 7  

---

## 1. The CCTV SENTINEL Truth Rule

In security and law enforcement investigations, claiming that a secondary snapshot was captured at the exact microsecond of a sensor alert when network latency intervened is a critical forensic violation.

CCTV SENTINEL strictly implements **The Truth Rule**:
1. **Camera Sensor Timestamp (`Alarm.EventTimestamp`):**
   - Extracted directly from the camera's internal hardware clock embedded in the XML/JSON event notification payload.
   - Represents the physical moment the motion sensor, tripwire, or video blind detector tripped.
2. **Snapshot Capture Timestamp (`Alarm.SnapshotCaptureTime`):**
   - Recorded at the moment the JPEG frame was pulled from the camera via HTTP GET or RTSP snapshot socket.
3. **Latency Delta ($\Delta t$):**
   $$\Delta t = T_{\text{snapshot}} - T_{\text{event}}$$
   The UI and export reports explicitly present both timestamps and the measured delta (e.g. $+0.24\text{s}$).

---

## 2. Evidence Integrity & Cryptographic Hashing

To guarantee chain-of-custody integrity for forensic investigations:
1. Every captured snapshot evidence frame is digested using **SHA-256**.
2. The hash is saved alongside the snapshot metadata in `Alarm.SnapshotEvidenceID`:
   `SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
3. If an evidence file is modified on disk by an unauthorized party, subsequent verification detects the mismatch and raises a tampering warning.

---

## 3. CSV & JSON Forensic Audit Export

The engine supports instant unredacted evidence log exports via `GET /api/v1/alarms/export?format={csv|json}`.

### CSV Columns:
- `AlarmID`: Unique identifier
- `DeviceID`: Hardware identity
- `DeviceName`: Camera label
- `Category`: `MOTION`, `TAMPER`, `LINE_CROSSING`, etc.
- `Severity`: `CRITICAL`, `MAJOR`, `MINOR`, `INFO`
- `State`: `NEW`, `ACKNOWLEDGED`, `RESOLVED`, `DISMISSED`
- `EventTimestamp`: Exact ISO-8601 camera sensor time
- `SnapshotTime`: Exact ISO-8601 frame acquisition time
- `OccurrenceCount`: Number of duplicate events suppressed by cooldown
- `Description`: Incident summary
- `AssignedTechnician`: Responsible field technician
- `ResolutionNotes`: Technician closing documentation
- `ResolvedBy`: Sign-off technician identity
- `SnapshotURL`: Relative path to forensic image
