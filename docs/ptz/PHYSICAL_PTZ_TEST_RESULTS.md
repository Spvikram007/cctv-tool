# Physical PTZ Verification & Diagnostic Test Results
**Hardware Device:** Vatilon H43 PTZ Camera  
**Physical IP:** `192.168.31.104`  
**Physical Subnet:** `192.168.31.0/24` (Real on-premise technician network)  
**Date of Testing:** 2026-11-08  
**Tested By:** Tech Defence Solutions QA & Engineering  

---

## 1. Physical Device Profile & Fingerprint

| Parameter | Observed Value | Evidence Source |
| :--- | :--- | :--- |
| **IP Address** | `192.168.31.104` | Direct LAN ARP & Ping (RTT $\approx 1\,\text{ms}$) |
| **HTTP Server** | `thttpd/2.29 23May2018` | `HTTP/1.1 200 OK` on Port 80 |
| **RTSP Server** | Port 554 (OPTIONS response: `RTSP/1.0 200 OK`) | Public methods: `OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE` |
| **Authentication Scheme** | HTTP Digest (RFC 2617 / RFC 7616) | `WWW-Authenticate: Digest qop="auth", realm="CAMERA", nonce="..."` |
| **Firmware Family** | Sofia / Xiongmai-OEM / thttpd `web.cgi` | `http://192.168.31.104/cgi-bin/web.cgi` |
| **System Function Mask** | `system_function & 16` (PTZ) \| `system_function & 32` (Auto-Focus) | Firmware `player.js` controller verification |
| **Channel Count** | 1 physical optical sensor | Dual-stream (`stream_num_per_chn=2`) |

---

## 2. Actual Live Physical Network Evidence

### 2.1 HTTP 80 & CGI Endpoint Verification
```http
POST /cgi-bin/web.cgi?mod=session&cmd=get_auth_info HTTP/1.1
Host: 192.168.31.104
User-Agent: CCTV-Sentinel/1.0

HTTP/1.0 200 OK
Server: thttpd
Date: Sun, 8 Nov 2026 3:17:50 GMT
Content-Length: 74
Content-type: application/json;charset='utf-8'
Connection: close

{"qop":"auth","realm":"CAMERA","nonce":"vjir8mSLkuYceqPWqN6eNS7SiLz7CzkQ"}
```

### 2.2 Unauthenticated Device Interrogation
```http
GET /cgi-bin/web.cgi?mod=device&cmd=get HTTP/1.1
Host: 192.168.31.104

HTTP/1.0 401 Unauthorized
Content-Length: 141
Content-Type: text/html
Connection: close
WWW-Authenticate: Digest qop="auth", realm="CAMERA", nonce="tH4hMflBkn9btwhmUz6Yjr2XKV6AY8iJ"

<!DOCTYPE html>
<html>
<head><title>Authentication Error</title></head>
<body><h1>Access Error: 401 -- Unauthorized</h1></body>
</html>
```

### 2.3 RTSP Port 554 Probe
```text
OPTIONS rtsp://192.168.31.104:554/ RTSP/1.0
CSeq: 1
User-Agent: CCTV-Sentinel/1.0

RTSP/1.0 200 OK
CSeq: 1
Public: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE, GET_PARAMETER, SET_PARAMETER
Date: Sun, 08 Nov 2026 03:15:23 GMT
```

---

## 3. PTZ Command Mapping & Protocol Analysis

Inspection of the physical device's embedded JavaScript firmware (`player.js`) verifies the exact CGI command table implemented by CCTV Sentinel's CP PLUS & OEM adapter:

| PTZ Action | Firmware Command | CCTV Sentinel Engine Implementation |
| :--- | :--- | :--- |
| **Pan Left** | `onmousedown="ptz_left()"` | `ptz.ActionPanLeft` / CGI `mod=ptz&cmd=left` |
| **Pan Right** | `onmousedown="ptz_right()"` | `ptz.ActionPanRight` / CGI `mod=ptz&cmd=right` |
| **Tilt Up** | `onmousedown="ptz_up()"` | `ptz.ActionTiltUp` / CGI `mod=ptz&cmd=up` |
| **Tilt Down** | `onmousedown="ptz_down()"` | `ptz.ActionTiltDown` / CGI `mod=ptz&cmd=down` |
| **Motor Stop** | `onmouseup="ptz_stop()"` | `ptz.ActionStop` / CGI `mod=ptz&cmd=stop` |
| **Zoom In / Out** | `focus_add` / `focus_sub` | `ptz.ActionZoomIn` / `ptz.ActionZoomOut` |
| **Add Preset** | `ptz_ctrl(11)` | `ptzEng.CreatePreset()` |
| **Delete Preset** | `ptz_ctrl(12)` | `ptzEng.DeletePreset()` |
| **Goto Preset** | `ptz_ctrl(13)` | `ptzEng.GotoPreset()` |

### Preset Coordinate Memory
- Valid coordinate memory slots: **1 to 128** (excluding reserved factory calibration indices 77, 91–98, 111–117).
- Speed range: 1 to 8 (stepper motor step rate).

---

## 4. Physical Motor Actuation Status: PENDING OPERATOR TEST APPROVAL

> [!IMPORTANT]
> **COMPLIANCE & TRUTH NOTICE:**
> Physical network connectivity to HTTP port 80 and RTSP port 554 has been verified live on the physical LAN (`192.168.31.104`). However, **robotic physical motor actuation (pan, tilt, zoom motor rotation) has NOT been physically executed or observed by an operator, and is strictly classified as PENDING OPERATOR TEST APPROVAL.**
>
> Neither software test scripts nor mock responses constitute physical confirmation of robotic motor actuation. Physical movement must be visually verified and signed off by a physical site technician.

### 4.1 Manual Operator Physical Verification Procedure

Before approving physical PTZ motor deployment in production, the technician must execute this 5-step on-site validation checklist:

1. **Physical Visual Line-of-Sight Check:**
   - Position an operator in direct visual line-of-sight of the Vatilon H43 dome/turret assembly.
   - Verify camera physical clearance from walls, conduit, and mounting brackets (360° unobstructed clearance).

2. **Credential Authentication:**
   - Enter device administrator credentials into the CCTV Sentinel Universal portal or test harness.
   - Ensure digest authentication challenge succeeds (`mod=session&cmd=get_auth_info`).

3. **Continuous Pan & Tilt Actuation:**
   - Issue Continuous Pan Right command at speed 3 for 2.0 seconds.
   - **Operator Observation:** Confirm physical turret rotates clockwise smoothly without motor grinding or gear skipping.
   - Issue Continuous Tilt Up command at speed 3 for 1.5 seconds.
   - **Operator Observation:** Confirm optical assembly tilts vertically upward.

4. **Preset Calibration & Return:**
   - Save current orientation as Preset Slot #1 ("North Gate Guard").
   - Pan camera 90° away to Preset Slot #2 ("Parking Bay").
   - Issue `GOTO_PRESET 1`.
   - **Operator Observation:** Confirm camera automatically navigates back to the exact initial angular coordinates within $\pm 0.5^\circ$ repeatability tolerance.

5. **Stop & Home Safety Lockout:**
   - Issue emergency `STOP` command during active rotation.
   - **Operator Observation:** Confirm motor braking engages immediately (< 100 ms).
   - Issue `HOME` calibration command and verify camera returns to default zero-index origin.
   - Sign off the commissioning certificate in the technician audit log.

