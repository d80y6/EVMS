import { useState, useEffect, useRef, useCallback, type ComponentType } from 'react';
import {
  ChevronUp,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  MoveUpLeft,
  MoveUpRight,
  MoveDownLeft,
  MoveDownRight,
  Square,
  Home,
  Plus,
  Minus,
  X,
} from 'lucide-react';
import { api, Preset } from '../api/client';

interface PtzOverlayProps {
  cameraId: string;
  visible: boolean;
  onVisibilityChange?: (visible: boolean) => void;
}

const DIRECTIONS = [
  'up-left',
  'up',
  'up-right',
  'left',
  'right',
  'down-left',
  'down',
  'down-right',
] as const;

const DIR_LABELS: Record<string, ComponentType<{ className?: string }>> = {
  'up-left': MoveUpLeft,
  up: ChevronUp,
  'up-right': MoveUpRight,
  left: ChevronLeft,
  right: ChevronRight,
  'down-left': MoveDownLeft,
  down: ChevronDown,
  'down-right': MoveDownRight,
};

const SPEEDS = [
  { label: 'Slow', value: 0.3 },
  { label: 'Med', value: 0.5 },
  { label: 'Fast', value: 0.8 },
];

export default function PtzOverlay({
  cameraId,
  visible,
  onVisibilityChange,
}: PtzOverlayProps) {
  const [speed, setSpeed] = useState(0.5);
  const [zoom, setZoom] = useState(0);
  const [presets, setPresets] = useState<Preset[]>([]);
  const [showPresets, setShowPresets] = useState(false);
  const [sending, setSending] = useState<string | null>(null);

  const inactivityRef = useRef<ReturnType<typeof setTimeout>>();
  const overlayRef = useRef<HTMLDivElement>(null);

  const resetInactivity = useCallback(() => {
    if (inactivityRef.current) clearTimeout(inactivityRef.current);

    onVisibilityChange?.(true);

    inactivityRef.current = setTimeout(() => {
      onVisibilityChange?.(false);
    }, 3000);
  }, [onVisibilityChange]);

  useEffect(() => {
    return () => {
      if (inactivityRef.current) clearTimeout(inactivityRef.current);
    };
  }, []);

  useEffect(() => {
    if (visible) {
      api
        .ptzGetPresets(cameraId)
        .then((data) => setPresets(data.presets))
        .catch(() => {
          /* Non-critical */ console.debug('Failed to get presets');
        });
    }
  }, [cameraId, visible]);

  const sendCommand = useCallback(
    async (command: string) => {
      setSending(command);

      try {
        await api.ptzMove(cameraId, command, speed);
      } catch {
        /* Non-critical */ console.debug('PTZ move failed');
      } finally {
        setSending(null);
      }

      resetInactivity();
    },
    [cameraId, speed, resetInactivity]
  );

  const handleDirection = useCallback(
    (dir: string) => {
      sendCommand(dir);
    },
    [sendCommand]
  );

  const handleStop = useCallback(async () => {
    setSending('stop');

    try {
      await api.ptzStop(cameraId);
    } catch {
      /* Non-critical */ console.debug('PTZ stop failed');
    } finally {
      setSending(null);
    }

    resetInactivity();
  }, [cameraId, resetInactivity]);

  const handleZoom = useCallback(
    async (delta: number) => {
      const newZoom = Math.max(-1, Math.min(1, zoom + delta));

      setZoom(newZoom);
      setSending('zoom');

      try {
        await api.ptzZoom(cameraId, newZoom);
      } catch {
        /* Non-critical */ console.debug('PTZ zoom failed');
      } finally {
        setSending(null);
      }

      resetInactivity();
    },
    [cameraId, zoom, resetInactivity]
  );

  const handlePresetGoto = useCallback(
    async (presetToken: string) => {
      setSending(`preset-${presetToken}`);

      try {
        await api.ptzGotoPreset(cameraId, presetToken);
      } catch {
        /* Non-critical */ console.debug('PTZ goto preset failed');
      } finally {
        setSending(null);
      }

      resetInactivity();
    },
    [cameraId, resetInactivity]
  );

  const handleHome = useCallback(async () => {
    setSending('home');

    try {
      await api.ptzHome(cameraId);
    } catch {
      /* Non-critical */ console.debug('PTZ home failed');
    } finally {
      setSending(null);
    }

    resetInactivity();
  }, [cameraId, resetInactivity]);

  const handleSetPreset = useCallback(async () => {
    const name = prompt('Preset name:');

    if (!name) return;

    setSending('set-preset');

    try {
      await api.ptzSetPreset(cameraId, Date.now(), name);

      const data = await api.ptzGetPresets(cameraId);

      setPresets(data.presets);
    } catch {
      /* Non-critical */ console.debug('PTZ set preset failed');
    } finally {
      setSending(null);
    }

    resetInactivity();
  }, [cameraId, resetInactivity]);

  const handleRemovePreset = useCallback(
    async (presetToken: string) => {
      setSending(`remove-${presetToken}`);

      try {
        await api.ptzRemovePreset(cameraId, presetToken);

        const data = await api.ptzGetPresets(cameraId);

        setPresets(data.presets);
      } catch {
        /* Non-critical */ console.debug('PTZ remove preset failed');
      } finally {
        setSending(null);
      }

      resetInactivity();
    },
    [cameraId, resetInactivity]
  );

  if (!visible) return null;

  return (
    <div
      ref={overlayRef}
      className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center"
      onMouseMove={resetInactivity}
      onMouseEnter={resetInactivity}
    >
      <div className="flex items-center gap-6">
        <div>
          <div className="relative w-32 h-32">
            <div className="absolute inset-0 grid grid-cols-3 grid-rows-3">
              {DIRECTIONS.map((dir) => {
                const [row, col] = {
                  'up-left': [0, 0],
                  up: [0, 1],
                  'up-right': [0, 2],
                  left: [1, 0],
                  right: [1, 2],
                  'down-left': [2, 0],
                  down: [2, 1],
                  'down-right': [2, 2],
                }[dir];

                const Icon = DIR_LABELS[dir];

                return (
                  <button
                    key={dir}
                    onClick={() => handleDirection(dir)}
                    disabled={sending === dir}
                    aria-label={`Move ${dir}`}
                    className="flex items-center justify-center text-white/70 hover:text-white hover:bg-white/10 rounded disabled:opacity-50 transition-colors"
                    style={{
                      gridRow: row + 1,
                      gridColumn: col + 1,
                    }}
                  >
                    <Icon className="w-5 h-5" />
                  </button>
                );
              })}
            </div>

            <button
              onClick={handleStop}
              disabled={sending === 'stop'}
              aria-label="Stop movement"
              className="absolute inset-0 m-auto w-10 h-10 bg-red-500/80 hover:bg-red-500 rounded-full flex items-center justify-center text-white disabled:opacity-50 transition-colors z-10"
            >
              <Square className="w-5 h-5 fill-current" />
            </button>
          </div>

          <button
            onClick={handleHome}
            disabled={sending === 'home'}
            aria-label="Return to home position"
            className="mt-1 w-full py-0.5 text-[10px] bg-white/10 hover:bg-white/20 text-white/70 hover:text-white rounded disabled:opacity-50 transition-colors"
          >
            <span className="flex items-center justify-center gap-1">
              <Home className="w-3 h-3" />
              Home
            </span>
          </button>
        </div>

        <div className="flex flex-col items-center gap-3">
          <div className="flex items-center gap-2">
            <button
              onClick={() => handleZoom(-0.2)}
              disabled={sending === 'zoom'}
              aria-label="Zoom out"
              className="w-8 h-8 flex items-center justify-center bg-white/10 hover:bg-white/20 rounded text-white/70 hover:text-white disabled:opacity-50 transition-colors"
            >
              <Minus className="w-4 h-4" />
            </button>

            <div className="w-2 h-20 bg-white/10 rounded-full relative overflow-hidden">
              <div
                className="absolute bottom-0 w-full bg-indigo-500 transition-all duration-150"
                style={{
                  height: `${((zoom + 1) / 2) * 100}%`,
                }}
              />
            </div>

            <button
              onClick={() => handleZoom(0.2)}
              disabled={sending === 'zoom'}
              aria-label="Zoom in"
              className="w-8 h-8 flex items-center justify-center bg-white/10 hover:bg-white/20 rounded text-white/70 hover:text-white disabled:opacity-50 transition-colors"
            >
              <Plus className="w-4 h-4" />
            </button>
          </div>

          <div className="flex gap-1">
            {SPEEDS.map((s) => (
              <button
                key={s.label}
                onClick={() => setSpeed(s.value)}
                aria-pressed={Math.abs(speed - s.value) < 0.01}
                className={`px-2 py-0.5 text-[10px] rounded transition-colors ${
                  Math.abs(speed - s.value) < 0.01
                    ? 'bg-indigo-600 text-white'
                    : 'bg-white/10 text-white/60 hover:bg-white/20'
                }`}
              >
                {s.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex flex-col gap-1">
          <button
            onClick={() => setShowPresets(!showPresets)}
            className="px-2 py-1 text-[10px] bg-white/10 hover:bg-white/20 text-white/70 hover:text-white rounded transition-colors"
          >
            {showPresets ? 'Hide' : `Presets (${presets.length})`}
          </button>

          <button
            onClick={handleSetPreset}
            disabled={sending === 'set-preset'}
            className="px-2 py-0.5 text-[10px] bg-indigo-600/60 hover:bg-indigo-600 text-white rounded disabled:opacity-50 transition-colors"
          >
            + Set Preset
          </button>

          {showPresets && presets.length === 0 && (
            <span className="text-[10px] text-white/40">No presets</span>
          )}

          {showPresets &&
            presets.map((p) => (
              <div key={p.id} className="flex items-center gap-1">
                <button
                  onClick={() => handlePresetGoto(String(p.id))}
                  disabled={sending === `preset-${p.id}`}
                  className="flex-1 px-2 py-0.5 text-[10px] bg-white/10 hover:bg-white/20 text-white/70 hover:text-white rounded disabled:opacity-50 transition-colors text-left"
                >
                  {p.name || `Preset ${p.id}`}
                </button>

                <button
                  onClick={() => handleRemovePreset(String(p.id))}
                  disabled={sending === `remove-${p.id}`}
                  aria-label="Remove preset"
                  title="Remove preset"
                  className="w-5 h-5 flex items-center justify-center bg-red-500/60 hover:bg-red-500 text-white rounded text-[10px] disabled:opacity-50 transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              </div>
            ))}
        </div>
      </div>
    </div>
  );
}
