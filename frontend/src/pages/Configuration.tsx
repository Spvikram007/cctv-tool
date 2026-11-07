import React, { useState, useEffect } from 'react';
import { Device, NetworkConfig, ImageConfig, VideoEncodingConfig, ConfigPreview } from '../types';
import { api } from '../services/api';
import { Sliders, Network, Sun, CheckCircle, RotateCcw, AlertTriangle, RefreshCw } from 'lucide-react';

interface ConfigurationProps {
  devices: Device[];
  selectedDevice: Device | null;
  onSelectDevice: (dev: Device) => void;
}

export const Configuration: React.FC<ConfigurationProps> = ({
  devices,
  selectedDevice,
  onSelectDevice,
}) => {
  const currentDev = selectedDevice || devices[0];
  const [activeTab, setActiveTab] = useState<'network' | 'image' | 'video'>('network');
  const [loading, setLoading] = useState(false);

  // Staged states
  const [networkCfg, setNetworkCfg] = useState<NetworkConfig>({
    device_id: '',
    dhcp: false,
    ip_address: '',
    subnet_mask: '255.255.255.0',
    gateway: '',
    dns1: '8.8.8.8',
    dns2: '1.1.1.1',
    http_port: 80,
    rtsp_port: 554,
    onvif_port: 80,
    ntp_enabled: true,
    ntp_server: 'pool.ntp.org',
  });

  const [imageCfg, setImageCfg] = useState<ImageConfig>({
    device_id: '',
    brightness: 50,
    contrast: 50,
    saturation: 50,
    sharpness: 50,
    wdr_mode: 'auto',
    wdr_level: 50,
    ir_mode: 'auto',
    backlight_comp: true,
  });

  const [videoCfgs, setVideoCfgs] = useState<VideoEncodingConfig[]>([]);
  console.debug('Loaded video configs:', videoCfgs.length);

  // 12-Step flow state
  const [previewModal, setPreviewModal] = useState<ConfigPreview | null>(null);
  const [lastRollbackKey, setLastRollbackKey] = useState<string>('');
  const [applyMessage, setApplyMessage] = useState<string>('');
  const [isApplying, setIsApplying] = useState(false);

  useEffect(() => {
    if (currentDev) {
      loadDeviceConfigs(currentDev.id);
    }
  }, [currentDev?.id]);

  const loadDeviceConfigs = async (id: string) => {
    setLoading(true);
    try {
      const net = await api.getNetworkConfig(id);
      setNetworkCfg(net);
      const img = await api.getImageConfig(id);
      setImageCfg(img);
      const vids = await api.getVideoConfigs(id);
      setVideoCfgs(vids);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  // Step 5 & 6: Preview & Validation
  const handleGeneratePreview = async () => {
    if (!currentDev) return;
    try {
      if (activeTab === 'network') {
        const preview = await api.previewNetworkConfig(currentDev.id, networkCfg);
        setPreviewModal(preview);
      } else if (activeTab === 'image') {
        const preview = await api.previewImageConfig(currentDev.id, imageCfg);
        setPreviewModal(preview);
      }
    } catch (err: any) {
      alert(`Preview generation failed: ${err.message}`);
    }
  };

  // Step 8, 9, 10, 11: Apply Confirmed Changes
  const handleConfirmApply = async () => {
    if (!currentDev || !previewModal) return;
    setIsApplying(true);
    try {
      let res: any = null;
      if (previewModal.config_type === 'NETWORK') {
        res = await api.applyNetworkConfig(currentDev.id, previewModal.rollback_key, networkCfg);
      } else {
        res = await api.applyImageConfig(currentDev.id, previewModal.rollback_key, imageCfg);
      }
      setLastRollbackKey(previewModal.rollback_key);
      setApplyMessage(res.message);
      setPreviewModal(null);
      // Reload verified read-back
      await loadDeviceConfigs(currentDev.id);
    } catch (err: any) {
      alert(`Apply failed: ${err.message}`);
    } finally {
      setIsApplying(false);
    }
  };

  // Step 12: Rollback
  const handleRollback = async () => {
    if (!currentDev || !lastRollbackKey) return;
    if (confirm('Execute Step 12 rollback to previous configuration state?')) {
      try {
        const res = await fetch(`http://127.0.0.1:8080/api/v1/devices/${currentDev.id}/network/apply`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ rollback_key: lastRollbackKey, config: networkCfg }),
        });
        const data = await res.json();
        alert(data.message || 'Rollback executed');
        await loadDeviceConfigs(currentDev.id);
      } catch (err: any) {
        alert(`Rollback failed: ${err.message}`);
      }
    }
  };

  if (!currentDev) {
    return (
      <div className="p-8 text-center text-slate-500 font-mono">
        No devices available to configure. Please add or discover devices first.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Device Picker Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Sliders className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">UNIVERSAL 12-STEP CONFIGURATION ENGINE</h2>
            <p className="text-[11px] text-slate-400 font-mono">Preview, validate, verify read-back, audit and rollback</p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <label className="text-xs font-mono text-slate-400">TARGET UNIT:</label>
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
                {d.name} ({d.ip})
              </option>
            ))}
          </select>
        </div>
      </div>

      {applyMessage && (
        <div className="p-3 bg-emerald-950/60 border border-emerald-800 text-emerald-300 rounded-xl text-xs font-mono flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <CheckCircle className="w-4 h-4 text-emerald-400" />
            <span>{applyMessage}</span>
          </div>
          {lastRollbackKey && (
            <button
              onClick={handleRollback}
              className="px-2 py-1 bg-amber-600/20 hover:bg-amber-600/30 text-amber-300 border border-amber-600/40 rounded flex items-center space-x-1"
            >
              <RotateCcw className="w-3 h-3" />
              <span>ROLLBACK (STEP 12)</span>
            </button>
          )}
        </div>
      )}

      {/* Tabs */}
      <div className="flex space-x-2 border-b border-[#30363d] pb-2 text-xs font-mono">
        <button
          onClick={() => setActiveTab('network')}
          className={`flex items-center space-x-2 px-4 py-2 rounded-lg font-bold transition-all ${
            activeTab === 'network'
              ? 'bg-emerald-600/20 text-emerald-400 border border-emerald-500/30'
              : 'text-slate-400 hover:bg-[#161b22]'
          }`}
        >
          <Network className="w-4 h-4" />
          <span>NETWORK & IP INTERFACES</span>
        </button>

        <button
          onClick={() => setActiveTab('image')}
          className={`flex items-center space-x-2 px-4 py-2 rounded-lg font-bold transition-all ${
            activeTab === 'image'
              ? 'bg-emerald-600/20 text-emerald-400 border border-emerald-500/30'
              : 'text-slate-400 hover:bg-[#161b22]'
          }`}
        >
          <Sun className="w-4 h-4" />
          <span>IMAGE & SENSOR PARAMETERS</span>
        </button>
      </div>

      {/* Tab Contents */}
      {loading ? (
        <div className="p-12 text-center text-slate-400 font-mono text-xs flex items-center justify-center space-x-2">
          <RefreshCw className="w-4 h-4 animate-spin text-emerald-400" />
          <span>Synchronizing settings from hardware adapter...</span>
        </div>
      ) : activeTab === 'network' ? (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-mono text-slate-400 mb-1">ADDRESSING MODE</label>
              <div className="flex items-center space-x-4 mt-2">
                <label className="flex items-center space-x-2 text-xs font-mono text-slate-300">
                  <input
                    type="radio"
                    name="dhcp"
                    checked={!networkCfg.dhcp}
                    onChange={() => setNetworkCfg({ ...networkCfg, dhcp: false })}
                    className="accent-emerald-500"
                  />
                  <span>STATIC IP (RECOMMENDED)</span>
                </label>
                <label className="flex items-center space-x-2 text-xs font-mono text-slate-300">
                  <input
                    type="radio"
                    name="dhcp"
                    checked={networkCfg.dhcp}
                    onChange={() => setNetworkCfg({ ...networkCfg, dhcp: true })}
                    className="accent-emerald-500"
                  />
                  <span>DHCP DYNAMIC</span>
                </label>
              </div>
            </div>

            <div>
              <label className="block text-xs font-mono text-slate-400 mb-1">IP ADDRESS</label>
              <input
                type="text"
                disabled={networkCfg.dhcp}
                value={networkCfg.ip_address}
                onChange={(e) => setNetworkCfg({ ...networkCfg, ip_address: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500 disabled:opacity-50"
              />
            </div>

            <div>
              <label className="block text-xs font-mono text-slate-400 mb-1">SUBNET MASK</label>
              <input
                type="text"
                disabled={networkCfg.dhcp}
                value={networkCfg.subnet_mask}
                onChange={(e) => setNetworkCfg({ ...networkCfg, subnet_mask: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500 disabled:opacity-50"
              />
            </div>

            <div>
              <label className="block text-xs font-mono text-slate-400 mb-1">DEFAULT GATEWAY</label>
              <input
                type="text"
                disabled={networkCfg.dhcp}
                value={networkCfg.gateway}
                onChange={(e) => setNetworkCfg({ ...networkCfg, gateway: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500 disabled:opacity-50"
              />
            </div>

            <div>
              <label className="block text-xs font-mono text-slate-400 mb-1">PRIMARY DNS</label>
              <input
                type="text"
                value={networkCfg.dns1}
                onChange={(e) => setNetworkCfg({ ...networkCfg, dns1: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs font-mono text-slate-400 mb-1">NTP TIME SERVER</label>
              <input
                type="text"
                value={networkCfg.ntp_server}
                onChange={(e) => setNetworkCfg({ ...networkCfg, ntp_server: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
              />
            </div>
          </div>

          <div className="flex justify-end pt-4 border-t border-[#30363d]">
            <button
              onClick={handleGeneratePreview}
              className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-mono font-bold shadow-lg shadow-emerald-900/30 transition-all"
            >
              STAGE & PREVIEW CHANGES (STEP 5 & 6)
            </button>
          </div>
        </div>
      ) : (
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div>
              <div className="flex justify-between text-xs font-mono text-slate-400 mb-1">
                <span>BRIGHTNESS ({imageCfg.brightness}%)</span>
              </div>
              <input
                type="range"
                min="0"
                max="100"
                value={imageCfg.brightness}
                onChange={(e) => setImageCfg({ ...imageCfg, brightness: parseInt(e.target.value) })}
                className="w-full accent-emerald-500"
              />
            </div>

            <div>
              <div className="flex justify-between text-xs font-mono text-slate-400 mb-1">
                <span>CONTRAST ({imageCfg.contrast}%)</span>
              </div>
              <input
                type="range"
                min="0"
                max="100"
                value={imageCfg.contrast}
                onChange={(e) => setImageCfg({ ...imageCfg, contrast: parseInt(e.target.value) })}
                className="w-full accent-emerald-500"
              />
            </div>

            <div>
              <div className="flex justify-between text-xs font-mono text-slate-400 mb-1">
                <span>SATURATION ({imageCfg.saturation}%)</span>
              </div>
              <input
                type="range"
                min="0"
                max="100"
                value={imageCfg.saturation}
                onChange={(e) => setImageCfg({ ...imageCfg, saturation: parseInt(e.target.value) })}
                className="w-full accent-emerald-500"
              />
            </div>

            <div>
              <div className="flex justify-between text-xs font-mono text-slate-400 mb-1">
                <span>SHARPNESS ({imageCfg.sharpness}%)</span>
              </div>
              <input
                type="range"
                min="0"
                max="100"
                value={imageCfg.sharpness}
                onChange={(e) => setImageCfg({ ...imageCfg, sharpness: parseInt(e.target.value) })}
                className="w-full accent-emerald-500"
              />
            </div>
          </div>

          <div className="flex justify-end pt-4 border-t border-[#30363d]">
            <button
              onClick={handleGeneratePreview}
              className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-mono font-bold shadow-lg shadow-emerald-900/30 transition-all"
            >
              STAGE & PREVIEW CHANGES (STEP 5 & 6)
            </button>
          </div>
        </div>
      )}

      {/* Step 6 Preview Diff Modal */}
      {previewModal && (
        <div className="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-xl rounded-2xl bg-[#161b22] border border-[#30363d] shadow-2xl p-6 space-y-4 font-mono">
            <div className="flex items-center justify-between border-b border-[#30363d] pb-3">
              <div className="flex items-center space-x-2">
                <AlertTriangle className="w-5 h-5 text-amber-400" />
                <h3 className="text-sm font-bold text-slate-100 uppercase">
                  STEP 6: CONFIGURATION PREVIEW & DIFF CONFIRMATION
                </h3>
              </div>
              <button
                onClick={() => setPreviewModal(null)}
                className="text-slate-400 hover:text-slate-200 text-sm"
              >
                ✕
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div>
                <span className="text-slate-400">TARGET:</span>{' '}
                <span className="text-slate-200 font-bold">{previewModal.device_name}</span>
              </div>

              <div>
                <span className="text-slate-400">PARAMETER DIFF:</span>
                <div className="mt-1 p-3 rounded-lg bg-[#0d1117] border border-[#30363d] text-emerald-400 font-bold">
                  {previewModal.diff}
                </div>
              </div>

              {previewModal.validation_errors && previewModal.validation_errors.length > 0 ? (
                <div className="p-3 bg-red-950/40 border border-red-800 text-red-300 rounded-lg space-y-1">
                  <div className="font-bold">VALIDATION BLOCKED:</div>
                  {previewModal.validation_errors.map((err, i) => (
                    <div key={i}>• {err}</div>
                  ))}
                </div>
              ) : (
                <div className="p-3 bg-emerald-950/30 border border-emerald-800/40 text-emerald-300 rounded-lg flex items-center space-x-2">
                  <CheckCircle className="w-4 h-4 text-emerald-400" />
                  <span>Proposed parameters passed syntactic and hardware validation checks.</span>
                </div>
              )}
            </div>

            <div className="pt-3 border-t border-[#30363d] flex justify-end space-x-3 text-xs">
              <button
                onClick={() => setPreviewModal(null)}
                className="px-4 py-2 rounded-lg bg-[#21262d] text-slate-300 hover:bg-[#30363d]"
              >
                CANCEL
              </button>
              <button
                disabled={!previewModal.valid || isApplying}
                onClick={handleConfirmApply}
                className={`px-5 py-2 rounded-lg font-bold flex items-center space-x-2 ${
                  previewModal.valid && !isApplying
                    ? 'bg-emerald-600 hover:bg-emerald-500 text-white'
                    : 'bg-slate-700 text-slate-500 cursor-not-allowed'
                }`}
              >
                {isApplying && <RefreshCw className="w-3.5 h-3.5 animate-spin" />}
                <span>CONFIRM & APPLY TO HARDWARE (STEP 7 & 8)</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
