import { useEffect, useRef } from 'react';

interface TouchGesturesOptions {
  onPinchZoom?: (scale: number) => void;
  onSwipe?: (direction: 'left' | 'right' | 'up' | 'down') => void;
  onTap?: () => void;
  minSwipeDistance?: number;
}

export function useTouchGestures<T extends HTMLElement>(
  options: TouchGesturesOptions
) {
  const ref = useRef<T>(null);
  const optsRef = useRef(options);
  optsRef.current = options;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    let startX = 0;
    let startY = 0;
    let startDist = 0;
    let isPinching = false;

    const handleTouchStart = (e: TouchEvent) => {
      if (e.touches.length === 2) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        startDist = Math.hypot(dx, dy);
        isPinching = true;
      } else if (e.touches.length === 1) {
        startX = e.touches[0].clientX;
        startY = e.touches[0].clientY;
        isPinching = false;
      }
    };

    const handleTouchMove = (e: TouchEvent) => {
      const opts = optsRef.current;
      if (e.touches.length === 2 && opts.onPinchZoom) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        const dist = Math.hypot(dx, dy);
        if (startDist > 0) {
          const scale = dist / startDist;
          if (Math.abs(scale - 1) > 0.05) {
            opts.onPinchZoom(scale);
            startDist = dist;
          }
        }
        isPinching = true;
      }
    };

    const handleTouchEnd = (e: TouchEvent) => {
      const opts = optsRef.current;
      if (isPinching) {
        isPinching = false;
        return;
      }

      if (e.changedTouches.length === 1) {
        const dx = e.changedTouches[0].clientX - startX;
        const dy = e.changedTouches[0].clientY - startY;
        const minDist = opts.minSwipeDistance || 50;

        if (Math.abs(dx) > minDist || Math.abs(dy) > minDist) {
          if (Math.abs(dx) > Math.abs(dy)) {
            opts.onSwipe?.(dx > 0 ? 'right' : 'left');
          } else {
            opts.onSwipe?.(dy > 0 ? 'down' : 'up');
          }
        } else {
          opts.onTap?.();
        }
      }
    };

    el.addEventListener('touchstart', handleTouchStart, { passive: true });
    el.addEventListener('touchmove', handleTouchMove, { passive: true });
    el.addEventListener('touchend', handleTouchEnd, { passive: true });

    return () => {
      el.removeEventListener('touchstart', handleTouchStart);
      el.removeEventListener('touchmove', handleTouchMove);
      el.removeEventListener('touchend', handleTouchEnd);
    };
  }, []);

  return ref;
}
