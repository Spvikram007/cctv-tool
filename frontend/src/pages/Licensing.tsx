import React, { useState } from 'react';
import { License } from '../types';
import { api } from '../services/api';
import { KeyRound, FileCheck, Copy } from 'lucide-react';

interface LicensingProps {
  license: License | null;
  machineHash: string;
  onRefreshLicense: () => void;
}

export const Licensing: React.FC<LicensingProps> = ({
  license,
  machineHash,
  onRefreshLicense,
}) => {
  const [keyInput, setKeyInput] = useState('');
  const [companyInput, setCompanyInput] = useState('');
  const [gstInput, setGstInput] = useState('29AABCT1337Q1ZS');
  const [stateInput, setStateInput] = useState('Karnataka');
  const [tierSelect] = useState('TECHNICIAN_PRO');
  const [activating, setActivating] = useState(false);
  const [invoice, setInvoice] = useState<any>(null);
  const [copied, setCopied] = useState(false);

  const handleCopyHash = () => {
    navigator.clipboard.writeText(machineHash);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleActivate = async () => {
    if (!keyInput) {
      alert('Please enter a license key.');
      return;
    }
    setActivating(true);
    try {
      await api.activateLicense(keyInput, companyInput || 'Enterprise Client', gstInput);
      alert('License activated successfully!');
      onRefreshLicense();
      setKeyInput('');
    } catch (err: any) {
      alert(`Activation rejected: ${err.message}`);
    } finally {
      setActivating(false);
    }
  };

  const handleGenerateInvoice = async () => {
    try {
      const res = await api.generateGSTInvoice(companyInput || 'Enterprise Customer', gstInput, stateInput, tierSelect);
      setInvoice(res);
    } catch (err: any) {
      alert(err.message);
    }
  };

  return (
    <div className="space-y-6 font-mono">
      {/* Current License Card */}
      <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <div className="flex items-center space-x-2">
              <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                ACTIVE LICENSE
              </span>
              <span className="text-xs text-slate-400">COMMERCIAL ENTITLEMENT</span>
            </div>
            <h2 className="text-2xl font-bold text-slate-100 mt-1">{license?.tier || 'COMMUNITY FREE'}</h2>
            <p className="text-xs text-slate-400 mt-0.5">Licensed to: {license?.owner_name || 'Community Technician'}</p>
          </div>

          <div className="flex items-center space-x-4">
            <div className="text-right">
              <p className="text-[10px] text-slate-400">DEVICE QUOTA</p>
              <p className="text-lg font-bold text-emerald-400">{license?.max_devices || 8} Cameras</p>
            </div>
            <div className="text-right">
              <p className="text-[10px] text-slate-400">EXPIRATION</p>
              <p className="text-lg font-bold text-slate-200">
                {license?.expires_at ? new Date(license.expires_at).toLocaleDateString() : 'Perpetual'}
              </p>
            </div>
          </div>
        </div>

        {/* Hardware ID / Machine Hash */}
        <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg flex items-center justify-between">
          <div className="text-xs">
            <span className="text-slate-400">MACHINE HARDWARE FINGERPRINT: </span>
            <span className="text-emerald-400 font-bold">{machineHash}</span>
          </div>
          <button
            onClick={handleCopyHash}
            className="px-2.5 py-1 bg-[#21262d] hover:bg-[#30363d] text-slate-300 rounded text-xs flex items-center space-x-1"
          >
            <Copy className="w-3 h-3" />
            <span>{copied ? 'COPIED!' : 'COPY ID'}</span>
          </button>
        </div>
      </div>

      {/* Tier Comparison Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Free */}
        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3 flex flex-col justify-between">
          <div className="space-y-2">
            <h3 className="text-sm font-bold text-slate-200">COMMUNITY FREE</h3>
            <p className="text-xl font-bold text-slate-100">₹0 <span className="text-xs text-slate-400">/ forever</span></p>
            <ul className="text-xs text-slate-400 space-y-1.5 pt-2">
              <li>✓ Up to 8 Cameras / Devices</li>
              <li>✓ Real ONVIF Discovery</li>
              <li>✓ Basic RTSP Streaming</li>
              <li>✓ Subnet & HDD Calculators</li>
              <li>✗ No Batch Configuration</li>
              <li>✗ No Offline Activation</li>
            </ul>
          </div>
        </div>

        {/* Pro */}
        <div className="p-5 rounded-xl bg-[#161b22] border-2 border-emerald-500/50 space-y-3 flex flex-col justify-between relative">
          <span className="absolute -top-3 right-4 px-2 py-0.5 rounded text-[10px] bg-emerald-600 text-white font-bold">
            RECOMMENDED
          </span>
          <div className="space-y-2">
            <h3 className="text-sm font-bold text-emerald-400">TECHNICIAN PRO</h3>
            <p className="text-xl font-bold text-slate-100">₹4,999 <span className="text-xs text-slate-400">+ 18% GST</span></p>
            <ul className="text-xs text-slate-300 space-y-1.5 pt-2">
              <li>✓ Up to 64 Cameras across 10 Sites</li>
              <li>✓ Universal 12-Step Config Engine</li>
              <li>✓ Hikvision ISAPI & Dahua CGI</li>
              <li>✓ XVR Analog Format Switching (UTC)</li>
              <li>✓ Automated Batch Operations</li>
              <li>✓ Offline Hardware Key Activation</li>
            </ul>
          </div>
        </div>

        {/* Enterprise */}
        <div className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3 flex flex-col justify-between">
          <div className="space-y-2">
            <h3 className="text-sm font-bold text-purple-400">DEALER ENTERPRISE</h3>
            <p className="text-xl font-bold text-slate-100">₹14,999 <span className="text-xs text-slate-400">+ 18% GST</span></p>
            <ul className="text-xs text-slate-300 space-y-1.5 pt-2">
              <li>✓ Unlimited Devices & Unlimited Sites</li>
              <li>✓ Custom Company Branding & Logo</li>
              <li>✓ Priority Firmware Deployment</li>
              <li>✓ Multi-seat Technician Licensing</li>
              <li>✓ GST Invoicing & Tax Compliance</li>
              <li>✓ 24/7 Dedicated Integrator Support</li>
            </ul>
          </div>
        </div>
      </div>

      {/* Activation and GST Invoice Generator */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Offline License Activation Box */}
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
          <div className="flex items-center space-x-2">
            <KeyRound className="w-5 h-5 text-emerald-400" />
            <h3 className="text-sm font-bold text-slate-100 uppercase">OFFLINE LICENSE ACTIVATION</h3>
          </div>
          <p className="text-xs text-slate-400">
            Enter the signed cryptographically verified key provided by Tech Defence Solutions.
          </p>

          <div className="space-y-3">
            <div>
              <label className="block text-[11px] text-slate-400 mb-1">LICENSE KEY</label>
              <input
                type="text"
                placeholder="SENTINEL-TECHNICIAN_PRO-YYYYMMDD-XXXX"
                value={keyInput}
                onChange={(e) => setKeyInput(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500 font-mono"
              />
            </div>

            <div>
              <label className="block text-[11px] text-slate-400 mb-1">COMPANY / LICENSEE NAME</label>
              <input
                type="text"
                placeholder="Tech Integrators Pvt Ltd"
                value={companyInput}
                onChange={(e) => setCompanyInput(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <button
              onClick={handleActivate}
              disabled={activating}
              className="w-full py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold transition-all shadow-lg shadow-emerald-900/30"
            >
              {activating ? 'VERIFYING KEY...' : 'ACTIVATE HARDWARE LICENSE'}
            </button>
          </div>
        </div>

        {/* Commercial GST Invoice Generator */}
        <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
          <div className="flex items-center space-x-2">
            <FileCheck className="w-5 h-5 text-emerald-400" />
            <h3 className="text-sm font-bold text-slate-100 uppercase">TAX INVOICE GENERATOR (GST COMPLIANT)</h3>
          </div>
          <p className="text-xs text-slate-400">
            Instantly calculate HSN 998313 software licensing tax breakdown with CGST/SGST/IGST.
          </p>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] text-slate-400 mb-1">CUSTOMER GSTIN</label>
              <input
                type="text"
                value={gstInput}
                onChange={(e) => setGstInput(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-[11px] text-slate-400 mb-1">STATE</label>
              <input
                type="text"
                value={stateInput}
                onChange={(e) => setStateInput(e.target.value)}
                className="w-full px-3 py-2 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs text-slate-200 focus:outline-none"
              />
            </div>
          </div>

          <button
            onClick={handleGenerateInvoice}
            className="w-full py-2.5 rounded-lg bg-[#21262d] hover:bg-[#30363d] text-slate-200 text-xs font-bold border border-slate-700 transition-colors"
          >
            GENERATE COMMERCIAL GST INVOICE
          </button>

          {invoice && (
            <div className="p-3 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs space-y-1">
              <div className="flex justify-between text-slate-400">
                <span>INVOICE NUMBER:</span>
                <span className="text-slate-200 font-bold">{invoice.invoice_number}</span>
              </div>
              <div className="flex justify-between text-slate-400">
                <span>BASE AMOUNT:</span>
                <span>₹{invoice.base_amount.toFixed(2)}</span>
              </div>
              {invoice.cgst > 0 && (
                <div className="flex justify-between text-slate-400">
                  <span>CGST (9%) + SGST (9%):</span>
                  <span>₹{(invoice.cgst + invoice.sgst).toFixed(2)}</span>
                </div>
              )}
              {invoice.igst > 0 && (
                <div className="flex justify-between text-slate-400">
                  <span>IGST (18%):</span>
                  <span>₹{invoice.igst.toFixed(2)}</span>
                </div>
              )}
              <div className="flex justify-between font-bold text-emerald-400 pt-1 border-t border-[#21262d]">
                <span>TOTAL PAYABLE:</span>
                <span>₹{invoice.total_amount.toFixed(2)}</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
