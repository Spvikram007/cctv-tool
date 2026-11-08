import React, { useState, useEffect } from 'react';
import { api } from './services/api';
import { Device, License, AuditLogEntry, Session } from './types';
import { Navbar } from './components/Navbar';
import { Sidebar, NavTab } from './components/Sidebar';
import { AuthModal } from './components/AuthModal';
import { Dashboard } from './pages/Dashboard';
import { Discovery } from './pages/Discovery';
import { Devices } from './pages/Devices';
import { LiveView } from './pages/LiveView';
import { Configuration } from './pages/Configuration';
import { RecorderManager } from './pages/RecorderManager';
import { PTZControl } from './pages/PTZControl';
import { Diagnostics } from './pages/Diagnostics';
import { Calculators } from './pages/Calculators';
import { Licensing } from './pages/Licensing';
import { Compatibility } from './pages/Compatibility';
import { AuditLogs } from './pages/AuditLogs';
import { BatchConfig } from './pages/BatchConfig';
import { Firmware } from './pages/Firmware';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<NavTab>('dashboard');
  const [devices, setDevices] = useState<Device[]>([]);
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
  const [license, setLicense] = useState<License | null>(null);
  const [machineHash, setMachineHash] = useState<string>('');
  const [status, setStatus] = useState<any>(null);
  const [auditLogs, setAuditLogs] = useState<AuditLogEntry[]>([]);
  const [darkMode, setDarkMode] = useState(true);

  // Authentication State
  const [session, setSession] = useState<Session | null>(null);
  const [isAuthModalOpen, setIsAuthModalOpen] = useState(false);

  useEffect(() => {
    // Restore authenticated session from HttpOnly cookie on initial mount
    const restoreSession = async () => {
      try {
        const res = await api.getCurrentSession();
        if (res && res.session) {
          setSession(res.session);
        }
      } catch {
        // Not authenticated yet
      }
    };
    restoreSession();
    loadAllData();
  }, []); // Run once on initial mount

  // Periodic polling every 10s only when authenticated
  useEffect(() => {
    if (!session) return;
    const interval = setInterval(() => {
      loadAllData();
    }, 10000);
    return () => clearInterval(interval);
  }, [session]);

  const loadAllData = async () => {
    try {
      const [devs, sys, licData, logs] = await Promise.all([
        api.getDevices(),
        api.getStatus(),
        api.getLicensing(),
        api.getAuditLogs(),
      ]);
      setDevices(devs);
      setStatus(sys);
      setLicense(licData.license);
      setMachineHash(licData.machine_hash);
      setAuditLogs(logs);

      if (devs.length > 0 && !selectedDevice) {
        setSelectedDevice(devs[0]);
      }
    } catch (err: any) {
      if (err.message && err.message.includes('Authentication required')) {
        // Unauthenticated visitor: clear session without popping up intrusive blocking modals
        setSession(null);
      } else {
        console.warn('Backend connection notice:', err);
      }
    }
  };

  const handleLogout = async () => {
    await api.logout();
    setSession(null);
    setDevices([]);
    setSelectedDevice(null);
    setIsAuthModalOpen(true);
  };

  return (
    <div className={`h-screen overflow-hidden ${darkMode ? 'dark bg-[#0d1117] text-slate-100' : 'bg-slate-50 text-slate-900'} flex flex-col font-sans transition-colors duration-200`}>
      {/* Top Navbar */}
      <Navbar
        license={license}
        status={status}
        session={session}
        darkMode={darkMode}
        setDarkMode={setDarkMode}
        onOpenAuth={() => setIsAuthModalOpen(true)}
        onLogout={handleLogout}
      />

      {/* Auth & Setup Modal */}
      <AuthModal
        isOpen={isAuthModalOpen}
        onClose={() => setIsAuthModalOpen(false)}
        onAuthenticated={(newSession) => {
          setSession(newSession);
          setIsAuthModalOpen(false);
          loadAllData();
        }}
      />

      <div className="flex-1 flex overflow-hidden min-h-0">
        {/* Sidebar */}
        <Sidebar
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          deviceCount={devices.length}
        />

        {/* Main Workspace Body */}
        <main className="flex-1 overflow-y-auto p-6 bg-[#090d13] min-h-0">
          {activeTab === 'dashboard' && (
            <Dashboard
              devices={devices}
              license={license}
              auditLogs={auditLogs}
              onNavigate={setActiveTab}
              onSelectDevice={(d) => setSelectedDevice(d)}
            />
          )}

          {activeTab === 'discovery' && (
            <Discovery onDeviceEnrolled={loadAllData} />
          )}

          {activeTab === 'devices' && (
            <Devices
              devices={devices}
              onSelectDevice={(d) => setSelectedDevice(d)}
              onNavigate={setActiveTab}
              onRefresh={loadAllData}
            />
          )}

          {activeTab === 'live' && (
            <LiveView
              devices={devices}
              selectedDevice={selectedDevice}
            />
          )}

          {activeTab === 'config' && (
            <Configuration
              devices={devices}
              selectedDevice={selectedDevice}
              onSelectDevice={(d) => setSelectedDevice(d)}
            />
          )}

          {activeTab === 'recorder' && (
            <RecorderManager
              devices={devices}
              selectedDevice={selectedDevice}
              onSelectDevice={(d) => setSelectedDevice(d)}
            />
          )}

          {activeTab === 'ptz' && (
            <PTZControl
              devices={devices}
              selectedDevice={selectedDevice}
              onSelectDevice={(d) => setSelectedDevice(d)}
            />
          )}

          {activeTab === 'diagnostics' && (
            <Diagnostics />
          )}

          {activeTab === 'calculators' && (
            <Calculators />
          )}

          {activeTab === 'batch' && (
            <BatchConfig
              devices={devices}
              onRefresh={loadAllData}
            />
          )}

          {activeTab === 'firmware' && (
            <Firmware
              devices={devices}
            />
          )}

          {activeTab === 'compatibility' && (
            <Compatibility />
          )}

          {activeTab === 'licensing' && (
            <Licensing
              license={license}
              machineHash={machineHash}
              onRefreshLicense={loadAllData}
            />
          )}

          {activeTab === 'audit' && (
            <AuditLogs
              auditLogs={auditLogs}
              devices={devices}
            />
          )}
        </main>
      </div>
    </div>
  );
};

export default App;
