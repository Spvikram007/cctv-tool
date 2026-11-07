# CCTV SENTINEL UNIVERSAL — PHASE 0 REPOSITORY AUDIT & BASELINE
**Author:** Principal Software Architect & QA Automation Lead  
**Organization:** Tech Defence Solutions  
**Date:** November 7, 2026  
**Repository Path:** `c:\xampp\htdocs\cctv tool`  
**Git Baseline Commit:** `e460493`  

---

## 1. Executive Summary
Phase 0 establishes the factual baseline of the CCTV SENTINEL UNIVERSAL codebase prior to initiating subsequent engineering phases. 

The existing repository consists of:
1. **Core Go Backend** (`backend/`): A modular server implementing universal device abstraction, capability-based protocol adapters, an embedded transactional JSON/SQLite store, an audit logger, a licensing engine, and a multi-device simulator.
2. **React + TypeScript Frontend** (`frontend/`): A single-page application built on Vite, Tailwind CSS, and Lucide icons, providing 13 technician modules.
3. **Hardware Simulation Harness** (`backend/pkg/simulator`): Emulation services for Hikvision ISAPI (port 18081), Dahua CGI (port 18082), Generic ONVIF (port 18083), and Mock RTSP (port 18554).
4. **Master Specifications**: Technical Architecture ([`a`](../../a)), Development Prompt ([`1`](../../1)), and Deep Protocol Research ([`deep-research-report.md`](../../deep-research-report.md)).

> [!IMPORTANT]
> **Hardware Status Classification**: Current functionality is strictly **Development Build & Verified Simulator Verified**. Zero physical camera certifications exist. Simulated test results are rigorously separated from physical device results.

---

## 2. Existing Feature Inventory

| Module / Component | Implementation Path | Status | Verification Mechanism |
| :--- | :--- | :--- | :--- |
| **System Health & Licensing** | `backend/pkg/licensing`, `backend/pkg/api` | Implemented | Live HTTP probe (`/api/v1/system/status`), Unit tested |
| **Device Inventory** | `backend/pkg/database` | Implemented | Thread-safe in-memory + JSON store with 1000-op stress test |
| **Discovery Engine** | `backend/pkg/discovery` | Partial | Active TCP CIDR port scanning; **UDP ONVIF WS-Discovery not yet implemented** |
| **Universal Configuration Engine** | `backend/pkg/configengine` | Implemented | 12-step validation, preview, confirmation, read-back & rollback key |
| **Hikvision ISAPI Adapter** | `backend/pkg/adapters/hikvision` | Implemented | Tested against simulated ISAPI endpoint (`DS-2CD2043G2-I`) |
| **Dahua CGI Adapter** | `backend/pkg/adapters/dahua` | Implemented | Tested against simulated CGI endpoint (`DHI-XVR5108HS-4KL-I3`) |
| **ONVIF Profile S/T/G Adapter** | `backend/pkg/adapters/onvif` | Implemented | Tested with WS-Security PasswordDigest against simulated SOAP endpoint |
| **CP PLUS Universal Adapter** | `backend/pkg/adapters/cpplus` | Implemented | Wraps Dahua CGI stack; fallback SVG snapshot |
| **Diagnostics & Calculators** | `backend/pkg/diagnostics` | Implemented | TCP ping, RTSP probe, Subnet/CIDR calc, HDD calc, Bandwidth calc |
| **Audit Logger** | `backend/pkg/audit` | Implemented | In-memory + persistent event logging |
| **Batch Configuration** | `backend/pkg/database` | Implemented | Job tracking & fleet execution state |
| **Live Video & Snapshot** | `frontend/src/pages/LiveView.tsx` | Simulated | Mock RTSP responder + SVG placeholder snapshot generation |
| **GST Invoice Engine** | `backend/pkg/licensing` | Implemented | CGST/SGST/IGST computation with verification |

---

## 3. API Implementation Matrix
All 29 frontend methods in `frontend/src/services/api.ts` were audited against routes in `backend/pkg/api/api.go`.

