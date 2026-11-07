import React from 'react';
import {
  LayoutDashboard,
  Radar,
  Video,
  Grid,
  Sliders,
  Server,
  Compass,
  Wrench,
  Calculator,
  Layers,
  Database,
  KeyRound,
  FileText,
  Cpu
} from 'lucide-react';

export type NavTab =
  | 'dashboard'
  | 'discovery'
  | 'devices'
  | 'live'
  | 'config'
  | 'recorder'
  | 'ptz'
  | 'diagnostics'
  | 'calculators'
  | 'batch'
  | 'firmware'
  | 'compatibility'
  | 'licensing'
  | 'audit';

interface SidebarProps {
  activeTab: NavTab;
  setActiveTab: (tab: NavTab) => void;
  deviceCount: number;
}

export const Sidebar: React.FC<SidebarProps> = ({ activeTab, setActiveTab, deviceCount }) => {
  const items: { id: NavTab; label: string; icon: React.FC<any>; badge?: number }[] = [
    { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
    { id: 'discovery', label: 'Auto Discovery', icon: Radar },
    { id: 'devices', label: 'Device Inventory', icon: Video, badge: deviceCount },
    { id: 'live', label: 'Live Video Grid', icon: Grid },
    { id: 'config', label: 'Universal Config', icon: Sliders },
    { id: 'recorder', label: 'Recorder & XVR', icon: Server },
    { id: 'ptz', label: 'PTZ Controller', icon: Compass },
    { id: 'diagnostics', label: 'Diagnostics & RTSP', icon: Wrench },
    { id: 'calculators', label: 'Calculators (HDD/BW)', icon: Calculator },
    { id: 'batch', label: 'Batch Configuration', icon: Layers },
    { id: 'firmware', label: 'Firmware & Updates', icon: Cpu },
    { id: 'compatibility', label: 'Hardware Matrix', icon: Database },
    { id: 'licensing', label: 'Licensing & GST', icon: KeyRound },
    { id: 'audit', label: 'Audit & Reports', icon: FileText },
  ];

  return (
    <aside className="w-64 border-r border-[#30363d] bg-[#0d1117] flex flex-col justify-between p-3 select-none">
      <div className="space-y-1">
        <div className="px-3 py-2 text-[10px] font-mono font-semibold uppercase text-slate-400 tracking-wider">
          PLATFORM MODULES
        </div>
        {items.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;
          return (
            <button
              key={item.id}
              onClick={() => setActiveTab(item.id)}
              className={`w-full flex items-center justify-between px-3 py-2.5 rounded-lg text-xs font-medium transition-all ${
                isActive
                  ? 'bg-emerald-600/20 text-emerald-300 border border-emerald-500/30'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]'
              }`}
            >
              <div className="flex items-center space-x-3">
                <Icon className={`w-4 h-4 ${isActive ? 'text-emerald-400' : 'text-slate-400'}`} />
                <span>{item.label}</span>
              </div>
              {item.badge !== undefined && (
                <span className={`px-1.5 py-0.5 text-[10px] rounded font-mono ${
                  isActive ? 'bg-emerald-500/20 text-emerald-300' : 'bg-slate-800 text-slate-400'
                }`}>
                  {item.badge}
                </span>
              )}
            </button>
          );
        })}
      </div>

      <div className="p-3 bg-[#161b22] border border-[#30363d] rounded-lg text-[11px] font-mono text-slate-400 space-y-1">
        <div className="text-slate-300 font-semibold flex items-center justify-between">
          <span>HARDWARE ENGINE</span>
          <span className="h-2 w-2 rounded-full bg-emerald-400"></span>
        </div>
        <p className="text-[10px] text-slate-400">ONVIF, ISAPI, Dahua CGI, CP PLUS, RTSP Native Adapters Ready.</p>
      </div>
    </aside>
  );
};
