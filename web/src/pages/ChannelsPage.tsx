import { useState, useEffect } from 'react';
import { api } from '../api/client';

const CHANNEL_TYPES = ['email', 'sms', 'push'] as const;

const TYPE_LABELS: Record<string, string> = {
  email: 'Email',
  sms: 'SMS',
  push: 'Push Notification',
};

export default function ChannelsPage() {
  const [channels, setChannels] = useState<any[]>([]);
  const [logs, setLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [editChannel, setEditChannel] = useState<any>(null);
  const [showLogs, setShowLogs] = useState(false);
  const [form, setForm] = useState({
    name: '',
    type: 'email' as string,
    config: '',
    enabled: true,
  });

  const fetchChannels = () => {
    setLoading(true);
    api.getNotificationChannels()
      .then((data) => setChannels(data.channels || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchChannels(); }, []);

  const resetForm = () => {
    setForm({ name: '', type: 'email', config: '', enabled: true });
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError(null);
      let config: any;
      try { config = JSON.parse(form.config); } catch { config = { address: form.config }; }
      const payload = { name: form.name, type: form.type, config, enabled: form.enabled };
      if (editChannel) {
        await api.updateNotificationChannel(editChannel.id, payload);
      } else {
        await api.createNotificationChannel(payload);
      }
      setShowCreate(false);
      setEditChannel(null);
      resetForm();
      fetchChannels();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save channel');
    }
  };

  const handleTest = async (id: string) => {
    try {
      setError(null);
      await api.testNotificationChannel(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Test failed');
    }
  };

  const handleDelete = async (id: string) => {
    if (!window.confirm('Delete this channel?')) return;
    try {
      await api.deleteNotificationChannel(id);
      fetchChannels();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  const handleEdit = (ch: any) => {
    setEditChannel(ch);
    setForm({
      name: ch.name,
      type: ch.type,
      config: JSON.stringify(ch.config || {}, null, 2),
      enabled: ch.enabled ?? true,
    });
    setShowCreate(true);
  };

  const handleViewLogs = async (channelId?: string) => {
    try {
      const data = await api.getNotificationLogs(channelId);
      setLogs(data.logs || []);
      setShowLogs(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load logs');
    }
  };

  const getHealthColor = (channel: any): string => {
    if (channel.health === 'healthy' || channel.status === 'active') return 'text-green-400';
    if (channel.health === 'degraded' || channel.status === 'error') return 'text-yellow-400';
    if (channel.status === 'inactive' || channel.health === 'down') return 'text-red-400';
    return 'text-slate-500';
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Notification Channels</h2>
        <div className="flex gap-2">
          <button onClick={() => { handleViewLogs(); }}
            className="px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-white text-xs font-medium rounded-lg transition-colors">
            View Logs
          </button>
          <button onClick={() => { setShowCreate(!showCreate); setEditChannel(null); resetForm(); }}
            className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium rounded-lg transition-colors">
            {showCreate ? 'Cancel' : '+ New Channel'}
          </button>
        </div>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {showCreate && (
        <form onSubmit={handleSave} className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h3 className="text-sm font-medium text-slate-400">{editChannel ? 'Edit Channel' : 'New Channel'}</h3>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Name</label>
              <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Type</label>
              <select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
                {CHANNEL_TYPES.map((t) => <option key={t} value={t}>{TYPE_LABELS[t] || t}</option>)}
              </select>
            </div>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Configuration (JSON)</label>
            <textarea value={form.config} onChange={(e) => setForm({ ...form, config: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white font-mono" rows={4}
              placeholder='{"address": "..."} or enter email/SMS address' />
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Enabled</label>
            <select value={String(form.enabled)} onChange={(e) => setForm({ ...form, enabled: e.target.value === 'true' })}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
              <option value="true">Yes</option>
              <option value="false">No</option>
            </select>
          </div>
          <div className="flex gap-2">
            <button type="submit" className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
              {editChannel ? 'Update' : 'Create'}
            </button>
            <button type="button" onClick={() => { setShowCreate(false); setEditChannel(null); resetForm(); }}
              className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-colors">
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* Channel List */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {loading && <p className="p-4 text-sm text-slate-400">Loading channels...</p>}
        {!loading && channels.length === 0 && <p className="p-6 text-sm text-slate-500">No notification channels configured.</p>}
        {!loading && channels.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left p-3">Name</th>
                <th className="text-left p-3">Type</th>
                <th className="text-left p-3">Status</th>
                <th className="text-left p-3">Enabled</th>
                <th className="text-left p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {channels.map((ch) => (
                <tr key={ch.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="p-3 font-medium">{ch.name}</td>
                  <td className="p-3">
                    <span className="text-xs px-2 py-0.5 rounded bg-indigo-900/30 text-indigo-300">
                      {TYPE_LABELS[ch.type] || ch.type}
                    </span>
                  </td>
                  <td className="p-3">
                    <span className={`flex items-center gap-1 text-xs ${getHealthColor(ch)}`}>
                      <span className="w-1.5 h-1.5 rounded-full bg-current" />
                      {ch.health || ch.status || 'unknown'}
                    </span>
                  </td>
                  <td className="p-3">
                    <span className={`text-xs px-2 py-0.5 rounded-full ${
                      ch.enabled ? 'bg-green-900/30 text-green-400' : 'bg-slate-700 text-slate-400'
                    }`}>
                      {ch.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                  </td>
                  <td className="p-3">
                    <div className="flex gap-2">
                      <button onClick={() => handleTest(ch.id)}
                        className="text-xs px-2 py-1 bg-green-700 hover:bg-green-600 text-white rounded transition-colors">Test</button>
                      <button onClick={() => handleEdit(ch)}
                        className="text-xs px-2 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Edit</button>
                      <button onClick={() => handleDelete(ch.id)}
                        className="text-xs px-2 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">Delete</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Notification Logs Modal */}
      {showLogs && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-2xl max-h-[80vh] overflow-y-auto space-y-4">
            <div className="flex items-center justify-between">
              <h4 className="text-sm font-medium text-slate-300">Notification Logs</h4>
              <button onClick={() => setShowLogs(false)} className="text-xs text-slate-500 hover:text-slate-300">Close</button>
            </div>
            {logs.length === 0 && <p className="text-sm text-slate-500">No notification logs.</p>}
            {logs.map((log: any, i: number) => (
              <div key={i} className="bg-slate-800 rounded-lg p-3 text-xs space-y-1">
                <div className="text-slate-300">{log.channel_name || log.channel_id} - {log.status}</div>
                {log.message && <div className="text-slate-500">{log.message}</div>}
                <div className="text-slate-600">{log.created_at ? new Date(log.created_at).toLocaleString() : '-'}</div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
