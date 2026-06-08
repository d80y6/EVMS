import { useEffect, useState } from 'react';
import { api } from '../api/client';

interface Webhook {
  id: string;
  name: string;
  url: string;
  event_types: string[];
  camera_ids: string[];
  enabled: boolean;
  secret?: string;
}

export default function WebhooksPage() {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Webhook | null>(null);
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [eventTypes, setEventTypes] = useState('');
  const [loading, setLoading] = useState(true);

  const load = () => {
    setLoading(true);
    api.listWebhooks().then(d => setWebhooks(d.webhooks || [])).catch(() => {}).finally(() => setLoading(false));
  };

  useEffect(() => { load(); }, []);

  const handleSave = async () => {
    const data = { name, url, event_types: eventTypes.split(',').map(s => s.trim()).filter(Boolean) };
    if (editing) {
      await api.updateWebhook(editing.id, data);
    } else {
      await api.createWebhook(data);
    }
    setShowForm(false);
    setEditing(null);
    setName('');
    setUrl('');
    setEventTypes('');
    load();
  };

  const handleDelete = async (id: string) => {
    await api.deleteWebhook(id);
    load();
  };

  const handleEdit = (wh: Webhook) => {
    setEditing(wh);
    setName(wh.name);
    setUrl(wh.url);
    setEventTypes((wh.event_types || []).join(', '));
    setShowForm(true);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Webhooks</h1>
        <button onClick={() => { setEditing(null); setName(''); setUrl(''); setEventTypes(''); setShowForm(true); }}
          className="px-4 py-2 text-sm bg-indigo-600 rounded hover:bg-indigo-500">
          + Add Webhook
        </button>
      </div>

      {showForm && (
        <div className="bg-slate-900 p-4 rounded-lg space-y-3 max-w-lg">
          <input placeholder="Name" value={name} onChange={e => setName(e.target.value)}
            className="w-full bg-slate-800 text-white rounded px-3 py-2 text-sm" />
          <input placeholder="URL (https://...)" value={url} onChange={e => setUrl(e.target.value)}
            className="w-full bg-slate-800 text-white rounded px-3 py-2 text-sm" />
          <input placeholder="Event types (comma-separated)" value={eventTypes} onChange={e => setEventTypes(e.target.value)}
            className="w-full bg-slate-800 text-white rounded px-3 py-2 text-sm" />
          <div className="flex gap-2">
            <button onClick={handleSave} className="px-4 py-2 text-sm bg-indigo-600 rounded hover:bg-indigo-500">
              {editing ? 'Update' : 'Create'}
            </button>
            <button onClick={() => { setShowForm(false); setEditing(null); }}
              className="px-4 py-2 text-sm bg-slate-700 rounded hover:bg-slate-600">
              Cancel
            </button>
          </div>
        </div>
      )}

      {loading ? (
        <p className="text-sm text-slate-400">Loading...</p>
      ) : webhooks.length === 0 ? (
        <p className="text-sm text-slate-500">No webhooks configured.</p>
      ) : (
        <div className="space-y-2">
          {webhooks.map(wh => (
            <div key={wh.id} className="bg-slate-900 p-4 rounded-lg flex items-center justify-between">
              <div>
                <div className="flex items-center gap-2">
                  <span className="font-medium">{wh.name}</span>
                  <span className={`text-[10px] px-1.5 py-0.5 rounded ${wh.enabled ? 'bg-green-900 text-green-400' : 'bg-slate-700 text-slate-400'}`}>
                    {wh.enabled ? 'Active' : 'Disabled'}
                  </span>
                </div>
                <p className="text-xs text-slate-400 mt-1">{wh.url}</p>
                {wh.event_types?.length > 0 && (
                  <p className="text-[10px] text-slate-500 mt-1">Events: {wh.event_types.join(', ')}</p>
                )}
              </div>
              <div className="flex gap-2">
                <button onClick={() => handleEdit(wh)} className="px-2 py-1 text-xs bg-slate-700 rounded hover:bg-slate-600">Edit</button>
                <button onClick={() => handleDelete(wh.id)} className="px-2 py-1 text-xs bg-red-800 rounded hover:bg-red-700">Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
