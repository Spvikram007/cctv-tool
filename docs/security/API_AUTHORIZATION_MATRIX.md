# CCTV SENTINEL UNIVERSAL — API AUTHORIZATION & TENANT ISOLATION MATRIX

**Product:** CCTV SENTINEL UNIVERSAL  
**Phase:** Phase 6.1.2 Final Stabilization  
**Company:** Tech Defence Solutions  
**Security Level:** Enterprise Mission-Critical / Defense-Grade  
**Branch:** `security/phase-6-1-7-1-stabilization`  

---

## 1. Executive Overview

This specification establishes the authoritative, route-by-route authorization, multi-tenant isolation, and session handling policy for CCTV Sentinel Universal.

The architecture adheres strictly to:
1. **Principle of Least Privilege:** Operators receive only the specific capabilities required for their organizational role.
2. **Explicit Deny by Default:** Any missing permission, missing tenant ownership, or ambiguous resource identifier strictly rejects access.
3. **Physical Safety & Separation of Duties:** Viewing live footage (`VIEW_LIVE`) does **NOT** grant authority to move a camera's robotic motors (`PTZ_CONTROL`) or reconfigure camera coordinates (`PTZ_MANAGE`).
4. **Tenant & Site Boundaries:** Non-super-admin users are strictly isolated to their provisioned `TenantID` and `SiteID`. Cross-tenant reads, writes, batch operations, and firmware upgrades are rejected with HTTP 403 `CROSS_TENANT_ACCESS_DENIED`.

---

## 2. Role-Based Access Control (RBAC) Permissions Matrix

| Permission String | Description | Admin | Technician | Auditor | Viewer |
| :--- | :--- | :---: | :---: | :---: | :---: |
| `VIEW_LIVE` | View live video streams, enumerate devices/recorders, read telemetry | ✅ | ✅ | ✅ | ✅ |
| `PLAYBACK_VIEW` | Search recording timelines, scrub playback sessions, view test frames | ✅ | ✅ | ✅ | ✅ |
| `PLAYBACK_EXPORT` | Export recording video clips to disk or network storage | ✅ | ✅ | ✅ | ❌ |
| `PTZ_CONTROL` | Actuate physical pan/tilt/zoom motors, stop, focus/iris, goto presets, autoscan | ✅ | ✅ | ❌ | ❌ |
| `PTZ_MANAGE` | Create, rename, delete coordinate presets, build patrol tours, calibrate home | ✅ | ✅ | ❌ | ❌ |
| `DEVICE_CONFIG` | Modify network settings, image adjustments, encoding, device credentials | ✅ | ✅ | ❌ | ❌ |
| `RECORDER_MANAGE` | Configure analog signal formats, coaxial UTC, register IP cams, schedules | ✅ | ✅ | ❌ | ❌ |
| `EVENT_MANAGE` | Create & cancel event subscriptions, create & delete automated alarm rules | ✅ | ✅ | ❌ | ❌ |
| `ALARM_MANAGE` | Acknowledge, assign, resolve, and dismiss active surveillance alarms | ✅ | ✅ | ✅ | ❌ |
| `ALARM_EXPORT` | Export cryptographic alarm ledger and triage history (CSV / JSON) | ✅ | ✅ | ✅ | ❌ |
| `BATCH_EXECUTE` | Generate and execute fleet-wide batch configuration workflows | ✅ | ✅ | ❌ | ❌ |
| `REBOOT` | Reboot physical cameras/recorders, stage storage formatting | ✅ | ✅ | ❌ | ❌ |
| `AUDIT_VIEW` | Inspect tamper-evident audit logs and cryptographic HMAC checkpoints | ✅ | ✅ | ✅ | ❌ |
| `FIRMWARE_UPGRADE` | Stage and execute signed firmware package updates | ✅ | ❌ | ❌ | ❌ |
| `LICENSE_MANAGE` | Activate commercial licenses and generate GST compliance invoices | ✅ | ❌ | ❌ | ❌ |
| `USER_MANAGEMENT` | Provision user accounts, assign roles, rotate audit HMAC checkpoint keys | ✅ | ❌ | ❌ | ❌ |

---

## 3. Comprehensive Route-by-Route Endpoint Matrix

### 3.1 Public & Unauthenticated Endpoints

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :---: |
| `GET` | `/api/v1/healthz` | Service liveness probe & uptime | Public |
| `GET` | `/api/v1/readyz` | Service readiness probe (DB, registry, licensing) | Public |
| `GET` | `/api/v1/auth/setup/status` | Reports whether first-run setup is required | Public |
| `POST` | `/api/v1/auth/setup` | Initializes first administrator using single-use 24-hr token | Setup Token |
| `POST` | `/api/v1/auth/login` | Authenticates operator, returns JWT/session, sets cookie | Public |
| `POST` | `/api/v1/security/login` | Alias for operator authentication | Public |
| `POST` | `/api/v1/auth/logout` | Revokes session token server-side and expires cookie | Public / Bearer |
| `POST` | `/api/v1/security/logout` | Revokes session token server-side and expires cookie | Public / Bearer |

