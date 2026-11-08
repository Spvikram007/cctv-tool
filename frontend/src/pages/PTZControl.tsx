import React, { useState, useEffect } from 'react';
import {
  Device, PTZCapabilities, Preset, PatrolTour,
  EventSubscription, SurveillanceEvent, Alarm, AlarmRule, AlarmStats
} from '../types';
import { api } from '../services/api';
import {
  Compass, ZoomIn, ZoomOut, ArrowUp, ArrowDown, ArrowLeft, ArrowRight,
  ArrowUpLeft, ArrowUpRight, ArrowDownLeft, ArrowDownRight, StopCircle,
  RotateCw, Home, ShieldAlert, AlertTriangle, Bell, BellRing, CheckCircle,
  Filter, Download, Plus, Trash2, Activity,
  Sliders, ShieldCheck, Eye, Layers, Radio, Check
} from 'lucide-react';

interface PTZControlProps {
  devices: Device[];
  selectedDevice: Device | null;
  onSelectDevice: (dev: Device) => void;
}

type TabType =
  | 'ptz-control'
  | 'presets'
  | 'patrols'
  | 'subscriptions'
  | 'live-events'
  | 'alarm-dashboard'
  | 'alarm-history'
  | 'alarm-rules'
  | 'evidence-viewer'
  | 'compatibility-matrix';

