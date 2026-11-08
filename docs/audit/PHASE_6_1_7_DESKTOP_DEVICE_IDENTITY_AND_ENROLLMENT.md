# CCTV SENTINEL UNIVERSAL — PHASE 6.1.7 AUDIT REPORT
## Desktop Device Identity, Persistent Enrollment & Production Security Corrections

**Execution Date:** 2026-11-08  
**Branch:** `security/phase-6-1-7-1-stabilization`  
**Platform Owner:** Tech Defence Solutions  
**Status:** COMPLETED & VERIFIED — FULL REGRESSION PASS  

---

## 1. Executive Summary & Defect Remediation

In Phase 6.1.7, six critical architectural and security improvements were implemented to eliminate shared HMAC secrets, enforce asymmetric device identities, provide persistent registries across server restarts, and harden production cookies:

| Area | Prior Architecture / Limitation | Phase 6.1.7 Remediation | Verification Status |
|------|--------------------------------|-------------------------|---------------------|
| **Asymmetric Workstation Identity** | Server generated shared 32-byte HMAC secret and transmitted it back to the client. | Replaced with **Ed25519** asymmetric public-key cryptography. Native workstation generates keypair locally; private key NEVER leaves workstation; only public key is submitted. Workstation signs challenge nonce; backend verifies via `ed25519.Verify()`. | **VERIFIED** (Tests pass) |
| **Two-Phase Enrollment** | Workstations were immediately active upon registration. | Workstations enter `PENDING` approval status upon enrollment (`24-hour TTL`). Authenticated administrator must inspect cryptographic fingerprint (`SHA256:<hex>`) and approve (`POST .../approve`). Unapproved or expired devices cannot obtain challenges. | **VERIFIED** |
| **Persistent Device & User Registry** | Devices and users resided only in memory; backend restart purged all registrations and accounts. | Upgraded `Store` to **Schema Version 3**. Migrated database with auto-migration and rollback. Device records and user accounts are persisted to `sentinel_store.json`. Reconstructed managers reload all registrations seamlessly. | **VERIFIED** |
| **Fail-Closed Production Cookies** | Accidental or malicious environment flags could weaken cookie security in production. | In `SENTINEL_ENV=production`, `NewServer()` validates that `SENTINEL_SECURE_COOKIES` and `SENTINEL_COOKIE_HOST_PREFIX` are not disabled. Any attempt to disable them causes immediate fatal panic (fail closed). | **VERIFIED** |
| **NTFS ACL Hardening on Setup Token** | Token files relied solely on POSIX file modes which Windows NTFS ignores. | Implemented `RestrictFilePermissions()` in `file_perms.go`. On Windows, executes `icacls` to strip inheritance (`/inheritance:r`) and grant Full Control only to SYSTEM, Builtin Administrators, and `%USERNAME%`. | **VERIFIED** |
| **Session Invalidation on Revocation** | Revoking a workstation did not invalidate existing active bearer sessions issued to that device. | Added `DeviceID` binding to `Session` struct. Revoking a workstation (`POST .../revoke`) calls `rbacMgr.RevokeDeviceSessions(id)`, immediately invalidating all active sessions issued to that device. | **VERIFIED** |

---

## 2. Test Execution & Coverage Summary

