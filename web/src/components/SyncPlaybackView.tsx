import { useEffect, useRef } from 'react';
import type { useSyncPlayback } from '../hooks/useSyncPlayback';
import { authUrl } from '../api/client';

interface SyncPlaybackViewProps {
  cameraId: string;
  cameraName: string;
  sync: ReturnType<typeof useSyncPlayback>;
}

export default function SyncPlaybackView({ cameraId, cameraName, sync }: SyncPlaybackViewProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const lastSrcRef = useRef<string>('');

  useEffect(() => {
    const src = authUrl(`/api/playback/${cameraId}?start=${sync.state.currentTime}`);
    if (videoRef.current && src !== lastSrcRef.current) {
      lastSrcRef.current = src;
      videoRef.current.src = src;
      if (sync.state.playing) {
        videoRef.current.play().catch(() => {});
      }
    }
  }, [cameraId, sync.state.currentTime, sync.state.playing]);

  useEffect(() => {
    const unsub = sync.subscribe((s) => {
      if (!videoRef.current) return;
      if (!s.playing) {
        videoRef.current.pause();
      }
    });
    return () => { unsub(); };
  }, [sync]);

  return (
    <div className="border border-slate-700 rounded overflow-hidden bg-slate-900">
      <div className="text-xs text-slate-400 px-2 py-1 bg-slate-800">{cameraName}</div>
      <video
        ref={videoRef}
        className="w-full aspect-video bg-black"
        controls={false}
        autoPlay
        muted
      />
    </div>
  );
}
