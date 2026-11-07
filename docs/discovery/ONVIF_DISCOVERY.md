# CCTV SENTINEL UNIVERSAL
## ONVIF WS-Discovery Protocol Specification & Implementation Guide

```
Product:   CCTV SENTINEL UNIVERSAL
Owner:     Tech Defence Solutions
Module:    Universal Discovery Engine — ONVIF WS-Discovery
Status:    Implemented & Certified in Development Build (Phase 2)
```

---

### 1. Executive Summary

CCTV Sentinel Universal implements genuine multicast **Web Services Dynamic Discovery (WS-Discovery v1.1)** conforming to the ONVIF Core Specification. The discovery engine transmits standards-compliant SOAP `Probe` envelopes via UDP multicast and processes both unicast and multicast `ProbeMatches` replies to detect and identify ONVIF-compliant Network Video Transmitters (NVT) and Network Video Storage (NVS) devices without sending unauthorized authentication challenges.

---

### 2. Protocol Architecture & Network Topology

```
+-------------------------------------------------------------------------+
|                    CCTV SENTINEL DISCOVERY ENGINE                      |
|                                                                         |
|  [Interface Selector] ---> [UDP Socket Pool] ---> [SOAP Envelope Builder]
+-------------------------------------------------------------------------+
                                 |
           UDP Multicast Probe (239.255.255.250:3702)
                                 v
     +-------------------------------------------------------+
     |                  Target LAN Broadcast                |
     +-------------------------------------------------------+
         |                                |               |
         v                                v               v
+------------------+            +------------------+    +------------------+
|  Hikvision Camera|            |   Dahua NVR/XVR  |    |  CP PLUS Camera  |
|  (Port 80/8000)  |            |  (Port 80/37777) |    |  (Port 80/554)   |
+------------------+            +------------------+    +------------------+
         |                                |               |
         +--------------------------------+---------------+
                                 |
              Unicast SOAP ProbeMatches (Ephemeral UDP)
                                 v
+-------------------------------------------------------------------------+
|  [Robust XML Parser] ---> [Scope Analyzer] ---> [Fingerprint Assigner]  |
+-------------------------------------------------------------------------+
```

---

### 3. WS-Discovery Probe Message Specification

Probes are formatted using standard WS-Addressing and WS-Discovery schemas:

```xml
<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"
               xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"
               xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
               xmlns:dn="http://www.onvif.org/ver10/network/wsdl"
               xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <soap:Header>
    <wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action>
    <wsa:MessageID>urn:uuid:48494b56-3030-3030-3030-bc5451a2b3c4</wsa:MessageID>
    <wsa:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</wsa:To>
  </soap:Header>
  <soap:Body>
    <d:Probe>
      <d:Types>dn:NetworkVideoTransmitter tds:Device</d:Types>
    </d:Probe>
  </soap:Body>
</soap:Envelope>
```

#### Key Fields:
- **Multicast Destination**: `239.255.255.250:3702` (IPv4)
- **Action**: `http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe`
- **Types**: `dn:NetworkVideoTransmitter` (Cameras and Encoders) and `tds:Device` (Generic ONVIF units)
- **MessageID**: Unique RFC 4122 v4 UUID generated per probe session to correlate replies.

---

### 4. SOAP `ProbeMatches` Extraction & Semantics

Incoming responses are decoded via Go's streaming token XML parser (`xml.NewDecoder`) designed to be resilient against non-conforming camera firmware:

#### 1. EndpointReference (`wsa:Address`)
Extracts the unique hardware URN (e.g. `urn:uuid:48494b56-3030-3030-3030-bc5451a2b3c4`). Used for global device deduplication.

#### 2. Service Endpoints (`d:XAddrs`)
Space-delimited list of HTTP/HTTPS URLs pointing to the device service endpoint:
- E.g.: `http://192.168.1.64:80/onvif/device_service`
- Parsed to extract target IP (`192.168.1.64`) and ONVIF listening port (`80`).

#### 3. Scope Semantics (`d:Scopes`)
Space-delimited URI strings indicating hardware profile, manufacturer, and model:
- `onvif://www.onvif.org/hardware/<model>`: Extracted directly into device model (e.g., `DS-2CD2042WD-I`).
- `onvif://www.onvif.org/name/<name>`: Device name or vendor identification.
- `onvif://www.onvif.org/type/ptz`: Triggers classification as `PTZ_CAMERA`.
- `onvif://www.onvif.org/type/video_encoder`: Triggers classification as `IP_CAMERA` or `ENCODER`.
- `onvif://www.onvif.org/type/network_video_storage`: Triggers classification as `NVR`.

---

### 5. Multi-NIC & Windows Firewall Resilience

1. **Multi-Interface Support**:
   - Technicians frequently operate laptops with both an active Wi-Fi adapter (office network) and an Ethernet NIC connected directly to the CCTV PoE switch.
   - Sentinel Universal enumerates local interfaces via `GetAvailableInterfaces()` and allows the technician to select the physical interface.
   - When no specific interface is selected, it iterates across all multicast-capable UP interfaces.

2. **Windows Firewall Considerations**:
   - In standard Windows installations, binding to port `3702` on all interfaces can cause socket collision with the native Windows SSDP / Discovery service.
   - Sentinel Universal binds an ephemeral UDP port (`:0`) on the selected interface IP and transmits to multicast group `239.255.255.250:3702`. Responding cameras transmit unicast `ProbeMatches` back to the ephemeral port, completely bypassing port collision.

3. **Socket Lifecycle & Goroutine Leak Prevention**:
   - All network sockets are closed via `defer conn.Close()` upon context cancellation or timeout.
   - Context timeouts default to 3 seconds, ensuring no hanging routines.