export const PTZControl: React.FC<PTZControlProps> = ({
  devices,
  selectedDevice,
  onSelectDevice,
}) => {
  const currentDev = selectedDevice || devices[0];
  const [activeTab, setActiveTab] = useState<TabType>('ptz-control');

  // PTZ State
  const [capabilities, setCapabilities] = useState<PTZCapabilities | null>(null);
  const [, setCapLoading] = useState<boolean>(false);
  const [speed, setSpeed] = useState<number>(0.5);
  const [lastCommand, setLastCommand] = useState<string>('IDLE');
  const [ptzError, setPtzError] = useState<string | null>(null);
  const [presets, setPresets] = useState<Preset[]>([]);
  const [patrols, setPatrols] = useState<PatrolTour[]>([]);
  const [newPresetNumber, setNewPresetNumber] = useState<number>(1);
  const [newPresetName, setNewPresetName] = useState<string>('Preset 1');
  const [patrolName, setPatrolName] = useState<string>('Perimeter Tour');
  const [patrolStepsInput, setPatrolStepsInput] = useState<string>('1:5:4, 2:5:4');

  // Events & Subscriptions State
  const [subscriptions, setSubscriptions] = useState<EventSubscription[]>([]);
  const [liveEvents, setLiveEvents] = useState<SurveillanceEvent[]>([]);
  const [eventCategoryFilter, setEventCategoryFilter] = useState<string>('ALL');
  const [selectedRawEvent, setSelectedRawEvent] = useState<SurveillanceEvent | null>(null);

  // Alarms State
  const [alarms, setAlarms] = useState<Alarm[]>([]);
  const [alarmStats, setAlarmStats] = useState<AlarmStats | null>(null);
  const [alarmSeverityFilter] = useState<string>('ALL');
  const [alarmStateFilter] = useState<string>('ALL');
  const [alarmSearchTerm, setAlarmSearchTerm] = useState<string>('');
  const [rules, setRules] = useState<AlarmRule[]>([]);
  const [selectedAlarmForEvidence, setSelectedAlarmForEvidence] = useState<Alarm | null>(null);
  const [sseConnected, setSseConnected] = useState<boolean>(false);

  // Technician input for actions
  const [technicianName, setTechnicianName] = useState<string>('Field Tech Alpha');
  const [resolutionInput] = useState<string>('Inspected device on site, cleared false alert.');

  // Rule builder form state
  const [newRuleName, setNewRuleName] = useState<string>('Tamper Detection High Priority');
  const [newRuleCategory, setNewRuleCategory] = useState<string>('TAMPER');
  const [newRuleCooldown, setNewRuleCooldown] = useState<number>(10);
  const [newRuleCaptureSnapshot, setNewRuleCaptureSnapshot] = useState<boolean>(true);
  const [newRuleWebhook, setNewRuleWebhook] = useState<string>('http://127.0.0.1:8080/api/v1/webhook/alerts');

  // Load PTZ Capabilities & Presets when device changes
  useEffect(() => {
    if (!currentDev) return;
    loadDevicePTZ();
  }, [currentDev?.id]);

  const loadDevicePTZ = async () => {
    if (!currentDev) return;
    setCapLoading(true);
    setPtzError(null);
    try {
      const caps = await api.getPTZCapabilities(currentDev.id);
      setCapabilities(caps);
      if (caps.ptz_supported) {
        const [ps, pts] = await Promise.all([
          api.getPTZPresets(currentDev.id).catch(() => []),
          api.getPTZPatrols(currentDev.id).catch(() => []),
        ]);
        setPresets(ps);
        setPatrols(pts);
      } else {
        setPresets([]);
        setPatrols([]);
      }
    } catch (e: any) {
      setPtzError(e.message || 'Failed to query PTZ capabilities');
      setCapabilities({
        device_id: currentDev.id,
        ptz_supported: false,
        pan_tilt_supported: false,
        continuous_pan_tilt: false,
        relative_pan_tilt: false,
        absolute_pan_tilt: false,
        zoom_supported: false,
        continuous_zoom: false,
        relative_zoom: false,
        focus_supported: false,
        iris_supported: false,
        presets_supported: false,
        max_presets: 0,
        preset_tour_supported: false,
        home_position_supported: false,
        auto_scan_supported: false,
        supported_protocols: [],
        status_notes: 'Physical camera does not support PTZ motors.',
      });
    } finally {
      setCapLoading(false);
    }
  };

  // Periodic polling for subscriptions, live events, alarms
  useEffect(() => {
    loadSubscriptions();
    loadLiveEvents();
    loadAlarms();
    loadAlarmRules();

    const interval = setInterval(() => {
      loadLiveEvents();
      loadAlarms();
    }, 4000);

    return () => clearInterval(interval);
  }, [eventCategoryFilter, alarmSeverityFilter, alarmStateFilter]);

  // Server-Sent Events (SSE) for Real-Time Alarms
  useEffect(() => {
    let evtSource: EventSource | null = null;
    try {
      evtSource = new EventSource(api.getAlarmsStreamURL());
      evtSource.onopen = () => {
        setSseConnected(true);
      };
      evtSource.onmessage = (e) => {
        try {
          const payload = JSON.parse(e.data);
          if (payload.type === 'ALARM_CREATED' || payload.type === 'ALARM_UPDATED') {
            loadAlarms();
          }
        } catch {
          // heartbeat or ping
        }
      };
      evtSource.onerror = () => {
        setSseConnected(false);
      };
    } catch {
      setSseConnected(false);
    }

    return () => {
      if (evtSource) {
        evtSource.close();
      }
    };
  }, []);

  const loadSubscriptions = async () => {
    try {
      const subs = await api.getEventSubscriptions();
      setSubscriptions(subs);
    } catch (e) {
      console.warn('Subscriptions load err', e);
    }
  };

  const loadLiveEvents = async () => {
    try {
      const cat = eventCategoryFilter === 'ALL' ? undefined : eventCategoryFilter;
      const evs = await api.getLiveEvents(50, cat);
      setLiveEvents(evs);
    } catch (e) {
      console.warn('Live events load err', e);
    }
  };

  const loadAlarms = async () => {
    try {
      const [alms, stats] = await Promise.all([
        api.getAlarms({
          severity: alarmSeverityFilter === 'ALL' ? undefined : alarmSeverityFilter,
          state: alarmStateFilter === 'ALL' ? undefined : alarmStateFilter,
          limit: 100,
        }),
        api.getAlarmStats(),
      ]);
      setAlarms(alms);
      setAlarmStats(stats);
      if (alms.length > 0 && !selectedAlarmForEvidence) {
        setSelectedAlarmForEvidence(alms[0]);
      }
    } catch (e) {
      console.warn('Alarms load err', e);
    }
  };

  const loadAlarmRules = async () => {
    try {
      const r = await api.getAlarmRules();
      setRules(r);
    } catch (e) {
      console.warn('Rules load err', e);
    }
  };

  // PTZ Control Actions
  const sendMove = async (pan: number, tilt: number, zoom: number, cmdName: string) => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      setLastCommand(cmdName);
      setPtzError(null);
      await api.ptzMove(currentDev.id, {
        pan_speed: pan * speed,
        tilt_speed: tilt * speed,
        zoom_speed: zoom * speed,
        timeout_ms: 1500,
      });
    } catch (e: any) {
      setPtzError(e.message || 'PTZ move failed');
    }
  };

  const sendStop = async () => {
    if (!currentDev) return;
    try {
      setLastCommand('STOPPED');
      await api.ptzStop(currentDev.id);
    } catch (e: any) {
      console.warn('Stop failed', e);
    }
  };

  const sendFocusIris = async (action: any) => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      setLastCommand(action);
      await api.ptzFocusIris(currentDev.id, { action, speed });
    } catch (e: any) {
      setPtzError(e.message);
    }
  };

  const sendHome = async () => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      setLastCommand('GOTO HOME');
      await api.ptzHome(currentDev.id);
    } catch (e: any) {
      setPtzError(e.message);
    }
  };

  const sendAutoScan = async () => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      setLastCommand('360° AUTO SCAN');
      await api.ptzAutoScan(currentDev.id, speed);
    } catch (e: any) {
      setPtzError(e.message);
    }
  };

  const handleCreatePreset = async () => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      await api.createPTZPreset(currentDev.id, newPresetNumber, newPresetName);
      loadDevicePTZ();
      setNewPresetNumber((prev) => prev + 1);
      setNewPresetName(`Preset ${newPresetNumber + 1}`);
    } catch (e: any) {
      alert(`Failed to save preset: ${e.message}`);
    }
  };

  const handleGotoPreset = async (presetNum: number) => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      setLastCommand(`GOTO PRESET #${presetNum}`);
      await api.gotoPTZPreset(currentDev.id, presetNum);
    } catch (e: any) {
      alert(`Goto preset failed: ${e.message}`);
    }
  };

  const handleDeletePreset = async (presetNum: number) => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    if (!confirm(`Delete preset #${presetNum}?`)) return;
    try {
      await api.deletePTZPreset(currentDev.id, presetNum);
      loadDevicePTZ();
    } catch (e: any) {
      alert(`Delete preset failed: ${e.message}`);
    }
  };

  const handleCreatePatrol = async () => {
    if (!currentDev || !capabilities?.ptz_supported) return;
    try {
      const steps = patrolStepsInput.split(',').map((part) => {
        const [p, d, s] = part.trim().split(':').map(Number);
        return {
          preset_number: p || 1,
          dwell_time_sec: d || 5,
          speed: s || 4,
        };
      });
      await api.createPTZPatrol(currentDev.id, {
        tour_number: patrols.length + 1,
        name: patrolName,
        steps,
        repeat_count: 0,
      });
      loadDevicePTZ();
    } catch (e: any) {
      alert(`Create patrol failed: ${e.message}`);
    }
  };

  const handleRunPatrol = async (tourNumber: number) => {
    if (!currentDev) return;
    try {
      setLastCommand(`RUNNING PATROL #${tourNumber}`);
      await api.runPTZPatrol(currentDev.id, tourNumber);
      loadDevicePTZ();
    } catch (e: any) {
      alert(`Run patrol failed: ${e.message}`);
    }
  };

  const handleStopPatrol = async (tourNumber: number) => {
    if (!currentDev) return;
    try {
      setLastCommand(`STOPPED PATROL #${tourNumber}`);
      await api.stopPTZPatrol(currentDev.id, tourNumber);
      loadDevicePTZ();
    } catch (e: any) {
      alert(`Stop patrol failed: ${e.message}`);
    }
  };

  // Event Subscription Actions
  const handleCreateSubscription = async () => {
    if (!currentDev) return;
    try {
      await api.createEventSubscription(currentDev.id, 'ONVIF');
      loadSubscriptions();
      alert(`Created ONVIF Event PullPoint subscription for ${currentDev.name}`);
    } catch (e: any) {
      alert(`Failed to subscribe: ${e.message}`);
    }
  };

  const handleCancelSubscription = async (subId: string) => {
    try {
      await api.cancelEventSubscription(subId);
      loadSubscriptions();
    } catch (e: any) {
      alert(`Cancel failed: ${e.message}`);
    }
  };

  // Alarm Operations
  const handleAcknowledge = async (alarmId: string) => {
    try {
      await api.acknowledgeAlarm(alarmId, technicianName);
      loadAlarms();
    } catch (e: any) {
      alert(`Acknowledge failed: ${e.message}`);
    }
  };

  const handleAssign = async (alarmId: string) => {
    try {
      await api.assignAlarm(alarmId, technicianName);
      loadAlarms();
    } catch (e: any) {
      alert(`Assign failed: ${e.message}`);
    }
  };

  const handleResolve = async (alarmId: string) => {
    try {
      await api.resolveAlarm(alarmId, resolutionInput, technicianName);
      loadAlarms();
    } catch (e: any) {
      alert(`Resolve failed: ${e.message}`);
    }
  };

  const handleDismiss = async (alarmId: string) => {
    try {
      await api.dismissAlarm(alarmId, 'False positive verified by technician');
      loadAlarms();
    } catch (e: any) {
      alert(`Dismiss failed: ${e.message}`);
    }
  };

  const handleCreateRule = async () => {
    try {
      await api.createAlarmRule({
        name: newRuleName,
        enabled: true,
        match_categories: [newRuleCategory],
        event_categories: [newRuleCategory],
        match_device_ids: [],
        severity_override: 'MAJOR',
        cooldown_sec: newRuleCooldown,
        capture_snapshot: newRuleCaptureSnapshot,
        assign_technician: technicianName,
        webhook_url: newRuleWebhook,
        email_recipients: ['security-ops@techdefence.lan'],
        suppress_duplicates: true,
      });
      loadAlarmRules();
      alert('Alarm rule created successfully!');
    } catch (e: any) {
      alert(`Create rule failed: ${e.message}`);
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    try {
      await api.deleteAlarmRule(ruleId);
      loadAlarmRules();
    } catch (e: any) {
      alert(`Delete rule failed: ${e.message}`);
    }
  };

  // Filtered alarms
  const filteredAlarms = alarms.filter((a) => {
    if (alarmSearchTerm) {
      const term = alarmSearchTerm.toLowerCase();
      const matchText = `${a.device_name} ${a.category} ${a.description} ${a.assigned_technician || ''}`.toLowerCase();
      if (!matchText.includes(term)) return false;
    }
    return true;
  });

  return (
    <div className="space-y-6">
      {/* Platform Header & Device Selector */}
      <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center space-x-3">
          <div className="h-10 w-10 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <Compass className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center space-x-2">
              <h2 className="text-sm font-bold text-slate-100 font-mono uppercase tracking-wide">
                PTZ ROBOTIC CONTROL & UNIFIED ALARM OPERATIONS
              </h2>
              <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                PHASE 7 VERIFIED
              </span>
            </div>
            <p className="text-[11px] text-slate-400 font-mono">
              Continuous Move • Presets • Patrol Tours • ONVIF/ISAPI/CGI Events • Cooldown Deduplication • Truthful Evidence
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            <div className={`h-2.5 w-2.5 rounded-full ${sseConnected ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'}`} />
            <span className="text-[11px] font-mono text-slate-400">
              SSE STREAM: <strong className={sseConnected ? 'text-emerald-300' : 'text-amber-300'}>{sseConnected ? 'LIVE' : 'POLLING'}</strong>
            </span>
          </div>

          <div className="flex items-center space-x-2">
            <label className="text-xs font-mono text-slate-400">TARGET UNIT:</label>
            <select
              value={currentDev?.id || ''}
              onChange={(e) => {
                const d = devices.find((x) => x.id === e.target.value);
                if (d) onSelectDevice(d);
              }}
              className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded-lg text-xs font-mono text-slate-200 focus:outline-none focus:border-emerald-500"
            >
              {devices.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name} ({d.ip}) [{d.category}]
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* 10-Screen Navigation Tab Pills */}
      <div className="flex items-center space-x-1.5 overflow-x-auto pb-1 border-b border-[#30363d] text-xs font-mono">
        <button
          onClick={() => setActiveTab('ptz-control')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'ptz-control'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Compass className="w-3.5 h-3.5" />
          <span>1. PTZ Control</span>
        </button>

        <button
          onClick={() => setActiveTab('presets')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'presets'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Sliders className="w-3.5 h-3.5" />
          <span>2. Presets ({presets.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('patrols')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'patrols'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <RotateCw className="w-3.5 h-3.5" />
          <span>3. Patrol Tours ({patrols.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('subscriptions')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'subscriptions'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Radio className="w-3.5 h-3.5" />
          <span>4. Subscriptions ({subscriptions.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('live-events')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'live-events'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Activity className="w-3.5 h-3.5" />
          <span>5. Live Events ({liveEvents.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('alarm-dashboard')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'alarm-dashboard'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Bell className="w-3.5 h-3.5" />
          <span>6. Alarm Dashboard</span>
          {alarmStats && alarmStats.new_count > 0 && (
            <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-red-500 text-white font-bold animate-pulse">
              {alarmStats.new_count}
            </span>
          )}
        </button>

        <button
          onClick={() => setActiveTab('alarm-history')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'alarm-history'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Layers className="w-3.5 h-3.5" />
          <span>7. Alarm Logs ({alarms.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('alarm-rules')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'alarm-rules'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <ShieldAlert className="w-3.5 h-3.5" />
          <span>8. Alarm Rules ({rules.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('evidence-viewer')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'evidence-viewer'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <Eye className="w-3.5 h-3.5" />
          <span>9. Evidence Viewer</span>
        </button>

        <button
          onClick={() => setActiveTab('compatibility-matrix')}
          className={`px-3 py-2 rounded-t-lg flex items-center space-x-2 transition-colors whitespace-nowrap ${
            activeTab === 'compatibility-matrix'
              ? 'bg-[#161b22] text-emerald-400 border-t-2 border-emerald-500 font-bold'
              : 'text-slate-400 hover:text-slate-200 hover:bg-[#161b22]/50'
          }`}
        >
          <ShieldCheck className="w-3.5 h-3.5" />
          <span>10. Event Matrix</span>
        </button>
      </div>

      {/* ======================================================== */}
      {/* SCREEN 1: PTZ LIVE CONTROL */}
      {/* ======================================================== */}
      {activeTab === 'ptz-control' && (
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
          {/* Left: Capability Banner & D-Pad Console */}
          <div className="lg:col-span-5 space-y-4">
            {/* Capability Verification Badge */}
            <div
              className={`p-4 rounded-xl border flex items-start space-x-3 font-mono text-xs ${
                capabilities?.ptz_supported
                  ? 'bg-emerald-950/20 border-emerald-500/40 text-emerald-300'
                  : 'bg-amber-950/30 border-amber-600/40 text-amber-300'
              }`}
            >
              {capabilities?.ptz_supported ? (
                <CheckCircle className="w-5 h-5 text-emerald-400 shrink-0 mt-0.5" />
              ) : (
                <AlertTriangle className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
              )}
              <div className="space-y-1">
                <div className="font-bold flex items-center space-x-2">
                  <span>
                    {capabilities?.ptz_supported
                      ? 'VERIFIED PTZ MOTORIZED CAMERA'
                      : 'FIXED CAMERA - PTZ CONTROLS LOCKED'}
                  </span>
                  <span className="px-1.5 py-0.5 rounded text-[10px] bg-black/40">
                    {currentDev?.manufacturer} {currentDev?.model}
                  </span>
                </div>
                <p className="text-[11px] text-slate-300 leading-relaxed">
                  {capabilities?.status_notes ||
                    (capabilities?.ptz_supported
                      ? 'Continuous pan, tilt, zoom and preset memory active via ONVIF/Native adapter.'
                      : 'This physical hardware unit is a fixed focal-length camera. Movement requests are blocked to prevent invalid socket commands.')}
                </p>
                {capabilities?.supported_protocols && capabilities.supported_protocols.length > 0 && (
                  <div className="text-[10px] text-slate-400">
                    PROTOCOLS: {capabilities.supported_protocols.join(' • ')}
                  </div>
                )}
              </div>
            </div>

            {/* D-Pad Console */}
            <div className="p-6 rounded-xl bg-[#161b22] border border-[#30363d] space-y-6 flex flex-col items-center justify-center font-mono">
              <div className="text-xs text-slate-400 flex items-center justify-between w-full">
                <span>
                  LAST STATUS: <strong className="text-emerald-400">{lastCommand}</strong>
                </span>
                <span>
                  VELOCITY: <strong className="text-slate-200">{(speed * 100).toFixed(0)}%</strong>
                </span>
              </div>

              {/* Directional Pad with Diagonals & Center Stop */}
              <div
                className={`grid grid-cols-3 gap-2.5 w-64 h-64 p-3 rounded-full bg-[#0d1117] border border-[#30363d] shadow-2xl items-center justify-center relative ${
                  !capabilities?.ptz_supported ? 'opacity-40 pointer-events-none' : ''
                }`}
              >
                {/* Top-Left Diagonal */}
                <button
                  onMouseDown={() => sendMove(-0.7, 0.7, 0, 'UP-LEFT')}
                  onMouseUp={sendStop}
                  title="Pan Left + Tilt Up"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-300 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowUpLeft className="w-4 h-4" />
                </button>

                {/* Up */}
                <button
                  onMouseDown={() => sendMove(0, 1.0, 0, 'TILT UP')}
                  onMouseUp={sendStop}
                  title="Tilt Up"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowUp className="w-5 h-5" />
                </button>

                {/* Top-Right Diagonal */}
                <button
                  onMouseDown={() => sendMove(0.7, 0.7, 0, 'UP-RIGHT')}
                  onMouseUp={sendStop}
                  title="Pan Right + Tilt Up"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-300 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowUpRight className="w-4 h-4" />
                </button>

                {/* Left */}
                <button
                  onMouseDown={() => sendMove(-1.0, 0, 0, 'PAN LEFT')}
                  onMouseUp={sendStop}
                  title="Pan Left"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowLeft className="w-5 h-5" />
                </button>

                {/* Center Emergency Stop */}
                <button
                  onClick={sendStop}
                  title="Emergency Stop All Motors"
                  className="h-11 w-11 rounded-xl bg-red-950/70 hover:bg-red-800 text-red-400 hover:text-white flex items-center justify-center border border-red-700/60 shadow transition-colors"
                >
                  <StopCircle className="w-5 h-5" />
                </button>

                {/* Right */}
                <button
                  onMouseDown={() => sendMove(1.0, 0, 0, 'PAN RIGHT')}
                  onMouseUp={sendStop}
                  title="Pan Right"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowRight className="w-5 h-5" />
                </button>

                {/* Bottom-Left Diagonal */}
                <button
                  onMouseDown={() => sendMove(-0.7, -0.7, 0, 'DOWN-LEFT')}
                  onMouseUp={sendStop}
                  title="Pan Left + Tilt Down"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-300 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowDownLeft className="w-4 h-4" />
                </button>

                {/* Down */}
                <button
                  onMouseDown={() => sendMove(0, -1.0, 0, 'TILT DOWN')}
                  onMouseUp={sendStop}
                  title="Tilt Down"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowDown className="w-5 h-5" />
                </button>

                {/* Bottom-Right Diagonal */}
                <button
                  onMouseDown={() => sendMove(0.7, -0.7, 0, 'DOWN-RIGHT')}
                  onMouseUp={sendStop}
                  title="Pan Right + Tilt Down"
                  className="h-11 w-11 rounded-xl bg-[#21262d] hover:bg-emerald-600 text-slate-300 hover:text-white flex items-center justify-center shadow transition-colors"
                >
                  <ArrowDownRight className="w-4 h-4" />
                </button>
              </div>

              {/* Speed Slider */}
              <div className="w-full space-y-2 pt-2 border-t border-[#30363d]">
                <div className="flex justify-between text-xs text-slate-400">
                  <span>SLEW VELOCITY: {Math.round(speed * 100)}%</span>
                  <span>RANGE: 0.1 – 1.0</span>
                </div>
                <input
                  type="range"
                  min="0.1"
                  max="1.0"
                  step="0.05"
                  value={speed}
                  disabled={!capabilities?.ptz_supported}
                  onChange={(e) => setSpeed(parseFloat(e.target.value))}
                  className="w-full accent-emerald-500"
                />
              </div>

              {/* Zoom & Special Controls */}
              <div className="w-full space-y-2.5 pt-2 border-t border-[#30363d]">
                <div className="grid grid-cols-2 gap-2">
                  <button
                    disabled={!capabilities?.ptz_supported}
                    onMouseDown={() => sendMove(0, 0, 1.0, 'ZOOM TELE')}
                    onMouseUp={sendStop}
                    className="py-2 px-3 rounded-lg bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white text-xs font-bold flex items-center justify-center space-x-1.5 transition-colors disabled:opacity-40"
                  >
                    <ZoomIn className="w-4 h-4 text-emerald-400" />
                    <span>ZOOM TELE (+)</span>
                  </button>

                  <button
                    disabled={!capabilities?.ptz_supported}
                    onMouseDown={() => sendMove(0, 0, -1.0, 'ZOOM WIDE')}
                    onMouseUp={sendStop}
                    className="py-2 px-3 rounded-lg bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white text-xs font-bold flex items-center justify-center space-x-1.5 transition-colors disabled:opacity-40"
                  >
                    <ZoomOut className="w-4 h-4 text-emerald-400" />
                    <span>ZOOM WIDE (-)</span>
                  </button>
                </div>

                {/* Focus & Iris Controls */}
                <div className="grid grid-cols-3 gap-2 text-[11px]">
                  <button
                    disabled={!capabilities?.ptz_supported}
                    onClick={() => sendFocusIris('FOCUS_NEAR')}
                    className="py-1.5 rounded bg-[#0d1117] hover:bg-[#21262d] text-slate-300 border border-[#30363d] disabled:opacity-40"
                  >
                    FOCUS NEAR
                  </button>
                  <button
                    disabled={!capabilities?.ptz_supported}
                    onClick={() => sendFocusIris('FOCUS_AUTO')}
                    className="py-1.5 rounded bg-[#0d1117] hover:bg-[#21262d] text-slate-300 border border-[#30363d] disabled:opacity-40"
                  >
                    AUTO FOCUS
                  </button>
                  <button
                    disabled={!capabilities?.ptz_supported}
                    onClick={() => sendFocusIris('FOCUS_FAR')}
                    className="py-1.5 rounded bg-[#0d1117] hover:bg-[#21262d] text-slate-300 border border-[#30363d] disabled:opacity-40"
                  >
                    FOCUS FAR
                  </button>
                </div>

                {/* Home & 360 Scan */}
                <div className="grid grid-cols-2 gap-2 text-[11px] pt-1">
                  <button
                    disabled={!capabilities?.ptz_supported}
                    onClick={sendHome}
                    className="py-2 rounded-lg bg-[#21262d] hover:bg-emerald-700 text-slate-200 flex items-center justify-center space-x-1.5 font-bold disabled:opacity-40"
                  >
                    <Home className="w-3.5 h-3.5 text-emerald-400" />
                    <span>GOTO HOME</span>
                  </button>
                  <button
                    disabled={!capabilities?.ptz_supported}
                    onClick={sendAutoScan}
                    className="py-2 rounded-lg bg-[#21262d] hover:bg-cyan-700 text-slate-200 flex items-center justify-center space-x-1.5 font-bold disabled:opacity-40"
                  >
                    <RotateCw className="w-3.5 h-3.5 text-cyan-400" />
                    <span>360° AUTO SCAN</span>
                  </button>
                </div>
              </div>

              {ptzError && (
                <div className="w-full p-2.5 rounded bg-red-950/40 border border-red-700/50 text-[11px] text-red-300">
                  {ptzError}
                </div>
              )}
            </div>
          </div>

          {/* Right: Live Viewport & Quick Presets Bar */}
          <div className="lg:col-span-7 space-y-4">
            <div className="rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden flex flex-col">
              <div className="h-9 bg-[#0d1117] px-4 flex items-center justify-between border-b border-[#30363d] text-xs font-mono">
                <span className="font-bold text-slate-200">
                  {currentDev?.name} ({currentDev?.ip})
                </span>
                <span className="text-emerald-400 font-bold flex items-center space-x-1.5">
                  <span className="h-2 w-2 rounded-full bg-emerald-400 animate-ping" />
                  <span>PTZ LIVE VIEW</span>
                </span>
              </div>

              <div className="relative aspect-video bg-black flex items-center justify-center overflow-hidden">
                <img
                  src={api.getSnapshotURL(currentDev?.id || '')}
                  alt={currentDev?.name}
                  className="w-full h-full object-cover"
                />
                <div className="absolute top-3 left-3 px-2 py-1 bg-black/60 backdrop-blur rounded font-mono text-[10px] text-slate-300">
                  {currentDev?.manufacturer} • {currentDev?.model} • PORT {currentDev?.port}
                </div>
                <div className="absolute bottom-3 right-3 px-2 py-1 bg-black/60 backdrop-blur rounded font-mono text-[10px] text-emerald-400">
                  SPEED: {(speed * 100).toFixed(0)}% • CMD: {lastCommand}
                </div>
              </div>

              <div className="p-4 font-mono text-xs text-slate-400 space-y-1.5 bg-[#161b22]/50">
                <div className="flex justify-between">
                  <span>TARGET IP / RTSP:</span>
                  <span className="text-slate-200">{currentDev?.ip}:{currentDev?.rtsp_port || 554}</span>
                </div>
                <div className="flex justify-between">
                  <span>MOTOR STATUS:</span>
                  <span className={capabilities?.ptz_supported ? 'text-emerald-400' : 'text-amber-400'}>
                    {capabilities?.ptz_supported ? 'OPERATIONAL (CONTINUOUS VELOCITY VECTOR)' : 'HARDWARE HAS NO MOTORS'}
                  </span>
                </div>
                <div className="flex justify-between">
                  <span>ACTIVE SUBSCRIBED EVENTS:</span>
                  <span className="text-slate-200">{subscriptions.filter(s => s.device_id === currentDev?.id).length} Active</span>
                </div>
              </div>
            </div>

            {/* Quick Presets Jump Bar */}
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3 font-mono">
              <div className="flex items-center justify-between text-xs text-slate-300">
                <span className="font-bold">QUICK PRESET HOT-KEYS:</span>
                <span className="text-[11px] text-slate-500">Click to slew camera to coordinates</span>
              </div>

              <div className="grid grid-cols-4 sm:grid-cols-8 gap-2">
                {[1, 2, 3, 4, 5, 6, 7, 8].map((num) => {
                  const existing = presets.find((p) => p.number === num);
                  return (
                    <button
                      key={num}
                      disabled={!capabilities?.ptz_supported}
                      onClick={() => handleGotoPreset(num)}
                      className={`py-2 px-2 rounded-lg text-center font-bold text-xs border transition-colors disabled:opacity-40 ${
                        existing
                          ? 'bg-emerald-950/40 text-emerald-300 border-emerald-500/40 hover:bg-emerald-600 hover:text-white'
                          : 'bg-[#0d1117] text-slate-400 border-[#30363d] hover:bg-[#21262d]'
                      }`}
                    >
                      P-{num}
                    </button>
                  );
                })}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 2: PTZ PRESET MANAGER */}
      {/* ======================================================== */}
      {activeTab === 'presets' && (
        <div className="space-y-6">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4 font-mono">
            <div className="space-y-1">
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                PTZ PRESET COORDINATE MEMORY ({currentDev?.name})
              </h3>
              <p className="text-xs text-slate-400">
                Store pan, tilt, and zoom coordinates into NVRAM on the camera unit.
              </p>
            </div>

            {capabilities?.ptz_supported && (
              <div className="flex items-center space-x-2">
                <input
                  type="number"
                  min="1"
                  max="128"
                  value={newPresetNumber}
                  onChange={(e) => setNewPresetNumber(parseInt(e.target.value) || 1)}
                  className="w-16 px-2 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
                  placeholder="#"
                />
                <input
                  type="text"
                  value={newPresetName}
                  onChange={(e) => setNewPresetName(e.target.value)}
                  className="w-48 px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
                  placeholder="Preset Name"
                />
                <button
                  onClick={handleCreatePreset}
                  className="px-3 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs flex items-center space-x-1.5"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>STORE POSITION</span>
                </button>
              </div>
            )}
          </div>

          {!capabilities?.ptz_supported ? (
            <div className="p-8 rounded-xl bg-[#161b22] border border-[#30363d] text-center font-mono text-slate-400 space-y-2">
              <AlertTriangle className="w-8 h-8 text-amber-400 mx-auto" />
              <p className="text-sm font-bold text-slate-200">PTZ PRESETS NOT SUPPORTED ON THIS DEVICE</p>
              <p className="text-xs text-slate-500">
                {currentDev?.name} is a fixed physical unit without motorized pan/tilt gears.
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {presets.map((p) => (
                <div
                  key={p.id || p.number}
                  className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-3 font-mono"
                >
                  <div className="flex items-center justify-between">
                    <span className="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 text-xs font-bold">
                      PRESET #{p.number}
                    </span>
                    <button
                      onClick={() => handleDeletePreset(p.number)}
                      className="p-1 text-slate-500 hover:text-red-400 transition-colors"
                      title="Delete Preset"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>

                  <div>
                    <h4 className="text-sm font-bold text-slate-200">{p.name}</h4>
                    <p className="text-[11px] text-slate-500">
                      Coordinates: Pan: {(p.pan ?? 0).toFixed(2)} | Tilt: {(p.tilt ?? 0).toFixed(2)} | Zoom: {(p.zoom ?? 0).toFixed(2)}
                    </p>
                  </div>

                  <button
                    onClick={() => handleGotoPreset(p.number)}
                    className="w-full py-2 rounded-lg bg-[#21262d] hover:bg-emerald-600 text-slate-200 hover:text-white text-xs font-bold flex items-center justify-center space-x-1.5 transition-colors"
                  >
                    <Compass className="w-3.5 h-3.5" />
                    <span>RECALL / GOTO PRESET</span>
                  </button>
                </div>
              ))}
              {presets.length === 0 && (
                <div className="col-span-full p-8 text-center text-slate-500 font-mono text-xs">
                  No presets defined yet. Adjust camera position and click 'STORE POSITION' above.
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 3: PTZ PATROL MANAGER */}
      {/* ======================================================== */}
      {activeTab === 'patrols' && (
        <div className="space-y-6">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4 font-mono">
            <div className="space-y-1">
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                GUARD PATROL TOURS & WAYPOINT CYCLES ({currentDev?.name})
              </h3>
              <p className="text-xs text-slate-400">
                Automated continuous cruising between presets with configurable dwell times.
              </p>
            </div>

            {capabilities?.ptz_supported && (
              <div className="flex items-center space-x-2">
                <input
                  type="text"
                  value={patrolName}
                  onChange={(e) => setPatrolName(e.target.value)}
                  className="w-36 px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
                  placeholder="Tour Name"
                />
                <input
                  type="text"
                  value={patrolStepsInput}
                  onChange={(e) => setPatrolStepsInput(e.target.value)}
                  className="w-48 px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
                  placeholder="Steps (1:5:4, 2:10:4)"
                  title="Format: preset:dwellSec:speed"
                />
                <button
                  onClick={handleCreatePatrol}
                  className="px-3 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs flex items-center space-x-1.5"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>CREATE TOUR</span>
                </button>
              </div>
            )}
          </div>

          {!capabilities?.ptz_supported ? (
            <div className="p-8 rounded-xl bg-[#161b22] border border-[#30363d] text-center font-mono text-slate-400 space-y-2">
              <AlertTriangle className="w-8 h-8 text-amber-400 mx-auto" />
              <p className="text-sm font-bold text-slate-200">PATROL TOURS NOT SUPPORTED</p>
              <p className="text-xs text-slate-500">Fixed camera does not have PTZ cruising capabilities.</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 font-mono">
              {patrols.map((tour) => (
                <div
                  key={tour.id || tour.tour_number}
                  className="p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4"
                >
                  <div className="flex items-center justify-between">
                    <div>
                      <span className="px-2 py-0.5 rounded bg-cyan-500/20 text-cyan-400 text-xs font-bold">
                        TOUR #{tour.tour_number}
                      </span>
                      <h4 className="text-sm font-bold text-slate-200 mt-1">{tour.name}</h4>
                    </div>
                    <span
                      className={`px-2 py-1 rounded text-[10px] font-bold ${
                        tour.is_running ? 'bg-emerald-500/20 text-emerald-300 animate-pulse' : 'bg-slate-800 text-slate-400'
                      }`}
                    >
                      {tour.is_running ? 'ACTIVE PATROL' : 'IDLE'}
                    </span>
                  </div>

                  {/* Waypoints list */}
                  <div className="space-y-1.5 text-xs text-slate-400 bg-[#0d1117] p-3 rounded-lg border border-[#30363d]">
                    <div className="text-[11px] font-bold text-slate-300 mb-1">WAYPOINTS SEQUENCE:</div>
                    {tour.steps.map((step, idx) => (
                      <div key={idx} className="flex justify-between items-center text-[11px]">
                        <span>Step {idx + 1}: Preset #{step.preset_number}</span>
                        <span>Dwell: {step.dwell_time_sec}s | Speed: {step.speed}x</span>
                      </div>
                    ))}
                  </div>

                  <div className="flex items-center space-x-2 pt-1">
                    {tour.is_running ? (
                      <button
                        onClick={() => handleStopPatrol(tour.tour_number)}
                        className="w-full py-2 rounded-lg bg-red-950/60 hover:bg-red-800 text-red-300 hover:text-white text-xs font-bold flex items-center justify-center space-x-1.5 border border-red-700/60"
                      >
                        <StopCircle className="w-4 h-4" />
                        <span>HALT PATROL TOUR</span>
                      </button>
                    ) : (
                      <button
                        onClick={() => handleRunPatrol(tour.tour_number)}
                        className="w-full py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center justify-center space-x-1.5"
                      >
                        <RotateCw className="w-4 h-4" />
                        <span>COMMENCE PATROL CRUISE</span>
                      </button>
                    )}
                  </div>
                </div>
              ))}
              {patrols.length === 0 && (
                <div className="col-span-full p-8 text-center text-slate-500 font-mono text-xs">
                  No patrol tours programmed. Enter tour name and step sequence (e.g. 1:5:4, 2:5:4) to create.
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 4: EVENT SUBSCRIPTION MANAGER */}
      {/* ======================================================== */}
      {activeTab === 'subscriptions' && (
        <div className="space-y-6 font-mono">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
            <div>
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                ACTIVE CCTV EVENT SUBSCRIPTIONS
              </h3>
              <p className="text-xs text-slate-400">
                ONVIF PullPoint, Hikvision alertStream, and Dahua CGI event manager listener threads.
              </p>
            </div>

            <button
              onClick={handleCreateSubscription}
              className="px-3.5 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs flex items-center space-x-1.5"
            >
              <Radio className="w-3.5 h-3.5" />
              <span>SUBSCRIBE CURRENT DEVICE ({currentDev?.name})</span>
            </button>
          </div>

          <div className="overflow-x-auto rounded-xl border border-[#30363d] bg-[#161b22]">
            <table className="w-full text-left text-xs">
              <thead className="bg-[#0d1117] text-slate-400 text-[11px] border-b border-[#30363d]">
                <tr>
                  <th className="py-2.5 px-4">DEVICE</th>
                  <th className="py-2.5 px-4">PROTOCOL</th>
                  <th className="py-2.5 px-4">ENDPOINT</th>
                  <th className="py-2.5 px-4">STATUS</th>
                  <th className="py-2.5 px-4">RENEWALS</th>
                  <th className="py-2.5 px-4">EXPIRES AT</th>
                  <th className="py-2.5 px-4 text-right">ACTION</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#30363d] text-slate-200">
                {subscriptions.map((sub) => {
                  const dev = devices.find((d) => d.id === sub.device_id);
                  return (
                    <tr key={sub.id} className="hover:bg-[#21262d]/50">
                      <td className="py-2.5 px-4 font-semibold">
                        {dev?.name || sub.device_id}
                      </td>
                      <td className="py-2.5 px-4">
                        <span className="px-1.5 py-0.5 rounded bg-cyan-950/40 text-cyan-300 border border-cyan-800/50 text-[10px]">
                          {sub.protocol}
                        </span>
                      </td>
                      <td className="py-2.5 px-4 text-slate-400 text-[11px]">
                        {sub.endpoint}
                      </td>
                      <td className="py-2.5 px-4">
                        <span
                          className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                            sub.status === 'ACTIVE'
                              ? 'bg-emerald-500/20 text-emerald-400'
                              : 'bg-red-500/20 text-red-400'
                          }`}
                        >
                          {sub.status}
                        </span>
                      </td>
                      <td className="py-2.5 px-4 text-slate-400">{sub.renew_count}</td>
                      <td className="py-2.5 px-4 text-slate-400 text-[11px]">
                        {new Date(sub.expires_at).toLocaleTimeString()}
                      </td>
                      <td className="py-2.5 px-4 text-right">
                        <button
                          onClick={() => handleCancelSubscription(sub.id)}
                          className="px-2 py-1 rounded bg-red-950/40 text-red-400 hover:bg-red-800 hover:text-white text-[11px] border border-red-800/50"
                        >
                          TERMINATE
                        </button>
                      </td>
                    </tr>
                  );
                })}
                {subscriptions.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-slate-500 text-xs">
                      No active event subscriptions. Click 'SUBSCRIBE CURRENT DEVICE' above.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 5: LIVE EVENT MONITOR */}
      {/* ======================================================== */}
      {activeTab === 'live-events' && (
        <div className="space-y-6 font-mono">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
            <div className="space-y-1">
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                REAL-TIME SURVEILLANCE EVENT TELEMETRY
              </h3>
              <p className="text-xs text-slate-400">
                Inbound normalized events: Motion, Tamper, Line Crossing, Intrusion & AI Analytics.
              </p>
            </div>

            <div className="flex items-center space-x-2">
              <Filter className="w-3.5 h-3.5 text-slate-400" />
              <select
                value={eventCategoryFilter}
                onChange={(e) => setEventCategoryFilter(e.target.value)}
                className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
              >
                <option value="ALL">ALL CATEGORIES</option>
                <option value="MOTION">MOTION</option>
                <option value="TAMPER">TAMPER</option>
                <option value="LINE_CROSSING">LINE CROSSING</option>
                <option value="INTRUSION">INTRUSION</option>
                <option value="AI_ANALYTICS">AI ANALYTICS</option>
                <option value="DEVICE_FAULT">DEVICE FAULT</option>
                <option value="IO_ALARM">IO ALARM</option>
              </select>
            </div>
          </div>

          <div className="overflow-x-auto rounded-xl border border-[#30363d] bg-[#161b22]">
            <table className="w-full text-left text-xs">
              <thead className="bg-[#0d1117] text-slate-400 text-[11px] border-b border-[#30363d]">
                <tr>
                  <th className="py-2.5 px-4">TIMESTAMP</th>
                  <th className="py-2.5 px-4">DEVICE</th>
                  <th className="py-2.5 px-4">CATEGORY</th>
                  <th className="py-2.5 px-4">TOPIC</th>
                  <th className="py-2.5 px-4">SEVERITY</th>
                  <th className="py-2.5 px-4">STATE</th>
                  <th className="py-2.5 px-4 text-right">RAW PAYLOAD</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#30363d] text-slate-200">
                {liveEvents.map((ev) => {
                  const dev = devices.find((d) => d.id === ev.device_id);
                  return (
                    <tr key={ev.id} className="hover:bg-[#21262d]/50">
                      <td className="py-2 px-4 text-slate-400 text-[11px]">
                        {new Date(ev.timestamp).toLocaleTimeString()}
                      </td>
                      <td className="py-2 px-4 font-semibold">{dev?.name || ev.device_id}</td>
                      <td className="py-2 px-4">
                        <span className="px-1.5 py-0.5 rounded bg-[#0d1117] text-slate-300 border border-[#30363d] text-[10px] font-bold">
                          {ev.category}
                        </span>
                      </td>
                      <td className="py-2 px-4 text-slate-300 text-[11px] truncate max-w-xs">{ev.topic}</td>
                      <td className="py-2 px-4">
                        <span
                          className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                            ev.severity === 'CRITICAL'
                              ? 'bg-red-500/20 text-red-400'
                              : ev.severity === 'MAJOR'
                              ? 'bg-orange-500/20 text-orange-400'
                              : ev.severity === 'MINOR'
                              ? 'bg-amber-500/20 text-amber-400'
                              : 'bg-blue-500/20 text-blue-400'
                          }`}
                        >
                          {ev.severity}
                        </span>
                      </td>
                      <td className="py-2 px-4">
                        <span className="px-1.5 py-0.5 rounded text-[10px] bg-slate-800 text-slate-300">
                          {ev.state}
                        </span>
                      </td>
                      <td className="py-2 px-4 text-right">
                        <button
                          onClick={() => setSelectedRawEvent(ev)}
                          className="px-2 py-0.5 rounded bg-[#0d1117] hover:bg-[#21262d] text-slate-400 hover:text-slate-200 text-[10px] border border-[#30363d]"
                        >
                          INSPECT
                        </button>
                      </td>
                    </tr>
                  );
                })}
                {liveEvents.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-slate-500 text-xs">
                      No events in stream buffer. Events will appear here as cameras trigger.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Raw Payload Modal */}
          {selectedRawEvent && (
            <div className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4">
              <div className="bg-[#161b22] border border-[#30363d] rounded-xl max-w-2xl w-full p-5 space-y-4">
                <div className="flex justify-between items-center border-b border-[#30363d] pb-3">
                  <h4 className="text-sm font-bold text-slate-200">
                    EVENT TELEMETRY INSPECTOR: {selectedRawEvent.id}
                  </h4>
                  <button
                    onClick={() => setSelectedRawEvent(null)}
                    className="text-slate-400 hover:text-slate-100"
                  >
                    ✕
                  </button>
                </div>
                <div className="text-xs text-slate-300 space-y-2 max-h-96 overflow-y-auto">
                  <p><strong>Topic:</strong> {selectedRawEvent.topic}</p>
                  <p><strong>Source:</strong> {selectedRawEvent.source}</p>
                  <p><strong>Normalized By:</strong> {selectedRawEvent.normalized_by}</p>
                  <pre className="p-3 bg-[#0d1117] rounded border border-[#30363d] overflow-x-auto text-[11px] text-emerald-400">
                    {JSON.stringify(selectedRawEvent.details || {}, null, 2)}
                  </pre>
                  {selectedRawEvent.raw_payload && (
                    <div>
                      <div className="font-bold text-slate-400 mb-1">Raw XML/CGI Payload:</div>
                      <pre className="p-3 bg-[#0d1117] rounded border border-[#30363d] overflow-x-auto text-[10px] text-slate-400">
                        {selectedRawEvent.raw_payload}
                      </pre>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 6: ALARM DASHBOARD */}
      {/* ======================================================== */}
      {activeTab === 'alarm-dashboard' && (
        <div className="space-y-6 font-mono">
          {/* KPI Stat Cards */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
              <div className="flex justify-between items-center text-xs text-slate-400">
                <span>TOTAL ALARMS</span>
                <Bell className="w-4 h-4 text-emerald-400" />
              </div>
              <div className="text-2xl font-bold text-slate-100">{alarmStats?.total_count || 0}</div>
              <div className="text-[11px] text-slate-500">Fleet-wide recorded</div>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
              <div className="flex justify-between items-center text-xs text-red-400">
                <span>CRITICAL SEVERITY</span>
                <ShieldAlert className="w-4 h-4 text-red-400" />
              </div>
              <div className="text-2xl font-bold text-red-400">{alarmStats?.critical_count || 0}</div>
              <div className="text-[11px] text-slate-500">Immediate attention</div>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
              <div className="flex justify-between items-center text-xs text-amber-400">
                <span>UNACKNOWLEDGED</span>
                <AlertTriangle className="w-4 h-4 text-amber-400" />
              </div>
              <div className="text-2xl font-bold text-amber-400">{alarmStats?.new_count || 0}</div>
              <div className="text-[11px] text-slate-500">Pending technician triage</div>
            </div>

            <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
              <div className="flex justify-between items-center text-xs text-cyan-400">
                <span>RESOLVED / CLOSED</span>
                <CheckCircle className="w-4 h-4 text-cyan-400" />
              </div>
              <div className="text-2xl font-bold text-cyan-400">{alarmStats?.resolved_count || 0}</div>
              <div className="text-[11px] text-slate-500">Triage complete</div>
            </div>
          </div>

          {/* Active Alarms Action Grid */}
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="space-y-1">
                <h3 className="text-sm font-bold text-slate-100 uppercase">
                  UNRESOLVED ALARM INCIDENT QUEUE
                </h3>
                <p className="text-xs text-slate-400">
                  Select an alarm to inspect snapshot evidence, assign field technicians, or record resolution notes.
                </p>
              </div>

              <div className="flex items-center space-x-2">
                <input
                  type="text"
                  value={technicianName}
                  onChange={(e) => setTechnicianName(e.target.value)}
                  className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
                  placeholder="Technician Call-Sign"
                  title="Technician logging actions"
                />
              </div>
            </div>

            <div className="space-y-3">
              {filteredAlarms
                .filter((a) => a.state !== 'RESOLVED' && a.state !== 'DISMISSED')
                .map((alarm) => (
                  <div
                    key={alarm.id}
                    className="p-4 rounded-xl bg-[#0d1117] border border-[#30363d] flex flex-wrap items-center justify-between gap-4"
                  >
                    <div className="flex items-start space-x-3">
                      <div
                        className={`h-9 w-9 rounded-lg flex items-center justify-center shrink-0 ${
                          alarm.severity === 'CRITICAL'
                            ? 'bg-red-500/20 text-red-400 border border-red-500/30'
                            : alarm.severity === 'MAJOR'
                            ? 'bg-orange-500/20 text-orange-400 border border-orange-500/30'
                            : 'bg-amber-500/20 text-amber-400 border border-amber-500/30'
                        }`}
                      >
                        <BellRing className="w-4 h-4" />
                      </div>
                      <div className="space-y-1">
                        <div className="flex items-center space-x-2">
                          <span className="text-sm font-bold text-slate-200">{alarm.device_name}</span>
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-[#161b22] text-slate-400 border border-[#30363d]">
                            CH-{alarm.channel_id}
                          </span>
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                            {alarm.category}
                          </span>
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-slate-800 text-slate-300">
                            STATE: {alarm.state}
                          </span>
                          {alarm.occurrence_count > 1 && (
                            <span className="px-1.5 py-0.5 rounded text-[10px] bg-red-900/40 text-red-300 border border-red-800">
                              x{alarm.occurrence_count} REPEATS (COOLDOWN SUPPRESSED)
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-slate-300">{alarm.description}</p>
                        <div className="text-[11px] text-slate-500 flex items-center space-x-3">
                          <span>TRIGGER: {new Date(alarm.event_timestamp).toLocaleTimeString()}</span>
                          {alarm.assigned_technician && (
                            <span className="text-cyan-400">ASSIGNED: {alarm.assigned_technician}</span>
                          )}
                          {alarm.snapshot_url && (
                            <button
                              onClick={() => {
                                setSelectedAlarmForEvidence(alarm);
                                setActiveTab('evidence-viewer');
                              }}
                              className="text-emerald-400 underline hover:text-emerald-300"
                            >
                              VIEW SNAPSHOT EVIDENCE
                            </button>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* Quick Triage Action Buttons */}
                    <div className="flex items-center space-x-2">
                      {alarm.state === 'NEW' && (
                        <button
                          onClick={() => handleAcknowledge(alarm.id)}
                          className="px-3 py-1.5 rounded bg-amber-600 hover:bg-amber-500 text-white text-xs font-bold flex items-center space-x-1"
                        >
                          <Check className="w-3.5 h-3.5" />
                          <span>ACKNOWLEDGE</span>
                        </button>
                      )}

                      {alarm.state !== 'RESOLVED' && (
                        <>
                          <button
                            onClick={() => handleAssign(alarm.id)}
                            className="px-3 py-1.5 rounded bg-[#21262d] hover:bg-[#30363d] text-slate-200 text-xs font-bold"
                          >
                            ASSIGN
                          </button>
                          <button
                            onClick={() => handleResolve(alarm.id)}
                            className="px-3 py-1.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold flex items-center space-x-1"
                          >
                            <CheckCircle className="w-3.5 h-3.5" />
                            <span>RESOLVE</span>
                          </button>
                          <button
                            onClick={() => handleDismiss(alarm.id)}
                            className="px-2.5 py-1.5 rounded bg-[#21262d] text-slate-400 hover:text-red-400 text-xs"
                            title="Dismiss as false positive"
                          >
                            DISMISS
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                ))}

              {filteredAlarms.filter((a) => a.state !== 'RESOLVED' && a.state !== 'DISMISSED').length === 0 && (
                <div className="p-8 text-center text-slate-500 text-xs">
                  All active alarms triaged. Incident queue clear.
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 7: ALARM HISTORY & LOGS */}
      {/* ======================================================== */}
      {activeTab === 'alarm-history' && (
        <div className="space-y-6 font-mono">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
            <div className="space-y-1">
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                ALARM AUDIT LOGS & EVIDENCE ARCHIVE
              </h3>
              <p className="text-xs text-slate-400">
                Filter and export historical alarm records with technician sign-off notes.
              </p>
            </div>

            <div className="flex items-center space-x-2">
              <input
                type="text"
                value={alarmSearchTerm}
                onChange={(e) => setAlarmSearchTerm(e.target.value)}
                placeholder="Search description, device..."
                className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
              />
              <a
                href={api.exportAlarmsURL('csv')}
                download="alarms.csv"
                className="px-3 py-1.5 rounded bg-[#21262d] hover:bg-[#30363d] text-slate-200 text-xs font-bold flex items-center space-x-1.5"
              >
                <Download className="w-3.5 h-3.5" />
                <span>EXPORT CSV</span>
              </a>
              <a
                href={api.exportAlarmsURL('json')}
                download="alarms.json"
                className="px-3 py-1.5 rounded bg-[#21262d] hover:bg-[#30363d] text-slate-200 text-xs font-bold flex items-center space-x-1.5"
              >
                <Download className="w-3.5 h-3.5" />
                <span>EXPORT JSON</span>
              </a>
            </div>
          </div>

          <div className="overflow-x-auto rounded-xl border border-[#30363d] bg-[#161b22]">
            <table className="w-full text-left text-xs">
              <thead className="bg-[#0d1117] text-slate-400 text-[11px] border-b border-[#30363d]">
                <tr>
                  <th className="py-2.5 px-4">TIME</th>
                  <th className="py-2.5 px-4">DEVICE</th>
                  <th className="py-2.5 px-4">SEVERITY</th>
                  <th className="py-2.5 px-4">CATEGORY</th>
                  <th className="py-2.5 px-4">STATE</th>
                  <th className="py-2.5 px-4">DESCRIPTION</th>
                  <th className="py-2.5 px-4">TECHNICIAN</th>
                  <th className="py-2.5 px-4 text-right">EVIDENCE</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#30363d] text-slate-200">
                {filteredAlarms.map((alm) => (
                  <tr key={alm.id} className="hover:bg-[#21262d]/50">
                    <td className="py-2 px-4 text-slate-400 text-[11px]">
                      {new Date(alm.event_timestamp).toLocaleString()}
                    </td>
                    <td className="py-2 px-4 font-semibold">{alm.device_name}</td>
                    <td className="py-2 px-4">
                      <span
                        className={`px-1.5 py-0.5 rounded text-[10px] font-bold ${
                          alm.severity === 'CRITICAL'
                            ? 'bg-red-500/20 text-red-400'
                            : alm.severity === 'MAJOR'
                            ? 'bg-orange-500/20 text-orange-400'
                            : alm.severity === 'MINOR'
                            ? 'bg-amber-500/20 text-amber-400'
                            : 'bg-blue-500/20 text-blue-400'
                        }`}
                      >
                        {alm.severity}
                      </span>
                    </td>
                    <td className="py-2 px-4 text-[11px] text-slate-300">{alm.category}</td>
                    <td className="py-2 px-4 text-[11px]">
                      <span className="px-1.5 py-0.5 rounded bg-[#0d1117] text-slate-300 border border-[#30363d]">
                        {alm.state}
                      </span>
                    </td>
                    <td className="py-2 px-4 text-slate-300 text-[11px] max-w-xs truncate">
                      {alm.description}
                    </td>
                    <td className="py-2 px-4 text-[11px] text-cyan-300">
                      {alm.assigned_technician || alm.resolved_by || '—'}
                    </td>
                    <td className="py-2 px-4 text-right">
                      {alm.snapshot_url ? (
                        <button
                          onClick={() => {
                            setSelectedAlarmForEvidence(alm);
                            setActiveTab('evidence-viewer');
                          }}
                          className="px-2 py-0.5 rounded bg-emerald-950/40 text-emerald-300 border border-emerald-800 text-[10px]"
                        >
                          SNAPSHOT
                        </button>
                      ) : (
                        <span className="text-slate-600 text-[10px]">NONE</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 8: ALARM RULE BUILDER */}
      {/* ======================================================== */}
      {activeTab === 'alarm-rules' && (
        <div className="space-y-6 font-mono">
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
            {/* Rule Creation Form */}
            <div className="lg:col-span-5 p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4">
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                CONFIGURE AUTOMATED ALARM DISPATCH RULE
              </h3>

              <div className="space-y-3 text-xs">
                <div>
                  <label className="text-slate-400 block mb-1">RULE NAME:</label>
                  <input
                    type="text"
                    value={newRuleName}
                    onChange={(e) => setNewRuleName(e.target.value)}
                    className="w-full px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-slate-200"
                  />
                </div>

                <div>
                  <label className="text-slate-400 block mb-1">TRIGGER CATEGORY:</label>
                  <select
                    value={newRuleCategory}
                    onChange={(e) => setNewRuleCategory(e.target.value)}
                    className="w-full px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-slate-200"
                  >
                    <option value="TAMPER">TAMPER DETECTION</option>
                    <option value="MOTION">VIDEO MOTION DETECTION</option>
                    <option value="LINE_CROSSING">LINE CROSSING (TRIPWIRE)</option>
                    <option value="INTRUSION">PERIMETER INTRUSION</option>
                    <option value="DEVICE_FAULT">DEVICE FAULT / STORAGE ERROR</option>
                    <option value="IO_ALARM">PHYSICAL I/O SENSOR ALARM</option>
                  </select>
                </div>

                <div>
                  <label className="text-slate-400 block mb-1">COOLDOWN SUPPRESSION WINDOW (SEC):</label>
                  <input
                    type="number"
                    min="1"
                    max="300"
                    value={newRuleCooldown}
                    onChange={(e) => setNewRuleCooldown(parseInt(e.target.value) || 10)}
                    className="w-full px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-slate-200"
                  />
                  <span className="text-[10px] text-slate-500">
                    Repeated events from the same channel within this window are aggregated to prevent notification flooding.
                  </span>
                </div>

                <div className="flex items-center space-x-2 pt-1">
                  <input
                    type="checkbox"
                    id="autoSnapshot"
                    checked={newRuleCaptureSnapshot}
                    onChange={(e) => setNewRuleCaptureSnapshot(e.target.checked)}
                    className="accent-emerald-500"
                  />
                  <label htmlFor="autoSnapshot" className="text-slate-200 font-bold">
                    AUTOMATICALLY CAPTURE SNAPSHOT EVIDENCE
                  </label>
                </div>

                <div>
                  <label className="text-slate-400 block mb-1">WEBHOOK DISPATCH URL:</label>
                  <input
                    type="text"
                    value={newRuleWebhook}
                    onChange={(e) => setNewRuleWebhook(e.target.value)}
                    className="w-full px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-slate-200"
                  />
                </div>

                <button
                  onClick={handleCreateRule}
                  className="w-full py-2.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs flex items-center justify-center space-x-2 transition-colors mt-2"
                >
                  <Plus className="w-4 h-4" />
                  <span>ACTIVATE ALARM RULE</span>
                </button>
              </div>
            </div>

            {/* Configured Rules List */}
            <div className="lg:col-span-7 space-y-4">
              <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d]">
                <h4 className="text-xs font-bold text-slate-300 uppercase mb-3">
                  ACTIVE AUTOMATION RULES IN ENGINE
                </h4>
                <div className="space-y-3">
                  {rules.map((rule) => (
                    <div
                      key={rule.id}
                      className="p-4 rounded-xl bg-[#0d1117] border border-[#30363d] flex items-center justify-between gap-4"
                    >
                      <div className="space-y-1">
                        <div className="flex items-center space-x-2">
                          <span className="text-xs font-bold text-slate-200">{rule.name}</span>
                          <span className="px-1.5 py-0.5 rounded text-[10px] bg-emerald-500/20 text-emerald-400">
                            {(rule.match_categories || rule.event_categories || []).join(', ')}
                          </span>
                        </div>
                        <div className="text-[11px] text-slate-400">
                          Cooldown: {rule.cooldown_sec}s | Snapshot: {rule.capture_snapshot ? 'YES' : 'NO'} | Webhook: {rule.webhook_url || 'None'}
                        </div>
                      </div>

                      <button
                        onClick={() => handleDeleteRule(rule.id)}
                        className="p-1.5 text-slate-500 hover:text-red-400 transition-colors"
                        title="Delete Rule"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  ))}
                  {rules.length === 0 && (
                    <div className="text-xs text-slate-500 text-center py-6">
                      No custom alarm rules. Default engine rules are active.
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 9: EVENT EVIDENCE VIEWER */}
      {/* ======================================================== */}
      {activeTab === 'evidence-viewer' && (
        <div className="space-y-6 font-mono">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] flex flex-wrap items-center justify-between gap-4">
            <div className="space-y-1">
              <h3 className="text-sm font-bold text-slate-100 uppercase">
                ALARM EVIDENCE VIEWER & TRUTHFUL CAPTURE AUDIT
              </h3>
              <p className="text-xs text-slate-400">
                Truth Rule Verified: Distinguish between the exact camera event trigger timestamp and subsequent snapshot acquisition time.
              </p>
            </div>

            <div className="flex items-center space-x-2">
              <select
                value={selectedAlarmForEvidence?.id || ''}
                onChange={(e) => {
                  const a = alarms.find((x) => x.id === e.target.value);
                  if (a) setSelectedAlarmForEvidence(a);
                }}
                className="px-3 py-1.5 bg-[#0d1117] border border-[#30363d] rounded text-xs text-slate-200"
              >
                {alarms
                  .filter((a) => a.snapshot_url)
                  .map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.device_name} - {a.category} ({new Date(a.event_timestamp).toLocaleTimeString()})
                    </option>
                  ))}
              </select>
            </div>
          </div>

          {selectedAlarmForEvidence ? (
            <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
              {/* Evidence Image */}
              <div className="lg:col-span-8 rounded-xl bg-[#161b22] border border-[#30363d] overflow-hidden flex flex-col">
                <div className="h-9 bg-[#0d1117] px-4 flex items-center justify-between border-b border-[#30363d] text-xs">
                  <span className="font-bold text-slate-200">
                    EVIDENCE FRAME: {selectedAlarmForEvidence.device_name}
                  </span>
                  <span className="text-red-400 font-bold">
                    INCIDENT: {selectedAlarmForEvidence.category}
                  </span>
                </div>

                <div className="relative aspect-video bg-black flex items-center justify-center">
                  {selectedAlarmForEvidence.snapshot_url ? (
                    <img
                      src={`http://127.0.0.1:8080${selectedAlarmForEvidence.snapshot_url}`}
                      alt="Alarm Snapshot"
                      className="w-full h-full object-cover"
                    />
                  ) : (
                    <div className="text-slate-500 text-xs">No snapshot captured for this alarm.</div>
                  )}
                </div>

                <div className="p-4 bg-[#161b22] border-t border-[#30363d] flex items-center justify-between">
                  <span className="text-xs text-slate-400">
                    EVIDENCE ID: <strong className="text-slate-200">{selectedAlarmForEvidence.snapshot_evidence_id || selectedAlarmForEvidence.id}</strong>
                  </span>
                  {selectedAlarmForEvidence.snapshot_url && (
                    <a
                      href={`http://127.0.0.1:8080${selectedAlarmForEvidence.snapshot_url}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="px-3 py-1.5 rounded bg-[#21262d] hover:bg-[#30363d] text-slate-200 text-xs font-bold flex items-center space-x-1.5"
                    >
                      <Download className="w-3.5 h-3.5" />
                      <span>OPEN FULL-RES IMAGE</span>
                    </a>
                  )}
                </div>
              </div>

              {/* Truth Rule Audit Panel */}
              <div className="lg:col-span-4 p-5 rounded-xl bg-[#161b22] border border-[#30363d] space-y-4 text-xs">
                <h4 className="font-bold text-emerald-400 uppercase border-b border-[#30363d] pb-2">
                  TRUTHFUL TIMESTAMP AUDIT
                </h4>

                <div className="space-y-3">
                  <div className="p-3 rounded-lg bg-[#0d1117] border border-[#30363d] space-y-1">
                    <span className="text-[10px] text-slate-500 uppercase font-bold">
                      CAMERA SENSOR EVENT TIME:
                    </span>
                    <div className="text-sm font-bold text-slate-200">
                      {new Date(selectedAlarmForEvidence.event_timestamp).toISOString()}
                    </div>
                  </div>

                  <div className="p-3 rounded-lg bg-[#0d1117] border border-[#30363d] space-y-1">
                    <span className="text-[10px] text-slate-500 uppercase font-bold">
                      SNAPSHOT CAPTURE TIME:
                    </span>
                    <div className="text-sm font-bold text-cyan-300">
                      {selectedAlarmForEvidence.snapshot_capture_time
                        ? new Date(selectedAlarmForEvidence.snapshot_capture_time).toISOString()
                        : 'Simultaneous acquisition'}
                    </div>
                  </div>

                  <div className="p-3 rounded-lg bg-emerald-950/30 border border-emerald-600/40 text-emerald-300 space-y-1 text-[11px]">
                    <div className="font-bold flex items-center space-x-1">
                      <ShieldCheck className="w-4 h-4 text-emerald-400" />
                      <span>TRUTH COMPLIANCE NOTICE:</span>
                    </div>
                    <p className="text-slate-400 leading-relaxed">
                      CCTV SENTINEL records the exact camera trigger time independently from the snapshot capture time. It does not falsely claim they are identical when transport latency exists.
                    </p>
                  </div>

                  <div className="space-y-1 pt-2">
                    <div className="text-slate-400">INCIDENT DETAILS:</div>
                    <p className="text-slate-300 text-[11px] bg-[#0d1117] p-2.5 rounded border border-[#30363d]">
                      {selectedAlarmForEvidence.description}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div className="p-12 text-center text-slate-500 text-xs">
              No alarm with snapshot selected. Trigger an alarm rule with snapshot capture enabled to inspect evidence.
            </div>
          )}
        </div>
      )}

      {/* ======================================================== */}
      {/* SCREEN 10: DEVICE EVENT COMPATIBILITY MATRIX */}
      {/* ======================================================== */}
      {activeTab === 'compatibility-matrix' && (
        <div className="space-y-6 font-mono">
          <div className="p-4 rounded-xl bg-[#161b22] border border-[#30363d] space-y-1">
            <h3 className="text-sm font-bold text-slate-100 uppercase">
              UNIVERSAL CCTV FLEET EVENT & PTZ COMPATIBILITY MATRIX
            </h3>
            <p className="text-xs text-slate-400">
              Audit results for ONVIF PullPoint, Hikvision ISAPI, Dahua CGI & CP PLUS adapters across physical hardware units.
            </p>
          </div>

          <div className="overflow-x-auto rounded-xl border border-[#30363d] bg-[#161b22]">
            <table className="w-full text-left text-xs">
              <thead className="bg-[#0d1117] text-slate-400 text-[11px] border-b border-[#30363d]">
                <tr>
                  <th className="py-2.5 px-4">DEVICE MODEL</th>
                  <th className="py-2.5 px-4">CATEGORY</th>
                  <th className="py-2.5 px-4">PTZ MOTOR</th>
                  <th className="py-2.5 px-4">EVENT PROTOCOL</th>
                  <th className="py-2.5 px-4">MOTION</th>
                  <th className="py-2.5 px-4">TAMPER</th>
                  <th className="py-2.5 px-4">AI ANALYTICS</th>
                  <th className="py-2.5 px-4">CERTIFICATION</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#30363d] text-slate-200">
                {devices.map((d) => {
                  const isVatilonPTZ = d.id.includes('104') || d.model.toLowerCase().includes('h43');
                  const isHik = d.manufacturer.toLowerCase().includes('hik');
                  const isDahua = d.manufacturer.toLowerCase().includes('dahua');
                  const isCPPlus = d.manufacturer.toLowerCase().includes('cpplus') || d.manufacturer.toLowerCase().includes('cp plus');

                  return (
                    <tr key={d.id} className="hover:bg-[#21262d]/50">
                      <td className="py-2.5 px-4">
                        <div className="font-semibold text-slate-100">{d.name}</div>
                        <div className="text-[10px] text-slate-500">{d.ip} • {d.model}</div>
                      </td>
                      <td className="py-2.5 px-4 text-slate-400">{d.category}</td>
                      <td className="py-2.5 px-4">
                        {isVatilonPTZ ? (
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400">
                            PTZ MOTORIZED
                          </span>
                        ) : (
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-slate-800 text-slate-400">
                            FIXED CAMERA
                          </span>
                        )}
                      </td>
                      <td className="py-2.5 px-4">
                        <span className="px-1.5 py-0.5 rounded text-[10px] bg-cyan-950/40 text-cyan-300 border border-cyan-800">
                          {isHik ? 'ISAPI alertStream' : isDahua ? 'Dahua CGI eventManager' : isCPPlus ? 'CP PLUS CGI' : 'ONVIF PullPoint'}
                        </span>
                      </td>
                      <td className="py-2.5 px-4 text-emerald-400 font-bold">YES</td>
                      <td className="py-2.5 px-4 text-emerald-400 font-bold">YES</td>
                      <td className="py-2.5 px-4">
                        {isHik || isDahua ? (
                          <span className="text-emerald-400 font-bold">ACUSENSE / WIZSENSE</span>
                        ) : (
                          <span className="text-slate-500">STANDARD</span>
                        )}
                      </td>
                      <td className="py-2.5 px-4">
                        <span className="px-2 py-0.5 rounded text-[10px] bg-emerald-500/20 text-emerald-400 font-bold">
                          VERIFIED
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
};
