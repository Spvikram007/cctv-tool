import React, { useState, useEffect } from 'react';
import { Device, RecorderChannel } from '../types';
import { api } from '../services/api';
import { Server, HardDrive, Cpu, CheckCircle } from 'lucide-react';

interface RecorderManagerProps {
  devices: Device[];
  selectedDevice: Device | null;
  onSelectDevice: (dev: Device) => void;
}

export const RecorderManager: React.FC<RecorderManagerProps> = ({
  devices,
  selectedDevice,
  onSelectDevice,
}) => {
  const recorders = devices.filter((d) => d.category === 'XVR' || d.category === 'DVR' || d.category === 'NVR' || d.channel_count > 1);
  const currentDev = selectedDevice && recorders.some((r) => r.id === selectedDevice.id)
    ? selectedDevice
    : recorders[0] || devices[0];

  const [channels, setChannels] = useState<RecorderChannel[]>([]);
  const [, setLoading] = useState(false);
  const [statusMsg, setStatusMsg] = useState('');

  useEffect(() => {
    if (currentDev) {
      loadChannels(currentDev.id);
    }
  }, [currentDev?.id]);

  const loadChannels = async (id: string) => {
    setLoading(true);
    try {
      const data = await api.getChannels(id);
      setChannels(data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  const handleFormatChange = async (channelId: number, format: string) => {
    if (!currentDev) return;
    try {
      await api.setAnalogFormat(currentDev.id, channelId, format);
      setStatusMsg(`Channel ${channelId} format switched to ${format} via coaxial UTC.`);
      await loadChannels(currentDev.id);
      setTimeout(() => setStatusMsg(''), 4000);
    } catch (err: any) {
      alert(`Format switch failed: ${err.message}`);
    }
  };

  if (!currentDev) {
    return (
      <div className="p-8 text-center text-slate-500 font-mono">
        No hybrid DVR/NVR/XVR recorder devices available.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Server className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">HYBRID RECORDER & XVR ANALOG MANAGER</h2>
            <p className="text-[11px] text-slate-400 font-mono">TVI, CVI, AHD, CVBS Coaxial UTC Format Switching & Channel Mapping</p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <label className="text-xs font-mono text-slate-400">RECORDER UNIT:</label>
          <select
            value={currentDev.id}
            onChange={(e) => {
              const d = devices.find((x) => x.id === e.target.value);
              if (d) onSelectDevice(d);
            }}
            className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
          >
            {devices.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name} ({d.category})
              </option>
            ))}
          </select>
        </div>
      </div>

      {statusMsg && (
        <div className="p-3 bg-emerald-950/60 border border-emerald-800 text-emerald-300 rounded-xl text-xs font-mono flex items-center space-x-2">
          <CheckCircle className="w-4 h-4 text-emerald-400" />
          <span>{statusMsg}</span>
        </div>
      )}

      {/* Storage and System Info */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center space-x-3 font-mono">
          <HardDrive className="w-6 h-6 text-emerald-400" />
          <div>
            <p className="text-[10px] text-slate-400 uppercase">INTERNAL STORAGE</p>
            <p className="text-sm font-bold text-slate-100">1x 4TB Surveillance HDD</p>
            <p className="text-[11px] text-emerald-400">SMART Health: NORMAL (38°C)</p>
          </div>
        </div>

        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center space-x-3 font-mono">
          <Cpu className="w-6 h-6 text-purple-400" />
          <div>
            <p className="text-[10px] text-slate-400 uppercase">ENCODING CAPACITY</p>
            <p className="text-sm font-bold text-slate-100">8CH 4K / 5MP-N Hybrid</p>
            <p className="text-[11px] text-purple-400">AI SMD Plus Enabled</p>
          </div>
        </div>

        <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center space-x-3 font-mono">
          <Server className="w-6 h-6 text-blue-400" />
          <div>
            <p className="text-[10px] text-slate-400 uppercase">COAXIAL PROTOCOL</p>
            <p className="text-sm font-bold text-slate-100">HD-over-Coax UTC</p>
            <p className="text-[11px] text-blue-400">Audio Over Coax Active</p>
          </div>
        </div>
      </div>

      {/* Channel Mapping Table */}
      <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
        <div className="p-4 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between font-mono text-xs">
          <span className="font-bold text-slate-200 uppercase">CHANNEL MATRIX & ANALOG SIGNAL MODES</span>
          <span className="text-slate-400">TOTAL CHANNELS: {channels.length}</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs font-mono">
            <thead className="bg-[#0d1117]/60 text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
              <tr>
                <th className="py-3 px-4">Channel</th>
                <th className="py-3 px-4">Camera Label</th>
                <th className="py-3 px-4">Interface</th>
                <th className="py-3 px-4">Active Signal Format</th>
                <th className="py-3 px-4">Resolution</th>
                <th className="py-3 px-4">Record Mode</th>
                <th className="py-3 px-4 text-right">Switch Format (UTC)</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#21262d]">
              {channels.map((ch) => (
                <tr key={ch.channel_id} className="hover:bg-[#21262d]/50 transition-colors">
                  <td className="py-3 px-4 font-bold text-slate-200">CH {ch.channel_id}</td>
                  <td className="py-3 px-4 text-slate-300">{ch.name}</td>
                  <td className="py-3 px-4">
                    <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                      ch.type === 'IP' ? 'bg-blue-500/20 text-blue-400' : 'bg-emerald-500/20 text-emerald-400'
                    }`}>
                      {ch.type}
                    </span>
                  </td>
                  <td className="py-3 px-4 font-bold text-amber-400">{ch.signal_format}</td>
                  <td className="py-3 px-4 text-slate-400">{ch.resolution}</td>
                  <td className="py-3 px-4 text-slate-400 uppercase">{ch.record_mode}</td>
                  <td className="py-3 px-4 text-right">
                    {ch.type === 'ANALOG' ? (
                      <div className="flex items-center justify-end space-x-1">
                        {['TVI', 'CVI', 'AHD', 'CVBS'].map((fmt) => (
                          <button
                            key={fmt}
                            onClick={() => handleFormatChange(ch.channel_id, fmt)}
                            className={`px-2 py-0.5 rounded text-[10px] font-bold border transition-colors ${
                              ch.signal_format === fmt
                                ? 'bg-emerald-600 text-white border-emerald-500'
                                : 'bg-[#0d1117] text-slate-400 border-slate-700 hover:text-slate-200 hover:bg-slate-800'
                            }`}
                          >
                            {fmt}
                          </button>
                        ))}
                      </div>
                    ) : (
                      <span className="text-[10px] text-slate-500">DIGITAL STREAM</span>
                    )}
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
