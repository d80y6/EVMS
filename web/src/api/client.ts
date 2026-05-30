const API_BASE = '/api';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('auth_token');
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_BASE}${path}`, { ...options, headers });

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

export interface LoginResponse {
  token: string;
}

export const api = {
  login: (username: string, password: string) =>
    request<LoginResponse>('/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),

  getCameras: () =>
    request<{ cameras: Camera[] }>('/cameras'),

  getRecordings: () =>
    request<{ recordings: Recording[] }>('/recordings'),

  getEvents: () =>
    request<{ events: AIEvent[] }>('/events'),

  getPlaybackUrl: (path: string) =>
    `${API_BASE}/playback/${path}`,

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
    `${API_BASE}${path}`,

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

  smartSearch: (params: { camera_id?: string; object_type?: string; min_confidence?: number; start_time?: string; end_time?: string; limit?: number }) => {
    const q = new URLSearchParams();
    if (params.camera_id) q.set('camera_id', params.camera_id);
    if (params.object_type) q.set('object_type', params.object_type);
    if (params.min_confidence !== undefined) q.set('min_confidence', String(params.min_confidence));
    if (params.start_time) q.set('start_time', params.start_time);
    if (params.end_time) q.set('end_time', params.end_time);
    if (params.limit) q.set('limit', String(params.limit));
    return request<{ results: { id: string; camera_id: string; event_time: string; object_type: string; confidence: number; track_id: string; thumbnail: string }[]; total: number }>(`/search?${q}`);
  },
};
