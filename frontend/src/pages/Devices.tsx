import React, { useState } from 'react';
import { Device } from '../types';
import { api } from '../services/api';
import { Video, Sliders, Play, Wrench, Compass, Trash2, Filter } from 'lucide-react';

interface DevicesProps {
  devices: Device[];
  onSelectDevice: (dev: Device) => void;
  onNavigate: (tab: any) => void;
  onRefresh: () => void;
}

export const Devices: React.FC<DevicesProps> = ({
  devices,
  onSelectDevice,
  onNavigate,
  onRefresh,
}) => {
  const [vendorFilter, setVendorFilter] = useState('ALL');
  const [categoryFilter, setCategoryFilter] = useState('ALL');

  const filtered = devices.filter((d) => {
    if (vendorFilter !== 'ALL' && d.manufacturer !== vendorFilter) return false;
    if (categoryFilter !== 'ALL' && d.category !== categoryFilter) return false;
    return true;
  });

  const handleDelete = async (id: string, name: string) => {
    if (confirm(`Remove ${name} from management inventory?`)) {
      try {
        await api.deleteDevice(id);
        onRefresh();
      } catch (err: any) {
        alert(err.message);
      }
    }
  };

  return (
    <div className="space-y-6">
      {/* Header and filters */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
            <Video className="w-5 h-5 text-emerald-400" />
            <span>ENROLLED CCTV FLEET INVENTORY ({devices.length})</span>
          </h2>
          <p className="text-xs text-slate-400 font-mono mt-1">
            Centrally manage IP cameras, analog XVRs, PTZs, and hybrid recorders across vendors.
          </p>
        </div>

        {/* Filters */}
        <div className="flex items-center gap-3">
          <div className="flex items-center space-x-2 bg-[#0d1117] border border-[#30363d] px-3 py-1.5 rounded-lg text-xs font-mono text-slate-300">
            <Filter className="w-3.5 h-3.5 text-slate-400" />
            <select
              value={vendorFilter}
              onChange={(e) => setVendorFilter(e.target.value)}
              className="bg-transparent text-slate-200 focus:outline-none"
            >
              <option value="ALL">All Manufacturers</option>
              <option value="Hikvision">Hikvision</option>
              <option value="Dahua">Dahua</option>
              <option value="CP PLUS">CP PLUS</option>
              <option value="Uniview">Uniview</option>
              <option value="Axis">Axis</option>
              <option value="Generic ONVIF">Generic ONVIF</option>
            </select>
          </div>

          <div className="flex items-center space-x-2 bg-[#0d1117] border border-[#30363d] px-3 py-1.5 rounded-lg text-xs font-mono text-slate-300">
            <select
              value={categoryFilter}
              onChange={(e) => setCategoryFilter(e.target.value)}
              className="bg-transparent text-slate-200 focus:outline-none"
            >
              <option value="ALL">All Categories</option>
              <option value="IP_CAMERA">IP Camera</option>
              <option value="XVR">XVR / Hybrid</option>
              <option value="DVR">DVR</option>
              <option value="NVR">NVR</option>
              <option value="PTZ_CAMERA">PTZ Camera</option>
            </select>
          </div>
        </div>
      </div>

      {/* Inventory Table */}
      <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs font-mono">
            <thead className="bg-[#0d1117] text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
              <tr>
                <th className="py-3 px-4">Device Name</th>
                <th className="py-3 px-4">Manufacturer</th>
                <th className="py-3 px-4">IP Address & Port</th>
                <th className="py-3 px-4">Model & Firmware</th>
                <th className="py-3 px-4">Type</th>
                <th className="py-3 px-4">Protocols</th>
                <th className="py-3 px-4">Status</th>
                <th className="py-3 px-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#21262d]">
              {filtered.map((dev) => (
                <tr key={dev.id} className="hover:bg-[#21262d]/50 transition-colors">
                  <td className="py-3 px-4 font-bold text-slate-200">{dev.name}</td>
                  <td className="py-3 px-4">
                    <span className="px-2 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300 border border-slate-700">
                      {dev.manufacturer}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-slate-300">{dev.ip}:{dev.port}</td>
                  <td className="py-3 px-4 text-slate-400">
                    <div>{dev.model}</div>
                    <div className="text-[10px] text-slate-500">{dev.firmware || 'Generic FW'}</div>
                  </td>
                  <td className="py-3 px-4 text-slate-400">{dev.category}</td>
                  <td className="py-3 px-4 text-emerald-400 text-[10px]">{dev.protocols?.join(' • ')}</td>
                  <td className="py-3 px-4">
                    <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                      ONLINE
                    </span>
                  </td>
                  <td className="py-3 px-4 text-right">
                    <div className="flex items-center justify-end space-x-1.5">
                      <button
                        title="Configure"
                        onClick={() => {
                          onSelectDevice(dev);
                          onNavigate('config');
                        }}
                        className="p-1.5 rounded bg-[#0d1117] hover:bg-emerald-600/30 text-slate-300 hover:text-emerald-300 border border-slate-700 transition-colors"
                      >
                        <Sliders className="w-3.5 h-3.5" />
                      </button>

                      <button
                        title="Live Stream"
                        onClick={() => {
                          onSelectDevice(dev);
                          onNavigate('live');
                        }}
                        className="p-1.5 rounded bg-[#0d1117] hover:bg-blue-600/30 text-slate-300 hover:text-blue-300 border border-slate-700 transition-colors"
                      >
                        <Play className="w-3.5 h-3.5" />
                      </button>

                      <button
                        title="Diagnostics"
                        onClick={() => {
                          onSelectDevice(dev);
                          onNavigate('diagnostics');
                        }}
                        className="p-1.5 rounded bg-[#0d1117] hover:bg-amber-600/30 text-slate-300 hover:text-amber-300 border border-slate-700 transition-colors"
                      >
                        <Wrench className="w-3.5 h-3.5" />
                      </button>

                      <button
                        title="PTZ Controls"
                        onClick={() => {
                          onSelectDevice(dev);
                          onNavigate('ptz');
                        }}
                        className="p-1.5 rounded bg-[#0d1117] hover:bg-purple-600/30 text-slate-300 hover:text-purple-300 border border-slate-700 transition-colors"
                      >
                        <Compass className="w-3.5 h-3.5" />
                      </button>

                      <button
                        title="Delete"
                        onClick={() => handleDelete(dev.id, dev.name)}
                        className="p-1.5 rounded bg-[#0d1117] hover:bg-red-600/30 text-slate-300 hover:text-red-300 border border-slate-700 transition-colors"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
