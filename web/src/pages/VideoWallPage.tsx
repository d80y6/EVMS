import { useEffect, useMemo, useState } from 'react';
import {
  LayoutGrid,
  RefreshCw,
} from 'lucide-react';

import { api, Camera } from '../api/client';
import CameraView from '../components/CameraView';

type LayoutMode =
  | '1x1'
  | '2x2'
  | '3x3'
  | '4x4'
  | '6x6';

const LAYOUT_SIZES = {
  '1x1': 1,
  '2x2': 4,
  '3x3': 9,
  '4x4': 16,
  '6x6': 36,
};

export default function VideoWallPage() {
  const [loading, setLoading] =
    useState(true);

  const [error, setError] =
    useState<string | null>(null);

  const [cameras, setCameras] =
    useState<Camera[]>([]);

  const [layout, setLayout] =
    useState<LayoutMode>('2x2');

  const [selectedIds, setSelectedIds] =
    useState<string[]>([]);

  const loadData = async () => {
    try {
      setLoading(true);

      const data =
        await api.listCameras();

      const online =
        data.filter(
          (camera) =>
            camera.status ===
            'online'
        );

      setCameras(online);

      if (
        selectedIds.length === 0
      ) {
        setSelectedIds(
          online
            .slice(0, 16)
            .map(
              (
                camera
              ) =>
                camera.id
            )
        );
      }

      setError(null);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to load cameras'
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const displayedCameras =
    useMemo(() => {
      const max =
        LAYOUT_SIZES[
          layout
        ];

      return cameras
        .filter((camera) =>
          selectedIds.includes(
            camera.id
          )
        )
        .slice(0, max);
    }, [
      cameras,
      selectedIds,
      layout,
    ]);

  const gridClass =
    useMemo(() => {
      switch (layout) {
        case '1x1':
          return 'grid-cols-1';

        case '2x2':
          return 'grid-cols-2';

        case '3x3':
          return 'grid-cols-3';

        case '4x4':
          return 'grid-cols-4';

        case '6x6':
          return 'grid-cols-6';

        default:
          return 'grid-cols-2';
      }
    }, [layout]);

  const toggleCamera = (
    id: string
  ) => {
    setSelectedIds(
      (prev) => {
        if (
          prev.includes(id)
        ) {
          return prev.filter(
            (
              value
            ) =>
              value !==
              id
          );
        }

        return [
          ...prev,
          id,
        ];
      }
    );
  };

  if (loading) {
    return (
      <div className="p-6 text-slate-400">
        Loading video wall...
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col gap-4">

      {/* Toolbar */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 flex flex-wrap gap-3 items-center justify-between">

        <div>

          <h1 className="text-xl font-semibold text-slate-200">
            Video Wall
          </h1>

          <p className="text-sm text-slate-500">
            Multi-camera
            monitoring
          </p>

        </div>

        <div className="flex gap-2">

          <button
            onClick={() =>
              setLayout(
                '1x1'
              )
            }
            className={`px-3 py-2 rounded ${
              layout ===
              '1x1'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-800 text-slate-400'
            }`}
          >
            1×1
          </button>

          <button
            onClick={() =>
              setLayout(
                '2x2'
              )
            }
            className={`px-3 py-2 rounded ${
              layout ===
              '2x2'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-800 text-slate-400'
            }`}
          >
            <LayoutGrid
              size={16}
            />
          </button>

          <button
            onClick={() =>
              setLayout(
                '3x3'
              )
            }
            className={`px-3 py-2 rounded ${
              layout ===
              '3x3'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-800 text-slate-400'
            }`}
          >
            <LayoutGrid
              size={16}
            />
          </button>

          <button
            onClick={() =>
              setLayout(
                '4x4'
              )
            }
            className={`px-3 py-2 rounded ${
              layout ===
              '4x4'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-800 text-slate-400'
            }`}
          >
            4×4
          </button>

          <button
            onClick={() =>
              setLayout(
                '6x6'
              )
            }
            className={`px-3 py-2 rounded ${
              layout ===
              '6x6'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-800 text-slate-400'
            }`}
          >
            <LayoutGrid
              size={16}
            />
          </button>

          <button
            onClick={() =>
              void loadData()
            }
            className="px-3 py-2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300"
          >
            <RefreshCw
              size={16}
            />
          </button>

        </div>

      </div>

      {error && (
        <div className="border border-red-800 bg-red-950/20 rounded-xl p-4 text-red-400">
          {error}
        </div>
      )}

      {/* Camera Selector */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 max-h-48 overflow-auto">

        <div className="grid md:grid-cols-3 xl:grid-cols-4 gap-2">

          {cameras.map(
            (
              camera
            ) => (
              <label
                key={
                  camera.id
                }
                className="flex items-center gap-2 text-sm text-slate-300"
              >

                <input
                  type="checkbox"
                  checked={selectedIds.includes(
                    camera.id
                  )}
                  onChange={() =>
                    toggleCamera(
                      camera.id
                    )
                  }
                />

                {
                  camera.name
                }

              </label>
            )
          )}

        </div>

      </div>

      {/* Wall */}

      <div className="flex-1 min-h-0">

        <div
          className={`grid gap-2 h-full ${gridClass}`}
        >

          {displayedCameras.map(
            (
              camera
            ) => (
              <div
                key={
                  camera.id
                }
                className="bg-slate-900 border border-slate-800 rounded-lg overflow-hidden flex flex-col"
              >

                <div className="px-3 py-2 border-b border-slate-800 flex justify-between">

                  <span className="text-sm text-slate-200">
                    {
                      camera.name
                    }
                  </span>

                  <span className="text-xs text-green-400">
                    ONLINE
                  </span>

                </div>

                <div className="flex-1 bg-black">

                  <CameraView
                    cameraId={
                      camera.id
                    }
                    streamType="sub"
                  />

                </div>

              </div>
            )
          )}

        </div>

      </div>

    </div>
  );
}
