import { useState, useEffect } from 'react';
import { api } from '../api/client';

interface Alert {
  id: string;
  camera_id: string;
  message: string;
  status: string;
  created_at: string;
}

export default function AlertsPage() {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchAlerts = async () => {
    try {
      const data = await api.listAlerts();
      setAlerts(data.alerts || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load alerts');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchAlerts(); }, []);

  const handleAck = async (id: string) => {
    try {
      await api.acknowledgeAlert(id, 'admin');
      setAlerts(prev => prev.map(a => a.id === id ? { ...a, status: 'acknowledged' } : a));
    } catch {
      setError('Failed to acknowledge alert');
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading alerts...</div>;

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Alerts</h2>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {alerts.length === 0 && <p className="p-6 text-sm text-slate-500">No alerts.</p>}
        {alerts.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Camera</th><th className="p-3">Message</th><th className="p-3">Status</th><th className="p-3">Time</th><th className="p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {alerts.map(a => (
                <tr key={a.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300">{a.camera_id}</td>
                  <td className="p-3 text-slate-300">{a.message}</td>
                  <td className="p-3"><span className={`px-2 py-0.5 rounded text-xs ${a.status === 'acknowledged' ? 'bg-green-900/40 text-green-400' : 'bg-yellow-900/40 text-yellow-400'}`}>{a.status}</span></td>
                  <td className="p-3 text-slate-300 text-xs">{new Date(a.created_at).toLocaleString()}</td>
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
    </div>
  );
}
