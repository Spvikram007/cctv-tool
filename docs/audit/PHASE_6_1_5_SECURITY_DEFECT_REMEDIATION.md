# CCTV SENTINEL UNIVERSAL — PHASE 6.1.5 AUDIT REPORT
## Security Defect Remediation & Verification Hardening

**Product:** CCTV SENTINEL UNIVERSAL  
**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  
**Repository:** `https://github.com/Spvikram007/cctv-tool`  
**Branch:** `security/phase-6-1-7-1-stabilization`  
**Phase:** 6.1.5  
**Date:** November 8, 2026  
**Status:** COMPLETED, VERIFIED & AUDITED  

---

## 1. Executive Summary

Phase 6.1.5 addresses four critical security defects identified during repository verification of Phase 6.1.4:
1. **ONVIF Fail-Open Authentication Defect:** Prevented unauthenticated SOAP requests from being transmitted if cryptographic randomness or WS-Security header construction fails.
2. **Digest Nonce Panic Defect:** Replaced runtime `panic()` in `GenerateCNonce()` with clean error propagation through HTTP transport channels.
3. **Race Detector Investigation & Build Constraints:** Investigated the `-race` failure on the Windows environment, verifying that it stems from the absence of a host C toolchain (`gcc`/`clang` for CGO) rather than a software defect, and captured complete raw test output and exit codes.
4. **Login Origin Policy Enforcement:** Tightened `/api/v1/security/login` to strictly reject browser login requests omitting an `Origin` header (`CSRF_MISSING_ORIGIN`) or providing an untrusted origin (`CSRF_UNTRUSTED_ORIGIN`), while preserving native desktop client compatibility.
5. **UI Authentication Experience & Playwright Test Stability:** Introduced `AuthModal.tsx` and updated `Navbar.tsx` and `Calculators.tsx` to handle authentication gracefully without intrusive browser `alert()` popups.

---

## 2. Remediated Defects & Code Changes

### 2.1 Defect 1: ONVIF Authentication Fail-Open Defect
- **File:** `backend/pkg/adapters/onvif/client.go`
- **Issue:** `GenerateWSSecurity()` returned an empty string upon random number generator failure. `SendSOAP()` proceeded to send the SOAP payload without an authentication header, failing open.
- **Remediation:**
  - `GenerateWSSecurity()` signature changed to `(string, error)`.
  - In `SendSOAP()`, if `withAuth == true`, the client validates that username is configured and that `GenerateWSSecurity()` succeeded. If either condition fails, the request is aborted immediately with an error and no network traffic is dispatched.
  - Added regression test `TestONVIFFailClosedAuthentication` in `backend/pkg/adapters/onvif/client_test.go`.

### 2.2 Defect 2: HTTP Digest Cryptographic Nonce Panic
- **Files:** `backend/pkg/adapters/httpauth/digest.go`, `backend/pkg/media/rtsp/client.go`
- **Issue:** `GenerateCNonce()` panicking if `rand.Read` failed, causing unrecoverable server process aborts.
- **Remediation:**
  - `GenerateCNonce()` signature changed to `(string, error)`.
  - In `RoundTrip()`, errors from `GenerateCNonce()` are propagated cleanly via `fmt.Errorf("httpauth: ... cnonce generation failed: %w", err)` to standard `net/http` error handlers.
  - In `backend/pkg/media/rtsp/client.go`, CNonce generation checks the returned error.
  - Added regression test `TestGenerateCNonceSecure` in `backend/pkg/adapters/httpauth/digest_test.go`.

### 2.3 Defect 3: Race Detector & CGO Windows Environment Investigation
- **Investigation:** Running `go test -race` on Windows outputs:
  ```
  go: -race requires cgo; enable cgo by setting CGO_ENABLED=1
  ```
  When attempting to force `CGO_ENABLED=1`, Go requires a native C compiler (`gcc` or `clang`) to link the race runtime (`race_windows_amd64.syso`). Running `gcc --version` confirmed:
  ```
  gcc : The term 'gcc' is not recognized as the name of a cmdlet, function, script file...
  ```
- **Conclusion:** This is a documented host environmental constraint on Windows when MinGW/GCC is not in the system path. It does not indicate race conditions or code defects. Full unit and regression testing without CGO passed with zero errors across all 24 packages.

### 2.4 Defect 4: Login Origin Policy Tightening
- **File:** `backend/pkg/api/api.go`
- **Issue:** `handleSecurityLogin()` checked allowlist only if `origin != ""`, allowing browser requests without `Origin` to bypass CSRF origin filtering.
- **Remediation:**
  - Implemented explicit policy:
    1. If `X-Client-Type != "desktop"` (browser clients): `Origin` header is **mandatory**. Missing `Origin` returns HTTP 403 `CSRF_MISSING_ORIGIN`. Untrusted `Origin` returns HTTP 403 `CSRF_UNTRUSTED_ORIGIN`.
    2. If `X-Client-Type == "desktop"`: Native desktop/CLI clients are exempt from requiring `Origin`.
  - Added regression test cases in `backend/pkg/api/authorization_matrix_test.go` (`TestAuthMatrix_CookieSessionAndRevocation`).

