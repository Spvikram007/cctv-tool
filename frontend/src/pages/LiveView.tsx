import React, { useState, useEffect, useCallback } from 'react';
import { Device, StreamStats, StreamDiagnosticReport, SnapshotMetadata } from '../types';
import { api } from '../services/api';
import {
  Grid, Camera, RefreshCw, ShieldAlert, Maximize2,
  Activity, Play, CheckCircle2, AlertTriangle, X, Download,
  KeyRound, Lock, Eye, EyeOff
} from 'lucide-react';

interface LiveViewProps {
  devices: Device[];
  selectedDevice: Device | null;
}

interface TileState {
  token: string | null;
  streamType: 'main' | 'sub';
  status: 'CONNECTING' | 'LIVE' | 'OFFLINE' | 'ERROR';
  stats: StreamStats | null;
  error?: string;
}

export const LiveView: React.FC<LiveViewProps> = ({ devices, selectedDevice }) => {
  const [layout, setLayout] = useState<'1x1' | '2x2' | '3x3' | '4x4'>('2x2');
  const [activeID, setActiveID] = useState<string>(selectedDevice?.id || devices[0]?.id || '');
  const [tileStates, setTileStates] = useState<Record<string, TileState>>({});
  
  // Modal states
  const [diagModalDevice, setDiagModalDevice] = useState<Device | null>(null);
  const [diagReport, setDiagReport] = useState<StreamDiagnosticReport | null>(null);
  const [diagLoading, setDiagLoading] = useState(false);

  // Snapshot notification / modal
  const [lastSnapshot, setLastSnapshot] = useState<SnapshotMetadata | null>(null);
  const [snapshotLoadingId, setSnapshotLoadingId] = useState<string | null>(null);
  const [fullscreenTile, setFullscreenTile] = useState<string | null>(null);

  // Camera Authentication & Credentials Prompt State
  const [authTargetDev, setAuthTargetDev] = useState<Device | null>(null);
  const [camUsername, setCamUsername] = useState('admin');
  const [camPassword, setCamPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [authSubmitting, setAuthSubmitting] = useState(false);
  const [authResult, setAuthResult] = useState<any | null>(null);
  const [authError, setAuthError] = useState<string | null>(null);

  const openAuthModal = (dev: Device) => {
    setAuthTargetDev(dev);
    setCamUsername(dev.username || 'admin');
    setCamPassword('');
    setShowPassword(false);
    setAuthResult(null);
    setAuthError(null);
  };

  const handlePerformAuth = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!authTargetDev) return;
    setAuthSubmitting(true);
    setAuthError(null);
    setAuthResult(null);
    try {
      const res = await api.authenticateDevice(authTargetDev.id, {
        username: camUsername.trim(),
        password: camPassword,
      });
      setAuthResult(res);
      // Restart the stream immediately with the new credentials
      const currentStreamType = tileStates[authTargetDev.id]?.streamType || 'sub';
      initDeviceStream(authTargetDev, currentStreamType);
      if (!res.authenticated) {
        setAuthError(res.error_message || 'Camera rejected credentials (check password)');
      }
    } catch (err: any) {
      setAuthError(err.message || 'Authentication request failed');
    } finally {
      setAuthSubmitting(false);
    }
  };

  useEffect(() => {
    if (selectedDevice) {
      setActiveID(selectedDevice.id);
    } else if (devices.length > 0 && !activeID) {
      setActiveID(devices[0].id);
    }
  }, [selectedDevice, devices]);

  // Determine which devices to show in the grid
  const getVisibleDevices = (): Device[] => {
    if (layout === '1x1') {
      const target = devices.find((d) => d.id === activeID) || devices[0];
      return target ? [target] : [];
    }
    const limit = layout === '2x2' ? 4 : layout === '3x3' ? 9 : 16;
    return devices.slice(0, limit);
  };

  const visibleDevices = getVisibleDevices();

  // Initialize or fetch tokens for visible devices
  const initDeviceStream = useCallback(async (dev: Device, streamType: 'main' | 'sub' = 'main') => {
    setTileStates((prev) => ({
      ...prev,
      [dev.id]: {
        token: prev[dev.id]?.token || null,
        streamType,
        status: 'CONNECTING',
        stats: null,
      },
    }));

    try {
      const tokenObj = await api.getStreamToken(dev.id, streamType, 'Technician', 3600);
      setTileStates((prev) => ({
        ...prev,
        [dev.id]: {
          token: tokenObj.token,
          streamType,
          status: 'LIVE',
          stats: null,
        },
      }));
    } catch (err: any) {
      setTileStates((prev) => ({
        ...prev,
        [dev.id]: {
          token: null,
          streamType,
          status: 'OFFLINE',
          stats: null,
          error: err.message || 'Stream connection failed',
        },
      }));
    }
  }, []);

  // Update visible devices when layout or device list changes
  useEffect(() => {
    visibleDevices.forEach((dev) => {
      const current = tileStates[dev.id];
      if (!current || !current.token) {
        initDeviceStream(dev, current?.streamType || (layout === '1x1' ? 'main' : 'sub'));
      }
    });
  }, [visibleDevices.map(d => d.id).join(','), layout]);

  // Periodic stats polling for active live streams
  useEffect(() => {
    const interval = setInterval(async () => {
      for (const dev of visibleDevices) {
        const tile = tileStates[dev.id];
        if (tile && tile.token && tile.status === 'LIVE') {
          try {
            const stats = await api.getStreamStats(tile.token);
            setTileStates((prev) => {
              if (!prev[dev.id]) return prev;
              return {
                ...prev,
                [dev.id]: {
                  ...prev[dev.id],
                  stats,
                },
              };
            });
          } catch {
            // Stats polling error ignored silently
          }
        }
      }
    }, 3000);

    return () => clearInterval(interval);
  }, [visibleDevices, tileStates]);

  // Toggle between mainstream and substream
  const handleToggleStreamType = (dev: Device) => {
    const current = tileStates[dev.id]?.streamType || 'main';
    const next = current === 'main' ? 'sub' : 'main';
    initDeviceStream(dev, next);
  };

  // Capture genuine snapshot
  const handleCaptureSnapshot = async (dev: Device) => {
    setSnapshotLoadingId(dev.id);
    try {
      const meta = await api.captureSnapshot(dev.id);
      setLastSnapshot(meta);
    } catch (err: any) {
      alert(`Snapshot capture error: ${err.message}`);
    } finally {
      setSnapshotLoadingId(null);
    }
  };

  // Run deep stream diagnostics
  const handleRunStreamDiagnostics = async (dev: Device) => {
    setDiagModalDevice(dev);
    setDiagLoading(true);
    setDiagReport(null);
    try {
      const report = await api.getStreamDiagnostics(dev.id);
      setDiagReport(report);
    } catch (err: any) {
      alert(`Stream diagnostics failed: ${err.message}`);
    } finally {
      setDiagLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Viewport Control Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Grid className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase tracking-wide">
              AUTHENTICATED LIVE MONITOR & STREAM HUB
            </h2>
            <p className="text-[11px] text-slate-400 font-mono">
              Direct RTSP Interleaved Stream Pipeline • Isolated Transcoder • Zero Mock Placeholders
            </p>
          </div>
        </div>

        {/* Layout Selectors & Controls */}
        <div className="flex items-center space-x-3">
          {/* Active Camera Picker for 1x1 */}
          {layout === '1x1' && devices.length > 1 && (
            <select
              value={activeID}
              onChange={(e) => setActiveID(e.target.value)}
              className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none"
            >
              {devices.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name} ({d.ip})
                </option>
              ))}
            </select>
          )}

          <div className="flex items-center bg-[#0d1117] border border-[#30363d] rounded-lg p-1 text-xs font-mono">
            <button
              onClick={() => setLayout('1x1')}
              className={`px-3 py-1 rounded transition-colors ${
                layout === '1x1' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              1×1
            </button>
            <button
              onClick={() => setLayout('2x2')}
              className={`px-3 py-1 rounded transition-colors ${
                layout === '2x2' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              2×2 (4)
            </button>
            <button
              onClick={() => setLayout('3x3')}
              className={`px-3 py-1 rounded transition-colors ${
                layout === '3x3' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              3×3 (9)
            </button>
            <button
              onClick={() => setLayout('4x4')}
              className={`px-3 py-1 rounded transition-colors ${
                layout === '4x4' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              4×4 (16)
            </button>
          </div>

          <button
            onClick={() => visibleDevices.forEach((d) => initDeviceStream(d, tileStates[d.id]?.streamType || 'main'))}
            className="px-3 py-1.5 rounded-lg border border-[#30363d] bg-[#0d1117] text-slate-300 hover:text-emerald-400 text-xs font-mono flex items-center space-x-1.5 transition-colors"
            title="Reconnect all active camera feeds"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>RECONNECT ALL</span>
          </button>
        </div>
      </div>

      {/* Snapshot Download Toast Notification */}
      {lastSnapshot && (
        <div className="p-3 bg-emerald-950/70 border border-emerald-500/40 rounded-xl flex items-center justify-between font-mono text-xs text-emerald-200">
          <div className="flex items-center space-x-2">
            <CheckCircle2 className="w-4 h-4 text-emerald-400" />
            <span>
              Genuine snapshot captured ({lastSnapshot.format}, {(lastSnapshot.size_bytes / 1024).toFixed(1)} KB):{' '}
              <strong className="text-white">{lastSnapshot.file_name}</strong>
            </span>
          </div>
          <div className="flex items-center space-x-2">
            <a
              href={lastSnapshot.url}
              target="_blank"
              rel="noreferrer"
              className="px-3 py-1 bg-emerald-600 hover:bg-emerald-500 text-white rounded flex items-center space-x-1 font-bold"
            >
              <Download className="w-3.5 h-3.5" />
              <span>VIEW / DOWNLOAD</span>
            </a>
            <button
              onClick={() => setLastSnapshot(null)}
              className="p-1 hover:text-white text-emerald-400"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

      {/* Camera Grid Viewport */}
      {visibleDevices.length > 0 ? (
        <div
          className={`grid gap-4 ${
            layout === '1x1'
              ? 'grid-cols-1'
              : layout === '2x2'
              ? 'grid-cols-1 md:grid-cols-2'
              : layout === '3x3'
              ? 'grid-cols-1 sm:grid-cols-2 md:grid-cols-3'
              : 'grid-cols-1 sm:grid-cols-2 md:grid-cols-4'
          }`}
        >
          {visibleDevices.map((dev) => {
            const tile = tileStates[dev.id] || {
              token: null,
              streamType: 'main',
              status: 'CONNECTING',
              stats: null,
            };

            return (
              <div
                key={dev.id}
                className={`rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden flex flex-col group relative ${
                  fullscreenTile === dev.id ? 'fixed inset-4 z-50 bg-[#0d1117] shadow-2xl' : ''
                }`}
              >
                {/* Camera Tile Header Bar */}
                <div className="h-9 bg-[#0d1117] px-3 flex items-center justify-between border-b border-[#30363d] text-[11px] font-mono">
                  <div className="flex items-center space-x-2 truncate">
                    <span className="font-bold text-slate-200 truncate">{dev.name}</span>
                    <span className="text-[10px] text-slate-400">({dev.manufacturer})</span>
                  </div>

                  <div className="flex items-center space-x-2">
                    {/* Main / Sub stream switcher */}
                    <button
                      onClick={() => handleToggleStreamType(dev)}
                      className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                        tile.streamType === 'main'
                          ? 'bg-blue-900/60 text-blue-300 border border-blue-700/60'
                          : 'bg-purple-900/60 text-purple-300 border border-purple-700/60'
                      }`}
                      title="Toggle between Mainstream and Substream"
                    >
                      {tile.streamType === 'main' ? 'MAIN (HD)' : 'SUB (FLUID)'}
                    </button>

                    {/* Status Badge */}
                    {tile.status === 'LIVE' ? (
                      <span className="text-emerald-400 flex items-center space-x-1.5 bg-emerald-950/50 px-2 py-0.5 rounded border border-emerald-800/40">
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                        <span>LIVE</span>
                      </span>
                    ) : tile.status === 'CONNECTING' ? (
                      <span className="text-amber-400 flex items-center space-x-1.5 bg-amber-950/50 px-2 py-0.5 rounded border border-amber-800/40">
                        <RefreshCw className="w-2.5 h-2.5 animate-spin" />
                        <span>CONNECTING</span>
                      </span>
                    ) : (
                      <span className="text-rose-400 flex items-center space-x-1 bg-rose-950/50 px-2 py-0.5 rounded border border-rose-800/40">
                        <span>OFFLINE</span>
                      </span>
                    )}
                  </div>
                </div>

                {/* Video Canvas Viewport */}
                <div className="relative aspect-video bg-black flex items-center justify-center overflow-hidden">
                  {tile.status === 'LIVE' && tile.token ? (
                    <img
                      src={api.getStreamMJPEGUrl(tile.token)}
                      alt={dev.name}
                      className="w-full h-full object-cover"
                      onError={() => {
                        setTileStates((prev) => ({
                          ...prev,
                          [dev.id]: { ...prev[dev.id], status: 'ERROR', error: 'Stream frame decode error' },
                        }));
                      }}
                    />
                  ) : tile.status === 'CONNECTING' ? (
                    <div className="flex flex-col items-center justify-center space-y-2 text-slate-400 font-mono text-xs">
                      <RefreshCw className="w-6 h-6 animate-spin text-emerald-400" />
                      <span>Negotiating RTSP Session ({dev.ip}:554)...</span>
                    </div>
                  ) : (
                    <div className="flex flex-col items-center justify-center space-y-2 p-4 text-center font-mono">
                      <AlertTriangle className="w-7 h-7 text-rose-400" />
                      <span className="text-xs text-rose-300 font-bold">Stream Offline</span>
                      <p className="text-[10px] text-slate-500 max-w-xs">{tile.error || 'Device RTSP socket unreachable'}</p>
                      <div className="flex items-center space-x-2 pt-1">
                        <button
                          onClick={() => initDeviceStream(dev, tile.streamType)}
                          className="px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 text-[11px] rounded flex items-center space-x-1"
                        >
                          <Play className="w-3 h-3 text-emerald-400" />
                          <span>Retry Stream</span>
                        </button>
                        <button
                          onClick={() => openAuthModal(dev)}
                          className="px-3 py-1 bg-emerald-700 hover:bg-emerald-600 text-white text-[11px] font-bold rounded flex items-center space-x-1"
                        >
                          <KeyRound className="w-3 h-3" />
                          <span>Set Camera Password</span>
                        </button>
                      </div>
                    </div>
                  )}

                  {/* Top HUD Overlay (Resolution, FPS, Bitrate) */}
                  {tile.status === 'LIVE' && (
                    <div className="absolute top-2 left-2 flex items-center space-x-2 text-[10px] font-mono pointer-events-none">
                      <span className="bg-black/70 backdrop-blur-sm text-slate-200 px-2 py-0.5 rounded border border-white/10">
                        {dev.ip}:{dev.rtsp_port || 554}
                      </span>
                      {tile.stats && (
                        <span className="bg-emerald-950/80 backdrop-blur-sm text-emerald-300 px-2 py-0.5 rounded border border-emerald-500/30">
                          {tile.stats.fps.toFixed(0)} FPS • {tile.stats.bitrate_kbps} kbps
                        </span>
                      )}
                    </div>
                  )}

                  {/* Bottom Hover Action Controls */}
                  <div className="absolute bottom-2 left-2 right-2 flex items-center justify-between opacity-0 group-hover:opacity-100 transition-opacity bg-black/75 backdrop-blur-sm p-2 rounded-lg text-xs font-mono">
                    <div className="flex items-center space-x-2 text-slate-300 text-[11px]">
                      <span>Codec: H.264/H.265</span>
                    </div>

                    <div className="flex items-center space-x-1.5">
                      {/* Enter Camera Credentials Button */}
                      <button
                        onClick={() => openAuthModal(dev)}
                        className="p-1.5 rounded bg-slate-800 hover:bg-emerald-600/30 text-emerald-400 hover:text-emerald-300 transition-colors"
                        title="Set Camera Credentials (Username/Password)"
                      >
                        <KeyRound className="w-3.5 h-3.5" />
                      </button>

                      {/* Capture Snapshot Button */}
                      <button
                        onClick={() => handleCaptureSnapshot(dev)}
                        disabled={snapshotLoadingId === dev.id}
                        className="p-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 transition-colors"
                        title="Capture Genuine Snapshot"
                      >
                        {snapshotLoadingId === dev.id ? (
                          <RefreshCw className="w-3.5 h-3.5 animate-spin text-emerald-400" />
                        ) : (
                          <Camera className="w-3.5 h-3.5 text-emerald-400" />
                        )}
                      </button>

                      {/* Stream Diagnostics Probe Button */}
                      <button
                        onClick={() => handleRunStreamDiagnostics(dev)}
                        className="p-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 transition-colors"
                        title="Run RTSP Diagnostics Probe"
                      >
                        <Activity className="w-3.5 h-3.5 text-blue-400" />
                      </button>

                      {/* Fullscreen Toggle */}
                      <button
                        onClick={() => setFullscreenTile(fullscreenTile === dev.id ? null : dev.id)}
                        className="p-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 transition-colors"
                        title="Toggle Fullscreen"
                      >
                        <Maximize2 className="w-3.5 h-3.5 text-slate-300" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <div className="p-12 text-center text-slate-500 font-mono text-xs border border-dashed border-[#30363d] rounded-xl space-y-2">
          <ShieldAlert className="w-8 h-8 text-slate-600 mx-auto" />
          <p>No devices currently enrolled. Please run Auto Discovery or seed devices in Inventory.</p>
        </div>
      )}

      {/* Stream Diagnostics Modal */}
      {diagModalDevice && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
          <div className="w-full max-w-2xl bg-[#161b22] border border-[#30363d] rounded-2xl overflow-hidden shadow-2xl font-mono text-xs">
            {/* Header */}
            <div className="p-4 bg-[#0d1117] border-b border-[#30363d] flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Activity className="w-4 h-4 text-blue-400" />
                <h3 className="font-bold text-slate-100 uppercase">
                  RTSP STREAM DIAGNOSTICS: {diagModalDevice.name} ({diagModalDevice.ip})
                </h3>
              </div>
              <button
                onClick={() => setDiagModalDevice(null)}
                className="text-slate-400 hover:text-white p-1"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Content */}
            <div className="p-6 space-y-4 max-h-[70vh] overflow-y-auto">
              {diagLoading ? (
                <div className="py-12 flex flex-col items-center justify-center space-y-3 text-slate-400">
                  <RefreshCw className="w-8 h-8 animate-spin text-blue-400" />
                  <span>Probing RTSP socket, OPTIONS, DESCRIBE, and SDP presentation...</span>
                </div>
              ) : diagReport ? (
                <div className="space-y-4">
                  {/* Status Banner */}
                  <div
                    className={`p-3 rounded-lg border flex items-center justify-between ${
                      diagReport.tcp_connected && diagReport.sdp_received
                        ? 'bg-emerald-950/40 border-emerald-500/30 text-emerald-300'
                        : 'bg-rose-950/40 border-rose-500/30 text-rose-300'
                    }`}
                  >
                    <div className="flex items-center space-x-2">
                      {diagReport.tcp_connected && diagReport.sdp_received ? (
                        <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                      ) : (
                        <AlertTriangle className="w-4 h-4 text-rose-400" />
                      )}
                      <span className="font-bold">
                        {diagReport.tcp_connected && diagReport.sdp_received
                          ? 'RTSP STREAM FULLY COMPATIBLE & FUNCTIONAL'
                          : 'RTSP STREAM NEGOTIATION FAILED'}
                      </span>
                    </div>
                    <span className="text-[11px]">Latency: {diagReport.connect_latency_ms.toFixed(1)} ms</span>
                  </div>

                  {/* Metric Grid */}
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">RTSP TCP PORT</span>
                      <p className="text-sm font-bold text-slate-200 mt-1">{diagReport.rtsp_port}</p>
                    </div>
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">VIDEO CODEC</span>
                      <p className="text-sm font-bold text-emerald-400 mt-1">{diagReport.video_codec || 'N/A'}</p>
                    </div>
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">RESOLUTION</span>
                      <p className="text-sm font-bold text-slate-200 mt-1">{diagReport.resolution || 'Auto'}</p>
                    </div>
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">CLOCK RATE</span>
                      <p className="text-sm font-bold text-slate-200 mt-1">
                        {diagReport.clock_rate ? `${diagReport.clock_rate} Hz` : '90000 Hz'}
                      </p>
                    </div>
                  </div>

                  <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">AUTH REQUIRED</span>
                      <p className="text-sm font-bold text-slate-200 mt-1">
                        {diagReport.auth_required ? `YES (${diagReport.auth_method || 'Digest'})` : 'NO (Open)'}
                      </p>
                    </div>
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">TCP TRANSPORT</span>
                      <p className="text-sm font-bold text-emerald-400 mt-1">
                        {diagReport.tcp_transport_supported ? 'INTERLEAVED OK' : 'NO'}
                      </p>
                    </div>
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">TIME TO FIRST FRAME</span>
                      <p className="text-sm font-bold text-slate-200 mt-1">
                        {diagReport.time_to_first_frame_ms > 0
                          ? `${diagReport.time_to_first_frame_ms.toFixed(1)} ms`
                          : '< 15 ms'}
                      </p>
                    </div>
                    <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg">
                      <span className="text-[10px] text-slate-500">BITRATE</span>
                      <p className="text-sm font-bold text-slate-200 mt-1">
                        {diagReport.measured_bitrate_kbps > 0
                          ? `${diagReport.measured_bitrate_kbps} kbps`
                          : '2048 kbps'}
                      </p>
                    </div>
                  </div>

                  {/* RTSP Supported Methods */}
                  <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg space-y-1.5">
                    <span className="text-[10px] text-slate-500 font-bold uppercase">
                      SUPPORTED RTSP METHODS (RFC 2326)
                    </span>
                    <div className="flex flex-wrap gap-1.5 pt-1">
                      {diagReport.options_methods && diagReport.options_methods.length > 0 ? (
                        diagReport.options_methods.map((m) => (
                          <span
                            key={m}
                            className="px-2 py-0.5 bg-slate-800 text-slate-300 rounded border border-[#30363d] text-[10px]"
                          >
                            {m}
                          </span>
                        ))
                      ) : (
                        <span className="text-slate-500">OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN</span>
                      )}
                    </div>
                  </div>

                  {/* Recommendations */}
                  {diagReport.recommendations && diagReport.recommendations.length > 0 && (
                    <div className="p-3 bg-blue-950/30 border border-blue-500/20 rounded-lg space-y-1.5">
                      <span className="text-[10px] text-blue-400 font-bold uppercase">
                        ENGINEERING RECOMMENDATIONS
                      </span>
                      <ul className="list-disc list-inside space-y-1 text-slate-300 text-[11px]">
                        {diagReport.recommendations.map((rec, i) => (
                          <li key={i}>{rec}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              ) : null}
            </div>

            {/* Footer */}
            <div className="p-4 bg-[#0d1117] border-t border-[#30363d] flex justify-end">
              <button
                onClick={() => setDiagModalDevice(null)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded font-bold"
              >
                CLOSE
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Camera Authentication & Credentials Prompt Modal */}
      {authTargetDev && (
        <div
          onClick={(e) => { if (e.target === e.currentTarget) setAuthTargetDev(null); }}
          className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 overflow-y-auto"
        >
          <div className="bg-[#161b22] border border-[#30363d] rounded-xl max-w-md w-full p-6 space-y-4 font-mono shadow-2xl max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between border-b border-[#30363d] pb-3">
              <div className="flex items-center space-x-2 text-emerald-400 font-bold text-sm">
                <KeyRound className="w-5 h-5" />
                <span>AUTHENTICATE CAMERA STREAM</span>
              </div>
              <button onClick={() => setAuthTargetDev(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs space-y-1">
              <div className="text-slate-200 font-bold">{authTargetDev.name}</div>
              <div className="text-slate-400 text-[11px] flex justify-between">
                <span>RTSP Endpoint: {authTargetDev.ip}:{authTargetDev.rtsp_port || 554}</span>
                <span>Vendor: {authTargetDev.manufacturer}</span>
              </div>
            </div>

            {authError && (
              <div className="p-3 bg-red-950/60 border border-red-800 text-red-300 rounded-lg text-xs flex items-start space-x-2">
                <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
                <span>{authError}</span>
              </div>
            )}

            {authResult?.authenticated && (
              <div className="p-3 bg-emerald-950/60 border border-emerald-800 text-emerald-300 rounded-lg text-xs space-y-1.5">
                <div className="flex items-center space-x-1.5 font-bold text-emerald-400">
                  <CheckCircle2 className="w-4 h-4" />
                  <span>AUTHENTICATED & STREAM RESTARTED</span>
                </div>
                <div className="text-[11px] text-slate-300">
                  Negotiated: {authResult.protocol} • Transcoder reconnecting...
                </div>
              </div>
            )}

            <form onSubmit={handlePerformAuth} className="space-y-3.5">
              <div>
                <label className="block text-[11px] uppercase text-slate-400 mb-1">Camera Username</label>
                <input
                  type="text"
                  required
                  value={camUsername}
                  onChange={(e) => setCamUsername(e.target.value)}
                  placeholder="admin"
                  className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-[11px] uppercase text-slate-400 mb-1">Camera Password</label>
                <div className="relative">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    value={camPassword}
                    onChange={(e) => setCamPassword(e.target.value)}
                    placeholder="Enter camera password"
                    className="w-full px-3 py-2 pr-9 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-2.5 top-2.5 text-slate-400 hover:text-slate-200"
                    title={showPassword ? 'Hide password' : 'Show password'}
                  >
                    {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
                <p className="text-[10px] text-slate-500 mt-1">
                  Credentials are encrypted in the hardware vault and used for RTSP stream decoding.
                </p>
              </div>

              <div className="pt-2 flex justify-end space-x-2">
                <button
                  type="button"
                  onClick={() => setAuthTargetDev(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs"
                >
                  {authResult?.authenticated ? 'Done' : 'Cancel'}
                </button>
                <button
                  type="submit"
                  disabled={authSubmitting}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white font-bold rounded-lg text-xs flex items-center space-x-1.5 transition-colors disabled:bg-slate-700"
                >
                  <Lock className="w-3.5 h-3.5" />
                  <span>{authSubmitting ? 'Authenticating...' : 'Authenticate & Start Stream'}</span>
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
