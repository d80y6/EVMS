import { useEffect, useRef, useState, useCallback } from 'react';

interface CameraViewProps {
  cameraId: string;
  streamType?: string;
}

const ICE_SERVERS = [{ urls: 'stun:stun.l.google.com:19302' }];
const RECONNECT_DELAY = 5000;

export default function CameraView({ cameraId, streamType }: CameraViewProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<'connecting' | 'online' | 'offline'>('connecting');
  const [streamReady, setStreamReady] = useState(false);
  const [dewarped, setDewarped] = useState(false);
  const thumbnailUrl = `/api/thumbnails/image/${cameraId}/latest.jpg`;
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout>>();

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
      const pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });
      pcRef.current = pc;

      pc.ontrack = (event) => {
        if (videoRef.current) {
          videoRef.current.srcObject = event.streams[0];
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
      const response = await fetch(`/api/webrtc/offer?camera_id=${cameraId}${streamParam}`, {
        method: 'POST',
        body: JSON.stringify(pc.localDescription),
        headers: { 'Content-Type': 'application/json' },
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
    return () => {
      if (retryRef.current) clearTimeout(retryRef.current);
      cleanup();
    };
  }, [startStream, cleanup]);

  return (
    <div className="relative aspect-video bg-slate-900 rounded-lg overflow-hidden border border-slate-700 group">
      {!streamReady && (
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
        <div className="absolute inset-0 flex items-center justify-center bg-slate-900/70 backdrop-blur-sm">
          <span className="text-slate-500 text-sm">Stream Offline</span>
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
