import { useState, useEffect } from 'react';
import { api, Bookmark } from '../api/client';

export default function BookmarksPage() {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showDialog, setShowDialog] = useState(false);
  const [newCameraId, setNewCameraId] = useState('');
  const [newTimestamp, setNewTimestamp] = useState('');
  const [newLabel, setNewLabel] = useState('');

  const fetchBookmarks = async () => {
    try {
      const data = await api.listBookmarks();
      setBookmarks(data.bookmarks || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load bookmarks');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchBookmarks(); }, []);

  const handleCreate = async () => {
    if (!newCameraId) return;
    try {
      await api.createBookmark(newCameraId, newTimestamp || new Date().toISOString(), newLabel);
      setShowDialog(false);
      setNewCameraId('');
      setNewTimestamp('');
      setNewLabel('');
      await fetchBookmarks();
    } catch {
      setError('Failed to create bookmark');
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading bookmarks...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Bookmarks</h2>
        <button onClick={() => setShowDialog(true)}
          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">+ Add Bookmark</button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {bookmarks.length === 0 && <p className="p-6 text-sm text-slate-500">No bookmarks.</p>}
        {bookmarks.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Camera</th><th className="p-3">Timestamp</th><th className="p-3">Label</th><th className="p-3">Created By</th>
              </tr>
            </thead>
            <tbody>
              {bookmarks.map(b => (
                <tr key={b.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300">{b.camera_id}</td>
                  <td className="p-3 text-slate-300">{new Date(b.timestamp).toLocaleString()}</td>
                  <td className="p-3 text-slate-300">{b.label || '-'}</td>
                  <td className="p-3 text-slate-300">{b.created_by}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showDialog && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-md space-y-4">
            <h3 className="text-sm font-medium text-slate-300">Add Bookmark</h3>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Camera ID</label>
              <input type="text" value={newCameraId} onChange={e => setNewCameraId(e.target.value)}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Timestamp</label>
              <input type="datetime-local" value={newTimestamp} onChange={e => setNewTimestamp(e.target.value)}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Label</label>
              <input type="text" value={newLabel} onChange={e => setNewLabel(e.target.value)}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowDialog(false)}
                className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Cancel</button>
              <button onClick={handleCreate} disabled={!newCameraId}
                className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
