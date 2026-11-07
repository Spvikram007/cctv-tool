import {
  Device, NetworkConfig, ImageConfig, VideoEncodingConfig,
  RecorderChannel, DiagnosticsReport, SubnetCalc, HDDCalc,
  BandwidthCalc, License, AuditLogEntry, ConfigPreview,
  CompatibilityEntry, BatchJob, NetworkInterface, DiscoveryJob,
  DiscoveredDevice, DeviceCapabilities, AuthStatus, ConfigResult
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

  // Batch
  getBatchJobs: () => request<BatchJob[]>('/batch'),
  createBatchJob: (job: Partial<BatchJob>) => request<BatchJob>('/batch', { method: 'POST', body: JSON.stringify(job) }),
};
