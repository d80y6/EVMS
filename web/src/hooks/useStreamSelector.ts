import { useState, useCallback, useEffect } from 'react';
import { StreamType, api } from '../api/client';

interface StreamState {
  main: string;
  sub: string;
  thumbnail: string | null;
}

export function useStreamSelector(cameraId: string) {
  const [streams, setStreams] = useState<StreamState | null>(null);
  const [activeType, setActiveType] = useState<StreamType>('sub');

  const loadStreams = useCallback(async () => {
    try {
      const [main, sub] = await Promise.all([
        api.getStreamUrl(cameraId, 'main'),
        api.getStreamUrl(cameraId, 'sub'),
      ]);
      setStreams({ main, sub, thumbnail: null });
    } catch (err) {
      console.error('Failed to load stream URLs:', err);
    }
  }, [cameraId]);

  useEffect(() => {
    loadStreams();
  }, [loadStreams]);

  return { streams, activeType, setActiveType, loadStreams };
}
