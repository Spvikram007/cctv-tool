# CCTV SENTINEL UNIVERSAL — PRODUCTION COOKIE & CSRF SECURITY SPECIFICATION

**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** Web Session Management & Transport Security  
**Standard Compliance:** RFC 6265bis, OWASP CSRF Prevention Cheat Sheet, NIST SP 800-63B  
**Author:** Tech Defence Solutions Security Engineering Team  
**Status:** IMPLEMENTED & AUDITED  

---

## 1. Overview & Architecture

CCTV Sentinel Universal implements an enterprise-grade cookie management and Cross-Site Request Forgery (CSRF) defense mechanism. The architecture supports modern browser security standards while remaining fully compatible with direct HTTPS, reverse proxies (Nginx, Traefik, HAProxy, AWS ALB), and local development environments.

```
+---------------------------------------------------------------------------------------+
|                                    INCOMING REQUEST                                    |
+---------------------------------------------------------------------------------------+
                                           |
                                           v
                         +-----------------------------------+
                         | Extract Session & Transport State |
                         +-----------------------------------+
                                           |
                 +-------------------------+-------------------------+
                 |                                                   |
                 v                                                   v
        [Bearer Token Auth]                                 [Cookie Token Auth]
                 |                                                   |
                 | (Desktop Client / CLI)                            v
                 |                                      +-------------------------+
                 |                                      | Is Method State-Change? |
                 |                                      | (POST, PUT, PATCH, DEL) |
                 |                                      +-------------------------+
                 |                                        |                     |
                 |                                        | Yes                 | No (GET/HEAD)
                 |                                        v                     v
                 |                            +-----------------------+     [Pass to Handler]
                 |                            | Origin Present in     |
                 |                            | Allowed Origin List?  |
                 |                            +-----------------------+
                 |                              |                   |
                 |                              | No                | Yes
                 |                              v                   v
                 |                       [403 FORBIDDEN]   +-----------------------+
                 |                    (MISSING/UNTRUSTED)  | Validate CSRF Token   |
                 |                                         | (Constant-Time Match) |
                 |                                         +-----------------------+
                 |                                           |                   |
                 |                                           | Mismatch          | Valid
                 |                                           v                   v
                 v                                    [403 FORBIDDEN]    [Pass to Handler]
           [Pass to Handler]                       (CSRF_TOKEN_INVALID)
```

---

## 2. Cookie Attribute Configuration

### 2.1 Session Cookie (`sentinel_session` / `__Host-sentinel_session`)

| Attribute | Specification | Justification |
| :--- | :--- | :--- |
| **HttpOnly** | `true` (Always) | Prevents JavaScript (`document.cookie`) from accessing session tokens, defeating XSS token theft. |
| **Secure** | `true` (in HTTPS / Production), `false` (in local HTTP) | Ensures credentials are encrypted in transit over TLS and never transmitted in plaintext. |
| **SameSite** | `Strict` | Withholds cookie on all cross-site navigations, preventing cross-site request attacks. |
| **Path** | `/` | Scoped to the entire application domain. |
| **Domain** | Omitted | Prevents child subdomains from accessing or spoofing parent session cookies (RFC 6265bis). |
| **Prefix** | `__Host-` (when HTTPS) | Enforces browser-level pinning: requires Secure=true, Path=/, and no Domain. |

### 2.2 CSRF Cookie (`sentinel_csrf` / `__Host-sentinel_csrf`)

| Attribute | Specification | Justification |
| :--- | :--- | :--- |
| **HttpOnly** | `false` | Must be readable by the frontend application (`document.cookie`) to construct request headers. |
| **Secure** | Matches transport security | Transmitted securely when HTTPS is enabled. |
| **SameSite** | `Strict` | Blocks cookie injection or transmission from untrusted origins. |
| **Path** | `/` | Accessible to all frontend routes. |
| **Domain** | Omitted | Defends against cross-subdomain cookie overwrite attacks. |
| **Prefix** | `__Host-` (when HTTPS) | Enforces RFC 6265bis host-only scoping in HTTPS mode. |

