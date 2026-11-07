# CCTV SENTINEL UNIVERSAL
## Analog CCTV Compatibility & Coaxial UTC Signaling

**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  
**Module:** `backend/pkg/recorder`  
**Classification:** Field Engineering Guide

---

## 1. High-Definition Composite Video Standards

Modern surveillance retrofits frequently utilize existing 75-ohm coaxial cabling (RG59 / RG6). Four predominant analog video transmission standards exist:

1. **HDCVI (High Definition Composite Video Interface)**:
   - Developed by Dahua Technology.
   - Modulation: Quadrature Amplitude Modulation (QAM) with separated chrominance and luminance (Y/C).
   - UTC Signaling: FSK (Frequency Shift Keying) telemetry embedded in the vertical blanking interval (VBI).
   - Native audio transmission over the same coaxial conductor (Audio-over-Coax).
2. **HDTVI (High Definition Transport Video Interface)**:
   - Developed by Techpoint / Hikvision (TurboHD).
   - UTC Signaling: Proprietary Hikvision C-protocol in the VBI.
3. **AHD (Analog High Definition)**:
   - Developed by Nextchip. Open standard used by many white-label and OEM manufacturers.
4. **CVBS (Composite Video Blanking and Sync / 960H)**:
   - Legacy analog composite video (NTSC 525 lines / PAL 625 lines, 1.0 Vp-p into 75 ohms).
   - Used for legacy test monitors and antique surveillance cameras.

---

## 2. Coaxial UTC (Up-The-Coax) Technology

Traditional analog PTZ and OSD cameras required running a separate twisted-pair cable (RS-485 Pelco-D / Pelco-P) alongside the coaxial video line to control pan, tilt, zoom, and camera internal setup menus.

Coaxial UTC modulates control commands directly over the center copper conductor of the RG59/RG6 cable during the vertical blanking interval. This enables:
- Bidirectional camera configuration without extra wires.
- In-place switching of the camera output format (e.g. from CVI to TVI or AHD) directly from the recorder's front panel or management software.
- Full camera On-Screen Display (OSD) navigation (exposure, backlight compensation, WDR, 3D-DNR, day/night switching).

---

## 3. Strict Pure IP NVR Guard

> [!CAUTION]
> Pure IP Network Video Recorders (such as the physical CP PLUS CP-UNR-4K4162-V2 on our benchmark test bench) contain zero physical BNC analog inputs.
> Attempting to execute analog format switching or coaxial UTC signaling on a pure IP NVR will be strictly rejected with HTTP status code `400 Bad Request` and error code `ErrAnalogNotSupported`:
> ```
> recorder: analog signal formats are not supported on pure IP NVR units
> ```

---

## 4. Fundamental Truth: Physical Discovery Reality

> [!IMPORTANT]
> Ordinary analog cameras connected via BNC coaxial cables do **NOT** have:
> - Network interface cards (NICs).
> - IP addresses (IPv4 or IPv6).
> - MAC addresses.
> - ONVIF WS-Discovery endpoints.
> - HTTP / RTSP / SSH servers.
> 
> Therefore, ordinary analog cameras can **NEVER** be discovered using IP broadcast, ARP scans, or ONVIF WS-Discovery probes. Only the host DVR/XVR recorder has an Ethernet network interface. CCTV Sentinel Universal communicates with the host DVR/XVR to inspect, configure, and control the attached analog cameras.
