# CCTV SENTINEL UNIVERSAL — Architecture Documentation
**Owner:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.

## 1. System Overview
CCTV Sentinel Universal is a commercial-grade, multi-platform CCTV technician, discovery, configuration, and management system. It provides an offline-first architecture for field technicians and enterprise dealers to manage multi-vendor camera fleets (Hikvision, Dahua, CP PLUS, Uniview, Axis, and Generic ONVIF) through standardized abstraction layers.

```
                           +-------------------------------------+
                           |    Web Portal & Tauri 2 Desktop     |
                           |     (React 18, Vite, Tailwind)      |
                           +-------------------------------------+
                                              |
                                              | REST API / WebSocket
                                              v
+---------------------------------------------------------------------------------------+
|                                CCTV SENTINEL CORE DAEMON                              |
|                                                                                       |
|  +--------------------+  +--------------------+  +---------------------------------+  |
|  |  REST API Gateway  |  |   Discovery Engine |  |  12-Step Configuration Engine   |  |
|  |  (/api/v1/...)     |  |  (ONVIF/CIDR/SSDP) |  |  (Preview, Diff, Apply, Verify) |  |
|  +--------------------+  +--------------------+  +---------------------------------+  |
|                                                                                       |
|  +---------------------------------------------------------------------------------+  |
|  |                        Universal Protocol Adapter Registry                      |  |
|  |  +------------------+  +------------------+  +---------------+  +------------+  |  |
|  |  | Hikvision ISAPI  |  | Dahua CGI/NetSDK |  | CP PLUS SDK   |  | ONVIF S/T  |  |  |
|  |  +------------------+  +------------------+  +---------------+  +------------+  |  |
|  +---------------------------------------------------------------------------------+  |
|                                                                                       |
|  +--------------------+  +--------------------+  +---------------------------------+  |
|  |  Audit Log Ledger  |  |  Offline Database  |  |    Multi-Device HW Simulator    |  |
|  |  (SHA-256 Chained) |  |  (JSON / SQLite)   |  |    (ISAPI, CGI, SOAP, RTSP)     |  |
|  +--------------------+  +--------------------+  +---------------------------------+  |
+---------------------------------------------------------------------------------------+
```

## 2. Universal Protocol Adapters
All vendor-specific protocols implement the unified `ProtocolAdapter` interface:
- **`ONVIFAdapter`**: WS-Discovery (UDP 3702), DeviceManagement (SOAP GetDeviceInformation, GetNetworkInterfaces, SetNetworkInterfaces), Media Service (Profiles, RTSP URIs), and PTZ Service (ContinuousMove, Stop, Presets).
- **`HikvisionAdapter`**: RESTful ISAPI XML endpoints (`/ISAPI/System/deviceInfo`, `/ISAPI/System/network/interfaces/1/ipAddress`, `/ISAPI/Image/channels/1/color`, `/ISAPI/PTZCtrl/channels/1/continuous`).
- **`DahuaAdapter`**: HTTP CGI key-value endpoints (`/cgi-bin/magicBox.cgi`, `/cgi-bin/configManager.cgi?action=getConfig&name=Network`, `VideoColor`, `CoaxialSensor`, `/cgi-bin/ptz.cgi`).
- **`CPPlusAdapter`**: Dedicated Cosmic & Indigo series adapter supporting auto-detection and 5-in-1 hybrid XVR control.

## 3. The 12-Step Configuration Engine
Guarantees zero device bricking and complete auditability:
1. **Authenticate**: Validate technician credentials against device endpoint.
2. **Read Device Info**: Query model, serial number, firmware version.
3. **Detect Capabilities**: Identify supported interfaces (PTZ, UTC, WDR, H.265).
4. **Read Current Config**: Snapshot existing network/image/encoding parameters.
5. **Validate Proposed Config**: Verify IPv4 format, netmask validity, gateway reachability, and range bounds.
6. **Display Preview**: Render a structured parameter diff with validation status.
7. **Obtain Confirmation**: User or batch job confirmation required.
8. **Apply Changes**: Push parameters via matching vendor protocol adapter.
9. **Read Back Settings**: Re-query hardware socket to verify applied state.
10. **Report Status**: Return verified execution response.
11. **Save Audit Logs**: Record tamper-evident SHA-256 log entry with scrubbed secrets.
12. **Support Rollback**: Provide pre-change rollback token to instantly restore previous state.

## 4. Hardware Simulation Layer
Included in `backend/pkg/simulator/simulator.go` to fulfill Section 18 acceptance criteria:
- Simulates real TCP sockets and HTTP/SOAP servers for Hikvision (port 18081), Dahua (port 18082), Generic ONVIF (port 18083), and Mock RTSP (port 18554).
- Enables 100% genuine automated unit and end-to-end integration test execution without physical camera hardware.
