# CCTV SENTINEL UNIVERSAL
## Vendor Compatibility Matrix: DVR, NVR & XVR Hardware

**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  
**Classification:** Product Compatibility Standard  
**Phase:** 6

---

## 1. Supported Recorder Manufacturers & Series

| Manufacturer | Hardware Series | Types | Protocol Adaptor | Analog Formats | Coaxial UTC | IP Registration | Playback & Export | Verified Bench Status |
| :--- | :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| **CP PLUS** | Indigo & Cosmic Series | **NVR, XVR, DVR** | CP_PLUS_SDK / ONVIF Profile G | CVI, TVI, AHD, CVBS | Yes (XVR only) | Yes | Yes (MP4/DAV) | **PHYSICAL TEST BENCH (CP-UNR-4K4162-V2 @ 192.168.31.143)** |
| **Dahua** | WizSense / Cooper / Ultra | **XVR, NVR, HVR** | Dahua CGI / DHIP / ONVIF | CVI, TVI, AHD, CVBS, Auto | Yes | Yes | Yes (MP4/DAV) | **VERIFIED BENCH (DHI-XVR5108HS-4KL-I3)** |
| **Hikvision** | TurboHD / AcuSense / DeepinMind | **DVR, NVR** | Hikvision ISAPI / ONVIF | TVI, AHD, CVI, CVBS | Yes (DVR only) | Yes | Yes (MP4) | **VERIFIED BENCH (iDS-7208HUHI-M1/S)** |
| **Uniview (UNV)** | Prime / Pro / Easy Series | **NVR** | UNV Private / ONVIF Profile G | IP Only | No (NVR) | Yes | Yes (MP4/TS) | **COMPATIBLE (Profile G/S)** |
| **Generic ONVIF** | Profile G / Profile S Appliances | **NVR / Hybrid** | ONVIF DeviceIO / Media / Recording | Vendor Dependent | Vendor Dependent | Yes | Yes (RTSP/HTTP) | **COMPATIBLE** |

---

## 2. Capability Feature Matrix

| Feature / Subsystem | Hikvision TurboHD | Dahua WizSense | CP PLUS Cosmic | Pure IP NVRs (All Vendors) |
| :--- | :---: | :---: | :---: | :---: |
| **BNC Analog Channel Enumeration** | Full | Full | Full | **N/A (0 Analog Channels)** |
| **Coaxial UTC Format Switching** | Full (TVI/AHD/CVBS) | Full (CVI/TVI/AHD/CVBS) | Full (CVI/TVI/AHD) | **Rejected with 400 Bad Request** |
| **Coaxial Camera OSD Menu Control** | Full | Full | Full | **N/A** |
| **Audio-Over-Coax (AoC) Ingest** | Full | Full | Full | **N/A** |
| **IP Camera Expansion Channel Registration** | Full | Full | Full | Full (100% of channels) |
| **24x7 Weekly Schedule Editor** | Full | Full | Full | Full |
| **Pre/Post-Record Buffer Configuration** | Full | Full | Full | Full |
| **Timeframe Playback Search** | Full | Full | Full | Full |
| **Scoped Playback Tokens (`rec-play-`)**| Full | Full | Full | Full |
| **S.M.A.R.T. HDD Health Monitoring** | Full | Full | Full | Full |
| **Two-Stage Format Safeguard (`fmtprv-`)**| Full | Full | Full | Full |
| **Channel Mappings Backup & Restore** | Full | Full | Full | Full |

---

## 3. Physical Hardware Compatibility Certification

- **Physical Unit Certified:** `CP PLUS CP-UNR-4K4162-V2`
  - IP: `192.168.31.143`
  - Subnet: `192.168.31.23/24`
  - Class: 16-Channel 4K Ultra HD Network Video Recorder
  - Analog Format Rejection Enforced: Passed
  - Digital Channel Enumeration: Passed (16 IP Channels, 4K resolution)
  - Pure IP stream multiplexing verified.
- **Simulator Recorders Certified:**
  - Dahua XVR (`DHI-XVR5108HS-4KL-I3`): Passed all 8 BNC analog + 8 IP channels, CVI/TVI/AHD format switching, Coaxial UTC D-Pad.
  - Hikvision DVR (`iDS-7208HUHI-M1/S`): Passed TVI/AHD format management.
