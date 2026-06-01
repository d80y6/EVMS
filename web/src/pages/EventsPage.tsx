import { useEffect, useState } from 'react';
import { api, AIEvent } from '../api/client';

interface Alert {
  id: string;
  rule_id: string;
  camera_id: string;
  message: string;
  status: 'triggered' | 'acknowledged' | 'escalated' | 'resolved';
  created_at: string;
}

interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  conditions: { source: string; camera_id: string; operator: string; value: string }[];
  actions: { type: string; target: string; params: Record<string, string> }[];
  logic: string;
}

export default function EventsPage() {
  const [events, setEvents] = useState<AIEvent[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [rules, setRules] = useState<Rule[]>([]);

  useEffect(() => {
    api.getEvents()
      .then((data) => setEvents(data.events))
      .catch(() => {});
  }, []);

  useEffect(() => {
    const load = async () => {
      try {
        const data = await api.listAlerts();
        setAlerts(data.alerts as Alert[]);
      } catch {}
    };
    load();
    const interval = setInterval(load, 10000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    const load = async () => {
      try {
        const resp = await fetch('/api/rules');
        const data = await resp.json();
        setRules(data.rules || []);
      } catch {}
    };
    load();
  }, []);

  const handleAck = async (id: string) => {
    const username = localStorage.getItem('username') || 'unknown';
    await api.acknowledgeAlert(id, username);
    setAlerts(prev => prev.map(a => a.id === id ? { ...a, status: 'acknowledged' as const } : a));
  };

  const confidenceColor = (c: number) => {
    if (c >= 0.8) return 'text-green-400';
    if (c >= 0.5) return 'text-yellow-400';
    return 'text-slate-400';
  };

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200 mb-4">Events</h2>

      {alerts.length > 0 && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <div className="px-4 py-2 bg-slate-800/50 border-b border-slate-700">
            <h3 className="text-sm font-medium text-slate-300">Active Alerts</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                  <th className="text-left pb-3 pr-4 px-4 pt-3">Message</th>
                  <th className="text-left pb-3 pr-4">Status</th>
                  <th className="text-left pb-3 pr-4">Time</th>
                  <th className="text-left pb-3 pr-4">Actions</th>
                </tr>
              </thead>
              <tbody>
                {alerts.map(alert => (
                  <tr key={alert.id} className={`border-b border-slate-800/50 text-slate-300 ${alert.status === 'escalated' ? 'bg-red-900/20' : ''}`}>
                    <td className="py-3 pr-4 px-4">{alert.message}</td>
                    <td className="py-3 pr-4">
                      <span className={`px-2 py-0.5 rounded text-xs font-medium ${
                        alert.status === 'triggered' ? 'bg-yellow-600/30 text-yellow-400' :
                        alert.status === 'escalated' ? 'bg-red-600/30 text-red-400 animate-pulse' :
                        'bg-green-600/30 text-green-400'
                      }`}>{alert.status}</span>
                    </td>
                    <td className="py-3 pr-4 text-xs">{new Date(alert.created_at).toLocaleString()}</td>
                    <td className="py-3 pr-4">
                      {alert.status === 'triggered' && (
                        <button onClick={() => handleAck(alert.id)} className="bg-indigo-600 hover:bg-indigo-500 px-2 py-1 rounded text-xs font-medium transition-colors">
                          Acknowledge
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {rules.length > 0 && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <div className="px-4 py-2 bg-slate-800/50 border-b border-slate-700">
            <h3 className="text-sm font-medium text-slate-300">Rules ({rules.filter(r => r.enabled).length} active)</h3>
          </div>
          <div className="divide-y divide-slate-800">
            {rules.map(rule => (
              <div key={rule.id} className="px-4 py-3 flex items-center justify-between">
                <div className="text-sm">
                  <span className="text-slate-300 font-medium">{rule.name}</span>
                  <span className="text-xs text-slate-500 ml-2">
                    IF {rule.conditions.map(c => `${c.source} ${c.operator} ${c.value}`).join(` ${rule.logic} `)}
                    → {rule.actions.map(a => a.type).join(', ')}
                  </span>
                </div>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input type="checkbox" checked={rule.enabled} readOnly
                         className="sr-only peer" />
                  <div className="w-9 h-5 bg-slate-600 rounded-full peer peer-checked:bg-indigo-600 after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full" />
                </label>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <div className="px-4 py-2 bg-slate-800/50 border-b border-slate-700">
          <h3 className="text-sm font-medium text-slate-300">AI Events</h3>
        </div>
        {events.length === 0 ? (
          <div className="flex items-center justify-center h-32">
            <p className="text-slate-500">No AI events detected yet.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                  <th className="text-left pb-3 pr-4 px-4 pt-3">Time</th>
                  <th className="text-left pb-3 pr-4">Camera</th>
                  <th className="text-left pb-3 pr-4">Object</th>
                  <th className="text-left pb-3 pr-4">Confidence</th>
                </tr>
              </thead>
              <tbody>
                {events.map((ev, i) => (
                  <tr key={i} className="border-b border-slate-800/50 text-slate-300">
                    <td className="py-3 pr-4 px-4">{new Date(ev.event_time).toLocaleString()}</td>
                    <td className="py-3 pr-4">{ev.camera_id}</td>
                    <td className="py-3 pr-4 capitalize">{ev.object_type}</td>
                    <td className={`py-3 pr-4 font-medium ${confidenceColor(ev.confidence)}`}>
                      {(ev.confidence * 100).toFixed(0)}%
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
