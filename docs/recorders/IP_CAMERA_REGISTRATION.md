# CCTV SENTINEL UNIVERSAL
## IP Camera Channel Registration & Stream Multiplexing

**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  
**Module:** `backend/pkg/recorder`  
**Classification:** Technical Operations Guide

---

## 1. Overview

IP camera channel registration is the process of binding a network surveillance stream to an NVR or hybrid XVR digital channel. Once registered, the recorder handles continuous stream ingestion, hardware decoding, video indexing, and storage allocation.

---

## 2. Supported Registration Protocols

| Registration Protocol | Typical Port | Vendor Compatibility | Capabilities |
| :--- | :---: | :--- | :--- |
| **ONVIF Profile S / T** | `80`, `8080`, `2020` | All ONVIF Conformant Cameras | H.264/H.265 video, audio, motion alarms, PTZ |
| **Dahua Private (DHIP)** | `37777` | Dahua, CP PLUS, IMOU | Fast handshake, AI metadata, smart search |
| **Hikvision ISAPI / Private**| `8000`, `80` | Hikvision, Hilook, Ezviz | AcuSense alerts, perimeter protection |
| **Direct RTSP Transport** | `554`, `8554` | Any RTSP server / NVR pass-through | Raw video and audio transport |

---

## 3. Security & Anti-SSRF Safeguards

When a technician instructs a recorder to register an IP camera, the recorder initiates outbound network requests to the target IP address. Without strict verification, this could be abused as a Server-Side Request Forgery (SSRF) vector against internal infrastructure.

CCTV Sentinel Universal enforces strict pre-registration validation via `security.ValidateSurveillanceTarget(targetIP, allowLoopback)`:
- Blocks AWS/GCP/Azure link-local cloud metadata endpoints (`169.254.169.254`).
- Blocks unauthorized non-routable addresses (`0.0.0.0/8`, `224.0.0.0/4`).
- Strictly rejects loopback addresses (`127.0.0.0/8`) unless operating in local test/simulator mode.
- Validates that the target IP belongs to a legitimate surveillance network segment.

---

## 4. Credential Protection Rule

> [!CAUTION]
> Registering an IP camera on an NVR/XVR channel binds existing credentials (`username` and `password`) to establish the authenticated video session.
> CCTV Sentinel Universal **NEVER** alters, overwrites, or factory-resets camera credentials during channel registration unless the technician explicitly executes a credential rotation in the Device Security Vault.
