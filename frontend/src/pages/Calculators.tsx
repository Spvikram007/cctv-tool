import React, { useState } from 'react';
import { api } from '../services/api';
import { SubnetCalc, HDDCalc, BandwidthCalc, CapacityBandwidthCalculation } from '../types';
import { Calculator, HardDrive, Network, Server, Cpu } from 'lucide-react';

export const Calculators: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'capacity' | 'subnet' | 'hdd' | 'bandwidth'>('capacity');

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

  // Unified Capacity & Bandwidth state (Phase 4)
  const [capCams, setCapCams] = useState(16);
  const [capResolution, setCapResolution] = useState('1080P');
  const [capCodec, setCapCodec] = useState('H.265');
  const [capFps, setCapFps] = useState(25);
  const [capDays, setCapDays] = useState(30);
  const [capHours, setCapHours] = useState(24);
  const [capViewers, setCapViewers] = useState(4);
  const [capResult, setCapResult] = useState<CapacityBandwidthCalculation | null>(null);
  const [capLoading, setCapLoading] = useState(false);
  const [calcError, setCalcError] = useState<string | null>(null);

  const handleSubnetCalc = async () => {
    setCalcError(null);
    try {
      const res = await api.calcSubnet(cidr);
      setSubnetResult(res);
    } catch (err: any) {
      setCalcError(err.message);
    }
  };

  const handleHDDCalc = async () => {
    setCalcError(null);
    try {
      const res = await api.calcHDD(hddCams, resolution, codec, days);
      setHddResult(res);
    } catch (err: any) {
      setCalcError(err.message);
    }
  };

  const handleBandwidthCalc = async () => {
    setCalcError(null);
    try {
      const res = await api.calcBandwidth(bwCams, mainKbps, subKbps);
      setBwResult(res);
    } catch (err: any) {
      setCalcError(err.message);
    }
  };

  const handleCapacityCalc = async () => {
    setCalcError(null);
    setCapLoading(true);
    try {
      const res = await api.calcCapacity({
        cameras: capCams,
        resolution: capResolution,
        codec: capCodec,
        fps: capFps,
        days: capDays,
        hours: capHours,
        viewers: capViewers,
      });
      setCapResult(res);
    } catch (err: any) {
      setCalcError(err.message);
    } finally {
      setCapLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Header Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Calculator className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">ENGINEERING & CAPACITY CALCULATORS</h2>
            <p className="text-[11px] text-slate-400 font-mono">
              Bandwidth, Storage Capacity, Switch Headroom & Subnet Sizing for Enterprise CCTV
            </p>
          </div>
        </div>

        {/* Tab switcher */}
        <div className="flex bg-[#0d1117] border border-[#30363d] rounded-lg p-1 text-xs font-mono">
          <button
            onClick={() => setActiveTab('capacity')}
            className={`px-3 py-1 rounded font-bold transition-colors ${
              activeTab === 'capacity' ? 'bg-emerald-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            CAPACITY & 30% HEADROOM
          </button>
          <button
            onClick={() => setActiveTab('subnet')}
            className={`px-3 py-1 rounded font-bold transition-colors ${
              activeTab === 'subnet' ? 'bg-emerald-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            IP / SUBNET
          </button>
          <button
            onClick={() => setActiveTab('hdd')}
            className={`px-3 py-1 rounded font-bold transition-colors ${
              activeTab === 'hdd' ? 'bg-emerald-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            HDD STORAGE
          </button>
          <button
            onClick={() => setActiveTab('bandwidth')}
            className={`px-3 py-1 rounded font-bold transition-colors ${
              activeTab === 'bandwidth' ? 'bg-emerald-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            BANDWIDTH & SWITCH
          </button>
        </div>
      </div>

      {calcError && (
        <div className="p-3 bg-rose-950/40 border border-rose-800/80 rounded-xl text-xs font-mono text-rose-300 flex items-center justify-between">
          <span>{calcError}</span>
          <button
            onClick={() => setCalcError(null)}
            className="text-slate-400 hover:text-white px-2 py-0.5 rounded text-[11px]"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Unified Capacity & Bandwidth Calculator View */}
      {activeTab === 'capacity' && (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 font-mono">
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <label className="block text-xs text-slate-400 mb-1">CAMERA COUNT</label>
              <input
                type="number"
                min="1"
                max="512"
                value={capCams}
                onChange={(e) => setCapCams(parseInt(e.target.value) || 1)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">RESOLUTION</label>
              <select
                value={capResolution}
                onChange={(e) => setCapResolution(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="720P">720P (1.0 MP)</option>
                <option value="1080P">1080P Full HD (2.0 MP)</option>
                <option value="4MP">4MP (2K QHD)</option>
                <option value="4K">4K UHD (8.0 MP)</option>
                <option value="8K">8K UHD (32.0 MP)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">CODEC COMPRESSION</label>
              <select
                value={capCodec}
                onChange={(e) => setCapCodec(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="H.264">H.264 / AVC</option>
                <option value="H.265">H.265 / HEVC (-40% bitrate)</option>
                <option value="H.265+">H.265+ / Smart Codec (-60% bitrate)</option>
                <option value="MJPEG">MJPEG (+150% bitrate)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">FRAME RATE (FPS)</label>
              <select
                value={capFps}
                onChange={(e) => setCapFps(parseInt(e.target.value) || 25)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="15">15 FPS (Smooth Economy)</option>
                <option value="20">20 FPS (Surveillance Standard)</option>
                <option value="25">25 FPS (PAL Realtime)</option>
                <option value="30">30 FPS (NTSC Realtime)</option>
              </select>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label className="block text-xs text-slate-400 mb-1">RETENTION PERIOD (DAYS)</label>
              <input
                type="number"
                min="1"
                max="365"
                value={capDays}
                onChange={(e) => setCapDays(parseInt(e.target.value) || 30)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">RECORDING HOURS / DAY</label>
              <input
                type="number"
                min="1"
                max="24"
                value={capHours}
                onChange={(e) => setCapHours(parseInt(e.target.value) || 24)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">SIMULTANEOUS LIVE VIEWERS</label>
              <input
                type="number"
                min="0"
                max="64"
                value={capViewers}
                onChange={(e) => setCapViewers(parseInt(e.target.value) || 0)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>
          </div>

          <button
            onClick={handleCapacityCalc}
            disabled={capLoading}
            className="px-6 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold transition-colors"
          >
            {capLoading ? 'COMPUTING...' : 'COMPUTE CAPACITY & SWITCH REQUIREMENTS'}
          </button>

          {capResult && (
            <div className="space-y-4 pt-4 border-t border-[#30363d]">
              <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
                <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                  <div className="flex items-center space-x-2 text-slate-400 text-[10px] uppercase font-bold">
                    <HardDrive className="w-3.5 h-3.5 text-blue-400" />
                    <span>STORAGE NEEDED</span>
                  </div>
                  <p className="text-2xl font-bold text-emerald-400 mt-1">{capResult.total_storage_tb} TB</p>
                  <p className="text-[11px] text-slate-400 mt-0.5">{capResult.total_storage_gb.toLocaleString()} GB Total</p>
                </div>

                <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                  <div className="flex items-center space-x-2 text-slate-400 text-[10px] uppercase font-bold">
                    <Server className="w-3.5 h-3.5 text-purple-400" />
                    <span>RECOMMENDED HDDS</span>
                  </div>
                  <p className="text-2xl font-bold text-slate-100 mt-1">{capResult.recommended_hdds} × 4TB Disks</p>
                  <p className="text-[11px] text-emerald-400 mt-0.5">Surveillance Grade (WD Purple / SkyHawk)</p>
                </div>

                <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                  <div className="flex items-center space-x-2 text-slate-400 text-[10px] uppercase font-bold">
                    <Network className="w-3.5 h-3.5 text-amber-400" />
                    <span>TOTAL NETWORK TRAFFIC</span>
                  </div>
                  <p className="text-2xl font-bold text-slate-100 mt-1">{capResult.total_network_mbps} Mbps</p>
                  <p className="text-[11px] text-slate-400 mt-0.5">
                    Rec: {capResult.aggregate_camera_mbps}M | View: {capResult.viewing_bandwidth_mbps}M
                  </p>
                </div>

                <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                  <div className="flex items-center space-x-2 text-slate-400 text-[10px] uppercase font-bold">
                    <Cpu className="w-3.5 h-3.5 text-emerald-400" />
                    <span>SWITCH HEADROOM (+30%)</span>
                  </div>
                  <p className="text-2xl font-bold text-emerald-400 mt-1">{capResult.recommended_headroom_mbps} Mbps</p>
                  <p className="text-[11px] text-slate-300 mt-0.5 font-bold">{capResult.recommended_switch_port}</p>
                </div>
              </div>

              <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg flex items-center justify-between text-[11px] text-slate-400">
                <span>Model Basis: {capResult.calculation_basis}</span>
                <span>Main Bitrate: {capResult.main_bitrate_kbps} kbps/ch | Sub Bitrate: {capResult.sub_bitrate_kbps} kbps/ch</span>
              </div>
            </div>
          )}
        </div>
      )}

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
                className="flex-1 px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
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
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-4 border-t border-[#30363d]">
              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">NETWORK ADDRESS</span>
                <p className="text-lg font-bold text-slate-100 mt-1">{subnetResult.network_address}</p>
                <p className="text-[11px] text-slate-400 mt-0.5">Mask: {subnetResult.netmask}</p>
              </div>

              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">USABLE IP RANGE</span>
                <p className="text-xs font-bold text-emerald-400 mt-1">{subnetResult.first_usable_ip} -</p>
                <p className="text-xs font-bold text-emerald-400">{subnetResult.last_usable_ip}</p>
                <p className="text-[11px] text-slate-400 mt-0.5">Broadcast: {subnetResult.broadcast_address}</p>
              </div>

              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">MAX ALLOCATABLE CAMERAS</span>
                <p className="text-2xl font-bold text-slate-100 mt-1">{subnetResult.usable_hosts}</p>
                <p className="text-[11px] text-emerald-400 mt-0.5">Total Hosts: {subnetResult.total_hosts}</p>
              </div>
            </div>
          )}
        </div>
      )}

      {/* HDD Storage Calculator View */}
      {activeTab === 'hdd' && (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 font-mono">
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <label className="block text-xs text-slate-400 mb-1">TOTAL CAMERAS</label>
              <input
                type="number"
                value={hddCams}
                onChange={(e) => setHddCams(parseInt(e.target.value) || 1)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">RESOLUTION</label>
              <select
                value={resolution}
                onChange={(e) => setResolution(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="1080P">1080P (2MP)</option>
                <option value="4MP">4MP (2K)</option>
                <option value="4K">4K (8MP)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">CODEC</label>
              <select
                value={codec}
                onChange={(e) => setCodec(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              >
                <option value="H.264">H.264</option>
                <option value="H.265">H.265 (HEVC)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">DAYS TO RETAIN</label>
              <input
                type="number"
                value={days}
                onChange={(e) => setDays(parseInt(e.target.value) || 30)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>
          </div>

          <button
            onClick={handleHDDCalc}
            className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold transition-colors"
          >
            COMPUTE HDD CAPACITY
          </button>

          {hddResult && (
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-4 border-t border-[#30363d]">
              <div className="p-4 bg-[#0d1117] border border-[#30363d] rounded-lg">
                <span className="text-[10px] text-slate-400">TOTAL STORAGE REQUIRED</span>
                <p className="text-2xl font-bold text-emerald-400 mt-1">{hddResult.total_storage_tb} TB</p>
                <p className="text-[11px] text-slate-400 mt-0.5">{hddResult.total_storage_gb} GB Total</p>
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
                onChange={(e) => setBwCams(parseInt(e.target.value) || 1)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">MAINSTREAM BITRATE (KBPS)</label>
              <input
                type="number"
                value={mainKbps}
                onChange={(e) => setMainKbps(parseInt(e.target.value) || 4096)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-400 mb-1">SUBSTREAM BITRATE (KBPS)</label>
              <input
                type="number"
                value={subKbps}
                onChange={(e) => setSubKbps(parseInt(e.target.value) || 1024)}
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
