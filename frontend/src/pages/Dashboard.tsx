import React from 'react';
import { Device, License, AuditLogEntry } from '../types';
import { Video, ShieldCheck, Activity, Cpu, ArrowUpRight, Radar, Wrench } from 'lucide-react';

interface DashboardProps {
  devices: Device[];
  license: License | null;
  auditLogs: AuditLogEntry[];
  onNavigate: (tab: any) => void;
  onSelectDevice: (dev: Device) => void;
}

export const Dashboard: React.FC<DashboardProps> = ({
  devices,
  license,
  auditLogs,
  onNavigate,
  onSelectDevice,
}) => {
  const onlineCount = devices.filter((d) => d.status === 'online').length;

  return (
    <div className="space-y-6">
      {/* Welcome Banner */}
      <div className="p-6 rounded-xl bg-gradient-to-r from-[#161b22] to-[#0d1117] border border-[#30363d] flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center space-x-2">
            <span className="px-2 py-0.5 text-[10px] font-mono uppercase bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 rounded">
              TECHNICIAN CONSOLE
            </span>
            <span className="text-xs text-slate-400 font-mono">SITE: MAIN FLEET OVERVIEW</span>
          </div>
          <h2 className="text-2xl font-bold text-slate-100 mt-1">CCTV SENTINEL UNIVERSAL</h2>
          <p className="text-xs text-slate-400 max-w-xl mt-1">
            Production-grade discovery, multi-vendor configuration, XVR analog format control, and diagnostics platform by Tech Defence Solutions.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => onNavigate('discovery')}
            className="flex items-center space-x-2 px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold shadow-lg shadow-emerald-900/30 transition-all"
          >
            <Radar className="w-4 h-4" />
            <span>DISCOVER DEVICES</span>
          </button>
          <button
            onClick={() => onNavigate('diagnostics')}
            className="flex items-center space-x-2 px-4 py-2 rounded-lg bg-[#21262d] hover:bg-[#30363d] text-slate-200 text-xs font-semibold border border-[#30363d] transition-all"
          >
            <Wrench className="w-4 h-4" />
            <span>RUN DIAGNOSTICS</span>
          </button>
        </div>
      </div>

      {/* KPI Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center justify-between">
          <div>
            <p className="text-xs font-mono text-slate-400">ENROLLED FLEET</p>
            <h3 className="text-2xl font-bold text-slate-100 mt-1">{devices.length}</h3>
            <p className="text-[11px] text-emerald-400 font-mono mt-0.5">{onlineCount} Online & Verified</p>
          </div>
          <div className="h-12 w-12 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Video className="w-6 h-6" />
          </div>
        </div>

        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center justify-between">
          <div>
            <p className="text-xs font-mono text-slate-400">LICENSE STATUS</p>
            <h3 className="text-xl font-bold text-slate-100 mt-1">{license?.tier || 'COMMUNITY'}</h3>
            <p className="text-[11px] text-slate-400 font-mono mt-0.5">Limit: {license?.max_devices || 8} Units</p>
          </div>
          <div className="h-12 w-12 rounded-lg bg-blue-500/10 border border-blue-500/20 flex items-center justify-center text-blue-400">
            <ShieldCheck className="w-6 h-6" />
          </div>
        </div>

        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center justify-between">
          <div>
            <p className="text-xs font-mono text-slate-400">PROTOCOL STACK</p>
            <h3 className="text-xl font-bold text-slate-100 mt-1">4 ADAPTERS</h3>
            <p className="text-[11px] text-emerald-400 font-mono mt-0.5">ISAPI / CGI / ONVIF / RTSP</p>
          </div>
          <div className="h-12 w-12 rounded-lg bg-purple-500/10 border border-purple-500/20 flex items-center justify-center text-purple-400">
            <Cpu className="w-6 h-6" />
          </div>
        </div>

        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center justify-between">
          <div>
            <p className="text-xs font-mono text-slate-400">AUDIT TRAIL</p>
            <h3 className="text-2xl font-bold text-slate-100 mt-1">{auditLogs.length}</h3>
            <p className="text-[11px] text-emerald-400 font-mono mt-0.5">Zero Secret Leakage</p>
          </div>
          <div className="h-12 w-12 rounded-lg bg-amber-500/10 border border-amber-500/20 flex items-center justify-center text-amber-400">
            <Activity className="w-6 h-6" />
          </div>
        </div>
      </div>

      {/* Grid: Active Fleet & Audit Log */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Device Quick List */}
        <div className="lg:col-span-2 p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-slate-200 tracking-wide uppercase font-mono">
              ACTIVE HARDWARE UNITS
            </h3>
            <button
              onClick={() => onNavigate('devices')}
              className="text-xs text-emerald-400 hover:text-emerald-300 font-mono flex items-center space-x-1"
            >
              <span>MANAGE FLEET</span>
              <ArrowUpRight className="w-3.5 h-3.5" />
            </button>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs font-mono">
              <thead className="text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
                <tr>
                  <th className="pb-2">Device Name</th>
                  <th className="pb-2">IP & Port</th>
                  <th className="pb-2">Vendor</th>
                  <th className="pb-2">Category</th>
                  <th className="pb-2">Status</th>
                  <th className="pb-2 text-right">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#21262d]">
                {devices.map((dev) => (
                  <tr key={dev.id} className="hover:bg-[#21262d]/50 transition-colors">
                    <td className="py-2.5 font-semibold text-slate-200">{dev.name}</td>
                    <td className="py-2.5 text-slate-400">{dev.ip}:{dev.port}</td>
                    <td className="py-2.5">
                      <span className="px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300 border border-slate-700">
                        {dev.manufacturer}
                      </span>
                    </td>
                    <td className="py-2.5 text-slate-400">{dev.category}</td>
                    <td className="py-2.5">
                      <span className="flex items-center space-x-1.5 text-emerald-400">
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                        <span>ONLINE</span>
                      </span>
                    </td>
                    <td className="py-2.5 text-right">
                      <button
                        onClick={() => {
                          onSelectDevice(dev);
                          onNavigate('config');
                        }}
                        className="px-2 py-1 rounded bg-[#21262d] hover:bg-emerald-600/30 hover:text-emerald-300 text-slate-300 border border-slate-700 text-[11px] transition-colors"
                      >
                        CONFIGURE
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Recent Audit Trail */}
        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-slate-200 tracking-wide uppercase font-mono">
              AUDIT LOGS
            </h3>
            <button
              onClick={() => onNavigate('audit')}
              className="text-xs text-emerald-400 hover:text-emerald-300 font-mono"
            >
              VIEW ALL
            </button>
          </div>

          <div className="space-y-3 font-mono text-xs">
            {auditLogs.slice(0, 5).map((log) => (
              <div key={log.id} className="p-2.5 rounded-lg bg-[#0d1117] border border-[#21262d] space-y-1">
                <div className="flex items-center justify-between text-[10px] text-slate-500">
                  <span className="text-emerald-400 font-bold">{log.action}</span>
                  <span>{new Date(log.timestamp).toLocaleTimeString()}</span>
                </div>
                <p className="text-slate-300 text-[11px] line-clamp-1">{log.details}</p>
                <div className="text-[10px] text-slate-400 flex items-center justify-between">
                  <span>Target: {log.device_ip || 'SYSTEM'}</span>
                  <span className="text-emerald-500">{log.status}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
