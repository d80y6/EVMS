import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

export interface CameraMapPosition {
  cameraId: string;
  name: string;
  status: string;
  lat: number;
  lng: number;
}

export function useMapCameras(siteId?: string) {
  const [positions, setPositions] = useState<CameraMapPosition[]>([]);

  const load = useCallback(async () => {
    const cameras = await api.listCameras(siteId);
    const withPos: CameraMapPosition[] = [];
    for (const cam of cameras) {
      let pos: { lat: number; lng: number } | null = null;
      if (cam.config) {
        try {
          const parsed = JSON.parse(cam.config);
          if (parsed.map_position) {
            pos = parsed.map_position;
          }
        } catch { /* ignore parse errors */ }
      }
      if (pos) {
        withPos.push({ cameraId: cam.id, name: cam.name, status: cam.status, lat: pos.lat, lng: pos.lng });
      }
    }
    if (withPos.length === 0 && cameras.length > 0) {
      cameras.forEach((cam, i) => {
        withPos.push({
          cameraId: cam.id,
          name: cam.name,
          status: cam.status,
          lat: 40.7128 + i * 0.01,
          lng: -74.006 + i * 0.01,
        });
      });
    }
    setPositions(withPos);
  }, [siteId]);

  const savePosition = async (cameraId: string, lat: number, lng: number) => {
    const cam = await api.getCamera(cameraId);
    let config: Record<string, unknown> = {};
    if (cam.config) {
      try { config = JSON.parse(cam.config); } catch { /* ignore parse errors */ }
    }
    config.map_position = { lat, lng };
    await api.updateCameraConfig(cameraId, config);
    await load();
  };

  useEffect(() => { load(); }, [load]);

  return { positions, savePosition, reload: load };
}