---

### 3.2 Devices & Discovery Endpoints

| Method | Endpoint | Required Permission | Tenant Ownership Check | Admin | Tech | Auditor | Viewer |
| :--- | :--- | :--- | :---: | :---: | :---: | :---: | :---: |
| `GET` | `/api/v1/devices` | `VIEW_LIVE` | Filtered by tenant/site | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/devices` | `DEVICE_CONFIG` | Scoped to caller tenant | 201 | 201 | 403 | 403 |
| `GET` | `/api/v1/devices/{id}` | `VIEW_LIVE` | Strict resource check | 200 | 200 | 200 | 200 |
| `DELETE` | `/api/v1/devices/{id}` | `DEVICE_CONFIG` | Strict resource check | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/migrate` | `USER_MANAGEMENT` (Super-Admin) | Legacy unassigned claim | 200 | 403 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/reboot` | `REBOOT` | Strict resource check | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/auth` | `DEVICE_CONFIG` | Strict resource check | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/devices/{id}/capabilities` | `VIEW_LIVE` | Strict resource check | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/devices/{id}/config/preview` | `DEVICE_CONFIG` | Strict resource check | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/config/apply` | `DEVICE_CONFIG` | Strict resource check | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/discover` | `DEVICE_CONFIG` | Fleet-wide scan | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/devices/discover/progress` | `VIEW_LIVE` | Fleet-wide progress | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/discovery/interfaces` | `DEVICE_CONFIG` | Network adapter list | 200 | 200 | 403 | 403 |

---

### 3.3 Robotic PTZ Control Endpoints

| Method | Endpoint | Required Permission | Action Scope | Admin | Tech | Auditor | Viewer |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `GET` | `/api/v1/devices/{id}/ptz` | `VIEW_LIVE` | Read PTZ capabilities | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/devices/{id}/ptz/capabilities` | `VIEW_LIVE` | Read PTZ capabilities | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/devices/{id}/ptz/move` | `PTZ_CONTROL` | Actuate pan/tilt/zoom motors | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/stop` | `PTZ_CONTROL` | Emergency motor brake | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/focus-iris` | `PTZ_CONTROL` | Motorized focus/iris step | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/devices/{id}/ptz/presets` | `VIEW_LIVE` | Enumerate saved presets | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/devices/{id}/ptz/presets` | `PTZ_MANAGE` | Save new coordinate preset | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/presets/{pId}/goto` | `PTZ_CONTROL` | Navigate motor to preset | 200 | 200 | 403 | 403 |
| `PUT` | `/api/v1/devices/{id}/ptz/presets/{pId}` | `PTZ_MANAGE` | Rename preset label | 200 | 200 | 403 | 403 |
| `DELETE` | `/api/v1/devices/{id}/ptz/presets/{pId}` | `PTZ_MANAGE` | Erase coordinate preset | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/devices/{id}/ptz/patrols` | `VIEW_LIVE` | Enumerate patrol tours | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/devices/{id}/ptz/patrols` | `PTZ_MANAGE` | Create multi-waypoint tour | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/patrols/{tId}/run` | `PTZ_CONTROL` | Start motor cruise cycle | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/patrols/{tId}/stop` | `PTZ_CONTROL` | Stop cruise tour | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/home` (`goto`) | `PTZ_CONTROL` | Navigate to zero index | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/home` (`set`) | `PTZ_MANAGE` | Calibrate zero index origin | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/devices/{id}/ptz/autoscan` | `PTZ_CONTROL` | Start continuous sweep | 200 | 200 | 403 | 403 |

---

### 3.4 DVR / NVR / XVR Recorders & Playback Endpoints

