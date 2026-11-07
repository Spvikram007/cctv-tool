# CCTV SENTINEL UNIVERSAL
### Enterprise CCTV Technician Platform
**Owner:** Tech Defence Solutions  
**Tagline:** *ONE TOOL. EVERY CCTV. COMPLETE CONTROL.*

---

## Highlights & Capabilities
- **Universal Protocol Adapters**: Full native support for **Hikvision ISAPI**, **Dahua CGI & NetSDK**, **CP PLUS Cosmic & Indigo series**, **Standard ONVIF Profile S/T/G**, and **RTSP streaming**.
- **12-Step Universal Configuration Engine**: Read, validate, preview diff, obtain confirmation, apply, read back, verify, and rollback with tamper-evident audit logging.
- **Integrated CCTV Hardware Simulator**: Simulates real network sockets, HTTP endpoints, XML ISAPI responses, Dahua key-value CGI responses, SOAP ONVIF envelopes, and RTSP stream sessions for 100% deterministic testing and verification.
- **XVR & Analog Format Manager**: Coaxial UTC controls to switch channels between TVI, CVI, AHD, CVBS, and IP modes.
- **Robotic PTZ Controller**: 8-way directional pad, continuous zoom tele/wide, speed throttling, and preset memory slots.
- **Engineering Calculators**: Subnet / CIDR calculator, HDD surveillance storage calculator, and network switch throughput calculator.
- **Commercial Licensing System**: Free Community edition, Technician Pro, and Dealer Enterprise with machine fingerprinting, offline cryptographically signed key validation, and GST invoicing (HSN 998313).
- **Tamper-Evident Audit Ledger**: SHA-256 chained transaction logs with automated credential and token redaction.

---

## Architecture
```
cctv-sentinel/
├── backend/                  # High-performance Go core & local agent daemon
│   ├── cmd/sentinel-server/  # Main entrypoint, HTTP REST API & static web host
│   ├── pkg/adapters/         # Protocol adapters (Hikvision, Dahua, CP PLUS, ONVIF)
│   ├── pkg/api/              # RESTful API handlers (/api/v1/...) & OpenAPI schema
│   ├── pkg/audit/            # SHA-256 chained audit logger with secret scrubbing
│   ├── pkg/configengine/     # 12-step configuration engine (preview, diff, rollback)
│   ├── pkg/database/         # JSON/SQLite offline storage & compatibility matrix
│   ├── pkg/diagnostics/      # Ping, RTSP tester, subnet/HDD/bandwidth calculators
│   ├── pkg/discovery/        # ONVIF WS-Discovery, MAC OUI lookup, subnet scanner
│   ├── pkg/licensing/        # Hardware hash, offline activation & GST invoicing
│   ├── pkg/models/           # Core domain data models
│   └── pkg/simulator/        # Multi-vendor CCTV & XVR loopback hardware simulator
├── frontend/                 # React 18 + TypeScript + Vite + Tailwind CSS UI
│   ├── src/components/       # Navbar, Sidebar, Theme provider
│   ├── src/pages/            # Dashboard, Discovery, Devices, LiveView, Config,
│   │                         # Recorder, PTZ, Diagnostics, Calculators, Licensing,
│   │                         # Compatibility, AuditLogs, BatchConfig
│   └── dist/                 # Production-optimized web bundle served by daemon
├── docs/                     # Architecture, API Reference, Technician Manual
├── sentinel-server.exe       # Standalone compiled zero-dependency Windows binary
└── sentinel_store.json       # Persistent offline database store
```

---

## Running the Platform
### Standalone Binary (Windows)
Double-click `sentinel-server.exe` or execute from terminal:
```powershell
.\sentinel-server.exe
```
Then navigate to:
```
http://127.0.0.1:8080
```

### Running Backend Tests
```powershell
cd backend
..\.tools\go\bin\go.exe test -v ./...
```

### Building the Frontend
```powershell
cd frontend
npm run build
```

---
**Copyright © 2026 Tech Defence Solutions. All Rights Reserved.**
