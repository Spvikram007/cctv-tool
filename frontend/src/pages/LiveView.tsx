import React, { useState, useEffect } from 'react';
import { Device } from '../types';
import { api } from '../services/api';
import { Grid, Camera, RefreshCw, Volume2, ShieldAlert } from 'lucide-react';

interface LiveViewProps {
  devices: Device[];
  selectedDevice: Device | null;
}

export const LiveView: React.FC<LiveViewProps> = ({ devices, selectedDevice }) => {
  const [layout, setLayout] = useState<'1x1' | '2x2' | '3x3'>('2x2');
  const [activeID, setActiveID] = useState<string>(selectedDevice?.id || devices[0]?.id || '');
  const [refreshKey, setRefreshKey] = useState<number>(Date.now());
  const [autoRefresh, setAutoRefresh] = useState(true);

  useEffect(() => {
    if (selectedDevice) {
      setActiveID(selectedDevice.id);
    } else if (devices.length > 0 && !activeID) {
      setActiveID(devices[0].id);
    }
  }, [selectedDevice, devices]);

  useEffect(() => {
    let timer: any = null;
    if (autoRefresh) {
      timer = setInterval(() => {
        setRefreshKey(Date.now());
      }, 2000);
    }
    return () => clearInterval(timer);
  }, [autoRefresh]);

  const displayedDevices = layout === '1x1'
    ? devices.filter((d) => d.id === activeID)
    : layout === '2x2'
    ? devices.slice(0, 4)
    : devices.slice(0, 9);

  return (
    <div className="space-y-6">
      {/* Viewport Control Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Grid className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">LIVE MATRIX & STREAM MONITOR</h2>
            <p className="text-[11px] text-slate-400 font-mono">RTSP Low-latency Substream & Snapshot Ingestion</p>
          </div>
        </div>

        {/* Layout Selectors & Controls */}
        <div className="flex items-center space-x-3">
          <div className="flex items-center bg-[#0d1117] border border-[#30363d] rounded-lg p-1 text-xs font-mono">
            <button
              onClick={() => setLayout('1x1')}
              className={`px-3 py-1 rounded ${layout === '1x1' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400'}`}
            >
              1x1
            </button>
            <button
              onClick={() => setLayout('2x2')}
              className={`px-3 py-1 rounded ${layout === '2x2' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400'}`}
            >
              2x2
            </button>
            <button
              onClick={() => setLayout('3x3')}
              className={`px-3 py-1 rounded ${layout === '3x3' ? 'bg-emerald-600 text-white font-bold' : 'text-slate-400'}`}
            >
              3x3
            </button>
          </div>

          <button
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={`px-3 py-1.5 rounded-lg border text-xs font-mono flex items-center space-x-1.5 ${
              autoRefresh ? 'bg-emerald-950 text-emerald-400 border-emerald-700' : 'bg-slate-800 text-slate-400 border-slate-700'
            }`}
          >
            <RefreshCw className={`w-3.5 h-3.5 ${autoRefresh ? 'animate-spin' : ''}`} />
            <span>{autoRefresh ? 'LIVE POLLING (2s)' : 'PAUSED'}</span>
          </button>
        </div>
      </div>

      {/* Camera Grid Viewport */}
      {devices.length > 0 ? (
        <div
          className={`grid gap-4 ${
            layout === '1x1'
              ? 'grid-cols-1'
              : layout === '2x2'
              ? 'grid-cols-1 md:grid-cols-2'
              : 'grid-cols-1 sm:grid-cols-2 md:grid-cols-3'
          }`}
        >
          {displayedDevices.map((dev) => (
            <div
              key={dev.id}
              className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden flex flex-col group relative"
            >
              {/* Camera Header Banner */}
              <div className="h-8 bg-[#0d1117] px-3 flex items-center justify-between border-b border-[#30363d] text-[11px] font-mono">
                <span className="font-bold text-slate-200">{dev.name}</span>
                <span className="text-emerald-400 flex items-center space-x-1">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                  <span>1080P 25FPS</span>
                </span>
              </div>

              {/* Viewport Canvas Frame */}
              <div className="relative aspect-video bg-black flex items-center justify-center overflow-hidden">
                <img
                  src={`${api.getSnapshotURL(dev.id)}&rand=${refreshKey}`}
                  alt={dev.name}
                  className="w-full h-full object-cover"
                />

                {/* Overlay controls on hover */}
                <div className="absolute bottom-2 left-2 right-2 flex items-center justify-between opacity-0 group-hover:opacity-100 transition-opacity bg-black/60 backdrop-blur-sm p-2 rounded-lg text-xs font-mono">
                  <span className="text-slate-300">{dev.ip}:554</span>
                  <div className="flex items-center space-x-2">
                    <button
                      onClick={() => window.open(api.getSnapshotURL(dev.id), '_blank')}
                      className="p-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200"
                      title="Download Snapshot"
                    >
                      <Camera className="w-3.5 h-3.5" />
                    </button>
                    <button className="p-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-200" title="Audio">
                      <Volume2 className="w-3.5 h-3.5 text-emerald-400" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="p-12 text-center text-slate-500 font-mono text-xs border border-dashed border-[#30363d] rounded-xl space-y-2">
          <ShieldAlert className="w-8 h-8 text-slate-600 mx-auto" />
          <p>No devices currently enrolled. Please run Auto Discovery or seed devices.</p>
        </div>
      )}
    </div>
  );
};
