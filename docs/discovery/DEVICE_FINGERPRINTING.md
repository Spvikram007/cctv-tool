# CCTV SENTINEL UNIVERSAL
## Confidence-Weighted Device Fingerprinting & Classification Engine

```
Product:   CCTV SENTINEL UNIVERSAL
Owner:     Tech Defence Solutions
Module:    Universal Discovery Engine — Modular Fingerprinter & Classifier
Status:    Phase 2.1 Hardened & Physical Bench Certified
Date:      November 7, 2026
```

---

### 1. Architectural Philosophy & Hardening Invariants

In surveillance networking, **no single heuristic is 100% reliable**:
- OEM rebranding means a camera with a generic MAC may run Dahua firmware or customized OEM software.
- Default HTTP banners may report generic web server strings (`webserver`, `App-webs/1.0`, `lighttpd`).
- Multicast WS-Discovery may be filtered by managed PoE switch VLAN boundaries or IGMP snooping.

To deliver carrier-grade reliability without brute-forcing passwords or violating network boundaries, CCTV Sentinel Universal implements a **Confidence-Weighted Multi-Factor Evidence Matrix** governed by 6 strict engineering invariants:

> [!IMPORTANT]
> **Hardening Rule 1: Never Infer a Specific Model from an OUI Alone**  
> An IEEE MAC prefix only identifies the manufacturer who purchased the OUI block. It provides zero information regarding whether the device is an entry-level bullet camera, a 4K PTZ camera, or a 32-channel NVR. Model is marked as `Unknown` if only OUI is present.

> [!IMPORTANT]
> **Hardening Rule 2: IEEE 802 Locally Administered MAC Addresses (U/L Bit)**  
> If bit 1 of the first byte of the MAC address is set (`val & 0x02 != 0`), the address is locally administered (virtual interface, Docker bridge, or randomized MAC). OUI lookup is invalidated with 0 weight and an explicit explanation logged.

> [!IMPORTANT]
> **Hardening Rule 3: Anti-Double-Counting for Correlated Evidence**  
> Evidence items originating from the same protocol handshake (e.g. `Server:` header and `WWW-Authenticate:` header in the same HTTP response) are consolidated into a single evidence contribution to prevent artificial confidence inflation.

> [!IMPORTANT]
> **Hardening Rule 4: OEM & Rebranded Hardware Recognition**  
> Known OEM footprints (e.g. Xiongmai XM port `34567/tcp`, Dahua NetSDK port `37777/tcp`, Hikvision SADP port `8000/tcp`) are tagged with the underlying chipset/firmware architecture family.

> [!IMPORTANT]
> **Hardening Rule 5: Credential & Configuration Preservation Guarantee**  
> Subsequent unauthenticated discovery scans cannot overwrite or blank out previously authenticated vault credentials, verified serial numbers, or verified firmware versions in the inventory database.

> [!IMPORTANT]
> **Hardening Rule 6: No Identity Downgrades from Weaker Evidence**  
> If an enrolled device was verified as `Hikvision DS-2CD1023G0-IU`, a subsequent raw port sweep that detects only port 80 cannot downgrade its manufacturer to `Generic ONVIF` or its model to `Unknown`.

---

### 2. Evidence Sources & Weight Allocation

| Evidence Source | Weight | Significance | Engineering Rationale |
| :--- | :---: | :--- | :--- |
| **ONVIF WS-Discovery Scopes** | **+55%** | Definitive Profile | Hardware model, vendor name, and capabilities embedded in standard URI scopes (`/hardware/`, `/name/`, `/profile/`). |
| **MAC OUI Prefix Match** | **+15% - 25%** | Probable Vendor | Registered IEEE OUI block. Contributes +25% alone, or +15% corroboration if ONVIF is already present. **Never 100% on its own**. |
| **RTSP OPTIONS Server Banner** | **+20% - 25%** | Strong Protocol | Direct protocol response from RTSP stack (e.g. `Server: Hikvision RTSP Server`, `Server: Dahua RTSP Server`). |
| **HTTP Server & WWW-Authenticate** | **+15% - 20%** | Consolidated HTTP | Single consolidated proof inspecting server banner and auth realm without attempting password brute forcing. |
| **HTTPS X.509 Certificate Subject** | **+15%** | Medium to Strong | TLS handshake retrieves CommonName, Organization, and Issuer strings from self-signed certificate. |
| **Proprietary Port Signatures** | **+10% - 15%** | Corroborating Signature | `8000/tcp` (Hikvision SADP), `37777/tcp` (Dahua NetSDK), `34567/tcp` (Xiongmai XM). Weighted as +10% if vendor is already corroborated. |

