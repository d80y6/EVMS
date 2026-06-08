import { useState, useEffect, useCallback } from 'react';
import { api, Camera } from '../api/client';

const ZOOM_LEVELS = ['hour', 'day', 'week'] as const;
type ZoomLevel = typeof ZOOM_LEVELS[number];

const SEGMENT_COLORS: Record<string, string> = {
  recording: 'bg-blue-500',
  event: 'bg-red-500',
  bookmark: 'bg-yellow-500',
  motion: 'bg-green-500',
};

const SEGMENT_LABELS: Record<string, string> = {
  recording: 'Recording',
  event: 'Event',
  bookmark: 'Bookmark',
  motion: 'Motion',
};

export default function TimelinePage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [segments, setSegments] = useState<any[]>([]);
  const [density, setDensity] = useState<{ timestamp: string; count: number }[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [zoom, setZoom] = useState<ZoomLevel>('day');
  const [selectedCamera, setSelectedCamera] = useState('');
  const [filterTypes, setFilterTypes] = useState<Set<string>>(new Set(['recording', 'event', 'bookmark', 'motion']));
  const [now] = useState(() => new Date());

  const getTimeRange = useCallback(() => {
    const end = new Date(now);
    const start = new Date(end);
    if (zoom === 'hour') start.setHours(start.getHours() - 1);
    else if (zoom === 'day') start.setDate(start.getDate() - 1);
    else start.setDate(start.getDate() - 7);
    return { start: start.toISOString(), end: end.toISOString() };
  }, [zoom, now]);

  useEffect(() => {
    api.listCameras().then(setCameras).catch(() => {});
  }, []);

  useEffect(() => {
    setLoading(true);
    const range = getTimeRange();
    api.getTimelineData({
      start_time: range.start,
      end_time: range.end,
      cameras: selectedCamera ? [selectedCamera] : undefined,
      zoom,
    })
      .then((data) => {
        setSegments(data.segments || []);
        setDensity(data.density || []);
        setTotal(data.total || 0);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [zoom, selectedCamera, getTimeRange]);

  const toggleFilter = (type: string) => {
    setFilterTypes((prev) => {
      const next = new Set(prev);
      if (next.has(type)) next.delete(type);
      else next.add(type);
      return next;
    });
  };

  const filteredSegments = segments.filter((s) => filterTypes.has(s.type));
  const maxDensity = Math.max(...density.map((d) => d.count), 1);

  const timeRange = getTimeRange();
  const rangeStart = new Date(timeRange.start).getTime();
  const rangeEnd = new Date(timeRange.end).getTime();
  const rangeDuration = rangeEnd - rangeStart;

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Timeline</h2>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {/* Controls */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 flex items-center gap-4 flex-wrap">
        <div className="flex items-center gap-1 bg-slate-800 rounded-lg p-0.5">
          {ZOOM_LEVELS.map((z) => (
            <button key={z} onClick={() => setZoom(z)}
              className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${
                zoom === z ? 'bg-indigo-600 text-white' : 'text-slate-400 hover:text-slate-300'
              }`}>
              {z}
            </button>
          ))}
        </div>

        <select value={selectedCamera} onChange={(e) => setSelectedCamera(e.target.value)}
          className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-300">
          <option value="">All Cameras</option>
          {cameras.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>

        <div className="flex items-center gap-2 ml-auto">
          {Object.entries(SEGMENT_LABELS).map(([type, label]) => (
            <button key={type} onClick={() => toggleFilter(type)}
              className={`flex items-center gap-1.5 text-xs px-2 py-1 rounded transition-colors ${
                filterTypes.has(type) ? 'bg-slate-700 text-slate-300' : 'bg-slate-800/50 text-slate-600'
              }`}>
              <span className={`w-2 h-2 rounded-sm ${SEGMENT_COLORS[type] || 'bg-slate-500'}`} />
              {label}
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-4">
        {/* Density Graph */}
        {density.length > 0 && (
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-4">
            <h4 className="text-xs text-slate-500 mb-2">Activity Density</h4>
            <div className="h-16 flex items-end gap-px">
              {density.map((d, i) => (
                <div key={i} className="flex-1 flex flex-col justify-end"
                  title={`${new Date(d.timestamp).toLocaleString()}: ${d.count} events`}>
                  <div
                    className="w-full bg-indigo-500/60 rounded-t transition-all"
                    style={{ height: `${(d.count / maxDensity) * 100}%` }}
                  />
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Timeline Segments */}
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-4">
          {loading ? (
            <p className="text-sm text-slate-400 py-4">Loading timeline...</p>
          ) : filteredSegments.length === 0 ? (
            <p className="text-sm text-slate-500 py-4">No timeline data for the selected period.</p>
          ) : (
            <div className="space-y-1">
              {filteredSegments.map((seg, i) => {
                const segStart = new Date(seg.start_time || seg.timestamp).getTime();
                const segEnd = seg.end_time ? new Date(seg.end_time).getTime() : segStart + 60000;
                const left = ((segStart - rangeStart) / rangeDuration) * 100;
                const width = Math.max(((segEnd - segStart) / rangeDuration) * 100, 0.5);
                return (
                  <div key={i} className="relative h-6 flex items-center group cursor-pointer hover:bg-slate-800/30 rounded">
                    <div className="w-24 text-[10px] text-slate-500 shrink-0">
                      {new Date(segStart).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                    </div>
                    <div className="flex-1 relative h-4">
                      <div
                        className={`absolute h-full rounded ${SEGMENT_COLORS[seg.type] || 'bg-slate-500'} opacity-70 group-hover:opacity-100 transition-opacity`}
                        style={{ left: `${Math.max(left, 0)}%`, width: `${Math.min(width, 100 - Math.max(left, 0))}%` }}
                      />
                    </div>
                    <div className="w-40 text-[10px] text-slate-500 ml-2 truncate">
                      {seg.camera_name || seg.camera_id}
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          {total > filteredSegments.length && (
            <p className="text-xs text-slate-600 mt-2">{total} total entries (showing {filteredSegments.length})</p>
          )}
        </div>

        {/* Legend */}
        <div className="flex items-center gap-4 text-xs text-slate-600">
          <span className="text-slate-500">Legend:</span>
          {Object.entries(SEGMENT_LABELS).map(([type, label]) => (
            <span key={type} className="flex items-center gap-1">
              <span className={`w-2 h-2 rounded-sm ${SEGMENT_COLORS[type] || 'bg-slate-500'}`} />
              {label}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
