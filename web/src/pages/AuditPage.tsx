import { useState, useEffect } from 'react';
import { api } from '../api/client';

export default function AuditPage() {
  const [entries, setEntries] = useState<{ id: string; action: string; actor: string; timestamp: string; hash: string; previous_hash: string }[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [verifyResult, setVerifyResult] = useState<{ valid: boolean; count: number; first_hash: string; last_hash: string } | null>(null);
  const [verifying, setVerifying] = useState(false);

  const fetchChain = async () => {
    try {
      const data = await api.getAuditChain();
      setEntries(data.entries || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load audit chain');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchChain(); }, []);

  const handleVerify = async () => {
    setVerifying(true);
    try {
      const result = await api.verifyAudit();
      setVerifyResult(result);
    } catch {
      setError('Failed to verify audit chain');
    } finally {
      setVerifying(false);
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading audit chain...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Audit Chain</h2>
        <button onClick={handleVerify} disabled={verifying}
          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">
          {verifying ? 'Verifying...' : 'Verify Integrity'}
        </button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {verifyResult && (
        <div className={`rounded-xl p-4 ${verifyResult.valid ? 'bg-green-900/20 border border-green-800' : 'bg-red-900/20 border border-red-800'}`}>
          <p className={`text-sm font-medium ${verifyResult.valid ? 'text-green-400' : 'text-red-400'}`}>
            Chain integrity: {verifyResult.valid ? 'VALID' : 'INVALID'}
          </p>
          <p className="text-xs text-slate-400 mt-1">{verifyResult.count} entries | First hash: {verifyResult.first_hash.slice(0, 16)}... | Last hash: {verifyResult.last_hash.slice(0, 16)}...</p>
        </div>
      )}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {entries.length === 0 && <p className="p-6 text-sm text-slate-500">No audit entries.</p>}
        {entries.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Action</th><th className="p-3">Actor</th><th className="p-3">Timestamp</th><th className="p-3">Hash</th>
              </tr>
            </thead>
            <tbody>
              {entries.map(e => (
                <tr key={e.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300">{e.action}</td>
                  <td className="p-3 text-slate-300">{e.actor}</td>
                  <td className="p-3 text-slate-300 text-xs">{new Date(e.timestamp).toLocaleString()}</td>
                  <td className="p-3 text-slate-400 font-mono text-[10px]">{e.hash.slice(0, 16)}...</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
