import { useEffect, useRef } from 'react';
import type { useSyncPlayback } from '../hooks/useSyncPlayback';

interface SyncPlaybackViewProps {
  cameraId: string;
  cameraName: string;
  sync: ReturnType<typeof useSyncPlayback>;
}

export default function SyncPlaybackView({ cameraId, cameraName, sync }: SyncPlaybackViewProps) {
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    const unsub = sync.subscribe((s) => {
      if (!videoRef.current) return;
      if (!s.playing) {
        videoRef.current.pause();
      }
    });
    return () => { unsub(); };
  }, [sync]);

  const src = `/api/playback/${cameraId}?start=${sync.state.currentTime}`;

  return (
    <div className="border border-slate-700 rounded overflow-hidden bg-slate-900">
      <div className="text-xs text-slate-400 px-2 py-1 bg-slate-800">{cameraName}</div>
      <video
        ref={videoRef}
        key={src}
        src={src}
        className="w-full aspect-video bg-black"
        controls={false}
        autoPlay
        muted
      />
    </div>
  );
}
