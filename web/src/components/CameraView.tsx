import { useEffect, useRef, useState, useCallback } from 'react';
import { api, getCSRFToken, getAuthToken } from '../api/client';

interface CameraViewProps {
  cameraId: string;
  streamType?: string;
  cameraName?: string;
}

const RECONNECT_DELAY = 5000;

export default function CameraView({ cameraId, streamType }: CameraViewProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<'connecting' | 'online' | 'offline'>('connecting');
  const [streamReady, setStreamReady] = useState(false);
  const [dewarped, setDewarped] = useState(false);
  const [thumbnailUrl, setThumbnailUrl] = useState('');
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    const now = new Date();
    const start = new Date(now.getTime() - 3600000).toISOString();
    const end = now.toISOString();
    api.getTimeline(cameraId, start, end, 3600)
      .then((data) => {
        const valid = data.thumbnails.filter(t => t.url);
        if (valid.length > 0) {
            setThumbnailUrl(`/api${valid[valid.length - 1].url}`);
        }
      })
      .catch(() => {});
  }, [cameraId]);

  const cleanup = useCallback(() => {
    if (pcRef.current) {
      pcRef.current.close();
      pcRef.current = null;
    }
  }, []);

  const startStream = useCallback(async () => {
    cleanup();
    setStatus('connecting');

    try {
      const pc = new RTCPeerConnection();
      pcRef.current = pc;

      pc.ontrack = (event) => {
        setStreamReady(true);
        if (videoRef.current) {
          const stream = event.streams[0] || new MediaStream([event.track]);
          videoRef.current.srcObject = stream;
          videoRef.current.play().catch(() => {});
        }
      };

      pc.oniceconnectionstatechange = () => {
        if (pc.iceConnectionState === 'connected') setStatus('online');
        if (pc.iceConnectionState === 'disconnected' || pc.iceConnectionState === 'failed') {
          setStatus('offline');
          retryRef.current = setTimeout(startStream, RECONNECT_DELAY);
        }
      };

      const offer = await pc.createOffer({ offerToReceiveVideo: true });
      await pc.setLocalDescription(offer);

      const streamParam = streamType && streamType !== 'main' ? `&stream_type=${streamType}` : '';
      const token = getAuthToken();
      const csrfToken = getCSRFToken();
      const response = await fetch(`/api/webrtc/offer?camera_id=${cameraId}${streamParam}`, {
        method: 'POST',
        body: JSON.stringify(pc.localDescription),
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
          ...(csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
        },
      });

      if (!response.ok) throw new Error(`Server returned ${response.status}`);
      const answer = await response.json();
      await pc.setRemoteDescription(new RTCSessionDescription(answer));
    } catch (err) {
      console.error('WebRTC stream error:', err);
      setStatus('offline');
      retryRef.current = setTimeout(startStream, RECONNECT_DELAY);
    }
  }, [cameraId, streamType, cleanup]);

  useEffect(() => {
    startStream();
    const timeout = setTimeout(() => {
      if (pcRef.current && pcRef.current.iceConnectionState !== 'connected') {
        setStatus('offline');
      }
    }, 10000);
    return () => {
      if (retryRef.current) clearTimeout(retryRef.current);
      clearTimeout(timeout);
      cleanup();
    };
  }, [startStream, cleanup]);

  return (
    <div className="relative aspect-video bg-slate-900 rounded-lg overflow-hidden border border-slate-700 group">
      {!streamReady && !thumbnailUrl && (
        <div className="absolute inset-0 flex items-center justify-center">
          <svg className="w-16 h-16 text-slate-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
        </div>
      )}
      {!streamReady && thumbnailUrl && (
        <img
          src={thumbnailUrl}
          alt=""
          className="absolute inset-0 w-full h-full object-cover"
          onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }}
        />
      )}
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        onLoadedMetadata={() => setStreamReady(true)}
        onCanPlay={() => setStreamReady(true)}
        className={streamReady ? 'block w-full h-full object-cover' : 'invisible w-full h-full object-cover'}
      />

      <div className="absolute top-4 left-4 flex items-center gap-2">
        <div
          className={`w-2 h-2 rounded-full ${
            status === 'online'
              ? 'bg-green-500 animate-pulse'
              : status === 'connecting'
              ? 'bg-yellow-500 animate-pulse'
              : 'bg-red-500'
          }`}
        />
        <span className="text-xs font-medium text-white drop-shadow-md uppercase tracking-wider">
          {cameraId}
        </span>
      </div>

      {status === 'connecting' && (
        <div className="absolute inset-0 flex items-center justify-center bg-slate-900/50 backdrop-blur-sm">
          <span className="text-slate-400 text-sm animate-pulse">Establishing Secure Stream...</span>
        </div>
      )}

      {status === 'offline' && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-slate-900/80">
          <span className="text-slate-600 text-3xl font-bold mb-2">{cameraId.slice(0, 8)}</span>
          <span className="text-slate-500 text-xs">Waiting for video feed</span>
        </div>
      )}

      <div className="absolute top-4 right-4 flex items-center gap-2">
        <button
          onClick={() => setDewarped(!dewarped)}
          className={`px-2 py-1 text-xs font-medium rounded shadow-md transition-colors ${
            dewarped
              ? 'bg-blue-600 text-white'
              : 'bg-slate-700/80 text-slate-300 hover:bg-slate-600/80'
          }`}
        >
          Dewarp
        </button>
      </div>
    </div>
  );
}