### 2.1 Backend Unit & Integration Tests (100% Passing)
```
ok  	cctv-sentinel/pkg/adapters	0.736s
ok  	cctv-sentinel/pkg/adapters/cpplus	0.674s
ok  	cctv-sentinel/pkg/adapters/dahua	0.918s
ok  	cctv-sentinel/pkg/adapters/hikvision	0.814s
ok  	cctv-sentinel/pkg/adapters/httpauth	0.881s
ok  	cctv-sentinel/pkg/adapters/onvif	0.813s
ok  	cctv-sentinel/pkg/alarms	0.504s
ok  	cctv-sentinel/pkg/api	5.127s
ok  	cctv-sentinel/pkg/audit	0.755s
ok  	cctv-sentinel/pkg/batch	1.097s
ok  	cctv-sentinel/pkg/configengine	0.807s
ok  	cctv-sentinel/pkg/database	28.157s  [Schema v3 migration & rollback verified]
ok  	cctv-sentinel/pkg/diagnostics	0.729s
ok  	cctv-sentinel/pkg/discovery	0.714s
ok  	cctv-sentinel/pkg/events	0.391s
ok  	cctv-sentinel/pkg/firmware	0.852s
ok  	cctv-sentinel/pkg/licensing	0.621s
ok  	cctv-sentinel/pkg/media/rtsp	0.626s
ok  	cctv-sentinel/pkg/media/snapshot	0.634s
ok  	cctv-sentinel/pkg/media/streamhub	0.941s
ok  	cctv-sentinel/pkg/ptz	0.430s
ok  	cctv-sentinel/pkg/recorder	0.535s
ok  	cctv-sentinel/pkg/security	2.306s  [Ed25519 lifecycle & ACLs verified]
```
`go vet ./...`: 0 warnings, 0 errors.

### 2.2 Frontend Playwright E2E Suite (14/14 Passing)
```
Running 14 tests using 1 worker
  ok  1 [chromium] › tests\cookie-security.spec.ts:4:3 › session lifecycle: no bearer token in localStorage and CSRF cookie is present (372ms)
  ok  2 [chromium] › tests\cookie-security.spec.ts:34:3 › state-changing write requests include Origin and CSRF token (590ms)
  ok  3 [chromium] › tests\platform-modules.spec.ts:4:3 › loads dashboard and displays system health (337ms)
  ok  4 [chromium] › tests\platform-modules.spec.ts:10:3 › navigates through all core platform modules (1.2s)
  ok  5 [chromium] › tests\ptz-alarms.spec.ts:11:3 › Screen 1: PTZ Live Control — Capability Detection on Vatilon H43 vs Fixed Camera (540ms)
  ok  6 [chromium] › tests\ptz-alarms.spec.ts:32:3 › Screen 2: PTZ Presets — View coordinate memory (588ms)
  ok  7 [chromium] › tests\ptz-alarms.spec.ts:37:3 › Screen 3: Patrol Tours — View waypoint cycles and cruise engine (539ms)
  ok  8 [chromium] › tests\ptz-alarms.spec.ts:42:3 › Screen 4: Event Subscriptions — View active listener threads (469ms)
  ok  9 [chromium] › tests\ptz-alarms.spec.ts:49:3 › Screen 5: Live Events — View telemetry and category filter (457ms)
  ok 10 [chromium] › tests\ptz-alarms.spec.ts:56:3 › Screen 6: Alarm Dashboard — View KPI metrics and incident queue (473ms)
  ok 11 [chromium] › tests\ptz-alarms.spec.ts:65:3 › Screen 7: Alarm Logs — Searchable table and export links (453ms)
  ok 12 [chromium] › tests\ptz-alarms.spec.ts:72:3 › Screen 8: Alarm Rules — Builder form and active automation rules (484ms)
  ok 13 [chromium] › tests\ptz-alarms.spec.ts:78:3 › Screen 9: Evidence Viewer — Truth Rule compliance audit (506ms)
  ok 14 [chromium] › tests\ptz-alarms.spec.ts:84:3 › Screen 10: Event Matrix — Fleet-wide hardware certification table (468ms)

14 passed (8.4s)
```

---

## 3. Scope Boundaries & Stop Gate Compliance
- **Working Branch:** `security/phase-6-1-7-1-stabilization` exclusively.
- **Master Branch:** Not modified. Not merged.
- **Phase 8:** Not started.
- **Physical PTZ Motor Actuation (Vatilon H43):** Remained in **PENDING OPERATOR APPROVAL** state during automated testing.
- **Secret Redaction:** Setup tokens and private keys are never committed or logged in plaintext.
