import { useEffect, useMemo, useState } from 'react';
import {
  Search,
  RefreshCw,
  Camera,
  Target,
} from 'lucide-react';

import { api } from '../api/client';

interface SearchResult {
  id: string;
  camera_id: string;
  event_time: string;
  object_type: string;
  confidence: number;
  track_id: string;
  thumbnail: string;
}

export default function SearchPage() {
  const [loading, setLoading] =
    useState(false);

  const [results, setResults] =
    useState<SearchResult[]>(
      []
    );

  const [selected, setSelected] =
    useState<SearchResult | null>(
      null
    );

  const [objectType, setObjectType] =
    useState('');

  const [cameraId, setCameraId] =
    useState('');

  const [minConfidence, setMinConfidence] =
    useState(0.5);

  const [startTime, setStartTime] =
    useState('');

  const [endTime, setEndTime] =
    useState('');

  const [error, setError] =
    useState<string | null>(null);

  const [searched, setSearched] = useState(false);

  const executeSearch =
    async () => {
      try {
        setLoading(true);
        setError(null);

        const response =
          await api.smartSearch({
            camera_id:
              cameraId || undefined,
            object_type:
              objectType || undefined,
            min_confidence:
              minConfidence,
            start_time:
              startTime || undefined,
            end_time:
              endTime || undefined,
          });

        setResults(
          response.results || []
        );
        setSearched(true);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Search failed'
        );
      } finally {
        setLoading(false);
      }
    };

  useEffect(() => {
    void executeSearch();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const stats =
    useMemo(() => {
      return {
        total:
          results.length,
        uniqueCameras:
          new Set(
            results.map(
              (
                r
              ) =>
                r.camera_id
            )
          ).size,
        avgConfidence:
          results.length
            ? (
                results.reduce(
                  (
                    sum,
                    r
                  ) =>
                    sum +
                    r.confidence,
                  0
                ) /
                results.length
              ).toFixed(2)
            : '0',
      };
    }, [results]);

  return (
    <div className="space-y-6">

      {/* Header */}

      <div className="flex justify-between items-center">

        <div>

          <h1 className="text-2xl font-bold text-slate-200">
            Smart Search
          </h1>

          <p className="text-slate-500">
            Search AI detections
            and forensic events
          </p>

        </div>

        <button
          onClick={() =>
            void executeSearch()
          }
          className="px-3 py-2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 flex gap-2 items-center"
        >
          <RefreshCw
            size={16}
          />
          Refresh
        </button>

      </div>

      {error && (
        <div className="border border-red-800 bg-red-950/20 rounded-xl p-4 text-red-400">
          {error}
        </div>
      )}

      {/* Stats */}

      <div className="grid md:grid-cols-3 gap-4">

        <StatCard
          icon={Target}
          title="Results"
          value={stats.total}
        />

        <StatCard
          icon={Camera}
          title="Cameras"
          value={
            stats.uniqueCameras
          }
        />

        <StatCard
          icon={Search}
          title="Avg Confidence"
          value={
            stats.avgConfidence
          }
        />

      </div>

      {/* Filters */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">

        <div className="grid md:grid-cols-5 gap-4">

          <input
            placeholder="Object Type"
            value={
              objectType
            }
            onChange={(e) =>
              setObjectType(
                e.target.value
              )
            }
            className="bg-slate-800 border border-slate-700 rounded px-3 py-2"
          />

          <input
            placeholder="Camera ID"
            value={
              cameraId
            }
            onChange={(e) =>
              setCameraId(
                e.target.value
              )
            }
            className="bg-slate-800 border border-slate-700 rounded px-3 py-2"
          />

          <input
            type="datetime-local"
            value={
              startTime
            }
            onChange={(e) =>
              setStartTime(
                e.target.value
              )
            }
            className="bg-slate-800 border border-slate-700 rounded px-3 py-2"
          />

          <input
            type="datetime-local"
            value={
              endTime
            }
            onChange={(e) =>
              setEndTime(
                e.target.value
              )
            }
            className="bg-slate-800 border border-slate-700 rounded px-3 py-2"
          />

          <button
            onClick={() =>
              void executeSearch()
            }
            disabled={
              loading
            }
            className="bg-indigo-600 hover:bg-indigo-500 rounded px-4 py-2 text-white"
          >
            Search
          </button>

        </div>

        <div className="mt-4">

          <label className="block text-sm text-slate-500 mb-2">
            Minimum Confidence
          </label>

          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={
              minConfidence
            }
            onChange={(e) =>
              setMinConfidence(
                Number(
                  e.target.value
                )
              )
            }
            className="w-full"
          />

          <div className="text-sm text-slate-400 mt-1">
            {minConfidence}
          </div>

        </div>

      </div>

      {/* Loading indicator */}
      {loading && (
        <div className="flex items-center justify-center py-8 text-slate-400">
          <RefreshCw className="animate-spin mr-2" size={16} />
          Searching...
        </div>
      )}

      {/* Empty state */}
      {!loading && searched && results.length === 0 && (
        <div className="h-96 flex items-center justify-center text-slate-500">
          <div className="text-center space-y-2">
            <Search className="mx-auto" size={32} />
            <p>No results found. Try adjusting your search filters.</p>
          </div>
        </div>
      )}

      {/* Results */}
      {results.length > 0 && <>
      <div className="grid xl:grid-cols-3 gap-6">

        <div className="xl:col-span-1">

          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">

            <div className="px-4 py-3 border-b border-slate-800">
              Results
            </div>

            <div className="max-h-[700px] overflow-auto">

              {results.map(
                (result) => (
                  <button
                    key={
                      result.id
                    }
                    onClick={() =>
                      setSelected(
                        result
                      )
                    }
                    className={`w-full text-left p-4 border-b border-slate-800 hover:bg-slate-800/40 ${
                      selected?.id ===
                      result.id
                        ? 'bg-indigo-900/20'
                        : ''
                    }`}
                  >

                    <div className="font-medium text-slate-200">
                      {
                        result.object_type
                      }
                    </div>

                    <div className="text-xs text-slate-500 mt-1">
                      {
                        result.event_time
                      }
                    </div>

                  </button>
                )
              )}

            </div>

          </div>

        </div>

        <div className="xl:col-span-2">

          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">

            {!selected ? (
              <div className="h-96 flex items-center justify-center text-slate-500">
                Select result
              </div>
            ) : (
              <div className="space-y-4">

                {selected.thumbnail && (
                  <img
                    src={
                      selected.thumbnail
                    }
                    alt=""
                    className="w-full max-h-96 object-contain rounded-lg border border-slate-800"
                  />
                )}

                <InfoRow
                  label="Event ID"
                  value={
                    selected.id
                  }
                />

                <InfoRow
                  label="Camera"
                  value={
                    selected.camera_id
                  }
                />

                <InfoRow
                  label="Object"
                  value={
                    selected.object_type
                  }
                />

                <InfoRow
                  label="Confidence"
                  value={String(
                    selected.confidence
                  )}
                />

                <InfoRow
                  label="Track"
                  value={
                    selected.track_id
                  }
                />

                <InfoRow
                  label="Time"
                  value={
                    selected.event_time
                  }
                />

              </div>
            )}

          </div>

        </div>

      </div>
      </>}

    </div>
  );
}

function StatCard({
  icon: Icon,
  title,
  value,
}: {
  icon: any;
  title: string;
  value: string | number;
}) {
  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
      <div className="flex justify-between">
        <div>
          <div className="text-sm text-slate-500">
            {title}
          </div>
          <div className="text-2xl font-bold text-slate-200 mt-2">
            {value}
          </div>
        </div>
        <Icon className="text-indigo-400" />
      </div>
    </div>
  );
}

function InfoRow({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="flex justify-between border-b border-slate-800 py-2">
      <span className="text-slate-500">
        {label}
      </span>
      <span className="text-slate-200">
        {value}
      </span>
    </div>
  );
}
