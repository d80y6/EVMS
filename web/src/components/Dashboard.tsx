import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { VirtuosoGrid } from 'react-virtuoso';
import { api, Camera } from '../api/client';
import CameraCard from './CameraCard';

type LayoutMode = '1x1' | '2x2' | '3x3';

interface HeatmapCell {
  camera_id: string;
  x: number;
  y: number;
  count: number;
  bucket: string;
}

const LAYOUT_COLS: Record<LayoutMode, string> = {
  '1x1': 'grid-cols-1',
  '2x2': 'grid-cols-1 xl:grid-cols-2',
  '3x3': 'grid-cols-1 xl:grid-cols-2 2xl:grid-cols-3',
};

export default function Dashboard() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [layout, setLayout] = useState<LayoutMode>('3x3');
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [heatmapEnabled, setHeatmapEnabled] = useState(false);
  const [heatmapData, setHeatmapData] = useState<Record<string, HeatmapCell[]>>({});
  const [searchParams] = useSearchParams();
  const selectedSite = searchParams.get('site') || '';

  useEffect(() => {
    api.getCameras()
      .then((data) => {
        if (data.cameras && data.cameras.length > 0) {
          setCameras(data.cameras);
        }
        setIsLoading(false);
      })
      .catch(() => {
        setIsLoading(false);
      });

    const loadCounts = async () => {
      try {
        const data = await api.getPeopleCounts();
        const m: Record<string, number> = {};
        data.counts.forEach(c => { m[c.camera_id] = (m[c.camera_id] || 0) + c.count; });
        setCounts(m);
      } catch {}
    };
    loadCounts();
    const interval = setInterval(loadCounts, 60000);
    return () => clearInterval(interval);
  }, []);

  const fetchHeatmaps = useCallback(async (cams: Camera[]) => {
    const data: Record<string, HeatmapCell[]> = {};
    const results = await Promise.allSettled(
      cams.map(cam => api.getHeatmap(cam.id))
    );
    cams.forEach((cam, i) => {
      if (results[i].status === 'fulfilled') {
        data[cam.id] = (results[i] as PromiseFulfilledResult<{ cells: HeatmapCell[] }>).value.cells;
      }
    });
    setHeatmapData(data);
  }, []);

  const filteredCameras = selectedSite
    ? cameras.filter((c) => c.site_id === selectedSite)
    : cameras;

  const displayCameras = filteredCameras.length > 0 ? filteredCameras : cameras;

  useEffect(() => {
    if (heatmapEnabled && displayCameras.length > 0) {
      fetchHeatmaps(displayCameras);
    } else {
      setHeatmapData({});
    }
  }, [heatmapEnabled, displayCameras, fetchHeatmaps]);

  return (
    <>
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-4">
          <h2 className="text-lg font-semibold text-slate-200">Live View</h2>
          {isLoading && (
            <span className="text-xs text-slate-500">Connecting...</span>
          )}
          {filteredCameras.length > 0 && (
            <span className="text-xs text-slate-500">{filteredCameras.length} cameras</span>
          )}
        </div>
        <div className="flex items-center gap-2 bg-slate-900 border border-slate-800 rounded-lg p-1">
          <button
            onClick={() => setHeatmapEnabled(v => !v)}
            className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
              heatmapEnabled
                ? 'bg-red-600 text-white'
                : 'text-slate-400 hover:text-slate-300'
            }`}
          >
            {heatmapEnabled ? 'Heatmap ON' : 'Heatmap'}
          </button>
          {(['1x1', '2x2', '3x3'] as LayoutMode[]).map((mode) => (
            <button
              key={mode}
              onClick={() => setLayout(mode)}
              className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
                layout === mode
                  ? 'bg-indigo-600 text-white'
                  : 'text-slate-400 hover:text-slate-300'
              }`}
            >
              {mode}
            </button>
          ))}
        </div>
      </div>

      <div className="-mx-2">
        <VirtuosoGrid
          totalCount={displayCameras.length}
          overscan={200}
          itemContent={(index) => {
            const cam = displayCameras[index];
            const cells = heatmapData[cam.id];
            const maxCount = cells ? Math.max(...cells.map(c => c.count), 1) : 0;
            return (
              <div className="px-2 pb-4">
                <div className="relative">
                  {counts[cam.id] !== undefined && (
                    <span className="absolute top-2 right-2 z-10 text-xs bg-blue-700 px-1.5 py-0.5 rounded-full">
                      👤 {counts[cam.id]}
                    </span>
                  )}
                  <CameraCard
                    cameraId={cam.id}
                    name={cam.name}
                    status={cam.status}
                    ptzProtocol={cam.ptz_protocol}
                  />
                  {heatmapEnabled && cells && cells.length > 0 && (
                    <div className="absolute inset-0 z-20 pointer-events-none">
                      {cells.map((cell) => (
                        <div
                          key={`${cell.x}-${cell.y}`}
                          style={{
                            position: 'absolute',
                            left: `${cell.x * 5}%`,
                            top: `${cell.y * 5}%`,
                            width: '5%',
                            height: '5%',
                            backgroundColor: `rgba(255, 0, 0, ${cell.count / maxCount})`,
                          }}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </div>
            );
          }}
          listClassName={`grid ${LAYOUT_COLS[layout]} gap-0`}
        />
      </div>
    </>
  );
}
