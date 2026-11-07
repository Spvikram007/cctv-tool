# CCTV SENTINEL UNIVERSAL — STAGED CONFIGURATION & APPROVAL FRAMEWORK
## Pre-Flight Verification, Token Mechanics & Rollback Protocols

**Product:** CCTV SENTINEL UNIVERSAL  
**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  

---

## 1. The Principle of Staged Verification

Applying configurations directly to live security infrastructure without a preview phase introduces severe operational risks:
- Misconfigured subnet masks or gateways can instantly sever camera network access.
- Incorrect DNS addresses prevent NTP time servers and cloud monitoring from functioning.
- Unsupported video encoding profiles can cause recorder decoding pipelines to crash.

To protect critical facilities, CCTV Sentinel Universal mandates a **Two-Step Staged Execution Pipeline**:

$$\text{DISCOVER} \longrightarrow \text{AUDIT} \longrightarrow \mathbf{STAGE\ \&\ PREVIEW} \longrightarrow \mathbf{APPROVE} \longrightarrow \text{APPLY} \longrightarrow \text{READ-BACK} \longrightarrow \text{AUDIT}$$

---

## 2. Preview Token Mechanics (`bprv-...`)

When an operator requests a staged preview (`POST /api/v1/batch/preview`), the engine evaluates every target device without modifying device state:

```json
{
  "preview_token": "bprv-9a43043cc1b8ea6c49d4aee9cbdcd163",
  "action": "NETWORK_CONFIG",
  "total_targets": 8,
  "eligible_count": 8,
  "ineligible_count": 0,
  "reboot_required": false,
  "expires_at": "2026-11-07T16:15:00Z",
  "created_at": "2026-11-07T16:05:00Z",
  "previews": [ ... ]
}
```

### Key Token Properties:
1. **Entropy & Prefix**: Generated using 16 bytes of cryptographically secure random bytes from `crypto/rand`, hex-encoded with prefix `bprv-` (e.g. `bprv-9a43043cc1b8ea6c49d4aee9cbdcd163`).
2. **Time-To-Live (TTL)**: Exactly 10 minutes. If not approved within 10 minutes, the token expires automatically to prevent executing stale diffs against hardware whose state may have changed.
3. **Single-Use Enforcement**: Upon execution, the preview token is immediately consumed and deleted from the active registry. Re-submitting the same token yields `400 Bad Request: preview token expired or invalid`.
4. **Scope Lock**: The preview token is cryptographically bound to the exact target list and payload snapshot generated during preview. It cannot be used to execute a different action or target different IP addresses.

---

## 3. Pre-Flight Capability Matrix Evaluation

Before generating a preview, the engine cross-references the requested action against each device's authenticated capability profile:

| Action | Required Capability | Ineligibility Reason If Missing |
|---|---|---|
| `TIME_NTP_SYNC` | ONVIF DeviceManagement or Dahua/Hikvision System Service | `Device does not support remote NTP time configuration` |
| `IMAGE_CONFIG` | ONVIF Imaging Service or ISAPI Image Channels | `Imaging service unsupported by hardware revision` |
| `NETWORK_CONFIG` | Network Interface Configuration Permission | `Device network interface configuration is read-only or locked` |
| `REBOOT` | System Reboot Service | `Reboot RPC service not exposed by device` |

Devices lacking required capabilities are marked `UNSUPPORTED` in the preview table with a clear warning explaining which service is absent, allowing technicians to uncheck them before execution.

---

## 4. High-Impact Operation Safeguards: FLEET REBOOT

Fleet reboots temporarily drop live surveillance feeds and video recording (typically 60 to 180 seconds). To eliminate accidental dispatches:

1. **Mandatory Confirmation Flag**: The payload must include `operator_confirmed: true`.
2. **UI Confirmation Input**: The web console forces the technician to explicitly type the confirmation string:
   $$\mathbf{CONFIRM-REBOOT}$$
3. **Pre-Execution Warning**: The staged preview modal displays a high-visibility red banner:
   > **[!] HIGH-IMPACT FLEET OPERATION: SCHEDULED REBOOT**  
   > *Execution will restart 8 physical cameras and disconnect active live feeds.*
4. **Rejection on Missing Token**: Invoking `/api/v1/batch/execute` with action `REBOOT` without a valid `bprv-...` token or without confirmation yields HTTP 403 Forbidden.

---

## 5. Automated Rollback & Recovery Protocols

For configuration actions (`NETWORK_CONFIG`, `IMAGE_CONFIG`):
1. **State Preservation**: The engine caches the original configuration snapshot prior to applying any modification.
2. **Read-Back Verification**: Immediately after applying parameters, the worker reads back the live settings from the camera.
3. **Discrepancy Detection**: If read-back values deviate from the intended state, the device is flagged with `read_back_match: false`.
4. **Rollback Trigger**: Technicians can click "Rollback" on the device result card, restoring the cached pre-execution configuration within the same session.
