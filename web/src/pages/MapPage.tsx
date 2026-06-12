import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import MapView from '../components/MapView';
import { useMapCameras } from '../hooks/useMapCameras';
import { useAuth } from '../context/AuthContext';

export default function MapPage() {
  const navigate = useNavigate();
  const { role } = useAuth();
  const { positions, savePosition, reload } = useMapCameras();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const canEdit = role === 'admin' || role === 'operator';

  useEffect(() => {
    reload().then(() => setLoading(false)).catch((err) => { setError(err.message); setLoading(false); });
  }, [reload]);

  if (loading) return <div className="p-4 text-slate-400">Loading camera map...</div>;
  if (error) return <div className="p-4 text-red-400">Error: {error}</div>;

  return (
    <div className="h-full p-4">
      <h2 className="text-lg font-semibold text-slate-200 mb-4">Camera Map</h2>
      <div className="h-[calc(100vh-8rem)]">
        <MapView
          positions={positions}
          onCameraClick={(id) => navigate(`/cameras?id=${id}`)}
          onPositionChange={canEdit ? savePosition : () => {}}
        />
      </div>
    </div>
  );
}
