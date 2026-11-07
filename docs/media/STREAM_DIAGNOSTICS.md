# CCTV SENTINEL UNIVERSAL — STREAM DIAGNOSTICS & CAPACITY CALCULATOR
# TECHNICAL SPECIFICATION & ALGORITHMS

**Product**: CCTV SENTINEL UNIVERSAL  
**Module**: Diagnostics & Precision Calculators  
**Package**: `backend/pkg/diagnostics`  
**Distribution**: Commercial CCTV Technician Platform  

---

## 1. Deep RTSP Stream Diagnostics (`RunStreamDiagnostics`)

When a technician experiences video dropouts, stuttering, or blank screens, the Stream Diagnostics tool probes the physical device through an empirical 7-stage test pipeline:

```
[ Stage 1: TCP Socket Probe ]
       |
       +---> Measures socket handshake latency (ms)
       |
[ Stage 2: RTSP OPTIONS Probe ]
       |
       +---> Extracts RFC 2326 Public methods: OPTIONS, DESCRIBE, SETUP, PLAY, etc.
       |
[ Stage 3: RTSP DESCRIBE Probe ]
       |
       +---> Intercepts 401 Unauthorized; extracts realm, nonce, auth algorithm (Digest/Basic)
       |
[ Stage 4: Authentication & SDP Presentation ]
       |
       +---> Calculates MD5 Digest auth header; downloads and parses raw SDP
       |
[ Stage 5: SDP Track Inspection ]
       |
       +---> Video Codec (H.264, H.265), Clock Rate (90 kHz), Resolution, Audio Codec (PCMA/PCMU)
       |
[ Stage 6: TCP Interleaved Transport Negotiation ]
       |
       +---> Sends SETUP with Transport: RTP/AVP/TCP;interleaved=0-1
       +---> Verifies camera accepts reliable TCP transport instead of lossy UDP
       |
[ Stage 7: PLAY & Bitrate Measurement ]
       |
       +---> Sends PLAY; reads RTP packets for 500 ms
       +---> Calculates time-to-first-frame (TTFF) and measured bitrate in kbps
```

### Diagnostics Metric Definitions:
- `connect_latency_ms`: Pure TCP socket round-trip time to port 554.
- `options_methods`: List of standard RTSP methods supported by the camera's firmware.
- `auth_required` / `auth_method`: Authentication challenge verification.
- `sdp_received`: Whether presentation description was successfully retrieved.
- `tcp_transport_supported`: True if the camera permits reliable interleaved streaming over TCP.
- `time_to_first_frame_ms`: Milliseconds between `PLAY` command and arrival of first valid RTP packet.
- `measured_bitrate_kbps`: Empirical throughput calculated from actual payload bytes received:
  $$\text{Bitrate}_{\text{kbps}} = \frac{\text{Bytes Received} \times 8}{\Delta t_{\text{sec}} \times 1000}$$
- `recommendations`: Actionable technician remediation advice (e.g. check MTU, enable TCP interleaved mode, configure sub-stream on high-latency links).

---

## 2. Bandwidth & Capacity Calculator (`CalculateCapacityAndBandwidth`)

### Core Sizing Formulas:

#### A. Storage Capacity:
$$\text{Bitrate}_{\text{bytes/sec}} = \frac{\text{Bitrate}_{\text{kbps}} \times 1000}{8}$$
$$\text{Total Bytes} = \text{Bitrate}_{\text{bytes/sec}} \times (\text{Days} \times \text{Hours/Day} \times 3600) \times N_{\text{cameras}}$$
$$\text{Total Storage (TB)} = \frac{\text{Total Bytes}}{1024^4}$$

#### B. Surveillance Hard Drive Recommendations:
$$\text{HDDs (4TB Disks)} = \left\lceil \frac{\text{Total Storage (TB)}}{3.6} \right\rceil$$
*(Note: 3.6 TB usable capacity accounts for sector formatting overhead on standard 4TB surveillance disks).*

#### C. Aggregate Camera Recording Throughput:
$$\text{Aggregate Recording (Mbps)} = \frac{N_{\text{cameras}} \times \text{MainBitrate}_{\text{kbps}}}{1000}$$

#### D. Live View Viewing Bandwidth:
$$\text{Viewing Bandwidth (Mbps)} = \frac{N_{\text{viewers}} \times \text{SubBitrate}_{\text{kbps}}}{1000}$$

#### E. Switch Uplink Port & 30% Headroom:
$$\text{Total Network (Mbps)} = \text{Aggregate Recording} + \text{Viewing Bandwidth}$$
$$\text{Recommended Headroom (Mbps)} = \text{Total Network} \times 1.30$$

- If $\text{Headroom} \le 80 \text{ Mbps}$: **FastEthernet (100 Mbps)**
- If $80 < \text{Headroom} \le 800 \text{ Mbps}$: **GigabitEthernet (1000 Mbps)**
- If $\text{Headroom} > 800 \text{ Mbps}$: **10-Gigabit SFP+ (10 Gbps)**
