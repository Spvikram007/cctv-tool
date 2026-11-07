# CP PLUS Compatibility & Architecture Report

## 1. Ecosystem Overview

**CP PLUS** (Aditya Infotech Ltd.) is one of the most widely deployed CCTV brands across India and Southeast Asia. The hardware portfolio spans:
- **Indigo & Orange Series**: Enterprise and commercial IP cameras (`CP-UNC-*`).
- **Cosmic Series**: Analog HD XVRs (`CP-UVR-*`).
- **Network Video Recorders**: High-density 4K NVRs (`CP-UNR-*`).

### 1.1 Underlying Protocol Stack

Empirical investigation of physical CP PLUS devices reveals that the firmware incorporates a dual-stack control architecture:
1. **Primary Protocol**: Dahua-derived CGI subsystem operating over `/cgi-bin/magicBox.cgi` and `/cgi-bin/configManager.cgi`.
2. **Standard Protocol**: ONVIF Profile S/T/G stack operating over `/onvif/device_service`.

---

## 2. Empirical Physical Bench Findings

During Phase 2.1 and Phase 3 physical testing on `192.168.31.23/24`, two physical CP PLUS units were validated:

### Target 1: CP PLUS IP Camera (`CP-UNC-TA21PL3-V3`) — `192.168.31.106:80`
- **MAC Address**: `28:18:fd:0b:74:61` (OUI assigned to Zhejiang Dahua Technology).
- **HTTP Probe**: Returns 200 OK.
- **ISAPI Probe**: Returns 404 Not Found (correctly rejects Hikvision proprietary protocols).
- **CGI Probe**:
  ```http
  HTTP/1.1 401 Unauthorized
  WWW-Authenticate: Digest realm="Login to 6FFD4C0EBE899260", qop="auth", nonce="55b96a22-23f3-4fbd-80a2-5196006028f8", opaque=""
  ```
- **ONVIF Probe**: Returns 200 OK with `Clock Skew: -1.7 sec`.
- **Capabilities**: Media: SUPPORTED, PTZ: UNSUPPORTED, Imaging: SUPPORTED, Events: SUPPORTED.

### Target 2: CP PLUS 16-Channel 4K NVR (`CP-UNR-4K4162-V2`) — `192.168.31.143:80`
- **MAC Address**: `14:07:08:9e:d0:f1`.
- **HTTP Probe**: Returns 200 OK.
- **CGI Probe**:
  ```http
  HTTP/1.1 401 Unauthorized
  WWW-Authenticate: Digest realm="Login to ff981ec992290c5fe110215131116c00", qop="auth", nonce="1407021915", opaque="f28b7fda9e571499f93837425c61c9f054dcd15f"
  ```
- **ONVIF Probe**: Returns 200 OK with `Clock Skew: -0.9 sec`.
- **Capabilities**: Media: SUPPORTED, PTZ: SUPPORTED, Recording: SUPPORTED, Events: SUPPORTED.

---

## 3. Model Classification Taxonomy

CCTV Sentinel Universal parses model strings to automatically configure channel capacity and feature limits:

| Model Prefix | Category | Typical Capacity | Analog Format Allowed |
| :--- | :--- | :--- | :--- |
| `CP-UNC-*` | `models.CategoryIPCamera` | 1 Channel | **No** (Rejected) |
| `CP-UNR-*` | `models.CategoryNVR` | 4, 8, 16, or 32 IP Channels | **No** (Rejected) |
| `CP-UVR-*` | `models.CategoryXVR` | 4, 8, 16 Hybrid Channels | **Yes** (CVI, TVI, AHD, CVBS) |
| `CP-VNR-*` | `models.CategoryNVR` | 4, 8, 16 Channels | **No** (Rejected) |

---

## 4. Hardware Safety Guards

- **Pure IP NVR Protection**: Calling `SetChannelAnalogMode` on a pure IP NVR like `CP-UNR-4K4162-V2` is explicitly blocked in software with `ErrUnsupportedOperation`, preventing technicians from attempting to send invalid coaxial commands to IP network channels.
- **Dual-Stack Fallback**: If a technician changes a password or firmware disables CGI, the adapter automatically switches to ONVIF WS-Security without requiring reconfiguration.

