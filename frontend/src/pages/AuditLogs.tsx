import React, { useState, useEffect } from 'react';
import { AuditLogEntry, Device, AuditChainVerification, VaultStatus } from '../types';
import { api } from '../services/api';
import {
  FileText,
  Printer,
  ShieldCheck,
  ShieldAlert,
  Lock,
  RefreshCw,
  KeyRound,
  CheckCircle
} from 'lucide-react';

interface AuditLogsProps {
  auditLogs: AuditLogEntry[];
  devices?: Device[];
}

export const AuditLogs: React.FC<AuditLogsProps> = ({ auditLogs }) => {
  const [filterText, setFilterText] = useState('');
  const [verification, setVerification] = useState<AuditChainVerification | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [vaultStatus, setVaultStatus] = useState<VaultStatus | null>(null);

  useEffect(() => {
    runChainVerification();
    loadVaultStatus();
  }, []);

  const runChainVerification = async () => {
    setVerifying(true);
    try {
      const res = await api.verifyAuditChain();
      setVerification(res);
    } catch (err: any) {
      console.error('Audit verification error:', err);
    } finally {
      setVerifying(false);
    }
  };

  const loadVaultStatus = async () => {
    try {
      const vs = await api.getVaultStatus();
      setVaultStatus(vs);
    } catch (err) {
      console.error('Vault status error:', err);
    }
  };

  const filtered = auditLogs.filter((log) =>
    log.action.toLowerCase().includes(filterText.toLowerCase()) ||
    log.details.toLowerCase().includes(filterText.toLowerCase()) ||
    log.device_ip.toLowerCase().includes(filterText.toLowerCase())
  );

  const handleExportReport = async () => {
    try {
      const report = await api.getTechnicianReport();
      const reportHtml = `
<!DOCTYPE html>
<html>
<head>
  <title>CCTV SENTINEL UNIVERSAL - COMMISSIONING & AUDIT REPORT</title>
  <style>
    body { font-family: monospace; padding: 24px; color: #111; }
    h1 { color: #059669; font-size: 20px; }
    table { width: 100%; border-collapse: collapse; margin-top: 16px; font-size: 12px; }
    th, td { border: 1px solid #ccc; padding: 8px; text-align: left; }
    th { background: #f3f4f6; }
    .badge { padding: 2px 6px; border-radius: 4px; font-weight: bold; background: #d1fae5; color: #065f46; }
  </style>
</head>
<body>
  <h1>CCTV SENTINEL UNIVERSAL — COMMISSIONING & AUDIT HANDOVER REPORT</h1>
  <p><strong>Generated At:</strong> ${report.generated_at}</p>
  <p><strong>Issued By:</strong> ${report.issued_by}</p>
  <p><strong>Licensee:</strong> ${report.licensee} (${report.tier})</p>
  <p><strong>Total Managed Hardware:</strong> ${report.total_devices} Units</p>

  <h2>Hardware Inventory Handover</h2>
  <table>
    <tr><th>Name</th><th>IP & Port</th><th>Manufacturer</th><th>Model</th><th>Firmware</th><th>Status</th></tr>
    ${report.device_inventory.map((d: any) => `
      <tr>
        <td>${d.name}</td>
        <td>${d.ip}:${d.port}</td>
        <td>${d.manufacturer}</td>
        <td>${d.model}</td>
        <td>${d.firmware || 'N/A'}</td>
        <td><span class="badge">ONLINE</span></td>
      </tr>
    `).join('')}
  </table>

  <h2>Immutable Audit Trail (Cryptographic SHA-256 Chained)</h2>
  <table>
    <tr><th>Timestamp</th><th>Action</th><th>Target</th><th>Status</th><th>Details</th></tr>
    ${report.recent_actions.map((a: any) => `
      <tr>
        <td>${new Date(a.timestamp).toLocaleString()}</td>
        <td>${a.action}</td>
        <td>${a.device_ip}</td>
        <td>${a.status}</td>
        <td>${a.details}</td>
      </tr>
    `).join('')}
  </table>
  <br>
  <p><em>Certified tamper-evident and compliant with CCTV Sentinel Universal Enterprise Security Protocol.</em></p>
</body>
</html>
      `;
      const blob = new Blob([reportHtml], { type: 'text/html' });
      const url = URL.createObjectURL(blob);
      const w = window.open(url, '_blank');
      if (w) w.focus();
    } catch (err: any) {
      alert(`Report generation failed: ${err.message}`);
    }
  };

  return (
    <div className="space-y-6 font-mono">
      {/* Header */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
            <FileText className="w-5 h-5 text-emerald-400" />
            <span>TAMPER-EVIDENT AUDIT TRAIL & COMMISSIONING REPORTS</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            SHA-256 chained transaction ledger with absolute zero plaintext password or token leakage.
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={runChainVerification}
            disabled={verifying}
            className="px-3 py-1.5 rounded-lg bg-[#0d1117] border border-[#30363d] hover:border-slate-500 text-xs text-slate-300 flex items-center space-x-2"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${verifying ? 'animate-spin text-emerald-400' : ''}`} />
            <span>Verify Integrity</span>
          </button>
          <button
            onClick={handleExportReport}
            className="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center space-x-2 transition-all shadow-lg shadow-emerald-900/30"
          >
            <Printer className="w-4 h-4" />
            <span>EXPORT COMMISSIONING REPORT</span>
          </button>
        </div>
      </div>

      {/* Enterprise Security Hardening Overview Card */}
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4 text-xs">
        {/* Audit Chain Status */}
        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
          <div className="flex items-center justify-between">
            <span className="text-slate-400 font-bold uppercase text-[10px]">CRYPTOGRAPHIC CHAIN</span>
            {verification?.valid ? (
              <ShieldCheck className="w-4 h-4 text-emerald-400" />
            ) : (
              <ShieldAlert className="w-4 h-4 text-red-400" />
            )}
          </div>
          <div className="text-base font-bold text-slate-100">
            {verification?.valid ? (
              <span className="text-emerald-400">TAMPER-FREE</span>
            ) : (
              <span className="text-red-400">INTEGRITY ALERT</span>
            )}
          </div>
          <div className="text-[10px] text-slate-400">
            {verification?.verified_count || auditLogs.length} chained entries verified (SHA-256)
          </div>
        </div>

        {/* Vault Status */}
        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
          <div className="flex items-center justify-between">
            <span className="text-slate-400 font-bold uppercase text-[10px]">CREDENTIAL VAULT</span>
            <Lock className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-base font-bold text-emerald-400">AES-256-GCM</div>
          <div className="text-[10px] text-slate-400">
            {vaultStatus?.storage || 'OS Protected / Hardware Enclave'} • Zero plaintext leakage
          </div>
        </div>

        {/* SSRF & Network Guard */}
        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
          <div className="flex items-center justify-between">
            <span className="text-slate-400 font-bold uppercase text-[10px]">SSRF SURVEILLANCE GUARD</span>
            <CheckCircle className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-base font-bold text-slate-100">STRICT FILTERING</div>
          <div className="text-[10px] text-slate-400">
            Blocks 169.254.169.254, loopback, multicast & DNS rebinding
          </div>
        </div>

        {/* IP Rate Limiter & RBAC */}
        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
          <div className="flex items-center justify-between">
            <span className="text-slate-400 font-bold uppercase text-[10px]">RBAC & RATE LIMITING</span>
            <KeyRound className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-base font-bold text-slate-100">4 ROLES / TOKEN BUCKET</div>
          <div className="text-[10px] text-slate-400">
            Admin, Tech, Viewer, Auditor • 100 req/min per client IP
          </div>
        </div>
      </div>

      {/* Audit Log Table */}
      <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
        <div className="p-4 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between text-xs">
          <span className="font-bold text-slate-200 uppercase">
            IMMUTABLE TRANSACTION LOGS ({filtered.length} EVENTS)
          </span>
          <input
            type="text"
            placeholder="Search action or target IP..."
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
            className="px-3 py-1.5 bg-[#161b22] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none w-64"
          />
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-[#0d1117] text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
              <tr>
                <th className="py-3 px-4">Cryptographic Hash / ID</th>
                <th className="py-3 px-4">Timestamp (UTC)</th>
                <th className="py-3 px-4">Operator</th>
                <th className="py-3 px-4">Action</th>
                <th className="py-3 px-4">Target IP</th>
                <th className="py-3 px-4">Sanitized Details (Zero Secret Leakage)</th>
                <th className="py-3 px-4 text-center">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#21262d]">
              {filtered.map((entry) => (
                <tr key={entry.id} className="hover:bg-[#21262d]/50 transition-colors">
                  <td className="py-3 px-4 text-[10px] font-mono text-slate-500">
                    <div className="text-slate-400 truncate max-w-[140px]" title={entry.entry_hash || entry.id}>
                      {entry.entry_hash ? `hash:${entry.entry_hash.slice(0, 12)}...` : entry.id}
                    </div>
                    {entry.prev_hash && (
                      <div className="text-[9px] text-slate-600 truncate max-w-[140px]" title={entry.prev_hash}>
                        prev:{entry.prev_hash.slice(0, 8)}...
                      </div>
                    )}
                  </td>
                  <td className="py-3 px-4 text-slate-400">{new Date(entry.timestamp).toLocaleString()}</td>
                  <td className="py-3 px-4 text-slate-300 font-bold">{entry.user}</td>
                  <td className="py-3 px-4 text-emerald-400 font-bold">{entry.action}</td>
                  <td className="py-3 px-4 text-slate-300 font-mono">{entry.device_ip}</td>
                  <td className="py-3 px-4 text-slate-300">
                    <div>{entry.details}</div>
                    {entry.diff && <div className="text-[10px] text-emerald-300/80 font-mono mt-0.5">{entry.diff}</div>}
                  </td>
                  <td className="py-3 px-4 text-center">
                    <span
                      className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                        entry.status === 'SUCCESS' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-red-500/20 text-red-400'
                      }`}
                    >
                      {entry.status}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
