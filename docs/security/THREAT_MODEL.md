# CCTV SENTINEL UNIVERSAL — THREAT MODEL & RISK ASSESSMENT
## STRIDE Analysis, Attack Surface Review & Countermeasures

**Product:** CCTV SENTINEL UNIVERSAL  
**Company:** Tech Defence Solutions  
**Tagline:** ONE TOOL. EVERY CCTV. COMPLETE CONTROL.  

---

## 1. Scope & System Boundaries

The threat model evaluates risks associated with running CCTV Sentinel Universal in enterprise surveillance LANs, commercial facility management networks, and edge technician workstations.

### Trust Boundaries:
1. **Technician Web Console $\longleftrightarrow$ Sentinel HTTP API**: Semi-trusted LAN. Potential threats: Man-in-the-middle eavesdropping, session hijacking, brute-force authentication.
2. **Sentinel Engine $\longleftrightarrow$ Physical CCTV Devices**: OT Surveillance VLAN. Potential threats: Compromised IoT cameras, rogue rogue network responders, packet sniffing, ARP spoofing.
3. **Storage Subsystem $\longleftrightarrow$ Host Operating System**: Local host environment. Potential threats: Unauthorized database file modification, key extraction, forensic recovery of deleted credentials.

---

## 2. STRIDE Threat Analysis Matrix

| Threat Category | Potential Attack Vector | Impact | Mitigations Implemented in CCTV Sentinel |
|---|---|---|---|
| **Spoofing** | Adversary imitates technician session or injects forged device responses. | Unauthorized fleet control; false inventory records. | Cryptographic session tokens (`sent-sess-...`); bearer token validation; strict correlation IDs on all requests. |
| **Tampering** | Modifying database records, editing firmware binaries, or rewriting audit logs. | Unaudited privilege escalation; bricked cameras; forensic blind spots. | AES-256-GCM authentication tags detect database tampering; SHA-256 checksums verify firmware binaries; cryptographic SHA-256 hash chaining on all audit logs. |
| **Repudiation** | Operator denies initiating a destructive fleet reboot or changing camera passwords. | Inability to establish legal/administrative accountability. | Immutable audit log records `user`, `device_ip`, `action`, `timestamp`, and `diff`; chain verified by `VerifyChainIntegrity()`. |
| **Information Disclosure** | Camera passwords exposed in plaintext logs, API errors, or network captures. | Mass lateral compromise across the CCTV network. | AES-256-GCM encrypted vault; 8 regex scrubbing rules redact passwords, Basic, Digest, Bearer, and RTSP credentials before logging or API serialization. |
| **Denial of Service** | Flooding Sentinel server with HTTP requests, or crashing cameras with high concurrency. | Video loss; technician management interface offline. | Token-bucket IP rate limiter on HTTP server (100 req/min); configurable concurrency (1-20 threads) and per-host rate limiting on camera worker pool. |
| **Elevation of Privilege** | A `VIEWER` account executes batch reboots or updates camera firmware. | Unauthorized configuration alterations. | 4-tier Role-Based Access Control (`ADMIN`, `TECHNICIAN`, `VIEWER`, `AUDITOR`) enforced on every protected endpoint. |

---

## 3. Detailed Attack Surface Review & Specific Defenses

### 3.1 Rogue Camera Cross-Flash / Brick Attack
- **Attack Vector**: An attacker uploads a corrupted binary or a Dahua binary labeled as CP PLUS to trick a technician into flashing an entire branch of cameras, destroying hardware availability.
- **Sentinel Defense**:
  - Binary must match signed catalog SHA-256 checksum.
  - Model check: Device model must match firmware header model.
  - Cross-flash blocker: Explicitly prohibits Dahua $\leftrightarrow$ CP PLUS cross-flashing.
  - Staged preview: Mandates explicit operator confirmation checkbox on power and network stability.

### 3.2 Server-Side Request Forgery (SSRF) Cloud Metadata Pivot
- **Attack Vector**: If Sentinel is deployed on a cloud VM (AWS EC2, Azure VM, GCP Compute), an attacker inputs `http://169.254.169.254/latest/meta-data/` as a target IP in diagnostics or discovery to extract cloud IAM credentials.
- **Sentinel Defense**:
  - `ValidateSurveillanceTarget` parses all input IPs and categorically rejects `169.254.169.254` and link-local ranges (`169.254.0.0/16`).
  - Multicast and loopback targets are rejected by default.

### 3.3 Command Injection via Shell Subprocesses
- **Attack Vector**: Malicious device name or RTSP URL containing shell metacharacters (e.g. `camera1; rm -rf /;`) injected into subprocess calls.
- **Sentinel Defense**:
  - Direct execution via `os/exec` argument slices (`exec.Command("ping", "-n", "1", ip)`).
  - Subshells (`sh -c`, `cmd.exe /c`, `powershell`) are never invoked for device operations.

### 3.4 Plaintext Credential Leaks in Audit Trails
- **Attack Vector**: System error messages containing `401 Unauthorized: user 'admin' pass 'Secret123'` dumped directly into audit logs or technician PDF reports.
- **Sentinel Defense**:
  - `audit.ScrubSecrets()` applies a comprehensive suite of compiled regular expressions targeting:
    - `"password":\s*"[^"]*"`
    - `Basic\s+[A-Za-z0-9+/=]+`
    - `Digest\s+username="[^"]*"`
    - `Bearer\s+[A-Za-z0-9._~+/-]+`
    - `rtsp://[^:]+:[^@]+@`
    - `<wsse:Password[^>]*>[^<]*</wsse:Password>`
  - Plaintext values are replaced with `[REDACTED]`.
