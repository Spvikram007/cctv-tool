# Device Configuration Safety Engine: 10-Step Lifecycle

## 1. Safety Philosophy

Applying network or sensor changes to IP cameras and recorders in production environments poses severe operational risks:
- Subnet misconfiguration can isolate a camera permanently, requiring physical ladder access or factory reset.
- Silent camera firmware rejections can cause technician tools to falsely report success when hardware values never changed.
- Plaintext credential leakage in audit logs breaches corporate security policies.

To eliminate these risks, **CCTV Sentinel Universal** enforces a mandatory **10-Step Configuration Lifecycle**.

---

## 2. The 10-Step Lifecycle Architecture

```
[1. AUTHENTICATE]
       │  Verifies session & credentials using AES-256-GCM vault
       ▼
[2. READ EXISTING]
       │  Reads current live configuration from physical device
       ▼
[3. VALIDATE PROPOSED]
       │  Checks syntax, IPv4 sanity, netmask ranges, brightness/contrast [0-100]
       ▼
[4. GENERATE STRUCTURAL DIFF]
       │  Produces human-readable before-and-after comparison
       ▼
[5. ISSUE PREVIEW TOKEN]
       │  Creates ephemeral preview token (valid 10 minutes) & rollback key
       ▼
[6. REQUIRE EXPLICIT APPROVAL]
       │  Blocks apply execution unless authorized with valid preview token
       ▼
[7. APPLY CHANGES]
       │  Dispatches protocol-specific writes (ISAPI / CGI / ONVIF)
       ▼
[8. READ-BACK VERIFICATION]
       │  Immediately fetches live settings again from device hardware
       ▼
[9. DETECT MISMATCHES & AUDIT]
       │  Compares desired target with read-back state; records immutable audit entry
       ▼
[10. ROLLBACK CAPABILITY]
          Restores original snapshot upon technician command or verified failure
```

---

## 3. Strict Read-Back Verification

In Step 8, the engine immediately re-queries the device to ensure that the target setting was successfully written to NVRAM/flash:

### 3.1 Network Read-Back Check
```go
if !targetCfg.DHCP && verified.IPAddress != targetCfg.IPAddress {
    match = false
    mismatchReasons = append(mismatchReasons, fmt.Sprintf("IP mismatch: expected %s, got %s", targetCfg.IPAddress, verified.IPAddress))
}
if verified.DHCP != targetCfg.DHCP {
    match = false
    mismatchReasons = append(mismatchReasons, fmt.Sprintf("DHCP mode mismatch: expected %t, got %t", targetCfg.DHCP, verified.DHCP))
}
```

### 3.2 Result Handling
- **Match Confirmed**: Engine returns `read_back_match: true` and status `"SUCCESS"`.
- **Mismatch Detected**: Engine flags `read_back_match: false` with detailed `read_back_diff`, notifies the technician with a high-priority warning, and preserves the rollback snapshot for instant recovery.

---

## 4. Rollback Mechanism (Step 10)

Whenever a preview is generated, an atomic snapshot of the pre-change state is indexed under a cryptographic `rollback_key`:
```
Snapshot Index: rb-fd3d662bbea66a9b9b4bb56c90e90545 -> {"ip_address":"192.168.31.103","dhcp":false,...}
```

When a technician clicks **ROLLBACK** or invokes `POST /api/v1/devices/{id}/config/rollback`:
1. The engine retrieves the pre-change snapshot.
2. It re-applies the original configuration to the physical hardware.
3. It performs a read-back check to confirm restoration.
4. An immutable audit record is logged with zero secret exposure.
