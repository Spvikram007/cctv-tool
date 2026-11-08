# HIKVISION ISAPI EVENT STREAMING
## alertStream Multipart/Mixed Parsing, AcuSense Classification & Event Taxonomy

**Company:** Tech Defence Solutions  
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** `backend/pkg/events`  
**Phase:** 7  

---

## 1. Hikvision ISAPI AlertStream Architecture

Hikvision network cameras, DVRs, and NVRs publish real-time security events via an HTTP continuous multipart stream:
- **Endpoint:** `GET /ISAPI/Event/notification/alertStream`
- **Content-Type:** `multipart/mixed; boundary=boundary`
- **Protocol:** HTTP 1.1 / Digest or Basic Authentication

When established, the camera holds the HTTP connection open indefinitely, emitting XML boundary chunks whenever a sensor, video analytics, or hardware event occurs.

---

## 2. Event XML Payload Schema

```xml
--boundary
Content-Type: application/xml; charset="UTF-8"
Content-Length: 720

<EventNotificationAlert version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
  <ipAddress>192.168.31.103</ipAddress>
  <portNo>80</portNo>
  <protocol>HTTP</protocol>
  <macAddress>c0:51:7e:be:be:a6</macAddress>
  <channelID>1</channelID>
  <dateTime>2026-11-08T07:45:12+05:30</dateTime>
  <activePostCount>1</activePostCount>
  <eventType>tamper-detection</eventType>
  <eventState>active</eventState>
  <eventDescription>Tamper detection started</eventDescription>
  <channelName>Front Gate Camera</channelName>
</EventNotificationAlert>
--boundary
```

---

## 3. Hikvision Event Type Mapping

The ISAPI event parser maps Hikvision XML event types to normalized CCTV SENTINEL event categories:

| Hikvision `eventType` | Event Description | Normalized Category | Default Severity |
|---|---|---|---|
| `VMD` | Video Motion Detection | `MOTION` | `MINOR` |
| `tamper-detection` | Camera lens covered/defocused/spray-painted | `TAMPER` | `CRITICAL` |
| `shelteralarm` | Video Blind / Occlusion | `TAMPER` | `CRITICAL` |
| `linedetection` | Tripwire / Virtual Line Crossing | `LINE_CROSSING` | `MAJOR` |
| `fielddetection` | Intrusion into defined zone | `INTRUSION` | `MAJOR` |
| `regionEntrance` | Target entered defined perimeter | `INTRUSION` | `MAJOR` |
| `regionExiting` | Target exited defined perimeter | `INTRUSION` | `MAJOR` |
| `faceDetection` | Facial feature detected | `AI_ANALYTICS` | `INFO` |
| `IO` | Physical alarm input sensor triggered | `IO_ALARM` | `INFO` |
| `videoloss` | Video signal lost / disconnected | `DEVICE_FAULT` | `CRITICAL` |
| `diskFull` / `diskError` | HDD storage fault or capacity exhausted | `DEVICE_FAULT` | `CRITICAL` |

---

## 4. AcuSense Target Classification

On Hikvision AcuSense cameras (e.g. `DS-2CD2043G2-I`), the parser inspects `<DetectionTarget>` and `<targetType>` tags:
- `human` $\to$ Details: `target_type: "HUMAN"`, triggers human-specific rule evaluation.
- `vehicle` $\to$ Details: `target_type: "VEHICLE"`, triggers vehicle-specific rule evaluation.
- Leaves/Shadows/Weather $\to$ Suppressed by the camera's deep-learning classifier before reaching the alert stream.
