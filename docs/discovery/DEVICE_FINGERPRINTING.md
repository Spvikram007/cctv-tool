# CCTV SENTINEL UNIVERSAL
## Confidence-Weighted Device Fingerprinting & Classification Engine

```
Product:   CCTV SENTINEL UNIVERSAL
Owner:     Tech Defence Solutions
Module:    Universal Discovery Engine — Modular Fingerprinter & Classifier
Status:    Implemented & Certified in Development Build (Phase 2)
```

---

### 1. Architectural Philosophy

In surveillance networking, **no single heuristic is 100% reliable**:
- OEM rebranding means a camera with a Hikvision MAC prefix may run Dahua firmware or customized OEM software.
- Default HTTP banners may simply report generic web server strings (`webserver`, `App-webs/1.0`, `lighttpd`).
- Multicast WS-Discovery may be blocked by managed PoE switch VLAN boundaries or IGMP snooping.

To deliver carrier-grade reliability without brute-forcing passwords or violating network boundaries, CCTV Sentinel Universal implements a **Confidence-Weighted Multi-Factor Evidence Matrix**:

```
                       [RAW NETWORK DATA]
                                |
      +-------------------------+-------------------------+
      |                         |                         |
[MAC OUI Table]         [HTTP/S Headers]           [RTSP Server]
 (Prefix Lookup)       (Server/Auth/Cert)        (OPTIONS Banner)
      |                         |                         |
 (+30% Max)                (+25% Max)                (+30% Max)
      |                         |                         |
      +-------------------------+-------------------------+
                                |
      +-------------------------+-------------------------+
      |                                                   |
[Port Signatures]                               [ONVIF WS-Discovery]
(8000/37777/34567)                              (Scopes/Types/Model)
      |                                                   |
 (+20% Max)                                          (+55% Max)
      +---------------------------------------------------+
                                |
                                v
               [CONFIDENCE ARBITRATION ENGINE]
                   Score: 0 - 100 Percentage
            Classification: HIGH / MEDIUM / LOW Level
```

---

### 2. Evidence Sources & Weight Allocation

| Evidence Source | Weight | Significance | Notes |
| :--- | :---: | :--- | :--- |
| **ONVIF WS-Discovery Scopes** | **+55%** | Definitive Profile | Hardware model, vendor name, and capabilities embedded in standard URI scopes. |
| **MAC OUI Prefix Match** | **+30%** | Probable Vendor | **Never 100% on its own**. Identifies hardware manufacturer prefix (Hikvision, Dahua, CP PLUS, Axis, Uniview). |
| **RTSP OPTIONS Server Banner** | **+20% - 30%** | Strong Protocol | Direct protocol response from RTSP stack (e.g. `Server: Hikvision RTSP Server`, `Server: Dahua RTSP Server`). |
| **HTTP Server & WWW-Authenticate** | **+15% - 25%** | Medium to Strong | Inspects server banner and auth realm without attempting credential brute forcing. |
| **HTTPS X.509 Certificate Subject** | **+15% - 20%** | Medium to Strong | TLS handshake retrieves CommonName, Organization, and Issuer strings without verifying CA trust. |
| **Proprietary Port Signatures** | **+15% - 20%** | Medium | Detection of vendor-specific discovery/SDK ports: `8000/tcp` (Hikvision SADP), `37777/tcp` (Dahua NetSDK), `34567/tcp` (Xiongmai XM). |

#### Total Confidence Score Calculation:
$$\text{Total Confidence} = \min\left(100, \sum \text{Evidence Weights}\right)$$

- **HIGH (80% - 100%)**: Multiple corroborating protocol proofs (e.g., ONVIF Scope + RTSP Banner, or MAC OUI + HTTP Banner + Proprietary Port). Confirmed model identification.
- **MEDIUM (50% - 79%)**: Definite CCTV unit with established manufacturer, but model or firmware unconfirmed.
- **LOW (<50%)**: Generic streaming device or MAC OUI alone. Marked as probable candidate for technician review.

---

### 3. Classification Taxonomy

The classifier maps device evidence into 9 discrete operational categories:

1. **`IP_CAMERA`**: Single-sensor network video transmitter. Default classification for units with RTSP streaming and optical imaging capabilities.
2. **`PTZ_CAMERA`**: Motorized Pan/Tilt/Zoom cameras. Detected via ONVIF `ptz` scopes, or model prefixes (`DS-2DE`, `DS-2DF`, `DH-SD`, `CP-VNP`).
3. **`DVR`**: Digital Video Recorders handling coaxial/analog inputs. Detected via models (`DS-71xx`, `DS-72xx`, `DH-DVR`, `CP-UVR`).
4. **`NVR`**: Network Video Recorders. Detected via ONVIF `network_video_storage` scopes or model prefixes (`DS-76xx`, `DS-77xx`, `DS-96xx`, `DH-NVR`, `CP-UNR`).
5. **`XVR`**: Universal/Pentabrid video recorders supporting IP, HDCVI, AHD, TVI, and CVBS signals.
6. **`HYBRID_RECORDER`**: Recorders combining native analog channels with IP camera decoding channels.
7. **`ENCODER`**: Multi-channel video servers converting analog CCTV composite video to IP streams (`DS-67xx`).
8. **`NETWORK_VIDEO_SERVER`**: Rackmount enterprise multi-channel video encoders.
9. **`UNKNOWN`**: Hardware detected with open ports but indeterminate CCTV streaming capabilities.

---

### 4. Inventory Registration & Credential Preservation Guarantee

A critical technician requirement is that running subsequent discovery scans must **never degrade or overwrite verified device configurations**:

```go
// PRESERVATION RULE: Never overwrite authenticated credentials or verified fields
if devToSave.Username == "" && existing.Username != "" {
    devToSave.Username = existing.Username
    devToSave.Password = existing.Password
}
if existing.SerialNumber != "" && devToSave.SerialNumber == "" {
    devToSave.SerialNumber = existing.SerialNumber
}
if existing.Firmware != "" && devToSave.Firmware == "" {
    devToSave.Firmware = existing.Firmware
}
```

This ensures that technician-enrolled credentials stored securely inside the AES-256-GCM vault are permanently preserved across subnet sweeps.
