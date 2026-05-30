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
};
