import { useNavigate } from 'react-router-dom';
import MapView from '../components/MapView';
import { useMapCameras } from '../hooks/useMapCameras';
import { useAuth } from '../context/AuthContext';

export default function MapPage() {
  const navigate = useNavigate();
  const { role } = useAuth();
  const { positions, savePosition } = useMapCameras();

  const canEdit = role === 'admin' || role === 'operator';

  return (
    <div className="h-full p-4">
      <h1 className="text-xl font-bold mb-4">Camera Map</h1>
      <div className="h-[calc(100vh-8rem)]">
        <MapView
          positions={positions}
          onCameraClick={(id) => navigate(`/dashboard?camera=${id}`)}
          onPositionChange={canEdit ? savePosition : () => {}}
        />
      </div>
    </div>
  );
}
