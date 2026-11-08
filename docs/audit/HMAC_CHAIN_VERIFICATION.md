# HMAC Audit Chain Verification & Ledger Integrity
**Product:** CCTV SENTINEL UNIVERSAL  
**Module:** Enterprise Cryptographic Audit Ledger (`backend/pkg/audit/audit.go`)  
**Release:** Phase 6.1 & 7.1 Security Remediation & Reliability  
**Author:** Tech Defence Solutions  

---

## 1. Cryptographic Architecture

CCTV Sentinel Universal implements an immutable, tamper-evident audit ledger modeled after forward-secure cryptographic event journals. Every operational action (authentication, configuration preview/apply/rollback, firmware flashing, PTZ robotic adjustment, disk format, and session management) is committed as a sequential ledger entry.

### 1.1 Hash Chain Linkage
Every log record $E_i$ is bound cryptographically to its predecessor $E_{i-1}$ using SHA-256:
$$\text{EntryHash}_i = \text{SHA256}(\text{Index}_i \parallel \text{Timestamp}_i \parallel \text{Operator}_i \parallel \text{TargetDevice}_i \parallel \text{Action}_i \parallel \text{Status}_i \parallel \text{PrevHash}_{i-1})$$

For the genesis entry ($i = 0$):
$$\text{PrevHash}_0 = 0000000000000000000000000000000000000000000000000000000000000000$$

### 1.2 HMAC-SHA256 Periodic Checkpoints
Every 10 entries (and at forced synchronization points), an authenticated HMAC-SHA256 checkpoint signature is computed:
$$\text{CheckpointHMAC}_k = \text{HMAC-SHA256}(K_{\text{active}}, \text{EntryHash}_k \parallel \text{Timestamp}_k \parallel \text{Index}_k)$$

This prevents an attacker who gains access to the database file from modifying historical records and recalculating the SHA-256 chain, because the HMAC root key is stored outside the database store and protected with `0600` file system permissions.

---

## 2. Keyring Architecture & Rotation

### 2.1 Key Lifecycle & Persistence
1. **Entropy & Generation:** The HMAC root key is generated exclusively using `crypto/rand` (256-bit entropy). Deterministic or hardcoded fallback keys have been completely removed.
2. **Key Storage:** Persisted to `./data/sentinel_audit.key` (or local vault) with restricted permissions (`0600`).
3. **Key Rotation (`RotateCheckpointKey()`):**
   - A new 32-byte cryptographically secure key is generated with key identifier `audit-key-<timestamp>`.
   - Prior keys are archived in an in-memory and disk-persisted `keyRing map[string][]byte`.
   - Historical checkpoints signed with older keys remain fully verifiable without invalidating the ledger history.

---

## 3. Cryptographically Signed Ledger Export

The ledger can be exported for regulatory compliance and forensic testimony via `GET /api/v1/audit/export/signed`.

### 3.1 Verification Format
```json
{
  "version": "1.0",
  "exported_at": "2026-11-08T03:15:23Z",
  "key_id": "audit-key-1794107723",
  "signature": "3f88b0a5c4d5e9f8...",
  "payload": {
    "total_records": 128,
    "chain_valid": true,
    "genesis_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "tip_hash": "a4d9523bf1...",
    "entries": [ ... ]
  }
}
```

### 3.2 Verification Procedure (`VerifyExportedLedger`)
1. Payload bytes are parsed as canonical `json.RawMessage`.
2. The key identifier `key_id` is looked up in the keyring.
3. The HMAC-SHA256 signature is calculated over the exact raw payload bytes and compared using `subtle.ConstantTimeCompare`.
4. The SHA-256 hash link between every adjacent entry $E_{i-1}$ and $E_i$ is validated sequentially.
5. If any field is modified (operator, IP address, timestamp, status), verification immediately fails with `ErrAuditTampered`.

---

## 4. Test Verification Evidence

Validated via unit and regression test suite `backend/pkg/audit/audit_test.go`:
- `TestAuditSecretRedaction`: Confirms passwords, tokens, and digest nonces are sanitized (`***REDACTED***`).
- `TestAuditHashChainAndTamperDetection`: Confirms SHA-256 chain integrity and rejection of altered records.
- `TestHMACCheckpointsAndKeyRotation`: Confirms periodic checkpoint validation across key rotation events.
- `TestExportedLedgerCryptographicVerification`: Confirms signature validity and instant detection of single-byte payload tampering.

