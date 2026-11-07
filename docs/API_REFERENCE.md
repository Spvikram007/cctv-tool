# CCTV SENTINEL UNIVERSAL — REST API Reference
**Version:** v1.0.0-PROD  
**Base URL:** `http://127.0.0.1:8080/api/v1`

## 1. System Endpoints
### `GET /system/status`
Returns agent health, uptime, version, and license quota.
```json
{
  "product": "CCTV SENTINEL UNIVERSAL",
  "owner": "Tech Defence Solutions",
  "tagline": "ONE TOOL. EVERY CCTV. COMPLETE CONTROL.",
  "version": "1.0.0-PROD",
  "uptime_sec": 142,
  "license_tier": "FREE",
  "active_hosts": 3,
  "max_allowed": 8,
  "status": "HEALTHY"
}
```

## 2. Fleet Inventory & Discovery
### `GET /devices`
List all enrolled cameras, XVRs, and recorders.

### `POST /devices`
Manually register or enroll a device.

### `DELETE /devices/:id`
De-register a device from fleet management.

### `POST /devices/discover`
Trigger an asynchronous subnet scan.
```json
{
  "cidr": "192.168.1.0/24",
  "username": "admin",
  "password": "Password123"
}
```

### `GET /devices/discover/progress`
Poll current discovery scan progress, scanned IP count, percentage, and found units.

## 3. Configuration Engine (12 Steps)
### `GET /devices/:id/network`
Fetch active IP address, netmask, gateway, and DHCP status.

### `POST /devices/:id/network/preview`
Generate Step 6 parameter diff and validate proposed settings.
```json
{
  "dhcp": false,
  "ip_address": "192.168.1.150",
  "subnet_mask": "255.255.255.0",
  "gateway": "192.168.1.1"
}
```
**Response:**
```json
{
  "device_id": "hik-127-0-0-1-18081",
  "diff": "IP: 127.0.0.1 -> 192.168.1.150 | DHCP: false -> false",
  "valid": true,
  "validation_errors": [],
  "rollback_key": "rb-hik-127-0-0-1-18081-1794056208118201700"
}
```

### `POST /devices/:id/network/apply`
Apply confirmed settings (Step 8), verify read-back (Step 9), and audit log (Step 11).

### `GET /devices/:id/image` & `POST /devices/:id/image/apply`
Read and adjust brightness, contrast, saturation, sharpness, and WDR levels.

### `GET /devices/:id/snapshot`
Retrieve live sensor snapshot (JPEG/SVG).

## 4. Recorder & Analog XVR
### `GET /devices/:id/channels`
List recorder analog and digital channels.

### `POST /devices/:id/channels/analog-format`
Switch coaxial video format via UTC:
```json
{
  "channel_id": 1,
  "signal_format": "TVI"
}
```
Supported formats: `TVI`, `CVI`, `AHD`, `CVBS`.

## 5. Diagnostics & Calculators
### `GET /diagnostics/run?ip=127.0.0.1&http_port=80&rtsp_port=554`
Execute socket latency, port scan, and RTSP DESCRIBE/OPTIONS tests.

### `GET /diagnostics/subnet?cidr=192.168.1.0/24`
Returns network address, broadcast address, netmask, wildcard mask, and usable host count.

### `GET /diagnostics/hdd?cameras=16&resolution=4MP&codec=H.265&days=30`
Returns raw storage in TB and recommended 4TB surveillance hard disk drive count.

### `GET /diagnostics/bandwidth?cameras=16&main_kbps=4096&sub_kbps=1024`
Returns total aggregate throughput in Mbps and recommended network switch port capacity.

## 6. Commercial Licensing & Audit
### `GET /licensing`
Get current license tier, expiry, and hardware machine hash.

### `POST /licensing/activate`
Validate cryptographically signed offline activation key.

### `POST /licensing/gst-invoice`
Compute Indian GST tax breakdown (CGST 9% + SGST 9% or IGST 18%, HSN 998313).

### `GET /audit/logs`
Retrieve SHA-256 chained transaction logs with scrubbed secrets.
