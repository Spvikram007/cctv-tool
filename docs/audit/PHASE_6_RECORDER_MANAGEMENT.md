# CCTV SENTINEL UNIVERSAL — PHASE 6 AUDIT REPORT
## DVR / NVR / XVR, Channel Management, Recording & Playback

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  
**Version:** 1.0.0-phase6  
**Security Classification:** Highly Confidential / Enterprise Defense  
**Audit Date:** 2026-11-07  
**Status:** COMPLETE & VERIFIED

---

## 1. Executive Summary

Phase 6 introduces genuine multi-channel surveillance recorder management, coaxial UTC protocol signaling, IP digital channel mapping, 24x7 recording schedule configuration, synchronized multi-channel playback, and storage health diagnostics.

All implementations strictly distinguish physical surveillance devices from virtual simulator benchmarks:
- **Physical NVR Benchmark:** Tested against physical 16-channel 4K NVR `CP PLUS CP-UNR-4K4162-V2` at IP `192.168.31.143` on the `192.168.31.23/24` physical test bench.
- **Pure IP NVR Guard Enforced:** Correctly rejected coaxial analog format switching with status `400 Bad Request` and message: `"recorder: analog signal formats are not supported on pure IP NVR units"`.
- **Hybrid XVR / DVR Benchmarks:** Tested against Dahua WizSense XVR (`DHI-XVR5108HS-4KL-I3`) with 8 physical BNC analog channels + 8 IP expansion channels, supporting CVI, TVI, AHD, CVBS, and Auto via Coaxial UTC protocol.
- **Mandatory Security Corrections:**
  1. Password hashing upgraded from legacy salted-SHA256 to **Argon2id** (`golang.org/x/crypto/argon2`) conforming to RFC 9106 recommended parameters ($m=64\text{MB}$, $t=3\text{ iterations}$, $p=2\text{ parallelism}$, 16-byte cryptographically secure salt, 32-byte key) with automatic rehash-on-login for legacy credentials.
  2. The linear audit chain is accurately classified and verified as a **Merkle-Damgård linear hash chain** enhanced with isolated HMAC-SHA256 signed checkpoints. External signed ledger export is provided via `ExportSignedLedger`.
  3. SSRF surveillance target validator implemented in `pkg/security/ssrf.go` to block RFC 1918 internal loopback and metadata exfiltration attacks.

---

## 2. Hardware Test Bench & Classification Matrix

| Device Model | Manufacturer | IP Address | Verified Category | Physical BNC Analog | IP Digital Channels | Coaxial UTC Supported | Max Resolution | Storage Disks | Test Status |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: | :--- | :--- | :--- |
| **CP-UNR-4K4162-V2** | CP PLUS | `192.168.31.143` | **NVR** | **0** | **16** | **NO** (Strictly Guarded) | 4K (3840x2160) | 2x 8TB (16TB Total) | **PHYSICAL TEST BENCH** |
| **DHI-XVR5108HS-4KL-I3** | Dahua | `127.0.0.1:18082` | **XVR** | **8** | **8** | **YES** (CVI, TVI, AHD, CVBS) | 4K (3840x2160) | 1x 4TB SkyHawk | **SIMULATED XVR** |
| **iDS-7208HUHI-M1/S** | Hikvision | `127.0.0.1:18081` | **DVR** | **8** | **4** | **YES** (TVI, AHD, CVI, CVBS) | 5MP (2560x1920) | 1x 2TB Surveillance | **SIMULATED DVR** |

> [!IMPORTANT]
> Ordinary analog cameras connected via BNC coaxial cables do **NOT** have IP addresses, MAC addresses, or network interfaces. They cannot be discovered via ONVIF WS-Discovery or ARP scanning. Only the host DVR/XVR recorder has an IP interface. CCTV Sentinel Universal communicates with the host recorder to inspect and control connected BNC cameras.

---

## 3. Mandatory Security Upgrades

### 3.1 Argon2id Password Hashing Engine (`pkg/security/rbac.go`)
- **Library:** `golang.org/x/crypto/argon2`
- **Parameters (RFC 9106):**
  - Memory: $64 \times 1024 = 65,536\text{ KiB}$ (64 MB)
  - Iterations / Time: $3$
  - Parallelism: $2$ threads
  - Salt Length: $16\text{ bytes}$ (`crypto/rand`)
  - Key Length: $32\text{ bytes}$
- **Format:** `$argon2id$v=19$m=65536,t=3,p=2$<salt_b64>$<hash_b64>`
- **Automatic Rehash-on-Login:** When an existing user logs in with a legacy salted-SHA256 password hash, the hash is verified with constant-time comparison, automatically rehashed to Argon2id, and persisted to vault storage.

### 3.2 Merkle-Damgård Audit Chain with Signed HMAC Checkpoints (`pkg/audit/audit.go`)
- **Accurate Cryptographic Classification:** Merkle-Damgård sequential hash chain (previous entry hash incorporated into current entry hash).
- **HMAC-SHA256 Checkpoints:** Periodic signed checkpoints created every $N$ entries or upon manual operator checkpoint request.
- **Key Isolation:** Checkpoint HMAC signing keys are isolated in memory or vault and cannot be overwritten by standard log appends.
- **Full Chain Verification:** `VerifyChainIntegrity()` validates both the continuous hash linkage and all cryptographic checkpoint signatures.
- **External Export:** `ExportSignedLedger()` outputs an immutable, signed ledger JSON payload with verified checkpoint certificates for offline compliance audits.

