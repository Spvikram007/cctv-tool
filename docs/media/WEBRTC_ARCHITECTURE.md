# CCTV SENTINEL UNIVERSAL — STREAMING ARCHITECTURE & WEBRTC ROADMAP
# EVALUATION & FUTURE ARCHITECTURE

**Product**: CCTV SENTINEL UNIVERSAL  
**Module**: Streaming Strategy & Transport Architecture  
**Distribution**: Commercial CCTV Technician Platform  

---

## 1. Transport Strategy Evaluation

| Protocol | Latency | Browser Support | CPU Overhead | NAT / Firewall Traversal | Multi-Camera Grid | Verdict in CCTV Sentinel |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **RTSP Direct** | < 100 ms | None (Requires plugin/desktop app) | Zero | Good (TCP interleaved) | Excellent | Core internal protocol |
| **MJPEG / HTTP Multipart** | ~150-250 ms | 100% Native (`<img>` tag) | Moderate | Excellent (HTTP Port 8080) | Fluid (1, 4, 9, 16 grids) | **Active Production Pipeline** |
| **HLS (HTTP Live Streaming)** | 2-6 sec | Good (hls.js / Native Safari) | Moderate | Excellent | High buffer latency | Unsuitable for PTZ / Live |
| **WebRTC (Peer-to-Peer)** | < 100 ms | 100% Modern Browsers | Low (Direct Passthrough) | Requires ICE / STUN / TURN | Heavy on WebRTC peer limit | **Future Phase Roadmap** |

---

## 2. Why MJPEG Multipart Stream Is Selected for Phase 4

1. **Zero Browser Friction**:
   Works out of the box on Chromium, Edge, Firefox, Safari, Android Chrome, and WebView without requiring WebAssembly compilation or client-side video decoding libraries.
2. **Deterministic Credential Isolation**:
   Browser clients connect to `http://127.0.0.1:8080/api/v1/streams/{token}/mjpeg` using scoped, short-lived tokens. The camera's actual IP, port, RTSP credentials, and password hashes never touch the client application.
3. **Multi-Camera Matrix Efficiency**:
   Browsers can easily render 16 simultaneous `<img>` feeds without hitting browser WebRTC peer connection limits (typically 8-10 hardware decoders on client machines).

---

## 3. WebRTC Migration Roadmap (Phase 6+)

For future ultra-low-latency bidirectional audio/video intercom (2-way audio cameras like Dahua `DH-P3B-PV`):

```
+------------------+         RTSP/RTP         +----------------------+         WebRTC (Pion)        +------------------+
| Physical Camera  | -----------------------> | Sentinel Media Core  | =============================> | Browser / Mobile |
| (Dahua / Hik / CP| <----------------------- | (Go Pion WebRTC Hub) | <============================= | (Microphone Mic) |
+------------------+      Backchannel Audio   +----------------------+         Two-Way Opus/PCMU      +------------------+
```

### Components for WebRTC Upgrade:
- **Pion WebRTC Library**: Pure Go implementation of WebSockets, STUN, DTLS, and SRTP.
- **RTP Passthrough**: Direct forward of camera H.264 NAL units without transcoding to MJPEG when client supports native hardware H.264 decoding.
- **Signaling Server**: WebSocket signaling channel (`/api/v1/webrtc/signal`) exchanging SDP offer/answer.
- **Audio Backchannel (Talk-Back)**: Forwarding microphone Opus/G.711 PCMA/PCMU audio back to physical camera via ONVIF 2-way audio or Dahua CGI backchannel.
