import React, { useState, useEffect } from 'react';
import { Device, BatchJob, BatchAction, BatchPreview } from '../types';
import { api } from '../services/api';
import {
  Layers,
  RefreshCw,
  Play,
  CheckCircle,
  AlertTriangle,
  StopCircle,
  Eye,
  Download,
  GitCompare,
  FileText,
  ShieldAlert,
  Search
} from 'lucide-react';

interface BatchConfigProps {
  devices: Device[];
  onRefresh: () => void;
}

export const BatchConfig: React.FC<BatchConfigProps> = ({ devices, onRefresh }) => {
  const [activeSubTab, setActiveSubTab] = useState<'dispatch' | 'compare' | 'history'>('dispatch');
  const [jobs, setJobs] = useState<BatchJob[]>([]);
  const [currentJob, setCurrentJob] = useState<BatchJob | null>(null);
  const [jobName, setJobName] = useState('Fleet Routine Management');
  const [actionType, setActionType] = useState<BatchAction>('HEALTH_CHECK');
  const [selectedIds, setSelectedIds] = useState<string[]>(devices.map((d) => d.id));
  const [brandFilter, setBrandFilter] = useState<string>('ALL');
  const [searchFilter, setSearchFilter] = useState<string>('');

  // Worker Pool Tuning
  const [concurrency, setConcurrency] = useState<number>(4);
  const [rateLimit, setRateLimit] = useState<number>(5);

  // Action-specific parameters
  const [ntpServer, setNtpServer] = useState<string>('time.google.com');
  const [dns1, setDns1] = useState<string>('8.8.8.8');
  const [dns2, setDns2] = useState<string>('1.1.1.1');
  const [wdrMode, setWdrMode] = useState<string>('ON');
  const [brightness, setBrightness] = useState<number>(50);
  const [rebootConfirmation, setRebootConfirmation] = useState<string>('');

  // Staged Preview State
  const [previewData, setPreviewData] = useState<BatchPreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [showPreviewModal, setShowPreviewModal] = useState(false);
  const [executing, setExecuting] = useState(false);

  // Compare Tab State
  const [compareResult, setCompareResult] = useState<any>(null);
  const [compareLoading, setCompareLoading] = useState(false);
  const [compareType, setCompareType] = useState<'NETWORK' | 'IMAGE' | 'GENERAL'>('NETWORK');

  useEffect(() => {
    loadJobs();
  }, []);

  // Poll current job if running
  useEffect(() => {
    if (!currentJob || (currentJob.status !== 'RUNNING' && currentJob.status !== 'PENDING')) return;
    const timer = setInterval(async () => {
      try {
        const updated = await api.getBatchJob(currentJob.id);
        setCurrentJob(updated);
        if (updated.status !== 'RUNNING' && updated.status !== 'PENDING') {
          setExecuting(false);
          loadJobs();
          onRefresh();
        }
      } catch (e) {
        console.error('Job poll error:', e);
      }
    }, 1500);
    return () => clearInterval(timer);
  }, [currentJob]);

  const loadJobs = async () => {
    try {
      const list = await api.getBatchJobs();
      setJobs(list);
      if (list.length > 0 && !currentJob) {
        setCurrentJob(list[0]);
      }
    } catch (e) {
      console.error(e);
    }
  };

  // Build payload based on action type
  const getActionPayload = () => {
    switch (actionType) {
      case 'TIME_NTP_SYNC':
        return { ntp_server: ntpServer, timezone: 'Asia/Kolkata', ntp_enabled: true };
      case 'NETWORK_CONFIG':
        return { dns1, dns2 };
      case 'IMAGE_CONFIG':
        return { wdr_mode: wdrMode, brightness, contrast: 50, saturation: 50 };
      case 'REBOOT':
        return { operator_confirmed: rebootConfirmation === 'CONFIRM-REBOOT' };
      default:
        return {};
    }
  };

  // Step 1: Generate Staged Preview
  const handleGeneratePreview = async () => {
    if (selectedIds.length === 0) {
      alert('Select at least one device for batch execution.');
      return;
    }
    if (actionType === 'REBOOT' && rebootConfirmation !== 'CONFIRM-REBOOT') {
      alert('Fleet reboot is a high-impact operation. You must type "CONFIRM-REBOOT" to proceed.');
      return;
    }

    setPreviewLoading(true);
    try {
      const payload = getActionPayload();
      const preview = await api.previewBatch(actionType, selectedIds, payload);
      setPreviewData(preview);
      setShowPreviewModal(true);
    } catch (err: any) {
      alert(`Staged Preview failed: ${err.message}`);
    } finally {
      setPreviewLoading(false);
    }
  };

  // Step 2: Approve and Execute
  const handleApproveAndExecute = async () => {
    if (!previewData) return;
    setShowPreviewModal(false);
    setExecuting(true);
    try {
      const payload = getActionPayload();
      const job = await api.executeBatch({
        name: jobName,
        action: actionType,
        target_ids: selectedIds,
        payload,
        preview_token: previewData.preview_token,
        concurrency,
        rate_limit: rateLimit,
      });
      setCurrentJob(job);
      await loadJobs();
    } catch (err: any) {
      alert(`Batch dispatch failed: ${err.message}`);
      setExecuting(false);
    }
  };

  // Cancel Job
  const handleCancelJob = async () => {
    if (!currentJob) return;
    try {
      await api.cancelBatchJob(currentJob.id);
      const updated = await api.getBatchJob(currentJob.id);
      setCurrentJob(updated);
      setExecuting(false);
      loadJobs();
    } catch (err: any) {
      alert(`Cancel failed: ${err.message}`);
    }
  };

  // Run Fleet Comparison
  const handleRunCompare = async () => {
    if (selectedIds.length < 2) {
      alert('Select at least 2 devices to compare configuration drift.');
      return;
    }
    setCompareLoading(true);
    try {
      const res = await api.compareBatchConfigs(selectedIds, compareType);
      setCompareResult(res);
    } catch (err: any) {
      alert(`Compare failed: ${err.message}`);
    } finally {
      setCompareLoading(false);
    }
  };

  // Export Configurations
  const handleExportConfigs = async () => {
    if (selectedIds.length === 0) {
      alert('Select devices to export.');
      return;
    }
    try {
      const res = await api.exportBatchConfigs(selectedIds);
      const blob = new Blob([JSON.stringify(res, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `cctv-fleet-config-export-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
    } catch (err: any) {
      alert(`Export failed: ${err.message}`);
    }
  };

  // Filter devices
  const filteredDevices = devices.filter((dev) => {
    const matchesBrand = brandFilter === 'ALL' || dev.manufacturer === brandFilter;
    const matchesSearch =
      dev.name.toLowerCase().includes(searchFilter.toLowerCase()) ||
      dev.ip.includes(searchFilter) ||
      dev.model.toLowerCase().includes(searchFilter.toLowerCase());
    return matchesBrand && matchesSearch;
  });

  return (
    <div className="space-y-6 font-mono">
      {/* Top Navigation Tabs */}
      <div className="flex border-b border-[#30363d] space-x-6 text-sm">
        <button
          onClick={() => setActiveSubTab('dispatch')}
          className={`pb-3 font-bold flex items-center space-x-2 transition-colors ${
            activeSubTab === 'dispatch'
              ? 'text-emerald-400 border-b-2 border-emerald-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Layers className="w-4 h-4" />
          <span>BATCH DISPATCH ENGINE</span>
        </button>
        <button
          onClick={() => setActiveSubTab('compare')}
          className={`pb-3 font-bold flex items-center space-x-2 transition-colors ${
            activeSubTab === 'compare'
              ? 'text-emerald-400 border-b-2 border-emerald-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <GitCompare className="w-4 h-4" />
          <span>FLEET CONFIG COMPARISON</span>
        </button>
        <button
          onClick={() => setActiveSubTab('history')}
          className={`pb-3 font-bold flex items-center space-x-2 transition-colors ${
            activeSubTab === 'history'
              ? 'text-emerald-400 border-b-2 border-emerald-400'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <FileText className="w-4 h-4" />
          <span>JOB HISTORY & AUDIT LOGS</span>
        </button>
      </div>

      {activeSubTab === 'dispatch' && (
        <>
          {/* Dispatch Control Bar */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
              <div>
                <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
                  <Layers className="w-5 h-5 text-emerald-400" />
                  <span>ENTERPRISE BATCH DISPATCH CONSOLE</span>
                </h2>
                <p className="text-xs text-slate-400 mt-1">
                  Worker pool with staged pre-flight diffs, capability checks, rate limiting, and rollback protection.
                </p>
              </div>

              <div className="flex items-center space-x-3">
                <button
                  onClick={handleExportConfigs}
                  className="px-3 py-1.5 rounded-lg bg-[#0d1117] border border-[#30363d] hover:border-slate-500 text-xs text-slate-300 flex items-center space-x-2"
                >
                  <Download className="w-3.5 h-3.5" />
                  <span>Export Backup</span>
                </button>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-4 gap-4 pt-2">
              <div>
                <label className="block text-[11px] text-slate-400 mb-1">BATCH OPERATION NAME</label>
                <input
                  type="text"
                  value={jobName}
                  onChange={(e) => setJobName(e.target.value)}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-[11px] text-slate-400 mb-1">BATCH OPERATION ACTION</label>
                <select
                  value={actionType}
                  onChange={(e) => setActionType(e.target.value as BatchAction)}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
                >
                  <option value="HEALTH_CHECK">HEALTH_CHECK — Probe Ping, Ports & Video Responders</option>
                  <option value="TIME_NTP_SYNC">TIME_NTP_SYNC — Synchronize Clock & Time Server</option>
                  <option value="INFO_RETRIEVAL">INFO_RETRIEVAL — Query Firmware, Serial & Uptime</option>
                  <option value="NETWORK_CONFIG">NETWORK_CONFIG — Standardize Fleet DNS & Gateway</option>
                  <option value="IMAGE_CONFIG">IMAGE_CONFIG — Standardize Fleet WDR & Image Presets</option>
                  <option value="REBOOT">REBOOT — Coordinated Fleet Device Restart (High-Risk)</option>
                  <option value="CONFIG_COMPARE">CONFIG_COMPARE — Run Deviation Analysis</option>
                </select>
              </div>

              <div>
                <label className="block text-[11px] text-slate-400 mb-1">
                  WORKER POOL ({concurrency} THREADS / {rateLimit} REQ/S)
                </label>
                <div className="space-y-1">
                  <div className="flex items-center space-x-2">
                    <input
                      type="range"
                      min="1"
                      max="20"
                      value={concurrency}
                      onChange={(e) => setConcurrency(parseInt(e.target.value))}
                      className="w-full accent-emerald-500"
                    />
                    <span className="text-xs font-bold text-emerald-400 w-6">{concurrency}</span>
                  </div>
                  <div className="flex items-center justify-between text-[10px] text-slate-400">
                    <span>Host Rate Limit:</span>
                    <select
                      value={rateLimit}
                      onChange={(e) => setRateLimit(parseInt(e.target.value))}
                      className="bg-[#0d1117] border border-[#30363d] rounded px-1 text-slate-200"
                    >
                      <option value="2">2 req/s</option>
                      <option value="5">5 req/s</option>
                      <option value="10">10 req/s</option>
                      <option value="20">20 req/s</option>
                    </select>
                  </div>
                </div>
              </div>

              <div className="flex items-end">
                <button
                  onClick={handleGeneratePreview}
                  disabled={previewLoading || executing}
                  className="w-full py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center justify-center space-x-2 transition-all shadow-lg shadow-emerald-900/30 disabled:opacity-50"
                >
                  {previewLoading ? (
                    <>
                      <RefreshCw className="w-4 h-4 animate-spin" />
                      <span>VALIDATING CAPABILITIES...</span>
                    </>
                  ) : (
                    <>
                      <Eye className="w-4 h-4" />
                      <span>STAGE & PREVIEW ({selectedIds.length} TARGETS)</span>
                    </>
                  )}
                </button>
              </div>
            </div>

            {/* Action-Specific Parameter Box */}
            {actionType === 'TIME_NTP_SYNC' && (
              <div className="p-3 bg-[#0d1117] border border-[#21262d] rounded-lg grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                <div>
                  <label className="block text-slate-400 mb-1">NTP SERVER ADDRESS</label>
                  <input
                    type="text"
                    value={ntpServer}
                    onChange={(e) => setNtpServer(e.target.value)}
                    placeholder="e.g. time.google.com or pool.ntp.org"
                    className="w-full px-3 py-1.5 bg-[#161b22] border border-[#30363d] rounded text-slate-200"
                  />
                </div>
                <div className="flex items-center text-slate-400 text-[11px]">
                  <span>Will be applied and synchronized across all capable ONVIF / ISAPI / Dahua units.</span>
                </div>
              </div>
            )}

            {actionType === 'NETWORK_CONFIG' && (
              <div className="p-3 bg-[#0d1117] border border-[#21262d] rounded-lg grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                <div>
                  <label className="block text-slate-400 mb-1">PRIMARY DNS</label>
                  <input
                    type="text"
                    value={dns1}
                    onChange={(e) => setDns1(e.target.value)}
                    className="w-full px-3 py-1.5 bg-[#161b22] border border-[#30363d] rounded text-slate-200"
                  />
                </div>
                <div>
                  <label className="block text-slate-400 mb-1">SECONDARY DNS</label>
                  <input
                    type="text"
                    value={dns2}
                    onChange={(e) => setDns2(e.target.value)}
                    className="w-full px-3 py-1.5 bg-[#161b22] border border-[#30363d] rounded text-slate-200"
                  />
                </div>
              </div>
            )}

            {actionType === 'IMAGE_CONFIG' && (
              <div className="p-3 bg-[#0d1117] border border-[#21262d] rounded-lg grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                <div>
                  <label className="block text-slate-400 mb-1">WDR PRESET</label>
                  <select
                    value={wdrMode}
                    onChange={(e) => setWdrMode(e.target.value)}
                    className="w-full px-3 py-1.5 bg-[#161b22] border border-[#30363d] rounded text-slate-200"
                  >
                    <option value="ON">WDR Enabled (Wide Dynamic Range)</option>
                    <option value="OFF">WDR Disabled (Standard)</option>
                    <option value="AUTO">WDR Auto Day/Night Detection</option>
                  </select>
                </div>
                <div>
                  <label className="block text-slate-400 mb-1">TARGET BRIGHTNESS ({brightness}%)</label>
                  <input
                    type="range"
                    min="0"
                    max="100"
                    value={brightness}
                    onChange={(e) => setBrightness(parseInt(e.target.value))}
                    className="w-full accent-emerald-500 mt-2"
                  />
                </div>
              </div>
            )}

            {actionType === 'REBOOT' && (
              <div className="p-4 bg-red-950/30 border border-red-700/50 rounded-lg space-y-2 text-xs">
                <div className="flex items-center space-x-2 text-red-400 font-bold">
                  <ShieldAlert className="w-4 h-4" />
                  <span>HIGH-IMPACT FLEET OPERATION: SCHEDULED REBOOT</span>
                </div>
                <p className="text-slate-300 text-[11px]">
                  Rebooting cameras causes temporary video outage (~60-120s). Type <span className="text-red-300 font-bold">CONFIRM-REBOOT</span> below to unlock execution:
                </p>
                <input
                  type="text"
                  value={rebootConfirmation}
                  onChange={(e) => setRebootConfirmation(e.target.value)}
                  placeholder="CONFIRM-REBOOT"
                  className="w-64 px-3 py-1.5 bg-[#0d1117] border border-red-600 rounded text-red-200 text-xs font-mono uppercase"
                />
              </div>
            )}
          </div>

          {/* Active Job Progress Panel (if job is running or selected) */}
          {currentJob && (
            <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <div className="flex items-center space-x-3">
                    <span className="font-bold text-slate-100 text-sm">{currentJob.name}</span>
                    <span className="px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300 font-bold">
                      {currentJob.action}
                    </span>
                    <span
                      className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                        currentJob.status === 'RUNNING'
                          ? 'bg-blue-500/20 text-blue-400 animate-pulse'
                          : currentJob.status === 'COMPLETED'
                          ? 'bg-emerald-500/20 text-emerald-400'
                          : currentJob.status === 'CANCELLED'
                          ? 'bg-amber-500/20 text-amber-400'
                          : 'bg-red-500/20 text-red-400'
                      }`}
                    >
                      {currentJob.status}
                    </span>
                  </div>
                  <div className="text-[11px] text-slate-400 mt-1">
                    Job ID: <span className="font-mono text-slate-300">{currentJob.id}</span> • Concurrency: {currentJob.concurrency || 4} threads • Rate limit: {currentJob.rate_limit || 5} req/sec
                  </div>
                </div>

                <div className="flex items-center space-x-3">
                  {(currentJob.status === 'RUNNING' || currentJob.status === 'PENDING') && (
                    <button
                      onClick={handleCancelJob}
                      className="px-3 py-1.5 rounded-lg bg-red-600/20 border border-red-500/40 hover:bg-red-600/30 text-red-300 text-xs font-bold flex items-center space-x-1"
                    >
                      <StopCircle className="w-3.5 h-3.5" />
                      <span>ABORT EXECUTION</span>
                    </button>
                  )}
                </div>
              </div>

              {/* Progress Bar */}
              <div className="space-y-1">
                <div className="flex justify-between text-xs text-slate-400">
                  <span>Progress: {currentJob.progress || 0}%</span>
                  <span>
                    Success: <strong className="text-emerald-400">{currentJob.succeeded || 0}</strong> • Failed: <strong className="text-red-400">{currentJob.failed || 0}</strong> • Skipped: <strong className="text-amber-400">{currentJob.skipped || 0}</strong> of {currentJob.total || currentJob.target_ids.length}
                  </span>
                </div>
                <div className="w-full h-2.5 bg-[#0d1117] rounded-full overflow-hidden border border-[#21262d]">
                  <div
                    className={`h-full transition-all duration-300 ${
                      currentJob.status === 'FAILED'
                        ? 'bg-red-500'
                        : currentJob.status === 'CANCELLED'
                        ? 'bg-amber-500'
                        : 'bg-emerald-500'
                    }`}
                    style={{ width: `${currentJob.progress || 0}%` }}
                  />
                </div>
              </div>

              {/* Device Results Table */}
              {currentJob.results && currentJob.results.length > 0 && (
                <div className="mt-3 border border-[#21262d] rounded-lg overflow-hidden">
                  <table className="w-full text-xs text-left">
                    <thead className="bg-[#0d1117] text-slate-400 font-bold border-b border-[#21262d]">
                      <tr>
                        <th className="p-2.5">TARGET DEVICE</th>
                        <th className="p-2.5">IP</th>
                        <th className="p-2.5">STATUS</th>
                        <th className="p-2.5">LATENCY</th>
                        <th className="p-2.5">DIFF / RESULT DETAILS</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-[#21262d]">
                      {currentJob.results.map((res) => (
                        <tr key={res.device_id} className="hover:bg-[#1f242c]">
                          <td className="p-2.5 font-bold text-slate-200">{res.device_name || res.device_id}</td>
                          <td className="p-2.5 text-slate-400 font-mono">{res.ip}</td>
                          <td className="p-2.5">
                            <span
                              className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                                res.status === 'SUCCESS'
                                  ? 'bg-emerald-500/20 text-emerald-400'
                                  : res.status === 'UNSUPPORTED'
                                  ? 'bg-amber-500/20 text-amber-400'
                                  : res.status === 'CANCELLED'
                                  ? 'bg-slate-700 text-slate-300'
                                  : 'bg-red-500/20 text-red-400'
                              }`}
                            >
                              {res.status}
                            </span>
                          </td>
                          <td className="p-2.5 text-slate-400 font-mono">{res.duration_ms}ms</td>
                          <td className="p-2.5 text-[11px] text-slate-300 font-mono truncate max-w-md">
                            {res.diff || res.details || res.error || 'OK'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}

          {/* Target Hardware Selector */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
              <div>
                <span className="font-bold text-slate-200 uppercase text-xs">
                  FLEET TARGET HARDWARE SELECTION ({selectedIds.length} of {devices.length} selected)
                </span>
                <p className="text-[11px] text-slate-400">Select devices for batch execution. Capability matrix will validate compatibility before dispatch.</p>
              </div>

              {/* Filters */}
              <div className="flex items-center space-x-2 text-xs">
                <div className="relative">
                  <Search className="w-3.5 h-3.5 absolute left-2.5 top-2.5 text-slate-500" />
                  <input
                    type="text"
                    value={searchFilter}
                    onChange={(e) => setSearchFilter(e.target.value)}
                    placeholder="Filter by IP, name..."
                    className="pl-8 pr-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 text-xs"
                  />
                </div>

                <select
                  value={brandFilter}
                  onChange={(e) => setBrandFilter(e.target.value)}
                  className="px-2.5 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 text-xs"
                >
                  <option value="ALL">All Brands</option>
                  <option value="Hikvision">Hikvision</option>
                  <option value="Dahua">Dahua</option>
                  <option value="CP PLUS">CP PLUS</option>
                  <option value="Uniview">Uniview</option>
                </select>

                <button
                  onClick={() => setSelectedIds(selectedIds.length === devices.length ? [] : devices.map((d) => d.id))}
                  className="px-3 py-1.5 rounded-lg bg-[#0d1117] border border-[#30363d] text-emerald-400 hover:text-emerald-300"
                >
                  {selectedIds.length === devices.length ? 'Clear All' : 'Select All'}
                </button>
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2.5 text-xs">
              {filteredDevices.map((dev) => {
                const isChecked = selectedIds.includes(dev.id);
                return (
                  <label
                    key={dev.id}
                    className={`flex items-start space-x-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                      isChecked
                        ? 'bg-emerald-950/30 border-emerald-700/60 text-slate-100'
                        : 'bg-[#0d1117] border-[#30363d] text-slate-400 hover:border-slate-500'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={isChecked}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedIds([...selectedIds, dev.id]);
                        } else {
                          setSelectedIds(selectedIds.filter((id) => id !== dev.id));
                        }
                      }}
                      className="accent-emerald-500 mt-1"
                    />
                    <div className="overflow-hidden">
                      <div className="font-bold text-slate-200 truncate">{dev.name}</div>
                      <div className="text-[11px] text-emerald-400 font-mono">{dev.ip}:{dev.port}</div>
                      <div className="text-[10px] text-slate-400 truncate">
                        {dev.manufacturer} • {dev.model || 'Unknown Model'}
                      </div>
                    </div>
                  </label>
                );
              })}
            </div>
          </div>
        </>
      )}

      {/* Comparison Tab */}
      {activeSubTab === 'compare' && (
        <div className="space-y-4">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
            <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
              <div>
                <h3 className="font-bold text-slate-100 flex items-center space-x-2">
                  <GitCompare className="w-5 h-5 text-emerald-400" />
                  <span>FLEET CONFIGURATION DRIFT DETECTOR</span>
                </h3>
                <p className="text-xs text-slate-400 mt-1">
                  Identifies configuration discrepancies across selected devices (e.g. DNS mismatches, NTP drift, inconsistent imaging).
                </p>
              </div>

              <div className="flex items-center space-x-3 text-xs">
                <select
                  value={compareType}
                  onChange={(e) => setCompareType(e.target.value as any)}
                  className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200"
                >
                  <option value="NETWORK">Network & DNS Settings</option>
                  <option value="IMAGE">Image & WDR Presets</option>
                  <option value="GENERAL">General System & Identification</option>
                </select>

                <button
                  onClick={handleRunCompare}
                  disabled={compareLoading}
                  className="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-bold flex items-center space-x-2"
                >
                  {compareLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                  <span>COMPARE SELECTED ({selectedIds.length})</span>
                </button>
              </div>
            </div>
          </div>

          {compareResult && (
            <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
              <div className="flex items-center justify-between text-xs">
                <span className="font-bold text-slate-200">
                  DEVIATION ANALYSIS ({compareResult.count} Devices Evaluated)
                </span>
                <span className="text-slate-400 font-mono">Type: {compareResult.config_type}</span>
              </div>

              {Object.keys(compareResult.deviations || {}).length > 0 ? (
                <div className="p-3 bg-amber-950/30 border border-amber-700/50 rounded-lg text-xs space-y-1">
                  <div className="font-bold text-amber-400 flex items-center space-x-1">
                    <AlertTriangle className="w-4 h-4" />
                    <span>Configuration Deviations Detected:</span>
                  </div>
                  {Object.entries(compareResult.deviations).map(([key, val]: any) => (
                    <div key={key} className="text-slate-300 font-mono">
                      • <strong className="text-amber-300">{key}:</strong> {JSON.stringify(val)}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="p-3 bg-emerald-950/30 border border-emerald-700/50 rounded-lg text-xs text-emerald-400 flex items-center space-x-2">
                  <CheckCircle className="w-4 h-4" />
                  <span>All selected devices have 100% synchronized and matching configurations.</span>
                </div>
              )}

              {/* Devices Parameter Grid */}
              <div className="border border-[#21262d] rounded-lg overflow-x-auto">
                <table className="w-full text-xs text-left">
                  <thead className="bg-[#0d1117] text-slate-400 border-b border-[#21262d]">
                    <tr>
                      <th className="p-2.5">DEVICE ID</th>
                      <th className="p-2.5">CURRENT VALUES</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[#21262d]">
                    {Object.entries(compareResult.devices || {}).map(([id, cfg]: any) => (
                      <tr key={id} className="hover:bg-[#1f242c]">
                        <td className="p-2.5 font-bold text-slate-200 font-mono">{id}</td>
                        <td className="p-2.5 font-mono text-slate-400 text-[11px]">
                          <pre className="whitespace-pre-wrap">{JSON.stringify(cfg, null, 2)}</pre>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}

      {/* History Tab */}
      {activeSubTab === 'history' && (
        <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
          <div className="p-4 bg-[#0d1117] border-b border-[#30363d] text-xs font-bold text-slate-200 uppercase flex items-center justify-between">
            <span>HISTORICAL BATCH JOBS ({jobs.length} Records)</span>
            <button onClick={loadJobs} className="text-emerald-400 hover:text-emerald-300">
              <RefreshCw className="w-4 h-4" />
            </button>
          </div>

          <div className="divide-y divide-[#21262d] text-xs">
            {jobs.map((job) => (
              <div
                key={job.id}
                onClick={() => {
                  setCurrentJob(job);
                  setActiveSubTab('dispatch');
                }}
                className="p-4 space-y-2 hover:bg-[#1a2029] cursor-pointer transition-colors"
              >
                <div className="flex items-center justify-between">
                  <div>
                    <span className="font-bold text-slate-200">{job.name}</span>
                    <span className="ml-3 px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300">
                      {job.action}
                    </span>
                    <span className="ml-2 text-slate-400 text-[11px]">
                      ({job.target_ids.length} devices)
                    </span>
                  </div>
                  <div className="flex items-center space-x-2">
                    <span className="text-[11px] text-slate-400">
                      {new Date(job.created_at).toLocaleString()}
                    </span>
                    <span
                      className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                        job.status === 'COMPLETED'
                          ? 'bg-emerald-500/20 text-emerald-400'
                          : job.status === 'RUNNING'
                          ? 'bg-blue-500/20 text-blue-400'
                          : 'bg-red-500/20 text-red-400'
                      }`}
                    >
                      {job.status}
                    </span>
                  </div>
                </div>

                <div className="p-2.5 rounded bg-[#0d1117] border border-[#21262d] text-[11px] text-slate-400 space-y-0.5 max-h-24 overflow-y-auto">
                  {job.logs?.map((line, i) => (
                    <div key={i}>• {line}</div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Staged Preview Modal */}
      {showPreviewModal && previewData && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 font-mono">
          <div className="bg-[#161b22] border border-[#30363d] rounded-xl max-w-4xl w-full max-h-[85vh] flex flex-col shadow-2xl overflow-hidden">
            {/* Modal Header */}
            <div className="p-4 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between">
              <div>
                <h3 className="font-bold text-slate-100 flex items-center space-x-2">
                  <Eye className="w-5 h-5 text-emerald-400" />
                  <span>STAGED BATCH PRE-FLIGHT AUDIT & CAPABILITY MATRIX</span>
                </h3>
                <p className="text-xs text-slate-400 mt-0.5">
                  Token: <span className="text-emerald-400">{previewData.preview_token}</span> • TTL: 10 mins • Targets: {previewData.total_targets} ({previewData.eligible_count} Eligible, {previewData.ineligible_count} Ineligible)
                </p>
              </div>
              <button
                onClick={() => setShowPreviewModal(false)}
                className="text-slate-400 hover:text-slate-200 text-lg font-bold"
              >
                ✕
              </button>
            </div>

            {/* Modal Content */}
            <div className="p-4 overflow-y-auto space-y-4 flex-1 text-xs">
              {previewData.reboot_required && (
                <div className="p-3 bg-red-950/40 border border-red-700/60 rounded-lg text-red-300 flex items-center space-x-2">
                  <ShieldAlert className="w-4 h-4 text-red-400" />
                  <span>REBOOT MANDATED: Execution will restart target devices and drop live streaming temporarily.</span>
                </div>
              )}

              <table className="w-full text-left border border-[#21262d] rounded-lg overflow-hidden">
                <thead className="bg-[#0d1117] text-slate-400 border-b border-[#21262d]">
                  <tr>
                    <th className="p-2.5">DEVICE</th>
                    <th className="p-2.5">IP</th>
                    <th className="p-2.5">CAPABILITY CHECK</th>
                    <th className="p-2.5">STATUS</th>
                    <th className="p-2.5">PROPOSED DIFF / WARNINGS</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#21262d]">
                  {previewData.previews.map((prev) => (
                    <tr key={prev.device_id} className="hover:bg-[#1f242c]">
                      <td className="p-2.5 font-bold text-slate-200">
                        {prev.device_name}
                        <div className="text-[10px] text-slate-400 font-normal">{prev.manufacturer} {prev.model}</div>
                      </td>
                      <td className="p-2.5 font-mono text-emerald-400">{prev.ip}</td>
                      <td className="p-2.5">
                        {prev.capable ? (
                          <span className="flex items-center space-x-1 text-emerald-400 text-[11px]">
                            <CheckCircle className="w-3.5 h-3.5" />
                            <span>Supported</span>
                          </span>
                        ) : (
                          <span className="flex items-center space-x-1 text-amber-400 text-[11px]">
                            <AlertTriangle className="w-3.5 h-3.5" />
                            <span>Missing: {prev.missing_capabilities.join(', ')}</span>
                          </span>
                        )}
                      </td>
                      <td className="p-2.5">
                        <span
                          className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                            prev.status === 'ELIGIBLE'
                              ? 'bg-emerald-500/20 text-emerald-400'
                              : 'bg-amber-500/20 text-amber-400'
                          }`}
                        >
                          {prev.status}
                        </span>
                      </td>
                      <td className="p-2.5 text-[11px] font-mono text-slate-300">
                        {prev.diff ? (
                          <div className="bg-[#0d1117] p-1.5 rounded border border-[#21262d] text-emerald-300">
                            {prev.diff}
                          </div>
                        ) : prev.warnings && prev.warnings.length > 0 ? (
                          <div className="text-amber-400">{prev.warnings.join('; ')}</div>
                        ) : (
                          <span className="text-slate-500">No diff</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Modal Actions */}
            <div className="p-4 bg-[#0d1117] border-t border-[#30363d] flex items-center justify-between">
              <button
                onClick={() => setShowPreviewModal(false)}
                className="px-4 py-2 rounded-lg bg-[#161b22] border border-[#30363d] text-slate-300 text-xs hover:border-slate-500"
              >
                Cancel & Review Targets
              </button>

              <button
                onClick={handleApproveAndExecute}
                disabled={previewData.eligible_count === 0}
                className="px-5 py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center space-x-2 transition-all shadow-lg shadow-emerald-900/30 disabled:opacity-50"
              >
                <Play className="w-4 h-4" />
                <span>APPROVE & DISPATCH ({previewData.eligible_count} ELIGIBLE DEVICES)</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
