# Hikvision Authenticated ISAPI Protocol Architecture

## 1. Overview & Architecture

The **Hikvision Intelligent Security API (ISAPI)** is an XML/REST protocol deployed across Hikvision IP cameras, DVRs, NVRs, and hybrid video recorders. 

In CCTV Sentinel Universal, the Hikvision integration operates over a dedicated HTTP client featuring automatic RFC 2617 HTTP Digest authentication, non-replayable body caching, and strict credential isolation.

```
[Hikvision Hardware] <--- 1. GET /ISAPI/System/deviceInfo (No Auth) --- [Sentinel ISAPI Client]
                     ---> 2. Returns HTTP 401 Unauthorized         ---> Parses WWW-Authenticate: Digest
                          realm="IP Camera(E9603)", nonce="..."         Calculates HA1, HA2, Response
                     <--- 3. Re-dispatches with Authorization: Digest --
                     ---> 4. Returns 200 OK + Device XML            ---> Normalizes into Device Entity
```

---

## 2. HTTP Digest Authentication (RFC 2617 / RFC 7616)

Modern Hikvision cameras reject Basic authentication by default and strictly require MD5 or SHA-256 Digest authentication.

### 2.1 Digest Computation

$$\text{HA}_1 = \text{MD5}(\text{username} : \text{realm} : \text{password})$$

$$\text{HA}_2 = \text{MD5}(\text{HTTP Method} : \text{Request URI})$$

$$\text{Response} = \text{MD5}(\text{HA}_1 : \text{nonce} : \text{nc} : \text{cnonce} : \text{qop} : \text{HA}_2)$$

Where:
- $\text{nc}$: 8-digit hexadecimal nonce counter (e.g., `00000001`).
- $\text{cnonce}$: 16-hex client nonce generated using CSPRNG.
- $\text{qop}$: `"auth"`.

### 2.2 Physical Device Verification

Empirically captured from physical Hikvision IP Camera `DS-2CD1023G0-IU` (`192.168.31.103:80`):

```http
HTTP/1.1 401 Unauthorized
Server: webserver
WWW-Authenticate: Digest qop="auth", realm="IP Camera(E9603)", nonce="4e7a646c5a574e694e6a6336596a59324d4759355954633d", stale="FALSE"
```

The `httpauth` package parses this challenge, calculates the response hash, and successfully signs the request without exposing the password in any HTTP logs or audit trails.

---

## 3. Implemented Endpoints & Schema

| Endpoint | Method | Payload / Response | Function |
| :--- | :--- | :--- | :--- |
| `/ISAPI/System/deviceInfo` | `GET` | XML `<DeviceInfo>` | Queries hardware model, firmware version, and serial number. |
| `/ISAPI/System/network/interfaces/1/ipAddress` | `GET` / `PUT` | XML `<IPAddress>` | Reads and configures static IPv4 address, subnet mask, default gateway, and DHCP mode. |
| `/ISAPI/Image/channels/1/color` | `GET` / `PUT` | XML `<Color>` | Reads and adjusts brightness, contrast, saturation, and sharpness levels (0–100). |
| `/ISAPI/PTZCtrl/channels/1/continuous` | `PUT` | XML `<PTZData>` | Controls continuous Pan/Tilt/Zoom velocity. |
| `/ISAPI/Streaming/channels/101/picture` | `GET` | JPEG binary | Captures live camera frame snapshots. |
| `/ISAPI/System/Video/inputs/channels/{id}/videoFormat` | `PUT` | XML `<VideoFormat>` | Switches analog video signal format (TVI / CVI / AHD / CVBS) on TurboHD XVRs. |

---

## 4. Safety & Security Boundary

- **Zero Secret Leakage**: HTTP requests containing `Authorization: Digest ...` have sensitive hashes stripped before logging to `AuditLogEntry`.
- **Read-Back Verification**: After `PUT /ISAPI/System/network/interfaces/1/ipAddress`, the system immediately executes `GET` to read back the hardware state and verify that the target IP and subnet were applied by the camera OS.

