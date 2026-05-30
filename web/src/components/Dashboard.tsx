import React, { useEffect, useState } from 'react';
import { api, Camera } from '../api/client';
import CameraView from './CameraView';

const FALLBACK_CAMERAS = [
  { id: 'demo_cam', name: 'Front Entrance' },
  { id: 'parking_lot', name: 'Parking Lot' },
  { id: 'warehouse', name: 'Main Warehouse' },
];

export default function Dashboard() {
  const [cameras, setCameras] = useState<{ id: string; name: string }[]>(FALLBACK_CAMERAS);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getCameras()
      .then((data) => {
        if (data.cameras && data.cameras.length > 0) {
          setCameras(data.cameras.map((c: Camera) => ({ id: c.id, name: c.name })));
        }
        setIsLoading(false);
      })
      .catch(() => {
        setIsLoading(false);
      });
  }, []);

  return (
    <>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold text-slate-200">Live View</h2>
        {isLoading && (
          <span className="text-xs text-slate-500">Connecting to camera service...</span>
        )}
        {!isLoading && error && (
          <span className="text-xs text-amber-400">Using fallback camera list</span>
        )}
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 2xl:grid-cols-3 gap-8">
        {cameras.map((cam) => (
          <div key={cam.id} className="space-y-3">
            <CameraView cameraId={cam.id} />
            <div className="flex justify-between items-center px-1">
              <h3 className="text-sm font-bold text-slate-200">{cam.name}</h3>
              <div className="flex gap-2">
                <span className="text-[10px] px-2 py-0.5 bg-slate-800 text-slate-400 rounded-md font-bold border border-slate-700">H.264</span>
                <span className="text-[10px] px-2 py-0.5 bg-slate-800 text-slate-400 rounded-md font-bold border border-slate-700">1080P</span>
              </div>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
