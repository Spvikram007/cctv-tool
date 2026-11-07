import React, { useState } from 'react';
import { api } from '../services/api';
import { DiagnosticsReport } from '../types';
import { Wrench, CheckCircle, XCircle, AlertCircle, RefreshCw } from 'lucide-react';

export const Diagnostics: React.FC = () => {
  const [targetIP, setTargetIP] = useState('127.0.0.1');
  const [httpPort, setHttpPort] = useState(80);
  const [rtspPort, setRtspPort] = useState(554);
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState<DiagnosticsReport | null>(null);

  const handleRun = async () => {
    setLoading(true);
    try {
      const res = await api.runDiagnostics(targetIP, rtspPort, httpPort);
      setReport(res);
    } catch (err: any) {
      alert(`Diagnostics failed: ${err.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header & Input Bar */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
        <div>
          <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
            <Wrench className="w-5 h-5 text-emerald-400" />
            <span>NETWORK & PROTOCOL DIAGNOSTICS SUITE</span>
          </h2>
          <p className="text-xs text-slate-400 font-mono mt-1">
            Conduct socket latency, open port detection, RTSP DESCRIBE/OPTIONS handshake verification, and ONVIF SOAP tests.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">TARGET IP ADDRESS</label>
            <input
              type="text"
              value={targetIP}
              onChange={(e) => setTargetIP(e.target.value)}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">HTTP / ONVIF PORT</label>
            <input
              type="number"
              value={httpPort}
              onChange={(e) => setHttpPort(parseInt(e.target.value))}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">RTSP STREAMING PORT</label>
            <input
              type="number"
              value={rtspPort}
              onChange={(e) => setRtspPort(parseInt(e.target.value))}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
            />
          </div>

          <div className="flex items-end">
            <button
              onClick={handleRun}
              disabled={loading}
              className={`w-full py-2 px-4 rounded-lg text-xs font-bold font-mono flex items-center justify-center space-x-2 transition-all ${
                loading
                  ? 'bg-slate-700 text-slate-400 cursor-not-allowed'
                  : 'bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-900/30'
              }`}
            >
              {loading ? (
                <>
                  <RefreshCw className="w-4 h-4 animate-spin" />
                  <span>PROBING SOCKETS...</span>
                </>
              ) : (
                <>
                  <Wrench className="w-4 h-4" />
                  <span>EXECUTE DIAGNOSTICS</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>

      {/* Diagnostics Report Display */}
      {report && (
        <div className="space-y-6 font-mono">
          {/* Key Metrics */}
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d]">
              <span className="text-[10px] text-slate-400 uppercase">PING LATENCY</span>
              <p className="text-xl font-bold text-slate-100 mt-1">{report.latency_ms.toFixed(2)} ms</p>
              <p className="text-[11px] text-emerald-400 mt-0.5">{report.packet_loss_pct}% Packet Loss</p>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d]">
              <span className="text-[10px] text-slate-400 uppercase">RTSP HANDSHAKE</span>
              <div className="flex items-center space-x-2 mt-1">
                {report.rtsp_working ? (
                  <CheckCircle className="w-5 h-5 text-emerald-400" />
                ) : (
                  <XCircle className="w-5 h-5 text-red-400" />
                )}
                <span className="text-lg font-bold text-slate-100">
                  {report.rtsp_working ? 'FUNCTIONAL' : 'REFUSED'}
                </span>
              </div>
              <p className="text-[10px] text-slate-400 truncate mt-0.5">{report.rtsp_response || 'No Response'}</p>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d]">
              <span className="text-[10px] text-slate-400 uppercase">ONVIF SOAP RESPONDER</span>
              <div className="flex items-center space-x-2 mt-1">
                {report.onvif_working ? (
                  <CheckCircle className="w-5 h-5 text-emerald-400" />
                ) : (
                  <XCircle className="w-5 h-5 text-red-400" />
                )}
                <span className="text-lg font-bold text-slate-100">
                  {report.onvif_working ? 'ACTIVE' : 'OFFLINE'}
                </span>
              </div>
              <p className="text-[10px] text-emerald-400 mt-0.5">Profile S / T Ready</p>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d]">
              <span className="text-[10px] text-slate-400 uppercase">OPEN CCTV PORTS</span>
              <p className="text-lg font-bold text-slate-100 mt-1">
                {report.open_ports?.length > 0 ? report.open_ports.join(', ') : 'None Detected'}
              </p>
              <p className="text-[10px] text-slate-400 mt-0.5">Scanned: 80, 443, 554, 8000, 37777</p>
            </div>
          </div>

          {/* Recommendations Card */}
          {report.recommendations && report.recommendations.length > 0 && (
            <div className="p-5 rounded-xl bg-amber-950/30 border border-amber-800 text-amber-200 text-xs space-y-2">
              <div className="font-bold flex items-center space-x-2 text-amber-400">
                <AlertCircle className="w-4 h-4" />
                <span>TECHNICIAN RECOMMENDATIONS & ACTION ITEMS:</span>
              </div>
              <ul className="list-disc pl-5 space-y-1 text-slate-300">
                {report.recommendations.map((rec, i) => (
                  <li key={i}>{rec}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
