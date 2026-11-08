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
  prev_hash?: string;
  entry_hash?: string;
  signature?: string;
  tenant_id?: string;
  site_id?: string;
}

export type BatchAction =
  | 'IDENTIFY'
  | 'HEALTH_CHECK'
  | 'INFO_RETRIEVAL'
  | 'NETWORK_CONFIG'
  | 'TIME_NTP_SYNC'
  | 'VIDEO_CONFIG'
  | 'IMAGE_CONFIG'
  | 'USER_SECURITY'
  | 'REBOOT'
  | 'CONFIG_COMPARE'
  | 'CONFIG_EXPORT_IMPORT';

export interface DeviceBatchPreview {
  device_id: string;
  device_name: string;
  ip: string;
  model: string;
  manufacturer: string;
  capable: boolean;
  missing_capabilities: string[];
  diff: string;
  status: string;
  warnings: string[];
}

export interface BatchPreview {
  preview_token: string;
  action: BatchAction;
  total_targets: number;
  eligible_count: number;
  ineligible_count: number;
  previews: DeviceBatchPreview[];
  reboot_required: boolean;
  expires_at: string;
  created_at: string;
}

export interface DeviceBatchResult {
  device_id: string;
  device_name: string;
  ip: string;
  status: 'PENDING' | 'IN_PROGRESS' | 'SUCCESS' | 'FAILED' | 'UNSUPPORTED' | 'SKIPPED' | 'CANCELLED';
  error?: string;
  details?: string;
  duration_ms: number;
  diff?: string;
  timestamp: string;
}

export interface BatchJob {
  id: string;
  name: string;
  action: BatchAction | string;
  target_ids: string[];
  payload?: any;
  status: 'PENDING' | 'RUNNING' | 'COMPLETED' | 'FAILED' | 'CANCELLED' | string;
  progress: number;
  logs: string[];
  created_at: string;
  preview_token?: string;
  concurrency?: number;
  rate_limit?: number;
  total?: number;
  completed?: number;
  succeeded?: number;
  failed?: number;
  skipped?: number;
  results?: DeviceBatchResult[];
  completed_at?: string;
}

export interface FirmwarePackage {
  id: string;
  model: string;
  manufacturer: string;
  hardware_revision: string;
  version: string;
  release_date: string;
  region: string;
  sha256_checksum: string;
  file_size_bytes: number;
  file_name: string;
  download_url: string;
  critical: boolean;
  release_notes: string;
}

export interface UpdateEligibility {
  eligible: boolean;
  reason: string;
  cross_flash_blocked: boolean;
  model_mismatch: boolean;
  hw_rev_mismatch: boolean;
  region_mismatch: boolean;
  current_version: string;
  target_version: string;
}

export interface FirmwareUpdatePreview {
  preview_token: string;
  device_id: string;
  device_name: string;
  firmware_id: string;
  model: string;
  manufacturer: string;
  current_firmware: string;
  target_firmware: string;
  sha256_checksum: string;
  eligible: boolean;
  ineligibility_reason: string;
  estimated_reboot_sec: number;
  expires_at: string;
  created_at: string;
}

export interface FirmwareUpdateJob {
  id: string;
  device_id: string;
  firmware_id: string;
  status: 'PREPARING' | 'UPLOADING' | 'FLASHING' | 'REBOOTING' | 'VERIFYING' | 'SUCCESS' | 'FAILED';
  progress_percent: number;
  stage: string;
  verified_new_firmware: string;
  error?: string;
  created_at: string;
  completed_at?: string;
}

export interface UserAccount {
  id: string;
  username: string;
  role: 'ADMIN' | 'TECHNICIAN' | 'VIEWER' | 'AUDITOR';
  site_id?: string;
  created_at: string;
}

export interface Session {
  token?: string;
  user_id?: string;
  username: string;
  role: 'ADMIN' | 'TECHNICIAN' | 'VIEWER' | 'AUDITOR' | string;
  tenant_id?: string;
  site_id?: string;
  is_global_admin?: boolean;
  scopes?: string[];
  expires_at: string;
}

