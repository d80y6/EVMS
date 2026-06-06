const API_BASE = '/api';

let csrfToken: string | null = null;

export function setCSRFToken(token: string) {
  csrfToken = token;
}

export function getCSRFToken(): string | null {
  return csrfToken;
}

export async function fetchCSRFToken(): Promise<string> {
  try {
    const res = await fetch(`${API_BASE}/csrf-token`, { credentials: 'include' });
    if (res.ok) {
      const data = await res.json();
      csrfToken = data.csrf_token;
      return data.csrf_token;
    }
  } catch {
    // Ignore - will retry
  }
  return '';
}

export function authUrl(path: string): string {
  const token = localStorage.getItem('auth_token');
  if (!token) return path;
  const sep = path.includes('?') ? '&' : '?';
  return `${path}${sep}token=${encodeURIComponent(token)}`;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('auth_token');
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const method = (options.method || 'GET').toUpperCase();
  if (csrfToken && (method === 'POST' || method === 'PUT' || method === 'DELETE')) {
    headers['X-CSRF-Token'] = csrfToken;
  }

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers, credentials: 'include' });

  if (res.status === 401) {
    localStorage.removeItem('auth_token');
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }

  if (!res.ok) {
    const body = await res.text();
    throw new Error(body || `Request failed: ${res.status}`);
  }

  return res.json();
}

export type StreamType = 'main' | 'sub' | 'thumbnail';

export interface Camera {
  id: string;
  site_id: string;
  name: string;
  description: string;
  connection_url: string;
  substream_url: string;
  status: string;
  ptz_protocol: string;
  retention_days: number;
  onvif_username: string;
  config?: string;
}

export interface Recording {
  camera_id: string;
  start_time: string;
  end_time: string;
  file_path: string;
  file_size: number;
}

export interface AIEvent {
  id: string;
  camera_id: string;
  object_type: string;
  confidence: number;
  event_time: string;
}

export interface Preset {
  id: number;
  name: string;
}

export interface TourStep {
  camera_id: string;
  preset_token?: string;
  dwell_seconds: number;
}

export interface Tour {
  id: string;
  name: string;
  enabled: boolean;
  steps: TourStep[];
  interval: number;
  created_at: string;
}

export interface POSItem {
  sku: string;
  description: string;
  quantity: number;
  unit_price: number;
  total: number;
}

export interface POSTransaction {
  id: string;
  camera_id: string;
  store_id: string;
  register_id: string;
  transaction_id: string;
  timestamp: string;
  items: POSItem[];
  subtotal: number;
  tax: number;
  total: number;
  tender_type: string;
}

export interface ScanRecord {
  id: string;
  site_id: string;
  status: 'pending' | 'running' | 'completed' | 'cancelled' | 'failed';
  methods: string[];
  subnets: string[];
  ports: number[];
  total_found: number;
  error?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface ResultRecord {
  id: string;
  scan_id: string;
  site_id: string;
  ip_address: string;
  port?: number;
  xaddr?: string;
  manufacturer?: string;
  model?: string;
  firmware?: string;
  serial_number?: string;
  hostname?: string;
  capabilities: Record<string, boolean>;
  is_new: boolean;
  already_in_db: boolean;
  imported: boolean;
  created_at: string;
}

export interface Bookmark {
  id: string;
  camera_id: string;
  timestamp: string;
  label: string;
  created_at: string;
  created_by: string;
}

export interface LegalHold {
  id: string;
  camera_id: string;
  reason: string;
  created_by: string;
  created_at: string;
  released_at: string | null;
}

export interface LoginResponse {
  token: string;
  refresh_token?: string;
  mfa_required?: boolean;
  mfa_token?: string;
}

export interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  scopes: string;
  active: boolean;
  last_used_at?: string;
  expires_at?: string;
  created_at: string;
}

export const apiClient = {
  fetch: (path: string, options?: RequestInit) => {
    const token = localStorage.getItem('auth_token');
    const headers: Record<string, string> = {
      ...(options?.headers as Record<string, string>),
    };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    return fetch(path, { ...options, headers });
  },
};

