import React from 'react';
import { Shield, Activity, HardDrive, Sun, Moon, User, LogOut, Key } from 'lucide-react';
import { License, Session } from '../types';

interface NavbarProps {
  license: License | null;
  status: any;
  session: Session | null;
  darkMode: boolean;
  setDarkMode: (val: boolean) => void;
  onOpenAuth: () => void;
  onLogout: () => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  license,
  status,
  session,
  darkMode,
  setDarkMode,
  onOpenAuth,
  onLogout,
}) => {
  return (
    <header className="h-16 border-b border-[#30363d] bg-[#161b22] px-6 flex items-center justify-between z-20">
      <div className="flex items-center space-x-3">
        <div className="h-10 w-10 rounded-lg bg-emerald-600/20 border border-emerald-500/40 flex items-center justify-center text-emerald-400">
          <Shield className="w-6 h-6 text-emerald-400" />
        </div>
        <div>
          <div className="flex items-center space-x-2">
            <h1 className="text-base font-bold tracking-wider text-slate-100 uppercase">CCTV SENTINEL</h1>
            <span className="text-[10px] font-mono uppercase bg-emerald-950 text-emerald-400 border border-emerald-800 px-1.5 py-0.5 rounded">UNIVERSAL v1.0</span>
          </div>
          <p className="text-[11px] text-slate-400 font-mono tracking-tight">ONE TOOL. EVERY CCTV. COMPLETE CONTROL. — TECH DEFENCE SOLUTIONS</p>
        </div>
      </div>

      <div className="flex items-center space-x-3">
        {/* Connection status */}
        <div className="flex items-center space-x-2 px-3 py-1 bg-slate-900 border border-slate-700 rounded-md text-xs font-mono">
          <Activity className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
          <span className="text-slate-300">CORE AGENT:</span>
          <span className="text-emerald-400 font-bold">{status?.status || 'ONLINE'}</span>
        </div>

        {/* License Tier */}
        <div className="flex items-center space-x-1.5 px-3 py-1 bg-emerald-950/60 border border-emerald-700/60 rounded-md text-xs font-mono">
          <HardDrive className="w-3.5 h-3.5 text-emerald-400" />
          <span className="text-slate-300">TIER:</span>
          <span className="text-emerald-300 font-bold">{license?.tier || 'COMMUNITY FREE'}</span>
        </div>

        {/* Auth / Operator Profile */}
        {session ? (
          <div className="flex items-center space-x-2 px-3 py-1 bg-slate-900 border border-slate-700 rounded-md text-xs font-mono">
            <User className="w-3.5 h-3.5 text-emerald-400" />
            <span className="text-slate-300">{session.username}</span>
            <span className="text-[10px] bg-slate-800 text-emerald-400 px-1 py-0.5 rounded border border-slate-700 uppercase">
              {session.is_global_admin ? 'GLOBAL ADMIN' : session.role}
            </span>
            <button
              onClick={onLogout}
              className="ml-1 p-1 hover:text-rose-400 text-slate-400 transition-colors"
              title="Sign Out"
            >
              <LogOut className="w-3.5 h-3.5" />
            </button>
          </div>
        ) : (
          <button
            onClick={onOpenAuth}
            className="flex items-center space-x-1.5 px-3 py-1 bg-emerald-600/20 hover:bg-emerald-600/30 border border-emerald-500/50 text-emerald-300 rounded-md text-xs font-mono transition-colors font-bold"
          >
            <Key className="w-3.5 h-3.5 text-emerald-400" />
            <span>SIGN IN / SETUP</span>
          </button>
        )}

        {/* Dark/Light mode toggle */}
        <button
          onClick={() => setDarkMode(!darkMode)}
          className="p-2 rounded-md border border-slate-700 hover:bg-slate-800 text-slate-300 transition-colors"
          title="Toggle Theme"
        >
          {darkMode ? <Sun className="w-4 h-4 text-amber-400" /> : <Moon className="w-4 h-4 text-slate-300" />}
        </button>
      </div>
    </header>
  );
};
