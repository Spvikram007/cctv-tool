import React, { useState, useEffect } from 'react';
import { Device, NetworkConfig, ImageConfig, VideoEncodingConfig, ConfigPreview, DeviceCapabilities, ConfigResult } from '../types';
import { api } from '../services/api';
import { Sliders, Network, Sun, CheckCircle, RotateCcw, AlertTriangle, ShieldCheck, Cpu } from 'lucide-react';

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
  const [capabilities, setCapabilities] = useState<DeviceCapabilities | null>(null);

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

  const [, setVideoCfgs] = useState<VideoEncodingConfig[]>([]);

  // 10-Step Safety Engine state
  const [previewModal, setPreviewModal] = useState<ConfigPreview | null>(null);
  const [lastResult, setLastResult] = useState<ConfigResult | null>(null);
  const [lastRollbackKey, setLastRollbackKey] = useState<string>('');
  const [isApplying, setIsApplying] = useState(false);

  useEffect(() => {
    if (currentDev) {
      loadDeviceConfigs(currentDev.id);
      loadCapabilities(currentDev.id);
    }
  }, [currentDev?.id]);

  const loadCapabilities = async (id: string) => {
    try {
      const caps = await api.getCapabilities(id);
      setCapabilities(caps);
    } catch (e) {
      console.warn('Failed to fetch capabilities:', e);
      setCapabilities(null);
    }
  };

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

  // Steps 1 to 6: Generate Preview & Structural Diff with Staged Token
  const handleGeneratePreview = async () => {
    if (!currentDev) return;
    try {
      if (activeTab === 'network') {
        const preview = await api.previewConfig(currentDev.id, 'NETWORK', networkCfg);
        setPreviewModal(preview);
      } else if (activeTab === 'image') {
        const preview = await api.previewConfig(currentDev.id, 'IMAGE', imageCfg);
        setPreviewModal(preview);
      }
    } catch (err: any) {
      alert(`Preview generation failed: ${err.message}`);
    }
  };

  // Steps 7 to 10: Explicit Approval -> Apply -> Read-back Verification -> Audit
  const handleConfirmApply = async () => {
    if (!currentDev || !previewModal) return;
    setIsApplying(true);
    try {
      const res = await api.applyConfig(
        currentDev.id,
        previewModal.config_type,
        previewModal.preview_token,
        previewModal.rollback_key
      );
      setLastResult(res);
      if (res.rollback_key) {
        setLastRollbackKey(res.rollback_key);
      }
      setPreviewModal(null);
      // Reload verified read-back from physical device
      await loadDeviceConfigs(currentDev.id);
    } catch (err: any) {
      alert(`Apply failed: ${err.message}`);
    } finally {
      setIsApplying(false);
    }
  };

  // Step 10: Rollback to previous configuration state
  const handleRollback = async () => {
    if (!currentDev || !lastRollbackKey) return;
    if (confirm('Execute Rollback to previous configuration snapshot?')) {
      try {
        const res = await api.rollbackConfig(currentDev.id, lastRollbackKey);
        setLastResult(res);
        setLastRollbackKey('');
        await loadDeviceConfigs(currentDev.id);
        alert(res.message);
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

  const renderCapBadge = (label: string, status?: string) => {
    const isSupp = status === 'SUPPORTED';
    const isUnsupp = status === 'UNSUPPORTED';
    return (
      <span
        key={label}
        className={`px-2 py-1 rounded text-[10px] font-mono font-bold flex items-center space-x-1 border ${
          isSupp
            ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
            : isUnsupp
            ? 'bg-slate-800 text-slate-500 border-slate-700 line-through'
            : 'bg-amber-500/10 text-amber-400 border-amber-500/30'
        }`}
      >
        <span>{label}</span>
        <span className="text-[9px] opacity-75">{status || 'UNKNOWN'}</span>
      </span>
    );
  };

  return (
    <div className="space-y-6">
      {/* Device Picker Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Sliders className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">UNIVERSAL 10-STEP CONFIGURATION SAFETY ENGINE</h2>
            <p className="text-[11px] text-slate-400 font-mono">Authenticate • Preview Diff • Explicit Approval • Read-Back Verify • Rollback</p>
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

      {/* Device Capabilities Matrix Strip */}
      <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-xl">
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center space-x-2 text-xs font-mono text-slate-300 font-bold">
            <Cpu className="w-4 h-4 text-emerald-400" />
            <span>AUTHENTICATED CAPABILITY MATRIX</span>
          </div>
          <span className="text-[10px] font-mono text-slate-400">
            Protocols: {currentDev.protocols?.join(', ') || 'ONVIF'}
          </span>
        </div>
        <div className="flex flex-wrap gap-2">
          {renderCapBadge('DEVICE MGMT', capabilities?.device_management || 'SUPPORTED')}
          {renderCapBadge('MEDIA STREAMING', capabilities?.media || 'SUPPORTED')}
          {renderCapBadge('MEDIA2', capabilities?.media2 || 'UNSUPPORTED')}
          {renderCapBadge('PTZ CONTROLS', capabilities?.ptz)}
          {renderCapBadge('IMAGING SENSOR', capabilities?.imaging || 'SUPPORTED')}
          {renderCapBadge('EVENT PULL', capabilities?.events)}
          {renderCapBadge('RECORDING', capabilities?.recording)}
          {renderCapBadge('ANALOG UTC', capabilities?.analog_format)}
        </div>
      </div>

      {/* Read-Back Verification / Apply Result Notification */}
      {lastResult && (
        <div
          className={`p-4 rounded-xl text-xs font-mono border ${
            lastResult.read_back_match
              ? 'bg-emerald-950/40 border-emerald-700/60 text-emerald-300'
              : 'bg-amber-950/40 border-amber-700/60 text-amber-300'
          }`}
        >
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2 font-bold">
              {lastResult.read_back_match ? (
                <>
                  <ShieldCheck className="w-4 h-4 text-emerald-400" />
                  <span>STEP 9 VERIFIED ON HARDWARE: MATCH</span>
                </>
              ) : (
                <>
                  <AlertTriangle className="w-4 h-4 text-amber-400" />
                  <span>STEP 9 READ-BACK VERIFICATION: MISMATCH NOTED</span>
                </>
              )}
            </div>
            {lastRollbackKey && (
              <button
                onClick={handleRollback}
                className="px-2.5 py-1 bg-amber-600/30 hover:bg-amber-600/50 text-amber-200 border border-amber-500/40 rounded flex items-center space-x-1.5 transition-colors font-bold"
              >
                <RotateCcw className="w-3.5 h-3.5" />
                <span>EXECUTE ROLLBACK (STEP 10)</span>
              </button>
            )}
          </div>
          <p className="mt-1.5 text-[11px] text-slate-300">{lastResult.message}</p>
          {lastResult.read_back_diff && (
            <div className="mt-1 text-[10px] text-amber-200/90 font-mono">
              Differences: {lastResult.read_back_diff}
            </div>
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

      {/* Tab 1: Network Form */}
      {activeTab === 'network' && (
        <div className="p-6 bg-[#161b22] border border-[#30363d] rounded-xl space-y-6">
          <div className="flex items-center justify-between border-b border-[#30363d] pb-4">
            <div>
              <h3 className="text-sm font-bold text-slate-100 font-mono">IP ADDRESS & NETWORK INTERFACE</h3>
              <p className="text-xs text-slate-400 font-mono">Configure static IPv4 or DHCP lease</p>
            </div>
            <div className="flex items-center space-x-2">
              <span className="text-xs font-mono text-slate-300">DHCP:</span>
              <input
                type="checkbox"
                checked={networkCfg.dhcp}
                onChange={(e) => setNetworkCfg({ ...networkCfg, dhcp: e.target.checked })}
                className="rounded border-[#30363d] text-emerald-500 focus:ring-emerald-500 h-4 w-4 bg-[#0d1117]"
              />
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs font-mono">
            <div>
              <label className="block text-slate-400 mb-1">IPv4 ADDRESS</label>
              <input
                type="text"
                disabled={networkCfg.dhcp}
                value={networkCfg.ip_address}
                onChange={(e) => setNetworkCfg({ ...networkCfg, ip_address: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 disabled:opacity-50"
              />
            </div>

            <div>
              <label className="block text-slate-400 mb-1">SUBNET MASK</label>
              <input
                type="text"
                disabled={networkCfg.dhcp}
                value={networkCfg.subnet_mask}
                onChange={(e) => setNetworkCfg({ ...networkCfg, subnet_mask: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 disabled:opacity-50"
              />
            </div>

            <div>
              <label className="block text-slate-400 mb-1">DEFAULT GATEWAY</label>
              <input
                type="text"
                disabled={networkCfg.dhcp}
                value={networkCfg.gateway}
                onChange={(e) => setNetworkCfg({ ...networkCfg, gateway: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 disabled:opacity-50"
              />
            </div>

            <div>
              <label className="block text-slate-400 mb-1">PRIMARY DNS</label>
              <input
                type="text"
                value={networkCfg.dns1}
                onChange={(e) => setNetworkCfg({ ...networkCfg, dns1: e.target.value })}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200"
              />
            </div>
          </div>

          <div className="pt-4 border-t border-[#30363d] flex justify-end">
            <button
              onClick={handleGeneratePreview}
              disabled={loading}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg font-mono text-xs font-bold transition-all flex items-center space-x-2"
            >
              <span>PREVIEW & VALIDATE (STEPS 1-6)</span>
            </button>
          </div>
        </div>
      )}

      {/* Tab 2: Image Parameters */}
      {activeTab === 'image' && (
        <div className="p-6 bg-[#161b22] border border-[#30363d] rounded-xl space-y-6">
          <div className="border-b border-[#30363d] pb-4">
            <h3 className="text-sm font-bold text-slate-100 font-mono">SENSOR & COLOR ADJUSTMENTS</h3>
            <p className="text-xs text-slate-400 font-mono">Adjust brightness, contrast, saturation, sharpness (0-100)</p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6 text-xs font-mono">
            <div>
              <div className="flex justify-between mb-1 text-slate-300">
                <span>BRIGHTNESS</span>
                <span className="text-emerald-400 font-bold">{imageCfg.brightness}</span>
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
              <div className="flex justify-between mb-1 text-slate-300">
                <span>CONTRAST</span>
                <span className="text-emerald-400 font-bold">{imageCfg.contrast}</span>
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
              <div className="flex justify-between mb-1 text-slate-300">
                <span>SATURATION</span>
                <span className="text-emerald-400 font-bold">{imageCfg.saturation}</span>
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
              <div className="flex justify-between mb-1 text-slate-300">
                <span>SHARPNESS</span>
                <span className="text-emerald-400 font-bold">{imageCfg.sharpness}</span>
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

          <div className="pt-4 border-t border-[#30363d] flex justify-end">
            <button
              onClick={handleGeneratePreview}
              disabled={loading}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg font-mono text-xs font-bold transition-all flex items-center space-x-2"
            >
              <span>PREVIEW & VALIDATE (STEPS 1-6)</span>
            </button>
          </div>
        </div>
      )}

      {/* Step 6 Modal: Preview & Staged Diff */}
      {previewModal && (
        <div className="fixed inset-0 bg-black/75 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="bg-[#161b22] border border-[#30363d] rounded-xl max-w-2xl w-full p-6 space-y-4 font-mono shadow-2xl">
            <div className="flex items-center justify-between border-b border-[#30363d] pb-3">
              <div className="flex items-center space-x-2 text-emerald-400 font-bold text-sm">
                <CheckCircle className="w-5 h-5" />
                <span>STEP 6: CONFIGURATION PREVIEW & DIFF</span>
              </div>
              <span className="text-[10px] bg-slate-800 text-slate-400 px-2 py-0.5 rounded border border-slate-700">
                Token: {previewModal.preview_token.slice(0, 12)}...
              </span>
            </div>

            {/* Validation Alert */}
            {!previewModal.valid ? (
              <div className="p-3 bg-red-950/50 border border-red-800 text-red-300 rounded-lg text-xs space-y-1">
                <div className="font-bold flex items-center space-x-1.5">
                  <AlertTriangle className="w-4 h-4 text-red-400" />
                  <span>VALIDATION CHECKS FAILED</span>
                </div>
                {previewModal.validation_errors.map((err, i) => (
                  <div key={i} className="text-[11px]">• {err}</div>
                ))}
              </div>
            ) : (
              <div className="p-3 bg-emerald-950/40 border border-emerald-800 text-emerald-300 rounded-lg text-xs">
                <span>All syntactic and hardware limits verified successfully.</span>
              </div>
            )}

            {/* Structural Diff */}
            <div>
              <label className="text-xs text-slate-400 block mb-1">STRUCTURAL FIELD DIFF:</label>
              <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200">
                {previewModal.diff}
              </div>
            </div>

            <div className="flex items-center justify-end space-x-3 pt-3 border-t border-[#30363d]">
              <button
                onClick={() => setPreviewModal(null)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs"
              >
                CANCEL
              </button>
              <button
                disabled={!previewModal.valid || isApplying}
                onClick={handleConfirmApply}
                className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white rounded-lg text-xs font-bold flex items-center space-x-1.5"
              >
                <span>{isApplying ? 'APPLYING & VERIFYING...' : 'APPROVE & APPLY (STEPS 7-9)'}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
