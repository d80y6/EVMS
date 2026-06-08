import { useState, useEffect } from 'react';
import { api } from '../api/client';

const ZONE_TYPES = ['intrusion', 'loitering', 'abandoned_object'] as const;
type ZoneType = typeof ZONE_TYPES[number];

const ZONE_LABELS: Record<ZoneType, string> = {
  intrusion: 'Intrusion Zones',
  loitering: 'Loitering Zones',
  abandoned_object: 'Abandoned Object Zones',
};

export default function ZonesPage() {
  const [tab, setTab] = useState<ZoneType>('intrusion');
  const [zones, setZones] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [editZone, setEditZone] = useState<any>(null);
  const [form, setForm] = useState({
    name: '',
    type: 'intrusion' as ZoneType,
    coordinates: '',
    sensitivity: 50,
    dwell_time: 5,
    direction: 'any',
    enabled: true,
  });
  const [zoneEvents, setZoneEvents] = useState<any[]>([]);
  const [showEvents, setShowEvents] = useState<string | null>(null);

  const fetchZones = () => {
    setLoading(true);
    api.getZones(tab)
      .then((data) => setZones(data.zones || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchZones(); }, [tab]);

  const resetForm = () => {
    setForm({ name: '', type: tab, coordinates: '', sensitivity: 50, dwell_time: 5, direction: 'any', enabled: true });
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError(null);
      const coords = form.coordinates.split(';').map((p) => {
        const [x, y] = p.trim().split(',').map(Number);
        return { x, y };
      }).filter((p) => !isNaN(p.x) && !isNaN(p.y));
      const payload: any = {
        name: form.name,
        type: form.type,
        coordinates: coords,
        enabled: form.enabled,
      };
      if (form.type === 'loitering') payload.dwell_time = form.dwell_time;
      if (form.type === 'intrusion') payload.direction = form.direction;
      payload.sensitivity = form.sensitivity;

      if (editZone) {
        await api.updateZone(editZone.id, payload);
      } else {
        await api.createZone(payload);
      }
      setShowCreate(false);
      setEditZone(null);
      resetForm();
      fetchZones();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save zone');
    }
  };

  const handleToggle = async (id: string, enabled: boolean) => {
    try {
      await api.toggleZone(id, !enabled);
      setZones((prev) => prev.map((z) => (z.id === id ? { ...z, enabled: !enabled } : z)));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to toggle zone');
    }
  };

  const handleDelete = async (id: string) => {
    if (!window.confirm('Delete this zone?')) return;
    try {
      await api.deleteZone(id);
      fetchZones();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete zone');
    }
  };

  const handleEdit = (zone: any) => {
    setEditZone(zone);
    const coordsStr = (zone.coordinates || []).map((p: any) => `${p.x},${p.y}`).join('; ');
    setForm({
      name: zone.name,
      type: zone.type || tab,
      coordinates: coordsStr,
      sensitivity: zone.sensitivity ?? 50,
      dwell_time: zone.dwell_time ?? 5,
      direction: zone.direction || 'any',
      enabled: zone.enabled ?? true,
    });
    setShowCreate(true);
  };

  const handleViewEvents = async (id: string) => {
    try {
      const data = await api.getZoneEvents(id);
      setZoneEvents(data.events || []);
      setShowEvents(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load events');
    }
  };

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">AI Zone Management</h2>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {/* Tabs */}
      <div className="flex items-center gap-4 border-b border-slate-800 pb-2">
        {ZONE_TYPES.map((t) => (
          <button key={t} onClick={() => { setTab(t); setShowCreate(false); setEditZone(null); }}
            className={`pb-2 text-sm font-medium transition-colors border-b-2 -mb-2 ${
              tab === t ? 'text-indigo-400 border-indigo-400' : 'text-slate-500 border-transparent hover:text-slate-300'
            }`}>
            {ZONE_LABELS[t]}
          </button>
        ))}
        <div className="flex-1" />
        <button onClick={() => { setShowCreate(!showCreate); setEditZone(null); resetForm(); }}
          className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium rounded-lg transition-colors">
          {showCreate ? 'Cancel' : '+ New Zone'}
        </button>
      </div>

      {/* Create/Edit Form */}
      {showCreate && (
        <form onSubmit={handleCreate} className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h3 className="text-sm font-medium text-slate-400">{editZone ? 'Edit Zone' : 'New Zone'}</h3>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Zone Name</label>
              <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Type</label>
              <select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value as ZoneType })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
                {ZONE_TYPES.map((t) => <option key={t} value={t}>{ZONE_LABELS[t]}</option>)}
              </select>
            </div>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Polygon Coordinates (x,y; x,y; ...)</label>
            <input type="text" value={form.coordinates} onChange={(e) => setForm({ ...form, coordinates: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white font-mono"
              placeholder="0.1,0.1; 0.9,0.1; 0.9,0.9; 0.1,0.9" />
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Sensitivity: {form.sensitivity}%</label>
              <input type="range" min={1} max={100} value={form.sensitivity}
                onChange={(e) => setForm({ ...form, sensitivity: Number(e.target.value) })}
                className="w-full accent-indigo-500" />
            </div>
            {form.type === 'loitering' && (
              <div className="space-y-2">
                <label className="text-xs text-slate-500">Dwell Time (seconds)</label>
                <input type="number" value={form.dwell_time} onChange={(e) => setForm({ ...form, dwell_time: Number(e.target.value) })} min={1}
                  className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" />
              </div>
            )}
            {form.type === 'intrusion' && (
              <div className="space-y-2">
                <label className="text-xs text-slate-500">Direction</label>
                <select value={form.direction} onChange={(e) => setForm({ ...form, direction: e.target.value })}
                  className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
                  <option value="any">Any</option>
                  <option value="left-to-right">Left to Right</option>
                  <option value="right-to-left">Right to Left</option>
                  <option value="top-to-bottom">Top to Bottom</option>
                  <option value="bottom-to-top">Bottom to Top</option>
                </select>
              </div>
            )}
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Enabled</label>
              <select value={String(form.enabled)} onChange={(e) => setForm({ ...form, enabled: e.target.value === 'true' })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
                <option value="true">Yes</option>
                <option value="false">No</option>
              </select>
            </div>
          </div>
          <div className="flex gap-2">
            <button type="submit" className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
              {editZone ? 'Update' : 'Create'}
            </button>
            <button type="button" onClick={() => { setShowCreate(false); setEditZone(null); resetForm(); }}
              className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-colors">
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* Zone List */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {loading && <p className="p-4 text-sm text-slate-400">Loading zones...</p>}
        {!loading && zones.length === 0 && <p className="p-6 text-sm text-slate-500">No {tab} zones configured.</p>}
        {!loading && zones.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left p-3">Name</th>
                <th className="text-left p-3">Coordinates</th>
                <th className="text-left p-3">Sensitivity</th>
                <th className="text-left p-3">Enabled</th>
                <th className="text-left p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {zones.map((zone) => (
                <tr key={zone.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="p-3 font-medium">{zone.name}</td>
                  <td className="p-3 text-xs text-slate-500">
                    {(zone.coordinates || []).length} points
                  </td>
                  <td className="p-3 text-xs">{zone.sensitivity ?? '-'}%</td>
                  <td className="p-3">
                    <button onClick={() => handleToggle(zone.id, zone.enabled)}
                      className={`px-3 py-1 text-xs rounded transition-colors ${
                        zone.enabled ? 'bg-green-600 text-white' : 'bg-slate-700 text-slate-400'
                      }`}>
                      {zone.enabled ? 'ON' : 'OFF'}
                    </button>
                  </td>
                  <td className="p-3">
                    <div className="flex gap-2">
                      <button onClick={() => handleEdit(zone)}
                        className="text-xs px-2 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Edit</button>
                      <button onClick={() => handleViewEvents(zone.id)}
                        className="text-xs px-2 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">Events</button>
                      <button onClick={() => handleDelete(zone.id)}
                        className="text-xs px-2 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">Delete</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Zone Events Modal */}
      {showEvents && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-lg max-h-[80vh] overflow-y-auto space-y-4">
            <div className="flex items-center justify-between">
              <h4 className="text-sm font-medium text-slate-300">Zone Events</h4>
              <button onClick={() => setShowEvents(null)} className="text-xs text-slate-500 hover:text-slate-300">Close</button>
            </div>
            {zoneEvents.length === 0 && <p className="text-sm text-slate-500">No recent events.</p>}
            {zoneEvents.map((evt: any, i: number) => (
              <div key={i} className="bg-slate-800 rounded-lg p-3 text-xs space-y-1">
                <div className="text-slate-300">{evt.object_type || evt.object_class}</div>
                <div className="text-slate-500">{evt.camera_id} - {evt.event_time ? new Date(evt.event_time).toLocaleString() : '-'}</div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
