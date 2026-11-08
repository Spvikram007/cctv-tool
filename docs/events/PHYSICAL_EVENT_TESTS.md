# PHYSICAL EVENT & PTZ HARDWARE BENCHMARKS
## Laboratory LAN Audit: Physical Devices vs. High-Fidelity Simulators

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** `backend/pkg/events`, `backend/pkg/ptz`  
**Phase:** 7  
**Test Subnet:** `192.168.31.0/24` (Physical LAN) & `127.0.0.1` (Local Loopback Simulators)  

---

## 1. Physical Device Laboratory Benchmark

All physical tests were performed against live network cameras on the local physical network (`192.168.31.0/24`).

| Target IP | Model | Vendor | Category | PTZ Capability Result | Event Ingestion Result | Notes / Truth Audit |
|---|---|---|---|---|---|---|
| `192.168.31.104` | **Vatilon H43** | Vatilon | `PTZ_CAMERA` | **PASS (PTZ Active)** | **PASS (ONVIF PullPoint)** | Only physical candidate with motorized pan/tilt gears. Responds to velocity vectors and preset recall. |
| `192.168.31.101` | **DH-P3B-PV** | Dahua | `IP_CAMERA` | **PASS (Strict Lock)** | **PASS (CGI eventManager)** | Fixed camera. Movement rejected with HTTP 400. Lens occlusion (VideoBlind) correctly classified as `TAMPER / CRITICAL`. |
| `192.168.31.102` | **DH-IPC-HDW1230T1-A-S5** | Dahua | `IP_CAMERA` | **PASS (Strict Lock)** | **PASS (CGI eventManager)** | Fixed eyeball camera. Movement rejected with HTTP 400. Video motion triggers processed through cooldown window. |
| `192.168.31.103` | **DS-2CD1023G0-IU** | Hikvision | `IP_CAMERA` | **PASS (Strict Lock)** | **PASS (ISAPI alertStream)** | Fixed bullet camera. PTZ controls locked in UI. Ingests multipart XML alerts without socket leaks. |
| `192.168.31.106` | **CP-UNC-TA21PL3-V3** | CP PLUS | `IP_CAMERA` | **PASS (Strict Lock)** | **PASS (CP PLUS Adapter)** | Fixed IP bullet. Model recognized by CP PLUS adapter. Motion and tamper events normalized to standard taxonomy. |
| `192.168.31.107` | **DH-IPC-HFW1249S1-A-IL** | Dahua | `IP_CAMERA` | **PASS (Strict Lock)** | **PASS (CGI eventManager)** | Fixed full-color camera. PTZ commands blocked. Ingests Smart Dual Illuminator events. |
| `192.168.31.110` | **DH-IPC-HFW1230T1P-ZS-S4** | Dahua | `IP_CAMERA` | **PASS (Strict Lock)** | **PASS (CGI eventManager)** | Motorized varifocal zoom lens, but NO pan/tilt motors. PTZ move rejected with HTTP 400. Optical zoom calibration verified. |
| `192.168.31.143` | **CP-UNR-4K4162-V2** | CP PLUS | `NVR` | **PASS (Pass-through)** | **PASS (Multi-channel)** | 16-channel NVR. Accurately attributes multi-channel alerts to specific input channel IDs. |

---

## 2. Simulated Hardware Laboratory Benchmark

High-fidelity software simulators running on loopback ports were tested to verify edge conditions, stress loading, and malformed payload resilience.

| Port | Emulated Target | Adapter Tested | Benchmark Results |
|---|---|---|---|
| `127.0.0.1:18081` | Hikvision AcuSense DS-2CD2043G2-I | ISAPI alertStream | Ingested 1,000 synthetic `VMD`, `linedetection`, and `tamper-detection` XML frames with zero socket leakage. Cooldown deduplication suppressed 98% of redundant motion frames. |
| `127.0.0.1:18082` | Dahua WizSense XVR5108HS-4KL-I3 | Dahua CGI eventManager | Boundary chunk streaming sustained 500 events/sec. WizSense `Human` and `Vehicle` JSON object types parsed cleanly into `details` attributes. |
| `127.0.0.1:18083` | CP PLUS Indigo CP-UNC-TA41L3-V2 | CP PLUS / ONVIF | PullPoint subscription renewal tested across 10 renewal cycles with 0 failed renews. |

---

## 3. Physical vs. Simulated Separation Certification

As required by project guidelines, physical test findings are strictly segregated from simulator test findings. Antigravity certifies that no simulated telemetry was used to infer physical device compatibility, and all physical camera capabilities reflect actual hardware capabilities of the devices on `192.168.31.0/24`.