| Client Method (`api.ts`) | Target HTTP Route | Backend Handler (`api.go`) | Implementation Status |
| :--- | :--- | :--- | :--- |
| `getStatus()` | `GET /api/v1/system/status` | Line 96 | ✅ Fully Implemented |
| `getDevices()` | `GET /api/v1/devices` | Line 113 | ✅ Fully Implemented |
| `addDevice()` | `POST /api/v1/devices` | Line 113 | ✅ Fully Implemented |
| `deleteDevice()` | `DELETE /api/v1/devices/:id` | Line 384 | ✅ Fully Implemented |
| `startDiscovery()` | `POST /api/v1/devices/discover` | Line 137 | ✅ Fully Implemented |
| `getDiscoveryProgress()` | `GET /api/v1/devices/discover/progress` | Line 167 | ✅ Fully Implemented |
| `getNetworkConfig()` | `GET /api/v1/devices/:id/network` | Line 396 | ✅ Fully Implemented |
| `previewNetworkConfig()` | `POST /api/v1/devices/:id/network/preview` | Line 397 | ✅ Fully Implemented |
| `applyNetworkConfig()` | `POST /api/v1/devices/:id/network/apply` | Line 408 | ✅ Fully Implemented |
| `getImageConfig()` | `GET /api/v1/devices/:id/image` | Line 430 | ✅ Fully Implemented |
| `previewImageConfig()` | `POST /api/v1/devices/:id/image/preview` | Line 431 | ✅ Fully Implemented |
| `applyImageConfig()` | `POST /api/v1/devices/:id/image/apply` | Line 442 | ✅ Fully Implemented |
| `getVideoConfigs()` | `GET /api/v1/devices/:id/encoding` | Line 464 | ✅ Fully Implemented |
| `sendPTZ()` | `POST /api/v1/devices/:id/ptz` | Line 472 | ✅ Fully Implemented |
| `getSnapshotURL()` | `GET /api/v1/devices/:id/snapshot` | Line 482 | ✅ Fully Implemented |
| `getChannels()` | `GET /api/v1/devices/:id/channels` | Line 492 | ✅ Fully Implemented |
| `setAnalogFormat()` | `POST /api/v1/devices/:id/channels/analog-format` | Line 493 | ✅ Fully Implemented |
| `runDiagnostics()` | `GET /api/v1/diagnostics/run` | Line 176 | ✅ Fully Implemented |
| `calcSubnet()` | `GET /api/v1/diagnostics/subnet` | Line 193 | ✅ Fully Implemented |
| `calcHDD()` | `GET /api/v1/diagnostics/hdd` | Line 206 | ✅ Fully Implemented |
| `calcBandwidth()` | `GET /api/v1/diagnostics/bandwidth` | Line 225 | ✅ Fully Implemented |
| `getLicensing()` | `GET /api/v1/licensing` | Line 243 | ✅ Fully Implemented |
| `activateLicense()` | `POST /api/v1/licensing/activate` | Line 251 | ✅ Fully Implemented |
| `generateGSTInvoice()`| `POST /api/v1/licensing/gst-invoice` | Line 273 | ✅ Fully Implemented |
| `getAuditLogs()` | `GET /api/v1/audit/logs` | Line 286 | ✅ Fully Implemented |
| `getCompatibility()` | `GET /api/v1/compatibility` | Line 292 | ✅ Fully Implemented |
| `getTechnicianReport()`| `GET /api/v1/reports/technician` | Line 346 | ✅ Fully Implemented |
| `getBatchJobs()` | `GET /api/v1/batch` | Line 317 | ✅ Fully Implemented |
| `createBatchJob()` | `POST /api/v1/batch` | Line 317 | ✅ Fully Implemented |

**Audit Result**: 29/29 routes mapped. Zero orphan API endpoints or disconnected frontend services.

---

## 4. Adapter Implementation Matrix
Evaluated against `adapters.ProtocolAdapter` interface contract:

| Interface Method | Hikvision ISAPI | Dahua CGI | ONVIF Core | CP PLUS (Wrapped) |
| :--- | :---: | :---: | :---: | :---: |
| `Name()` | ✅ | ✅ | ✅ | ✅ |
| `Vendor()` | ✅ | ✅ | ✅ | ✅ |
| `Detect()` | ✅ (`/ISAPI/System/deviceInfo`) | ✅ (`/cgi-bin/magicBox.cgi`) | ✅ (`GetDeviceInformation`) | ✅ (Dahua CGI probe) |
| `GetNetworkConfig()` | ✅ (`/ISAPI/.../interfaces/1`) | ✅ (`configManager.cgi?name=Network`) | ✅ (`GetNetworkInterfaces`) | ✅ (Delegated) |
| `ApplyNetworkConfig()`| ✅ (`PUT /ISAPI/.../interfaces/1`)| ✅ (`action=setConfig&name=Network`) | ✅ (`SetNetworkInterfaces`) | ✅ (Delegated) |
| `GetImageConfig()` | ✅ (`/ISAPI/Image/channels/1`) | ✅ (`configManager.cgi?name=VideoColor`) | ✅ (`GetImagingSettings`) | ✅ (Delegated) |
| `ApplyImageConfig()` | ✅ (`PUT /ISAPI/Image/channels/1`) | ✅ (`action=setConfig&name=VideoColor`) | ✅ (`SetImagingSettings`) | ✅ (Delegated) |
| `GetVideoEncodingConfig()`| ✅ | ✅ | ✅ | ✅ |
| `ApplyVideoEncodingConfig()`| ✅ | ✅ | ✅ | ✅ |
| `SendPTZ()` | ✅ (`/ISAPI/PTZCtrl/...`) | ✅ (`/cgi-bin/ptz.cgi`) | ✅ (`ContinuousMove` SOAP) | ✅ (Delegated) |
| `GetStreamURI()` | ✅ (`rtsp://...:554/Streaming/Channels/101`)| ✅ (`rtsp://...:554/cam/realmonitor`)| ✅ (`GetStreamUri` SOAP)| ✅ (Delegated) |
| `CaptureSnapshot()` | ✅ (`/picture` HTTP) | ✅ (`snapshot.cgi` HTTP) | ✅ (`GetSnapshotUri`) | ⚠️ Fallback SVG generator |
| `GetRecorderChannels()` | ✅ (XVR detection) | ✅ (`CoaxialSensor`) | ℹ️ Returns `ErrUnsupportedOperation` | ✅ (Formats as Cosmic) |
| `SetChannelAnalogMode()`| ✅ (UTC command) | ✅ (`CoaxialSensor Format`) | ℹ️ Returns `ErrUnsupportedOperation` | ✅ (Delegated) |