#### Confidence Level Stratification:
- **`HIGH` ($\ge 80\%$)**: Multiple corroborating protocol proofs (e.g., ONVIF Scope + RTSP Banner + Port Signature, or Authenticated Adapter Handshake). Confirmed model identification.
- **`MEDIUM` ($50\% - 79\%$)**: Confirmed CCTV hardware with established manufacturer and model via ONVIF or OUI + Protocol Banner, awaiting authenticated credential login.
- **`LOW` ($< 50\%$)**: Uncorroborated single-signal candidate (e.g. MAC OUI alone, or open port 80 alone). Marked as candidate for technician review.

---

### 3. Classification Taxonomy

The classifier categorizes hardware into 9 discrete operational surveillance categories:

1. **`IP_CAMERA`**: Single-sensor network video transmitter. Default classification for units with RTSP streaming and optical imaging capabilities.
2. **`PTZ_CAMERA`**: Motorized Pan/Tilt/Zoom cameras. Detected via ONVIF `ptz` scopes, or model prefixes (`DS-2DE`, `DS-2DF`, `DH-SD`, `CP-VNP`, `H43`).
3. **`DVR`**: Digital Video Recorders handling coaxial/analog inputs (`DS-71xx`, `DS-72xx`, `DH-DVR`, `CP-UVR`).
4. **`NVR`**: Network Video Recorders. Detected via ONVIF `network_video_storage` scopes or model prefixes (`DS-76xx`, `DS-77xx`, `DH-NVR`, `CP-UNR`).
5. **`XVR`**: Universal/Pentabrid video recorders supporting IP, HDCVI, AHD, TVI, and CVBS signals.
6. **`HYBRID_RECORDER`**: Recorders combining native analog channels with IP camera decoding channels.
7. **`ENCODER`**: Multi-channel video servers converting analog CCTV composite video to IP streams (`DS-67xx`).
8. **`NETWORK_VIDEO_SERVER`**: Enterprise multi-channel video servers.
9. **`UNKNOWN`**: Hardware detected with open ports but indeterminate CCTV streaming capabilities.

---

### 4. Empirical Bench Verification Examples

#### Example 1: Physical Hikvision Camera (`192.168.31.103`)
```json
{
  "identified_manufacturer": "Hikvision",
  "model": "DS-2CD1023G0-IU",
  "category": "IP_CAMERA",
  "confidence": 70,
  "confidence_level": "MEDIUM",
  "confidence_explanation": "Confidence: 70% (MEDIUM) [Corroborated by: ONVIF WS-Discovery (+55%), MAC OUI Hikvision (+15%)]",
  "evidence_sources": [
    {
      "category": "ONVIF_WS_DISCOVERY",
      "description": "Active ONVIF WS-Discovery SOAP ProbeMatch response received",
      "weight": 55,
      "value": "Scopes: onvif://www.onvif.org/hardware/DS-2CD1023G0-IU onvif://www.onvif.org/name/HIKVISION%20DS-2CD1023G0-IU onvif://www.onvif.org/location/city/hangzhou"
    },
    {
      "category": "MAC_OUI",
      "description": "IEEE OUI prefix matches registered vendor block for Hikvision",
      "weight": 15,
      "value": "c0:51:7e:be:be:a6"
    }
  ]
}
```

#### Example 2: Physical Third-Party PTZ Camera with Private MAC (`192.168.31.104`)
```json
{
  "identified_manufacturer": "Vatilon",
  "model": "H43",
  "category": "PTZ_CAMERA",
  "confidence": 55,
  "confidence_level": "MEDIUM",
  "confidence_explanation": "Confidence: 55% (MEDIUM) [Corroborated by: ONVIF WS-Discovery (+55%)]",
  "evidence_sources": [
    {
      "category": "ONVIF_WS_DISCOVERY",
      "description": "Active ONVIF WS-Discovery SOAP ProbeMatch response received",
      "weight": 55,
      "value": "Scopes: onvif://www.onvif.org/type/ptz onvif://www.onvif.org/hardware/H43 onvif://www.onvif.org/manufacturer/Vatilon"
    },
    {
      "category": "MAC_OUI",
      "description": "Locally Administered / Private MAC address (IEEE U/L bit set) - vendor OUI not globally assigned",
      "weight": 0,
      "value": "f6:3a:80:12:e0:e8",
      "explanation": "Bit 1 of first octet is set. Hardware uses randomized or virtual switch MAC; OUI vendor inference is invalid."
    }
  ]
}
```
