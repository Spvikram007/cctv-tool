import React, { useState } from 'react';
import { Device } from '../types';
import { api } from '../services/api';
import { Compass, ZoomIn, ZoomOut, ArrowUp, ArrowDown, ArrowLeft, ArrowRight, StopCircle } from 'lucide-react';

interface PTZControlProps {
  devices: Device[];
  selectedDevice: Device | null;
  onSelectDevice: (dev: Device) => void;
}

export const PTZControl: React.FC<PTZControlProps> = ({
  devices,
  selectedDevice,
  onSelectDevice,
}) => {
  const currentDev = selectedDevice || devices[0];
  const [speed, setSpeed] = useState<number>(4);
  const [lastCommand, setLastCommand] = useState<string>('IDLE');

  const sendCommand = async (cmd: string, pan = 0, tilt = 0, zoom = 0, presetId?: number) => {
    if (!currentDev) return;
    try {
      setLastCommand(cmd);
      await api.sendPTZ(currentDev.id, {
        command: cmd,
        pan,
        tilt,
        zoom,
        speed: speed / 8,
        preset_id: presetId,
      });
    } catch (e: any) {
      console.error(e);
    }
  };

  if (!currentDev) {
    return (
      <div className="p-8 text-center text-slate-500 font-mono">
        No PTZ devices available.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header Bar */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-9 w-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Compass className="w-5 h-5" />
          </div>
          <div>
            <h2 className="text-sm font-bold text-slate-100 font-mono uppercase">PTZ ROBOTIC POSITION CONTROLLER</h2>
            <p className="text-[11px] text-slate-400 font-mono">Continuous Move, Speed Regulation & Preset Memory Manager</p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <label className="text-xs font-mono text-slate-400">TARGET CAMERA:</label>
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

      {/* Grid: Controller and Viewport */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* PTZ D-Pad Console */}
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 flex flex-col items-center justify-center font-mono">
          <div className="text-xs text-slate-400 flex items-center justify-between w-full">
            <span>LAST COMMAND: <strong className="text-emerald-400">{lastCommand}</strong></span>
            <span>SPEED: <strong className="text-slate-200">{speed}x</strong></span>
          </div>

          {/* Directional Pad */}
          <div className="grid grid-cols-3 gap-3 w-56 h-56 p-3 rounded-full bg-[#0d1117] border border-[#30363d] shadow-inner items-center justify-center">
            <div></div>
            <button
              onMouseDown={() => sendCommand('UP', 0, 1, 0)}
              onMouseUp={() => sendCommand('STOP')}
              className="h-12 w-12 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
            >
              <ArrowUp className="w-5 h-5" />
            </button>
            <div></div>

            <button
              onMouseDown={() => sendCommand('LEFT', -1, 0, 0)}
              onMouseUp={() => sendCommand('STOP')}
              className="h-12 w-12 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
            >
              <ArrowLeft className="w-5 h-5" />
            </button>
            <button
              onClick={() => sendCommand('STOP')}
              className="h-12 w-12 rounded-xl bg-red-950/60 hover:bg-red-800 text-red-400 hover:text-white flex items-center justify-center border border-red-700/60 shadow transition-colors"
            >
              <StopCircle className="w-5 h-5" />
            </button>
            <button
              onMouseDown={() => sendCommand('RIGHT', 1, 0, 0)}
              onMouseUp={() => sendCommand('STOP')}
              className="h-12 w-12 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
            >
              <ArrowRight className="w-5 h-5" />
            </button>

            <div></div>
            <button
              onMouseDown={() => sendCommand('DOWN', 0, -1, 0)}
              onMouseUp={() => sendCommand('STOP')}
              className="h-12 w-12 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
            >
              <ArrowDown className="w-5 h-5" />
            </button>
            <div></div>
          </div>

          {/* Zoom controls & speed slider */}
          <div className="w-full space-y-4 pt-4 border-t border-[#30363d]">
            <div className="flex items-center justify-center space-x-4">
              <button
                onMouseDown={() => sendCommand('ZOOM_IN', 0, 0, 1)}
                onMouseUp={() => sendCommand('STOP')}
                className="px-4 py-2 rounded-lg bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white text-xs font-bold flex items-center space-x-1.5 transition-colors"
              >
                <ZoomIn className="w-4 h-4" />
                <span>ZOOM TELE</span>
              </button>
              <button
                onMouseDown={() => sendCommand('ZOOM_OUT', 0, 0, -1)}
                onMouseUp={() => sendCommand('STOP')}
                className="px-4 py-2 rounded-lg bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white text-xs font-bold flex items-center space-x-1.5 transition-colors"
              >
                <ZoomOut className="w-4 h-4" />
                <span>ZOOM WIDE</span>
              </button>
            </div>

            <div>
              <div className="flex justify-between text-xs text-slate-400 mb-1">
                <span>SLEW SPEED: {speed}X</span>
                <span>MAX 8X</span>
              </div>
              <input
                type="range"
                min="1"
                max="8"
                value={speed}
                onChange={(e) => setSpeed(parseInt(e.target.value))}
                className="w-full accent-emerald-500"
              />
            </div>
          </div>

          {/* Presets Grid */}
          <div className="w-full space-y-2 pt-2">
            <span className="text-xs text-slate-400">GOTO PRESET:</span>
            <div className="grid grid-cols-4 gap-2">
              {[1, 2, 3, 4, 5, 6, 7, 8].map((num) => (
                <button
                  key={num}
                  onClick={() => sendCommand('GOTO_PRESET', 0, 0, 0, num)}
                  className="py-1.5 rounded bg-[#0d1117] hover:bg-emerald-600/30 text-slate-300 hover:text-emerald-300 border border-slate-700 text-xs font-bold"
                >
                  PRESET {num}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* Live Camera Viewport */}
        <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden flex flex-col">
          <div className="h-8 bg-[#0d1117] px-3 flex items-center justify-between border-b border-[#30363d] text-[11px] font-mono">
            <span className="font-bold text-slate-200">{currentDev.name}</span>
            <span className="text-emerald-400 font-bold">PTZ LIVE VIEW</span>
          </div>
          <div className="relative aspect-video bg-black flex items-center justify-center">
            <img
              src={api.getSnapshotURL(currentDev.id)}
              alt={currentDev.name}
              className="w-full h-full object-cover"
            />
          </div>
          <div className="p-4 font-mono text-xs text-slate-400 space-y-1">
            <div>TARGET IP: <span className="text-slate-200">{currentDev.ip}</span></div>
            <div>ONVIF PTZ SERVICE: <span className="text-emerald-400">ACTIVE</span></div>
            <div>CONTINUOUS VELOCITY VECTOR: <span className="text-slate-300">SUPPORTED</span></div>
          </div>
        </div>
      </div>
    </div>
  );
};
