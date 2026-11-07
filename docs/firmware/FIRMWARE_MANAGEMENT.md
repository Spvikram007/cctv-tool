# CCTV SENTINEL UNIVERSAL — SECURE FIRMWARE MANAGEMENT
## Firmware Verification, Lifecycle Pipeline & Emergency Recovery Playbooks

**Product:** CCTV SENTINEL UNIVERSAL  
**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  

---

## 1. Overview

Firmware corruption is the leading cause of permanent hardware loss ("bricking") in field CCTV deployments. Common causes include:
- Uploading firmware intended for a different hardware revision.
- Cross-flashing OEM rebranded cameras (e.g. flashing CP PLUS hardware with generic Dahua firmware).
- Flashing corrupted binary files downloaded over unstable Internet connections.
- Power loss or network disconnection during active flash memory writes.

The **CCTV Sentinel Universal Firmware Management Framework** enforces cryptographic validation, model and hardware revision checks, and staged operator approval before any binary write can occur.

---

## 2. Verified Firmware Catalog Architecture

Firmware packages registered with CCTV Sentinel Universal are tracked in an immutable, signed catalog:

```go
type FirmwarePackage struct {
    ID               string    // e.g. "fw-hik-2cd2043g2-v571"
    Manufacturer     string    // "Hikvision", "Dahua", "CP PLUS"
    Model            string    // "DS-2CD2043G2-I"
    HardwareRevision string    // "G2-REV-B"
    Region           string    // "GLOBAL", "IN", "EU", "US"
    Version          string    // "V5.7.1 build 230915"
    SHA256Checksum   string    // 64-character lowercase hex hash
    FileSizeBytes    int64     // Exact binary byte count
    FileName         string    // "digicap.dav", "General_IPC-...bin"
    Critical         bool      // True for zero-day / CVE patches
    ReleaseNotes     string    // Detailed technician change summary
}
```

### Pre-Seeded Reference Firmware:
- **Hikvision**:
  - `DS-2CD2043G2-I` (v5.7.1 build 230915, `digicap.dav`, G2-REV-B)
  - `DS-7608NI-K2/8P` (v4.30.060 build 230810, `digicap.dav`)
- **Dahua**:
  - `DH-IPC-HFW1230T1P-ZS-S4` (v2.820.0000000.38.R, S4 revision)
  - `DHI-NVR4108HS-4KS2/L` (v4.002.0000000.1.R)
- **CP PLUS**:
  - `CP-UNC-TA21PL3-V3` (v2.800.0000000.18.R, V3 revision, IN region)
  - `CP-UVR-0801E1-V4` (v4.000.0000000.4.R)

---

## 3. Cryptographic Binary Checksum Validation

Before any firmware update is dispatched or staged:
1. The binary file is read using a streaming `crypto/sha256` hasher (`io.Copy`).
2. The computed 64-character hex digest is compared against the catalog checksum.
3. If the computed hash deviates by even one bit, the binary is immediately flagged as **CORRUPTED OR TAMPERED** and deleted from the upload cache.

```go
func VerifyFileChecksum(filePath, expectedSHA256 string) (bool, error) {
    f, err := os.Open(filePath)
    if err != nil { return false, err }
    defer f.Close()

    hasher := sha256.New()
    if _, err := io.Copy(hasher, f); err != nil { return false, err }
    calculated := hex.EncodeToString(hasher.Sum(nil))
    return strings.EqualFold(calculated, expectedSHA256), nil
}
```

---

## 4. Multi-Stage Update Pipeline

When an update is approved, the background worker transitions through 6 distinct stages:

```
[PREPARING] --> [UPLOADING] --> [FLASHING] --> [REBOOTING] --> [VERIFYING] --> [SUCCESS]
     |               |              |              |               |
     +---------------+--------------+--------------+---------------+---> [FAILED]
```

1. **`PREPARING`**: Re-authenticates with camera; verifies storage space and RAM availability.
2. **`UPLOADING`**: Streams the verified binary over HTTP POST / ISAPI upgrade / Dahua RPC multipart stream.
3. **`FLASHING`**: Camera unpacks binary and writes to NAND flash memory. (Device UI indicates upgrade mode).
4. **`REBOOTING`**: Camera restarts. Sentinel initiates connection retry polling on port 80/554.
5. **`VERIFYING`**: Reads back the active firmware version from the device after reboot. Verifies that `running_firmware == target_version`.
6. **`SUCCESS`**: Appends completion event to cryptographic audit ledger.

---

## 5. Field Emergency Unbrick Playbooks

If a camera loses power during an upgrade or fails to complete reboot, field technicians can execute these vendor-specific hardware recovery procedures:

### Playbook A: Hikvision TFTP Auto-Recovery
1. Configure technician laptop Ethernet IP to **`192.0.0.128`**, subnet mask **`255.255.255.0`**.
2. Rename the verified firmware file to **`digicap.dav`** and place it in the TFTP server root directory.
3. Launch the Hikvision TFTP Server executable.
4. Connect the laptop directly to the camera's RJ45 port via a PoE injector.
5. Power on the camera. The Hikvision U-Boot bootloader broadcasts a UDP probe to `192.0.0.128:69` during the first 5 seconds of boot.
6. The TFTP server transfers `digicap.dav`; the camera flashes and automatically reboots to factory defaults.

### Playbook B: Dahua ConfigTool & Emergency Subnet Reflash
1. Set PC network interface to **`192.168.254.254`** / **`255.255.255.0`**.
2. Open Dahua ConfigTool $\to$ Upgrade Menu.
3. Search for broadcasting cameras on the default recovery IP range (`192.168.1.108` or `192.168.254.x`).
4. Select the matching `.bin` package and click "Upgrade".
5. For bricked cameras, connect via 3.3V UART serial pinout (TX, RX, GND) at 115200 baud; interrupt U-Boot and run:
   ```sh
   setenv serverip 192.168.254.254
   setenv ipaddr 192.168.254.100
   run up
   ```

### Playbook C: CP PLUS UVR / NVR USB Recovery
1. Format a clean USB flash drive (16GB or less) as **FAT32**.
2. Copy the verified recovery firmware named **`update.bin`** to the root of the USB drive.
3. Insert the drive into the front USB port of the CP PLUS recorder.
4. Press and hold the front-panel Reset button while switching on power.
5. Hold for 15 seconds until the power and HDD LEDs begin flashing alternately.
6. The bootloader executes auto-recovery from the USB drive within 3 minutes and reboots.
