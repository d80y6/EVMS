import { useState } from 'react';
import CameraView from './CameraView';
import PtzOverlay from './PtzOverlay';
import { useStreamSelector } from '../hooks/useStreamSelector';

interface CameraCardProps {
  cameraId: string;
  name: string;
  status: string;
  ptzProtocol: string;
  onClick?: (cameraId: string) => void;
}

export default function CameraCard({
  cameraId,
  name,
  status,
  ptzProtocol,
  onClick,
}: CameraCardProps) {
  const [ptzVisible, setPtzVisible] = useState(false);
  const { activeType, setActiveType } = useStreamSelector(cameraId);

  return (
    <div className="space-y-3">
        <div
          className="relative aspect-video bg-slate-900 rounded-lg overflow-hidden border border-slate-700 group cursor-pointer"
          onMouseEnter={() => setPtzVisible(true)}
          onClick={() => onClick?.(cameraId)}
        >
          <CameraView cameraId={cameraId} streamType={activeType} cameraName={name} />

        {ptzProtocol !== 'none' && (
          <PtzOverlay
            cameraId={cameraId}
            visible={ptzVisible}
            onVisibilityChange={setPtzVisible}
          />
        )}
      </div>

      <div className="flex justify-between items-center px-1">
        <div className="flex items-center gap-2">
          <div
            className={`w-1.5 h-1.5 rounded-full ${
              status === 'online'
                ? 'bg-green-500 shadow-[0_0_6px_rgba(34,197,94,0.6)]'
                : 'bg-red-500'
            }`}
          />
          <h3 className="text-sm font-bold text-slate-200">{name}</h3>
        </div>

        <div className="flex items-center gap-2">
          <span className="text-[10px] px-2 py-0.5 bg-slate-800 text-slate-400 rounded-md font-bold border border-slate-700">
            H.264
          </span>

          <span className="text-[10px] px-2 py-0.5 bg-slate-800 text-slate-400 rounded-md font-bold border border-slate-700">
            1080P
          </span>

          <button
            onClick={() =>
              setActiveType(activeType === 'main' ? 'sub' : 'main')
            }
            aria-label={
              activeType === 'main'
                ? 'Switch to standard definition'
                : 'Switch to high definition'
            }
            className={`text-[10px] px-2 py-0.5 rounded-md font-bold border transition-colors ${
              activeType === 'main'
                ? 'bg-blue-600 text-white border-blue-500'
                : 'bg-slate-800 text-slate-400 border-slate-700'
            }`}
          >
            {activeType === 'main' ? 'HD' : 'SD'}
          </button>
        </div>
      </div>
    </div>
  );
}
