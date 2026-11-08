import {
  Device, NetworkConfig, ImageConfig, VideoEncodingConfig,
  RecorderChannel, DiagnosticsReport, SubnetCalc, HDDCalc,
  BandwidthCalc, License, AuditLogEntry, ConfigPreview,
  CompatibilityEntry, BatchJob, NetworkInterface, DiscoveryJob,
  DiscoveredDevice, DeviceCapabilities, AuthStatus, ConfigResult,
  StreamToken, StreamStats, StreamDiagnosticReport, SnapshotMetadata,
  CapacityBandwidthCalculation, BatchAction, BatchPreview,
  FirmwarePackage, UpdateEligibility, FirmwareUpdatePreview,
  FirmwareUpdateJob, UserAccount, Session, AuditChainVerification,
  VaultStatus, RecorderProfile, RecorderChannelInfo, AnalogFormatResult,
  IPRegistrationReq, RecordingSchedule, RecordingSegment, PlaybackSession,
  StorageHealthReport, DiskFormatPreview,
  PTZCapabilities, Preset, PatrolTour, MoveRequest, FocusIrisRequest,
  EventSubscription, SurveillanceEvent, Alarm, AlarmRule, AlarmStats
} from '../types';

const BASE_URL = 'http://127.0.0.1:8080/api/v1';

// In-memory CSRF token storage - NEVER exposed or stored in localStorage / sessionStorage
let memoryCsrfToken = '';

function getCsrfToken(): string {
  if (memoryCsrfToken) {
    return memoryCsrfToken;
  }
  if (typeof document !== 'undefined' && document.cookie) {
    const match = document.cookie.match(/(?:^|;\s*)sentinel_csrf=([^;]+)/);
    if (match && match[1]) {
      memoryCsrfToken = decodeURIComponent(match[1]);
      return memoryCsrfToken;
    }
  }
  return '';
}

async function request<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const method = (options?.method || 'GET').toUpperCase();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  };

  // CSRF Protection: Attach anti-CSRF token to all state-changing requests
  if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
    const csrf = getCsrfToken();
    if (csrf && !headers['X-CSRF-Token']) {
      headers['X-CSRF-Token'] = csrf;
    }
  }

  // Pure cookie-based browser session: credentials: 'include' automatically transmits HttpOnly sentinel_session
  const res = await fetch(`${BASE_URL}${endpoint}`, {
    ...options,
    credentials: 'include',
    headers,
  });
  if (!res.ok) {
    let errMessage = 'Request failed';
    try {
      const errData = await res.json();
      errMessage = (typeof errData.error === 'object' && errData.error?.message) 
        ? errData.error.message 
        : (errData.error || errData.message || res.statusText);
    } catch {
      errMessage = await res.text();
    }
    throw new Error(errMessage);
  }
  return res.json();
}

