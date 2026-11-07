import React, { useState } from 'react';
import { api } from '../services/api';
import { SubnetCalc, HDDCalc, BandwidthCalc } from '../types';
import { Calculator } from 'lucide-react';

export const Calculators: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'subnet' | 'hdd' | 'bandwidth'>('subnet');

  // Subnet state
  const [cidr, setCidr] = useState('192.168.1.0/24');
  const [subnetResult, setSubnetResult] = useState<SubnetCalc | null>(null);

  // HDD state
  const [hddCams, setHddCams] = useState(8);
  const [resolution, setResolution] = useState('1080P');
  const [codec, setCodec] = useState('H.265');
  const [days, setDays] = useState(30);
  const [hddResult, setHddResult] = useState<HDDCalc | null>(null);

  // Bandwidth state
  const [bwCams, setBwCams] = useState(8);
  const [mainKbps, setMainKbps] = useState(4096);
  const [subKbps, setSubKbps] = useState(1024);
  const [bwResult, setBwResult] = useState<BandwidthCalc | null>(null);

  const handleSubnetCalc = async () => {
    try {
      const res = await api.calcSubnet(cidr);
      setSubnetResult(res);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleHDDCalc = async () => {
    try {
      const res = await api.calcHDD(hddCams, resolution, codec, days);
      setHddResult(res);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleBandwidthCalc = async () => {
    try {
      const res = await api.calcBandwidth(bwCams, mainKbps, subKbps);
      setBwResult(res);
    } catch (err: any) {
      alert(err.message);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex items-center justify-between">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Calculator className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">ENGINEERING & DESIGN CALCULATORS</h2>
            <p className="text-[11px] text-slate-400 font-mono">Precision CCTV Subnetting, Storage Capacity & Switch Throughput Calculations</p>
          </div>
        </div>

        {/* Tab switcher */}
        <div className="flex bg-[#0d1117] border border-[#30363d] rounded-lg p-1 text-xs font-mono">
          <button
            onClick={() => setActiveTab('subnet')}
            className={`px-3 py-1 rounded font-bold ${activeTab === 'subnet' ? 'bg-emerald-600 text-white' : 'text-slate-400'}`}
          >
            IP / SUBNET
          </button>
          <button
            onClick={() => setActiveTab('hdd')}
            className={`px-3 py-1 rounded font-bold ${activeTab === 'hdd' ? 'bg-emerald-600 text-white' : 'text-slate-400'}`}
          >
            HDD STORAGE
          </button>
          <button
            onClick={() => setActiveTab('bandwidth')}
            className={`px-3 py-1 rounded font-bold ${activeTab === 'bandwidth' ? 'bg-emerald-600 text-white' : 'text-slate-400'}`}
          >
            BANDWIDTH & SWITCH
          </button>
        </div>
      </div>

      {/* Subnet Calculator View */}
      {activeTab === 'subnet' && (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 font-mono">
          <div className="max-w-xl space-y-3">
            <label className="block text-xs text-slate-400">INPUT CIDR BLOCK</label>
            <div className="flex space-x-3">
              <input
                type="text"
                value={cidr}
                onChange={(e) => setCidr(e.target.value)}
                placeholder="192.168.10.0/24"
                className="flex-1 px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
              />
              <button
                onClick={handleSubnetCalc}
                className="px-5 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold transition-colors"
              >
                CALCULATE
              </button>
            </div>
          </div>

          {subnetResult && (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 pt-4 border-t border-[#30363d]">
              <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">NETWORK ADDRESS</span>
                <p className="text-sm font-bold text-slate-100 mt-1">{subnetResult.network_address}</p>
              </div>
              <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">BROADCAST ADDRESS</span>
                <p className="text-sm font-bold text-slate-100 mt-1">{subnetResult.broadcast_address}</p>
              </div>
              <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">NETMASK</span>
                <p className="text-sm font-bold text-slate-100 mt-1">{subnetResult.netmask}</p>
              </div>
              <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">USABLE CCTV HOSTS</span>
                <p className="text-sm font-bold text-emerald-400 mt-1">{subnetResult.usable_hosts} Hosts</p>
                <p className="text-[10px] text-slate-500">{subnetResult.first_usable_ip} - {subnetResult.last_usable_ip}</p>
              </div>
            </div>
          )}
        </div>
      )}

      {/* HDD Storage Calculator View */}
      {activeTab === 'hdd' && (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 font-mono">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div>
              <label className="block text-xs text-slate-400 mb-1">CAMERA COUNT</label>
              <input
                type="number"
                value={hddCams}
                onChange={(e) => setHddCams(parseInt(e.target.value))}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">STREAM RESOLUTION</label>
              <select
                value={resolution}
                onChange={(e) => setResolution(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="1080P">1080P (2MP)</option>
                <option value="4MP">4MP (2560x1440)</option>
                <option value="4K">4K (8MP / 3840x2160)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">VIDEO CODEC</label>
              <select
                value={codec}
                onChange={(e) => setCodec(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="H.265">H.265 (High Efficiency)</option>
                <option value="H.264">H.264 (Standard)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">RECORDING RETENTION (DAYS)</label>
              <input
                type="number"
                value={days}
                onChange={(e) => setDays(parseInt(e.target.value))}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>
          </div>

          <button
            onClick={handleHDDCalc}
            className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold transition-colors"
          >
            COMPUTE HDD REQUIREMENTS
          </button>

          {hddResult && (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-4 border-t border-[#30363d]">
              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">TOTAL RAW STORAGE REQUIRED</span>
                <p className="text-2xl font-bold text-emerald-400 mt-1">{hddResult.total_storage_tb} TB</p>
                <p className="text-[11px] text-slate-400 mt-0.5">({hddResult.total_storage_gb} GB)</p>
              </div>

              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">RECOMMENDED 4TB SURVEILLANCE DRIVES</span>
                <p className="text-2xl font-bold text-slate-100 mt-1">{hddResult.recommended_hdds} Disks</p>
                <p className="text-[11px] text-emerald-400 mt-0.5">WD Purple or Seagate SkyHawk</p>
              </div>

              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">BITRATE BUDGET PER CHANNEL</span>
                <p className="text-2xl font-bold text-slate-100 mt-1">{hddResult.bitrate_kbps} Kbps</p>
                <p className="text-[11px] text-slate-400 mt-0.5">{hddResult.codec} Optimized Stream</p>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Bandwidth Calculator View */}
      {activeTab === 'bandwidth' && (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 font-mono">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label className="block text-xs text-slate-400 mb-1">CONCURRENT CAMERAS</label>
              <input
                type="number"
                value={bwCams}
                onChange={(e) => setBwCams(parseInt(e.target.value))}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">MAINSTREAM BITRATE (KBPS)</label>
              <input
                type="number"
                value={mainKbps}
                onChange={(e) => setMainKbps(parseInt(e.target.value))}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">SUBSTREAM BITRATE (KBPS)</label>
              <input
                type="number"
                value={subKbps}
                onChange={(e) => setSubKbps(parseInt(e.target.value))}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>
          </div>

          <button
            onClick={handleBandwidthCalc}
            className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold transition-colors"
          >
            COMPUTE SWITCH BANDWIDTH
          </button>

          {bwResult && (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-4 border-t border-[#30363d]">
              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">TOTAL COMBINED THROUGHPUT</span>
                <p className="text-2xl font-bold text-emerald-400 mt-1">{bwResult.total_combined_mbps} Mbps</p>
                <p className="text-[11px] text-slate-400 mt-0.5">Main: {bwResult.total_main_mbps}M | Sub: {bwResult.total_sub_mbps}M</p>
              </div>

              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">RECOMMENDED SWITCH UPLINK</span>
                <p className="text-xl font-bold text-slate-100 mt-1">{bwResult.recommended_switch_port}</p>
                <p className="text-[11px] text-emerald-400 mt-0.5">Managed PoE Switch Spec</p>
              </div>

              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">NETWORK OVERHEAD SAFETY BUFFER</span>
                <p className="text-xl font-bold text-slate-100 mt-1">+20% Headroom</p>
                <p className="text-[11px] text-slate-400 mt-0.5">Prevents RTSP packet drop</p>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
