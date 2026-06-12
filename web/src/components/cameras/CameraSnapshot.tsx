import { useEffect, useState } from 'react';
import {
  Camera,
  RefreshCw,
  AlertCircle,
} from 'lucide-react';

import { api } from '../../api/client';

interface CameraSnapshotProps {
  cameraId: string;

  autoRefresh?: boolean;

  refreshInterval?: number;

  className?: string;
}

export default function CameraSnapshot({
  cameraId,
  autoRefresh = false,
  refreshInterval = 30000,
  className = '',
}: CameraSnapshotProps) {
  const [loading, setLoading] =
    useState(true);

  const [error, setError] =
    useState<string | null>(null);

  const [snapshotUrl, setSnapshotUrl] =
    useState('');

  const loadSnapshot =
    async () => {
      try {
        setError(null);

        const response =
          await api.getSnapshotUri(
            cameraId
          );

        const uri =
          response.snapshot_uri;

        if (!uri) {
          throw new Error(
            'Snapshot URI not available'
          );
        }

        setSnapshotUrl(
          uri
        );
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Failed to load snapshot'
        );
      } finally {
        setLoading(false);
      }
    };

  useEffect(() => {
    void loadSnapshot();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cameraId]);

  useEffect(() => {
    if (!autoRefresh) {
      return;
    }

    const timer =
      setInterval(() => {
        void loadSnapshot();
      }, refreshInterval);

    return () =>
      clearInterval(
        timer
      );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    cameraId,
    autoRefresh,
    refreshInterval,
  ]);

  if (loading) {
    return (
      <div
        className={`aspect-video rounded-lg bg-slate-900 border border-slate-800 flex items-center justify-center ${className}`}
      >
        <RefreshCw
          size={20}
          className="animate-spin text-slate-500"
        />
      </div>
    );
  }

  if (error) {
    return (
      <div
        className={`aspect-video rounded-lg bg-slate-900 border border-slate-800 flex flex-col items-center justify-center gap-2 ${className}`}
      >
        <AlertCircle
          size={24}
          className="text-red-500"
        />

        <span className="text-xs text-red-400 text-center px-4">
          {error}
        </span>

        <button
          onClick={() =>
            void loadSnapshot()
          }
          className="text-xs px-3 py-1 bg-slate-800 hover:bg-slate-700 rounded text-slate-300"
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <div
      className={`relative aspect-video rounded-lg overflow-hidden border border-slate-800 bg-black ${className}`}
    >

      {snapshotUrl ? (
        <img
          src={snapshotUrl}
          alt="Camera Snapshot"
          className="w-full h-full object-cover"
          loading="lazy"
        />
      ) : (
        <div className="w-full h-full flex flex-col items-center justify-center text-slate-500">
          <Camera
            size={32}
          />

          <span className="mt-2 text-xs">
            No Snapshot
          </span>
        </div>
      )}

      <button
        onClick={() =>
          void loadSnapshot()
        }
        className="absolute top-2 right-2 p-2 rounded bg-black/60 hover:bg-black/80 text-white"
        title="Refresh Snapshot"
      >
        <RefreshCw
          size={14}
        />
      </button>

    </div>
  );
}
