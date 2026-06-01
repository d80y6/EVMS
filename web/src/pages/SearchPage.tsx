import { useState, useRef, useEffect, type FormEvent } from 'react';
import { api, Camera } from '../api/client';

interface SearchResult {
  id: string;
  camera_id: string;
  event_time: string;
  object_type: string;
  confidence: number;
  track_id: string;
  thumbnail: string;
}

interface FaceDetectionResult {
  id: string;
  camera_id: string;
  event_time: string;
  name: string;
  confidence: number;
  bounding_box: any;
  watchlisted: boolean;
}

export default function SearchPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [cameraId, setCameraId] = useState('');
  const [objectType, setObjectType] = useState('');
  const [plateText, setPlateText] = useState('');
  const [faceName, setFaceName] = useState('');
  const [faceResults, setFaceResults] = useState<FaceDetectionResult[]>([]);
  const [minConfidence, setMinConfidence] = useState(0.5);
  const [startTime, setStartTime] = useState('');
  const [endTime, setEndTime] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [total, setTotal] = useState(0);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [drawing, setDrawing] = useState(false);
  const [region, setRegion] = useState<{x1: number; y1: number; x2: number; y2: number} | null>(null);
  const drawingRef = useRef(false);
  const regionRef = useRef<{x1: number; y1: number; x2: number; y2: number} | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const handleMouseDown = (e: React.MouseEvent) => {
    if (!drawing) return;
    drawingRef.current = true;
    const rect = containerRef.current!.getBoundingClientRect();
    const x = (e.clientX - rect.left) / rect.width;
    const y = (e.clientY - rect.top) / rect.height;
    regionRef.current = { x1: x, y1: y, x2: x, y2: y };
    setRegion({ x1: x, y1: y, x2: x, y2: y });

    const handleMouseMove = (e: globalThis.MouseEvent) => {
      if (!drawingRef.current || !regionRef.current || !containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      regionRef.current = { ...regionRef.current, x2: (e.clientX - rect.left) / rect.width, y2: (e.clientY - rect.top) / rect.height };
      setRegion({ ...regionRef.current });
    };

    const handleMouseUp = () => {
      drawingRef.current = false;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  useEffect(() => {
    api.getCameras()
      .then((data) => setCameras(data.cameras || []))
      .catch(() => {});
  }, []);

  const handleSearch = async (e: FormEvent) => {
    e.preventDefault();
    setIsSearching(true);
    setHasSearched(true);
    try {
      const params: Record<string, unknown> = {};
      if (cameraId) params.camera_id = cameraId;
      if (objectType && objectType !== 'face') params.object_type = objectType;
      if (plateText) params.metadata = JSON.stringify({ plate: plateText });
      if (objectType === 'face') {
        const faceParams: any = { limit: 100 };
        if (cameraId) faceParams.camera_id = cameraId;
        if (faceName) faceParams.name = faceName;
        if (startTime) faceParams.start_time = new Date(startTime).toISOString();
        if (endTime) faceParams.end_time = new Date(endTime).toISOString();
        const data = await api.getFacialDetections(faceParams);
        setFaceResults(data.results || []);
        setTotal(data.results?.length || 0);
        setResults([]);
        return;
      }
      if (minConfidence > 0) params.min_confidence = minConfidence;
      if (startTime) params.start_time = new Date(startTime).toISOString();
      if (endTime) params.end_time = new Date(endTime).toISOString();
      if (region) params.bounding_box = `${region.x1},${region.y1},${region.x2},${region.y2}`;
      params.limit = 100;

      const data = await api.smartSearch(params as any);
      setResults(data.results);
      setTotal(data.total);
    } catch {
      setResults([]);
      setTotal(0);
    } finally {
      setIsSearching(false);
    }
  };

  return (
    <div className="max-w-5xl space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Smart Search</h2>

      <form onSubmit={handleSearch} className="bg-slate-900 border border-slate-800 rounded-xl p-6">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4">
          <div className="space-y-1.5">
            <label className="text-xs text-slate-500">Camera</label>
            <select
              value={cameraId}
              onChange={(e) => setCameraId(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            >
              <option value="">All Cameras</option>
              {cameras.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs text-slate-500">Object Type</label>
            <select
              value={objectType}
              onChange={(e) => setObjectType(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            >
              <option value="">All Objects</option>
              <option value="person">Person</option>
              <option value="vehicle">Vehicle</option>
              <option value="car">Car</option>
              <option value="truck">Truck</option>
              <option value="bicycle">Bicycle</option>
              <option value="animal">Animal</option>
              <option value="license_plate">License Plate</option>
              <option value="face">Face</option>
            </select>
          </div>

          {objectType === 'face' ? (
            <div className="space-y-1.5">
              <label className="text-xs text-slate-500">Face Name</label>
              <input
                type="text"
                value={faceName}
                onChange={(e) => setFaceName(e.target.value)}
                placeholder="e.g. John Doe"
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
          ) : (
            <div className="space-y-1.5">
              <label className="text-xs text-slate-500">Plate Text</label>
              <input
                type="text"
                value={plateText}
                onChange={(e) => setPlateText(e.target.value)}
                placeholder="e.g. ABC123"
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
          )}
          <div className="space-y-1.5">
            <label className="text-xs text-slate-500">Start Time</label>
            <input
              type="datetime-local"
              value={startTime}
              onChange={(e) => setStartTime(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs text-slate-500">End Time</label>
            <input
              type="datetime-local"
              value={endTime}
              onChange={(e) => setEndTime(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
        </div>

        <div className="flex items-center gap-4 mb-4">
          <label className="flex items-center gap-2 text-xs text-slate-500">
            <input
              type="checkbox"
              checked={drawing}
              onChange={(e) => {
                setDrawing(e.target.checked);
                if (!e.target.checked) setRegion(null);
              }}
              className="rounded bg-slate-800 border-slate-700"
            />
            Draw motion region
          </label>
          {region && (
            <span className="text-xs text-slate-600">
              Region: ({region.x1.toFixed(3)}, {region.y1.toFixed(3)}) → ({region.x2.toFixed(3)}, {region.y2.toFixed(3)})
            </span>
          )}
        </div>

        {drawing && (
          <div
            ref={containerRef}
            onMouseDown={handleMouseDown}
            className="relative w-full h-48 bg-slate-800 border-2 border-dashed border-red-500 rounded-lg mb-4 cursor-crosshair overflow-hidden"
          >
            <div className="absolute inset-0 flex items-center justify-center text-xs text-slate-600 pointer-events-none">
              Click and drag to select a region
            </div>
            {region && (
              <div
                className="absolute border-2 border-red-500 bg-red-500/20"
                style={{
                  left: `${Math.min(region.x1, region.x2) * 100}%`,
                  top: `${Math.min(region.y1, region.y2) * 100}%`,
                  width: `${Math.abs(region.x2 - region.x1) * 100}%`,
                  height: `${Math.abs(region.y2 - region.y1) * 100}%`,
                }}
              />
            )}
          </div>
        )}

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-3">
            <label className="text-xs text-slate-500">Min Confidence: {Math.round(minConfidence * 100)}%</label>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={minConfidence}
              onChange={(e) => setMinConfidence(parseFloat(e.target.value))}
              className="w-24 h-1.5 bg-slate-700 rounded-full appearance-none cursor-pointer accent-indigo-500"
            />
          </div>

          <button
            type="submit"
            disabled={isSearching}
            className="px-6 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors"
          >
            {isSearching ? 'Searching...' : 'Search'}
          </button>

          {hasSearched && (
            <span className="text-xs text-slate-500">{total} results</span>
          )}
        </div>
      </form>

      {hasSearched && results.length === 0 && faceResults.length === 0 && (
        <div className="flex items-center justify-center h-32">
          <p className="text-slate-500 text-sm">No matching events found.</p>
        </div>
      )}

      {objectType === 'face' && faceResults.length > 0 && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left pb-3 pr-4 pl-4 pt-3">Time</th>
                <th className="text-left pb-3 pr-4 pt-3">Camera</th>
                <th className="text-left pb-3 pr-4 pt-3">Name</th>
                <th className="text-left pb-3 pr-4 pt-3">Confidence</th>
                <th className="text-left pb-3 pr-4 pt-3">Watchlisted</th>
              </tr>
            </thead>
            <tbody>
              {faceResults.map((r) => (
                <tr key={r.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="py-3 pr-4 pl-4">{new Date(r.event_time).toLocaleString()}</td>
                  <td className="py-3 pr-4">{r.camera_id}</td>
                  <td className="py-3 pr-4">{r.name || 'Unknown'}</td>
                  <td className="py-3 pr-4">
                    <span className={`font-medium ${
                      r.confidence >= 0.8 ? 'text-green-400' : r.confidence >= 0.5 ? 'text-yellow-400' : 'text-slate-400'
                    }`}>
                      {(r.confidence * 100).toFixed(0)}%
                    </span>
                  </td>
                  <td className="py-3 pr-4">
                    {r.watchlisted ? (
                      <span className="text-xs text-red-400 font-medium">WATCHLISTED</span>
                    ) : (
                      <span className="text-xs text-slate-600">—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {results.length > 0 && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left pb-3 pr-4 pl-4 pt-3">Time</th>
                <th className="text-left pb-3 pr-4 pt-3">Camera</th>
                <th className="text-left pb-3 pr-4 pt-3">Object</th>
                <th className="text-left pb-3 pr-4 pt-3">Confidence</th>
                <th className="text-left pb-3 pr-4 pt-3">Track ID</th>
              </tr>
            </thead>
            <tbody>
              {results.map((r) => (
                <tr key={r.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="py-3 pr-4 pl-4">{new Date(r.event_time).toLocaleString()}</td>
                  <td className="py-3 pr-4">{r.camera_id}</td>
                  <td className="py-3 pr-4 capitalize">{r.object_type}</td>
                  <td className="py-3 pr-4">
                    <span className={`font-medium ${
                      r.confidence >= 0.8 ? 'text-green-400' : r.confidence >= 0.5 ? 'text-yellow-400' : 'text-slate-400'
                    }`}>
                      {(r.confidence * 100).toFixed(0)}%
                    </span>
                  </td>
                  <td className="py-3 pr-4">
                    <span className="text-xs text-slate-500 font-mono">{r.track_id?.slice(0, 8) || '-'}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
