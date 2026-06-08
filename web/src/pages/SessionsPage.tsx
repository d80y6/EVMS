import { useState, useEffect } from 'react';
import { api } from '../api/client';

export default function SessionsPage() {
  const [sessions, setSessions] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const fetchSessions = () => {
    setLoading(true);
    api.getSessions()
      .then((data) => setSessions(data.sessions || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchSessions(); }, []);

  const handleRevoke = async (sessionId: string) => {
    try {
      setError(null);
      await api.revokeSession(sessionId);
      setSessions((prev) => prev.filter((s) => s.id !== sessionId));
      setSuccess('Session revoked');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke session');
    }
  };

  const handleRevokeAll = async () => {
    if (!window.confirm('Revoke all sessions? You will be logged out of all devices.')) return;
    try {
      setError(null);
      await api.revokeAllSessions();
      setSessions([]);
      setSuccess('All sessions revoked');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke sessions');
    }
  };

  const formatUserAgent = (ua: string): string => {
    if (!ua) return 'Unknown';
    if (ua.length > 60) return ua.slice(0, 60) + '...';
    return ua;
  };

  const activeCount = sessions.filter((s) => s.active !== false).length;

  if (loading) return <div className="p-4 text-slate-400">Loading sessions...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-slate-200">Session Management</h2>
          <p className="text-xs text-slate-500 mt-1">{activeCount} active session(s)</p>
        </div>
        <button onClick={handleRevokeAll}
          className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white text-sm font-medium rounded-lg transition-colors">
          Revoke All Sessions
        </button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}
      {success && <div className="bg-green-900/20 border border-green-800 rounded-xl p-4"><p className="text-sm text-green-400">{success}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {sessions.length === 0 && <p className="p-6 text-sm text-slate-500">No sessions found.</p>}
        {sessions.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left p-3">IP Address</th>
                <th className="text-left p-3">User Agent</th>
                <th className="text-left p-3">Created</th>
                <th className="text-left p-3">Last Active</th>
                <th className="text-left p-3">Status</th>
                <th className="text-left p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((s) => (
                <tr key={s.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="p-3 font-mono text-xs">{s.ip_address || '-'}</td>
                  <td className="p-3 text-xs text-slate-400 max-w-[200px] truncate" title={s.user_agent}>
                    {formatUserAgent(s.user_agent)}
                  </td>
                  <td className="p-3 text-xs text-slate-500">
                    {s.created_at ? new Date(s.created_at).toLocaleString() : '-'}
                  </td>
                  <td className="p-3 text-xs text-slate-500">
                    {s.last_active_at ? new Date(s.last_active_at).toLocaleString() : s.expires_at ? new Date(s.expires_at).toLocaleString() : '-'}
                  </td>
                  <td className="p-3">
                    <span className={`text-xs px-2 py-0.5 rounded-full ${
                      s.active !== false ? 'bg-green-900/30 text-green-400' : 'bg-slate-700 text-slate-400'
                    }`}>
                      {s.active !== false ? 'Active' : 'Expired'}
                    </span>
                  </td>
                  <td className="p-3">
                    {s.active !== false && (
                      <button onClick={() => handleRevoke(s.id)}
                        className="text-xs px-3 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">
                        Revoke
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {activeCount > 0 && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-4">
          <p className="text-xs text-slate-500">
            Concurrent session limit: <span className="text-slate-300 font-medium">{activeCount}</span> active of{' '}
            <span className="text-slate-300 font-medium">unlimited</span> maximum
          </p>
        </div>
      )}
    </div>
  );
}
