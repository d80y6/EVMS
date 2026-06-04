import { useState, useEffect } from 'react';
import { api, Camera } from '../api/client';

export default function AnalyticsPage() {
  const [tab, setTab] = useState<'people' | 'facial' | 'heatmap'>('people');
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [peopleCounts, setPeopleCounts] = useState<{ camera_id: string; zone_id: string; count: number }[]>([]);
  const [faceResults, setFaceResults] = useState<any[]>([]);
  const [faceName, setFaceName] = useState('');
  const [heatmapData, setHeatmapData] = useState<any[]>([]);
  const [heatmapCamera, setHeatmapCamera] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listCameras().then(setCameras).catch(() => {});
    api.getPeopleCounts().then(d => { setPeopleCounts(d.counts || []); setError(null); }).catch(() => setError('Failed to load people counts')).finally(() => setLoading(false));
  }, []);

  const handleFaceSearch = async () => {
    try {
      const data = await api.getFacialDetections({ name: faceName || undefined, limit: 50 });
      setFaceResults(data.results || []);
    } catch {
      setError('Failed to search faces');
    }
  };

  const handleLoadHeatmap = async () => {
    if (!heatmapCamera) return;
    try {
      const data = await api.getHeatmap(heatmapCamera);
      setHeatmapData(data.cells || []);
    } catch {
      setError('Failed to load heatmap');
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading analytics...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4 border-b border-slate-800 pb-4">
        {(['people', 'facial', 'heatmap'] as const).map(t => (
          <button key={t} onClick={() => setTab(t)}
            className={`text-sm font-medium pb-2 -mb-4 border-b-2 transition-colors ${tab === t ? 'text-indigo-400 border-indigo-400' : 'text-slate-500 border-transparent hover:text-slate-300'}`}>
            {t === 'people' ? 'People Counting' : t === 'facial' ? 'Facial Detection' : 'Heatmap'}
          </button>
        ))}
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {tab === 'people' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          {peopleCounts.length === 0 && <p className="p-6 text-sm text-slate-500">No people count data.</p>}
          {peopleCounts.length > 0 && (
            <table className="w-full text-sm">
              <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Camera</th><th className="p-3">Zone</th><th className="p-3">Count</th>
              </tr></thead>
              <tbody>
                {peopleCounts.map((pc, i) => (
                  <tr key={i} className="border-b border-slate-800 hover:bg-slate-800/50">
                    <td className="p-3 text-slate-300">{pc.camera_id}</td>
                    <td className="p-3 text-slate-300">{pc.zone_id}</td>
                    <td className="p-3 text-slate-300 font-bold">{pc.count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === 'facial' && (
        <div className="space-y-4">
          <div className="flex gap-3">
            <input type="text" placeholder="Search by name" value={faceName} onChange={e => setFaceName(e.target.value)}
              className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300 flex-1" />
            <button onClick={handleFaceSearch}
              className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-sm rounded-lg transition-colors">Search</button>
          </div>
          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            {faceResults.length === 0 && <p className="p-6 text-sm text-slate-500">No facial detection results.</p>}
            {faceResults.length > 0 && (
              <table className="w-full text-sm">
                <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                  <th className="p-3">Time</th><th className="p-3">Camera</th><th className="p-3">Name</th><th className="p-3">Confidence</th>
                </tr></thead>
                <tbody>
                  {faceResults.map((r: any) => (
                    <tr key={r.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                      <td className="p-3 text-slate-300">{new Date(r.event_time).toLocaleString()}</td>
                      <td className="p-3 text-slate-300">{r.camera_id}</td>
                      <td className="p-3 text-slate-300">{r.name || 'Unknown'}</td>
                      <td className="p-3 text-slate-300">{(r.confidence * 100).toFixed(0)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}

      {tab === 'heatmap' && (
        <div className="space-y-4">
          <div className="flex gap-3">
            <select value={heatmapCamera} onChange={e => setHeatmapCamera(e.target.value)}
              className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300 flex-1">
              <option value="">Select camera</option>
              {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
            <button onClick={handleLoadHeatmap} disabled={!heatmapCamera}
              className="px-4 py-1.5 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-sm rounded-lg transition-colors">Load</button>
          </div>
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">
            {heatmapData.length === 0 && <p className="text-sm text-slate-500">No heatmap data. Select a camera and click Load.</p>}
            {heatmapData.length > 0 && (
              <div className="grid gap-1" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(20px, 1fr))' }}>
                {heatmapData.map((cell: any, i: number) => (
                  <div key={i} className="aspect-square rounded" style={{ backgroundColor: `rgba(99, 102, 241, ${cell.value || 0})` }} title={`${cell.x},${cell.y}: ${cell.value}`} />
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
