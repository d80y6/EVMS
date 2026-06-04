import { useState, useEffect } from 'react';
import { api, AIEvent } from '../api/client';

interface Alert {
  id: string;
  camera_id: string;
  message: string;
  status: string;
  created_at: string;
}

interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  camera_id: string;
  condition: string;
  action: string;
  created_at: string;
}

export default function EventsPage() {
  const [events, setEvents] = useState<AIEvent[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<'events' | 'alerts' | 'rules'>('events');

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api.getEvents().then(d => { if (!cancelled) setEvents((d as any).events || []); }),
      api.listAlerts().then(d => { if (!cancelled) setAlerts((d as any).alerts || []); }).catch(() => {}),
      api.getRules().then(d => { if (!cancelled) setRules(d.rules || []); }).catch(() => {}),
    ]).catch(err => {
      if (!cancelled) setError(err instanceof Error ? err.message : 'Failed to load');
    }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, []);

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

  if (loading) return <div className="p-4 text-slate-400">Loading events...</div>;
  if (error) return <div className="p-4 text-red-400">Error: {error}</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4 border-b border-slate-800 pb-4">
        {(['events', 'alerts', 'rules'] as const).map(t => (
          <button key={t} onClick={() => setTab(t)}
            className={`text-sm font-medium pb-2 -mb-4 border-b-2 transition-colors ${
              tab === t ? 'text-indigo-400 border-indigo-400' : 'text-slate-500 border-transparent hover:text-slate-300'
            }`}>
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === 'events' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          {events.length === 0 && <p className="p-6 text-sm text-slate-500">No events recorded.</p>}
          {events.length > 0 && (
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
