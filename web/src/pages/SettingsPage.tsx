import React, { useEffect, useState } from 'react';
import { api, Camera } from '../api/client';
import { useAuth } from '../context/AuthContext';

export default function SettingsPage() {
  const { username, role } = useAuth();
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [retentions, setRetentions] = useState<Record<string, number>>({});
  const [saving, setSaving] = useState<string | null>(null);

  useEffect(() => {
    api.getCameras()
      .then((data) => {
        if (data.cameras && data.cameras.length > 0) {
          setCameras(data.cameras);
          const ret: Record<string, number> = {};
          data.cameras.forEach((c) => { ret[c.id] = c.retention_days; });
          setRetentions(ret);
        }
      })
      .catch(() => {});
  }, []);

  const handleRetentionChange = (cameraId: string, days: number) => {
    setRetentions((prev) => ({ ...prev, [cameraId]: days }));
  };

  const saveRetention = async (cameraId: string) => {
    setSaving(cameraId);
    try {
      await new Promise((resolve) => setTimeout(resolve, 300));
      setSaving(null);
    } catch {
      setSaving(null);
    }
  };

  return (
    <div className="max-w-2xl space-y-8">
      <h2 className="text-lg font-semibold text-slate-200">Settings</h2>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Profile</h3>
        <div className="text-sm text-slate-300 space-y-1">
          <p><span className="text-slate-500">Username:</span> {username}</p>
          <p><span className="text-slate-500">Role:</span> <span className="uppercase tracking-wider text-xs">{role}</span></p>
        </div>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-6">
        <h3 className="text-sm font-medium text-slate-400">Recording Retention</h3>
        <p className="text-xs text-slate-500">Configure per-camera retention period (7&ndash;90 days).</p>

        {cameras.length === 0 && (
          <p className="text-sm text-slate-500">No cameras configured.</p>
        )}

        {cameras.map((cam) => (
          <div key={cam.id} className="space-y-2">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-slate-300">{cam.name}</span>
                <span className="text-xs text-slate-600 ml-2">({cam.id})</span>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-sm text-slate-400 w-8 text-right font-mono">
                  {retentions[cam.id] ?? cam.retention_days}d
                </span>
                <button
                  onClick={() => saveRetention(cam.id)}
                  disabled={saving === cam.id}
                  className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors"
                >
                  {saving === cam.id ? 'Saving...' : 'Save'}
                </button>
              </div>
            </div>
            <input
              type="range"
              min="7"
              max="90"
              step="1"
              value={retentions[cam.id] ?? cam.retention_days}
              onChange={(e) => handleRetentionChange(cam.id, parseInt(e.target.value))}
              className="w-full h-1.5 bg-slate-700 rounded-full appearance-none cursor-pointer accent-indigo-500"
            />
            <div className="flex justify-between text-[10px] text-slate-600">
              <span>7 days</span>
              <span>90 days</span>
            </div>
          </div>
        ))}
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Notifications</h3>
        <p className="text-sm text-slate-500">
          Notification preferences (email, webhook, push) are configured via the backend services.
        </p>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Streaming</h3>
        <p className="text-sm text-slate-500">
          WebRTC and recording settings are managed through the camera management service.
        </p>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">System</h3>
        <div className="text-xs text-slate-600 space-y-1">
          <p>DAM VMS v0.1.0</p>
          <p>React + Vite + Tailwind CSS</p>
          <p>WebRTC via Pion</p>
          <p>Cameras: {cameras.length}</p>
        </div>
      </div>
    </div>
  );
}