export const api = {
  login: async (username: string, password: string) => {
    await fetchCSRFToken();
    return request<LoginResponse>('/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  },

  getCameras: () =>
    request<{ cameras: Camera[] }>('/cameras'),

  listCameras: (siteId?: string) => {
    const q = siteId ? `?site_id=${siteId}` : '';
    return request<{ cameras: Camera[] }>(`/cameras${q}`).then((res) => res.cameras);
  },

  getCamera: (id: string) =>
    request<Camera>(`/cameras/${id}`),

  updateCameraConfig: (cameraId: string, config: Record<string, unknown>) =>
    request<{ status: string }>(`/cameras/${cameraId}/config`, {
      method: 'PUT',
      body: JSON.stringify({ config }),
    }),

  getRecordings: () =>
    request<{ recordings: Recording[] }>('/recordings'),

  getEvents: () =>
    request<{ events: AIEvent[] }>('/events'),

  getPlaybackUrl: (path: string) =>
    authUrl(`${API_BASE}/playback/${path}`),

  ptzMove: (cameraId: string, direction: string, speed: number) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/move`, {
      method: 'POST',
      body: JSON.stringify({ direction, speed }),
    }),

  ptzStop: (cameraId: string) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/stop`, {
      method: 'POST',
    }),

  ptzZoom: (cameraId: string, zoom: number) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/zoom`, {
      method: 'POST',
      body: JSON.stringify({ zoom }),
    }),

  ptzGetPresets: (cameraId: string) =>
    request<{ presets: Preset[] }>(`/cameras/${cameraId}/ptz/presets`),

  ptzGotoPreset: (cameraId: string, presetId: string) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/presets/${presetId}/goto`, {
      method: 'POST',
    }),

  getTimeline: (cameraId: string, start: string, end: string, interval?: number) => {
    const params = new URLSearchParams({ camera_id: cameraId, start, end });
    if (interval) params.set('interval', String(interval));
    return request<{ thumbnails: { timestamp: string; url: string }[] }>(`/thumbnails/timeline?${params}`);
  },

  getThumbnailUrl: (path: string) =>
    authUrl(`${API_BASE}${path}`),

  getUsers: () =>
    request<{ users: { id: string; username: string; role: string; active: boolean; created_at: string }[] }>('/admin/users'),

  createUser: (username: string, password: string, role: string) =>
    request<{ id: string; status: string }>('/admin/users', {
      method: 'POST',
      body: JSON.stringify({ username, password, role }),
    }),

  updateUser: (id: string, data: { role?: string; password?: string }) =>
    request<{ status: string }>(`/admin/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteUser: (id: string) =>
    request<{ status: string }>(`/admin/users/${id}`, {
      method: 'DELETE',
    }),

  getSites: () =>
    request<{ sites: { id: string; name: string; location: string }[] }>('/sites'),

  createSite: (name: string, location: string) =>
    request<{ id: string; name: string; location: string }>('/sites', {
      method: 'POST',
      body: JSON.stringify({ name, location }),
    }),

  smartSearch: (params: { camera_id?: string; object_type?: string; min_confidence?: number; start_time?: string; end_time?: string; limit?: number; bounding_box?: string; metadata?: string }) => {
    const q = new URLSearchParams();
    if (params.camera_id) q.set('camera_id', params.camera_id);
    if (params.object_type) q.set('object_type', params.object_type);
    if (params.min_confidence !== undefined) q.set('min_confidence', String(params.min_confidence));
    if (params.start_time) q.set('start_time', params.start_time);
    if (params.end_time) q.set('end_time', params.end_time);
    if (params.limit) q.set('limit', String(params.limit));
    if (params.bounding_box) q.set('bounding_box', params.bounding_box);
    if (params.metadata) q.set('metadata', params.metadata);
    return request<{ results: { id: string; camera_id: string; event_time: string; object_type: string; confidence: number; track_id: string; thumbnail: string }[]; total: number }>(`/search?${q}`);
  },

  ptzHome: (cameraId: string) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/home`, {
      method: 'POST',
    }),

  ptzSetHome: (cameraId: string) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/set-home`, {
      method: 'POST',
    }),

  ptzSetPreset: (cameraId: string, presetId: number, presetName?: string) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/presets`, {
      method: 'POST',
      body: JSON.stringify({ preset_id: presetId, preset_name: presetName }),
    }),

  ptzRemovePreset: (cameraId: string, presetToken: string) =>
    request<{ status: string }>(`/cameras/${cameraId}/ptz/presets/${presetToken}`, {
      method: 'DELETE',
    }),

  // Media (C)
  getProfiles: (cameraId: string) =>
    request<{ profiles: any[] }>(`/cameras/${cameraId}/profiles`),
  getSnapshotUri: (cameraId: string, profile?: string) => {
    const q = profile ? `?profile=${profile}` : '';
    return request<{ snapshot_uri: string }>(`/cameras/${cameraId}/snapshot${q}`);
  },
  getStreamUri: (cameraId: string, profile?: string, protocol?: string) => {
    const params = new URLSearchParams();
    if (profile) params.set('profile', profile);
    if (protocol) params.set('protocol', protocol);
    const q = params.toString() ? `?${params.toString()}` : '';
    return request<{ uri: string }>(`/cameras/${cameraId}/stream-uri${q}`);
  },
  getVideoSources: (cameraId: string) =>
    request<{ video_sources: any[] }>(`/cameras/${cameraId}/video-sources`),
  getAudioSources: (cameraId: string) =>
    request<{ audio_sources: any[] }>(`/cameras/${cameraId}/audio-sources`),

  // Imaging (E)
  getImagingSettings: (cameraId: string, profile?: string) => {
    const q = profile ? `?profile=${profile}` : '';
    return request<any>(`/cameras/${cameraId}/imaging/settings${q}`);
  },
  setImagingSettings: (cameraId: string, profileToken: string, settings: any) =>
    request<any>(`/cameras/${cameraId}/imaging/settings`, {
      method: 'PUT', body: JSON.stringify({ profile_token: profileToken, settings }),
    }),
  moveFocus: (cameraId: string, speed: number) =>
    request<any>(`/cameras/${cameraId}/imaging/focus/move`, {
      method: 'POST', body: JSON.stringify({ speed }),
    }),
  stopFocus: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/imaging/focus/stop`, { method: 'POST' }),
  getImagingStatus: (cameraId: string, profile?: string) => {
    const q = profile ? `?profile=${profile}` : '';
    return request<any>(`/cameras/${cameraId}/imaging/status${q}`);
  },

  // Device/Network (G)
  getDeviceInfo: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/device/info`),
  getDeviceCapabilities: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/device/capabilities`),
  getDeviceServices: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/device/services`),
  getDeviceDate: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/device/date`),
  rebootDevice: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/device/reboot`, { method: 'POST' }),
  getNetworkInterfaces: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/network/interfaces`),
  getDNS: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/network/dns`),
  setDNS: (cameraId: string, fromDhcp: boolean, dnsServers: string[]) =>
    request<any>(`/cameras/${cameraId}/network/dns`, {
      method: 'PUT', body: JSON.stringify({ from_dhcp: fromDhcp, dns_servers: dnsServers }),
    }),
  getNTP: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/network/ntp`),
  getHostname: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/network/hostname`),
  setHostname: (cameraId: string, hostname: string) =>
    request<any>(`/cameras/${cameraId}/network/hostname`, {
      method: 'PUT', body: JSON.stringify({ hostname }),
    }),

  // Recording (I)
  listOnvifRecordings: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/recording/recordings`),
  createOnvifRecording: (cameraId: string, data: any) =>
    request<any>(`/cameras/${cameraId}/recording/recordings`, {
      method: 'POST', body: JSON.stringify(data),
    }),
  deleteOnvifRecording: (cameraId: string, token: string) =>
    request<any>(`/cameras/${cameraId}/recording/recordings/${token}`, { method: 'DELETE' }),
  getRecordingTracks: (cameraId: string, token: string) =>
    request<any>(`/cameras/${cameraId}/recording/recordings/${token}/tracks`),
  getReplayUri: (cameraId: string, recordingToken: string) =>
    request<any>(`/cameras/${cameraId}/recording/replay?recording=${recordingToken}`),
  createRecordingJob: (cameraId: string, data: any) =>
    request<any>(`/cameras/${cameraId}/recording/jobs`, {
      method: 'POST', body: JSON.stringify(data),
    }),

  // Analytics (K)
  getAnalyticsModules: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/analytics/modules`),
  getAnalyticsRules: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/analytics/rules`),
  createAnalyticsRule: (cameraId: string, data: any) =>
    request<any>(`/cameras/${cameraId}/analytics/rules`, {
      method: 'POST', body: JSON.stringify(data),
    }),
  deleteAnalyticsRule: (cameraId: string, token: string) =>
    request<any>(`/cameras/${cameraId}/analytics/rules/${token}`, { method: 'DELETE' }),
  getAnalyticsState: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/analytics/state`),

  // Diagnostics (L)
  getDeviceDiagnostics: (cameraId: string) =>
    request<any>(`/cameras/${cameraId}/diagnostics`),
  getServiceDebug: () =>
    request<any>('/diagnostics'),

  getStreamUrl: async (cameraId: string, type: StreamType = 'main'): Promise<string> => {
    const data = await request<{ url: string }>(`/stream/${cameraId}?type=${type}`);
    return data.url;
  },

  listBookmarks: (cameraId?: string) => {
    const params = cameraId ? `?camera_id=${cameraId}` : '';
    return request<{ bookmarks: Bookmark[] }>(`/bookmarks${params}`);
  },

  createBookmark: (cameraId: string, timestamp: string, label: string) => {
    const username = localStorage.getItem('username') || 'unknown';
    return request<{ id: string; status: string }>('/bookmarks', {
      method: 'POST',
      body: JSON.stringify({ camera_id: cameraId, timestamp, label, created_by: username }),
    });
  },

  exportRecording: (cameraId: string, startTime: string, endTime: string, watermark: boolean) =>
    request<{ file_path: string; sha256: string; size_bytes: number }>('/export', {
      method: 'POST',
      body: JSON.stringify({ camera_id: cameraId, start_time: startTime, end_time: endTime, watermark }),
    }),

  listAlerts: () =>
    request<{ alerts: { id: string; rule_id: string; camera_id: string; message: string; status: string; created_at: string }[] }>('/alerts'),

  acknowledgeAlert: (id: string, username: string) =>
    request<{ status: string }>('/alerts', {
      method: 'POST',
      body: JSON.stringify({ id, username }),
    }),

  getPeopleCounts: () =>
    request<{ counts: { camera_id: string; zone_id: string; count: number }[] }>('/analytics/people-counts'),

  setRelayState: (cameraId: string, relayId: string, state: boolean) =>
    request<{ status: string }>(`/cameras/${cameraId}/io`, {
      method: 'POST',
      body: JSON.stringify({ relay_id: relayId, state: state ? 'on' : 'off' }),
    }),

  getFacialDetections: (params: {camera_id?: string; name?: string; start_time?: string; end_time?: string; limit?: number}) => {
    const qs = Object.entries(params).filter(([_, v]) => v !== undefined && v !== '').map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`).join('&');
    return request<any>(`/analytics/facial?${qs}`);
  },

  getHeatmap: (cameraId: string, start?: string, end?: string) => {
    const params = new URLSearchParams({ camera_id: cameraId });
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    return request<{cells: any[]}>(`/analytics/heatmap?${params}`);
  },

  listTours: () =>
    request<{ tours: Tour[] }>('/tours'),

  createTour: (data: { name: string; steps: TourStep[]; interval: number }) =>
    request<{ id: string; status: string }>('/tours', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  startTour: (id: string) =>
    request<{ status: string }>(`/tours/${id}/start`, { method: 'POST' }),

  stopTour: (id: string) =>
    request<{ status: string }>(`/tours/${id}/stop`, { method: 'POST' }),

  deleteTour: (id: string) =>
    request<{ status: string }>(`/tours/${id}`, { method: 'DELETE' }),

  getPOSTransactions: (params: { camera_id?: string; start_time?: string; end_time?: string; limit?: number }) => {
    const q = new URLSearchParams();
    if (params.camera_id) q.set('camera_id', params.camera_id);
    if (params.start_time) q.set('start_time', params.start_time);
    if (params.end_time) q.set('end_time', params.end_time);
    if (params.limit) q.set('limit', String(params.limit));
    return request<{ transactions: POSTransaction[] }>(`/pos/transaction?${q}`);
  },

  // Storage
  getStorageEstimates: () =>
    request<{ estimates: { camera_id: string; camera_name: string; retention_days: number; daily_usage_gb: number; current_usage_gb: number; estimated_total_gb: number; days_remaining: number }[]; total_daily_gb: number; total_storage_gb: number }>('/storage/estimates'),

  // Camera CRUD
  createCamera: (data: { site_id: string; name: string; connection_url: string; substream_url?: string; ptz_protocol?: string; retention_days?: number; onvif_username?: string; onvif_password?: string }) =>
    request<Camera>('/cameras', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateCamera: (id: string, data: Partial<{ name: string; connection_url: string; substream_url: string; ptz_protocol: string; retention_days: number; onvif_username: string; onvif_password: string }>) =>
    request<Camera>(`/cameras/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteCamera: (id: string) =>
    request<{ status: string }>(`/cameras/${id}`, {
      method: 'DELETE',
    }),

  // Legal Holds
  getLegalHolds: () =>
    request<{ legal_holds: LegalHold[] }>('/legal-holds'),

  createLegalHold: (data: { camera_id: string; reason: string; created_by: string }) =>
    request<{ id: string; status: string }>('/legal-holds', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  releaseLegalHold: (id: string) =>
    request<{ status: string }>(`/legal-holds/${id}/release`, { method: 'POST' }),

  // Audit
  getAuditChain: () =>
    request<{ entries: { id: string; action: string; actor: string; timestamp: string; hash: string; previous_hash: string }[] }>('/audit/chain'),

  verifyAudit: () =>
    request<{ valid: boolean; count: number; first_hash: string; last_hash: string }>('/audit/verify'),

  // Rules
  getRules: () =>
    request<{ rules: { id: string; name: string; enabled: boolean; camera_id: string; condition: string; action: string; created_at: string }[] }>('/rules'),

  toggleRule: (id: string, enabled: boolean) =>
    request<{ status: string }>(`/rules/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),

  // Discovery
  startDiscoveryScan: (data: { site_id: string; methods?: string[]; subnets?: string[]; ports?: number[] }) =>
    request<ScanRecord>('/discovery/scans', { method: 'POST', body: JSON.stringify(data) }),

  getDiscoveryScans: (params?: { site_id?: string; page?: number; per_page?: number }) =>
    request<{ scans: ScanRecord[]; total: number; page: number; per_page: number }>(
      '/discovery/scans?' + new URLSearchParams(params as Record<string, string>).toString()),

  getDiscoveryScan: (id: string) =>
    request<ScanRecord>(`/discovery/scans/${id}`),

  cancelDiscoveryScan: (id: string) =>
    request<{ status: string }>(`/discovery/scans/${id}/cancel`, { method: 'POST' }),

  getDiscoveryResults: (id: string, params?: { page?: number; per_page?: number; query?: string }) =>
    request<{ results: ResultRecord[]; total: number; page: number; per_page: number }>(
      `/discovery/scans/${id}/results?` + new URLSearchParams(params as Record<string, string>).toString()),

  importDiscoveryResults: (scanId: string, data: { result_ids: string[]; credentials?: { result_id: string; username: string; password: string }[] }) =>
    request<{ imported: number; failed: { result_id: string; error: string }[] }>(
      `/discovery/scans/${scanId}/import`, { method: 'POST', body: JSON.stringify(data) }),

  testOnvifCredentials: (data: { ip: string; port: number; username: string; password: string }) =>
    request<{ success: boolean; error?: string }>('/discovery/credentials/test', { method: 'POST', body: JSON.stringify(data) }),

  // ONVIF Events
  subscribeOnvifEvents: (cameraId: string, onvifDeviceUrl: string) =>
    request<{ id: string; status: string }>('/onvif-events/subscribe', {
      method: 'POST',
      body: JSON.stringify({ camera_id: cameraId, onvif_device_url: onvifDeviceUrl }),
    }),

  unsubscribeOnvifEvents: (cameraId: string) =>
    request<{ status: string }>(`/onvif-events/subscribe/${cameraId}`, { method: 'DELETE' }),

  listOnvifSubscriptions: () =>
    request<{ subscriptions: { id: string; camera_id: string; onvif_device_url: string; created_at: string }[] }>('/onvif-events/subscriptions'),

  listOnvifEvents: (params?: {camera_id?: string; event_type?: string; start_time?: string; end_time?: string; limit?: number; offset?: number}) => {
    const q = new URLSearchParams();
    if (params?.camera_id) q.set('camera_id', params.camera_id);
    if (params?.event_type) q.set('event_type', params.event_type);
    if (params?.start_time) q.set('start_time', params.start_time);
    if (params?.end_time) q.set('end_time', params.end_time);
    if (params?.limit) q.set('limit', String(params.limit));
    if (params?.offset) q.set('offset', String(params.offset));
    return request<{events: any[]; total: number}>(`/events?${q}`);
  },

  getEventStats: (params?: {camera_id?: string; start_time?: string; end_time?: string}) => {
    const q = new URLSearchParams();
    if (params?.camera_id) q.set('camera_id', params.camera_id);
    if (params?.start_time) q.set('start_time', params.start_time);
    if (params?.end_time) q.set('end_time', params.end_time);
    return request<{total: number; by_type: Record<string, number>}>(`/events/stats?${q}`);
  },

  listWebhooks: () => request<{webhooks: any[]}>('/webhooks'),
  createWebhook: (data: {name: string; url: string; event_types?: string[]; camera_ids?: string[]; secret?: string}) =>
    request<{id: string; status: string}>('/webhooks', {method: 'POST', body: JSON.stringify(data)}),
  updateWebhook: (id: string, data: any) =>
    request<{status: string}>(`/webhooks/${id}`, {method: 'PUT', body: JSON.stringify(data)}),
  deleteWebhook: (id: string) =>
    request<{status: string}>(`/webhooks/${id}`, {method: 'DELETE'}),

  // Password management
  getPasswordPolicy: () =>
    request<{min_length: number; require_uppercase: boolean; require_lowercase: boolean; require_digit: boolean; require_special: boolean; password_history: number; password_expiry_days: number; max_failed_attempts: number; lockout_minutes: number}>('/password/policy'),

  changePassword: (currentPassword: string, newPassword: string) =>
    request<{status: string; message?: string}>('/password/change', {
      method: 'POST',
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),

  // MFA
  getMFAStatus: () =>
    request<{enabled: boolean}>('/mfa/status'),

  enrollMFA: () =>
    request<{secret: string; uri: string; recovery_codes: string[]}>('/mfa/enroll', {
      method: 'POST',
    }),

  verifyMFA: (code: string) =>
    request<{status?: string; token?: string}>('/mfa/verify', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  recoverMFA: (code: string) =>
    request<{status: string; mfa_token?: string; message?: string}>('/mfa/recovery', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  // Sessions
  getSessions: () =>
    request<{sessions: {id: string; ip_address: string; user_agent: string; created_at: string; expires_at: string; active: boolean}[]}>('/sessions'),

  revokeSession: (sessionId: string) =>
    request<{status: string}>('/sessions/revoke', {
      method: 'POST',
      body: JSON.stringify({ session_id: sessionId }),
    }),

  revokeAllSessions: () =>
    request<{status: string}>('/sessions/revoke-all', {
      method: 'POST',
    }),

  // API Keys
  getAPIKeys: () =>
    request<{api_keys: APIKey[]}>('/api-keys'),

  createAPIKey: (data: {name: string; scopes?: string; camera_ids?: string[]; expires_in?: string}) =>
    request<{id: string; name: string; key: string; key_prefix: string; scopes: string; expires_at?: string}>('/api-keys', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  revokeAPIKey: (keyId: string) =>
    request<{status: string}>(`/api-keys/${keyId}`, {
      method: 'DELETE',
    }),

  // SSO
  getSSOProviders: () =>
    request<{providers: {id: string; name: string; provider_type: string; enabled: boolean; client_id?: string; issuer_url?: string; created_at: string}[]}>('/sso/providers'),

  createSSOProvider: (data: {name: string; provider_type: string; client_id: string; client_secret?: string; issuer_url: string; redirect_uri?: string; enabled?: boolean}) =>
    request<{id: string; status: string}>('/sso/providers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateSSOProvider: (id: string, data: any) =>
    request<{status: string}>(`/sso/providers/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteSSOProvider: (id: string) =>
    request<{status: string}>(`/sso/providers/${id}`, {
      method: 'DELETE',
    }),

  testSSOProvider: (id: string) =>
    request<{status: string; message?: string}>(`/sso/providers/${id}/test`, {
      method: 'POST',
    }),

  refreshToken: (refreshToken: string) =>
    request<{token: string; refresh_token: string}>('/refresh', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: refreshToken }),
    }),

  // Evidence
  getEvidenceCases: () =>
    request<{cases: {id: string; name: string; case_number: string; description: string; tags: string[]; status: string; created_at: string; updated_at: string; item_count: number}[]}>('/evidence'),

  createEvidenceCase: (data: {name: string; case_number: string; description?: string; tags?: string[]}) =>
    request<{id: string; status: string}>('/evidence', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getEvidenceCase: (id: string) =>
    request<{id: string; name: string; case_number: string; description: string; tags: string[]; status: string; items: any[]; chain_of_custody: any[]; created_at: string; updated_at: string}>('/evidence/' + id),

  addEvidenceItem: (caseId: string, data: {name: string; file_path?: string; notes?: string; recording_id?: string; camera_id?: string; timestamp?: string}) =>
    request<{id: string; status: string}>('/evidence/' + caseId + '/items', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  shareEvidence: (caseId: string, data: {expires_at: string; email?: string}) =>
    request<{share_url: string; status: string}>('/evidence/' + caseId + '/share', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  exportEvidenceBundle: (caseId: string) =>
    request<{file_path: string; size_bytes: number}>('/evidence/' + caseId + '/export', {
      method: 'POST',
    }),

  deleteEvidenceCase: (id: string) =>
    request<{status: string}>('/evidence/' + id, {
      method: 'DELETE',
    }),

  // Incidents
  getIncidents: (params?: {status?: string; severity?: string; assigned_to?: string}) => {
    const q = new URLSearchParams();
    if (params?.status) q.set('status', params.status);
    if (params?.severity) q.set('severity', params.severity);
    if (params?.assigned_to) q.set('assigned_to', params.assigned_to);
    const qs = q.toString();
    return request<{incidents: any[]; total: number}>('/incidents' + (qs ? '?' + qs : ''));
  },

  createIncident: (data: {title: string; description?: string; severity: string; camera_ids?: string[]; alert_ids?: string[]}) =>
    request<{id: string; status: string}>('/incidents', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  getIncident: (id: string) =>
    request<any>('/incidents/' + id),

  updateIncidentStatus: (id: string, status: string) =>
    request<{status: string}>('/incidents/' + id + '/status', {
      method: 'PUT',
      body: JSON.stringify({ status }),
    }),

  assignIncident: (id: string, userId: string) =>
    request<{status: string}>('/incidents/' + id + '/assign', {
      method: 'POST',
      body: JSON.stringify({ user_id: userId }),
    }),

  addIncidentNote: (id: string, content: string) =>
    request<{id: string; status: string}>('/incidents/' + id + '/notes', {
      method: 'POST',
      body: JSON.stringify({ content }),
    }),

  escalateIncident: (id: string) =>
    request<{status: string}>('/incidents/' + id + '/escalate', {
      method: 'POST',
    }),

  // Forensics
  forensicSearch: (params: {camera_ids?: string[]; start_time?: string; end_time?: string; object_classes?: string[]; colors?: string[]; direction?: string; min_confidence?: number; limit?: number; offset?: number}) => {
    const body: any = {};
    if (params.camera_ids?.length) body.camera_ids = params.camera_ids;
    if (params.start_time) body.start_time = params.start_time;
    if (params.end_time) body.end_time = params.end_time;
    if (params.object_classes?.length) body.object_classes = params.object_classes;
    if (params.colors?.length) body.colors = params.colors;
    if (params.direction) body.direction = params.direction;
    if (params.min_confidence) body.min_confidence = params.min_confidence;
    if (params.limit) body.limit = params.limit;
    if (params.offset) body.offset = params.offset;
    return request<{results: any[]; total: number; track_paths?: any[]}>('/forensics/search', {
      method: 'POST',
      body: JSON.stringify(body),
    });
  },

  exportForensics: (params: any, format: 'csv' | 'json') =>
    request<{file_path: string}>('/forensics/export', {
      method: 'POST',
      body: JSON.stringify({ ...params, format }),
    }),

  // Config
  getConfigCategories: () =>
    request<{categories: string[]}>('/admin/config'),

  getConfigCategory: (category: string) =>
    request<{category: string; config: Record<string, {value: any; type: string; description: string; schema?: any}>; updated_at: string}>('/admin/config/' + category),

  updateConfig: (category: string, config: Record<string, any>) =>
    request<{status: string}>('/admin/config/' + category, {
      method: 'PUT',
      body: JSON.stringify({ config }),
    }),

  getConfigHistory: (category?: string) => {
    const q = category ? '?category=' + encodeURIComponent(category) : '';
    return request<{entries: {id: string; category: string; key: string; old_value: any; new_value: any; changed_by: string; changed_at: string}[]}>('/admin/config/history' + q);
  },

  importConfig: (data: any) =>
    request<{status: string; count: number}>('/admin/config/import', {
      method: 'POST',
      body: JSON.stringify({ config: data }),
    }),

  exportConfig: () =>
    request<{config: Record<string, any>; exported_at: string}>('/admin/config/export'),

  // Retention
  getRetentionPolicies: () =>
    request<{policies: {camera_id: string; camera_name: string; retention_days: number; archive_enabled: boolean; archive_after_days: number; storage_class: string}[]; global_retention_days: number; global_archive_enabled: boolean; global_archive_after_days: number}>('/admin/retention'),

  updateRetentionPolicy: (cameraId: string, data: {retention_days?: number; archive_enabled?: boolean; archive_after_days?: number; storage_class?: string}) =>
    request<{status: string}>('/admin/retention/' + cameraId, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  bulkUpdateRetention: (policies: {camera_id: string; retention_days: number}[]) =>
    request<{status: string; count: number}>('/admin/retention/bulk', {
      method: 'POST',
      body: JSON.stringify({ policies }),
    }),

  updateGlobalRetention: (data: {retention_days?: number; archive_enabled?: boolean; archive_after_days?: number}) =>
    request<{status: string}>('/admin/retention/global', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  // Timeline
  getTimelineData: (params: {start_time: string; end_time: string; cameras?: string[]; zoom?: string}) => {
    const q = new URLSearchParams({ start_time: params.start_time, end_time: params.end_time });
    if (params.zoom) q.set('zoom', params.zoom);
    if (params.cameras?.length) q.set('cameras', params.cameras.join(','));
    return request<{segments: any[]; density: {timestamp: string; count: number}[]; total: number}>('/admin/timeline?' + q.toString());
  },

  // AI Zones
  getZones: (type?: string) => {
    const q = type ? '?type=' + encodeURIComponent(type) : '';
    return request<{zones: any[]}>('/admin/zones' + q);
  },

  createZone: (data: any) =>
    request<{id: string; status: string}>('/admin/zones', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateZone: (id: string, data: any) =>
    request<{status: string}>('/admin/zones/' + id, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteZone: (id: string) =>
    request<{status: string}>('/admin/zones/' + id, {
      method: 'DELETE',
    }),

  toggleZone: (id: string, enabled: boolean) =>
    request<{status: string}>('/admin/zones/' + id + '/toggle', {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),

  getZoneEvents: (id: string) =>
    request<{events: any[]}>('/admin/zones/' + id + '/events'),

  // Notification Channels
  getNotificationChannels: () =>
    request<{channels: any[]}>('/admin/channels'),

  createNotificationChannel: (data: any) =>
    request<{id: string; status: string}>('/admin/channels', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateNotificationChannel: (id: string, data: any) =>
    request<{status: string}>('/admin/channels/' + id, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteNotificationChannel: (id: string) =>
    request<{status: string}>('/admin/channels/' + id, {
      method: 'DELETE',
    }),

  testNotificationChannel: (id: string) =>
    request<{status: string; message?: string}>('/admin/channels/' + id + '/test', {
      method: 'POST',
    }),

  getNotificationLogs: (channelId?: string) => {
    const q = channelId ? '?channel_id=' + encodeURIComponent(channelId) : '';
    return request<{logs: any[]}>('/admin/channels/logs' + q);
  },

  // CSRF
  getCSRFStatus: () =>
    request<{token: string; enabled: boolean; created_at: string}>('/csrf/status'),

  regenerateCSRFToken: () =>
    request<{token: string; status: string}>('/csrf/regenerate', {
      method: 'POST',
    }),
};