export interface AuditChainVerification {
  valid: boolean;
  verified_count: number;
  broken_at_id?: string;
  error_reason?: string;
  checked_at: string;
}

export interface VaultStatus {
  sealed: boolean;
  keys_loaded: boolean;
  credential_count: number;
  mode: string;
  storage: string;
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

export interface StreamToken {
  token: string;
  device_id: string;
  stream_type: 'main' | 'sub';
  user: string;
  created_at: string;
  expires_at: string;
  active_viewers: number;
}

export interface StreamStats {
  status: 'CONNECTING' | 'LIVE' | 'RECONNECTING' | 'OFFLINE';
  device_id: string;
  stream_type: string;
  fps: number;
  bitrate_kbps: number;
  frame_count: number;
  bytes_received: number;
  last_frame_time: string;
  active_viewers: number;
  error_message?: string;
}

export interface StreamDiagnosticReport {
  device_id: string;
  stream_uri: string;
  rtsp_port: number;
  tcp_connected: boolean;
  connect_latency_ms: number;
  options_methods: string[];
  auth_required: boolean;
  auth_realm?: string;
  auth_method?: string;
  sdp_received: boolean;
  session_name?: string;
  video_codec: string;
  resolution: string;
  clock_rate: number;
  control_uri: string;
  audio_codec?: string;
  tcp_transport_supported: boolean;
  time_to_first_frame_ms: number;
  packets_received: number;
  measured_bitrate_kbps: number;
  packet_loss_status: string;
  snapshot_working: boolean;
  snapshot_latency_ms: number;
  recommendations: string[];
  run_at: string;
}

export interface SnapshotMetadata {
  id: string;
  device_id: string;
  timestamp: string;
  format: 'JPEG' | 'PNG';
  size_bytes: number;
  file_name: string;
  url: string;
}

export interface CapacityBandwidthCalculation {
  camera_count: number;
  resolution: string;
  codec: string;
  fps: number;
  main_bitrate_kbps: number;
  sub_bitrate_kbps: number;
  days_required: number;
  hours_per_day: number;
  simultaneous_viewers: number;
  aggregate_camera_mbps: number;
  viewing_bandwidth_mbps: number;
  total_network_mbps: number;
  recommended_headroom_mbps: number;
  recommended_switch_port: string;
  total_storage_gb: number;
  total_storage_tb: number;
  recommended_hdds: number;
  calculation_basis: string;
}

// Phase 6 - Recorder, Channel, Playback & Storage Types
export type RecorderType = 'DVR' | 'NVR' | 'XVR' | 'HVR' | 'HYBRID_RECORDER';
export type ChannelType = 'ANALOG' | 'IP';
export type AnalogFormat = 'CVI' | 'TVI' | 'AHD' | 'CVBS' | 'AUTO' | 'IP';
export type ChannelStatus = 'ONLINE' | 'OFFLINE' | 'NO_SIGNAL' | 'DISABLED';
export type SignalStatus = 'SIGNAL_LOCKED' | 'SIGNAL_LOST' | 'UNCONFIGURED';
export type RecordMode = 'CONTINUOUS' | 'MOTION' | 'ALARM' | 'MANUAL' | 'DISABLED';

export interface RecorderProfile {
  device_id: string;
  name: string;
  ip: string;
  port: number;
  manufacturer: string;
  model: string;
  hardware_revision: string;
  firmware_version: string;
  serial_number: string;
  recorder_type: RecorderType;
  physical_analog_channels: number;
  supported_ip_channels: number;
  total_channels: number;
  max_resolution: string;
  supported_analog_formats: AnalogFormat[];
  supported_codecs: string[];
  coaxial_control_supported: boolean;
  audio_over_coax_supported: boolean;
  ptz_over_coax_supported: boolean;
  ai_analytics_supported: boolean;
  disk_count: number;
  total_storage_gb: number;
  used_storage_gb: number;
  network_interfaces: string[];
}

export interface RecorderChannelInfo {
  recorder_id: string;
  channel_id: number;
  channel_number: number;
  name: string;
  channel_type: ChannelType;
  connected_camera_id?: string;
  connected_camera_ip?: string;
  connected_camera_model?: string;
  connection_status: ChannelStatus;
  signal_status: SignalStatus;
  signal_format: AnalogFormat;
  resolution: string;
  frame_rate: number;
  bitrate_kbps: number;
  encoding_format: string;
  recording_status: RecordMode;
  audio_enabled: boolean;
  ptz_supported: boolean;
  rtsp_stream_url?: string;
}

export interface AnalogFormatResult {
  channel_id: number;
  previous_format: AnalogFormat;
  current_format: AnalogFormat;
  switch_method: string;
  success: boolean;
  message: string;
  timestamp: string;
}

export interface IPRegistrationReq {
  channel_id: number;
  ip: string;
  port: number;
  protocol: string;
  username: string;
  password: string;
  stream_index: number;
}

export interface RecordingSchedule {
  recorder_id: string;
  channel_id: number;
  channel_name: string;
  pre_record_sec: number;
  post_record_sec: number;
  weekly_schedule: Record<number, string[]>;
  holiday_enabled: boolean;
}

export interface RecordingSegment {
  id: string;
  recorder_id: string;
  channel_id: number;
  channel_name: string;
  start_time: string;
  end_time: string;
  duration_sec: number;
  event_type: 'CONTINUOUS' | 'MOTION' | 'ALARM' | 'MANUAL' | string;
  size_bytes: number;
  file_format: string;
  playback_uri: string;
}

export interface PlaybackSession {
  session_id: string;
  playback_token: string;
  recorder_id: string;
  channel_id: number;
  start_time: string;
  end_time: string;
  current_position: string;
  speed: number;
  state: 'PLAYING' | 'PAUSED' | 'STOPPED';
  stream_url: string;
  expires_at: string;
}

export interface StorageDisk {
  disk_index: number;
  name: string;
  model: string;
  serial: string;
  capacity_gb: number;
  used_gb: number;
  free_gb: number;
  usage_percent: number;
  status: 'NORMAL' | 'ERROR' | 'UNFORMATTED' | 'REBUILDING' | string;
  smart_status: 'HEALTHY' | 'WARNING' | 'FAILED' | string;
  temperature_c: number;
  bad_sectors: number;
  overwrite_mode: 'OVERWRITE' | 'STOP_RECORDING' | string;
}

export interface RetentionEstimate {
  channel_count: number;
  avg_bitrate_kbps: number;
  daily_recording_hours: number;
  daily_storage_gb: number;
  total_retention_days: number;
}

export interface StorageHealthReport {
  recorder_id: string;
  total_disks: number;
  total_capacity_gb: number;
  total_used_gb: number;
  total_free_gb: number;
  disks: StorageDisk[];
  overall_health: 'GOOD' | 'WARNING' | 'CRITICAL' | string;
  recording_interruption_warning: boolean;
  estimated_retention_days: number;
  measured_retention_basis: string;
  retention_estimate: RetentionEstimate;
}

export interface DiskFormatPreview {
  format_token: string;
  recorder_id: string;
  disk_index: number;
  model: string;
  capacity_gb: number;
  warning: string;
  expires_at: string;
}

// Phase 7 - PTZ Control, Events & Unified Alarm Monitoring
export interface PTZCapabilities {
  device_id: string;
  ptz_supported: boolean;
  pan_tilt_supported: boolean;
  continuous_pan_tilt: boolean;
  relative_pan_tilt: boolean;
  absolute_pan_tilt: boolean;
  zoom_supported: boolean;
  continuous_zoom: boolean;
  relative_zoom: boolean;
  focus_supported: boolean;
  iris_supported: boolean;
  presets_supported: boolean;
  max_presets: number;
  preset_tour_supported: boolean;
  home_position_supported: boolean;
  auto_scan_supported: boolean;
  supported_protocols: string[];
  status_notes: string;
}

export interface Preset {
  id: string;
  device_id: string;
  number: number;
  name: string;
  pan: number;
  tilt: number;
  zoom: number;
  created_at: string;
}

export interface PatrolStep {
  preset_number: number;
  dwell_time_sec: number;
  speed: number;
}

export interface PatrolTour {
  id: string;
  device_id: string;
  tour_number: number;
  name: string;
  steps: PatrolStep[];
  repeat_count: number;
  is_running: boolean;
}

export interface MoveRequest {
  pan_speed: number;
  tilt_speed: number;
  zoom_speed: number;
  timeout_ms?: number;
}

export interface FocusIrisRequest {
  action: 'FOCUS_NEAR' | 'FOCUS_FAR' | 'FOCUS_AUTO' | 'IRIS_OPEN' | 'IRIS_CLOSE' | 'IRIS_RESET';
  speed?: number;
}

export type EventCategory =
  | 'MOTION'
  | 'TAMPER'
  | 'LINE_CROSSING'
  | 'INTRUSION'
  | 'AI_ANALYTICS'
  | 'DEVICE_FAULT'
  | 'IO_ALARM'
  | 'SYSTEM';

export interface EventSubscription {
  id: string;
  device_id: string;
  protocol: string;
  endpoint: string;
  status: 'ACTIVE' | 'CONNECTING' | 'RETRYING' | 'TERMINATED';
  subscribed_at: string;
  expires_at: string;
  renew_count: number;
  last_error?: string;
}

export interface SurveillanceEvent {
  id: string;
  device_id: string;
  channel_id: number;
  timestamp: string;
  category: EventCategory;
  topic: string;
  source: string;
  state: 'START' | 'STOP' | 'PULSE';
  severity: 'INFO' | 'MINOR' | 'MAJOR' | 'CRITICAL';
  details?: Record<string, any>;
  raw_payload?: string;
  normalized_by: string;
}

export type AlarmSeverity = 'CRITICAL' | 'MAJOR' | 'MINOR' | 'INFO';
export type AlarmState = 'NEW' | 'ACKNOWLEDGED' | 'INVESTIGATING' | 'RESOLVED' | 'DISMISSED';

export interface Alarm {
  id: string;
  device_id: string;
  device_name: string;
  channel_id: number;
  category: string;
  severity: AlarmSeverity;
  state: AlarmState;
  event_id: string;
  event_topic: string;
  event_timestamp: string;
  first_seen_at: string;
  last_seen_at: string;
  occurrence_count: number;
  description: string;
  assigned_technician?: string;
  resolution_notes?: string;
  acknowledged_at?: string;
  acknowledged_by?: string;
  resolved_at?: string;
  resolved_by?: string;
  snapshot_url?: string;
  snapshot_capture_time?: string;
  snapshot_evidence_id?: string;
  metadata?: Record<string, any>;
}

export interface AlarmRule {
  id: string;
  name: string;
  enabled: boolean;
  match_categories?: string[];
  event_categories?: string[];
  match_device_ids?: string[];
  device_ids?: string[];
  severity_override?: AlarmSeverity;
  severity?: AlarmSeverity;
  cooldown_sec: number;
  capture_snapshot: boolean;
  assign_technician?: string;
  webhook_url?: string;
  email_recipients?: string[];
  suppress_duplicates?: boolean;
}

export interface AlarmStats {
  total_count: number;
  new_count: number;
  acknowledged_count: number;
  investigating_count: number;
  resolved_count: number;
  critical_count: number;
  major_count: number;
  minor_count: number;
  info_count: number;
  by_category: Record<string, number>;
  by_device: Record<string, number>;
}