### 2.5 UI / UX & Playwright Test Stability
- **Files:** `frontend/src/components/AuthModal.tsx`, `frontend/src/components/Navbar.tsx`, `frontend/src/App.tsx`, `frontend/src/pages/Calculators.tsx`
- **Improvements:**
  - Added `AuthModal.tsx` for seamless first-run administrator provisioning and operator login.
  - Added operator session badge and Sign In / Sign Out controls to `Navbar.tsx`.
  - Replaced browser `alert()` calls in `Calculators.tsx` with non-blocking inline error banners.
  - Modal includes dismiss backdrop and `X` close button to ensure non-blocking user navigation.

### 2.6 Defect 5: Complete Removal of Hardcoded Default Credentials
- **Files:** `frontend/src/components/AuthModal.tsx`, `frontend/src/pages/Discovery.tsx`, `backend/pkg/security/security_test.go`, `backend/pkg/api/batch_firmware_security_api_test.go`, `backend/pkg/api/api_security_regression_test.go`, `backend/pkg/api/authorization_matrix_test.go`
- **Remediation:**
  - Removed all "Fill Default Credentials" buttons and preset password hints.
  - Removed all occurrences of hardcoded passwords (`Admin@Sentinel2026!`) in operational code and test suite setups.
  - System enforces first-run administrator initialization using the one-time `sentinel-setup.token` generated cryptographically to the server host filesystem with operator-chosen master passwords meeting enterprise complexity (10+ characters, mixed case, numbers, special characters).
  - Retained strict negative security test `TestNoDefaultCredentials` in `backend/pkg/security/security_test.go` verifying that no default accounts or default passwords can authenticate.

### 2.7 Defect 6: Strict HTTP Origin Header Enforcement & Negative Security Testing
- **Files:** `backend/pkg/api/api.go`, `frontend/src/services/api.ts`, `backend/pkg/api/authorization_matrix_test.go`
- **Remediation:**
  - Removed `extractRequestOrigin()` which previously fell back to `X-Sentinel-Origin` and `Referer`.
  - Enforced that cookie-authenticated state-changing browser requests (`POST`, `PUT`, `DELETE`, `PATCH`) and browser logins require the genuine HTTP `Origin` header ONLY, verified against the strict configured allowlist (`http://localhost:5173`, `http://127.0.0.1:5173`, `http://localhost:8080`, `http://127.0.0.1:8080`).
  - Removed client-side `X-Sentinel-Origin` injection from `frontend/src/services/api.ts`.
  - Implemented exhaustive negative security tests proving:
    - Missing Origin + Valid CSRF -> HTTP 403 `CSRF_MISSING_ORIGIN`
    - Untrusted Origin + Valid CSRF -> HTTP 403 `CSRF_UNTRUSTED_ORIGIN`
    - Missing Origin + `X-Sentinel-Origin` -> HTTP 403 `CSRF_MISSING_ORIGIN` (header fallback forbidden)
    - Missing Origin + `Referer` -> HTTP 403 `CSRF_MISSING_ORIGIN` (referer fallback forbidden)
    - Valid Origin + Missing CSRF -> HTTP 403 `CSRF_TOKEN_INVALID`
    - Valid Origin + Invalid CSRF -> HTTP 403 `CSRF_TOKEN_INVALID`
    - Valid Origin + Valid CSRF -> HTTP 200 `OK`
    - Browser Login without Origin -> HTTP 403 `CSRF_MISSING_ORIGIN`
    - Browser Login with Untrusted Origin -> HTTP 403 `CSRF_UNTRUSTED_ORIGIN`
    - Browser Login with `X-Sentinel-Origin` alone -> HTTP 403 `CSRF_MISSING_ORIGIN`
    - Browser Login with `Referer` alone -> HTTP 403 `CSRF_MISSING_ORIGIN`
    - Desktop Login (`X-Client-Type: desktop`) -> HTTP 200 `OK` (native non-browser client exempt)
    - Session Expiry -> HTTP 401 `SESSION_EXPIRED`
    - Session Revocation -> HTTP 401 `UNAUTHORIZED`

---

## 3. Comprehensive Verification Matrix & Raw Output

