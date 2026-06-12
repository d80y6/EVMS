import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import MapView from '../components/MapView';
import { useMapCameras } from '../hooks/useMapCameras';
import { useAuth } from '../context/AuthContext';
import { api } from '../api/client';

interface HeatmapCell {
  lat: number;
  lng: number;
  intensity: number;
}

export default function MapsPage() {
  const navigate = useNavigate();
  const { role } = useAuth();
  const { positions, savePosition, reload } = useMapCameras();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showHeatmap, setShowHeatmap] = useState(false);
  const [heatmapData, setHeatmapData] = useState<HeatmapCell[]>([]);

  const canEdit = role === 'admin' || role === 'operator';

  useEffect(() => {
    reload().then(() => setLoading(false)).catch((err) => { setError(err.message); setLoading(false); });
  }, [reload]);

  const toggleHeatmap = useCallback(async () => {
    if (showHeatmap) {
      setShowHeatmap(false);
      return;
    }
    try {
      const data = await api.getPeopleCounts();
      const cells: HeatmapCell[] = data.counts.map((c) => {
        const cam = positions.find(p => p.cameraId === c.camera_id);
        return {
          lat: cam ? cam.lat + (Math.random() - 0.5) * 0.01 : 40.7128,
          lng: cam ? cam.lng + (Math.random() - 0.5) * 0.01 : -74.006,
          intensity: c.count,
        };
      });
      setHeatmapData(cells);
      setShowHeatmap(true);
    } catch {
      // Heatmap data unavailable
    }
  }, [showHeatmap, positions]);

  if (loading) return <div className="p-4 text-slate-400">Loading camera map...</div>;
  if (error) return <div className="p-4 text-red-400">Error: {error}</div>;

  return (
    <div className="h-full p-4">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-slate-200">Enhanced Map</h2>
        <button
          onClick={toggleHeatmap}
          className={`px-3 py-1.5 rounded text-xs font-medium transition-colors ${
            showHeatmap ? 'bg-red-600 text-white' : 'bg-slate-700 text-slate-300 hover:bg-slate-600'
          }`}
        >
          {showHeatmap ? 'Hide Heatmap' : 'Show Heatmap'}
        </button>
      </div>
      <div className="h-[calc(100vh-12rem)]">
        <MapView
          positions={positions}
          onCameraClick={(id) => navigate(`/dashboard?camera=${id}`)}
          onPositionChange={canEdit ? savePosition : () => {}}
          showHeatmap={showHeatmap}
          heatmapData={heatmapData}
        />
      </div>
    </div>
  );
}
