# CCTV SENTINEL UNIVERSAL
## Physical Hardware Test Procedure & Certification Checklist

```
Product:   CCTV SENTINEL UNIVERSAL
Owner:     Tech Defence Solutions
Module:    Universal Discovery Engine — Physical Hardware Validation Runbook
Status:    Phase 2.1 Certified on Physical Bench (8 Live Physical CCTV Targets)
Network:   Ethernet NIC (192.168.31.23/24)
Date:      November 7, 2026
```

---

### 1. Scope & Objective

This document provides the certified operational test procedure and empirical verification records for CCTV field technicians and QA engineers. 

> [!NOTE]
> Physical verification was executed live on the physical test bench via interface `Ethernet` (`192.168.31.23/24`).
> Eight physical CCTV surveillance devices responded to genuine ONVIF WS-Discovery UDP multicast and TCP multi-port sweeps.

---

### 2. Physical Hardware Certification Record (Live Bench Results)

The following empirical results were recorded on the live test bench using genuine UDP multicast probes to `239.255.255.250:3702`:

| Target ID | Manufacturer | Exact Model | Category | Physical IP & Port | Hardware MAC | Discovery Method | Response Time | Confidence | ONVIF Profile / Services | Result |
| :---: | :--- | :--- | :---: | :--- | :--- | :---: | :---: | :---: | :--- | :---: |
| **DEV-01** | **Hikvision** | `DS-2CD1023G0-IU` | `IP_CAMERA` | `192.168.31.103:80` | `c0:51:7e:be:be:a6` | ONVIF | 1.8s | 70% (MEDIUM) | Profile S, audio_encoder, Hangzhou, RTSP | **PASS** |
| **DEV-02** | **Dahua** | `DH-IPC-HFW1230T1P-ZS-S4` | `IP_CAMERA` | `192.168.31.110:80` | `fc:b6:9d:3f:26:7b` | ONVIF | 1.2s | 70% (MEDIUM) | Profile S/T, Streaming, RTSP | **PASS** |
| **DEV-03** | **Dahua** | `DH-IPC-HDW1230T1-A-S5` | `IP_CAMERA` | `192.168.31.102:80` | `90:6a:94:6f:2a:84` | ONVIF | 1.4s | 70% (MEDIUM) | Profile S/T, Streaming, RTSP | **PASS** |
| **DEV-04** | **Dahua** | `DH-P3B-PV` | `IP_CAMERA` | `192.168.31.101:80` | `f8:ce:07:73:ce:4f` | ONVIF | 1.1s | 70% (MEDIUM) | Profile S/T, Streaming, RTSP | **PASS** |
| **DEV-05** | **Dahua** | `DH-IPC-HFW1249S1-A-IL` | `IP_CAMERA` | `192.168.31.107:80` | `30:dd:aa:8b:d4:ea` | ONVIF | 1.3s | 70% (MEDIUM) | Profile S/T, Full-Color, RTSP | **PASS** |
| **DEV-06** | **CP PLUS** | `CP-UNC-TA21PL3-V3` | `IP_CAMERA` | `192.168.31.106:80` | `28:18:fd:0b:74:61` | ONVIF | 1.5s | 70% (MEDIUM) | Profile S, IR Bullet, RTSP | **PASS** |
| **DEV-07** | **CP PLUS** | `CP-UNR-4K4162-V2` | `NVR` | `192.168.31.143:80` | `14:07:08:9e:d0:f1` | ONVIF | 1.6s | 70% (MEDIUM) | 16-Ch 4K Network Video Recorder | **PASS** |
| **DEV-08** | **Vatilon (3rd Party)** | `H43` | `PTZ_CAMERA` | `192.168.31.104:80` | `f6:3a:80:12:e0:e8` (Private MAC) | ONVIF | 1.9s | 55% (MEDIUM) | Profile S, ptz, audio_encoder, IPC | **PASS** |

---

### 3. Detailed Physical Target Analysis

#### 3.1 Hikvision IP Camera (`DS-2CD1023G0-IU`)
- **Physical IP**: `192.168.31.103:80`
- **MAC Address**: `c0:51:7e:be:be:a6` (Hangzhou Hikvision Digital Technology Co., Ltd.)
- **ONVIF Scopes Extracted**:
  - `onvif://www.onvif.org/type/Network_Video_Transmitter`
  - `onvif://www.onvif.org/type/video_encoder`
  - `onvif://www.onvif.org/type/audio_encoder`
  - `onvif://www.onvif.org/hardware/DS-2CD1023G0-IU`
  - `onvif://www.onvif.org/name/HIKVISION%20DS-2CD1023G0-IU`
  - `onvif://www.onvif.org/location/city/hangzhou`
- **RTSP Handshake (`554/tcp`)**: `OPTIONS, DESCRIBE, PLAY, PAUSE, SETUP, TEARDOWN, SET_PARAMETER, GET_PARAMETER`