---

## 3. Reverse Proxy & HTTPS Detection Policy

Never rely solely on `r.TLS != nil` to determine whether transport is secure, as enterprise deployments frequently terminate TLS at a fronting reverse proxy.

### 3.1 Security Evaluation Order
1. **Explicit Setting (`SecureCookies: true`):** If configured, cookies are *unconditionally* issued with `Secure: true`.
2. **Direct TLS (`r.TLS != nil`):** If TLS was terminated directly by Go HTTP server, transport is secure.
3. **Forwarded Headers (`X-Forwarded-Proto`):** 
   - Inspected **ONLY IF** the client IP (`r.RemoteAddr`) matches an entry in `TrustedProxies`.
   - If the request originated from an untrusted IP, `X-Forwarded-Proto` is **strictly ignored** to prevent header spoofing attacks.

```go
func (s *Server) isRequestSecure(r *http.Request) bool {
    if s.cookieConfig.SecureCookies {
        return true
    }
    if r.TLS != nil {
        return true
    }
    if s.isRemoteAddrTrustedProxy(r.RemoteAddr) {
        proto := r.Header.Get("X-Forwarded-Proto")
        if strings.EqualFold(proto, "https") {
            return true
        }
    }
    return false
}
```

---

## 4. Double-Submit CSRF & Origin Validation Protocol

For every cookie-authenticated request performing a state change (`POST`, `PUT`, `PATCH`, `DELETE`):

1. **Origin Verification:**
   - The request MUST contain an `Origin` header.
   - If missing: rejected immediately with HTTP 403 `CSRF_MISSING_ORIGIN`.
   - If present but not in the trusted origin list: rejected with HTTP 403 `CSRF_UNTRUSTED_ORIGIN`.

2. **Token Verification:**
   - The request MUST provide a CSRF token in the `X-CSRF-Token` or `X-Sentinel-CSRF` header.
   - The server compares the provided token with the session's bound `CSRFToken` or the CSRF cookie value using `subtle.ConstantTimeCompare`:
   ```go
   if subtle.ConstantTimeCompare([]byte(csrfHeader), []byte(expectedToken)) != 1 {
       writeError(w, r, http.StatusForbidden, "CSRF_TOKEN_INVALID", "Invalid or missing CSRF token")
       return
   }
   ```

3. **Desktop & Machine API Exemption:**
   - Native desktop clients (Wails/Electron) or CLI utilities utilizing `Authorization: Bearer <token>` or `X-Client-Type: desktop` are immune to browser CSRF vectors and are exempt from CSRF token checks.

---

## 5. Configuration & Environment Variables

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `SENTINEL_SECURE_COOKIES` | Boolean (`true`/`false`) | `false` (auto-detected) | Forces `Secure: true` on all issued cookies. |
| `SENTINEL_COOKIE_HOST_PREFIX` | Boolean (`true`/`false`) | `false` | Enables RFC 6265bis `__Host-` cookie prefixes in HTTPS mode. |
| `SENTINEL_TRUSTED_PROXIES` | Comma-delimited strings | `127.0.0.1,::1` | Allowed proxy IPs or CIDR networks (`10.0.0.0/8`, `172.16.0.0/12`). |
| `SENTINEL_CORS_ORIGINS` | Comma-delimited URLs | `http://localhost:5173,...` | Allowed CORS origins for Origin verification. |

---

## 6. Audit & Verification Evidence

The implementation has been thoroughly verified across multiple automated test suites:
- Direct HTTPS mode sets `Secure: true`, `HttpOnly: true`, `SameSite: Strict`, no Domain.
- Development HTTP mode issues plaintext cookies while maintaining `HttpOnly: true` and `SameSite: Strict`.
- Trusted reverse proxy correctly honors `X-Forwarded-Proto: https`.
- Untrusted proxy clients attempting to spoof `X-Forwarded-Proto: https` are ignored.
- Missing and untrusted Origin headers return 403 Forbidden with structured error codes.
- Constant-time comparison prevents timing side-channel attacks on CSRF tokens.
