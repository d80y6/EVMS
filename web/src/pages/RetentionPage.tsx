import { useState, useEffect } from 'react';
import { api } from '../api/client';

interface RetentionPolicy {
  camera_id: string;
  camera_name: string;
  retention_days: number;
  archive_enabled: boolean;
  archive_after_days: number;
  storage_class: string;
}

export default function RetentionPage() {
  const [policies, setPolicies] = useState<RetentionPolicy[]>([]);
  const [globalRetention, setGlobalRetention] = useState(30);
  const [globalArchive, setGlobalArchive] = useState(false);
  const [globalArchiveAfter, setGlobalArchiveAfter] = useState(90);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [editDays, setEditDays] = useState<Record<string, number>>({});
  const [editArchive, setEditArchive] = useState<Record<string, boolean>>({});
  const [selectedCameras, setSelectedCameras] = useState<Set<string>>(new Set());
  const [bulkDays, setBulkDays] = useState(30);

  useEffect(() => {
    api.getRetentionPolicies()
      .then((data) => {
        setPolicies(data.policies || []);
        setGlobalRetention(data.global_retention_days ?? 30);
        setGlobalArchive(data.global_archive_enabled ?? false);
        setGlobalArchiveAfter(data.global_archive_after_days ?? 90);
        const days: Record<string, number> = {};
        const arch: Record<string, boolean> = {};
        (data.policies || []).forEach((p) => {
          days[p.camera_id] = p.retention_days;
          arch[p.camera_id] = p.archive_enabled;
        });
        setEditDays(days);
        setEditArchive(arch);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const handleSaveCamera = async (cameraId: string) => {
    setSaving(cameraId);
    setError(null);
    try {
      await api.updateRetentionPolicy(cameraId, {
        retention_days: editDays[cameraId],
        archive_enabled: editArchive[cameraId],
      });
      setPolicies((prev) => prev.map((p) =>
        p.camera_id === cameraId ? { ...p, retention_days: editDays[cameraId], archive_enabled: editArchive[cameraId] } : p
      ));
      setSuccess(`Updated ${cameraId}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(null);
    }
  };

  const handleBulkUpdate = async () => {
    const ids = Array.from(selectedCameras);
    if (ids.length === 0) return;
    try {
      setError(null);
      const res = await api.bulkUpdateRetention(ids.map((id) => ({ camera_id: id, retention_days: bulkDays })));
      setSuccess(`Updated ${res.count} cameras`);
      ids.forEach((id) => setEditDays((prev) => ({ ...prev, [id]: bulkDays })));
      setPolicies((prev) => prev.map((p) =>
        ids.includes(p.camera_id) ? { ...p, retention_days: bulkDays } : p
      ));
      setSelectedCameras(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Bulk update failed');
    }
  };

  const handleGlobalSave = async () => {
    try {
      setError(null);
      await api.updateGlobalRetention({
        retention_days: globalRetention,
        archive_enabled: globalArchive,
        archive_after_days: globalArchiveAfter,
      });
      setSuccess('Global retention settings saved');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save global settings');
    }
  };

  const toggleSelect = (id: string) => {
    setSelectedCameras((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  if (loading) return <div className="p-4 text-slate-400">Loading retention policies...</div>;

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Per-Camera Retention Policies</h2>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}
      {success && <div className="bg-green-900/20 border border-green-800 rounded-xl p-4"><p className="text-sm text-green-400">{success}</p></div>}

      {/* Global Settings */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Global Retention Settings</h3>
        <div className="grid grid-cols-3 gap-4">
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Default Retention (days)</label>
            <input type="number" value={globalRetention} onChange={(e) => setGlobalRetention(Number(e.target.value))} min={1} max={365}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" />
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Archive Enabled</label>
            <select value={String(globalArchive)} onChange={(e) => setGlobalArchive(e.target.value === 'true')}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
              <option value="true">Yes</option>
              <option value="false">No</option>
            </select>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Archive After (days)</label>
            <input type="number" value={globalArchiveAfter} onChange={(e) => setGlobalArchiveAfter(Number(e.target.value))} min={1} max={365}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" />
          </div>
        </div>
        <button onClick={handleGlobalSave}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
          Save Global Settings
        </button>
      </div>

      {/* Bulk Update Bar */}
      {selectedCameras.size > 0 && (
        <div className="bg-indigo-900/30 border border-indigo-800 rounded-xl p-4 flex items-center gap-4">
          <span className="text-xs text-indigo-300">{selectedCameras.size} camera(s) selected</span>
          <input type="number" value={bulkDays} onChange={(e) => setBulkDays(Number(e.target.value))} min={1} max={365}
            className="w-20 bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-white" />
          <button onClick={handleBulkUpdate}
            className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">
            Set Retention Days
          </button>
          <button onClick={() => setSelectedCameras(new Set())}
            className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">
            Clear Selection
          </button>
        </div>
      )}

      {/* Camera List */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
              <th className="text-left pb-3 pr-2 w-8"></th>
              <th className="text-left pb-3 pr-4">Camera</th>
              <th className="text-left pb-3 pr-4">Retention (days)</th>
              <th className="text-left pb-3 pr-4">Archive</th>
              <th className="text-left pb-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {policies.length === 0 && (
              <tr>
                <td colSpan={5} className="py-8 text-center text-slate-500">No cameras configured.</td>
              </tr>
            )}
            {policies.map((p) => (
              <tr key={p.camera_id} className="border-b border-slate-800/50 text-slate-300">
                <td className="py-3 pr-2">
                  <input type="checkbox" checked={selectedCameras.has(p.camera_id)} onChange={() => toggleSelect(p.camera_id)}
                    className="accent-indigo-500" />
                </td>
                <td className="py-3 pr-4">
                  <span>{p.camera_name}</span>
                  <span className="text-xs text-slate-600 ml-2">({p.camera_id})</span>
                </td>
                <td className="py-3 pr-4">
                  <input type="number" value={editDays[p.camera_id] ?? p.retention_days}
                    onChange={(e) => setEditDays((prev) => ({ ...prev, [p.camera_id]: Number(e.target.value) }))}
                    min={1} max={365}
                    className="w-20 bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-white" />
                </td>
                <td className="py-3 pr-4">
                  <select value={String(editArchive[p.camera_id] ?? p.archive_enabled)}
                    onChange={(e) => setEditArchive((prev) => ({ ...prev, [p.camera_id]: e.target.value === 'true' }))}
                    className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-white">
                    <option value="true">Enabled</option>
                    <option value="false">Disabled</option>
                  </select>
                </td>
                <td className="py-3">
                  <button onClick={() => handleSaveCamera(p.camera_id)} disabled={saving === p.camera_id}
                    className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">
                    {saving === p.camera_id ? 'Saving...' : 'Save'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
