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
  VaultStatus
} from '../types';

const BASE_URL = 'http://127.0.0.1:8080/api/v1';

async function request<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${endpoint}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });
  if (!res.ok) {
    let errMessage = 'Request failed';
    try {
      const errData = await res.json();
      errMessage = errData.error || errData.message || res.statusText;
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

  // Security, RBAC & Audit Integrity (Phase 5)
  login: (username: string, password: string) =>
    request<Session>('/security/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  getUsers: () => request<UserAccount[]>('/security/users'),
  createUser: (user: { username: string; password: string; role: string; site_id?: string }) =>
    request<UserAccount>('/security/users', { method: 'POST', body: JSON.stringify(user) }),
  getVaultStatus: () => request<VaultStatus>('/security/vault'),
  verifyAuditChain: () => request<AuditChainVerification>('/audit/verify'),
};
