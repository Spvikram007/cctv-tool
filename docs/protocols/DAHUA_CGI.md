# Dahua Authenticated CGI Protocol Architecture

## 1. Overview & Architecture

**Dahua Technology** IP cameras, XVRs, and NVRs provide an embedded HTTP/CGI control interface alongside Dahua NetSDK. In CCTV Sentinel Universal, the Dahua adapter interfaces directly with the Dahua RPC/CGI subsystem via HTTP Digest authentication.

```
[Dahua Hardware] <--- 1. GET /cgi-bin/magicBox.cgi?action=getSystemInfo --- [Sentinel Dahua Client]
                 ---> 2. Returns HTTP 401 Unauthorized                  ---> Parses Realm & Nonce
                      realm="Login to 62F63944EFFEE4EF", nonce="..."
                 <--- 3. Re-dispatches with Authorization: Digest        ---
                 ---> 4. Returns 200 OK + Key-Value Configuration        ---> Normalizes into Device Entity
```

---

## 2. HTTP Digest Authentication & Physical Observation

Physical Dahua IP cameras and recorders enforce RFC 2617 Digest authentication across all `/cgi-bin/` paths.

Empirically observed responses from physical test hardware:

```http
# Target: Dahua DH-IPC-HFW1230T1P-ZS-S4 (192.168.31.110:80)
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Digest realm="Login to 62F63944EFFEE4EF", qop="auth", nonce="147348b2-6e00-481a-a63a-6a38dfda08aa", opaque=""

# Target: Dahua DH-IPC-HDW1230T1-A-S5 (192.168.31.102:80)
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Digest realm="Login to BCC9C8645589343A", qop="auth", nonce="c57d74f3-eef0-46a0-8051-a3257074c9b7", opaque=""

# Target: Dahua DH-IPC-HFW1249S1-A-IL (192.168.31.107:80)
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Digest realm="Login to 5E885F5AC5CDBFA4", qop="auth", nonce="b3d2a916-dab0-4152-acfb-dc947b8f9063", opaque="", algorithm=MD5
```

---

## 3. Implemented Endpoints & Key-Value Schemas

Dahua CGI returns responses formatted as key-value pairs separated by newlines: `key=value`.

| Endpoint & Action | Function | Sample Response Keys |
| :--- | :--- | :--- |
| `/cgi-bin/magicBox.cgi?action=getSystemInfo` | System & Device Identification | `deviceType=DH-IPC-HFW1230T1P-ZS`<br>`serialNumber=...`<br>`appVersion=4.001.0000001.0` |
| `/cgi-bin/configManager.cgi?action=getConfig&name=Network` | Network Configuration | `table.Network.eth0.IPAddress=192.168.31.110`<br>`table.Network.eth0.SubnetMask=255.255.255.0`<br>`table.Network.eth0.DhcpEnable=false` |
| `/cgi-bin/configManager.cgi?action=setConfig&Network.eth0...` | Network Apply | Returns `OK` |
| `/cgi-bin/configManager.cgi?action=getConfig&name=VideoColor` | Image & Sensor Calibration | `table.VideoColor[0][0].Brightness=50`<br>`table.VideoColor[0][0].Contrast=50`<br>`table.VideoColor[0][0].Saturation=50` |
| `/cgi-bin/configManager.cgi?action=setConfig&VideoColor...` | Image Apply | Returns `OK` |
| `/cgi-bin/ptz.cgi?action=start&channel=1&code=...` | Pan / Tilt / Zoom Velocity | Controls PTZ motors (`Up`, `Down`, `Left`, `Right`, `ZoomTele`, `ZoomWide`) |
| `/cgi-bin/snapshot.cgi?channel=1` | High-Resolution Snapshot | Returns JPEG image octets |
| `/cgi-bin/configManager.cgi?action=setConfig&CoaxialSensor[0].Format=...` | HDCVI Coaxial Analog Format | Sets analog format to CVI, TVI, AHD, or CVBS on XVR |

---

## 4. Hardware Verification Summary

Physical bench testing on `192.168.31.23/24` verified:
- Correct rejection of Hikvision ISAPI (returned 404).
- Reliable challenge-response Digest negotiation over `/cgi-bin/magicBox.cgi`.
- Fixed turret (`DH-IPC-HDW1230T1`) and fixed bullet (`DH-IPC-HFW1249S1`) models correctly identified with `PTZ: UNSUPPORTED`.
- Motorized varifocal bullet (`DH-IPC-HFW1230T1P-ZS-S4`) correctly identified with `PTZ: SUPPORTED` for motorized zoom and focus controls.