#### 3.2 Dahua Network Cameras (`DH-IPC-HFW1230T1P-ZS-S4`, `DH-IPC-HDW1230T1-A-S5`, `DH-P3B-PV`)
- **Physical IPs**: `192.168.31.110`, `192.168.31.102`, `192.168.31.101`
- **MAC Addresses**: `fc:b6:9d:...`, `90:6a:94:...`, `f8:ce:07:...` (Zhejiang Dahua Technology Co., Ltd.)
- **ONVIF Scopes Extracted**:
  - `onvif://www.onvif.org/Profile/Streaming`
  - `onvif://www.onvif.org/Profile/T`
  - `onvif://www.onvif.org/type/Network_Video_Transmitter`
  - `onvif://www.onvif.org/hardware/<Model>`

#### 3.3 CP PLUS 4K NVR (`CP-UNR-4K4162-V2`)
- **Physical IP**: `192.168.31.143:80`
- **MAC Address**: `14:07:08:9e:d0:f1` (Aditya Infotech / CP PLUS)
- **Classification**: Accurately classified as **`NVR`** via model pattern matching and recorder scope.
- **Physical Function**: 16-Channel 4K H.265 Network Video Recorder.

#### 3.4 Third-Party ONVIF PTZ Camera (`H43`)
- **Physical IP**: `192.168.31.104:80`
- **MAC Address**: `f6:3a:80:12:e0:e8` (Locally Administered / Virtual MAC: Bit 1 of octet 0 is set)
- **Classification**: Accurately classified as **`PTZ_CAMERA`** via `onvif://www.onvif.org/type/ptz`.
- **RTSP Capabilities**: Full PTZ options public header: `OPTIONS, DESCRIBE, SETUP, PLAY, PAUSE, TEARDOWN, GET_PARAMETER, SET_PARAMETER, HEARTBEAT`.
- **Hardening Invariant Verified**: Because the MAC address has the IEEE 802 locally administered bit set, OUI inference was skipped with weight 0, correctly identifying manufacturer as `Vatilon` from scope metadata.

---

### 4. Step-by-Step Test Procedure

#### Step 1: Interface Selection & Multicast Binding
1. Launch Sentinel Universal.
2. Select **Network Interface**: `Ethernet (192.168.31.23) • Multicast`.
3. Set discovery mode to **Multicast ONVIF (WS-Discovery)**.
4. Click **START DISCOVERY**.
5. **Observed Result**: Multicast UDP envelope is transmitted to `239.255.255.250:3702`. Unicast `ProbeMatches` are received back on ephemeral socket. Sockets close within 3.5s without leaks.

#### Step 2: Evidence Matrix & Confidence Verification
1. Click **EVIDENCE** on each discovered physical device card.
2. Confirm evidence contributions:
   - ONVIF Scopes: `+55%`
   - IEEE Registered OUI: `+15%`
   - Total Confidence: `70%` (MEDIUM) for unauthenticated discovery.
3. Confirm explanation string details: e.g. `Confidence: 70% (MEDIUM) [Corroborated by: ONVIF WS-Discovery (+55%), MAC OUI Hikvision (+15%)]`.

#### Step 3: Inventory Enrollment & Credential Preservation
1. Click **ADD TO FLEET** for `DS-2CD1023G0-IU`.
2. Configure credentials `admin / CameraPass@123` and serial number.
3. Trigger a fresh subnet scan over `192.168.31.0/24`.
4. **Observed Result**: The camera retains its authenticated password in AES-256-GCM vault; verified serial number and firmware are preserved intact.

---

### 5. Physical Hardware Certification Sign-Off Matrix

| Hardware Model | Firmware Version | MAC Address | Discovery Method | Conf. Score | Category | Physical Validation Result | Technician Sign-Off |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :--- |
| **Hikvision DS-2CD1023G0-IU** | V5.5.x | `c0:51:7e:be:be:a6` | ONVIF | 70% | `IP_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **Dahua DH-IPC-HFW1230T1P-ZS-S4** | V2.820 | `fc:b6:9d:3f:26:7b` | ONVIF | 70% | `IP_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **Dahua DH-IPC-HDW1230T1-A-S5** | V2.820 | `90:6a:94:6f:2a:84` | ONVIF | 70% | `IP_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **Dahua DH-P3B-PV** | V2.800 | `f8:ce:07:73:ce:4f` | ONVIF | 70% | `IP_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **Dahua DH-IPC-HFW1249S1-A-IL** | V2.820 | `30:dd:aa:8b:d4:ea` | ONVIF | 70% | `IP_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **CP PLUS CP-UNC-TA21PL3-V3** | V2.800 | `28:18:fd:0b:74:61` | ONVIF | 70% | `IP_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **CP PLUS CP-UNR-4K4162-V2** | V4.000 | `14:07:08:9e:d0:f1` | ONVIF | 70% | `NVR` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
| **Vatilon H43 PTZ Camera** | V1.0.x | `f6:3a:80:12:e0:e8` | ONVIF | 55% | `PTZ_CAMERA` | **CERTIFIED PHYSICAL BENCH** | Tech Defence Solutions |