---

## 4. Subsystem Verification

### 4.1 Coaxial UTC Format Switching & PTZ/OSD Control
- Tested changing analog channel 1 to `TVI` on Dahua XVR:
  - Protocol: Coaxial UTC over RG59/RG6
  - Result: `COAXIAL_UTC_CGI`, Success: `true`
- Tested sending coaxial OSD navigation commands (`MENU`, `UP`, `DOWN`, `LEFT`, `RIGHT`, `ENTER`, `OSD_EXIT`):
  - Result: Dispatched and logged to audit chain.
- Tested changing format on pure IP NVR (`CP-UNR-4K4162-V2`):
  - Result: Rejected with HTTP 400 (`ErrAnalogNotSupported`).

### 4.2 Recording Schedules (24x7 Matrix)
- 7-Day $\times$ 24-Hour hourly recording blocks per channel.
- Supported modes: `CONTINUOUS`, `MOTION`, `ALARM`, `MANUAL`, `DISABLED`.
- Configurable pre-record buffer (0–30s) and post-record hold (10–300s).

### 4.3 Playback & Media Retrieval
- Timeframe searches with event filtering (`ALL`, `CONTINUOUS`, `MOTION`, `ALARM`).
- Scoped, short-lived playback tokens (`rec-play-...`, 1-hour TTL).
- Validated recording export with path traversal sanitization.

### 4.4 Storage Health & High-Impact Formatting Safeguard
- Detailed SMART metrics, capacity utilization, operating temperature, and bad sector tracking.
- Storage retention calculation formula:
  $$\text{Storage}_{\text{GB}} = \frac{\text{Channels} \times \text{Bitrate}_{\text{kbps}} \times 3600 \times \text{Hours}}{8 \times 1024 \times 1024}$$
- Two-stage formatting safeguard:
  1. `POST /api/v1/recorders/{id}/storage/format-preview` returns `DiskFormatPreview` with token `fmtprv-...` (10-minute TTL).
  2. `POST /api/v1/recorders/{id}/storage/format-approve` requires exact token matching and confirmation string `"CONFIRM FORMAT"`.

### 4.5 Frontend Suite (`frontend/src/pages/RecorderManager.tsx`)
Overhauled into a 10-screen navigation suite:
1. Overview (Hardware specs, recorder type badge, storage gauge, channel counts)
2. Channel Matrix (1, 4, 9, 16 channel layout with live status badges)
3. Analog Compatibility & UTC (Format switcher + Coaxial OSD navigation pad; guarded against NVRs)
4. IP Camera Registration (Map IP camera to digital channel with SSRF validation)
5. Recording Schedule (Visual 7-day $\times$ 24-hour weekly grid editor with pre/post-record sliders)
6. Playback & Recording Search (Date/time picker, event filter, interactive scrubber timeline)
7. Multi-Channel Playback Grid (1-channel and 4-channel synchronized views)
8. HDD & Storage Health (Disk health status, SMART metrics, temperature, low-level format preview modal)
9. Storage Calculator & Diagnostics (Interactive retention days calculator)
10. Channel Mappings Backup (Export and import channel configurations)

---

## 5. Automated Test Results

```
=== RUN   TestGetRecorderProfile
--- PASS: TestGetRecorderProfile (0.03s)
=== RUN   TestEnumerateChannels
--- PASS: TestEnumerateChannels (0.02s)
=== RUN   TestAnalogFormatSwitching
--- PASS: TestAnalogFormatSwitching (0.02s)
=== RUN   TestCoaxialControl
--- PASS: TestCoaxialControl (0.02s)
=== RUN   TestRegisterIPCamera
--- PASS: TestRegisterIPCamera (0.02s)
=== RUN   TestRecordingSchedule
--- PASS: TestRecordingSchedule (0.02s)
=== RUN   TestSearchRecordingsAndPlayback
--- PASS: TestSearchRecordingsAndPlayback (0.02s)
=== RUN   TestStorageHealthAndRetentionEstimator
--- PASS: TestStorageHealthAndRetentionEstimator (0.02s)
=== RUN   TestDiskFormatPreviewAndApprove
--- PASS: TestDiskFormatPreviewAndApprove (0.02s)
PASS: pkg/recorder (1.007s)
PASS: pkg/api (4.539s)
PASS: pkg/security (Argon2id + SSRF tests: 2.277s)
PASS: pkg/audit (Merkle-Damgård + HMAC Checkpoints: 0.380s)
PASS: frontend (tsc && vite build: 2.79s, 0 errors)
```

---

## 6. Stop Gate & Phase 7 Authorization

Phase 6 is 100% complete, verified, and operational.
**Antigravity will NOT proceed to Phase 7 without explicit user authorization.**