---

## 5. Defect, Security & Technical Debt Register

| ID | Category | Severity | Description | Current Status / Remediation Plan |
| :--- | :--- | :---: | :--- | :--- |
| **SEC-01** | Secret Handling | Medium | Device credentials stored in `sentinel_store.json` without AES encryption at rest. | Remediation in Phase 1/Phase 11: Implement OS Keychain / Encrypted Vault. |
| **NET-01** | Discovery | High | Discovery engine relies entirely on TCP port scanning (ports 80, 554, 8000, 37777). True **UDP ONVIF WS-Discovery (239.255.255.250:3702)** is absent. | Scheduled for Phase 2: Add UDP Multicast listener and probe dispatch. |
| **MOCK-01**| Media Processing | High | Live video player in `LiveView.tsx` shows canvas/SVG mock feeds. Real RTSP demuxing (FFmpeg/H.264 WebRTC or WebSocket bridge) is not yet active. | Scheduled for Phase 7: Implement FFmpeg worker for H.264/H.265 streams. |
| **OEM-01** | Hardware Reality | Medium | `cpplus.go` assumes CP PLUS devices are Dahua-derived CGI. CP PLUS Orange/Cosmic vs. Indigo vs. OEM Hikvision/TVT variants must be strictly model-differentiated. | Scheduled for Phase 6: Expand model-specific OUI and vendor family detection. |
| **DB-01** | Thread Safety | **RESOLVED**| Mutex recursion in `database.go` caused deadlock on `UpsertDevice`. | **Fixed in Phase 0**: Added `saveLocked()` helper. Verified with 1,000 concurrent operations. |

---

## 6. Baseline Test Report

### 1. Go Backend Test Suites
Executed with Go 1.23.2:
```text
=== RUN   TestAdaptersWithSimulator
--- PASS: TestAdaptersWithSimulator (0.01s)
PASS
ok  	cctv-sentinel/pkg/adapters	0.668s

=== RUN   TestConfigEngine12Steps
--- PASS: TestConfigEngine12Steps (0.01s)
PASS
ok  	cctv-sentinel/pkg/configengine	0.610s

=== RUN   TestStoreConcurrencyAndIntegrity
    database_test.go:94: Successfully verified store concurrency with 1000 concurrent operations. Persisted 1000 devices.
--- PASS: TestStoreConcurrencyAndIntegrity (6.00s)
PASS
ok  	cctv-sentinel/pkg/database	(cached)

=== RUN   TestCalculatorsAndDiagnostics
--- PASS: TestCalculatorsAndDiagnostics (0.00s)
PASS
ok  	cctv-sentinel/pkg/diagnostics	0.557s

=== RUN   TestLicensingAndGST
--- PASS: TestLicensingAndGST (0.01s)
PASS
ok  	cctv-sentinel/pkg/licensing	0.581s
```
* **Go Vet**: `go vet ./...` completed with **0 warnings / 0 errors**.

### 2. Frontend Production Build
Executed with Vite 5.4.21 & TypeScript 5.5.4:
```text
vite v5.4.21 building for production...
transforming...
✓ 2090 modules transformed.
rendering chunks...
computing gzip size...
dist/index.html                   0.72 kB │ gzip:  0.49 kB
dist/assets/index-72b3Kvio.css   23.35 kB │ gzip:  4.92 kB
dist/assets/index-DcIJJM-l.js   263.07 kB │ gzip: 70.19 kB
✓ built in 2.79s
```
* **TypeScript Check**: `tsc --noEmit` passed with **0 errors**.

---

## 7. Phase 0 Gate Assessment
* **Architecture Integrity**: Clean modular monolith established; clean contracts between core and protocol adapters.
* **Data Integrity**: Concurrency deadlocks resolved and stress-tested.
* **API Completeness**: All 29 UI services mapped to backend endpoints.
* **Simulator vs. Physical Reality**: Transparently documented. No claims of physical hardware certification exist in this baseline.

**Gate Status: PHASE 0 COMPLETE — READY FOR PHASE 1 APPROVAL.**
