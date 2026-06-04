import { useState, useEffect } from 'react';
import { api, LegalHold } from '../api/client';

export function LegalHoldPage() {
  const [holds, setHolds] = useState<LegalHold[]>([]);
  const [showDialog, setShowDialog] = useState(false);
  const [cameraId, setCameraId] = useState('');
  const [reason, setReason] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchHolds = async () => {
    try {
      const data = await api.getLegalHolds();
      setHolds(data.legal_holds || []);
      setError(null);
    } catch {
      setError('Failed to load legal holds');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchHolds(); }, []);

  const handleCreate = async () => {
    if (!cameraId || !reason) return;
    try {
      await api.createLegalHold({ camera_id: cameraId, reason, created_by: 'admin' });
      setShowDialog(false);
      setCameraId('');
      setReason('');
      await fetchHolds();
    } catch {
      setError('Failed to create legal hold');
    }
  };

  const handleRelease = async (id: string) => {
    if (!confirm('Are you sure you want to release this legal hold?')) return;
    try {
      await api.releaseLegalHold(id);
      await fetchHolds();
    } catch {
      setError('Failed to release legal hold');
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading legal holds...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Legal Holds</h2>
        <button onClick={() => setShowDialog(true)}
          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">New Legal Hold</button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {holds.length === 0 && <p className="p-6 text-sm text-slate-500">No legal holds.</p>}
        {holds.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Camera ID</th><th className="p-3">Reason</th><th className="p-3">Created By</th><th className="p-3">Created At</th><th className="p-3">Status</th><th className="p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {holds.map(hold => (
                <tr key={hold.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300">{hold.camera_id}</td>
                  <td className="p-3 text-slate-300">{hold.reason}</td>
                  <td className="p-3 text-slate-300">{hold.created_by}</td>
                  <td className="p-3 text-slate-300">{new Date(hold.created_at).toLocaleString()}</td>
                  <td className="p-3">
                    <span className={`px-2 py-0.5 rounded text-xs ${hold.released_at ? 'bg-slate-700 text-slate-400' : 'bg-red-900/40 text-red-400'}`}>
                      {hold.released_at ? 'Released' : 'Active'}
                    </span>
                  </td>
                  <td className="p-3">
                    {!hold.released_at && (
                      <button onClick={() => handleRelease(hold.id)}
                        className="text-xs px-3 py-1 bg-yellow-600 hover:bg-yellow-500 text-white rounded transition-colors">Release</button>
                    )}
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
            <h3 className="text-sm font-medium text-slate-300">New Legal Hold</h3>
            <input placeholder="Camera ID" value={cameraId} onChange={e => setCameraId(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            <textarea placeholder="Reason for legal hold" value={reason} onChange={e => setReason(e.target.value)} rows={3}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowDialog(false)}
                className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Cancel</button>
              <button onClick={handleCreate} disabled={!cameraId || !reason}
                className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
