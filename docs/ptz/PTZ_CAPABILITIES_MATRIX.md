# Fleet PTZ Capabilities & Feature Support Matrix
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** Universal PTZ Robotic Engine (`backend/pkg/ptz/engine.go`)  
**Release:** Phase 7 & 7.1 Stabilization  
**Author:** Tech Defence Solutions  

---

## 1. Multi-Vendor PTZ Protocol Support

CCTV Sentinel Universal normalizes PTZ robotic operations across distinct vendor protocols into a unified API:

| Manufacturer / Protocol | Continuous Move | Absolute Positioning | Relative Positioning | Presets (Memory) | Patrol Tours (Cruise) | Optical Zoom | Auto-Focus / Iris |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **ONVIF Profile S / G** | Yes | Yes (Normalized) | Yes | Yes (1–255) | Yes | Yes | Yes |
| **Hikvision ISAPI** | Yes | Yes | Yes | Yes (1–300) | Yes (1–8) | Yes | Yes |
| **Dahua CGI** | Yes | Yes | Yes | Yes (1–80) | Yes (1–8) | Yes | Yes |
| **CP PLUS / OEM (Vatilon H43)**| Yes | Emulated | Emulated | Yes (1–128) | Yes (Software Loop) | Yes | Yes |
| **Fixed Cameras (All)** | **No** (Locked) | **No** (Locked) | **No** (Locked) | **No** | **No** | **No** | **No** |

---

## 2. Hardened Truth Rule Enforcement

Fixed focal-length cameras must never accept PTZ commands or pretend to move. CCTV Sentinel enforces strict capability gates:

1. **Rejection of PTZ on Fixed Units:**
   If a device does not declare `CategoryPTZCamera` or have verified PTZ service endpoints:
   - `ptzEng.GetCapabilities(dev)` returns `PTZSupported = false`.
   - Any move request returns HTTP 400 Bad Request with:
     ```json
     {
       "success": false,
       "error": {
         "code": "HARDWARE_NOT_SUPPORTED",
         "message": "Physical hardware unit is a fixed focal-length camera; robotic motor commands rejected"
       }
     }
     ```
2. **Speed & Boundary Clamping:**
   - Pan/Tilt continuous speed: strictly normalized to $[-1.0, 1.0]$.
   - Zoom velocity: $[-1.0, 1.0]$.
   - Out-of-bounds speed requests are clamped to max safe thresholds to prevent physical motor gear stripping.
3. **Emergency Stop Precedence:**
   - The `/ptz/stop` command bypasses pending action queues and delivers an immediate zero-velocity motor brake to all axes.

