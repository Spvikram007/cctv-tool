# CCTV SENTINEL UNIVERSAL
## Physical Hardware Test Procedure & Certification Checklist

```
Product:   CCTV SENTINEL UNIVERSAL
Owner:     Tech Defence Solutions
Module:    Universal Discovery Engine — Hardware Validation Runbook
Status:    Phase 2 Protocol Certified; Physical Bench Certification Ready
```

---

### 1. Scope & Objective

This document outlines the mandatory physical verification procedure for CCTV field technicians and QA engineers. 

> [!IMPORTANT]
> In accordance with project governance rules, **development simulator tests and unit tests do not constitute physical hardware certification**. 
> Physical certification requires executing this runbook against physical cameras and recorders connected via physical Ethernet switches.

---

### 2. Required Test Bench Equipment

| Item | Specification | Description |
| :--- | :--- | :--- |
| **PoE Switch** | 8-Port Gigabit PoE+ (802.3at) | Unmanaged or managed switch (e.g. TP-Link TL-SG108PE) with IGMP snooping disabled or configured. |
| **Physical Target 1** | Hikvision IP Camera | E.g., DS-2CD2042WD-I, DS-2CD2143G0-I, or DS-2DE4225IW-DE PTZ. |
| **Physical Target 2** | Dahua IP Camera / NVR | E.g., DH-IPC-HFW2431S-S-S2 or DH-NVR4108HS-4KS2. |
| **Physical Target 3** | CP PLUS Camera / XVR | E.g., CP-UNC-TA41L3 or CP-UVR-0801E1-V4 Pentabrid XVR. |
| **Technician Laptop** | Windows 10/11 or Linux | Dual-homed laptop running CCTV Sentinel Universal build. |

---

### 3. Pre-Flight Configuration

#### 3.1 Network Topology Setup
1. Connect the technician laptop via physical Ethernet cable to Port 1 of the PoE switch.
2. Assign static IP address to technician Ethernet NIC:
   - IP: `192.168.1.200`
   - Subnet Mask: `255.255.255.0` (`/24`)
   - Gateway: `192.168.1.1` (or leave blank for isolated bench)
3. Ensure physical targets are powered via PoE and link LEDs are solid green.

#### 3.2 Windows Firewall Rules
Ensure inbound UDP port `3702` and ephemeral UDP ports are permitted:
```powershell
New-NetFirewallRule -DisplayName "CCTV Sentinel ONVIF WS-Discovery" -Direction Inbound -Protocol UDP -LocalPort 3702 -Action Allow
New-NetFirewallRule -DisplayName "CCTV Sentinel REST API" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```

---

### 4. Step-by-Step Test Procedure

#### Step 1: Interface Enumeration Verification
1. Launch Sentinel Universal.
2. Navigate to **Universal LAN & ONVIF Discovery Engine** in the dashboard.
3. Confirm the **Network Interface** dropdown lists your physical Ethernet interface (`Ethernet` or `eth0`) with IP `192.168.1.200` and `Multicast: true`.
4. **Pass Criteria**: Physical interface appears correctly; loopback is not selected as default.

#### Step 2: Genuine Multicast WS-Discovery Execution
1. Select discovery method: **Multicast ONVIF (WS-Discovery)**.
2. Click **START DISCOVERY**.
3. Observe live progress indicator (sends UDP probe to `239.255.255.250:3702`).
4. **Pass Criteria**:
   - All physical ONVIF cameras appear in the results list within 3.5 seconds.
   - Identified Manufacturer, Model, and ONVIF Service URL (`/onvif/device_service`) match physical camera labels.
   - Confidence score indicates $\ge 85\%$ (HIGH).

#### Step 3: Subnet Multi-Port TCP Sweep Verification
1. Select discovery method: **Subnet TCP Multi-Port Sweep**.
2. Enter CIDR: `192.168.1.0/24`.
3. Ports: `80, 554, 8000, 37777, 8080, 8001, 8899, 34567`.
4. Click **START DISCOVERY**.
5. Observe progress bar iterating through 254 IP addresses with concurrent worker semaphore.
6. Click **CANCEL** mid-scan on an alternate run to verify immediate cancellation without hung sockets.
7. **Pass Criteria**:
   - Responding devices detect open ports accurately (`8000` on Hikvision, `37777` on Dahua).
   - Cancellation takes effect in under 200ms.

#### Step 4: Universal Hybrid Discovery Execution
1. Select discovery method: **Universal Hybrid (ONVIF + TCP Sweep)**.
2. Click **START DISCOVERY**.
3. Verify that devices discovered via ONVIF multicast are seamlessly enriched with open port signatures and RTSP banners without duplication.
4. **Pass Criteria**: Single deduplicated card per physical device with combined highest confidence score.

#### Step 5: Evidence & Classification Inspection
1. Click **EVIDENCE** on each discovered card.
2. Inspect the **Confidence Weighted Evidence Matrix**:
   - Verify MAC OUI prefix is credited (+30%).
   - Verify RTSP Server banner is credited (+20-30%).
   - Verify ONVIF Scopes are credited (+55%).
   - Verify PTZ cameras are classified as `PTZ_CAMERA` and XVR/NVR recorders as `XVR` / `NVR`.
3. **Pass Criteria**: Exact evidence sources match physical hardware without fabricated values.

#### Step 6: Fleet Enrollment & Credential Preservation
1. Click **ADD TO FLEET** for each unit.
2. Navigate to **Devices Inventory** and configure verified credentials.
3. Return to **Discovery** and run a fresh scan on the same subnet.
4. **Pass Criteria**: Existing enrolled devices retain authenticated credentials; no data loss or overwrite occurs.

---

### 5. Physical Hardware Certification Sign-Off Matrix

| Hardware Model | Firmware Version | MAC Address | Discovery Method | Conf. Score | Category | Pass / Fail | Technician Sign-Off |
| :--- | :--- | :--- | :--- | :--- | :--- | :---: | :--- |
| Hikvision DS-2CD2042WD-I | V5.5.82 | `bc:54:51:...` | HYBRID | 95% | `IP_CAMERA` | [PENDING BENCH] | Technician |
| Dahua DH-IPC-HFW2431S | V2.820 | `38:af:29:...` | HYBRID | 95% | `IP_CAMERA` | [PENDING BENCH] | Technician |
| CP PLUS CP-UVR-0801E1 | V4.000 | `f0:c8:50:...` | HYBRID | 90% | `XVR` | [PENDING BENCH] | Technician |
| Hikvision DS-2DE4225IW-DE | V5.6.14 | `a4:14:37:...` | ONVIF | 100% | `PTZ_CAMERA` | [PENDING BENCH] | Technician |
