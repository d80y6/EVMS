import { useState, useEffect } from 'react';
import { api, Camera } from '../api/client';

export default function CamerasPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [sites, setSites] = useState<{ id: string; name: string; location: string }[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showDialog, setShowDialog] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState({ site_id: '', name: '', connection_url: '', substream_url: '', ptz_protocol: 'none', retention_days: 30 });

  const fetchData = async () => {
    try {
      const [camData, sitesData] = await Promise.all([api.listCameras(), api.getSites()]);
      setCameras(camData);
      setSites(sitesData.sites || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchData(); }, []);

  const handleCreate = async () => {
    try {
      await api.createCamera(form);
      setShowDialog(false);
      setForm({ site_id: '', name: '', connection_url: '', substream_url: '', ptz_protocol: 'none', retention_days: 30 });
      await fetchData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create camera');
    }
  };

  const handleUpdate = async () => {
    if (!editing) return;
    try {
      await api.updateCamera(editing, form);
      setEditing(null);
      setForm({ site_id: '', name: '', connection_url: '', substream_url: '', ptz_protocol: 'none', retention_days: 30 });
      await fetchData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update camera');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this camera?')) return;
    try {
      await api.deleteCamera(id);
      await fetchData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete camera');
    }
  };

  const openEdit = (cam: Camera) => {
    setEditing(cam.id);
    setForm({ site_id: cam.site_id, name: cam.name, connection_url: cam.connection_url, substream_url: cam.substream_url || '', ptz_protocol: cam.ptz_protocol, retention_days: cam.retention_days });
    setShowDialog(true);
  };

  if (loading) return <div className="p-4 text-slate-400">Loading cameras...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Cameras</h2>
        <button onClick={() => { setEditing(null); setShowDialog(true); }}
          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">+ Add Camera</button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {cameras.length === 0 && <p className="p-6 text-sm text-slate-500">No cameras configured.</p>}
        {cameras.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Name</th><th className="p-3">Site</th><th className="p-3">Status</th><th className="p-3">PTZ</th><th className="p-3">Retention</th><th className="p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {cameras.map(cam => (
                <tr key={cam.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300">{cam.name}</td>
                  <td className="p-3 text-slate-300">{cam.site_id}</td>
                  <td className="p-3"><span className={`px-2 py-0.5 rounded text-xs ${cam.status === 'online' ? 'bg-green-900/40 text-green-400' : 'bg-red-900/40 text-red-400'}`}>{cam.status}</span></td>
                  <td className="p-3 text-slate-300">{cam.ptz_protocol || 'none'}</td>
                  <td className="p-3 text-slate-300">{cam.retention_days}d</td>
                  <td className="p-3 flex gap-2">
                    <button onClick={() => openEdit(cam)} className="text-xs px-2 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">Edit</button>
                    <button onClick={() => handleDelete(cam.id)} className="text-xs px-2 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showDialog && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-md space-y-4">
            <h3 className="text-sm font-medium text-slate-300">{editing ? 'Edit Camera' : 'Add Camera'}</h3>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Site</label>
              <select value={form.site_id} onChange={e => setForm({ ...form, site_id: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300">
                <option value="">Select site</option>
                {sites.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Name</label>
              <input type="text" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Connection URL</label>
              <input type="text" value={form.connection_url} onChange={e => setForm({ ...form, connection_url: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Substream URL</label>
              <input type="text" value={form.substream_url} onChange={e => setForm({ ...form, substream_url: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">PTZ Protocol</label>
              <select value={form.ptz_protocol} onChange={e => setForm({ ...form, ptz_protocol: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300">
                <option value="none">None</option>
                <option value="onvif">ONVIF</option>
                <option value="pelco_d">Pelco D</option>
                <option value="pelco_p">Pelco P</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Retention Days</label>
              <input type="number" value={form.retention_days} onChange={e => setForm({ ...form, retention_days: parseInt(e.target.value) || 30 })} min={1} max={365}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowDialog(false)}
                className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Cancel</button>
              <button onClick={editing ? handleUpdate : handleCreate} disabled={!form.name}
                className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">{editing ? 'Update' : 'Create'}</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
