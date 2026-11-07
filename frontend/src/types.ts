export type DeviceCategory = 'IP_CAMERA' | 'ANALOG_CAMERA' | 'DVR' | 'NVR' | 'XVR' | 'HVR' | 'HYBRID_RECORDER' | 'PTZ_CAMERA' | 'ENCODER' | 'NETWORK_VIDEO_SERVER' | 'UNKNOWN';

export type Manufacturer = 'Hikvision' | 'Dahua' | 'CP PLUS' | 'Uniview' | 'Axis' | 'Hanwha' | 'Generic ONVIF' | 'OEM Rebranded';

export type ProtocolType = 'ONVIF' | 'RTSP' | 'ISAPI' | 'DAHUA_CGI' | 'AXIS_VAPIX' | 'HTTP' | 'HTTPS';

export interface NetworkInterface {
  index: number;
  name: string;
  hardware_addr: string;
  ipv4_addresses: string[];
  ipv6_addresses: string[];
  subnets: string[];
  is_up: boolean;
  is_loopback: boolean;
  supports_multicast: boolean;
}

export interface EvidenceItem {
  category: string;
  description: string;
  weight: number;
  value: string;
}

export interface DeviceServices {
  open_ports: number[];
  http_server?: string;
  http_auth_header?: string;
  rtsp_server?: string;
  rtsp_public_opts?: string;
  ssl_cert_issuer?: string;
  ssl_cert_subject?: string;
  onvif_service_url?: string;
}

export interface FingerprintResult {
  ip: string;
  mac: string;
  identified_manufacturer: Manufacturer;
  probable_manufacturer: string;
  model: string;
  firmware: string;
  serial_number: string;
  category: DeviceCategory;
  confidence: number;
  confidence_level: 'HIGH' | 'MEDIUM' | 'LOW';
  evidence_sources: EvidenceItem[];
  unknown_fields: string[];
  services: DeviceServices;
  protocols: ProtocolType[];
  capabilities: string[];
  channel_count: number;
  discovered_via: string;
}

export interface DiscoveredDevice {
  device: Device;
  fingerprint: FingerprintResult;
}

export interface DiscoveryJob {
  id: string;
  method: string;
  status: 'RUNNING' | 'COMPLETED' | 'CANCELLED' | 'FAILED';
  interface?: string;
  cidr?: string;
  progress_percent: number;
  scanned_targets: number;
  total_targets: number;
  found_count: number;
  results: DiscoveredDevice[];
  created_at: string;
  completed_at?: string;
  error?: string;
}

export interface Device {
  id: string;
  name: string;
  ip: string;
  mac: string;
  manufacturer: Manufacturer;
  model: string;
  firmware: string;
  serial_number: string;
  category: DeviceCategory;
  port: number;
  rtsp_port: number;
  onvif_port: number;
  status: string;
  protocols: ProtocolType[];
  capabilities: string[];
  channel_count: number;
  last_seen: string;
  site_id?: string;
}

export interface NetworkConfig {
  device_id: string;
  dhcp: boolean;
  ip_address: string;
  subnet_mask: string;
  gateway: string;
  dns1: string;
  dns2: string;
  http_port: number;
  rtsp_port: number;
  onvif_port: number;
  ntp_enabled: boolean;
  ntp_server: string;
}

export interface ImageConfig {
  device_id: string;
  brightness: number;
  contrast: number;
  saturation: number;
  sharpness: number;
  wdr_mode: string;
  wdr_level: number;
  ir_mode: string;
  backlight_comp: boolean;
}

export interface VideoEncodingConfig {
  device_id: string;
  stream_index: number;
  stream_name: string;
  codec: string;
  resolution: string;
  frame_rate: number;
  bitrate_kbps: number;
  rate_control: string;
  rtsp_url: string;
}

export interface RecorderChannel {
  channel_id: number;
  name: string;
  type: string;
  signal_format: string;
  status: string;
  resolution: string;
  record_mode: string;
  audio_enabled: boolean;
  ptz_supported: boolean;
}

export interface DiagnosticsReport {
  target_ip: string;
  ping_success: boolean;
  latency_ms: number;
  packet_loss_pct: number;
  open_ports: number[];
  rtsp_working: boolean;
  rtsp_response: string;
  onvif_working: boolean;
  onvif_version: string;
  http_status: number;
  ssl_valid: boolean;
  recommendations: string[];
}

export interface SubnetCalc {
  cidr: string;
  network_address: string;
  broadcast_address: string;
  netmask: string;
  wildcard_mask: string;
  first_usable_ip: string;
  last_usable_ip: string;
  total_hosts: number;
  usable_hosts: number;
}

export interface HDDCalc {
  camera_count: number;
  resolution: string;
  codec: string;
  bitrate_kbps: number;
  days_required: number;
  hours_per_day: number;
  total_storage_gb: number;
  total_storage_tb: number;
  recommended_hdds: number;
}

export interface BandwidthCalc {
  camera_count: number;
  main_bitrate_kbps: number;
  sub_bitrate_kbps: number;
  total_main_mbps: number;
  total_sub_mbps: number;
  total_combined_mbps: number;
  recommended_switch_port: string;
}

export interface License {
  tier: string;
  license_key: string;
  owner_name: string;
  company: string;
  gst_number: string;
  max_devices: number;
  max_sites: number;
  issued_at: string;
  expires_at: string;
  active: boolean;
  machine_hash: string;
}

export interface AuditLogEntry {
  id: string;
  timestamp: string;
  user: string;
  device_id: string;
  device_ip: string;
  action: string;
  category: string;
  status: string;
  details: string;
  diff?: string;
}

export interface ConfigPreview {
  preview_token: string;
  device_id: string;
  device_name: string;
  config_type: string;
  current_config: any;
  proposed_config: any;
  diff: string;
  valid: boolean;
  validation_errors: string[];
  rollback_key: string;
  expires_at?: string;
}

export type CapabilityStatus = 'SUPPORTED' | 'UNSUPPORTED' | 'UNKNOWN';

export interface DeviceCapabilities {
  device_id: string;
  device_management: CapabilityStatus;
  media: CapabilityStatus;
  media2: CapabilityStatus;
  ptz: CapabilityStatus;
  imaging: CapabilityStatus;
  events: CapabilityStatus;
  recording: CapabilityStatus;
  analog_format: CapabilityStatus;
  profiles?: string[];
  details?: Record<string, string>;
}

export interface AuthStatus {
  device_id: string;
  authenticated: boolean;
  username: string;
  protocol: string;
  auth_method: string;
  clock_skew_sec: number;
  firmware: string;
  model: string;
  last_checked: string;
  error_message?: string;
}

export interface ConfigResult {
  success: boolean;
  device_id: string;
  config_type: string;
  message: string;
  applied_at: string;
  read_back_match: boolean;
  read_back_diff?: string;
  verified_read?: any;
  audit_entry_id: string;
  rollback_available: boolean;
  rollback_key?: string;
}

export interface CompatibilityEntry {
  manufacturer: Manufacturer;
  model: string;
  category: string;
  protocols: string[];
  ptz: boolean;
  audio: boolean;
  analog_format?: string;
  tested_fw: string;
  notes: string;
  verified: boolean;
}

export interface BatchJob {
  id: string;
  name: string;
  target_ids: string[];
  action: string;
  payload: any;
  status: string;
  progress: number;
  logs: string[];
  created_at: string;
}
