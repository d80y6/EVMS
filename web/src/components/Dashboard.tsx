import React, { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { VirtuosoGrid } from 'react-virtuoso';
import { api, Camera } from '../api/client';
import CameraCard from './CameraCard';

type LayoutMode = '1x1' | '2x2' | '3x3';

const LAYOUT_COLS: Record<LayoutMode, string> = {
  '1x1': 'grid-cols-1',
  '2x2': 'grid-cols-1 xl:grid-cols-2',
  '3x3': 'grid-cols-1 xl:grid-cols-2 2xl:grid-cols-3',
};

export default function Dashboard() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [layout, setLayout] = useState<LayoutMode>('3x3');
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
  }, []);

  const filteredCameras = selectedSite
    ? cameras.filter((c) => c.site_id === selectedSite)
    : cameras;

  const displayCameras = filteredCameras.length > 0 ? filteredCameras : cameras;

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
            return (
              <div className="px-2 pb-4">
                <CameraCard
                  cameraId={cam.id}
                  name={cam.name}
                  status={cam.status}
                  ptzProtocol={cam.ptz_protocol}
                />
              </div>
            );
          }}
          listClassName={`grid ${LAYOUT_COLS[layout]} gap-0`}
        />
      </div>
    </>
  );
}