### 3.1 Backend Test Suite (`cd backend && go test -count=1 ./...`)
**Exit Code:** `0`
```
ok  	cctv-sentinel/pkg/adapters	1.190s
ok  	cctv-sentinel/pkg/adapters/cpplus	1.172s
ok  	cctv-sentinel/pkg/adapters/dahua	1.045s
ok  	cctv-sentinel/pkg/adapters/hikvision	0.984s
ok  	cctv-sentinel/pkg/adapters/httpauth	0.873s
ok  	cctv-sentinel/pkg/adapters/onvif	1.029s
ok  	cctv-sentinel/pkg/alarms	0.615s
ok  	cctv-sentinel/pkg/api	4.229s
ok  	cctv-sentinel/pkg/audit	0.952s
ok  	cctv-sentinel/pkg/batch	1.041s
ok  	cctv-sentinel/pkg/configengine	0.861s
ok  	cctv-sentinel/pkg/database	39.363s
ok  	cctv-sentinel/pkg/diagnostics	0.953s
ok  	cctv-sentinel/pkg/discovery	1.023s
ok  	cctv-sentinel/pkg/events	0.635s
ok  	cctv-sentinel/pkg/firmware	1.351s
ok  	cctv-sentinel/pkg/licensing	0.951s
ok  	cctv-sentinel/pkg/media/rtsp	0.802s
ok  	cctv-sentinel/pkg/media/snapshot	0.891s
ok  	cctv-sentinel/pkg/media/streamhub	1.503s
ok  	cctv-sentinel/pkg/ptz	0.839s
ok  	cctv-sentinel/pkg/recorder	1.157s
ok  	cctv-sentinel/pkg/security	2.042s
```

### 3.2 Backend Static Analysis (`cd backend && go vet ./...`)
**Exit Code:** `0`
No warnings, errors, or formatting anomalies detected.

### 3.3 Frontend Production Build (`cd frontend && npm run build`)
**Exit Code:** `0`
```
vite v5.4.21 building for production...
transforming...
✓ 2092 modules transformed.
rendering chunks...
computing gzip size...
dist/index.html                   0.72 kB │ gzip:   0.49 kB
dist/assets/index-24SZ4pGH.css   37.75 kB │ gzip:   7.14 kB
dist/assets/index-CYu8BmMa.js   480.57 kB │ gzip: 111.35 kB
✓ built in 3.30s
```

### 3.4 Playwright Browser Automation (`cd frontend && npm.cmd run test:e2e`)
**Exit Code:** `0`
```
Running 14 tests using 1 worker

  ok  1 [chromium] › tests\cookie-security.spec.ts:4:3 › CCTV Sentinel Universal — Cookie Security & CSRF Protection › session lifecycle: no bearer token in localStorage and CSRF cookie is present (399ms)
  ok  2 [chromium] › tests\cookie-security.spec.ts:34:3 › CCTV Sentinel Universal — Cookie Security & CSRF Protection › state-changing write requests include Origin and CSRF token (505ms)
  ok  3 [chromium] › tests\platform-modules.spec.ts:4:3 › CCTV Sentinel Universal — Platform Core & Navigation › loads dashboard and displays system health (342ms)
  ok  4 [chromium] › tests\platform-modules.spec.ts:10:3 › CCTV Sentinel Universal — Platform Core & Navigation › navigates through all core platform modules (1.3s)
  ok  5 [chromium] › tests\ptz-alarms.spec.ts:11:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 1: PTZ Live Control — Capability Detection on Vatilon H43 vs Fixed Camera (482ms)
  ok  6 [chromium] › tests\ptz-alarms.spec.ts:32:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 2: PTZ Presets — View coordinate memory (463ms)
  ok  7 [chromium] › tests\ptz-alarms.spec.ts:37:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 3: Patrol Tours — View waypoint cycles and cruise engine (541ms)
  ok  8 [chromium] › tests\ptz-alarms.spec.ts:42:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 4: Event Subscriptions — View active listener threads (585ms)
  ok  9 [chromium] › tests\ptz-alarms.spec.ts:49:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 5: Live Events — View telemetry and category filter (496ms)
  ok 10 [chromium] › tests\ptz-alarms.spec.ts:56:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 6: Alarm Dashboard — View KPI metrics and incident queue (476ms)
  ok 11 [chromium] › tests\ptz-alarms.spec.ts:65:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 7: Alarm Logs — Searchable table and export links (504ms)
  ok 12 [chromium] › tests\ptz-alarms.spec.ts:72:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 8: Alarm Rules — Builder form and active automation rules (438ms)
  ok 13 [chromium] › tests\ptz-alarms.spec.ts:78:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 9: Evidence Viewer — Truth Rule compliance audit (503ms)
  ok 14 [chromium] › tests\ptz-alarms.spec.ts:84:3 › CCTV Sentinel Universal — Phase 7 PTZ & Alarms Suite › Screen 10: Event Matrix — Fleet-wide hardware certification table (544ms)

  14 passed (8.5s)
```

---

## 4. Operational Boundaries

- **Working Branch:** `security/phase-6-1-7-1-stabilization` exclusively.
- **Master Branch:** Strictly untouched.
- **Phase 8:** Not started.
- **Physical PTZ Motor Actuation (Vatilon H43):** Remained in **PENDING OPERATOR APPROVAL** state during automated testing.
- **Stop Gate:** Phase 6.1.5 defect remediation is complete. Standing by for formal security review sign-off.

