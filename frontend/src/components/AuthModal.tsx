import React, { useState, useEffect } from 'react';
import { Shield, Key, User, Lock, Mail, CheckCircle2, AlertTriangle, ArrowRight, X } from 'lucide-react';
import { api } from '../services/api';
import { Session } from '../types';

interface AuthModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAuthenticated: (session: Session) => void;
}

export const AuthModal: React.FC<AuthModalProps> = ({ isOpen, onClose, onAuthenticated }) => {
  const [setupRequired, setSetupRequired] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Login form state
  const [loginUsername, setLoginUsername] = useState('');
  const [loginPassword, setLoginPassword] = useState('');

  // Setup form state
  const [setupToken, setSetupToken] = useState('');
  const [setupUsername, setSetupUsername] = useState('');
  const [setupFullName, setSetupFullName] = useState('');
  const [setupEmail, setSetupEmail] = useState('');
  const [setupPassword, setSetupPassword] = useState('');
  const [setupConfirm, setSetupConfirm] = useState('');

  useEffect(() => {
    if (isOpen) {
      checkSetupStatus();
    }
  }, [isOpen]);

  const checkSetupStatus = async () => {
    try {
      const res = await api.getSetupStatus();
      setSetupRequired(res.setup_required);
      setError(null);
    } catch (err: any) {
      console.warn('Could not query setup status:', err);
      setSetupRequired(false);
    }
  };

  if (!isOpen) return null;

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const res = await api.login(loginUsername, loginPassword);
      if (res && res.session) {
        onAuthenticated(res.session);
        onClose();
      }
    } catch (err: any) {
      setError(err.message || 'Login failed: check username and password');
    } finally {
      setLoading(false);
    }
  };

  const handleCompleteSetup = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!setupToken.trim()) {
      setError('Please provide the one-time setup token from sentinel-setup.token on the server host.');
      return;
    }
    if (setupPassword !== setupConfirm) {
      setError('Passwords do not match.');
      return;
    }
    if (setupPassword.length < 10) {
      setError('Password must be at least 10 characters long.');
      return;
    }

    setLoading(true);
    try {
      const res = await api.completeSetup({
        setup_token: setupToken.trim(),
        username: setupUsername.trim(),
        password: setupPassword,
        full_name: setupFullName.trim(),
        email: setupEmail.trim(),
      });

      if (res && res.success) {
        setSuccess('Administrator provisioned successfully! Authenticating session...');
        // Automatically log in with the new credentials
        const loginRes = await api.login(setupUsername.trim(), setupPassword);
        if (loginRes && loginRes.session) {
          onAuthenticated(loginRes.session);
          onClose();
        }
      }
    } catch (err: any) {
      setError(err.message || 'Failed to complete setup');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4"
    >
      <div className="w-full max-w-md bg-[#161b22] border border-[#30363d] rounded-xl shadow-2xl overflow-hidden text-slate-100">
        {/* Header */}
        <div className="bg-[#0d1117] border-b border-[#30363d] px-6 py-4 flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="p-2 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
              <Shield className="w-6 h-6" />
            </div>
            <div>
              <h2 className="text-sm font-bold uppercase tracking-wider text-slate-100">
                {setupRequired ? 'First-Run Initialization' : 'Operator Authentication'}
              </h2>
              <p className="text-xs text-slate-400 font-mono">
                {setupRequired ? 'ZERO-DEFAULT-CREDENTIAL PROVISIONING' : 'SECURE HTTPONLY SESSION & CSRF GUARD'}
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-md text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
            title="Close"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form Body */}
        <div className="p-6">
          {error && (
            <div className="mb-4 p-3 bg-rose-950/50 border border-rose-800/80 rounded-lg flex items-start space-x-2.5 text-xs text-rose-300">
              <AlertTriangle className="w-4 h-4 shrink-0 text-rose-400 mt-0.5" />
              <span>{error}</span>
            </div>
          )}

          {success && (
            <div className="mb-4 p-3 bg-emerald-950/50 border border-emerald-800/80 rounded-lg flex items-start space-x-2.5 text-xs text-emerald-300">
              <CheckCircle2 className="w-4 h-4 shrink-0 text-emerald-400 mt-0.5" />
              <span>{success}</span>
            </div>
          )}

          {setupRequired ? (
            <form onSubmit={handleCompleteSetup} className="space-y-4">
              <div className="text-xs text-slate-400 font-mono bg-slate-900/60 p-3 rounded border border-slate-800">
                A cryptographic one-time setup token was generated in <span className="text-emerald-400">sentinel-setup.token</span> on the host server.
              </div>

              <div>
                <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Setup Token</label>
                <div className="relative">
                  <Key className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                  <input
                    type="text"
                    required
                    value={setupToken}
                    onChange={(e) => setSetupToken(e.target.value)}
                    placeholder="sent-setup-..."
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Username</label>
                  <div className="relative">
                    <User className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                    <input
                      type="text"
                      required
                      value={setupUsername}
                      onChange={(e) => setSetupUsername(e.target.value)}
                      className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Full Name</label>
                  <input
                    type="text"
                    value={setupFullName}
                    onChange={(e) => setSetupFullName(e.target.value)}
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 px-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Email</label>
                <div className="relative">
                  <Mail className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                  <input
                    type="email"
                    value={setupEmail}
                    onChange={(e) => setSetupEmail(e.target.value)}
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Master Password</label>
                <div className="relative">
                  <Lock className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                  <input
                    type="password"
                    required
                    value={setupPassword}
                    onChange={(e) => setSetupPassword(e.target.value)}
                    placeholder="10+ chars, uppercase, lowercase, symbol"
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Confirm Password</label>
                <div className="relative">
                  <Lock className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                  <input
                    type="password"
                    required
                    value={setupConfirm}
                    onChange={(e) => setSetupConfirm(e.target.value)}
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full mt-4 bg-emerald-600 hover:bg-emerald-500 disabled:bg-slate-700 text-white font-mono text-xs uppercase py-2.5 px-4 rounded-lg flex items-center justify-center space-x-2 transition-colors font-bold shadow-lg shadow-emerald-900/30"
              >
                {loading ? 'Provisioning...' : 'Provision Initial Administrator'}
                <ArrowRight className="w-4 h-4 ml-1" />
              </button>
            </form>
          ) : (
            <form onSubmit={handleLogin} className="space-y-4">
              <div>
                <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Operator Username</label>
                <div className="relative">
                  <User className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                  <input
                    type="text"
                    required
                    value={loginUsername}
                    onChange={(e) => setLoginUsername(e.target.value)}
                    placeholder="Operator username"
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-mono text-slate-300 uppercase mb-1">Password</label>
                <div className="relative">
                  <Lock className="w-4 h-4 text-slate-500 absolute left-3 top-2.5" />
                  <input
                    type="password"
                    required
                    value={loginPassword}
                    onChange={(e) => setLoginPassword(e.target.value)}
                    placeholder="••••••••••••"
                    className="w-full bg-[#0d1117] border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-xs font-mono text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full mt-4 bg-emerald-600 hover:bg-emerald-500 disabled:bg-slate-700 text-white font-mono text-xs uppercase py-2.5 px-4 rounded-lg flex items-center justify-center space-x-2 transition-colors font-bold shadow-lg shadow-emerald-900/30"
              >
                {loading ? 'Authenticating...' : 'Sign In To CCTV Sentinel'}
                <ArrowRight className="w-4 h-4 ml-1" />
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
};

