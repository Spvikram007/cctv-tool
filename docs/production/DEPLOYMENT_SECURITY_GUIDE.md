# CCTV SENTINEL UNIVERSAL — PRODUCTION DEPLOYMENT & SECURITY GUIDE
## Enterprise Security Architecture, Cookie Hardening, Asymmetric Device Identity & Zero-Default Provisioning

**Document Version:** 2.1.0-PROD  
**Target Release:** Phase 6.1.7 Stabilization  
**Platform Owner:** Tech Defence Solutions  

---

## 1. Production Cookie & Origin Enforcement

### 1.1 Cookie Security Directives
All browser web sessions in production MUST strictly adhere to the following cookie attributes:

| Attribute | Session Cookie (`sentinel_session`) | CSRF Cookie (`sentinel_csrf`) | Security Justification |
|-----------|-------------------------------------|-------------------------------|-------------------------|
| **`HttpOnly`** | `true` | `false` | Prevents XSS token exfiltration; CSRF cookie must remain accessible to frontend JavaScript. |
| **`Secure`** | `true` (in production/HTTPS) | `true` (in production/HTTPS) | Ensures cookies are only transmitted over TLS encrypted channels. |
| **`SameSite`** | `SameSite=Strict` | `SameSite=Strict` | Blocks ambient cross-site transmission on cross-origin requests. |
| **`Path`** | `/` | `/` | Confines cookie scope to platform domain root. |
| **`Domain`** | *Omitted* | *Omitted* | Prevents subdomain cookie manipulation or broad scope leakage. |
| **Prefix** | `__Host-` (when HTTPS active) | `__Host-` (when HTTPS active) | RFC 6265bis prefix enforcement: requires Secure, Path=/, and no Domain. |

### 1.2 Production Fail-Closed Rules
In `SENTINEL_ENV=production`:
- Disabling `SENTINEL_SECURE_COOKIES=false` or `SENTINEL_COOKIE_HOST_PREFIX=false` **FAILS CLOSED** immediately on startup (fatal panic) with an explicit security configuration violation message.
- Accidental or malicious downgrades to insecure cookies are strictly prevented.

### 1.3 Trusted Reverse-Proxy Architecture
When CCTV Sentinel is deployed behind reverse proxies (Nginx, Traefik, HAProxy, AWS ALB):
- The server NEVER infers HTTPS solely from incoming connection state if behind a proxy.
- It validates the remote client IP against the explicit allowlist configured via `SENTINEL_TRUSTED_PROXIES`.
- Only requests from verified upstream proxy IPs are permitted to supply `X-Forwarded-Proto: https` to activate secure cookie headers.
- Untrusted direct clients attempting to spoof `X-Forwarded-Proto` are treated as unencrypted HTTP.

---

## 2. Asymmetric Ed25519 Native Workstation Device Identity

### 2.1 Threat Model & Security Posture
1. **Local Key Generation:** The native workstation application generates its own Ed25519 keypair locally using genuine OS entropy (`crypto/rand`).
2. **Zero Private Key Transmission:** The private key NEVER leaves the workstation and is NEVER transmitted over the network or stored in application source code, logs, or databases.
3. **OS Credential Vault:** Private keys and active bearer tokens are stored exclusively in platform credential managers:
   - **Windows:** Windows Credential Manager (`wincred` API via DPAPI).
   - **macOS:** Apple Keychain Services (`Security.framework`).
   - **Linux:** Secret Service API via `libsecret` / FreeDesktop D-Bus.
4. **Persistent Registry & Two-Phase Enrollment:** Workstation devices submit their public key to `/api/v1/auth/desktop/enroll`. They remain in `PENDING` status until an authenticated administrator reviews their cryptographic fingerprint (`SHA256:<hex>`) and approves them via `/api/v1/auth/desktop/devices/{id}/approve`.
5. **Atomic Challenge Consumption & Anti-Replay:** Challenges (`ch-<hex>`) are 256-bit cryptographically random nonces valid for 60 seconds. They are atomically consumed immediately on ANY verification attempt (both successful and failed) to prevent replay attacks.
6. **Device Revocation & Session Invalidation:** When a workstation is revoked (`POST .../revoke`), the server immediately terminates all active bearer sessions issued to that workstation.

```
       WORKSTATION (Native Tauri)                       BACKEND (CCTV Sentinel)
             |                                                    |
   [1] Generate Ed25519 Keypair locally                           |
   [2] Store Private Key in OS Vault                              |
   [3] Submit Public Key -------- POST /desktop/enroll ---------> | (Status: PENDING)
             |                                                    |
             |                                           [4] Admin Reviews Fingerprint
             |                                           [5] Admin Approves Device
             |                                                    |
   [6] Request Challenge -------- POST /desktop/challenge ------> | Check Approved & Active
             | <--------------- Return ch-<hex> (60s TTL) ------- | (Atomically Bound)
   [7] Sign ch with Private Key                                   |
   [8] POST /desktop/login -------------------------------------> |
       (device_id, ch, signature, credentials)                   | [9] ATOMIC CONSUMPTION
             |                                                    | [10] ed25519.Verify()
             |                                                    | [11] Argon2id Auth
             | <--- Return Bearer Token (Bound to DeviceID) ----- |
   [12] Store Token in OS Vault                                   |
```

---

## 3. First-Run Administrator Provisioning & Secret Hygiene

### 3.1 Zero-Default-Credential Policy
CCTV Sentinel Universal does NOT ship with hardcoded default accounts, default passwords (`Admin@Sentinel2026!`, `admin123`), or backdoor credentials.

1. **Initial Boot:** Fresh installations with an empty user store enter `setupMode = true` and generate a single-use 256-bit cryptographically secure setup token (`sent-setup-<hex>`).
2. **Filesystem ACL Hardening:** The setup token is written to `sentinel-setup.token`. 
   - **Windows:** Hardened with `icacls` to strip inheritance (`/inheritance:r`) and grant Full Control only to SYSTEM (`*S-1-5-18`), Administrators (`*S-1-5-32-544`), and the current executing user (`%USERNAME%`).
   - **POSIX:** Restrictive `0600` permissions.
3. **Persistent User Accounts (Schema v3):** When first-run setup is completed with an operator-chosen strong master password, the administrator account is hashed with Argon2id and persisted to `sentinel_store.json`. Accounts permanently survive server restarts.
4. **Permanent Invalidation:** The setup token is destroyed immediately from memory and erased from disk. All subsequent calls to `/api/v1/auth/setup` return HTTP 403 `SETUP_DISABLED`.
5. **Secret Redaction:** Setup tokens, session keys, and camera credentials must NEVER be logged to audit ledgers, console outputs, or Git commits.

---

## 4. Hardware Safety & Technician Operational Rules

- **Physical PTZ Motor Actuation (Vatilon H43):** Remains strictly under **PENDING OPERATOR APPROVAL** until physical site clearance is confirmed by the technician on site.
- **Simulator & Field Mode:** Hardware simulators operate on loopback ports (Hikvision ISAPI `:18081`, Dahua CGI `:18082`, Generic ONVIF `:18083`, RTSP `:18554`).