| Method | Endpoint | Required Permission | Description | Admin | Tech | Auditor | Viewer |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `GET` | `/api/v1/recorders` | `VIEW_LIVE` | List tenant recorders | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/recorders/{id}` | `VIEW_LIVE` | Read recorder profile | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/recorders/{id}/channels` | `VIEW_LIVE` | List recorder channels | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/recorders/{id}/analog-format` | `RECORDER_MANAGE` | Change AHD/TVI/CVI/CVBS | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/recorders/{id}/coaxial-control` | `PTZ_CONTROL` | Coaxial UTC PTZ command | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/recorders/{id}/ip-registration` | `RECORDER_MANAGE` | Map IP camera to channel | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/recorders/{id}/schedule` | `VIEW_LIVE` | Read recording schedule | 200 | 200 | 200 | 200 |
| `PUT` | `/api/v1/recorders/{id}/schedule` | `RECORDER_MANAGE` | Update recording schedule | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/recorders/{id}/search-recordings` | `PLAYBACK_VIEW` | Search timeline segments | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/recorders/{id}/playback-session` | `PLAYBACK_VIEW` | Start playback scrubber | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/recorders/{id}/export` | `PLAYBACK_EXPORT` | Export clip to disk | 200 | 200 | 200 | 403 |
| `GET` | `/api/v1/recorders/{id}/storage` | `VIEW_LIVE` | Storage health report | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/recorders/{id}/storage/format-preview` | `REBOOT` | Stage format token | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/recorders/{id}/storage/format-approve` | `REBOOT` | Execute disk format | 200 | 200 | 403 | 403 |

---

### 3.5 Real-Time Telemetry, Events & Alarms Endpoints

| Method | Endpoint | Required Permission | Description | Admin | Tech | Auditor | Viewer |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `GET` | `/api/v1/events/subscriptions` | `VIEW_LIVE` | List active subscriptions | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/events/subscriptions` | `EVENT_MANAGE` | Create event subscription | 200 | 200 | 403 | 403 |
| `DELETE`| `/api/v1/events/subscriptions/{id}` | `EVENT_MANAGE` | Terminate subscription | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/events/live` | `VIEW_LIVE` | Filtered live event log | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/alarms` | `VIEW_LIVE` | Filtered active alarms | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/alarms/stats` | `VIEW_LIVE` | Alarm severity statistics | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/alarms/rules` | `VIEW_LIVE` | Enumerate alarm rules | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/alarms/rules` | `EVENT_MANAGE` | Create automation rule | 200 | 200 | 403 | 403 |
| `DELETE`| `/api/v1/alarms/rules/{id}` | `EVENT_MANAGE` | Erase automation rule | 200 | 200 | 403 | 403 |
| `GET` | `/api/v1/alarms/export` | `ALARM_EXPORT` | Export alarm ledger | 200 | 200 | 200 | 403 |
| `POST` | `/api/v1/alarms/{id}/acknowledge` | `ALARM_MANAGE` | Acknowledge incident | 200 | 200 | 200 | 403 |
| `POST` | `/api/v1/alarms/{id}/assign` | `ALARM_MANAGE` | Assign to technician | 200 | 200 | 200 | 403 |
| `POST` | `/api/v1/alarms/{id}/resolve` | `ALARM_MANAGE` | Resolve incident | 200 | 200 | 200 | 403 |
| `POST` | `/api/v1/alarms/{id}/dismiss` | `ALARM_MANAGE` | Dismiss false positive | 200 | 200 | 200 | 403 |

---

### 3.6 Batch Operations & Firmware Management

| Method | Endpoint | Required Permission | Description | Admin | Tech | Auditor | Viewer |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `POST` | `/api/v1/batch/preview` | `BATCH_EXECUTE` | Cross-tenant checked preview | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/batch/execute` | `BATCH_EXECUTE` | Bounded concurrent execution | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/batch/compare` | `VIEW_LIVE` | Cross-tenant checked compare | 200 | 200 | 200 | 200 |
| `GET` | `/api/v1/batch/export` | `BATCH_EXECUTE` | Tenant-filtered HMAC export | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/batch/import` | `BATCH_EXECUTE` | Scoped backup import | 200 | 200 | 403 | 403 |
| `POST` | `/api/v1/firmware/check-compatibility` | `VIEW_LIVE` | Tenant-verified eligibility | 200 | 200 | 200 | 200 |
| `POST` | `/api/v1/firmware/preview` | `FIRMWARE_UPGRADE` | Staged firmware update | 200 | 403 | 403 | 403 |
| `POST` | `/api/v1/firmware/approve` | `FIRMWARE_UPGRADE` | Execute binary flash | 200 | 403 | 403 | 403 |

---

### 3.7 Security, Administration & Audit Verification

