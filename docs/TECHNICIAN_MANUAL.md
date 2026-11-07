# CCTV SENTINEL UNIVERSAL — Field Technician Manual
**Owner:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.

## 1. Quick Start
1. Double-click `sentinel-server.exe` on Windows (or run `./sentinel-server` on Linux/macOS).
2. Open your web browser to `http://127.0.0.1:8080`.
3. The technician dashboard will display current status, fleet inventory, and live diagnostic tools.

## 2. Performing Subnet Discovery
1. Click **Auto Discovery** on the sidebar.
2. Enter the target customer subnet in CIDR notation (e.g. `192.168.1.0/24`). For offline bench testing with the integrated simulator, use `127.0.0.1/32`.
3. Enter the default camera credentials (e.g. `admin` / `Password`).
4. Click **Start Discovery**.
5. Discovered hardware appears in real time with identified manufacturer badges (Hikvision, Dahua, CP PLUS, Axis, Uniview). Click **Add to Fleet** to enroll.

## 3. Applying 12-Step Configuration
1. Navigate to **Universal Config**.
2. Select your target camera from the dropdown.
3. Modify IP Addressing (Static/DHCP) or Image Parameters (Brightness, Contrast, WDR).
4. Click **Stage & Preview Changes (Step 5 & 6)**.
5. Review the parameter diff in the confirmation modal. Ensure no validation errors are flagged.
6. Click **Confirm & Apply to Hardware (Step 7 & 8)**.
7. The platform pushes settings, verifies read-back from the camera socket, logs the action, and equips you with an instant **Step 12 Rollback** button.

## 4. Managing XVR Coaxial Formats (UTC)
1. Navigate to **Recorder & XVR**.
2. Select the XVR unit.
3. On any analog channel, click **TVI**, **CVI**, **AHD**, or **CVBS**.
4. The adapter will send the corresponding UTC coaxial control command to switch the recorder channel and analog camera sensor mode dynamically.

## 5. Site Handover & Commissioning Reports
1. Navigate to **Audit & Reports**.
2. Click **Export Commissioning Report**.
3. A tamper-evident, client-ready HTML commissioning report will open, summarizing enrolled hardware, firmware versions, IP allocations, and verified audit logs for signature handover.
