import React, { useState, useEffect } from 'react';
import {
  Device, RecorderProfile, RecorderChannelInfo,
  RecordingSchedule, RecordingSegment, PlaybackSession, StorageHealthReport,
  DiskFormatPreview, AnalogFormat, RecordMode
} from '../types';
import { api } from '../services/api';
import {
  Server, HardDrive, Cpu, CheckCircle, AlertTriangle, Play, Pause,
  Square, FastForward, Rewind, Calendar, Clock, Grid, Video,
  Layers, Activity, Download, Upload, RefreshCw,
  Sliders, Database, Search, PlusCircle, Film,
  ChevronRight, ChevronLeft, ChevronUp, ChevronDown, Radio, AlertOctagon, Terminal
} from 'lucide-react';

interface RecorderManagerProps {
  devices: Device[];
  selectedDevice: Device | null;
  onSelectDevice: (dev: Device) => void;
}

type TabType =
  | 'overview'
  | 'matrix'
  | 'analog'
  | 'ip_registration'
  | 'schedule'
  | 'playback'
  | 'multi_playback'
  | 'storage'
  | 'calculator'
  | 'backup';

export const RecorderManager: React.FC<RecorderManagerProps> = ({
  devices,
  selectedDevice,
  onSelectDevice,
}) => {
  // Filter for recorders or multi-channel devices
  const recorders = devices.filter(
    (d) =>
      d.category === 'XVR' ||
      d.category === 'DVR' ||
      d.category === 'NVR' ||
      d.category === 'HVR' ||
      d.category === 'HYBRID_RECORDER' ||
      d.channel_count > 1
  );

  const currentDev =
    selectedDevice && recorders.some((r) => r.id === selectedDevice.id)
      ? selectedDevice
      : recorders[0] || devices[0];

  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [loading, setLoading] = useState(false);
  const [statusMsg, setStatusMsg] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  // Recorder Profile & Channels
  const [profile, setProfile] = useState<RecorderProfile | null>(null);
  const [channels, setChannels] = useState<RecorderChannelInfo[]>([]);
  const [selectedChannelId, setSelectedChannelId] = useState<number>(1);
  const [matrixLayout, setMatrixLayout] = useState<1 | 4 | 9 | 16>(4);

  // Analog Coaxial UTC
  const [utcLastCommand, setUtcLastCommand] = useState<string>('');

  // IP Camera Registration Form
  const [ipForm, setIpForm] = useState({
    channel_id: 1,
    ip: '192.168.31.101',
    port: 80,
    protocol: 'ONVIF',
    username: 'admin',
    password: '',
    stream_index: 0,
  });

  // Recording Schedule State
  const [schedule, setSchedule] = useState<RecordingSchedule | null>(null);
  const [selectedBrush, setSelectedBrush] = useState<RecordMode>('CONTINUOUS');

  // Playback & Search State
  const [searchParams, setSearchParams] = useState({
    channel_id: 1,
    event_type: 'ALL',
    start_time: new Date(Date.now() - 24 * 3600 * 1000).toISOString().slice(0, 16),
    end_time: new Date().toISOString().slice(0, 16),
  });
  const [segments, setSegments] = useState<RecordingSegment[]>([]);
  const [playbackSession, setPlaybackSession] = useState<PlaybackSession | null>(null);
  const [playbackSpeed, setPlaybackSpeed] = useState<number>(1.0);
  const [playbackProgressPercent, setPlaybackProgressPercent] = useState<number>(25);

  // Storage Health & High-Impact Formatting State
  const [storageHealth, setStorageHealth] = useState<StorageHealthReport | null>(null);
  const [formatModalOpen, setFormatModalOpen] = useState(false);
  const [formatPreview, setFormatPreview] = useState<DiskFormatPreview | null>(null);
  const [formatConfirmationInput, setFormatConfirmationInput] = useState('');

  // Retention Calculator State
  const [calcChannels, setCalcChannels] = useState(8);
  const [calcBitrateKbps, setCalcBitrateKbps] = useState(4096);
  const [calcHoursPerDay, setCalcHoursPerDay] = useState(24);
  const [calcCapacityGB, setCalcCapacityGB] = useState(4000);

  // Backup & Restore
  const [backupJson, setBackupJson] = useState('');

  // Initial Data Load
  useEffect(() => {
    if (currentDev) {
      loadRecorderData(currentDev.id);
    }
  }, [currentDev?.id]);

  const showStatus = (text: string, type: 'success' | 'error' | 'info' = 'success') => {
    setStatusMsg({ type, text });
    setTimeout(() => setStatusMsg(null), 5000);
  };

  const loadRecorderData = async (devId: string) => {
    setLoading(true);
    try {
      const [profData, chData, storData] = await Promise.allSettled([
        api.getRecorderProfile(devId),
        api.getRecorderChannels(devId),
        api.getRecorderStorage(devId),
      ]);

      if (profData.status === 'fulfilled') setProfile(profData.value);
      if (chData.status === 'fulfilled') {
        setChannels(chData.value);
        if (chData.value.length > 0) {
          setSelectedChannelId(chData.value[0].channel_id);
          setIpForm((prev) => ({ ...prev, channel_id: chData.value[0].channel_id }));
          setSearchParams((prev) => ({ ...prev, channel_id: chData.value[0].channel_id }));
        }
      }
      if (storData.status === 'fulfilled') {
        setStorageHealth(storData.value);
        setCalcCapacityGB(storData.value.total_capacity_gb || 4000);
      }
    } catch (err: any) {
      showStatus(`Error loading recorder: ${err.message}`, 'error');
    } finally {
      setLoading(false);
    }
  };

  // Switch Analog Format via Coaxial UTC
  const handleFormatChange = async (chId: number, format: AnalogFormat) => {
    if (!currentDev) return;
    try {
      const res = await api.setRecorderAnalogFormat(currentDev.id, chId, format);
      showStatus(res.message, res.success ? 'success' : 'error');
      // Reload channels
      const updated = await api.getRecorderChannels(currentDev.id);
      setChannels(updated);
    } catch (err: any) {
      showStatus(`UTC Format switch failed: ${err.message}`, 'error');
    }
  };

  // Coaxial UTC PTZ / OSD Command
  const handleCoaxialCmd = async (command: string) => {
    if (!currentDev) return;
    try {
      setUtcLastCommand(command);
      const res = await api.sendRecorderCoaxialControl(currentDev.id, selectedChannelId, command);
      showStatus(`UTC Coax: ${res.command} (${res.message})`, 'info');
    } catch (err: any) {
      showStatus(`UTC Control failed: ${err.message}`, 'error');
    }
  };

  // Register IP Camera to Channel
  const handleRegisterIP = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentDev) return;
    try {
      const res = await api.registerRecorderIPCamera(currentDev.id, ipForm);
      showStatus(res.message, 'success');
      const updated = await api.getRecorderChannels(currentDev.id);
      setChannels(updated);
    } catch (err: any) {
      showStatus(`IP Camera Registration failed: ${err.message}`, 'error');
    }
  };

  // Load Schedule
  const loadSchedule = async (chId: number) => {
    if (!currentDev) return;
    try {
      const sched = await api.getRecorderSchedule(currentDev.id, chId);
      setSchedule(sched);
    } catch (err: any) {
      showStatus(`Schedule load failed: ${err.message}`, 'error');
    }
  };

  useEffect(() => {
    if (activeTab === 'schedule' && currentDev) {
      loadSchedule(selectedChannelId);
    }
  }, [activeTab, selectedChannelId, currentDev?.id]);

  // Update Schedule Matrix Cell
  const handleToggleScheduleCell = (day: number, hour: number) => {
    if (!schedule) return;
    const newWeekly = { ...schedule.weekly_schedule };
    const dayHours = [...(newWeekly[day] || Array(24).fill('CONTINUOUS'))];
    dayHours[hour] = selectedBrush;
    newWeekly[day] = dayHours;
    setSchedule({ ...schedule, weekly_schedule: newWeekly });
  };

  // Save Schedule
  const handleSaveSchedule = async () => {
    if (!currentDev || !schedule) return;
    try {
      const res = await api.updateRecorderSchedule(currentDev.id, schedule);
      setSchedule(res);
      showStatus(`Schedule saved for CH ${schedule.channel_id} (${schedule.channel_name})`, 'success');
    } catch (err: any) {
      showStatus(`Failed to save schedule: ${err.message}`, 'error');
    }
  };

  // Search Recordings
  const handleSearchRecordings = async () => {
    if (!currentDev) return;
    try {
      const res = await api.searchRecordings(
        currentDev.id,
        searchParams.channel_id,
        searchParams.start_time ? new Date(searchParams.start_time).toISOString() : undefined,
        searchParams.end_time ? new Date(searchParams.end_time).toISOString() : undefined,
        searchParams.event_type
      );
      setSegments(res);
      showStatus(`Found ${res.length} recording segments`, 'info');
    } catch (err: any) {
      showStatus(`Search failed: ${err.message}`, 'error');
    }
  };

  // Start Playback Session
  const handleStartPlayback = async (segment?: RecordingSegment) => {
    if (!currentDev) return;
    const ch = segment ? segment.channel_id : searchParams.channel_id;
    const st = segment ? segment.start_time : new Date(searchParams.start_time).toISOString();
    const et = segment ? segment.end_time : new Date(searchParams.end_time).toISOString();
    try {
      const session = await api.createPlaybackSession(currentDev.id, ch, st, et, playbackSpeed);
      setPlaybackSession(session);
      showStatus(`Playback session started (${session.session_id})`, 'success');
    } catch (err: any) {
      showStatus(`Playback session failed: ${err.message}`, 'error');
    }
  };

  // Export Recording
  const handleExportRecording = async (segId: string) => {
    if (!currentDev) return;
    const path = `C:\\CCTV_Exports\\export_${segId}.mp4`;
    try {
      const res = await api.exportRecording(currentDev.id, segId, path);
      showStatus(`Recording exported to ${res.output_path} (${(res.size_bytes / (1024 * 1024)).toFixed(1)} MB)`, 'success');
    } catch (err: any) {
      showStatus(`Export failed: ${err.message}`, 'error');
    }
  };

  // Stage Format Disk Preview
  const handleStageFormatDisk = async (diskIdx: number) => {
    if (!currentDev) return;
    try {
      const preview = await api.stageFormatDiskPreview(currentDev.id, diskIdx);
      setFormatPreview(preview);
      setFormatConfirmationInput('');
      setFormatModalOpen(true);
    } catch (err: any) {
      showStatus(`Format preview failed: ${err.message}`, 'error');
    }
  };

  // Approve Format Disk
  const handleApproveFormatDisk = async () => {
    if (!currentDev || !formatPreview) return;
    if (formatConfirmationInput.trim() !== 'CONFIRM FORMAT') {
      alert('You must type exactly "CONFIRM FORMAT" to approve disk formatting.');
      return;
    }
    try {
      const res = await api.approveFormatDisk(currentDev.id, formatPreview.format_token, formatConfirmationInput.trim());
      showStatus(res.message, 'success');
      setFormatModalOpen(false);
      setFormatPreview(null);
      // Reload storage
      const updated = await api.getRecorderStorage(currentDev.id);
      setStorageHealth(updated);
    } catch (err: any) {
      showStatus(`Disk formatting failed: ${err.message}`, 'error');
    }
  };

  // Channel Mappings Export / Import
  const handleExportMappings = () => {
    const backupData = {
      recorder_id: currentDev?.id,
      timestamp: new Date().toISOString(),
      channels: channels,
    };
    const blob = new Blob([JSON.stringify(backupData, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `channel_mappings_${currentDev?.id || 'recorder'}_${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
    showStatus('Channel mappings backup exported successfully', 'success');
  };

  const handleImportMappings = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (evt) => {
      try {
        const text = evt.target?.result as string;
        setBackupJson(text);
        const parsed = JSON.parse(text);
        if (parsed.channels && Array.isArray(parsed.channels)) {
          setChannels(parsed.channels);
          showStatus(`Imported ${parsed.channels.length} channel configurations`, 'success');
        } else {
          showStatus('Invalid mappings format: "channels" array missing', 'error');
        }
      } catch (err: any) {
        showStatus(`Import parse error: ${err.message}`, 'error');
      }
    };
    reader.readAsText(file);
  };

  // Storage Calculator Formula
  // Storage_GB = (Channels * Bitrate_kbps * 3600 * Hours) / (8 * 1024 * 1024)
  const calcDailyStorageGB = (calcChannels * calcBitrateKbps * 3600 * calcHoursPerDay) / (8 * 1024 * 1024);
  const calcRetentionDays = calcDailyStorageGB > 0 ? calcCapacityGB / calcDailyStorageGB : 0;

  if (!currentDev) {
    return (
      <div className="p-8 text-center text-slate-500 font-mono">
        No hybrid DVR/NVR/XVR recorder devices available. Connect or discover a recorder on the LAN.
      </div>
    );
  }

  const isPureNVR =
    profile?.recorder_type === 'NVR' ||
    (profile && profile.physical_analog_channels === 0) ||
    currentDev.category === 'NVR';

  return (
    <div className="space-y-6">
      {/* Top Header & Device Selector */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-10 w-10 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Server className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">
                {profile?.name || currentDev.name}
              </h2>
              <span
                className={`px-2 py-0.5 rounded text-[10px] font-bold font-mono ${
                  profile?.recorder_type === 'XVR'
                    ? 'bg-purple-500/20 text-purple-400 border border-purple-500/30'
                    : profile?.recorder_type === 'NVR'
                    ? 'bg-blue-500/20 text-blue-400 border border-blue-500/30'
                    : 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                }`}
              >
                {profile?.recorder_type || currentDev.category}
              </span>
              <span className="text-xs text-slate-400 font-mono">{currentDev.ip}:{currentDev.port || 80}</span>
            </div>
            <p className="text-[11px] text-slate-400 font-mono">
              ONE TOOL. EVERY CCTV. COMPLETE CONTROL. — Unified Channel, UTC Coaxial & Recording Engine
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={() => loadRecorderData(currentDev.id)}
            disabled={loading}
            className="px-3 py-1.5 bg-[#21262d] hover:bg-[#30363d] text-slate-200 border border-[#30363d] rounded-lg text-xs font-mono flex items-center space-x-1.5 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>

          <label className="text-xs font-mono text-slate-400">RECORDER UNIT:</label>
          <select
            value={currentDev.id}
            onChange={(e) => {
              const d = devices.find((x) => x.id === e.target.value);
              if (d) onSelectDevice(d);
            }}
            className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
          >
            {recorders.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name} ({d.category} - {d.ip})
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* Global Status Notification */}
      {statusMsg && (
        <div
          className={`p-3 rounded-xl border text-xs font-mono flex items-center justify-between transition-all ${
            statusMsg.type === 'success'
              ? 'bg-emerald-950/60 border-emerald-800 text-emerald-300'
              : statusMsg.type === 'error'
              ? 'bg-red-950/60 border-red-800 text-red-300'
              : 'bg-blue-950/60 border-blue-800 text-blue-300'
          }`}
        >
          <div className="flex items-center space-x-2">
            {statusMsg.type === 'success' && <CheckCircle className="w-4 h-4 text-emerald-400" />}
            {statusMsg.type === 'error' && <AlertTriangle className="w-4 h-4 text-red-400" />}
            {statusMsg.type === 'info' && <Activity className="w-4 h-4 text-blue-400" />}
            <span>{statusMsg.text}</span>
          </div>
          <button onClick={() => setStatusMsg(null)} className="text-slate-400 hover:text-slate-200 text-xs">✕</button>
        </div>
      )}

      {/* 10 Navigation Tabs */}
      <div className="flex overflow-x-auto border-b border-[#30363d] pb-px gap-1">
        {[
          { id: 'overview', label: '1. Overview', icon: Server },
          { id: 'matrix', label: '2. Channel Matrix', icon: Grid },
          { id: 'analog', label: '3. Coaxial UTC & Analog', icon: Radio },
          { id: 'ip_registration', label: '4. IP Camera Registration', icon: PlusCircle },
          { id: 'schedule', label: '5. Recording Schedule', icon: Calendar },
          { id: 'playback', label: '6. Playback & Search', icon: Clock },
          { id: 'multi_playback', label: '7. Multi-Channel Playback', icon: Layers },
          { id: 'storage', label: '8. HDD & Storage Health', icon: HardDrive },
          { id: 'calculator', label: '9. Retention Calculator', icon: Sliders },
          { id: 'backup', label: '10. Mappings Backup', icon: Database },
        ].map((tab) => {
          const Icon = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id as TabType)}
              className={`flex items-center space-x-2 px-3 py-2 text-xs font-mono font-medium rounded-t-lg transition-colors whitespace-nowrap border-b-2 ${
                isActive
                  ? 'border-emerald-500 bg-[#161b22] text-emerald-400 shadow-sm'
                  : 'border-transparent text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
              }`}
            >
              <Icon className="w-3.5 h-3.5" />
              <span>{tab.label}</span>
            </button>
          );
        })}
      </div>

      {/* ========================================================================= */}
      {/* TAB 1: RECORDER OVERVIEW */}
      {/* ========================================================================= */}
      {activeTab === 'overview' && (
        <div className="space-y-6">
          {/* Quick Metrics */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] font-mono">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-slate-400 uppercase">Recorder Type</span>
                <Server className="w-4 h-4 text-purple-400" />
              </div>
              <p className="text-lg font-bold text-slate-100 mt-1">
                {profile?.recorder_type || currentDev.category}
              </p>
              <p className="text-[11px] text-slate-400">
                {profile?.physical_analog_channels || 0} BNC Analog + {profile?.supported_ip_channels || profile?.total_channels || 8} IP
              </p>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] font-mono">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-slate-400 uppercase">Max Resolution</span>
                <Video className="w-4 h-4 text-emerald-400" />
              </div>
              <p className="text-lg font-bold text-slate-100 mt-1">{profile?.max_resolution || '4K Ultra HD'}</p>
              <p className="text-[11px] text-emerald-400">
                {profile?.supported_codecs?.join(', ') || 'H.265+, H.264, MJPEG'}
              </p>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] font-mono">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-slate-400 uppercase">Storage Pool</span>
                <HardDrive className="w-4 h-4 text-blue-400" />
              </div>
              <p className="text-lg font-bold text-slate-100 mt-1">
                {profile?.total_storage_gb ? `${(profile.total_storage_gb / 1000).toFixed(1)} TB` : '4.0 TB'}
              </p>
              <p className="text-[11px] text-blue-400">
                {profile?.disk_count || 1} Disk(s) • SMART Healthy
              </p>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] font-mono">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-slate-400 uppercase">Channel Status</span>
                <Activity className="w-4 h-4 text-amber-400" />
              </div>
              <p className="text-lg font-bold text-slate-100 mt-1">
                {channels.filter((c) => c.connection_status === 'ONLINE').length} / {channels.length || profile?.total_channels || 8} Online
              </p>
              <p className="text-[11px] text-slate-400">
                {channels.filter((c) => c.signal_status === 'SIGNAL_LOST').length} Signal Loss Alert
              </p>
            </div>
          </div>

          {/* Detailed Hardware Spec Matrix */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <h3 className="text-xs font-bold text-slate-200 font-mono uppercase flex items-center space-x-2">
              <Cpu className="w-4 h-4 text-purple-400" />
              <span>Hardware & Protocol Identification Profile</span>
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 font-mono text-xs">
              <div className="p-3 bg-[#0d1117] rounded-lg border border-[#30363d]/60 space-y-1">
                <span className="text-[10px] text-slate-400 uppercase">Manufacturer & Model</span>
                <p className="font-bold text-slate-200">{profile?.manufacturer || currentDev.manufacturer} — {profile?.model || currentDev.model}</p>
                <p className="text-[11px] text-slate-400">HW Rev: {profile?.hardware_revision || 'V2.0'} | FW: {profile?.firmware_version || currentDev.firmware}</p>
              </div>

              <div className="p-3 bg-[#0d1117] rounded-lg border border-[#30363d]/60 space-y-1">
                <span className="text-[10px] text-slate-400 uppercase">Serial Number & Target IP</span>
                <p className="font-bold text-slate-200">{profile?.serial_number || currentDev.serial_number || 'N/A'}</p>
                <p className="text-[11px] text-slate-400">IP: {profile?.ip || currentDev.ip} (Port {profile?.port || currentDev.port || 80})</p>
              </div>

              <div className="p-3 bg-[#0d1117] rounded-lg border border-[#30363d]/60 space-y-1">
                <span className="text-[10px] text-slate-400 uppercase">Coaxial Capabilities</span>
                <div className="flex flex-wrap gap-1 mt-1">
                  <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${profile?.coaxial_control_supported ? 'bg-emerald-500/20 text-emerald-400' : 'bg-slate-800 text-slate-500'}`}>
                    UTC PTZ / OSD
                  </span>
                  <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${profile?.audio_over_coax_supported ? 'bg-blue-500/20 text-blue-400' : 'bg-slate-800 text-slate-500'}`}>
                    Audio Over Coax
                  </span>
                  <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${profile?.ai_analytics_supported ? 'bg-purple-500/20 text-purple-400' : 'bg-slate-800 text-slate-500'}`}>
                    SMD / AI
                  </span>
                </div>
              </div>
            </div>

            {/* Supported Analog Formats */}
            <div className="p-4 bg-[#0d1117] rounded-lg border border-[#30363d]/60 font-mono text-xs">
              <span className="text-[11px] text-slate-400 uppercase font-bold">Supported Signal Modes & Auto-Detection:</span>
              <div className="flex flex-wrap items-center gap-2 mt-2">
                {profile?.supported_analog_formats && profile.supported_analog_formats.length > 0 ? (
                  profile.supported_analog_formats.map((fmt) => (
                    <span key={fmt} className="px-2.5 py-1 rounded bg-[#161b22] border border-[#30363d] text-amber-400 font-bold text-xs">
                      {fmt}
                    </span>
                  ))
                ) : (
                  <span className="text-slate-500">Pure IP NVR — Direct RTSP/ONVIF Profile G Stream Multiplexing</span>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 2: CHANNEL MATRIX & LIVE STATUS */}
      {/* ========================================================================= */}
      {activeTab === 'matrix' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <span className="text-xs font-mono text-slate-400">GRID LAYOUT:</span>
              {([1, 4, 9, 16] as const).map((n) => (
                <button
                  key={n}
                  onClick={() => setMatrixLayout(n)}
                  className={`px-3 py-1 rounded text-xs font-mono font-bold border transition-colors ${
                    matrixLayout === n
                      ? 'bg-emerald-600 text-white border-emerald-500'
                      : 'bg-[#161b22] text-slate-400 border-[#30363d] hover:text-slate-200'
                  }`}
                >
                  {n === 1 ? '1x1' : n === 4 ? '2x2 (4CH)' : n === 9 ? '3x3 (9CH)' : '4x4 (16CH)'}
                </button>
              ))}
            </div>
            <span className="text-xs font-mono text-slate-400">Total Enumerated: {channels.length} Channels</span>
          </div>

          <div
            className={`grid gap-4 ${
              matrixLayout === 1
                ? 'grid-cols-1'
                : matrixLayout === 4
                ? 'grid-cols-1 md:grid-cols-2'
                : matrixLayout === 9
                ? 'grid-cols-1 md:grid-cols-3'
                : 'grid-cols-1 sm:grid-cols-2 md:grid-cols-4'
            }`}
          >
            {channels.slice(0, matrixLayout).map((ch) => (
              <div
                key={ch.channel_id}
                onClick={() => setSelectedChannelId(ch.channel_id)}
                className={`p-4 rounded-xl bg-[#161b22] border transition-all cursor-pointer font-mono ${
                  selectedChannelId === ch.channel_id
                    ? 'border-emerald-500 ring-1 ring-emerald-500/50 shadow-lg'
                    : 'border-[#30363d] hover:border-slate-500'
                }`}
              >
                <div className="flex items-center justify-between border-b border-[#30363d] pb-2 mb-3">
                  <div className="flex items-center space-x-2">
                    <span className="font-bold text-slate-200 text-sm">CH {ch.channel_number}</span>
                    <span className="text-xs text-slate-400 truncate max-w-[130px]">{ch.name}</span>
                  </div>
                  <span
                    className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                      ch.connection_status === 'ONLINE'
                        ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                        : ch.connection_status === 'NO_SIGNAL'
                        ? 'bg-amber-500/20 text-amber-400 border border-amber-500/30'
                        : 'bg-red-500/20 text-red-400 border border-red-500/30'
                    }`}
                  >
                    {ch.connection_status}
                  </span>
                </div>

                <div className="space-y-1.5 text-xs">
                  <div className="flex justify-between">
                    <span className="text-slate-400">Interface:</span>
                    <span className={`font-bold ${ch.channel_type === 'ANALOG' ? 'text-purple-400' : 'text-blue-400'}`}>
                      {ch.channel_type} {ch.signal_format ? `(${ch.signal_format})` : ''}
                    </span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-slate-400">Resolution:</span>
                    <span className="text-slate-200 font-bold">{ch.resolution || '1080P'} @ {ch.frame_rate || 25}fps</span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-slate-400">Bitrate:</span>
                    <span className="text-slate-200">{ch.bitrate_kbps || 4096} kbps ({ch.encoding_format || 'H.265'})</span>
                  </div>

                  <div className="flex justify-between">
                    <span className="text-slate-400">Record Mode:</span>
                    <span className="text-emerald-400 font-bold uppercase">{ch.recording_status}</span>
                  </div>

                  {ch.connected_camera_ip && (
                    <div className="flex justify-between">
                      <span className="text-slate-400">IP Target:</span>
                      <span className="text-slate-300 font-mono">{ch.connected_camera_ip}</span>
                    </div>
                  )}
                </div>

                <div className="mt-3 pt-2 border-t border-[#30363d] flex items-center justify-between text-[11px]">
                  <span className={`flex items-center space-x-1 ${ch.signal_status === 'SIGNAL_LOCKED' ? 'text-emerald-400' : 'text-amber-400'}`}>
                    <Activity className="w-3 h-3" />
                    <span>{ch.signal_status}</span>
                  </span>
                  <div className="flex items-center space-x-2">
                    {ch.audio_enabled && <span className="text-blue-400 font-bold">AUDIO</span>}
                    {ch.ptz_supported && <span className="text-purple-400 font-bold">PTZ</span>}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 3: COAXIAL UTC & ANALOG COMPATIBILITY */}
      {/* ========================================================================= */}
      {activeTab === 'analog' && (
        <div className="space-y-6">
          {/* Pure NVR Warning Guard */}
          {isPureNVR ? (
            <div className="p-4 rounded-xl bg-amber-950/40 border border-amber-800 text-amber-300 font-mono text-xs flex items-start space-x-3">
              <AlertOctagon className="w-5 h-5 text-amber-400 mt-0.5 flex-shrink-0" />
              <div>
                <p className="font-bold uppercase">Pure IP Network Video Recorder Detected</p>
                <p className="text-[11px] text-amber-400/90 mt-1">
                  This unit ({profile?.model || currentDev.model}) is a dedicated IP NVR with 0 physical BNC analog inputs.
                  Analog format switching (CVI / TVI / AHD / CVBS) and Coaxial UTC control are strictly reserved for Hybrid DVR / XVR hardware.
                  All camera connections must be managed via digital IP Camera Registration (Tab 4).
                </p>
              </div>
            </div>
          ) : (
            <div className="p-4 rounded-xl bg-purple-950/40 border border-purple-800 text-purple-300 font-mono text-xs flex items-start space-x-3">
              <Radio className="w-5 h-5 text-purple-400 mt-0.5 flex-shrink-0" />
              <div>
                <p className="font-bold uppercase">HD-over-Coax UTC Protocol Active</p>
                <p className="text-[11px] text-purple-400/90 mt-1">
                  Commands and format changes are transmitted directly across the RG59/RG6 coaxial cable using embedded FSK / UTC modulation signals without requiring separate RS-485 telemetry wiring.
                </p>
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 font-mono">
            {/* Left: Channel Format Switcher */}
            <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
              <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
                <Sliders className="w-4 h-4 text-emerald-400" />
                <span>Coaxial Video Format Switcher</span>
              </h3>

              <div className="space-y-3">
                <label className="text-xs text-slate-400">Select BNC Channel:</label>
                <select
                  value={selectedChannelId}
                  onChange={(e) => setSelectedChannelId(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
                  disabled={isPureNVR}
                >
                  {channels
                    .filter((c) => c.channel_type === 'ANALOG' || !isPureNVR)
                    .map((c) => (
                      <option key={c.channel_id} value={c.channel_id}>
                        CH {c.channel_number}: {c.name} (Current: {c.signal_format || 'AUTO'})
                      </option>
                    ))}
                </select>
              </div>

              <div className="pt-2">
                <label className="text-xs text-slate-400 block mb-2">Switch Signal Mode (Coaxial UTC):</label>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                  {(['CVI', 'TVI', 'AHD', 'CVBS', 'AUTO'] as const).map((fmt) => {
                    const currentFormat = channels.find((c) => c.channel_id === selectedChannelId)?.signal_format;
                    const isSelected = currentFormat === fmt;
                    return (
                      <button
                        key={fmt}
                        onClick={() => handleFormatChange(selectedChannelId, fmt)}
                        disabled={isPureNVR}
                        className={`p-3 rounded-lg text-xs font-bold border flex flex-col items-center justify-center space-y-1 transition-all ${
                          isSelected
                            ? 'bg-emerald-600 text-white border-emerald-500 shadow-md'
                            : isPureNVR
                            ? 'bg-[#0d1117]/50 text-slate-600 border-[#30363d] cursor-not-allowed'
                            : 'bg-[#0d1117] text-slate-300 border-[#30363d] hover:bg-slate-800'
                        }`}
                      >
                        <span className="text-sm">{fmt}</span>
                        <span className="text-[10px] text-slate-400 font-normal">
                          {fmt === 'CVI' ? 'Dahua UTC' : fmt === 'TVI' ? 'Hikvision UTC' : fmt === 'AHD' ? 'Nextchip' : fmt === 'CVBS' ? 'Legacy 960H' : 'Auto Detect'}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>

            {/* Right: Coaxial UTC OSD & PTZ D-Pad */}
            <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
              <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
                <Terminal className="w-4 h-4 text-purple-400" />
                <span>Camera Coaxial OSD Menu & UTC Control</span>
              </h3>

              <p className="text-xs text-slate-400">
                Navigate the analog camera's internal on-screen display (OSD) menu over the coaxial cable:
              </p>

              {/* D-Pad Layout */}
              <div className="flex flex-col items-center justify-center p-4 bg-[#0d1117] rounded-xl border border-[#30363d]">
                <button
                  onClick={() => handleCoaxialCmd('UP')}
                  disabled={isPureNVR}
                  className="w-12 h-10 rounded-lg bg-[#21262d] hover:bg-[#30363d] text-slate-200 border border-[#30363d] flex items-center justify-center transition-colors disabled:opacity-40"
                >
                  <ChevronUp className="w-5 h-5" />
                </button>

                <div className="flex items-center space-x-4 my-2">
                  <button
                    onClick={() => handleCoaxialCmd('LEFT')}
                    disabled={isPureNVR}
                    className="w-12 h-10 rounded-lg bg-[#21262d] hover:bg-[#30363d] text-slate-200 border border-[#30363d] flex items-center justify-center transition-colors disabled:opacity-40"
                  >
                    <ChevronLeft className="w-5 h-5" />
                  </button>

                  <button
                    onClick={() => handleCoaxialCmd('MENU')}
                    disabled={isPureNVR}
                    className="w-16 h-10 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-bold text-xs flex items-center justify-center transition-colors shadow-sm disabled:opacity-40"
                  >
                    MENU
                  </button>

                  <button
                    onClick={() => handleCoaxialCmd('RIGHT')}
                    disabled={isPureNVR}
                    className="w-12 h-10 rounded-lg bg-[#21262d] hover:bg-[#30363d] text-slate-200 border border-[#30363d] flex items-center justify-center transition-colors disabled:opacity-40"
                  >
                    <ChevronRight className="w-5 h-5" />
                  </button>
                </div>

                <button
                  onClick={() => handleCoaxialCmd('DOWN')}
                  disabled={isPureNVR}
                  className="w-12 h-10 rounded-lg bg-[#21262d] hover:bg-[#30363d] text-slate-200 border border-[#30363d] flex items-center justify-center transition-colors disabled:opacity-40"
                >
                  <ChevronDown className="w-5 h-5" />
                </button>

                <div className="flex items-center space-x-3 mt-4">
                  <button
                    onClick={() => handleCoaxialCmd('ENTER')}
                    disabled={isPureNVR}
                    className="px-3 py-1.5 bg-[#21262d] hover:bg-[#30363d] text-slate-200 rounded text-xs font-mono border border-[#30363d] disabled:opacity-40"
                  >
                    ENTER
                  </button>
                  <button
                    onClick={() => handleCoaxialCmd('OSD_EXIT')}
                    disabled={isPureNVR}
                    className="px-3 py-1.5 bg-[#21262d] hover:bg-[#30363d] text-slate-200 rounded text-xs font-mono border border-[#30363d] disabled:opacity-40"
                  >
                    OSD EXIT
                  </button>
                </div>
              </div>

              {utcLastCommand && (
                <p className="text-[11px] text-purple-400 text-center font-mono">
                  Last Sent UTC Command: <span className="font-bold">{utcLastCommand}</span>
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 4: IP CAMERA REGISTRATION */}
      {/* ========================================================================= */}
      {activeTab === 'ip_registration' && (
        <div className="space-y-6">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4 font-mono">
            <div className="flex items-center justify-between border-b border-[#30363d] pb-3">
              <div>
                <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
                  <PlusCircle className="w-4 h-4 text-emerald-400" />
                  <span>Map IP Camera to Recorder Digital Channel</span>
                </h3>
                <p className="text-[11px] text-slate-400 mt-1">
                  Add or re-bind network IP surveillance streams directly into NVR / XVR expansion channels.
                </p>
              </div>
              <span className="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 text-[10px] font-bold border border-emerald-500/30">
                SSRF VALIDATED
              </span>
            </div>

            <form onSubmit={handleRegisterIP} className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 text-xs">
              <div>
                <label className="text-slate-400 block mb-1">Target Recorder Channel:</label>
                <select
                  value={ipForm.channel_id}
                  onChange={(e) => setIpForm({ ...ipForm, channel_id: Number(e.target.value) })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                >
                  {channels.map((c) => (
                    <option key={c.channel_id} value={c.channel_id}>
                      CH {c.channel_number}: {c.name} ({c.channel_type})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Camera IP Address:</label>
                <input
                  type="text"
                  value={ipForm.ip}
                  onChange={(e) => setIpForm({ ...ipForm, ip: e.target.value })}
                  placeholder="192.168.31.101"
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                  required
                />
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Service Port:</label>
                <input
                  type="number"
                  value={ipForm.port}
                  onChange={(e) => setIpForm({ ...ipForm, port: Number(e.target.value) })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                  required
                />
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Registration Protocol:</label>
                <select
                  value={ipForm.protocol}
                  onChange={(e) => setIpForm({ ...ipForm, protocol: e.target.value })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                >
                  <option value="ONVIF">ONVIF Profile S / T</option>
                  <option value="DAHUA_PRIVATE">Dahua Private (DHIP)</option>
                  <option value="HIKVISION_ISAPI">Hikvision ISAPI</option>
                  <option value="RTSP">Direct RTSP Transport</option>
                </select>
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Username:</label>
                <input
                  type="text"
                  value={ipForm.username}
                  onChange={(e) => setIpForm({ ...ipForm, username: e.target.value })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                  required
                />
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Password:</label>
                <input
                  type="password"
                  value={ipForm.password}
                  onChange={(e) => setIpForm({ ...ipForm, password: e.target.value })}
                  placeholder="••••••••"
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                />
              </div>

              <div className="md:col-span-2 lg:col-span-3 flex items-center justify-between pt-2">
                <span className="text-[11px] text-slate-500">
                  Note: Recorder will initiate an authenticated handshake to verify RTSP/ONVIF Profile S token compatibility.
                </span>
                <button
                  type="submit"
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-lg text-xs transition-colors flex items-center space-x-1.5"
                >
                  <PlusCircle className="w-4 h-4" />
                  <span>Register IP Camera</span>
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 5: RECORDING SCHEDULE (7-DAY x 24-HOUR) */}
      {/* ========================================================================= */}
      {activeTab === 'schedule' && (
        <div className="space-y-6 font-mono">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#30363d] pb-3">
              <div className="flex items-center space-x-3">
                <label className="text-xs text-slate-400">CHANNEL:</label>
                <select
                  value={selectedChannelId}
                  onChange={(e) => setSelectedChannelId(Number(e.target.value))}
                  className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
                >
                  {channels.map((c) => (
                    <option key={c.channel_id} value={c.channel_id}>
                      CH {c.channel_number}: {c.name}
                    </option>
                  ))}
                </select>
              </div>

              {/* Brush Selector */}
              <div className="flex items-center space-x-2">
                <span className="text-xs text-slate-400">PAINT BRUSH:</span>
                {(
                  [
                    { mode: 'CONTINUOUS', label: 'Continuous', color: 'bg-emerald-600 text-white' },
                    { mode: 'MOTION', label: 'Motion', color: 'bg-amber-600 text-white' },
                    { mode: 'ALARM', label: 'Alarm', color: 'bg-rose-600 text-white' },
                    { mode: 'DISABLED', label: 'None', color: 'bg-slate-700 text-slate-300' },
                  ] as const
                ).map((b) => (
                  <button
                    key={b.mode}
                    onClick={() => setSelectedBrush(b.mode as RecordMode)}
                    className={`px-2.5 py-1 rounded text-[11px] font-bold transition-all border ${
                      selectedBrush === b.mode
                        ? `${b.color} border-white ring-1 ring-white/50`
                        : 'bg-[#0d1117] text-slate-400 border-[#30363d]'
                    }`}
                  >
                    {b.label}
                  </button>
                ))}
              </div>
            </div>

            {/* 7-Day x 24-Hour Schedule Matrix */}
            <div className="overflow-x-auto">
              <div className="min-w-[700px]">
                {/* Hour Header */}
                <div className="grid grid-cols-[80px_repeat(24,1fr)] gap-1 text-[10px] text-slate-400 text-center mb-1">
                  <span>Day</span>
                  {Array.from({ length: 24 }).map((_, h) => (
                    <span key={h}>{h}</span>
                  ))}
                </div>

                {/* Days Rows */}
                {['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map((dayName, dayIdx) => {
                  const daySchedule = schedule?.weekly_schedule?.[dayIdx] || Array(24).fill('CONTINUOUS');
                  return (
                    <div key={dayIdx} className="grid grid-cols-[80px_repeat(24,1fr)] gap-1 items-center mb-1">
                      <span className="text-xs font-bold text-slate-300">{dayName}</span>
                      {Array.from({ length: 24 }).map((_, hour) => {
                        const cellMode = daySchedule[hour] || 'CONTINUOUS';
                        return (
                          <div
                            key={hour}
                            onClick={() => handleToggleScheduleCell(dayIdx, hour)}
                            className={`h-7 rounded cursor-pointer transition-all border ${
                              cellMode === 'CONTINUOUS'
                                ? 'bg-emerald-500/80 border-emerald-400 hover:bg-emerald-400'
                                : cellMode === 'MOTION'
                                ? 'bg-amber-500/80 border-amber-400 hover:bg-amber-400'
                                : cellMode === 'ALARM'
                                ? 'bg-rose-500/80 border-rose-400 hover:bg-rose-400'
                                : 'bg-[#0d1117] border-[#30363d] hover:bg-slate-800'
                            }`}
                            title={`${dayName} ${hour}:00 - ${cellMode}`}
                          />
                        );
                      })}
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Pre-record and Post-record Sliders */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-4 border-t border-[#30363d] text-xs">
              <div>
                <div className="flex justify-between mb-1">
                  <span className="text-slate-400">Pre-Record Buffer (0–30s):</span>
                  <span className="font-bold text-emerald-400">{schedule?.pre_record_sec ?? 5} seconds</span>
                </div>
                <input
                  type="range"
                  min="0"
                  max="30"
                  value={schedule?.pre_record_sec ?? 5}
                  onChange={(e) =>
                    schedule && setSchedule({ ...schedule, pre_record_sec: Number(e.target.value) })
                  }
                  className="w-full accent-emerald-500 cursor-pointer"
                />
              </div>

              <div>
                <div className="flex justify-between mb-1">
                  <span className="text-slate-400">Post-Record Hold (10–300s):</span>
                  <span className="font-bold text-emerald-400">{schedule?.post_record_sec ?? 30} seconds</span>
                </div>
                <input
                  type="range"
                  min="10"
                  max="300"
                  step="10"
                  value={schedule?.post_record_sec ?? 30}
                  onChange={(e) =>
                    schedule && setSchedule({ ...schedule, post_record_sec: Number(e.target.value) })
                  }
                  className="w-full accent-emerald-500 cursor-pointer"
                />
              </div>
            </div>

            {/* Quick Actions & Save Button */}
            <div className="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-[#30363d]">
              <div className="flex items-center space-x-2">
                <button
                  onClick={() => {
                    if (!schedule) return;
                    const allCont: Record<number, string[]> = {};
                    for (let d = 0; d < 7; d++) allCont[d] = Array(24).fill('CONTINUOUS');
                    setSchedule({ ...schedule, weekly_schedule: allCont });
                  }}
                  className="px-3 py-1 bg-[#21262d] hover:bg-[#30363d] text-slate-300 rounded text-xs border border-[#30363d]"
                >
                  Set All 24x7 Continuous
                </button>
                <button
                  onClick={() => {
                    if (!schedule) return;
                    const allMotion: Record<number, string[]> = {};
                    for (let d = 0; d < 7; d++) allMotion[d] = Array(24).fill('MOTION');
                    setSchedule({ ...schedule, weekly_schedule: allMotion });
                  }}
                  className="px-3 py-1 bg-[#21262d] hover:bg-[#30363d] text-slate-300 rounded text-xs border border-[#30363d]"
                >
                  Set All Motion Only
                </button>
              </div>

              <button
                onClick={handleSaveSchedule}
                className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-lg text-xs flex items-center space-x-1.5 transition-colors"
              >
                <CheckCircle className="w-4 h-4" />
                <span>Save Schedule Configuration</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 6: PLAYBACK & RECORDING SEARCH */}
      {/* ========================================================================= */}
      {activeTab === 'playback' && (
        <div className="space-y-6 font-mono">
          {/* Search Bar */}
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 text-xs">
              <div>
                <label className="text-slate-400 block mb-1">Search Channel:</label>
                <select
                  value={searchParams.channel_id}
                  onChange={(e) => setSearchParams({ ...searchParams, channel_id: Number(e.target.value) })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                >
                  {channels.map((c) => (
                    <option key={c.channel_id} value={c.channel_id}>
                      CH {c.channel_number}: {c.name}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Event Type Filter:</label>
                <select
                  value={searchParams.event_type}
                  onChange={(e) => setSearchParams({ ...searchParams, event_type: e.target.value })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                >
                  <option value="ALL">All Events (Continuous + Alarms)</option>
                  <option value="CONTINUOUS">Continuous Regular Only</option>
                  <option value="MOTION">Motion Detection</option>
                  <option value="ALARM">Alarm Sensor Trigger</option>
                </select>
              </div>

              <div>
                <label className="text-slate-400 block mb-1">From Date/Time:</label>
                <input
                  type="datetime-local"
                  value={searchParams.start_time}
                  onChange={(e) => setSearchParams({ ...searchParams, start_time: e.target.value })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                />
              </div>

              <div>
                <label className="text-slate-400 block mb-1">To Date/Time:</label>
                <input
                  type="datetime-local"
                  value={searchParams.end_time}
                  onChange={(e) => setSearchParams({ ...searchParams, end_time: e.target.value })}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                />
              </div>
            </div>

            <div className="flex justify-end pt-2 border-t border-[#30363d]">
              <button
                onClick={handleSearchRecordings}
                className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-lg text-xs flex items-center space-x-1.5 transition-colors"
              >
                <Search className="w-4 h-4" />
                <span>Search Recordings</span>
              </button>
            </div>
          </div>

          {/* Interactive Playback Scrubber & Transport Controls */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="flex items-center justify-between">
              <span className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
                <Film className="w-4 h-4 text-emerald-400" />
                <span>Interactive Timeline & Scrubber</span>
              </span>
              {playbackSession && (
                <span className="text-[11px] text-emerald-400 font-bold">
                  TOKEN: {playbackSession.playback_token.slice(0, 18)}... (Active)
                </span>
              )}
            </div>

            {/* Visual Timeline Bar with Segments */}
            <div className="relative h-10 bg-[#0d1117] rounded-lg border border-[#30363d] overflow-hidden p-1 flex items-center">
              {/* Segments Visualization */}
              {segments.length > 0 ? (
                segments.map((seg, idx) => (
                  <div
                    key={seg.id || idx}
                    className={`h-full mx-0.5 rounded cursor-pointer transition-transform hover:scale-y-110 flex-1 ${
                      seg.event_type === 'MOTION'
                        ? 'bg-amber-500/80 border border-amber-400'
                        : seg.event_type === 'ALARM'
                        ? 'bg-rose-500/80 border border-rose-400'
                        : 'bg-emerald-500/80 border border-emerald-400'
                    }`}
                    title={`${seg.event_type}: ${new Date(seg.start_time).toLocaleTimeString()} - ${new Date(seg.end_time).toLocaleTimeString()}`}
                    onClick={() => handleStartPlayback(seg)}
                  />
                ))
              ) : (
                <div className="w-full text-center text-xs text-slate-500">
                  No segments loaded. Click "Search Recordings" to populate timeline.
                </div>
              )}

              {/* Scrubber Cursor */}
              <div
                className="absolute top-0 bottom-0 w-1 bg-white shadow-lg pointer-events-none transition-all"
                style={{ left: `${playbackProgressPercent}%` }}
              />
            </div>

            {/* Scrubber Slider */}
            <input
              type="range"
              min="0"
              max="100"
              value={playbackProgressPercent}
              onChange={(e) => setPlaybackProgressPercent(Number(e.target.value))}
              className="w-full accent-emerald-500 cursor-pointer"
            />

            {/* Transport Bar */}
            <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
              <div className="flex items-center space-x-2">
                <button
                  onClick={() => setPlaybackProgressPercent(Math.max(0, playbackProgressPercent - 5))}
                  className="p-2 bg-[#21262d] hover:bg-[#30363d] text-slate-200 rounded border border-[#30363d]"
                  title="Rewind 10s"
                >
                  <Rewind className="w-4 h-4" />
                </button>
                <button
                  onClick={() => handleStartPlayback()}
                  className="px-3 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded font-bold text-xs flex items-center space-x-1"
                >
                  <Play className="w-4 h-4" />
                  <span>Play</span>
                </button>
                <button
                  onClick={() => showStatus('Playback paused', 'info')}
                  className="p-2 bg-[#21262d] hover:bg-[#30363d] text-slate-200 rounded border border-[#30363d]"
                  title="Pause"
                >
                  <Pause className="w-4 h-4" />
                </button>
                <button
                  onClick={() => setPlaybackSession(null)}
                  className="p-2 bg-[#21262d] hover:bg-[#30363d] text-slate-200 rounded border border-[#30363d]"
                  title="Stop"
                >
                  <Square className="w-4 h-4" />
                </button>
                <button
                  onClick={() => setPlaybackProgressPercent(Math.min(100, playbackProgressPercent + 5))}
                  className="p-2 bg-[#21262d] hover:bg-[#30363d] text-slate-200 rounded border border-[#30363d]"
                  title="Fast Forward 10s"
                >
                  <FastForward className="w-4 h-4" />
                </button>
              </div>

              {/* Speed Buttons */}
              <div className="flex items-center space-x-1">
                <span className="text-xs text-slate-400 mr-2">SPEED:</span>
                {[0.5, 1.0, 2.0, 4.0, 8.0, 16.0].map((spd) => (
                  <button
                    key={spd}
                    onClick={() => setPlaybackSpeed(spd)}
                    className={`px-2 py-1 rounded text-[11px] font-bold border transition-colors ${
                      playbackSpeed === spd
                        ? 'bg-purple-600 text-white border-purple-500'
                        : 'bg-[#0d1117] text-slate-400 border-[#30363d]'
                    }`}
                  >
                    {spd}x
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Search Results List */}
          <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
            <div className="p-3 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between text-xs">
              <span className="font-bold text-slate-200 uppercase">SEARCH RESULTS ({segments.length} CLIPS)</span>
            </div>

            <div className="max-h-64 overflow-y-auto">
              <table className="w-full text-left text-xs">
                <thead className="bg-[#0d1117]/60 text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
                  <tr>
                    <th className="py-2 px-4">Channel</th>
                    <th className="py-2 px-4">Start Time</th>
                    <th className="py-2 px-4">End Time</th>
                    <th className="py-2 px-4">Duration</th>
                    <th className="py-2 px-4">Event Type</th>
                    <th className="py-2 px-4">Size</th>
                    <th className="py-2 px-4 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#21262d]">
                  {segments.map((seg) => (
                    <tr key={seg.id} className="hover:bg-[#21262d]/50 transition-colors">
                      <td className="py-2 px-4 font-bold text-slate-200">CH {seg.channel_id}</td>
                      <td className="py-2 px-4 text-slate-300">{new Date(seg.start_time).toLocaleString()}</td>
                      <td className="py-2 px-4 text-slate-300">{new Date(seg.end_time).toLocaleString()}</td>
                      <td className="py-2 px-4 text-slate-400">{seg.duration_sec}s</td>
                      <td className="py-2 px-4">
                        <span
                          className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                            seg.event_type === 'MOTION'
                              ? 'bg-amber-500/20 text-amber-400'
                              : seg.event_type === 'ALARM'
                              ? 'bg-rose-500/20 text-rose-400'
                              : 'bg-emerald-500/20 text-emerald-400'
                          }`}
                        >
                          {seg.event_type}
                        </span>
                      </td>
                      <td className="py-2 px-4 text-slate-400">{(seg.size_bytes / (1024 * 1024)).toFixed(1)} MB</td>
                      <td className="py-2 px-4 text-right space-x-2">
                        <button
                          onClick={() => handleStartPlayback(seg)}
                          className="px-2 py-1 bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-400 rounded text-[11px] font-bold"
                        >
                          Play
                        </button>
                        <button
                          onClick={() => handleExportRecording(seg.id)}
                          className="px-2 py-1 bg-[#21262d] hover:bg-[#30363d] text-slate-300 rounded text-[11px]"
                        >
                          Export
                        </button>
                      </td>
                    </tr>
                  ))}
                  {segments.length === 0 && (
                    <tr>
                      <td colSpan={7} className="py-6 text-center text-slate-500">
                        No recording segments found for this filter criteria.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 7: MULTI-CHANNEL PLAYBACK GRID */}
      {/* ========================================================================= */}
      {activeTab === 'multi_playback' && (
        <div className="space-y-4 font-mono">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
              <Layers className="w-4 h-4 text-emerald-400" />
              <span>Synchronized Multi-Channel Playback Grid</span>
            </h3>
            <span className="text-xs text-slate-400">Synchronized Timecode: {new Date().toLocaleTimeString()}</span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {channels.slice(0, 4).map((ch) => (
              <div key={ch.channel_id} className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
                <div className="p-3 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between text-xs">
                  <div className="flex items-center space-x-2">
                    <span className="font-bold text-slate-200">CH {ch.channel_number}: {ch.name}</span>
                    <span className="px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-400 text-[10px] font-bold">
                      {ch.recording_status}
                    </span>
                  </div>
                  <span className="text-[11px] text-slate-400">{ch.resolution || '1080P'}</span>
                </div>

                {/* Simulated Feed / Video Canvas */}
                <div className="h-44 bg-slate-950 flex flex-col items-center justify-center p-4 relative group">
                  <Video className="w-10 h-10 text-slate-700 group-hover:text-emerald-400 transition-colors mb-2" />
                  <span className="text-xs text-slate-400">Authenticated Playback Stream</span>
                  <span className="text-[10px] text-slate-600 mt-1">{ch.rtsp_stream_url || `rtsp://${currentDev.ip}:554/Streaming/Channels/${ch.channel_id}01`}</span>

                  <div className="absolute bottom-2 left-2 right-2 flex items-center justify-between text-[10px] text-slate-400 bg-slate-900/80 px-2 py-1 rounded backdrop-blur">
                    <span>STATUS: SYNCED</span>
                    <span>1.0x PLAYING</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 8: HDD & STORAGE HEALTH + HIGH-IMPACT FORMATTING */}
      {/* ========================================================================= */}
      {activeTab === 'storage' && (
        <div className="space-y-6 font-mono">
          {/* Storage Summary */}
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="flex items-center justify-between border-b border-[#30363d] pb-3">
              <div>
                <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
                  <HardDrive className="w-4 h-4 text-blue-400" />
                  <span>Internal Hard Drive Health & Array Status</span>
                </h3>
                <p className="text-[11px] text-slate-400 mt-1">
                  SMART diagnostics, bad sectors, operating temperature and storage pool capacity.
                </p>
              </div>
              <span
                className={`px-2.5 py-1 rounded text-xs font-bold border ${
                  storageHealth?.overall_health === 'GOOD'
                    ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30'
                    : 'bg-amber-500/20 text-amber-400 border-amber-500/30'
                }`}
              >
                OVERALL: {storageHealth?.overall_health || 'GOOD'}
              </span>
            </div>

            {/* Disks Table */}
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="bg-[#0d1117] text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
                  <tr>
                    <th className="py-2.5 px-4">Disk #</th>
                    <th className="py-2.5 px-4">Model & Serial</th>
                    <th className="py-2.5 px-4">Capacity</th>
                    <th className="py-2.5 px-4">Used / Free</th>
                    <th className="py-2.5 px-4">SMART Health</th>
                    <th className="py-2.5 px-4">Temp</th>
                    <th className="py-2.5 px-4">Bad Sectors</th>
                    <th className="py-2.5 px-4 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#21262d]">
                  {storageHealth?.disks && storageHealth.disks.length > 0 ? (
                    storageHealth.disks.map((d) => (
                      <tr key={d.disk_index} className="hover:bg-[#21262d]/50 transition-colors">
                        <td className="py-3 px-4 font-bold text-slate-200">HDD {d.disk_index + 1}</td>
                        <td className="py-3 px-4">
                          <p className="font-bold text-slate-200">{d.model || 'Seagate SkyHawk Surveillance'}</p>
                          <p className="text-[10px] text-slate-400">S/N: {d.serial || 'WAK192837'}</p>
                        </td>
                        <td className="py-3 px-4 font-bold text-slate-200">{d.capacity_gb} GB</td>
                        <td className="py-3 px-4">
                          <div className="w-28 space-y-1">
                            <div className="flex justify-between text-[10px] text-slate-400">
                              <span>{d.usage_percent || 65}%</span>
                              <span>{d.free_gb} GB free</span>
                            </div>
                            <div className="h-1.5 bg-[#0d1117] rounded-full overflow-hidden">
                              <div
                                className="h-full bg-emerald-500 rounded-full"
                                style={{ width: `${d.usage_percent || 65}%` }}
                              />
                            </div>
                          </div>
                        </td>
                        <td className="py-3 px-4">
                          <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400">
                            {d.smart_status || 'HEALTHY'}
                          </span>
                        </td>
                        <td className="py-3 px-4 text-slate-300">{d.temperature_c || 38}°C</td>
                        <td className="py-3 px-4 text-slate-300">{d.bad_sectors || 0}</td>
                        <td className="py-3 px-4 text-right">
                          <button
                            onClick={() => handleStageFormatDisk(d.disk_index)}
                            className="px-2.5 py-1 bg-red-600/20 hover:bg-red-600/40 text-red-400 rounded text-xs font-bold border border-red-500/30 transition-colors"
                          >
                            Format Disk
                          </button>
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={8} className="py-6 text-center text-slate-500">
                        No disks reported by recorder storage API.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>

          {/* High-Impact Format Disk Modal */}
          {formatModalOpen && formatPreview && (
            <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 z-50">
              <div className="max-w-md w-full bg-[#161b22] border border-red-600 rounded-xl p-6 space-y-4 font-mono shadow-2xl">
                <div className="flex items-center space-x-3 text-red-400">
                  <AlertTriangle className="w-6 h-6 flex-shrink-0" />
                  <h3 className="text-sm font-bold uppercase">CRITICAL: Low-Level Disk Format Confirmation</h3>
                </div>

                <div className="p-3 bg-red-950/60 border border-red-800 text-red-300 rounded-lg text-xs space-y-2">
                  <p className="font-bold">TARGET: HDD {formatPreview.disk_index + 1} ({formatPreview.model})</p>
                  <p className="text-[11px]">{formatPreview.warning}</p>
                  <p className="text-[10px] text-red-400/90 font-mono">
                    STAGED TOKEN: {formatPreview.format_token.slice(0, 24)}... (Expires in 10 mins)
                  </p>
                </div>

                <div>
                  <label className="text-xs text-slate-300 block mb-1">
                    To proceed, type <span className="font-bold text-red-400">CONFIRM FORMAT</span> below:
                  </label>
                  <input
                    type="text"
                    value={formatConfirmationInput}
                    onChange={(e) => setFormatConfirmationInput(e.target.value)}
                    placeholder="CONFIRM FORMAT"
                    className="w-full px-3 py-2 bg-[#0d1117] border border-red-600/50 rounded-lg text-xs text-slate-200 focus:outline-none"
                  />
                </div>

                <div className="flex items-center justify-end space-x-3 pt-3 border-t border-[#30363d]">
                  <button
                    onClick={() => {
                      setFormatModalOpen(false);
                      setFormatPreview(null);
                    }}
                    className="px-4 py-2 bg-[#21262d] hover:bg-[#30363d] text-slate-300 rounded-lg text-xs"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleApproveFormatDisk}
                    className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white font-bold rounded-lg text-xs transition-colors"
                  >
                    Approve Format
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 9: STORAGE CALCULATOR & DIAGNOSTICS */}
      {/* ========================================================================= */}
      {activeTab === 'calculator' && (
        <div className="space-y-6 font-mono">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
              <Sliders className="w-4 h-4 text-emerald-400" />
              <span>CCTV Storage Retention Estimator</span>
            </h3>

            <p className="text-xs text-slate-400">
              Calculate HDD retention days based on video bitrates, camera channels, and daily recording schedules:
            </p>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 text-xs pt-2">
              <div>
                <label className="text-slate-400 block mb-1">Active Camera Channels:</label>
                <input
                  type="number"
                  min="1"
                  max="64"
                  value={calcChannels}
                  onChange={(e) => setCalcChannels(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                />
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Average Bitrate (kbps):</label>
                <select
                  value={calcBitrateKbps}
                  onChange={(e) => setCalcBitrateKbps(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                >
                  <option value="2048">2048 kbps (1080P H.265)</option>
                  <option value="4096">4096 kbps (4MP / 5MP H.265)</option>
                  <option value="8192">8192 kbps (4K 8MP Ultra HD)</option>
                  <option value="1024">1024 kbps (Substream / 720P)</option>
                </select>
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Hours Recorded / Day:</label>
                <input
                  type="number"
                  min="1"
                  max="24"
                  value={calcHoursPerDay}
                  onChange={(e) => setCalcHoursPerDay(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                />
              </div>

              <div>
                <label className="text-slate-400 block mb-1">Total HDD Capacity (GB):</label>
                <input
                  type="number"
                  step="500"
                  value={calcCapacityGB}
                  onChange={(e) => setCalcCapacityGB(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-slate-200 focus:outline-none"
                />
              </div>
            </div>

            {/* Results Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-4 border-t border-[#30363d]">
              <div className="p-4 bg-[#0d1117] rounded-lg border border-[#30363d]">
                <span className="text-[10px] text-slate-400 uppercase">Daily Storage Generated</span>
                <p className="text-xl font-bold text-emerald-400 mt-1">{calcDailyStorageGB.toFixed(1)} GB/day</p>
                <p className="text-[10px] text-slate-500 mt-0.5">Across {calcChannels} streams</p>
              </div>

              <div className="p-4 bg-[#0d1117] rounded-lg border border-[#30363d]">
                <span className="text-[10px] text-slate-400 uppercase">Estimated Retention Days</span>
                <p className="text-xl font-bold text-purple-400 mt-1">{calcRetentionDays.toFixed(1)} Days</p>
                <p className="text-[10px] text-slate-500 mt-0.5">With {calcCapacityGB} GB disk array</p>
              </div>

              <div className="p-4 bg-[#0d1117] rounded-lg border border-[#30363d]">
                <span className="text-[10px] text-slate-400 uppercase">30-Day Storage Target</span>
                <p className="text-xl font-bold text-blue-400 mt-1">{((calcDailyStorageGB * 30) / 1000).toFixed(2)} TB</p>
                <p className="text-[10px] text-slate-500 mt-0.5">Recommended HDD sizing</p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ========================================================================= */}
      {/* TAB 10: CHANNEL MAPPINGS BACKUP & RESTORE */}
      {/* ========================================================================= */}
      {activeTab === 'backup' && (
        <div className="space-y-6 font-mono">
          <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <h3 className="text-xs font-bold text-slate-200 uppercase flex items-center space-x-2">
              <Database className="w-4 h-4 text-emerald-400" />
              <span>Recorder Channel Mapping Backup & Migration</span>
            </h3>

            <p className="text-xs text-slate-400">
              Export and import complete recorder channel topologies, including BNC analog modes, IP camera credentials, and resolution profiles.
            </p>

            <div className="flex flex-wrap items-center gap-4 pt-2">
              <button
                onClick={handleExportMappings}
                className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-lg text-xs flex items-center space-x-1.5 transition-colors"
              >
                <Download className="w-4 h-4" />
                <span>Export Mappings (JSON)</span>
              </button>

              <label className="px-4 py-2 bg-[#21262d] hover:bg-[#30363d] text-slate-200 font-bold rounded-lg text-xs flex items-center space-x-1.5 cursor-pointer border border-[#30363d] transition-colors">
                <Upload className="w-4 h-4" />
                <span>Import Backup</span>
                <input type="file" accept=".json" onChange={handleImportMappings} className="hidden" />
              </label>
            </div>

            {backupJson && (
              <div className="mt-4">
                <span className="text-xs text-slate-400 block mb-1">Imported Configuration Payload:</span>
                <textarea
                  readOnly
                  value={backupJson}
                  rows={8}
                  className="w-full p-3 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-emerald-400 font-mono focus:outline-none"
                />
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
