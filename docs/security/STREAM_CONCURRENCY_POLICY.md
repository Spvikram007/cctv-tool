# Stream Concurrency & Transcoder Process Isolation Policy
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** Live Media Hub & RTSP Pipeline (`backend/pkg/media/streamhub/hub.go`)  
**Release:** Phase 6.1 Stabilization  
**Author:** Tech Defence Solutions  

---

## 1. Process Concurrency Limits & Resource Quotas

Live video transcoders (FFmpeg RTSP-to-MJPEG workers) consume CPU, socket handles, and network bandwidth. To protect server stability and avoid Denial of Service from excessive concurrent camera streams, strict limits are enforced at the Media Hub layer:

| Resource Metric | Configured Limit | Action on Threshold Exceeded |
| :--- | :--- | :--- |
| **Fleet-Wide Active Transcoders** | **16 concurrent streams** | Rejects new stream requests with `429 Too Many Requests` (`ERR_MAX_CONCURRENT_TRANSCODERS_REACHED`) |
| **Per-Device Active Streams** | **2 concurrent streams** | Rejects excess streams per camera with `409 Conflict` (`ERR_DEVICE_CONCURRENT_STREAM_LIMIT`) |
| **Stream Session Lifetime (TTL)** | Configurable (Default: **1 Hour**) | Active transcoder terminated and token revoked |
| **Idle Stream Auto-Termination** | **30 seconds** with 0 active HTTP viewers | Subprocess automatically terminated via context cancellation |

---

## 2. Process Tree Management & Zombie Prevention

### 2.1 Context-Bound Execution
All FFmpeg transcoding processes are spawned strictly using `exec.CommandContext(ctx, ...)`:
- When a client disconnects, `ctx.Done()` triggers.
- The Go runtime delivers `SIGKILL` (or `TerminateProcess` on Windows) to terminate the subprocess.
- No zombie processes or detached transcoders remain running in the background.

### 2.2 Transcoder Binary Security
- Prior reliance on third-party host installations (e.g., Wondershare Recoverit) has been removed.
- Transcoder binary path is resolved via validated system paths (`PATH`) or an explicitly configured directory (`CCTV_SENTINEL_FFMPEG_PATH`).
- Relative or arbitrary path injection is prevented.

---

## 3. Stderr Capture & Credential Leakage Prevention

FFmpeg logs device connection URLs, RTSP authentication headers, and network packets to `stderr`. To prevent leaking device credentials to application logs or error payloads:
1. **Bounded Buffer:** `stderr` capture is strictly capped at **32 KiB** to prevent unbounded memory allocation on noisy streams.
2. **Secret Sanitization:** All lines captured from `stderr` are processed through `audit.RedactSecrets()` before logging:
   - Plaintext passwords in RTSP URIs (`rtsp://admin:pass@ip/`) $\to$ `rtsp://admin:***REDACTED***@ip/`
   - Authorization headers and Digest nonces are sanitized.
   - Stack traces and OS file paths are stripped.

---

## 4. SSRF Validation for Stream Targets

Before any RTSP socket is opened or FFmpeg process is spawned:
- The destination IP of the target camera is checked against the SSRF blocking engine (`security.IsBlockedIP`).
- Connections to loopback (`127.0.0.1`), metadata services (`169.254.169.254`), and non-routable addresses are blocked with `ERR_SSRF_BLOCKED`.

