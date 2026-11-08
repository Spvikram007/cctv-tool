# CP PLUS EVENT INTEGRATION & MODEL-AWARE ADAPTER
## UNR, UNC, Indigo, Cosmic & Orange Series Event Normalization

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** `backend/pkg/events`  
**Phase:** 7  

---

## 1. CP PLUS Architecture & Protocol Diversity

CP PLUS hardware deployed across enterprise and SMB sites in India and Southeast Asia originates from multiple hardware architectures:
1. **Indigo / Cosmic IP Series (Dahua-OEM & Proprietary):** IP cameras such as `CP-UNC-TA21PL3-V3` utilize Dahua-derivative CGI or ONVIF Profile S/T event channels.
2. **CP PLUS UNR Network Video Recorders:** Recorders like `CP-UNR-4K4162-V2` (16-channel 4K NVR) aggregate events across all attached IP channels and broadcast multi-channel alert bundles.
3. **Orange Series & HDCVI / HDx Analog XVRs:** Coaxial sensor alarms (PIR cameras, Coaxial alarm relays) multiplexed over coaxial cables.

---

## 2. Model-Aware Normalization Logic

The CP PLUS adapter inspects model signatures to select the optimal ingestion parser:

```go
func ParseCPPlusEvent(model string, payload []byte) (*SurveillanceEvent, error) {
    if strings.Contains(model, "UNR") {
        // NVR Multi-channel aggregation format
        return parseCPPlusNVREvent(payload)
    }
    if strings.Contains(model, "UNC") || strings.Contains(model, "Indigo") {
        // IP Camera CGI / ONVIF parser
        return parseCPPlusIPCameraEvent(payload)
    }
    // Generic fallback with schema validation
    return parseCPPlusGenericEvent(payload)
}
```

---

## 3. Physical Device Event Audit: CP PLUS Fleet

### 3.1 CP PLUS `CP-UNC-TA21PL3-V3` (`192.168.31.106`)
- **Category:** Fixed IP Bullet Camera
- **Resolution:** 2MP (1080p)
- **Supported Event Types:** Video Motion Detection, Video Tampering (Lens occlusion), Network Disconnect, IP Conflict.
- **PTZ Motors:** None (`ptz_supported: false`). Rejects pan/tilt commands.

### 3.2 CP PLUS `CP-UNR-4K4162-V2` (`192.168.31.143`)
- **Category:** 16-Channel 4K NVR
- **Event Aggregation:** Multi-channel alert dispatch. If channel 3 triggers motion, the event specifies `channel_id: 3` and reports the originating camera's hardware identity.
- **Storage Diagnostics:** Alerts on HDD missing, SMART degradation, and abnormal disk temperature.

---

## 4. Unknown Alert Code Fallback

When a newly introduced CP PLUS firmware version issues an undocumented alert code:
1. The adapter creates a normalized event with `category: "SYSTEM"` and `topic: "CPPLUS/Unknown/<Code>"`.
2. The raw payload is preserved in `raw_payload`.
3. An audit log warning is written with the hex/text dump so firmware engineers can map the new code without application crashing or event loss.
