import React, { useState, useEffect } from 'react';
import { api } from '../services/api';
import { Device, NetworkInterface, DiscoveredDevice } from '../types';
import {
  Radar,
  Search,
  CheckCircle,
  ShieldAlert,
  PlusCircle,
  RefreshCw,
  XCircle,
  Network,
  Cpu,
  Eye,
  Info,
  Server
} from 'lucide-react';

interface DiscoveryProps {
  onDeviceEnrolled: () => void;
}

export const Discovery: React.FC<DiscoveryProps> = ({ onDeviceEnrolled }) => {
  const [method, setMethod] = useState<'HYBRID' | 'ONVIF' | 'TCP'>('HYBRID');
  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [selectedInterface, setSelectedInterface] = useState<string>('');
  const [cidr, setCidr] = useState('192.168.31.0/24');
  const [portsStr, setPortsStr] = useState('80, 554, 8000, 37777, 8080, 8001, 8899, 34567');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');

  const [activeJobID, setActiveJobID] = useState<string | null>(null);
  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState<{
    status: string;
    progress_percent: number;
    scanned_targets: number;
    total_targets: number;
    found_count: number;
    results: DiscoveredDevice[];
  }>({
    status: 'IDLE',
    progress_percent: 0,
    scanned_targets: 0,
    total_targets: 0,
    found_count: 0,
    results: [],
  });

  const [enrolledMap, setEnrolledMap] = useState<Record<string, boolean>>({});
  const [selectedDetailDevice, setSelectedDetailDevice] = useState<DiscoveredDevice | null>(null);

  // Load available network interfaces on mount
  useEffect(() => {
    const fetchInterfaces = async () => {
      try {
        const ifaces = await api.getInterfaces();
        setInterfaces(ifaces || []);
        // Prefer first non-loopback UP interface
        const preferred = ifaces.find((i) => i.is_up && !i.is_loopback && i.ipv4_addresses.length > 0);
        if (preferred) {
          setSelectedInterface(preferred.name);
          if (preferred.subnets.length > 0) {
            setCidr(preferred.subnets[0]);
          }
        } else if (ifaces.length > 0) {
          setSelectedInterface(ifaces[0].name);
        }
      } catch (err) {
        console.error('Failed to load interfaces', err);
      }
    };
    fetchInterfaces();
  }, []);

  // Update CIDR automatically when selected interface changes
  const handleInterfaceChange = (ifaceName: string) => {
    setSelectedInterface(ifaceName);
    const iface = interfaces.find((i) => i.name === ifaceName);
    if (iface && iface.subnets.length > 0) {
      setCidr(iface.subnets[0]);
    }
  };

  // Job progress polling loop
  useEffect(() => {
    let interval: any = null;
    if (scanning && activeJobID) {
      interval = setInterval(async () => {
        try {
          const job = await api.getDiscoveryJob(activeJobID);
          const resultsRes = await api.getDiscoveryResults(activeJobID);

          setProgress({
            status: job.status,
            progress_percent: job.progress_percent,
            scanned_targets: job.scanned_targets,
            total_targets: job.total_targets,
            found_count: resultsRes.found_count || (resultsRes.results ? resultsRes.results.length : 0),
            results: resultsRes.results || [],
          });

          if (job.status === 'COMPLETED' || job.status === 'CANCELLED' || job.status === 'FAILED') {
            setScanning(false);
            clearInterval(interval);
          }
        } catch (e) {
          console.error('Failed to poll discovery progress', e);
        }
      }, 700);
    }
    return () => clearInterval(interval);
  }, [scanning, activeJobID]);

  const handleStartScan = async () => {
    setScanning(true);
    setProgress({
      status: 'STARTING',
      progress_percent: 0,
      scanned_targets: 0,
      total_targets: 0,
      found_count: 0,
      results: [],
    });

    const parsedPorts = portsStr
      .split(',')
      .map((p) => parseInt(p.trim(), 10))
      .filter((p) => !isNaN(p) && p > 0 && p <= 65535);

    try {
      const res = await api.startDiscovery({
        method,
        interface: selectedInterface,
        cidr,
        ports: parsedPorts,
        timeout_ms: 350,
        concurrency: 32,
        username,
        password,
      });

      if (res && res.job_id) {
        setActiveJobID(res.job_id);
      }
    } catch (err: any) {
      alert(`Scan failed to start: ${err.message}`);
      setScanning(false);
    }
  };

  const handleCancelScan = async () => {
    if (!activeJobID) return;
    try {
      await api.cancelDiscovery(activeJobID);
      setScanning(false);
    } catch (err: any) {
      console.error('Failed to cancel scan', err);
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
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4 shadow-lg">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-2">
          <div>
            <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
              <Radar className="w-5 h-5 text-emerald-400" />
              <span>UNIVERSAL LAN & ONVIF DISCOVERY ENGINE</span>
            </h2>
            <p className="text-xs text-slate-400 font-mono mt-1">
              Phase 2 Engine: Multicast WS-Discovery (239.255.255.250:3702), Subnet Multi-Port Sweep, and Confidence-Weighted Fingerprinting.
            </p>
          </div>
          <div className="flex items-center space-x-2">
            <span className="px-2.5 py-1 rounded text-[11px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
              DISCOVER → FINGERPRINT → IDENTIFY → CLASSIFY → REGISTER
            </span>
          </div>
        </div>

        {/* Discovery Method Selector Tabs */}
        <div className="flex flex-wrap gap-2 pt-2 border-t border-[#21262d]">
          <button
            onClick={() => setMethod('HYBRID')}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono font-semibold transition-all ${method === 'HYBRID'
                ? 'bg-emerald-600 text-white shadow-md shadow-emerald-950/40'
                : 'bg-[#0d1117] text-slate-400 hover:text-slate-200 border border-[#30363d]'
              }`}
          >
            Universal Hybrid (ONVIF + TCP Sweep) [Recommended]
          </button>
          <button
            onClick={() => setMethod('ONVIF')}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono font-semibold transition-all ${method === 'ONVIF'
                ? 'bg-emerald-600 text-white shadow-md shadow-emerald-950/40'
                : 'bg-[#0d1117] text-slate-400 hover:text-slate-200 border border-[#30363d]'
              }`}
          >
            Multicast ONVIF (WS-Discovery)
          </button>
          <button
            onClick={() => setMethod('TCP')}
            className={`px-3 py-1.5 rounded-lg text-xs font-mono font-semibold transition-all ${method === 'TCP'
                ? 'bg-emerald-600 text-white shadow-md shadow-emerald-950/40'
                : 'bg-[#0d1117] text-slate-400 hover:text-slate-200 border border-[#30363d]'
              }`}
          >
            Subnet TCP Multi-Port Sweep
          </button>
        </div>

        {/* Scan Parameters */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1 flex items-center space-x-1">
              <Network className="w-3.5 h-3.5 text-slate-500" />
              <span>NETWORK INTERFACE</span>
            </label>
            <select
              value={selectedInterface}
              onChange={(e) => handleInterfaceChange(e.target.value)}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
            >
              {interfaces.map((i) => (
                <option key={i.name} value={i.name}>
                  {i.name} ({i.ipv4_addresses.join(', ') || 'No IPv4'}) {i.supports_multicast ? '• Multicast' : ''}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">TARGET SUBNET / CIDR</label>
            <input
              type="text"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              disabled={method === 'ONVIF'}
              placeholder="192.168.1.0/24"
              className={`w-full px-3 py-2 bg-[#0d1117] border rounded-lg text-xs font-mono focus:outline-none ${method === 'ONVIF'
                  ? 'border-[#21262d] text-slate-600 cursor-not-allowed'
                  : 'border-[#30363d] text-slate-200 focus:border-emerald-500'
                }`}
            />
          </div>

          <div>
            <label className="block text-[11px] font-mono text-slate-400 mb-1">PROBE PORTS</label>
            <input
              type="text"
              value={portsStr}
              onChange={(e) => setPortsStr(e.target.value)}
              disabled={method === 'ONVIF'}
              placeholder="80, 554, 8000, 37777..."
              className={`w-full px-3 py-2 bg-[#0d1117] border rounded-lg text-xs font-mono focus:outline-none ${method === 'ONVIF'
                  ? 'border-[#21262d] text-slate-600 cursor-not-allowed'
                  : 'border-[#30363d] text-slate-200 focus:border-emerald-500'
                }`}
            />
          </div>

          <div className="flex items-end space-x-2">
            <button
              onClick={handleStartScan}
              disabled={scanning}
              className={`flex-1 py-2 px-3 rounded-lg text-xs font-bold font-mono flex items-center justify-center space-x-2 transition-all ${scanning
                  ? 'bg-slate-800 text-slate-500 cursor-not-allowed border border-[#30363d]'
                  : 'bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-900/30'
                }`}
            >
              {scanning ? (
                <>
                  <RefreshCw className="w-4 h-4 animate-spin text-emerald-400" />
                  <span>DISCOVERING...</span>
                </>
              ) : (
                <>
                  <Search className="w-4 h-4" />
                  <span>START DISCOVERY</span>
                </>
              )}
            </button>

            {scanning && (
              <button
                onClick={handleCancelScan}
                className="py-2 px-3 rounded-lg text-xs font-bold font-mono bg-red-950/40 hover:bg-red-900/50 text-red-300 border border-red-500/40 flex items-center space-x-1.5 transition-colors"
                title="Cancel running discovery job safely"
              >
                <XCircle className="w-4 h-4" />
                <span>CANCEL</span>
              </button>
            )}
          </div>
        </div>

        {/* Probing credentials preview */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2 border-t border-[#21262d]">
          <div>
            <label className="block text-[10px] font-mono text-slate-400 mb-0.5">DEFAULT AUTH USERNAME</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-2.5 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs font-mono text-slate-200"
            />
          </div>
          <div>
            <label className="block text-[10px] font-mono text-slate-400 mb-0.5">CAMERA AUTH PASSWORD</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-2.5 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs font-mono text-slate-200"
            />
          </div>
        </div>

        {/* Live Progress Bar */}
        {(scanning || progress.status !== 'IDLE') && (
          <div className="space-y-1.5 pt-2 border-t border-[#21262d]">
            <div className="flex justify-between text-[11px] font-mono text-slate-400">
              <span className="flex items-center space-x-2">
                <span>STATUS: {progress.status}</span>
                {progress.total_targets > 0 && (
                  <span>
                    ({progress.scanned_targets} / {progress.total_targets} TARGETS PROBED)
                  </span>
                )}
              </span>
              <span>{progress.progress_percent}%</span>
            </div>
            <div className="w-full h-2 rounded-full bg-[#0d1117] overflow-hidden border border-[#30363d]">
              <div
                className={`h-full transition-all duration-300 ${progress.status === 'CANCELLED'
                    ? 'bg-amber-500'
                    : progress.status === 'FAILED'
                      ? 'bg-red-500'
                      : 'bg-emerald-500'
                  }`}
                style={{ width: `${progress.progress_percent}%` }}
              ></div>
            </div>
          </div>
        )}
      </div>

      {/* Discovered Items */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4 shadow-lg">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-bold text-slate-200 tracking-wide uppercase font-mono flex items-center space-x-2">
            <Cpu className="w-4 h-4 text-emerald-400" />
            <span>DISCOVERED CCTV HARDWARE ({progress.results?.length || 0})</span>
          </h3>
          <span className="text-[11px] text-slate-400 font-mono">
            CONFIDENCE WEIGHTED EVIDENCE MATRIX
          </span>
        </div>

        {progress.results && progress.results.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {progress.results.map((item: DiscoveredDevice) => {
              const dev = item.device;
              const fp = item.fingerprint;
              const isEnrolled = enrolledMap[dev.id];

              // Confidence badge styling
              let confBadgeClass = 'bg-slate-500/10 text-slate-400 border-slate-500/30';
              if (fp.confidence >= 80) {
                confBadgeClass = 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30';
              } else if (fp.confidence >= 50) {
                confBadgeClass = 'bg-amber-500/10 text-amber-400 border-amber-500/30';
              }

              return (
                <div
                  key={dev.id}
                  className="p-4 rounded-xl bg-[#0d1117] border border-[#30363d] space-y-3 relative overflow-hidden hover:border-slate-600 transition-all shadow-md"
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center space-x-2">
                        <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
                          {dev.manufacturer}
                        </span>
                        <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-blue-500/10 text-blue-400 border border-blue-500/30">
                          {dev.category}
                        </span>
                      </div>
                      <h4 className="text-sm font-bold text-slate-200 mt-2 font-mono truncate max-w-[240px]">
                        {dev.name}
                      </h4>
                      <p className="text-xs text-slate-400 font-mono mt-0.5">
                        IP: {dev.ip}:{dev.port} {dev.mac ? `• MAC: ${dev.mac}` : ''}
                      </p>
                    </div>

                    <div className="text-right">
                      <span className={`px-2 py-0.5 rounded text-[10px] font-mono border ${confBadgeClass}`}>
                        {fp.confidence}% {fp.confidence_level}
                      </span>
                      <div className="text-[9px] font-mono text-slate-500 mt-1">
                        VIA: {fp.discovered_via}
                      </div>
                    </div>
                  </div>

                  <div className="text-[11px] font-mono text-slate-400 space-y-1 bg-[#161b22] p-2.5 rounded-lg border border-[#21262d]">
                    <div className="flex justify-between">
                      <span className="text-slate-500">MODEL:</span>
                      <span className="text-slate-200 font-semibold">{dev.model}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-500">FIRMWARE:</span>
                      <span className="text-slate-300">{dev.firmware || 'Unestablished'}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-slate-500">PROTOCOLS:</span>
                      <span className="text-emerald-400">{dev.protocols?.join(', ') || 'N/A'}</span>
                    </div>
                    {fp.services && fp.services.open_ports && (
                      <div className="flex justify-between">
                        <span className="text-slate-500">OPEN PORTS:</span>
                        <span className="text-slate-300">{fp.services.open_ports.join(', ')}</span>
                      </div>
                    )}
                  </div>

                  <div className="pt-2 border-t border-[#21262d] flex items-center justify-between">
                    <button
                      onClick={() => setSelectedDetailDevice(item)}
                      className="text-[11px] font-mono text-slate-400 hover:text-slate-200 flex items-center space-x-1"
                    >
                      <Eye className="w-3.5 h-3.5 text-slate-400" />
                      <span>EVIDENCE ({fp.evidence_sources?.length || 0})</span>
                    </button>

                    <button
                      onClick={() => handleEnroll(dev)}
                      disabled={isEnrolled}
                      className={`px-3 py-1.5 rounded text-xs font-mono font-semibold flex items-center space-x-1.5 transition-colors ${isEnrolled
                          ? 'bg-slate-800 text-slate-500 cursor-not-allowed'
                          : 'bg-emerald-600 hover:bg-emerald-500 text-white'
                        }`}
                    >
                      {isEnrolled ? (
                        <>
                          <CheckCircle className="w-3.5 h-3.5" />
                          <span>ENROLLED</span>
                        </>
                      ) : (
                        <>
                          <PlusCircle className="w-3.5 h-3.5" />
                          <span>ADD TO FLEET</span>
                        </>
                      )}
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="p-8 text-center text-slate-500 font-mono text-xs border border-dashed border-[#30363d] rounded-xl space-y-2">
            <ShieldAlert className="w-8 h-8 text-slate-600 mx-auto" />
            <p>No new devices discovered in this scan session.</p>
            <p className="text-[10px] text-slate-600">
              Tip: Select your active network interface and click "START DISCOVERY" to probe the network.
            </p>
          </div>
        )}
      </div>

      {/* Deep Fingerprint & Evidence Inspector Modal */}
      {selectedDetailDevice && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 z-50">
          <div className="bg-[#161b22] border border-[#30363d] rounded-2xl max-w-2xl w-full max-h-[85vh] overflow-y-auto p-6 space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b border-[#30363d] pb-3">
              <div className="flex items-center space-x-2">
                <Info className="w-5 h-5 text-emerald-400" />
                <h3 className="text-base font-bold text-slate-100 font-mono">
                  DEVICE FINGERPRINT & IDENTIFICATION EVIDENCE
                </h3>
              </div>
              <button
                onClick={() => setSelectedDetailDevice(null)}
                className="text-slate-400 hover:text-slate-200 text-sm font-mono px-2 py-1 rounded"
              >
                ✕
              </button>
            </div>

            <div className="grid grid-cols-2 gap-3 text-xs font-mono bg-[#0d1117] p-3 rounded-lg border border-[#21262d]">
              <div>
                <span className="text-slate-500">IP ADDRESS:</span>{' '}
                <span className="text-slate-200 font-bold">{selectedDetailDevice.device.ip}</span>
              </div>
              <div>
                <span className="text-slate-500">HARDWARE MAC:</span>{' '}
                <span className="text-slate-200 font-bold">{selectedDetailDevice.device.mac || 'N/A'}</span>
              </div>
              <div>
                <span className="text-slate-500">IDENTIFIED MFR:</span>{' '}
                <span className="text-emerald-400 font-bold">{selectedDetailDevice.device.manufacturer}</span>
              </div>
              <div>
                <span className="text-slate-500">PROBABLE MFR:</span>{' '}
                <span className="text-slate-300">{selectedDetailDevice.fingerprint.probable_manufacturer}</span>
              </div>
              <div>
                <span className="text-slate-500">MODEL IDENTIFIED:</span>{' '}
                <span className="text-slate-200 font-bold">{selectedDetailDevice.device.model}</span>
              </div>
              <div>
                <span className="text-slate-500">CATEGORY:</span>{' '}
                <span className="text-blue-400">{selectedDetailDevice.device.category}</span>
              </div>
              <div>
                <span className="text-slate-500">CONFIDENCE:</span>{' '}
                <span className="text-emerald-400 font-bold">
                  {selectedDetailDevice.fingerprint.confidence}% ({selectedDetailDevice.fingerprint.confidence_level})
                </span>
              </div>
              <div>
                <span className="text-slate-500">DISCOVERED VIA:</span>{' '}
                <span className="text-slate-300">{selectedDetailDevice.fingerprint.discovered_via}</span>
              </div>
            </div>

            {/* Evidence Items Breakdown */}
            <div className="space-y-2">
              <h4 className="text-xs font-bold text-slate-300 font-mono uppercase tracking-wide">
                Evidence Breakdown & Scoring Contributions
              </h4>
              <div className="space-y-2">
                {selectedDetailDevice.fingerprint.evidence_sources &&
                  selectedDetailDevice.fingerprint.evidence_sources.length > 0 ? (
                  selectedDetailDevice.fingerprint.evidence_sources.map((ev, idx) => (
                    <div
                      key={idx}
                      className="p-3 rounded-lg bg-[#0d1117] border border-[#21262d] flex items-start justify-between gap-3 text-xs font-mono"
                    >
                      <div className="space-y-1">
                        <div className="flex items-center space-x-2">
                          <span className="px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300 border border-[#30363d]">
                            {ev.category}
                          </span>
                          <span className="text-slate-200 font-semibold">{ev.description}</span>
                        </div>
                        {ev.value && (
                          <div className="text-[11px] text-slate-400 truncate max-w-md bg-black/30 px-2 py-1 rounded">
                            Value: {ev.value}
                          </div>
                        )}
                      </div>
                      <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 shrink-0">
                        +{ev.weight}% WEIGHT
                      </span>
                    </div>
                  ))
                ) : (
                  <p className="text-xs text-slate-500 font-mono">No granular evidence logs recorded.</p>
                )}
              </div>
            </div>

            {/* Services & Banners */}
            {selectedDetailDevice.fingerprint.services && (
              <div className="space-y-2">
                <h4 className="text-xs font-bold text-slate-300 font-mono uppercase tracking-wide flex items-center space-x-1.5">
                  <Server className="w-3.5 h-3.5 text-slate-400" />
                  <span>Detected Network Services & Protocol Banners</span>
                </h4>
                <div className="p-3 rounded-lg bg-[#0d1117] border border-[#21262d] space-y-1.5 text-xs font-mono">
                  {selectedDetailDevice.fingerprint.services.http_server && (
                    <div>
                      <span className="text-slate-500">HTTP Server Banner:</span>{' '}
                      <span className="text-slate-300">{selectedDetailDevice.fingerprint.services.http_server}</span>
                    </div>
                  )}
                  {selectedDetailDevice.fingerprint.services.rtsp_server && (
                    <div>
                      <span className="text-slate-500">RTSP Server Banner:</span>{' '}
                      <span className="text-slate-300">{selectedDetailDevice.fingerprint.services.rtsp_server}</span>
                    </div>
                  )}
                  {selectedDetailDevice.fingerprint.services.onvif_service_url && (
                    <div>
                      <span className="text-slate-500">ONVIF XAddr Service URL:</span>{' '}
                      <span className="text-emerald-400">{selectedDetailDevice.fingerprint.services.onvif_service_url}</span>
                    </div>
                  )}
                </div>
              </div>
            )}

            <div className="pt-3 border-t border-[#30363d] flex justify-end">
              <button
                onClick={() => setSelectedDetailDevice(null)}
                className="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-mono font-bold"
              >
                CLOSE INSPECTOR
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
