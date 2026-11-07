# CCTV SENTINEL UNIVERSAL — RTSP STREAM ENGINE SPECIFICATION
# RFC 2326, RFC 3550 & RFC 4566 ARCHITECTURE

**Product**: CCTV SENTINEL UNIVERSAL  
**Module**: Media Stream Hub / RTSP Client Engine  
**Package**: `backend/pkg/media/rtsp` & `backend/pkg/media/streamhub`  
**Distribution**: Commercial CCTV Technician Platform  

---

## 1. Architectural Overview

The RTSP Stream Engine is designed specifically for demanding enterprise surveillance networks where embedded IP cameras, DVRs, and NVRs have constrained connection limits. Opening multiple independent RTSP sessions directly to an IP camera frequently crashes the camera's internal SoC.

To solve this, CCTV Sentinel Universal implements a **Multiplexed Interleaved RTSP Client & Stream Hub**:

```
+-------------------+       RTSP/TCP (RFC 2326)       +-----------------------+
|  Physical Camera  | <=============================> |   CCTV Sentinel Hub   |
| (Hik / DH / CP)   |        Interleaved RTP ($)      | (Multiplexed Session) |
+-------------------+                                 +-----------------------+
                                                                  |
                       +------------------------------------------+------------------------------------------+
                       |                                          |                                          |
                       v                                          v                                          v
             [ Browser Viewer 1 ]                       [ Browser Viewer 2 ]                       [ Diagnostics Worker ]
          GET /streams/{token}/mjpeg                 GET /streams/{token}/mjpeg                   Real-time Bitrate & FPS
```

---

## 2. RTSP Client State Machine (RFC 2326)

The client implements the complete RTSP 1.0 state lifecycle:

1. **OPTIONS**:
   - Queries server public methods.
   - Extracts `Public:` header (e.g. `OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN`).
   - Identifies server implementations (e.g. Hikvision RTSP Server, Dahua Streaming Server, Vatilon HTMS).

2. **DESCRIBE**:
   - Requests presentation metadata with `Accept: application/sdp`.
   - Handles `401 Unauthorized` challenges:
     - Parses `WWW-Authenticate: Digest realm="...", nonce="...", qop="auth"`.
     - Generates MD5 response:
       $$\text{HA}_1 = \text{MD5}(\text{username} : \text{realm} : \text{password})$$
       $$\text{HA}_2 = \text{MD5}(\text{"DESCRIBE"} : \text{uri})$$
       $$\text{Response} = \text{MD5}(\text{HA}_1 : \text{nonce} : \text{HA}_2)$$
     - Transparently re-executes query with `Authorization: Digest ...`.

3. **SETUP**:
   - Negotiates transport protocol with camera.
   - Requests TCP interleaved framing:
     `Transport: RTP/AVP/TCP;unicast;interleaved=0-1`
   - Receives assigned camera `Session:` ID and socket channels.

4. **PLAY**:
   - Dispatches `Range: npt=0.000-` to start streaming immediately.
   - Handles `454 Session Not Found` and reconnects if stale.

5. **PAUSE & TEARDOWN**:
   - Cleanly closes the session with `TEARDOWN` to free camera memory and DSP channels.

---

## 3. TCP Interleaved RTP Framing (RFC 3550)

In corporate enterprise networks, UDP multicast and ephemeral UDP ports are blocked by firewalls. The Sentinel engine uses RTSP TCP interleaved mode:

```
+--------+------------+---------------+--------------------------------+
| Magic  | Channel ID | Length (16b)  |          RTP Payload           |
|  '$'   |  (1 byte)  |    (2 byte)   |      (H.264 / H.265 / MJPEG)   |
+--------+------------+---------------+--------------------------------+
```

- Magic Byte: `0x24` (`$`)
- Channel 0: Video RTP Data
- Channel 1: Video RTCP Control
- Sequence Number Validation: Tracks RTP packet sequence (0-65535) to detect dropouts and jitter.

---

## 4. Session Description Protocol (SDP) Parser (RFC 4566)

The custom SDP parser extracts:
- Media Track Type: `video` or `audio`
- Codec Identification: `H264`, `H265`, `HEVC`, `JPEG`, `PCMA`, `PCMU`
- RTP Payload Type: Static (e.g. 0 for PCMU, 8 for PCMA) or Dynamic (96-127)
- Clock Rate: `90000 Hz` for video, `8000 Hz` for surveillance audio
- Control URI: `control:trackID=0` or `control:rtsp://...`
- Dimensions and profile parameters from `a=fmtp:` attributes (e.g., `sprop-parameter-sets`).

---

## 5. Security & Credential Isolation

- **Scoped Stream Tokens**: Viewers receive ephemeral stream access tokens (`stk-...`, default TTL 1 hour).
- **Zero URL Credentials**: Frontend clients never see RTSP URLs, IP camera passwords, or Digest nonces.
- **Auto-Shutdown**: When viewer count for an active camera reaches 0, a 20-second grace timer triggers and cleanly sends `TEARDOWN` to avoid exhausting camera resources.
