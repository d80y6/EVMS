import { useState, useEffect } from 'react';
import { api, Camera } from '../api/client';

export default function ExportPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [cameraId, setCameraId] = useState('');
  const [startTime, setStartTime] = useState('');
  const [endTime, setEndTime] = useState('');
  const [watermark, setWatermark] = useState(true);
  const [exporting, setExporting] = useState(false);
  const [result, setResult] = useState<{ file_path: string; sha256: string; size_bytes: number } | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listCameras().then(setCameras).catch(() => {});
  }, []);

  const handleExport = async () => {
    if (!cameraId || !startTime || !endTime) return;
    setExporting(true);
    setError(null);
    setResult(null);
    try {
      const res = await api.exportRecording(cameraId, new Date(startTime).toISOString(), new Date(endTime).toISOString(), watermark);
      setResult(res);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Export failed');
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="max-w-2xl space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Export Recording</h2>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <div>
          <label className="text-xs text-slate-500 block mb-1">Camera</label>
          <select value={cameraId} onChange={e => setCameraId(e.target.value)}
            className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300">
            <option value="">Select camera</option>
            {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-xs text-slate-500 block mb-1">Start Time</label>
            <input type="datetime-local" value={startTime} onChange={e => setStartTime(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
          </div>
          <div>
            <label className="text-xs text-slate-500 block mb-1">End Time</label>
            <input type="datetime-local" value={endTime} onChange={e => setEndTime(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-400 cursor-pointer">
          <input type="checkbox" checked={watermark} onChange={e => setWatermark(e.target.checked)} className="accent-indigo-500" />
          Apply watermark
        </label>
        <button onClick={handleExport} disabled={exporting || !cameraId || !startTime || !endTime}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-sm font-medium rounded-lg transition-colors">
          {exporting ? 'Exporting...' : 'Export'}
        </button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {result && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-2">
          <h3 className="text-sm font-medium text-slate-400">Export Result</h3>
          <div className="text-xs text-slate-400 space-y-1">
            <p><span className="text-slate-500">File:</span> {result.file_path}</p>
            <p><span className="text-slate-500">Size:</span> {(result.size_bytes / 1024 / 1024).toFixed(2)} MB</p>
            <p><span className="text-slate-500">SHA256:</span> <span className="font-mono text-green-400">{result.sha256}</span></p>
          </div>
          <div className="flex gap-2 pt-2">
            <button onClick={() => { const a = document.createElement('a'); a.href = result.file_path; a.download = result.file_path.split('/').pop() || 'export.mp4'; a.click(); }}
              className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium rounded-lg transition-colors">
              Download
            </button>
            <button onClick={() => { navigator.clipboard.writeText(result.sha256); }}
              className="px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-white text-xs font-medium rounded-lg transition-colors">
              Copy SHA256
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
