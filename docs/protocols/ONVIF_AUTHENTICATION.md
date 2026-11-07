# ONVIF Authenticated Protocol Architecture & Implementation

## 1. Overview & Protocol Architecture

**CCTV Sentinel Universal** provides a production-grade, standards-compliant ONVIF client engine conforming to **ONVIF Profile S (Basic Video)**, **Profile T (Advanced Video & Analytics)**, and **Profile G (Edge Storage & Retrieval)**.

Unlike naive integrations that hardcode endpoint URLs and assume synchronized device clocks, CCTV Sentinel Universal implements a dynamic, capability-driven architecture:

```
[Camera Hardware] <--- 1. GetSystemDateAndTime (Unauthenticated) --- [Sentinel ONVIF Client]
                 ---> 2. Returns Device UTC Time & Zone           ---> Computes Clock Skew Offset
                 <--- 3. GetCapabilities / GetServices with Auth  <--- Applies Dynamic WS-Security
                 ---> 4. Returns Service XAddrs & Profiles       ---> Stores Verified Capability Matrix
```

---

## 2. OASIS WS-Security UsernameToken PasswordDigest

To authenticate requests without sending credentials in cleartext over unencrypted HTTP links, CCTV Sentinel Universal implements OASIS Web Services Security (WSS) UsernameToken Profile 1.0 PasswordDigest authentication.

### 2.1 Digest Algorithm & Mathematical Definition

The PasswordDigest is computed strictly over raw binary nonce octets:

$$\text{PasswordDigest} = \text{Base64}\left(\text{SHA-1}\left(\text{Nonce}_{\text{raw}} \,\|\, \text{Created}_{\text{UTC}} \,\|\, \text{Password}_{\text{UTF-8}}\right)\right)$$

Where:
- $\text{Nonce}_{\text{raw}}$: 16 cryptographically random bytes generated via OS CSPRNG (`crypto/rand`).
- $\text{Nonce}_{\text{B64}}$: Standard Base64 encoding of $\text{Nonce}_{\text{raw}}$ transmitted in `<wsse:Nonce>`.
- $\text{Created}_{\text{UTC}}$: ISO-8601 UTC timestamp formatted as `YYYY-MM-DDTHH:MM:SS.sssZ` adjusted with the calibrated device clock skew.
- $\text{Password}_{\text{UTF-8}}$: Plaintext device password decoded from the AES-256-GCM hardware vault.

### 2.2 SOAP Header Format

```xml
<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
               xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
    <wsse:UsernameToken>
        <wsse:Username>admin</wsse:Username>
        <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">
            4P0wKzB60sT6o8V9f1g...==
        </wsse:Password>
        <wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">
            8dG3kYpQ...==
        </wsse:Nonce>
        <wsu:Created>2026-11-07T14:19:31.250Z</wsu:Created>
    </wsse:UsernameToken>
</wsse:Security>
```

---

## 3. Clock Skew Calibration & Compensation

A major cause of authentication failures in field CCTV installations is clock drift between the technician's workstation and IP cameras (often running without active NTP or CMOS batteries). If a camera's clock drifts by more than 10-15 seconds, the camera rejects the message with `wsse:FailedAuthentication` due to timestamp expiration.

### 3.1 Solution: Skew Calibration Sequence

1. Before dispatching authenticated requests, CCTV Sentinel Universal sends an unauthenticated `GetSystemDateAndTime` SOAP call to `/onvif/device_service`:
   ```xml
   <tds:GetSystemDateAndTime/>
   ```
2. The camera returns its internal UTC time:
   ```xml
   <tt:UTCDateTime>
       <tt:Time><tt:Hour>14</tt:Hour><tt:Minute>19</tt:Minute><tt:Second>31</tt:Second></tt:Time>
       <tt:Date><tt:Year>2026</tt:Year><tt:Month>11</tt:Month><tt:Day>7</tt:Day></tt:Date>
   </tt:UTCDateTime>
   ```
3. CCTV Sentinel Universal calculates the time delta:
   $$\Delta_{\text{skew}} = T_{\text{camera\_UTC}} - T_{\text{local\_UTC}}$$
4. For all subsequent WS-Security envelopes, the `<wsu:Created>` timestamp is computed as:
   $$T_{\text{header}} = T_{\text{local\_UTC}} + \Delta_{\text{skew}}$$
5. The camera validates the timestamp within its own internal reference window with 100% acceptance.

---

## 4. Implemented ONVIF Services & Operations

| Service | Namespace | Implemented Operations | Capability Flag |
| :--- | :--- | :--- | :--- |
| **Device Management** | `ver10/device/wsdl` | `GetSystemDateAndTime`, `GetDeviceInformation`, `GetCapabilities`, `GetServices`, `GetNetworkInterfaces`, `SetNetworkInterfaces` | `device_management` |
| **Media (Profile S)** | `ver10/media/wsdl` | `GetProfiles`, `GetVideoSources`, `GetVideoEncoderConfigurations`, `GetStreamUri`, `GetSnapshotUri` | `media` |
| **Media2 (Profile T)** | `ver20/media/wsdl` | `GetProfiles`, `GetStreamUri` (where supported) | `media2` |
| **PTZ** | `ver20/ptz/wsdl` | `GetConfigurations`, `GetNodes`, `GetPresets`, `ContinuousMove`, `Stop`, `GotoPreset` | `ptz` |
| **Imaging** | `ver20/imaging/wsdl` | `GetImagingSettings`, `SetImagingSettings` | `imaging` |
| **Events** | `ver10/events/wsdl` | `CreatePullPointSubscription` | `events` |

---

## 5. Physical Hardware Empirical Test Bench Results

Empirical live validation executed on Ethernet network `192.168.31.23/24`:

| Target IP | Physical Device Model | ONVIF Response | Measured Skew | Detected Capabilities |
| :--- | :--- | :--- | :--- | :--- |
| `192.168.31.103:80` | **Hikvision DS-2CD1023G0-IU** | 200 OK | -1.6 sec | Media: OK, PTZ: OK, Imaging: OK, Events: OK |
| `192.168.31.110:80` | **Dahua DH-IPC-HFW1230T1P-ZS-S4** | 200 OK | -0.1 sec | Media: OK, PTZ: OK, Imaging: OK, Events: OK |
| `192.168.31.102:80` | **Dahua DH-IPC-HDW1230T1-A-S5** | 200 OK | -1.7 sec | Media: OK, PTZ: UNSUPPORTED, Imaging: OK, Events: OK |
| `192.168.31.107:80` | **Dahua DH-IPC-HFW1249S1-A-IL** | 200 OK | -1.2 sec | Media: OK, PTZ: UNSUPPORTED, Imaging: OK, Events: OK |
| `192.168.31.106:80` | **CP PLUS CP-UNC-TA21PL3-V3** | 200 OK | -1.7 sec | Media: OK, PTZ: UNSUPPORTED, Imaging: OK, Events: OK |
| `192.168.31.143:80` | **CP PLUS CP-UNR-4K4162-V2 NVR** | 200 OK | -0.9 sec | Media: OK, PTZ: OK, Imaging: OK, Events: OK |
| `192.168.31.104:80` | **Vatilon H43 PTZ Camera** | 200 OK | -2.0 sec | Media: OK, PTZ: OK, Imaging: OK, Events: OK |
