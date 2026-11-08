# SSRF & Outbound Dialing Protection Matrix
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** Network Dialing & SSRF Defense (`backend/pkg/security/ssrf.go`)  
**Release:** Phase 6.1 Stabilization  
**Author:** Tech Defence Solutions  

---

## 1. Threat Model & Outbound Controls

As an on-premises CCTV and video surveillance management tool, CCTV Sentinel initiates outbound network connections across local subnets to discover, authenticate, stream, and configure cameras and recorders. This operational profile makes robust Server-Side Request Forgery (SSRF) and DNS Rebinding defenses mandatory.

---

## 2. IP Blocking Matrix

The safe HTTP client (`NewSafeHTTPClient`) and RTSP dialer intercept and validate destination IP addresses across all outbound network connections:

| IP Category / Range | CIDR / Address | Status | Rationale |
| :--- | :--- | :--- | :--- |
| **Loopback** | `127.0.0.0/8`, `::1` | **BLOCKED** | Prevents accessing host loopback services, Redis, DBs, internal metrics |
| **Cloud Metadata IPv4** | `169.254.169.254` | **BLOCKED** | Prevents AWS / GCP / Azure IMDS credential and role theft |
| **Alibaba Cloud Metadata** | `100.100.100.200` | **BLOCKED** | Prevents Alibaba Cloud ECS metadata access |
| **Cloud Metadata IPv6** | `fd00:ec2::254` | **BLOCKED** | Prevents AWS Nitro IMDSv2 IPv6 metadata exfiltration |
| **Link-Local IPv4** | `169.254.0.0/16` | **BLOCKED** | Prevents auto-configuration link-local abuse |
| **Link-Local IPv6** | `fe80::/10` | **BLOCKED** | Prevents link-local neighbor solicitation abuse |
| **Multicast** | `224.0.0.0/4`, `ff00::/8` | **RESTRICTED** | Blocked for unicast HTTP/RTSP; permitted only in dedicated discovery sockets |
| **Broadcast** | `255.255.255.255` | **BLOCKED** | Prevents broadcast flooding attacks |
| **Private RFC1918 (LAN)** | `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` | **ALLOWED** | Valid CCTV camera and recorder subnet communication |

---

## 3. DNS Rebinding & TOCTOU Mitigation

A common bypass against IP blacklisting is DNS rebinding, where a malicious hostname initially resolves to an allowed IP address to pass validation, and subsequently resolves to `127.0.0.1` or `169.254.169.254` when the socket is dialed (Time-of-Check to Time-of-Use).

CCTV Sentinel implements **Post-Resolution Pinning**:
```go
// 1. Resolve host and validate all resolved IPs
ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
for _, ip := range ips {
    if IsBlockedIP(ip) {
        return nil, ErrSSRFBlocked
    }
}

// 2. Pin the validated IP address directly into the socket dialer
dialer := &net.Dialer{Timeout: 5 * time.Second}
return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
```

### Redirect Target Validation
HTTP redirects (`CheckRedirect` hook) are recursively evaluated against the SSRF validation policy. If a remote device or proxy issues a `301/302` redirect pointing to a cloud metadata service or localhost, the redirect is immediately aborted with `ErrSSRFRedirectBlocked`.