| Method | Endpoint | Required Permission | Description | Admin | Tech | Auditor | Viewer |
| :--- | :--- | :--- | :--- | :---: | :---: | :---: | :---: |
| `GET` | `/api/v1/security/users` | `USER_MANAGEMENT` | Enumerate operator accounts | 200 | 403 | 403 | 403 |
| `POST` | `/api/v1/security/users` | `USER_MANAGEMENT` | Provision operator accounts | 201 | 403 | 403 | 403 |
| `GET` | `/api/v1/security/vault` | `USER_MANAGEMENT` | Inspect AES-256-GCM vault | 200 | 403 | 403 | 403 |
| `POST` | `/api/v1/audit/rotate-key` | `USER_MANAGEMENT` | Rotate HMAC checkpoint key | 200 | 403 | 403 | 403 |
| `GET` | `/api/v1/audit/logs` | `AUDIT_VIEW` | Tamper-evident ledger entries | 200 | 200 | 200 | 403 |
| `GET` | `/api/v1/audit/checkpoints` | `AUDIT_VIEW` | Periodic cryptographic hashes | 200 | 200 | 200 | 403 |
| `GET` | `/api/v1/audit/export/signed` | `AUDIT_VIEW` | Digitally signed ledger export | 200 | 200 | 200 | 403 |
| `POST` | `/api/v1/audit/verify` | `AUDIT_VIEW` | Verify SHA-256 chain integrity | 200 | 200 | 200 | 403 |
| `GET` | `/api/v1/reports/technician` | `AUDIT_VIEW` | Tenant-filtered health report | 200 | 200 | 200 | 403 |
| `POST` | `/api/v1/licensing/activate` | `LICENSE_MANAGE` | Cryptographic license unlock | 200 | 403 | 403 | 403 |

---

## 4. Multi-Tenant and Multi-Site Isolation Policy

### 4.1 Boundary Enforcement Logic (`checkTenantAccess`)
```text
IF caller is Super-Admin (Role: Admin AND TenantID in {"", "default", "system"}):
    ALLOW ALL RESOURCES

IF resource.TenantID == "":
    DENY (403 CROSS_TENANT_ACCESS_DENIED) — Legacy unassigned devices require super-admin claim

IF caller.TenantID != resource.TenantID:
    DENY (403 CROSS_TENANT_ACCESS_DENIED) — Cross-tenant boundary breach

IF caller.SiteID NOT IN {"", "default", "all"}:
    IF resource.SiteID == "" OR resource.SiteID != caller.SiteID:
        DENY (403 CROSS_TENANT_ACCESS_DENIED) — Cross-site boundary breach

ALLOW ACCESS
```

### 4.2 Legacy Unassigned Device Migration Protocol
1. **Initial Discovery State:** Devices discovered on the network without explicit tenant tags have `TenantID: ""` and `SiteID: ""`.
2. **Access Lockdown:** Tenant-scoped operators cannot view, configure, control, or claim unassigned devices.
3. **Super-Admin Assignment:** A genuine global administrator issues `POST /api/v1/devices/{id}/migrate` with payload:
   ```json
   {
     "target_tenant": "acme-corp",
     "target_site": "bldg-3-east"
   }
   ```
4. **Audit Immutability:** The migration event is written to the cryptographic audit trail recording both the previous and new tenant identifiers. Once claimed, the device becomes accessible to operators authorized for `acme-corp`.

---

## 5. Session Authentication & Token Security Standards

1. **Authorization Header:**
   - Standard format: `Authorization: Bearer <session-token>` (case-insensitive `Bearer ` prefix).
2. **Secure Session Cookie:**
   - Name: `sentinel_session`
   - Attributes: `HttpOnly; SameSite=Strict; Path=/`
   - Secure flag enabled automatically when TLS is present (`r.TLS != nil`).
3. **URL Query-String Parameters Strictly Prohibited:**
   - Parameter `?token=...` is explicitly discarded. Requests relying on query strings receive HTTP 401 `UNAUTHORIZED`.
4. **Server-Side Session Revocation:**
   - Calling `/api/v1/security/logout` deletes the session from the in-memory `RBACManager` session map and issues an expired `sentinel_session` cookie (`Max-Age: -1`).
   - Subsequent calls using the revoked session token or cookie immediately fail with HTTP 401.

---

## 6. Standardized API Security Error Codes

| HTTP Status | Error Code | Meaning |
| :---: | :--- | :--- |
| `401` | `UNAUTHORIZED` | Missing or invalid Bearer session token / cookie |
| `401` | `SESSION_EXPIRED` | Session validity window exceeded; re-authentication required |
| `401` | `AUTH_FAILED` | Incorrect username or password |
| `403` | `FORBIDDEN` | Caller lacks the specific permission string required for the action |
| `403` | `INSUFFICIENT_PERMISSIONS` | Caller's role cannot perform this privileged operation |
| `403` | `CROSS_TENANT_ACCESS_DENIED` | Attempted access to a device, recorder, or alarm on another tenant or site |
| `403` | `SETUP_DISABLED` | First-run setup has already completed; setup token permanently disabled |
| `429` | `RATE_LIMIT_EXCEEDED` | Client exceeded IP-based rate limiting thresholds (token bucket) |
