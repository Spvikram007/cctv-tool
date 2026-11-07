import React, { useState, useEffect } from 'react';
import { api } from '../services/api';
import { Device } from '../types';
import { Radar, Search, CheckCircle, ShieldAlert, PlusCircle, RefreshCw } from 'lucide-react';

interface DiscoveryProps {
  onDeviceEnrolled: () => void;
}

export const Discovery: React.FC<DiscoveryProps> = ({ onDeviceEnrolled }) => {
  const [cidr, setCidr] = useState('127.0.0.1/32');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('Admin@123');
  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState<any>({ total_ips: 0, scanned_ips: 0, percentage: 0, status: 'IDLE', found: [] });
  const [enrolledMap, setEnrolledMap] = useState<Record<string, boolean>>({});

  useEffect(() => {
    let interval: any = null;
    if (scanning) {
      interval = setInterval(async () => {
        try {
          const res = await api.getDiscoveryProgress();
          setProgress(res);
          if (res.status === 'FINISHED') {
            setScanning(false);
            clearInterval(interval);
          }
        } catch (e) {
          console.error(e);
        }
      }, 800);
    }
    return () => clearInterval(interval);
  }, [scanning]);

  const handleStartScan = async () => {
    setScanning(true);
    try {
      await api.startDiscovery(cidr, username, password);
    } catch (err: any) {
      alert(`Scan failed to start: ${err.message}`);
      setScanning(false);
    }
  };

  const handleEnroll = async (dev: Device) => {
    try {
      await api.addDevice(dev);
      setEnrolledMap((prev) => ({ ...prev, [dev.id]: true }));
      onDeviceEnrolled();
    } catch (err: any) {
      alert(`Enrollment failed: ${err.message}`);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
        <div>
          <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
            <Radar className="w-5 h-5 text-emerald-400" />
            <span>UNIVERSAL SUBNET & ONVIF DISCOVERY ENGINE</span>
          </h2>
          <p className="text-xs text-slate-400 font-mono mt-1">
            Multicast ONVIF WS-Discovery, ARP MAC OUI lookup, and user-authorized IP range probing without brute forcing.
          </p>
        </div>

        {/* Scan Parameters */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">SUBNET / CIDR RANGE</label>
            <input
              type="text"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              placeholder="192.168.1.0/24"
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">PROBE USERNAME</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">PROBE PASSWORD</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
            />
          </div>

          <div className="flex items-end">
            <button
              onClick={handleStartScan}
              disabled={scanning}
              className={`w-full py-2 px-4 rounded-lg text-xs font-bold font-mono flex items-center justify-center space-x-2 transition-all ${
                scanning
                  ? 'bg-slate-700 text-slate-400 cursor-not-allowed'
                  : 'bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-900/30'
              }`}
            >
              {scanning ? (
                <>
                  <RefreshCw className="w-4 h-4 animate-spin" />
                  <span>PROBING NETWORK...</span>
                </>
              ) : (
                <>
                  <Search className="w-4 h-4" />
                  <span>START DISCOVERY</span>
                </>
              )}
            </button>
          </div>
        </div>

        {/* Progress Bar */}
        {scanning && (
          <div className="space-y-1.5 pt-2">
            <div className="flex justify-between text-[11px] font-mono text-slate-400">
              <span>SCAN PROGRESS ({progress.scanned_ips} / {progress.total_ips} IPs)</span>
              <span>{progress.percentage}%</span>
            </div>
            <div className="w-full h-2 rounded-full bg-[#0d1117] overflow-hidden">
              <div
                className="h-full bg-emerald-500 transition-all duration-300"
                style={{ width: `${progress.percentage}%` }}
              ></div>
            </div>
          </div>
        )}
      </div>

      {/* Discovered Items */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-bold text-slate-200 tracking-wide uppercase font-mono">
            DISCOVERED CCTV HARDWARE ({progress.found?.length || 0})
          </h3>
          <span className="text-[11px] text-slate-400 font-mono">VERIFIED BY ACTIVE PROTOCOL HANDSHAKE</span>
        </div>

        {progress.found && progress.found.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {progress.found.map((dev: Device) => {
              const isEnrolled = enrolledMap[dev.id];
              return (
                <div
                  key={dev.id}
                  className="p-4 rounded-xl bg-[#0d1117] border border-[#30363d] space-y-3 relative overflow-hidden"
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
                        {dev.manufacturer}
                      </span>
                      <h4 className="text-sm font-bold text-slate-200 mt-1.5 font-mono">{dev.name}</h4>
                      <p className="text-xs text-slate-400 font-mono">IP: {dev.ip}:{dev.port}</p>
                    </div>
                  </div>

                  <div className="text-[11px] font-mono text-slate-400 space-y-0.5">
                    <div>MODEL: <span className="text-slate-200">{dev.model}</span></div>
                    <div>FIRMWARE: <span className="text-slate-300">{dev.firmware || 'N/A'}</span></div>
                    <div>PROTOCOLS: <span className="text-emerald-400">{dev.protocols?.join(', ')}</span></div>
                  </div>

                  <div className="pt-2 border-t border-[#21262d] flex items-center justify-between">
                    <span className="text-[10px] text-emerald-400 font-mono flex items-center space-x-1">
                      <CheckCircle className="w-3.5 h-3.5" />
                      <span>IDENTIFIED</span>
                    </span>

                    <button
                      onClick={() => handleEnroll(dev)}
                      disabled={isEnrolled}
                      className={`px-3 py-1.5 rounded text-xs font-mono font-semibold flex items-center space-x-1.5 transition-colors ${
                        isEnrolled
                          ? 'bg-slate-800 text-slate-500 cursor-not-allowed'
                          : 'bg-emerald-600 hover:bg-emerald-500 text-white'
                      }`}
                    >
                      <PlusCircle className="w-3.5 h-3.5" />
                      <span>{isEnrolled ? 'ENROLLED' : 'ADD TO FLEET'}</span>
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="p-8 text-center text-slate-500 font-mono text-xs border border-dashed border-[#30363d] rounded-xl space-y-2">
            <ShieldAlert className="w-8 h-8 text-slate-600 mx-auto" />
            <p>No new devices discovered yet. Initiate a scan above.</p>
            <p className="text-[10px] text-slate-600">Tip: Test with 127.0.0.1/32 to probe the integrated hardware simulator endpoints.</p>
          </div>
        )}
      </div>
    </div>
  );
};
