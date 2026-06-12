import { useState, useEffect } from 'react';
import { api, Camera } from '../api/client';

const OBJECT_CLASSES = ['person', 'vehicle', 'car', 'truck', 'bicycle', 'motorcycle', 'bus', 'animal', 'package'];
const COLORS = ['red', 'blue', 'white', 'black', 'silver', 'green', 'yellow', 'orange'];
const DIRECTIONS = ['left-to-right', 'right-to-left', 'top-to-bottom', 'bottom-to-top', 'unknown'];

export default function ForensicsPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [results, setResults] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [trackPaths, setTrackPaths] = useState<any[]>([]);
  const [selectedResult, setSelectedResult] = useState<any>(null);
  const [exportFormat, setExportFormat] = useState<'csv' | 'json'>('csv');

  // Filters
  const [selectedCameras, setSelectedCameras] = useState<string[]>([]);
  const [startTime, setStartTime] = useState('');
  const [endTime, setEndTime] = useState('');
  const [objectClasses, setObjectClasses] = useState<string[]>([]);
  const [colors, setColors] = useState<string[]>([]);
  const [direction, setDirection] = useState('');
  const [minConfidence, setMinConfidence] = useState(0.5);

  useEffect(() => {
    api.listCameras().then(setCameras).catch(() => {});
  }, []);

  const toggleArrayItem = (arr: string[], item: string, setter: (v: string[]) => void) => {
    setter(arr.includes(item) ? arr.filter((i) => i !== item) : [...arr, item]);
  };

  const handleSelectResult = async (result: any) => {
    setSelectedResult(result);
    if (result?.track_id) {
      try {
        const data = await api.getTrackPath(result.track_id);
        setTrackPaths(data.track || []);
      } catch {
        setTrackPaths([]);
      }
    } else {
      setTrackPaths([]);
    }
  };

  const handleSearch = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.forensicSearch({
        cameras: selectedCameras.length > 0 ? selectedCameras : undefined,
        start_time: startTime ? new Date(startTime).toISOString() : undefined,
        end_time: endTime ? new Date(endTime).toISOString() : undefined,
        object_classes: objectClasses.length > 0 ? objectClasses : undefined,
        colors: colors.length > 0 ? colors : undefined,
        direction: direction || undefined,
        min_confidence: minConfidence,
        limit: 100,
      });
      setResults(data.results || []);
      setTotal(data.total || 0);
      setSelectedResult(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
    } finally {
      setLoading(false);
    }
  };

  const handleExport = async () => {
    try {
      const data = await api.exportForensics({
        camera_ids: selectedCameras.length > 0 ? selectedCameras : undefined,
        start_time: startTime ? new Date(startTime).toISOString() : undefined,
        end_time: endTime ? new Date(endTime).toISOString() : undefined,
        object_classes: objectClasses.length > 0 ? objectClasses : undefined,
        colors: colors.length > 0 ? colors : undefined,
        direction: direction || undefined,
        min_confidence: minConfidence,
      }, exportFormat);
      const a = document.createElement('a');
      a.href = data.file_path || '';
      a.download = `forensics-export.${exportFormat}`;
      a.click();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Export failed');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">AI Forensics Search</h2>
        {results.length > 0 && (
          <div className="flex items-center gap-2">
            <select value={exportFormat} onChange={(e) => setExportFormat(e.target.value as 'csv' | 'json')}
              className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-300">
              <option value="csv">CSV</option>
              <option value="json">JSON</option>
            </select>
            <button onClick={handleExport}
              className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium rounded transition-colors">
              Export Results
            </button>
          </div>
        )}
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {/* Search Filters */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Start Time</label>
            <input type="datetime-local" value={startTime} onChange={(e) => setStartTime(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" />
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">End Time</label>
            <input type="datetime-local" value={endTime} onChange={(e) => setEndTime(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" />
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Direction</label>
            <select value={direction} onChange={(e) => setDirection(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
              <option value="">Any</option>
              {DIRECTIONS.map((d) => <option key={d} value={d}>{d}</option>)}
            </select>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Min Confidence: {minConfidence.toFixed(2)}</label>
            <input type="range" min="0" max="1" step="0.05" value={minConfidence} onChange={(e) => setMinConfidence(parseFloat(e.target.value))}
              className="w-full accent-indigo-500" />
          </div>
        </div>

        <div className="space-y-2">
          <label className="text-xs text-slate-500">Cameras</label>
          <div className="flex flex-wrap gap-1.5">
            {cameras.map((c) => (
              <button key={c.id} onClick={() => toggleArrayItem(selectedCameras, c.id, setSelectedCameras)}
                className={`text-xs px-2 py-1 rounded-full transition-colors ${
                  selectedCameras.includes(c.id) ? 'bg-indigo-600 text-white' : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                }`}>{c.name}</button>
            ))}
          </div>
        </div>

        <div className="space-y-2">
          <label className="text-xs text-slate-500">Object Classes</label>
          <div className="flex flex-wrap gap-1.5">
            {OBJECT_CLASSES.map((cls) => (
              <button key={cls} onClick={() => toggleArrayItem(objectClasses, cls, setObjectClasses)}
                className={`text-xs px-2 py-1 rounded-full transition-colors ${
                  objectClasses.includes(cls) ? 'bg-indigo-600 text-white' : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                }`}>{cls}</button>
            ))}
          </div>
        </div>

        <div className="space-y-2">
          <label className="text-xs text-slate-500">Colors</label>
          <div className="flex flex-wrap gap-1.5">
            {COLORS.map((col) => (
              <button key={col} onClick={() => toggleArrayItem(colors, col, setColors)}
                className={`text-xs px-2 py-1 rounded-full transition-colors ${
                  colors.includes(col) ? 'bg-indigo-600 text-white' : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                }`}>{col}</button>
            ))}
          </div>
        </div>

        <button onClick={handleSearch} disabled={loading}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-sm font-medium rounded-lg transition-colors">
          {loading ? 'Searching...' : 'Search'}
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Results Grid */}
        <div className="lg:col-span-2">
          {results.length === 0 && !loading && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-12 flex items-center justify-center">
              <p className="text-sm text-slate-500">No results. Adjust filters and search.</p>
            </div>
          )}

          {loading && <div className="p-4 text-slate-400">Searching...</div>}

          {results.length > 0 && (
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
              {results.map((r: any, i: number) => (
                <button key={r.id || i} onClick={() => handleSelectResult(r)}
                  className={`bg-slate-900 border rounded-xl overflow-hidden text-left transition-colors hover:border-indigo-600 ${
                    selectedResult?.id === r.id ? 'border-indigo-500' : 'border-slate-800'
                  }`}>
                  {r.thumbnail && (
                    <div className="aspect-video bg-slate-800 overflow-hidden">
                      <img src={r.thumbnail} alt="" className="w-full h-full object-cover" />
                    </div>
                  )}
                  {!r.thumbnail && (
                    <div className="aspect-video bg-slate-800 flex items-center justify-center">
                      <span className="text-2xl text-slate-600">📷</span>
                    </div>
                  )}
                  <div className="p-2 space-y-0.5">
                    <div className="text-xs text-slate-300 truncate">{r.camera_id || r.camera_name}</div>
                    <div className="text-[10px] text-slate-500">{r.object_class || r.object_type || 'unknown'}</div>
                    <div className="flex items-center gap-1">
                      <span className="text-[10px] text-indigo-400">{(r.confidence * 100).toFixed(0)}%</span>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}

          {total > results.length && (
            <p className="text-xs text-slate-500 mt-3">{total} total results found</p>
          )}
        </div>

        {/* Detail Panel */}
        <div className="lg:col-span-1">
          {!selectedResult && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 flex items-center justify-center h-48">
              <p className="text-xs text-slate-500">Click a result for details</p>
            </div>
          )}

          {selectedResult && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 space-y-3">
              {selectedResult.thumbnail && (
                <div className="aspect-video bg-slate-800 rounded-lg overflow-hidden">
                  <img src={selectedResult.thumbnail} alt="" className="w-full h-full object-cover" />
                </div>
              )}

              <h3 className="text-sm font-medium text-slate-300">Event Detail</h3>
              <div className="text-xs text-slate-400 space-y-1">
                <p><span className="text-slate-500">Camera:</span> {selectedResult.camera_id}</p>
                <p><span className="text-slate-500">Time:</span> {selectedResult.event_time ? new Date(selectedResult.event_time).toLocaleString() : '-'}</p>
                <p><span className="text-slate-500">Object:</span> {selectedResult.object_class || selectedResult.object_type}</p>
                <p><span className="text-slate-500">Confidence:</span> {(selectedResult.confidence * 100).toFixed(1)}%</p>
                {selectedResult.track_id && (
                  <p><span className="text-slate-500">Track ID:</span> {selectedResult.track_id}</p>
                )}
                {selectedResult.direction && (
                  <p><span className="text-slate-500">Direction:</span> {selectedResult.direction}</p>
                )}
              </div>
            </div>
          )}

          {/* Track Path Visualization */}
          {trackPaths.length > 0 && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 mt-4 space-y-3">
              <h4 className="text-sm font-medium text-slate-400">Track Path</h4>
              <div className="space-y-2">
                {trackPaths.map((path: any, i: number) => (
                  <div key={i} className="flex items-center gap-2 text-xs text-slate-400">
                    <div className="w-2 h-2 rounded-full bg-indigo-500 shrink-0" />
                    <span>{path.camera_name || path.camera_id}</span>
                    {path.timestamp && (
                      <span className="text-slate-600">{new Date(path.timestamp).toLocaleTimeString()}</span>
                    )}
                    {i < trackPaths.length - 1 && <span className="text-slate-700">→</span>}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
