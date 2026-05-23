import React, { useEffect, useRef, useState } from 'react';

interface CameraViewProps {
  cameraId: string;
  streamUrl: string;
}

const CameraView: React.FC<CameraViewProps> = ({ cameraId, streamUrl }) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<'connecting' | 'online' | 'offline'>('connecting');

  useEffect(() => {
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    });

    pc.ontrack = (event) => {
      if (videoRef.current) {
        videoRef.current.srcObject = event.streams[0];
      }
    };

    pc.oniceconnectionstatechange = () => {
      if (pc.iceConnectionState === 'connected') setStatus('online');
      if (pc.iceConnectionState === 'disconnected') setStatus('offline');
    };

    const startStream = async () => {
      try {
        const offer = await pc.createOffer({ offerToReceiveVideo: true });
        await pc.setLocalDescription(offer);

        const response = await fetch(`${streamUrl}/webrtc/offer?camera_id=${cameraId}`, {
          method: 'POST',
          body: JSON.stringify(pc.localDescription),
          headers: { 'Content-Type': 'application/json' }
        });

        const answer = await response.json();
        await pc.setRemoteDescription(new RTCSessionDescription(answer));
      } catch (err) {
        console.error("Failed to start WebRTC stream", err);
        setStatus('offline');
      }
    };

    startStream();

    return () => {
      pc.close();
    };
  }, [cameraId, streamUrl]);

  return (
    <div className="relative aspect-video bg-slate-900 rounded-lg overflow-hidden border border-slate-700 group">
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        className="w-full h-full object-cover"
      />

      {/* Overlay */}
      <div className="absolute top-4 left-4 flex items-center gap-2">
        <div className={`w-2 h-2 rounded-full ${status === 'online' ? 'bg-green-500 animate-pulse' : 'bg-red-500'}`} />
        <span className="text-xs font-medium text-white drop-shadow-md uppercase tracking-wider">
          {cameraId}
        </span>
      </div>

      {status === 'connecting' && (
        <div className="absolute inset-0 flex items-center justify-center bg-slate-900/50 backdrop-blur-sm">
          <span className="text-slate-400 text-sm animate-pulse">Establishing Secure Stream...</span>
        </div>
      )}
    </div>
  );
};

export default CameraView;
