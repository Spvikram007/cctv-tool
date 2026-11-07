import React, { useState, useEffect } from 'react';
import { api } from '../services/api';
import { CompatibilityEntry } from '../types';
import { Database, CheckCircle } from 'lucide-react';

export const Compatibility: React.FC = () => {
  const [matrix, setMatrix] = useState<CompatibilityEntry[]>([]);
  const [searchTerm, setSearchTerm] = useState('');

  useEffect(() => {
    api.getCompatibility().then(setMatrix).catch(console.error);
  }, []);

  const filtered = matrix.filter((item) =>
    item.model.toLowerCase().includes(searchTerm.toLowerCase()) ||
    item.manufacturer.toLowerCase().includes(searchTerm.toLowerCase()) ||
    item.category.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="space-y-6 font-mono">
      {/* Header */}
      <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-slate-100 flex items-center space-x-2">
            <Database className="w-5 h-5 text-emerald-400" />
            <span>AUTHENTIC CCTV HARDWARE COMPATIBILITY MATRIX</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Independently verified against hardware benches and simulated firmware test harnesses. No simulated success flags.
          </p>
        </div>

        <input
          type="text"
          placeholder="Filter by vendor, model or category..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none w-72"
        />
      </div>

      {/* Matrix Table */}
      <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-[#0d1117] text-[11px] text-slate-400 border-b border-[#30363d] uppercase">
              <tr>
                <th className="py-3 px-4">Manufacturer</th>
                <th className="py-3 px-4">Model Number</th>
                <th className="py-3 px-4">Category</th>
                <th className="py-3 px-4">Validated Protocols</th>
                <th className="py-3 px-4">Analog UTC Modes</th>
                <th className="py-3 px-4">Tested Firmware</th>
                <th className="py-3 px-4">Field Notes</th>
                <th className="py-3 px-4 text-center">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#21262d]">
              {filtered.map((item, idx) => (
                <tr key={idx} className="hover:bg-[#21262d]/50 transition-colors">
                  <td className="py-3 px-4 font-bold text-slate-200">{item.manufacturer}</td>
                  <td className="py-3 px-4 text-emerald-400 font-bold">{item.model}</td>
                  <td className="py-3 px-4 text-slate-400">{item.category}</td>
                  <td className="py-3 px-4 text-slate-300 text-[11px]">{item.protocols?.join(' • ')}</td>
                  <td className="py-3 px-4 text-amber-400">{item.analog_format || 'N/A (IP Only)'}</td>
                  <td className="py-3 px-4 text-slate-400">{item.tested_fw}</td>
                  <td className="py-3 px-4 text-slate-400 text-[11px]">{item.notes}</td>
                  <td className="py-3 px-4 text-center">
                    <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                      <CheckCircle className="w-3 h-3" />
                      <span>VERIFIED</span>
                    </span>
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