export const api = {
  // System
  getStatus: () => request<any>('/system/status'),

  // Devices
  getDevices: () => request<Device[]>('/devices'),
  addDevice: (dev: Partial<Device>) => request<Device>('/devices', { method: 'POST', body: JSON.stringify(dev) }),
  deleteDevice: (id: string) => request<any>(`/devices/${id}`, { method: 'DELETE' }),

  // Discovery
  getInterfaces: () => request<NetworkInterface[]>('/discovery/interfaces'),
  startDiscovery: (opts: {
    method?: string;
    interface?: string;
    cidr?: string;
    ports?: number[];
    timeout_ms?: number;
    concurrency?: number;
    username?: string;
    password?: string;
  }) => request<any>('/devices/discover', { method: 'POST', body: JSON.stringify(opts) }),
  getDiscoveryJob: (id: string) => request<DiscoveryJob>(`/discovery/${id}`),
  getDiscoveryResults: (id: string) =>
    request<{ job_id: string; status: string; found_count: number; results: DiscoveredDevice[] }>(`/discovery/${id}/results`),
  cancelDiscovery: (id: string) => request<any>(`/discovery/${id}/cancel`, { method: 'POST' }),
  getDiscoveryProgress: () => request<any>('/devices/discover/progress'),

  // Device Identity & Services
  getDeviceIdentity: (id: string) => request<any>(`/devices/${id}/identity`),
  getDeviceServices: (id: string) => request<any>(`/devices/${id}/services`),
  authenticateDevice: (id: string, credentials?: { username?: string; password?: string }) =>
    request<AuthStatus>(`/devices/${id}/auth`, { method: 'POST', body: JSON.stringify(credentials || {}) }),
  getCapabilities: (id: string) => request<DeviceCapabilities>(`/devices/${id}/capabilities`),

  // Unified Config Pipeline (Preview -> Approve -> Apply -> Read-back -> Rollback)
  previewConfig: (id: string, type: 'NETWORK' | 'IMAGE' | string, config: any) =>
    request<ConfigPreview>(`/devices/${id}/config/preview`, { method: 'POST', body: JSON.stringify({ type, config }) }),
  applyConfig: (id: string, type: 'NETWORK' | 'IMAGE' | string, previewToken: string, rollbackKey: string) =>
    request<ConfigResult>(`/devices/${id}/config/apply`, { method: 'POST', body: JSON.stringify({ type, preview_token: previewToken, rollback_key: rollbackKey }) }),
  rollbackConfig: (id: string, rollbackKey: string) =>
    request<ConfigResult>(`/devices/${id}/config/rollback`, { method: 'POST', body: JSON.stringify({ rollback_key: rollbackKey }) }),

  // Device Configs
  getNetworkConfig: (id: string) => request<NetworkConfig>(`/devices/${id}/network`),
  previewNetworkConfig: (id: string, config: NetworkConfig) =>
    request<ConfigPreview>(`/devices/${id}/network/preview`, { method: 'POST', body: JSON.stringify(config) }),
  applyNetworkConfig: (id: string, rollbackKey: string, config: NetworkConfig) =>
    request<any>(`/devices/${id}/network/apply`, { method: 'POST', body: JSON.stringify({ rollback_key: rollbackKey, config }) }),

  getImageConfig: (id: string) => request<ImageConfig>(`/devices/${id}/image`),
  previewImageConfig: (id: string, config: ImageConfig) =>
    request<ConfigPreview>(`/devices/${id}/image/preview`, { method: 'POST', body: JSON.stringify(config) }),
  applyImageConfig: (id: string, rollbackKey: string, config: ImageConfig) =>
    request<any>(`/devices/${id}/image/apply`, { method: 'POST', body: JSON.stringify({ rollback_key: rollbackKey, config }) }),

  getVideoConfigs: (id: string) => request<VideoEncodingConfig[]>(`/devices/${id}/encoding`),

  sendPTZ: (id: string, action: { command: string; pan?: number; tilt?: number; zoom?: number; speed?: number; preset_id?: number }) =>
    request<any>(`/devices/${id}/ptz`, { method: 'POST', body: JSON.stringify(action) }),

  // Streaming & Live Feeds (Phase 4 Authenticated Stream Hub)
  getStreamToken: (deviceId: string, streamType: 'main' | 'sub' = 'main', user = 'Technician', ttlSeconds = 3600) =>
    request<StreamToken>(`/devices/${deviceId}/stream/token`, {
      method: 'POST',
      body: JSON.stringify({ stream_type: streamType, user, ttl_seconds: ttlSeconds })
    }),
  getStreamMJPEGUrl: (token: string) => `${BASE_URL}/streams/${token}/mjpeg`,
  getStreamStats: (token: string) => request<StreamStats>(`/streams/${token}/stats`),
  getStreamDiagnostics: (deviceId: string) => request<StreamDiagnosticReport>(`/devices/${deviceId}/diagnostics/stream`),

  // Genuine Snapshot Engine
  captureSnapshot: (deviceId: string) => request<SnapshotMetadata>(`/devices/${deviceId}/snapshot`, { method: 'POST' }),
  getSnapshotHistory: (deviceId: string) => request<SnapshotMetadata[]>(`/devices/${deviceId}/snapshots`),
  getSnapshotImageURL: (deviceId: string, fileName: string) => `${BASE_URL}/devices/${deviceId}/snapshots/${encodeURIComponent(fileName)}`,
  getDirectSnapshotURL: (deviceId: string) => `${BASE_URL}/devices/${deviceId}/snapshot?t=${Date.now()}`,
  getSnapshotURL: (id: string) => `${BASE_URL}/devices/${id}/snapshot?t=${Date.now()}`,

  // Channels & Analog XVR
  getChannels: (id: string) => request<RecorderChannel[]>(`/devices/${id}/channels`),
  setAnalogFormat: (id: string, channelID: number, signalFormat: string) =>
    request<any>(`/devices/${id}/channels/analog-format`, { method: 'POST', body: JSON.stringify({ channel_id: channelID, signal_format: signalFormat }) }),

  // Diagnostics & Calculators
  runDiagnostics: (ip: string, rtspPort = 554, httpPort = 80) =>
    request<DiagnosticsReport>(`/diagnostics/run?ip=${encodeURIComponent(ip)}&rtsp_port=${rtspPort}&http_port=${httpPort}`),
  calcSubnet: (cidr: string) => request<SubnetCalc>(`/diagnostics/subnet?cidr=${encodeURIComponent(cidr)}`),
  calcHDD: (cameras: number, resolution: string, codec: string, days: number, hours = 24) =>
    request<HDDCalc>(`/diagnostics/hdd?cameras=${cameras}&resolution=${resolution}&codec=${codec}&days=${days}&hours=${hours}`),
  calcBandwidth: (cameras: number, mainKbps: number, subKbps: number) =>
    request<BandwidthCalc>(`/diagnostics/bandwidth?cameras=${cameras}&main_kbps=${mainKbps}&sub_kbps=${subKbps}`),
  calcCapacity: (params: {
    cameras?: number;
    resolution?: string;
    codec?: string;
    fps?: number;
    days?: number;
    hours?: number;
    viewers?: number;
    main_kbps?: number;
    sub_kbps?: number;
  }) => {
    const q = new URLSearchParams();
    if (params.cameras) q.set('cameras', params.cameras.toString());
    if (params.resolution) q.set('resolution', params.resolution);
    if (params.codec) q.set('codec', params.codec);
    if (params.fps) q.set('fps', params.fps.toString());
    if (params.days) q.set('days', params.days.toString());
    if (params.hours) q.set('hours', params.hours.toString());
    if (params.viewers) q.set('viewers', params.viewers.toString());
    if (params.main_kbps) q.set('main_kbps', params.main_kbps.toString());
    if (params.sub_kbps) q.set('sub_kbps', params.sub_kbps.toString());
    return request<CapacityBandwidthCalculation>(`/diagnostics/capacity?${q.toString()}`);
  },

  // Licensing
  getLicensing: () => request<{ license: License; machine_hash: string }>('/licensing'),
  activateLicense: (key: string, company: string, gst: string) =>
    request<any>('/licensing/activate', { method: 'POST', body: JSON.stringify({ key, company, gst }) }),
  generateGSTInvoice: (customerName: string, customerGST: string, state: string, tier: string) =>
    request<any>('/licensing/gst-invoice', { method: 'POST', body: JSON.stringify({ customer_name: customerName, customer_gst: customerGST, state, tier }) }),

  // Audit Logs & Reports
  getAuditLogs: () => request<AuditLogEntry[]>('/audit/logs'),
  getCompatibility: () => request<CompatibilityEntry[]>('/compatibility'),
  getTechnicianReport: () => request<any>('/reports/technician'),

  // Batch Operations Engine (Phase 5)
  getBatchJobs: () => request<BatchJob[]>('/batch/jobs'),
  getBatchJob: (id: string) => request<BatchJob>(`/batch/jobs/${id}`),
  createBatchJob: (job: Partial<BatchJob>) => request<BatchJob>('/batch', { method: 'POST', body: JSON.stringify(job) }),
  previewBatch: (action: BatchAction | string, targetIds: string[], payload?: any) =>
    request<BatchPreview>('/batch/preview', { method: 'POST', body: JSON.stringify({ action, target_ids: targetIds, payload }) }),
  executeBatch: (job: { name: string; action: BatchAction | string; target_ids: string[]; payload?: any; preview_token?: string; concurrency?: number; rate_limit?: number }) =>
    request<BatchJob>('/batch/execute', { method: 'POST', body: JSON.stringify(job) }),
  cancelBatchJob: (id: string) => request<any>(`/batch/jobs/${id}/cancel`, { method: 'POST' }),
  compareBatchConfigs: (targetIds: string[], configType: 'NETWORK' | 'IMAGE' | 'GENERAL' = 'NETWORK') =>
    request<{ count: number; config_type: string; devices: Record<string, any>; deviations: Record<string, any> }>('/batch/compare', { method: 'POST', body: JSON.stringify({ target_ids: targetIds, config_type: configType }) }),
  exportBatchConfigs: (targetIds: string[]) =>
    request<{ version: string; exported_at: string; device_count: number; configurations: any[] }>('/batch/export', { method: 'POST', body: JSON.stringify({ target_ids: targetIds }) }),
  importBatchConfigs: (backupData: any) =>
    request<{ total: number; restored: number; errors: string[] }>('/batch/import', { method: 'POST', body: JSON.stringify({ backup_data: backupData }) }),

  // Firmware Management Framework (Phase 5)
  getFirmwareInventory: () => request<FirmwarePackage[]>('/firmware/inventory'),
  checkFirmwareCompatibility: (deviceId: string, firmwareId: string) =>
    request<UpdateEligibility>(`/firmware/compatibility?device_id=${encodeURIComponent(deviceId)}&firmware_id=${encodeURIComponent(firmwareId)}`),
  stageFirmwarePreview: (deviceId: string, firmwareId: string) =>
    request<FirmwareUpdatePreview>('/firmware/stage', { method: 'POST', body: JSON.stringify({ device_id: deviceId, firmware_id: firmwareId }) }),
  approveFirmwareUpdate: (previewToken: string, operatorConfirmation: boolean) =>
    request<FirmwareUpdateJob>('/firmware/approve', { method: 'POST', body: JSON.stringify({ preview_token: previewToken, operator_confirmation: operatorConfirmation }) }),
  getFirmwareJob: (id: string) => request<FirmwareUpdateJob>(`/firmware/jobs/${id}`),

  // Security, RBAC & Audit Integrity (Phase 5 & 6.1)
  getSetupStatus: () => request<{ setup_required: boolean; users_count: number }>('/auth/setup/status'),
  completeSetup: (payload: { setup_token: string; username: string; password: string; full_name?: string; email?: string }) =>
    request<{ success: boolean; message: string; username: string }>('/auth/setup', { method: 'POST', body: JSON.stringify(payload) }),
  login: async (username: string, password: string) => {
    const res = await request<{ success: boolean; session: Session; csrf_token?: string; permissions: string[] }>('/security/login', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    });
    if (res && res.csrf_token) {
      memoryCsrfToken = res.csrf_token;
    }
    return res;
  },
  logout: async () => {
    try {
      await request<any>('/security/logout', { method: 'POST' });
    } catch {
      // ignore network errors during session termination
    }
    memoryCsrfToken = '';
  },
  getCSRFToken: () => request<{ csrf_token: string }>('/security/csrf'),
  migrateDevice: (deviceId: string, targetTenant: string, targetSite: string) =>
    request<{ success: boolean; device_id: string; target_tenant: string; target_site: string; message: string }>(
      `/devices/${deviceId}/migrate`,
      {
        method: 'POST',
        body: JSON.stringify({ target_tenant: targetTenant, target_site: targetSite })
      }
    ),
  getUsers: () => request<UserAccount[]>('/security/users'),
  createUser: (user: { username: string; password: string; role: string; site_id?: string }) =>
    request<UserAccount>('/security/users', { method: 'POST', body: JSON.stringify(user) }),
  getVaultStatus: () => request<VaultStatus>('/security/vault'),
  verifyAuditChain: () => request<AuditChainVerification>('/audit/verify'),
  rotateAuditKey: () => request<{ success: boolean; new_key_id: string }>('/audit/rotate-key', { method: 'POST' }),
  getSignedAuditExportUrl: () => `${BASE_URL}/audit/export/signed`,

  // DVR / NVR / XVR Channel, Recording & Playback (Phase 6)
  getRecorders: () => request<Device[]>('/recorders'),
  getRecorderProfile: (id: string) => request<RecorderProfile>(`/recorders/${id}/profile`),
  getRecorderChannels: (id: string) => request<RecorderChannelInfo[]>(`/recorders/${id}/channels`),
  setRecorderAnalogFormat: (id: string, channelId: number, format: string) =>
    request<AnalogFormatResult>(`/recorders/${id}/analog-format`, {
      method: 'POST',
      body: JSON.stringify({ channel_id: channelId, format })
    }),
  sendRecorderCoaxialControl: (id: string, channelId: number, command: string) =>
    request<{ channel_id: number; command: string; success: boolean; message: string }>(`/recorders/${id}/coaxial-control`, {
      method: 'POST',
      body: JSON.stringify({ channel_id: channelId, command })
    }),
  registerRecorderIPCamera: (id: string, payload: IPRegistrationReq) =>
    request<{ channel_id: number; status: string; message: string }>(`/recorders/${id}/ip-registration`, {
      method: 'POST',
      body: JSON.stringify(payload)
    }),
  getRecorderSchedule: (id: string, channelId: number) =>
    request<RecordingSchedule>(`/recorders/${id}/schedule?channel_id=${channelId}`),
  updateRecorderSchedule: (id: string, schedule: RecordingSchedule) =>
    request<RecordingSchedule>(`/recorders/${id}/schedule`, {
      method: 'PUT',
      body: JSON.stringify(schedule)
    }),
  searchRecordings: (id: string, channelId: number, startTime?: string, endTime?: string, eventType?: string) => {
    const q = new URLSearchParams({ channel_id: channelId.toString() });
    if (startTime) q.set('start_time', startTime);
    if (endTime) q.set('end_time', endTime);
    if (eventType && eventType !== 'ALL') q.set('event_type', eventType);
    return request<RecordingSegment[]>(`/recorders/${id}/search-recordings?${q.toString()}`);
  },
  createPlaybackSession: (id: string, channelId: number, startTime: string, endTime: string, speed: number = 1.0) =>
    request<PlaybackSession>(`/recorders/${id}/playback-session`, {
      method: 'POST',
      body: JSON.stringify({ channel_id: channelId, start_time: startTime, end_time: endTime, speed })
    }),
  exportRecording: (id: string, segmentId: string, outputPath: string) =>
    request<{ success: boolean; message: string; output_path: string; size_bytes: number }>(`/recorders/${id}/export`, {
      method: 'POST',
      body: JSON.stringify({ segment_id: segmentId, output_path: outputPath })
    }),
  getRecorderStorage: (id: string) => request<StorageHealthReport>(`/recorders/${id}/storage`),
  stageFormatDiskPreview: (id: string, diskIndex: number) =>
    request<DiskFormatPreview>(`/recorders/${id}/storage/format-preview`, {
      method: 'POST',
      body: JSON.stringify({ disk_index: diskIndex })
    }),
  approveFormatDisk: (id: string, formatToken: string, confirmation: string) =>
    request<{ success: boolean; message: string }>(`/recorders/${id}/storage/format-approve`, {
      method: 'POST',
      body: JSON.stringify({ format_token: formatToken, confirmation })
    }),

  // PTZ Control Engine (Phase 7)
  getPTZCapabilities: (deviceId: string) => request<PTZCapabilities>(`/devices/${deviceId}/ptz/capabilities`),
  ptzMove: (deviceId: string, req: MoveRequest) =>
    request<{ status: string; command: string }>(`/devices/${deviceId}/ptz/move`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  ptzStop: (deviceId: string) =>
    request<{ status: string; stopped: boolean }>(`/devices/${deviceId}/ptz/stop`, {
      method: 'POST',
    }),
  ptzFocusIris: (deviceId: string, req: FocusIrisRequest) =>
    request<{ status: string; action: string }>(`/devices/${deviceId}/ptz/focus-iris`, {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  getPTZPresets: (deviceId: string) => request<Preset[]>(`/devices/${deviceId}/ptz/presets`),
  createPTZPreset: (deviceId: string, number: number, name: string) =>
    request<Preset>(`/devices/${deviceId}/ptz/presets`, {
      method: 'POST',
      body: JSON.stringify({ number, name }),
    }),
  gotoPTZPreset: (deviceId: string, number: number) =>
    request<{ status: string; preset_number: number }>(`/devices/${deviceId}/ptz/presets/${number}/goto`, {
      method: 'POST',
    }),
  renamePTZPreset: (deviceId: string, number: number, name: string) =>
    request<Preset>(`/devices/${deviceId}/ptz/presets/${number}`, {
      method: 'PUT',
      body: JSON.stringify({ name }),
    }),
  deletePTZPreset: (deviceId: string, number: number) =>
    request<{ status: string; deleted: number }>(`/devices/${deviceId}/ptz/presets/${number}`, {
      method: 'DELETE',
    }),
  getPTZPatrols: (deviceId: string) => request<PatrolTour[]>(`/devices/${deviceId}/ptz/patrols`),
  createPTZPatrol: (deviceId: string, tour: Partial<PatrolTour>) =>
    request<PatrolTour>(`/devices/${deviceId}/ptz/patrols`, {
      method: 'POST',
      body: JSON.stringify(tour),
    }),
  runPTZPatrol: (deviceId: string, tourNumber: number) =>
    request<{ status: string; tour_number: number }>(`/devices/${deviceId}/ptz/patrols/${tourNumber}/run`, {
      method: 'POST',
    }),
  stopPTZPatrol: (deviceId: string, tourNumber: number) =>
    request<{ status: string; tour_number: number }>(`/devices/${deviceId}/ptz/patrols/${tourNumber}/stop`, {
      method: 'POST',
    }),
  ptzHome: (deviceId: string) =>
    request<{ status: string; action: string }>(`/devices/${deviceId}/ptz/home`, {
      method: 'POST',
    }),
  ptzAutoScan: (deviceId: string, speed: number = 0.5) =>
    request<{ status: string; action: string }>(`/devices/${deviceId}/ptz/auto-scan`, {
      method: 'POST',
      body: JSON.stringify({ speed }),
    }),

  // Events & Subscriptions (Phase 7)
  getEventSubscriptions: () => request<EventSubscription[]>('/events/subscriptions'),
  createEventSubscription: (deviceId: string, protocol?: string) =>
    request<EventSubscription>('/events/subscriptions', {
      method: 'POST',
      body: JSON.stringify({ device_id: deviceId, protocol: protocol || 'ONVIF' }),
    }),
  cancelEventSubscription: (subId: string) =>
    request<{ status: string; subscription_id: string }>(`/events/subscriptions/${subId}`, {
      method: 'DELETE',
    }),
  getLiveEvents: (limit: number = 50, category?: string) => {
    const q = new URLSearchParams({ limit: limit.toString() });
    if (category) q.set('category', category);
    return request<SurveillanceEvent[]>(`/events/live?${q.toString()}`);
  },

  // Alarms & Rules (Phase 7)
  getAlarms: (filters?: { device_id?: string; category?: string; severity?: string; state?: string; limit?: number }) => {
    const q = new URLSearchParams();
    if (filters?.device_id) q.set('device_id', filters.device_id);
    if (filters?.category) q.set('category', filters.category);
    if (filters?.severity) q.set('severity', filters.severity);
    if (filters?.state) q.set('state', filters.state);
    if (filters?.limit) q.set('limit', filters.limit.toString());
    return request<Alarm[]>(`/alarms?${q.toString()}`);
  },
  getAlarmStats: () => request<AlarmStats>('/alarms/stats'),
  acknowledgeAlarm: (alarmId: string, technician: string) =>
    request<Alarm>(`/alarms/${alarmId}/acknowledge`, {
      method: 'POST',
      body: JSON.stringify({ technician }),
    }),
  assignAlarm: (alarmId: string, technician: string) =>
    request<Alarm>(`/alarms/${alarmId}/assign`, {
      method: 'POST',
      body: JSON.stringify({ technician }),
    }),
  resolveAlarm: (alarmId: string, resolutionNotes: string, technician: string) =>
    request<Alarm>(`/alarms/${alarmId}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ resolution_notes: resolutionNotes, technician }),
    }),
  dismissAlarm: (alarmId: string, reason: string) =>
    request<Alarm>(`/alarms/${alarmId}/dismiss`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }),
  getAlarmRules: () => request<AlarmRule[]>('/alarms/rules'),
  createAlarmRule: (rule: Partial<AlarmRule>) =>
    request<AlarmRule>('/alarms/rules', {
      method: 'POST',
      body: JSON.stringify(rule),
    }),
  deleteAlarmRule: (ruleId: string) =>
    request<{ status: string; rule_id: string }>(`/alarms/rules/${ruleId}`, {
      method: 'DELETE',
    }),
  exportAlarmsURL: (format: 'csv' | 'json' = 'csv') => `${BASE_URL}/alarms/export?format=${format}`,
  getAlarmsStreamURL: () => `${BASE_URL}/alarms/stream`,
};
