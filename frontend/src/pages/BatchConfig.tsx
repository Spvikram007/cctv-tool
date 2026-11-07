import React, { useState, useEffect } from 'react';
import { Device, BatchJob } from '../types';
import { api } from '../services/api';
import { Layers, RefreshCw, Play } from 'lucide-react';

interface BatchConfigProps {
  devices: Device[];
  onRefresh: () => void;
}

export const BatchConfig: React.FC<BatchConfigProps> = ({ devices, onRefresh }) => {
  const [jobs, setJobs] = useState<BatchJob[]>([]);
  const [jobName, setJobName] = useState('Fleet NTP & Sensor Sync');
  const [actionType, setActionType] = useState('NTP_SYNC');
  const [selectedIds, setSelectedIds] = useState<string[]>(devices.map((d) => d.id));
  const [executing, setExecuting] = useState(false);

  useEffect(() => {
    loadJobs();
  }, []);

  const loadJobs = async () => {
    try {
      const list = await api.getBatchJobs();
      setJobs(list);
    } catch (e) {
      console.error(e);
    }
  };

  const handleCreateJob = async () => {
    if (selectedIds.length === 0) {
      alert('Select at least one device for batch execution.');
      return;
    }
    setExecuting(true);
    try {
      await api.createBatchJob({
        name: jobName,
        action: actionType,
        target_ids: selectedIds,
        payload: { executed_by: 'Technician', broadcast_time: new Date().toISOString() },
      });
      await loadJobs();
      onRefresh();
    } catch (err: any) {
      alert(`Job execution failed: ${err.message}`);
    } finally {
      setExecuting(false);
    }
  };

  return (
    <div className="space-y-6 font-mono">
      {/* Header Bar */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
        <div>
          <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
            <Layers className="w-5 h-5 text-emerald-400" />
            <span>BULK MULTI-CAMERA BATCH CONFIGURATION ENGINE</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Simultaneously push firmware parameters, synchronized time servers, or image profiles across the fleet.
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label className="block text-[11px] text-slate-400 mb-1">BATCH OPERATION NAME</label>
            <input
              type="text"
              value={jobName}
              onChange={(e) => setJobName(e.target.value)}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
            />
          </div>

          <div>
            <label className="block text-[11px] text-slate-400 mb-1">DISPATCH ACTION</label>
            <select
              value={actionType}
              onChange={(e) => setActionType(e.target.value)}
              className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
            >
              <option value="NTP_SYNC">Broadcast NTP Time Sync (pool.ntp.org)</option>
              <option value="IMAGE_PRESET">Apply Standard Low-Light WDR Image Preset</option>
              <option value="NETWORK_AUDIT">Force Interface Verification & Re-probe</option>
            </select>
          </div>

          <div className="flex items-end">
            <button
              onClick={handleCreateJob}
              disabled={executing}
              className="w-full py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center justify-center space-x-2 transition-all shadow-lg shadow-emerald-900/30"
            >
              {executing ? (
                <>
                  <RefreshCw className="w-4 h-4 animate-spin" />
                  <span>DISPATCHING TO FLEET...</span>
                </>
              ) : (
                <>
                  <Play className="w-4 h-4" />
                  <span>EXECUTE BATCH JOB</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>

      {/* Target Devices Selection */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-bold text-slate-200 uppercase">SELECT TARGET HARDWARE ({selectedIds.length} of {devices.length} selected)</span>
          <button
            onClick={() => setSelectedIds(selectedIds.length === devices.length ? [] : devices.map((d) => d.id))}
            className="text-emerald-400 hover:text-emerald-300"
          >
            {selectedIds.length === devices.length ? 'Deselect All' : 'Select All'}
          </button>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2 text-xs">
          {devices.map((dev) => {
            const isChecked = selectedIds.includes(dev.id);
            return (
              <label
                key={dev.id}
                className={`flex items-center space-x-2 p-2.5 rounded-lg border cursor-pointer transition-colors ${
                  isChecked
                    ? 'bg-emerald-950/40 border-emerald-700/60 text-slate-100'
                    : 'bg-[#0d1117] border-[#30363d] text-slate-400'
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
                  className="accent-emerald-500"
                />
                <span className="truncate">{dev.name} ({dev.ip})</span>
              </label>
            );
          })}
        </div>
      </div>

      {/* Batch History & Execution Logs */}
      <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
        <div className="p-4 bg-[#0d1117] border-b border-[#30363d] text-xs font-bold text-slate-200 uppercase">
          BATCH EXECUTION HISTORY & VERIFIED LOGS
        </div>

        <div className="divide-y divide-[#21262d] text-xs">
          {jobs.map((job) => (
            <div key={job.id} className="p-4 space-y-2">
              <div className="flex items-center justify-between">
                <div>
                  <span className="font-bold text-slate-200">{job.name}</span>
                  <span className="ml-3 px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300">
                    {job.action}
                  </span>
                </div>
                <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400">
                  {job.status}
                </span>
              </div>

              <div className="p-2.5 rounded bg-[#0d1117] border border-[#21262d] text-[11px] text-slate-400 space-y-0.5">
                {job.logs?.map((line, i) => (
                  <div key={i}>• {line}</div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
