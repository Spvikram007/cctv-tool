import React, { useState, useEffect } from 'react';
import { Device, FirmwarePackage, UpdateEligibility, FirmwareUpdatePreview, FirmwareUpdateJob } from '../types';
import { api } from '../services/api';
import {
  Cpu,
  RefreshCw,
  CheckCircle,
  XCircle,
  AlertTriangle,
  ShieldCheck,
  ShieldAlert,
  HardDrive,
  Terminal,
  Play
} from 'lucide-react';

interface FirmwareProps {
  devices: Device[];
}

export const Firmware: React.FC<FirmwareProps> = ({ devices }) => {
  const [activeTab, setActiveTab] = useState<'catalog' | 'stage' | 'recovery'>('catalog');
  const [packages, setPackages] = useState<FirmwarePackage[]>([]);
  const [loading, setLoading] = useState(false);

  // Staged update selection
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>(devices.length > 0 ? devices[0].id : '');
  const [selectedFirmwareId, setSelectedFirmwareId] = useState<string>('');
  const [eligibility, setEligibility] = useState<UpdateEligibility | null>(null);
  const [checkingEligibility, setCheckingEligibility] = useState(false);

  // Staged Preview Modal
  const [stagedPreview, setStagedPreview] = useState<FirmwareUpdatePreview | null>(null);
  const [stagingLoading, setStagingLoading] = useState(false);
  const [showStageModal, setShowStageModal] = useState(false);
  const [operatorConfirmation, setOperatorConfirmation] = useState(false);

  // Live Flash Job
  const [currentJob, setCurrentJob] = useState<FirmwareUpdateJob | null>(null);

  useEffect(() => {
    loadInventory();
  }, []);

  const loadInventory = async () => {
    setLoading(true);
    try {
      const list = await api.getFirmwareInventory();
      setPackages(list);
      if (list.length > 0 && !selectedFirmwareId) {
        setSelectedFirmwareId(list[0].id);
      }
    } catch (err) {
      console.error('Firmware inventory load error:', err);
    } finally {
      setLoading(false);
    }
  };

  // Poll current job if running
  useEffect(() => {
    if (!currentJob || (currentJob.status !== 'FLASHING' && currentJob.status !== 'REBOOTING' && currentJob.status !== 'PREPARING' && currentJob.status !== 'UPLOADING' && currentJob.status !== 'VERIFYING')) {
      return;
    }
    const timer = setInterval(async () => {
      try {
        const updated = await api.getFirmwareJob(currentJob.id);
        setCurrentJob(updated);
      } catch (e) {
        console.error('Job poll error:', e);
      }
    }, 2000);
    return () => clearInterval(timer);
  }, [currentJob]);

  // Check compatibility when device or package changes
  useEffect(() => {
    if (!selectedDeviceId || !selectedFirmwareId) return;
    checkCompatibility(selectedDeviceId, selectedFirmwareId);
  }, [selectedDeviceId, selectedFirmwareId]);

  const checkCompatibility = async (devId: string, fwId: string) => {
    setCheckingEligibility(true);
    try {
      const res = await api.checkFirmwareCompatibility(devId, fwId);
      setEligibility(res);
    } catch (e) {
      console.error(e);
      setEligibility(null);
    } finally {
      setCheckingEligibility(false);
    }
  };

  // Stage update preview
  const handleStageUpdate = async () => {
    if (!selectedDeviceId || !selectedFirmwareId) return;
    setStagingLoading(true);
    try {
      const preview = await api.stageFirmwarePreview(selectedDeviceId, selectedFirmwareId);
      setStagedPreview(preview);
      setOperatorConfirmation(false);
      setShowStageModal(true);
    } catch (err: any) {
      alert(`Staging failed: ${err.message}`);
    } finally {
      setStagingLoading(false);
    }
  };

  // Approve & execute flash
  const handleApproveFlash = async () => {
    if (!stagedPreview || !operatorConfirmation) {
      alert('You must check the operator safety confirmation before proceeding.');
      return;
    }
    setShowStageModal(false);
    try {
      const job = await api.approveFirmwareUpdate(stagedPreview.preview_token, true);
      setCurrentJob(job);
      setActiveTab('stage');
    } catch (err: any) {
      alert(`Flash initialization failed: ${err.message}`);
    }
  };

  return (
    <div className="space-y-6 font-mono">
      {/* Navigation Sub-Tabs */}
      <div className="flex border-b border-[#30363d] space-x-6 text-sm">
        <button
          onClick={() => setActiveTab('catalog')}
          className={`pb-3 font-bold flex items-center space-x-2 transition-colors ${
            activeTab === 'catalog'
              ? 'text-emerald-400 border-b-2 border-emerald-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Cpu className="w-4 h-4" />
          <span>FIRMWARE CATALOG & COMPATIBILITY</span>
        </button>
        <button
          onClick={() => setActiveTab('stage')}
          className={`pb-3 font-bold flex items-center space-x-2 transition-colors ${
            activeTab === 'stage'
              ? 'text-emerald-400 border-b-2 border-emerald-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Play className="w-4 h-4" />
          <span>STAGED FLASH PIPELINE</span>
        </button>
        <button
          onClick={() => setActiveTab('recovery')}
          className={`pb-3 font-bold flex items-center space-x-2 transition-colors ${
            activeTab === 'recovery'
              ? 'text-emerald-400 border-b-2 border-emerald-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Terminal className="w-4 h-4" />
          <span>EMERGENCY UNBRICK PLAYBOOKS</span>
        </button>
      </div>

      {/* Catalog & Compatibility Tab */}
      {activeTab === 'catalog' && (
        <div className="space-y-6">
          {/* Header Card */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-2">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
                  <Cpu className="w-5 h-5 text-emerald-400" />
                  <span>SECURE FIRMWARE MANAGEMENT & CROSS-FLASH GUARD</span>
                </h2>
                <p className="text-xs text-slate-400 mt-1">
                  Cryptographically verified SHA-256 binary validation, model/hardware revision checking, and strict OEM cross-flash protection.
                </p>
              </div>
              <button
                onClick={loadInventory}
                disabled={loading}
                className="px-3 py-1.5 rounded-lg bg-[#0d1117] border border-[#30363d] hover:border-slate-500 text-xs text-slate-300 flex items-center space-x-1"
              >
                <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin text-emerald-400' : ''}`} />
                <span>Refresh Catalog</span>
              </button>
            </div>
          </div>

          {/* Interactive Compatibility Validator Card */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="font-bold text-slate-200 text-xs uppercase flex items-center space-x-2">
              <ShieldCheck className="w-4 h-4 text-emerald-400" />
              <span>PRE-FLIGHT HARDWARE COMPATIBILITY MATRIX CHECKER</span>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs">
              <div>
                <label className="block text-slate-400 mb-1">SELECT TARGET HARDWARE</label>
                <select
                  value={selectedDeviceId}
                  onChange={(e) => setSelectedDeviceId(e.target.value)}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none focus:border-emerald-500"
                >
                  {devices.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name} ({d.ip}) — {d.manufacturer} {d.model} [FW: {d.firmware || 'N/A'}]
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-slate-400 mb-1">SELECT FIRMWARE PACKAGE</label>
                <select
                  value={selectedFirmwareId}
                  onChange={(e) => setSelectedFirmwareId(e.target.value)}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none focus:border-emerald-500"
                >
                  {packages.map((p) => (
                    <option key={p.id} value={p.id}>
                      [{p.manufacturer}] {p.model} v{p.version} ({p.region}) {p.critical ? '• CRITICAL' : ''}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            {/* Live Eligibility Banner */}
            {checkingEligibility ? (
              <div className="p-3 bg-[#0d1117] rounded-lg border border-[#30363d] text-xs text-slate-400 flex items-center space-x-2">
                <RefreshCw className="w-4 h-4 animate-spin text-emerald-400" />
                <span>Checking hardware revisions, OEM signatures and cross-flash rules...</span>
              </div>
            ) : eligibility ? (
              <div
                className={`p-4 rounded-lg border text-xs space-y-2 ${
                  eligibility.cross_flash_blocked
                    ? 'bg-red-950/40 border-red-700/60 text-red-200'
                    : eligibility.eligible
                    ? 'bg-emerald-950/40 border-emerald-700/60 text-emerald-200'
                    : 'bg-amber-950/40 border-amber-700/60 text-amber-200'
                }`}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2 font-bold text-sm">
                    {eligibility.cross_flash_blocked ? (
                      <>
                        <ShieldAlert className="w-5 h-5 text-red-400" />
                        <span className="text-red-400">CROSS-FLASH ATTEMPT BLOCKED</span>
                      </>
                    ) : eligibility.eligible ? (
                      <>
                        <CheckCircle className="w-5 h-5 text-emerald-400" />
                        <span className="text-emerald-400">HARDWARE COMPATIBLE & VERIFIED</span>
                      </>
                    ) : (
                      <>
                        <AlertTriangle className="w-5 h-5 text-amber-400" />
                        <span className="text-amber-400">COMPATIBILITY MISMATCH</span>
                      </>
                    )}
                  </div>

                  {eligibility.eligible && (
                    <button
                      onClick={handleStageUpdate}
                      disabled={stagingLoading}
                      className="px-4 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-bold flex items-center space-x-2 shadow-lg shadow-emerald-900/30"
                    >
                      <Play className="w-3.5 h-3.5" />
                      <span>STAGE THIS UPDATE</span>
                    </button>
                  )}
                </div>

                <p className="text-[11px] leading-relaxed font-mono">
                  {eligibility.reason}
                </p>

                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1 text-[11px] font-mono">
                  <div>
                    <span className="text-slate-400">Current FW:</span> {eligibility.current_version || 'N/A'}
                  </div>
                  <div>
                    <span className="text-slate-400">Target FW:</span> {eligibility.target_version}
                  </div>
                  <div>
                    <span className="text-slate-400">Cross-Flash Guard:</span>{' '}
                    {eligibility.cross_flash_blocked ? (
                      <strong className="text-red-400">BLOCKED</strong>
                    ) : (
                      <span className="text-emerald-400">SAFE</span>
                    )}
                  </div>
                  <div>
                    <span className="text-slate-400">Model Check:</span>{' '}
                    {eligibility.model_mismatch ? (
                      <strong className="text-amber-400">MISMATCH</strong>
                    ) : (
                      <span className="text-emerald-400">PASS</span>
                    )}
                  </div>
                </div>
              </div>
            ) : null}
          </div>

          {/* Verified Catalog Table */}
          <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
            <div className="p-4 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between text-xs">
              <span className="font-bold text-slate-200 uppercase">
                VERIFIED CCTV FIRMWARE PACKAGES ({packages.length} PACKAGES)
              </span>
              <span className="text-slate-400 text-[11px]">All binaries signed with SHA-256</span>
            </div>

            <div className="divide-y divide-[#21262d] text-xs">
              {packages.map((pkg) => (
                <div
                  key={pkg.id}
                  onClick={() => setSelectedFirmwareId(pkg.id)}
                  className={`p-4 flex flex-col md:flex-row md:items-center justify-between gap-3 cursor-pointer transition-colors ${
                    selectedFirmwareId === pkg.id
                      ? 'bg-emerald-950/20 border-l-4 border-l-emerald-500'
                      : 'hover:bg-[#1a2029]'
                  }`}
                >
                  <div className="space-y-1">
                    <div className="flex items-center space-x-2">
                      <span className="font-bold text-slate-100">{pkg.model}</span>
                      <span className="px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300 font-bold">
                        {pkg.manufacturer}
                      </span>
                      <span className="px-2 py-0.5 rounded text-[10px] bg-emerald-900/40 text-emerald-400 font-bold">
                        v{pkg.version}
                      </span>
                      {pkg.critical && (
                        <span className="px-2 py-0.5 rounded text-[10px] bg-red-900/40 text-red-400 font-bold">
                          CRITICAL SECURITY PATCH
                        </span>
                      )}
                    </div>
                    <div className="text-[11px] text-slate-400">
                      HW Rev: <span className="text-slate-300">{pkg.hardware_revision}</span> • Region: <span className="text-slate-300">{pkg.region}</span> • Size: {(pkg.file_size_bytes / 1024 / 1024).toFixed(1)} MB • Release: {pkg.release_date}
                    </div>
                    <div className="text-[10px] text-slate-500 font-mono">
                      SHA-256: <span className="text-slate-400">{pkg.sha256_checksum}</span>
                    </div>
                    <div className="text-[11px] text-slate-400 italic">
                      {pkg.release_notes}
                    </div>
                  </div>

                  <div className="flex items-center space-x-3">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        setSelectedFirmwareId(pkg.id);
                        handleStageUpdate();
                      }}
                      className="px-3 py-1.5 rounded-lg bg-[#0d1117] border border-[#30363d] hover:border-emerald-500 text-emerald-400 text-xs font-bold"
                    >
                      Stage Flash
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Staged Flash Pipeline Tab */}
      {activeTab === 'stage' && (
        <div className="space-y-6">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <h3 className="font-bold text-slate-100 flex items-center space-x-2">
              <Play className="w-5 h-5 text-emerald-400" />
              <span>ACTIVE FIRMWARE FLASH PIPELINE</span>
            </h3>

            {currentJob ? (
              <div className="space-y-4">
                <div className="p-4 bg-[#0d1117] border border-[#21262d] rounded-lg space-y-3">
                  <div className="flex items-center justify-between text-xs">
                    <div>
                      <span className="font-bold text-slate-200">JOB ID: {currentJob.id}</span>
                      <span className="ml-3 text-slate-400">Target Device: {currentJob.device_id}</span>
                    </div>
                    <span
                      className={`px-2.5 py-1 rounded text-xs font-bold ${
                        currentJob.status === 'SUCCESS'
                          ? 'bg-emerald-500/20 text-emerald-400'
                          : currentJob.status === 'FAILED'
                          ? 'bg-red-500/20 text-red-400'
                          : 'bg-blue-500/20 text-blue-400 animate-pulse'
                      }`}
                    >
                      {currentJob.status} ({currentJob.stage})
                    </span>
                  </div>

                  {/* Flash Progress Bar */}
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs text-slate-400">
                      <span>Pipeline Stage: {currentJob.stage}</span>
                      <span>{currentJob.progress_percent}%</span>
                    </div>
                    <div className="w-full h-3 bg-[#161b22] rounded-full overflow-hidden border border-[#30363d]">
                      <div
                        className={`h-full transition-all duration-500 ${
                          currentJob.status === 'SUCCESS'
                            ? 'bg-emerald-500'
                            : currentJob.status === 'FAILED'
                            ? 'bg-red-500'
                            : 'bg-blue-500'
                        }`}
                        style={{ width: `${currentJob.progress_percent}%` }}
                      />
                    </div>
                  </div>

                  {/* Stage Sequence Indicator */}
                  <div className="grid grid-cols-2 sm:grid-cols-6 gap-2 pt-2 text-[11px] text-center font-mono">
                    {['PREPARING', 'UPLOADING', 'FLASHING', 'REBOOTING', 'VERIFYING', 'SUCCESS'].map((st) => {
                      const stages = ['PREPARING', 'UPLOADING', 'FLASHING', 'REBOOTING', 'VERIFYING', 'SUCCESS'];
                      const currentIdx = stages.indexOf(currentJob.status);
                      const stepIdx = stages.indexOf(st);
                      const isDone = currentJob.status === 'SUCCESS' || currentIdx > stepIdx;
                      const isCurrent = currentJob.status === st;

                      return (
                        <div
                          key={st}
                          className={`p-2 rounded border ${
                            isDone
                              ? 'bg-emerald-950/30 border-emerald-700/60 text-emerald-300'
                              : isCurrent
                              ? 'bg-blue-950/40 border-blue-600 text-blue-300 font-bold animate-pulse'
                              : 'bg-[#161b22] border-[#30363d] text-slate-500'
                          }`}
                        >
                          {st}
                        </div>
                      );
                    })}
                  </div>

                  {currentJob.status === 'SUCCESS' && (
                    <div className="p-3 bg-emerald-950/40 border border-emerald-700/60 rounded-lg text-emerald-300 text-xs flex items-center space-x-2">
                      <CheckCircle className="w-4 h-4 text-emerald-400" />
                      <span>
                        Firmware Update Verified Successfully! Running Active Firmware:{' '}
                        <strong>{currentJob.verified_new_firmware}</strong>
                      </span>
                    </div>
                  )}

                  {currentJob.status === 'FAILED' && (
                    <div className="p-3 bg-red-950/40 border border-red-700/60 rounded-lg text-red-300 text-xs flex items-center space-x-2">
                      <XCircle className="w-4 h-4 text-red-400" />
                      <span>Firmware Update Failed: {currentJob.error || 'Unknown error'}</span>
                    </div>
                  )}
                </div>
              </div>
            ) : (
              <div className="p-8 text-center text-slate-500 text-xs">
                No active firmware flashing job in progress. Go to the Catalog tab to stage and approve an update.
              </div>
            )}
          </div>
        </div>
      )}

      {/* Emergency Unbrick Playbooks Tab */}
      {activeTab === 'recovery' && (
        <div className="space-y-6">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-2">
            <h3 className="font-bold text-slate-100 flex items-center space-x-2">
              <Terminal className="w-5 h-5 text-emerald-400" />
              <span>EMERGENCY HARDWARE RECOVERY & UNBRICK PLAYBOOKS</span>
            </h3>
            <p className="text-xs text-slate-400">
              Field engineering procedures for recovering non-responsive cameras and bricked firmware partitions via TFTP, ConfigTool, or UART serial.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs">
            {/* Hikvision TFTP Recovery */}
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
              <div className="flex items-center space-x-2 text-emerald-400 font-bold">
                <HardDrive className="w-4 h-4" />
                <span>HIKVISION TFTP AUTO-RECOVERY</span>
              </div>
              <p className="text-slate-400 text-[11px] leading-relaxed">
                Hikvision bootloaders broadcast a TFTP probe to <code className="text-slate-200">192.0.0.128</code> for 5 seconds on cold boot.
              </p>
              <div className="p-2.5 bg-[#0d1117] rounded border border-[#21262d] text-[11px] space-y-1 font-mono text-slate-300">
                <div>1. Set PC IP: <span className="text-emerald-400">192.0.0.128 / 255.255.255.0</span></div>
                <div>2. Place firmware named <span className="text-emerald-400">digicap.dav</span> in TFTP directory.</div>
                <div>3. Launch Hikvision TFTP server.</div>
                <div>4. Power-cycle the camera.</div>
                <div>5. Watch TFTP log: "Device[192.0.0.64] test tftpserver... Connect client success".</div>
                <div>6. Flashing takes ~3-5 mins; camera reboots automatically.</div>
              </div>
            </div>

            {/* Dahua ConfigTool Recovery */}
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
              <div className="flex items-center space-x-2 text-blue-400 font-bold">
                <HardDrive className="w-4 h-4" />
                <span>DAHUA CONFIGTOOL & BOOT REFLASH</span>
              </div>
              <p className="text-slate-400 text-[11px] leading-relaxed">
                Dahua hardware supports forced TFTP reflash using Dahua ConfigTool or u-boot emergency command mode.
              </p>
              <div className="p-2.5 bg-[#0d1117] rounded border border-[#21262d] text-[11px] space-y-1 font-mono text-slate-300">
                <div>1. Connect directly to PC NIC.</div>
                <div>2. Set PC IP to <span className="text-blue-400">192.168.254.254 / 24</span>.</div>
                <div>3. Open Dahua ConfigTool $\to$ Upgrade.</div>
                <div>4. Select .bin package matching HW revision.</div>
                <div>5. If unresponsive: short TX/RX serial to access U-Boot bootloader: <code className="text-blue-300">run up</code>.</div>
              </div>
            </div>

            {/* CP PLUS Hardware Recovery */}
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
              <div className="flex items-center space-x-2 text-amber-400 font-bold">
                <HardDrive className="w-4 h-4" />
                <span>CP PLUS RECOVERY & USB UPGRADE</span>
              </div>
              <p className="text-slate-400 text-[11px] leading-relaxed">
                CP PLUS recorders (CP-UVR) support boot-time USB reflash when the flash file system is corrupted.
              </p>
              <div className="p-2.5 bg-[#0d1117] rounded border border-[#21262d] text-[11px] space-y-1 font-mono text-slate-300">
                <div>1. Format USB drive as <span className="text-amber-400">FAT32</span>.</div>
                <div>2. Place <span className="text-amber-400">update.bin</span> in root directory.</div>
                <div>3. Insert USB into front port of DVR/NVR.</div>
                <div>4. Hold front reset button and power on.</div>
                <div>5. Release after 15 seconds. LED flashes rapidly during flash.</div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Staged Update Preview Approval Modal */}
      {showStageModal && stagedPreview && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 font-mono">
          <div className="bg-[#161b22] border border-[#30363d] rounded-xl max-w-2xl w-full shadow-2xl overflow-hidden">
            <div className="p-4 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between">
              <h3 className="font-bold text-slate-100 flex items-center space-x-2 text-sm">
                <Cpu className="w-5 h-5 text-emerald-400" />
                <span>STAGED FIRMWARE FLASH APPROVAL</span>
              </h3>
              <button
                onClick={() => setShowStageModal(false)}
                className="text-slate-400 hover:text-slate-200 text-lg font-bold"
              >
                ✕
              </button>
            </div>

            <div className="p-5 space-y-4 text-xs">
              <div className="p-3 bg-amber-950/30 border border-amber-700/50 rounded-lg text-amber-300 space-y-1">
                <div className="font-bold flex items-center space-x-1">
                  <AlertTriangle className="w-4 h-4" />
                  <span>OPERATOR CONFIRMATION MANDATED</span>
                </div>
                <p className="text-[11px]">
                  Flashing firmware rewrites NAND flash memory. Power loss or physical disconnection during this process will brick the hardware.
                </p>
              </div>

              <div className="p-3 bg-[#0d1117] rounded-lg border border-[#21262d] space-y-2 text-[11px]">
                <div className="flex justify-between">
                  <span className="text-slate-400">Target Device:</span>
                  <span className="font-bold text-slate-200">{stagedPreview.device_name}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-400">Current Firmware:</span>
                  <span className="font-bold text-slate-300">{stagedPreview.current_firmware}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-400">Target Firmware:</span>
                  <span className="font-bold text-emerald-400">{stagedPreview.target_firmware}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-400">SHA-256 Checksum:</span>
                  <span className="font-mono text-slate-400 truncate max-w-xs">{stagedPreview.sha256_checksum}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-400">Estimated Reboot Time:</span>
                  <span className="font-mono text-slate-300">~{stagedPreview.estimated_reboot_sec} seconds</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-400">Preview Token:</span>
                  <span className="font-mono text-emerald-400">{stagedPreview.preview_token}</span>
                </div>
              </div>

              <label className="flex items-start space-x-2 p-3 bg-[#0d1117] rounded-lg border border-[#30363d] cursor-pointer">
                <input
                  type="checkbox"
                  checked={operatorConfirmation}
                  onChange={(e) => setOperatorConfirmation(e.target.checked)}
                  className="accent-emerald-500 mt-1"
                />
                <span className="text-[11px] text-slate-300 leading-relaxed">
                  I confirm that I am an authorized CCTV technician, the camera is connected to a dedicated uninterrupted power supply (UPS), and the network will remain stable throughout the flash process.
                </span>
              </label>
            </div>

            <div className="p-4 bg-[#0d1117] border-t border-[#30363d] flex items-center justify-between">
              <button
                onClick={() => setShowStageModal(false)}
                className="px-4 py-2 rounded-lg bg-[#161b22] border border-[#30363d] text-slate-300 text-xs"
              >
                Abort & Cancel
              </button>
              <button
                onClick={handleApproveFlash}
                disabled={!operatorConfirmation}
                className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center space-x-2 transition-all shadow-lg shadow-emerald-900/30 disabled:opacity-40"
              >
                <Cpu className="w-4 h-4" />
                <span>EXECUTE FIRMWARE FLASH</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
