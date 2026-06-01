import React, { useEffect, useRef, useState, useCallback } from 'react';
import { api, Bookmark } from '../api/client';

interface TimelineScrubberProps {
  cameraId: string;
  onSeek?: (timestamp: string) => void;
  events?: { timestamp: string; type: string }[];
}

type ZoomLevel = '1h' | '6h' | '24h';

const ZOOM_INTERVALS: Record<ZoomLevel, number> = {
  '1h': 60,
  '6h': 300,
  '24h': 900,
};

const ZOOM_DURATIONS: Record<ZoomLevel, number> = {
  '1h': 3600,
  '6h': 21600,
  '24h': 86400,
};

export default function TimelineScrubber({ cameraId, onSeek, events = [] }: TimelineScrubberProps) {
  const [zoom, setZoom] = useState<ZoomLevel>('1h');
  const [thumbnails, setThumbnails] = useState<{ timestamp: string; url: string }[]>([]);
  const [playhead, setPlayhead] = useState<number | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [showBookmarkDialog, setShowBookmarkDialog] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const scrubberRef = useRef<HTMLDivElement>(null);

  const now = new Date();
  const end = now.toISOString();
  const start = new Date(now.getTime() - ZOOM_DURATIONS[zoom] * 1000).toISOString();

  useEffect(() => {
    api.getTimeline(cameraId, start, end, ZOOM_INTERVALS[zoom])
      .then((data) => setThumbnails(data.thumbnails))
      .catch(() => setThumbnails([]));
  }, [cameraId, zoom, start, end]);

  useEffect(() => {
    api.listBookmarks(cameraId).then((data) => setBookmarks(data.bookmarks)).catch(() => {});
  }, [cameraId]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return;
      if (e.key === 'b' && !e.ctrlKey && !e.metaKey && !e.repeat) {
        setShowBookmarkDialog(true);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  const handleScrubberClick = useCallback((e: React.MouseEvent) => {
    if (!scrubberRef.current) return;
    const rect = scrubberRef.current.getBoundingClientRect();
    const x = (e.clientX - rect.left) / rect.width;
    setPlayhead(x);
    setIsDragging(false);
  }, []);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    setIsDragging(true);
    handleScrubberClick(e);
  }, [handleScrubberClick]);

  useEffect(() => {
    if (!isDragging) return;
    const handleMouseMove = (e: MouseEvent) => {
      if (!scrubberRef.current) return;
      const rect = scrubberRef.current.getBoundingClientRect();
      const x = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
      setPlayhead(x);
    };
    const handleMouseUp = () => {
      setIsDragging(false);
      if (playhead !== null && thumbnails.length > 0) {
        const idx = Math.floor(playhead * thumbnails.length);
        const ts = thumbnails[Math.min(idx, thumbnails.length - 1)]?.timestamp;
        if (ts) onSeek?.(ts);
      }
    };
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isDragging, playhead, thumbnails, onSeek]);

  const eventPositions = events.map((ev) => {
    const evTime = new Date(ev.timestamp).getTime();
    const rangeStart = new Date(start).getTime();
    const rangeEnd = new Date(end).getTime();
    const pos = (evTime - rangeStart) / (rangeEnd - rangeStart);
    return { ...ev, position: Math.max(0, Math.min(1, pos)) };
  });

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-slate-400">Timeline</h3>
        <div className="flex items-center gap-1 bg-slate-900 border border-slate-800 rounded-lg p-0.5">
          {(['1h', '6h', '24h'] as ZoomLevel[]).map((level) => (
            <button
              key={level}
              onClick={() => setZoom(level)}
              className={`px-3 py-1 text-[10px] font-medium rounded-md transition-colors ${
                zoom === level
                  ? 'bg-indigo-600 text-white'
                  : 'text-slate-500 hover:text-slate-300'
              }`}
            >
              {level}
            </button>
          ))}
        </div>
      </div>

      <div
        ref={scrubberRef}
        className="relative h-20 bg-slate-900 rounded-lg overflow-hidden border border-slate-800 cursor-pointer select-none"
        onMouseDown={handleMouseDown}
        onClick={handleScrubberClick}
      >
        <div className="absolute inset-0 flex items-stretch" ref={containerRef}>
          {thumbnails.length === 0 ? (
            <div className="flex items-center justify-center w-full text-xs text-slate-600">
              Loading thumbnails...
            </div>
          ) : (
            thumbnails.map((thumb, i) => (
              <div
                key={i}
                className="flex-1 bg-slate-800 bg-cover bg-center border-r border-slate-700/50 last:border-r-0"
                style={thumb.url ? { backgroundImage: `url(${api.getThumbnailUrl(thumb.url)})` } : undefined}
              />
            ))
          )}
        </div>

        {bookmarks.map(bm => {
          const bmTime = new Date(bm.timestamp).getTime();
          const rangeStart = new Date(start).getTime();
          const rangeEnd = new Date(end).getTime();
          const pos = (bmTime - rangeStart) / (rangeEnd - rangeStart);
          if (pos < 0 || pos > 1) return null;
          return (
            <div
              key={bm.id}
              className="absolute top-0 w-1 h-full bg-yellow-400 cursor-pointer z-10"
              style={{ left: `${pos * 100}%`, transform: 'translateX(-50%)' }}
              title={bm.label}
            />
          );
        })}

        {eventPositions.map((ev, i) => (
          <div
            key={i}
            className={`absolute top-1 w-1.5 h-1.5 rounded-full ${
              ev.type === 'vehicle' ? 'bg-red-500' : 'bg-yellow-500'
            } z-10`}
            style={{ left: `${ev.position * 100}%`, transform: 'translateX(-50%)' }}
            title={`${ev.type} at ${ev.timestamp}`}
          />
        ))}

        <div className="absolute bottom-0 left-0 right-0 h-5 bg-gradient-to-t from-slate-900/80 to-transparent pointer-events-none" />

        <div className="absolute bottom-1 left-0 right-0 px-2 flex justify-between text-[9px] text-slate-600 pointer-events-none">
          <span>{new Date(start).toLocaleTimeString()}</span>
          <span>{new Date(end).toLocaleTimeString()}</span>
        </div>

        {playhead !== null && (
          <div
            className="absolute top-0 bottom-0 w-0.5 bg-red-500 shadow-[0_0_6px_rgba(239,68,68,0.6)] z-20 pointer-events-none"
            style={{ left: `${playhead * 100}%`, transform: 'translateX(-50%)' }}
          >
            <div className="absolute -top-0.5 left-1/2 -translate-x-1/2 w-2 h-2 bg-red-500 rounded-full" />
          </div>
        )}
      </div>

      {showBookmarkDialog && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-gray-800 p-4 rounded">
            <h3 className="text-sm font-medium text-slate-200">Add Bookmark</h3>
            <input
              autoFocus
              className="w-full p-2 bg-gray-700 rounded mt-2 text-slate-200"
              placeholder="Label (optional)"
              onKeyDown={async (e) => {
                if (e.key === 'Enter') {
                  try {
                    await api.createBookmark(cameraId, new Date().toISOString(), (e.target as HTMLInputElement).value);
                  } catch (err) {
                    console.error('Failed to create bookmark:', err);
                  }
                  setShowBookmarkDialog(false);
                  const data = await api.listBookmarks(cameraId);
                  setBookmarks(data.bookmarks);
                }
                if (e.key === 'Escape') setShowBookmarkDialog(false);
              }}
            />
          </div>
        </div>
      )}
    </div>
  );
}
