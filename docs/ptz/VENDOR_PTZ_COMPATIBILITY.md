# VENDOR PTZ COMPATIBILITY MATRIX & HARDWARE BOUNDARIES
## Physical Camera Verification, Protocol Adapters & Locked Fixed Units

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** `backend/pkg/ptz`  
**Phase:** 7  

---

## 1. Physical Device Verification: PTZ vs. Fixed Cameras

CCTV SENTINEL strictly adheres to hardware realism: **Never pretend a fixed camera has PTZ motors.**

| IP Address | Manufacturer | Model | Category | PTZ Motorized? | Supported Operations | Capability Badge |
|---|---|---|---|---|---|---|
| `192.168.31.104` | **Vatilon** | **H43** | `PTZ_CAMERA` | **YES** | Pan, Tilt, Digital Zoom, Presets, Patrol, 360° Scan, Home | `VERIFIED PTZ MOTORIZED CAMERA` |
| `192.168.31.101` | Dahua | DH-P3B-PV | `IP_CAMERA` | **NO** | Rejects move with HTTP 400 | `FIXED CAMERA - CONTROLS LOCKED` |
| `192.168.31.102` | Dahua | DH-IPC-HDW1230T1-A-S5 | `IP_CAMERA` | **NO** | Rejects move with HTTP 400 | `FIXED CAMERA - CONTROLS LOCKED` |
| `192.168.31.103` | Hikvision | DS-2CD1023G0-IU | `IP_CAMERA` | **NO** | Rejects move with HTTP 400 | `FIXED CAMERA - CONTROLS LOCKED` |
| `192.168.31.106` | CP PLUS | CP-UNC-TA21PL3-V3 | `IP_CAMERA` | **NO** | Rejects move with HTTP 400 | `FIXED CAMERA - CONTROLS LOCKED` |
| `192.168.31.107` | Dahua | DH-IPC-HFW1249S1-A-IL | `IP_CAMERA` | **NO** | Rejects move with HTTP 400 | `FIXED CAMERA - CONTROLS LOCKED` |
| `192.168.31.110` | Dahua | DH-IPC-HFW1230T1P-ZS-S4 | `IP_CAMERA` | **NO (Motorized Varifocal Zoom Only)** | Rejects Pan/Tilt move with HTTP 400 | `FIXED CAMERA - CONTROLS LOCKED` |
| `192.168.31.143` | CP PLUS | CP-UNR-4K4162-V2 | `NVR` | **NO (Pass-through)** | Channel PTZ dispatch only | `NVR RECORDER` |

---

## 2. Fixed Camera Rejection Mechanism

When any client attempts a PTZ movement on a fixed camera:
```go
if dev.Category != models.CategoryPTZCamera && !strings.Contains(strings.ToLower(dev.Name), "ptz") {
    return errFixedCamera // "device is a fixed camera without PTZ motors"
}
```
The REST API responds with:
- **HTTP Status:** `400 Bad Request`
- **JSON Payload:**
  ```json
  {
    "error": "device is a fixed camera without PTZ motors",
    "device_id": "dev-192-168-31-101",
    "ptz_supported": false
  }
  ```
The frontend UI inspects `capabilities.ptz_supported` upon selection and renders:
- An amber capability banner explaining that the physical unit has no pan/tilt motors.
- Fully disabled, locked D-pad and zoom controls.
- Prevention of accidental socket flood or error spamming.

---

## 3. Protocol Adapters

### 3.1 ONVIF PTZ Service (Profile S / Profile T)
- **ContinuousMove:** Dispatches `<tptz:ContinuousMove>` with `<tt:Velocity>` pan/tilt/zoom vector.
- **Stop:** Dispatches `<tptz:Stop><tptz:PanTilt>true</tptz:PanTilt><tptz:Zoom>true</tptz:Zoom></tptz:Stop>`.
- **AbsoluteMove / RelativeMove:** Supported where camera profile advertises positional limits.
- **Preset Management:** `<tptz:SetPreset>`, `<tptz:GotoPreset>`, `<tptz:RemovePreset>`.

### 3.2 Hikvision ISAPI PTZ
- **Endpoints:**
  - `PUT /ISAPI/PTZCtrl/channels/{ch}/continuous` — XML `<PTZData><pan>{x}</pan><tilt>{y}</tilt><zoom>{z}</zoom></PTZData>`.
  - `PUT /ISAPI/PTZCtrl/channels/{ch}/presets/{id}/goto` — Instant preset jump.
  - `PUT /ISAPI/PTZCtrl/channels/{ch}/presets/{id}` — Set preset position.

### 3.3 Dahua CGI PTZ
- **Endpoints:**
  - Start Continuous: `GET /cgi-bin/ptz.cgi?action=start&channel={ch}&code={Up|Down|Left|Right|ZoomWide|ZoomTele}&arg1=0&arg2={speed}&arg3=0`
  - Stop: `GET /cgi-bin/ptz.cgi?action=stop&channel={ch}&code={Up|Down|Left|Right|ZoomWide|ZoomTele}`
  - Preset Goto: `GET /cgi-bin/ptz.cgi?action=start&channel={ch}&code=GotoPreset&arg1=0&arg2={num}&arg3=0`

### 3.4 CP PLUS Coaxial PTZ & IP
- **Coaxial UTC:** Sent via DVR/XVR coax control channel over video cable.
- **IP Series:** Dispatched via ONVIF Profile S or CP PLUS CGI protocol.
