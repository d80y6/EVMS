import { useState, useCallback, useRef, useEffect } from 'react';

export interface SyncState {
  playing: boolean;
  speed: number;
  currentTime: number;
}

export function useSyncPlayback() {
  const [state, setState] = useState<SyncState>({ playing: false, speed: 1, currentTime: Date.now() });
  const listenersRef = useRef<Set<(s: SyncState) => void>>(new Set());
  const intervalRef = useRef<number | null>(null);

  const subscribe = useCallback((listener: (s: SyncState) => void) => {
    listenersRef.current.add(listener);
    return () => listenersRef.current.delete(listener);
  }, []);

  const broadcast = useCallback((newState: SyncState) => {
    listenersRef.current.forEach(l => l(newState));
  }, []);

  const play = useCallback((startTime: number) => {
    setState({ playing: true, speed: 1, currentTime: startTime });
  }, []);

  const pause = useCallback(() => {
    setState(s => ({ ...s, playing: false }));
  }, []);

  const seek = useCallback((t: number) => {
    setState(s => ({ ...s, currentTime: t }));
  }, []);

  const setSpeed = useCallback((speed: number) => {
    setState(s => ({ ...s, speed }));
  }, []);

  useEffect(() => {
    if (!state.playing) {
      if (intervalRef.current) clearInterval(intervalRef.current);
      return;
    }
    let lastTick = performance.now();
    intervalRef.current = window.setInterval(() => {
      const now = performance.now();
      const elapsed = now - lastTick;
      lastTick = now;
      setState(s => {
        const next = { ...s, currentTime: s.currentTime + elapsed * s.speed };
        broadcast(next);
        return next;
      });
    }, 33);
    return () => { if (intervalRef.current) clearInterval(intervalRef.current); };
  }, [state.playing, state.speed, broadcast]);

  return { state, play, pause, seek, setSpeed, subscribe };
}
