import { useState, useEffect } from 'react';
import { api, Camera } from '../api/client';

export default function AnalyticsPage() {
  const [tab, setTab] = useState<'people' | 'facial' | 'heatmap' | 'onvif-rules'>('people');
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [peopleCounts, setPeopleCounts] = useState<{ camera_id: string; zone_id: string; count: number }[]>([]);
  const [faceResults, setFaceResults] = useState<any[]>([]);
  const [faceName, setFaceName] = useState('');
  const [heatmapData, setHeatmapData] = useState<any[]>([]);
  const [heatmapCamera, setHeatmapCamera] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // ONVIF analytics rules
  const [analyticsCamera, setAnalyticsCamera] = useState('');
  const [analyticsModules, setAnalyticsModules] = useState<any[]>([]);
  const [analyticsRules, setAnalyticsRules] = useState<any[]>([]);
  const [analyticsState, setAnalyticsState] = useState<any>(null);
  const [newRuleName, setNewRuleName] = useState('');
  const [newRuleType, setNewRuleType] = useState('');

  useEffect(() => {
    api.listCameras().then(setCameras).catch(() => {});
    api.getPeopleCounts().then(d => { setPeopleCounts(d.counts || []); setError(null); }).catch(() => setError('Failed to load people counts')).finally(() => setLoading(false));
  }, []);

  const handleFaceSearch = async () => {
    setError('Facial detection API is not currently available');
    setFaceResults([]);
  };

  useEffect(() => {
    if (tab !== 'onvif-rules' || !analyticsCamera) return;
    setLoading(true);
    Promise.all([
      api.getAnalyticsModules(analyticsCamera).then(d => setAnalyticsModules(d.modules || [])).catch(() => setAnalyticsModules([])),
      api.getAnalyticsRules(analyticsCamera).then(d => setAnalyticsRules(d.rules || [])).catch(() => setAnalyticsRules([])),
      api.getAnalyticsState(analyticsCamera).then(d => setAnalyticsState(d)).catch(() => setAnalyticsState(null)),
    ]).finally(() => setLoading(false));
  }, [tab, analyticsCamera]);

  const handleLoadAnalytics = () => {
    if (!analyticsCamera) return;
    setTab('onvif-rules');
  };

  const handleCreateRule = async () => {
    if (!newRuleName || !newRuleType) return;
    try {
      await api.createAnalyticsRule(analyticsCamera, { name: newRuleName, type: newRuleType });
      setNewRuleName('');
      setNewRuleType('');
      const d = await api.getAnalyticsRules(analyticsCamera);
      setAnalyticsRules(d.rules || []);
    } catch { setError('Failed to create rule'); }
  };

  const handleDeleteRule = async (token: string) => {
    if (!confirm('Delete this analytics rule?')) return;
    try {
      await api.deleteAnalyticsRule(analyticsCamera, token);
      setAnalyticsRules(prev => prev.filter((r: any) => r.token !== token));
    } catch { setError('Failed to delete rule'); }
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

  if (loading && tab !== 'onvif-rules') return <div className="p-4 text-slate-400">Loading analytics...</div>;

  const allTabs = ['people', 'facial', 'heatmap', 'onvif-rules'] as const;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4 border-b border-slate-800 pb-4">
        {allTabs.map(t => (
          <button key={t} onClick={() => setTab(t)}
            className={`text-sm font-medium pb-2 -mb-4 border-b-2 transition-colors ${tab === t ? 'text-indigo-400 border-indigo-400' : 'text-slate-500 border-transparent hover:text-slate-300'}`}>
            {t === 'people' ? 'People Counting' : t === 'facial' ? 'Facial Detection' : t === 'heatmap' ? 'Heatmap' : 'ONVIF Rules'}
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

      {tab === 'onvif-rules' && (
        <div className="space-y-4">
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="text-[10px] text-slate-500 block mb-1">Camera</label>
              <select value={analyticsCamera} onChange={e => setAnalyticsCamera(e.target.value)}
                className="w-full bg-slate-800 text-white rounded px-2 py-1.5 text-xs">
                <option value="">Select camera...</option>
                {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
              </select>
            </div>
            <button onClick={handleLoadAnalytics} disabled={!analyticsCamera}
              className="px-3 py-1.5 text-xs bg-indigo-600 rounded hover:bg-indigo-500 disabled:opacity-50">Load</button>
          </div>

          {loading && <p className="text-sm text-slate-400">Loading...</p>}

          {!loading && analyticsCamera && (
            <>
              {analyticsModules.length > 0 && (
                <div className="bg-slate-900 p-4 rounded-lg">
                  <h3 className="text-sm font-medium mb-2">Analytics Modules</h3>
                  <div className="space-y-1">
                    {analyticsModules.map((m: any, i: number) => (
                      <p key={i} className="text-xs text-slate-300">{m.name || m.token || `Module ${i + 1}`}</p>
                    ))}
                  </div>
                </div>
              )}

              <div className="bg-slate-900 p-4 rounded-lg space-y-3">
                <h3 className="text-sm font-medium">Create Rule</h3>
                <div className="flex gap-2">
                  <input value={newRuleName} onChange={e => setNewRuleName(e.target.value)}
                    className="flex-1 bg-slate-800 text-white rounded px-2 py-1.5 text-xs" placeholder="Rule name" />
                  <input value={newRuleType} onChange={e => setNewRuleType(e.target.value)}
                    className="flex-1 bg-slate-800 text-white rounded px-2 py-1.5 text-xs" placeholder="Rule type (e.g. CellMotion)" />
                  <button onClick={handleCreateRule} disabled={!newRuleName || !newRuleType}
                    className="px-3 py-1 text-xs bg-green-700 rounded hover:bg-green-600 disabled:opacity-50">Create</button>
                </div>
              </div>

              {analyticsRules.length > 0 && (
                <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
                  <table className="w-full text-sm">
                    <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
                      <th className="p-3">Name</th><th className="p-3">Type</th><th className="p-3">Actions</th>
                    </tr></thead>
                    <tbody>
                      {analyticsRules.map((r: any, i: number) => (
                        <tr key={r.token || i} className="border-b border-slate-800 hover:bg-slate-800/50">
                          <td className="p-3 text-slate-300 text-xs">{r.name || 'Unnamed'}</td>
                          <td className="p-3 text-slate-300 text-xs">{r.type || '-'}</td>
                          <td className="p-3">
                            <button onClick={() => handleDeleteRule(r.token)}
                              className="text-xs px-2 py-1 bg-red-800 rounded hover:bg-red-700">Delete</button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              {analyticsState && (
                <div className="bg-slate-900 p-4 rounded-lg">
                  <h3 className="text-sm font-medium mb-2">Analytics State</h3>
                  <pre className="text-xs text-slate-400 overflow-auto max-h-40">{JSON.stringify(analyticsState, null, 2)}</pre>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
