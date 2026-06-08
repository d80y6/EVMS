import { useState, useEffect } from 'react';
import { api, AIEvent } from '../api/client';

interface Alert { id: string; camera_id: string; message: string; status: string; created_at: string; }
interface Rule { id: string; name: string; enabled: boolean; camera_id: string; condition: string; action: string; created_at: string; }
interface OnvifEvent { id: string; camera_id: string; event_type: string; topic: string; message: string; severity: string; event_time: string; }

export default function EventsPage() {
  const [events, setEvents] = useState<AIEvent[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [rules, setRules] = useState<Rule[]>([]);
  const [onvifEvents, setOnvifEvents] = useState<OnvifEvent[]>([]);
  const [onvifTotal, setOnvifTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<'events' | 'onvif' | 'alerts' | 'rules'>('events');
  const [filterCamera, setFilterCamera] = useState('');
  const [filterType, setFilterType] = useState('');
  const [filterDays, setFilterDays] = useState(1);
  const [page, setPage] = useState(0);
  const pageSize = 50;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    Promise.all([
      api.getEvents().then(d => { if (!cancelled) setEvents((d as any).events || []); }).catch(() => {}),
      api.listAlerts().then(d => { if (!cancelled) setAlerts((d as any).alerts || []); }).catch(() => {}),
      api.getRules().then(d => { if (!cancelled) setRules(d.rules || []); }).catch(() => {}),
    ]).catch(err => {
      if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load');
    }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (tab !== 'onvif') return;
    const start = new Date(Date.now() - filterDays * 86400000).toISOString();
    setLoading(true);
    api.listOnvifEvents({ camera_id: filterCamera || undefined, event_type: filterType || undefined, start_time: start, limit: pageSize, offset: page * pageSize })
      .then(d => { setOnvifEvents(d.events || []); setOnvifTotal(d.total || 0); })
      .catch(() => setError('Failed to load ONVIF events'))
      .finally(() => setLoading(false));
  }, [tab, filterCamera, filterType, filterDays, page]);

  const handleAck = async (id: string) => {
    try {
      await api.acknowledgeAlert(id, 'admin');
      setAlerts(prev => prev.map(a => a.id === id ? { ...a, status: 'acknowledged' } : a));
    } catch {}
  };

  const handleToggleRule = async (id: string, enabled: boolean) => {
    try {
      await api.toggleRule(id, !enabled);
      setRules(prev => prev.map(r => r.id === id ? { ...r, enabled: !enabled } : r));
    } catch {}
  };

  if (error) return <div className="p-4 text-red-400">Error: {error}</div>;

  const tabs = ['events', 'onvif', 'alerts', 'rules'] as const;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4 border-b border-slate-800 pb-4">
        {tabs.map(t => (
          <button key={t} onClick={() => { setTab(t); setPage(0); }}
            className={`text-sm font-medium pb-2 -mb-4 border-b-2 transition-colors capitalize ${
              tab === t ? 'text-indigo-400 border-indigo-400' : 'text-slate-500 border-transparent hover:text-slate-300'
            }`}>
            {t === 'onvif' ? 'ONVIF Events' : t}
          </button>
        ))}
      </div>

      {tab === 'events' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          {loading && <p className="p-4 text-sm text-slate-400">Loading...</p>}
          {!loading && events.length === 0 && <p className="p-6 text-sm text-slate-500">No AI events recorded.</p>}
          {!loading && events.length > 0 && (
            <table className="w-full text-sm">
              <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Time</th><th className="p-3">Camera</th><th className="p-3">Object</th><th className="p-3">Confidence</th>
              </tr></thead>
              <tbody>
                {events.map(e => (
                  <tr key={e.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                    <td className="p-3 text-slate-300">{new Date(e.event_time).toLocaleString()}</td>
                    <td className="p-3 text-slate-300">{e.camera_id}</td>
                    <td className="p-3 text-slate-300">{e.object_type}</td>
                    <td className="p-3 text-slate-300">{(e.confidence * 100).toFixed(0)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === 'onvif' && (
        <div className="space-y-4">
          <div className="flex flex-wrap gap-3 items-end">
            <div>
              <label className="text-[10px] text-slate-500 block mb-1">Camera ID</label>
              <input value={filterCamera} onChange={e => { setFilterCamera(e.target.value); setPage(0); }}
                className="bg-slate-800 text-white rounded px-2 py-1.5 text-xs w-40" placeholder="All cameras" />
            </div>
            <div>
              <label className="text-[10px] text-slate-500 block mb-1">Event Type</label>
              <input value={filterType} onChange={e => { setFilterType(e.target.value); setPage(0); }}
                className="bg-slate-800 text-white rounded px-2 py-1.5 text-xs w-40" placeholder="All types" />
            </div>
            <div>
              <label className="text-[10px] text-slate-500 block mb-1">Lookback (days)</label>
              <input type="number" min="1" max="90" value={filterDays} onChange={e => { setFilterDays(Number(e.target.value)); setPage(0); }}
                className="bg-slate-800 text-white rounded px-2 py-1.5 text-xs w-20" />
            </div>
          </div>

          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            {loading && <p className="p-4 text-sm text-slate-400">Loading ONVIF events...</p>}
            {!loading && onvifEvents.length === 0 && <p className="p-6 text-sm text-slate-500">No ONVIF events found.</p>}
            {!loading && onvifEvents.length > 0 && (
              <table className="w-full text-sm">
                <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                  <th className="p-3">Time</th><th className="p-3">Camera</th><th className="p-3">Type</th><th className="p-3">Severity</th><th className="p-3">Message</th>
                </tr></thead>
                <tbody>
                  {onvifEvents.map(e => (
                    <tr key={e.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                      <td className="p-3 text-slate-300 text-xs">{new Date(e.event_time).toLocaleString()}</td>
                      <td className="p-3 text-slate-300 text-xs">{e.camera_id}</td>
                      <td className="p-3"><span className="text-xs px-1.5 py-0.5 rounded bg-indigo-900/40 text-indigo-300">{e.event_type}</span></td>
                      <td className="p-3"><span className={`text-xs px-1.5 py-0.5 rounded ${
                        e.severity === 'Critical' ? 'bg-red-900/40 text-red-300' :
                        e.severity === 'Warning' ? 'bg-yellow-900/40 text-yellow-300' :
                        'bg-slate-700 text-slate-400'
                      }`}>{e.severity || 'Info'}</span></td>
                      <td className="p-3 text-slate-300 text-xs">{e.message || e.topic || '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {onvifTotal > pageSize && (
            <div className="flex items-center justify-between text-xs text-slate-400">
              <span>{onvifTotal} total events</span>
              <div className="flex gap-2">
                <button disabled={page === 0} onClick={() => setPage(p => p - 1)}
                  className="px-3 py-1 bg-slate-800 rounded hover:bg-slate-700 disabled:opacity-50">Previous</button>
                <span className="self-center">Page {page + 1}</span>
                <button disabled={(page + 1) * pageSize >= onvifTotal} onClick={() => setPage(p => p + 1)}
                  className="px-3 py-1 bg-slate-800 rounded hover:bg-slate-700 disabled:opacity-50">Next</button>
              </div>
            </div>
          )}
        </div>
      )}

      {tab === 'alerts' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          {alerts.length === 0 && <p className="p-6 text-sm text-slate-500">No alerts.</p>}
          {alerts.length > 0 && (
            <table className="w-full text-sm">
              <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Camera</th><th className="p-3">Message</th><th className="p-3">Status</th><th className="p-3">Actions</th>
              </tr></thead>
              <tbody>
                {alerts.map(a => (
                  <tr key={a.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                    <td className="p-3 text-slate-300">{a.camera_id}</td>
                    <td className="p-3 text-slate-300">{a.message}</td>
                    <td className="p-3"><span className={`px-2 py-0.5 rounded text-xs ${
                      a.status === 'acknowledged' ? 'bg-green-900/40 text-green-400' : 'bg-yellow-900/40 text-yellow-400'
                    }`}>{a.status}</span></td>
                    <td className="p-3">
                      {a.status !== 'acknowledged' && (
                        <button onClick={() => handleAck(a.id)}
                          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">Acknowledge</button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === 'rules' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          {rules.length === 0 && <p className="p-6 text-sm text-slate-500">No rules configured.</p>}
          {rules.length > 0 && (
            <table className="w-full text-sm">
              <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Name</th><th className="p-3">Camera</th><th className="p-3">Condition</th><th className="p-3">Enabled</th>
              </tr></thead>
              <tbody>
                {rules.map(r => (
                  <tr key={r.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                    <td className="p-3 text-slate-300">{r.name}</td>
                    <td className="p-3 text-slate-300">{r.camera_id}</td>
                    <td className="p-3 text-slate-300">{r.condition}</td>
                    <td className="p-3">
                      <button onClick={() => handleToggleRule(r.id, r.enabled)}
                        className={`px-3 py-1 rounded text-xs transition-colors ${
                          r.enabled ? 'bg-green-600 text-white' : 'bg-slate-700 text-slate-400'
                        }`}>
                        {r.enabled ? 'ON' : 'OFF'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
