import { useEffect, useMemo, useState } from 'react';
import {
  Download,
  RefreshCw,
  Search,
} from 'lucide-react';

import {
  api,
  Camera,
  Recording,
} from '../api/client';

export default function PlaybackPage() {
  const [loading, setLoading] =
    useState(true);

  const [searching, setSearching] =
    useState(false);

  const [cameras, setCameras] =
    useState<Camera[]>([]);

  const [recordings, setRecordings] =
    useState<Recording[]>([]);

  const [selectedCamera, setSelectedCamera] =
    useState('');

  const [selectedRecording, setSelectedRecording] =
    useState<Recording | null>(null);

  const [startDate, setStartDate] =
    useState('');

  const [endDate, setEndDate] =
    useState('');

  const [error, setError] =
    useState<string | null>(null);

  useEffect(() => {
    void loadInitial();
  }, []);

  const loadInitial =
    async () => {
      try {
        setLoading(true);

        const cams =
          await api.listCameras();

        setCameras(cams);

        if (cams.length > 0) {
          setSelectedCamera(
            cams[0].id
          );
        }

        setError(null);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Failed to load data'
        );
      } finally {
        setLoading(false);
      }
    };

  const searchRecordings =
    async () => {
      try {
        setSearching(true);

        const result =
          await api.getRecordings({
            start_time: startDate || undefined,
            end_time: endDate || undefined,
            camera_id: selectedCamera || undefined,
          });

        const items =
          result.recordings ||
          [];

        setRecordings(
          items
        );

        setError(null);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Failed to search recordings'
        );
      } finally {
        setSearching(false);
      }
    };

  const exportClip =
    async () => {
      if (
        !selectedCamera ||
        !selectedRecording
      ) {
        return;
      }

      try {
        const result =
          await api.exportRecording(
            selectedCamera,
            selectedRecording.start_time,
            selectedRecording.end_time,
            true
          );

        if (
          result?.file_path
        ) {
          window.open(
            result.file_path,
            '_blank'
          );
        }
      } catch (err) {
        console.error(
          err
        );
      }
    };

  const selectedCameraName =
    useMemo(() => {
      return (
        cameras.find(
          (c) =>
            c.id ===
            selectedCamera
        )?.name || ''
      );
    }, [
      cameras,
      selectedCamera,
    ]);

  if (loading) {
    return (
      <div className="p-6 text-slate-400">
        Loading playback...
      </div>
    );
  }

  return (
    <div className="space-y-6">

      {/* Header */}

      <div className="flex justify-between items-center">

        <div>

          <h1 className="text-2xl font-bold text-slate-200">
            Playback
          </h1>

          <p className="text-slate-500">
            Search and review
            recordings
          </p>

        </div>

        <button
          onClick={() =>
            void loadInitial()
          }
          className="px-3 py-2 bg-slate-800 hover:bg-slate-700 rounded text-slate-300 flex items-center gap-2"
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

      {/* Search Panel */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">

        <div className="grid md:grid-cols-4 gap-4">

          <div>

            <label className="block text-sm text-slate-500 mb-2">
              Camera
            </label>

            <select
              value={
                selectedCamera
              }
              onChange={(e) =>
                setSelectedCamera(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            >
              {cameras.map(
                (
                  camera
                ) => (
                  <option
                    key={
                      camera.id
                    }
                    value={
                      camera.id
                    }
                  >
                    {
                      camera.name
                    }
                  </option>
                )
              )}
            </select>

          </div>

          <div>

            <label className="block text-sm text-slate-500 mb-2">
              Start
            </label>

            <input
              type="datetime-local"
              value={
                startDate
              }
              onChange={(e) =>
                setStartDate(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            />

          </div>

          <div>

            <label className="block text-sm text-slate-500 mb-2">
              End
            </label>

            <input
              type="datetime-local"
              value={
                endDate
              }
              onChange={(e) =>
                setEndDate(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            />

          </div>

          <div className="flex items-end">

            <button
              onClick={() =>
                void searchRecordings()
              }
              disabled={
                searching
              }
              className="w-full px-4 py-2 bg-indigo-600 hover:bg-indigo-500 rounded text-white flex items-center justify-center gap-2"
            >
              <Search
                size={16}
              />
              Search
            </button>

          </div>

        </div>

      </div>

      {/* Timeline / Results */}

      <div className="grid xl:grid-cols-3 gap-6">

        <div className="xl:col-span-1">

          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">

            <div className="px-4 py-3 border-b border-slate-800">
              <h2 className="font-semibold text-slate-200">
                Recordings
              </h2>
            </div>

            <div className="max-h-[700px] overflow-auto">

              {recordings.length ===
              0 ? (
                <div className="p-6 text-slate-500">
                  No recordings
                  found
                </div>
              ) : (
                recordings.map(
                  (
                    recording
                  ) => (
                    <button
                      key={
                        `${recording.camera_id}-${recording.start_time}`
                      }
                      onClick={() =>
                        setSelectedRecording(
                          recording
                        )
                      }
                      className={`w-full text-left p-4 border-b border-slate-800 hover:bg-slate-800/40 ${
                        selectedRecording?.start_time ===
  recording.start_time &&
selectedRecording?.camera_id ===
  recording.camera_id
                          ? 'bg-indigo-900/20'
                          : ''
                      }`}
                    >
                      <div className="font-medium text-slate-200">
  {new Date(
    recording.start_time
  ).toLocaleString()}
</div>

                      <div className="text-xs text-slate-500 mt-1">
                        {
                          recording.start_time
                        }
                      </div>
                      <InfoRow
                        label="Status"
                        value={recording.file_path ? 'Recorded' : 'Pending'}
                      />

                    </button>
                  )
                )
              )}

            </div>

          </div>

        </div>

        <div className="xl:col-span-2">

          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">

            {!selectedRecording ? (
              <div className="aspect-video bg-black rounded-lg flex items-center justify-center text-slate-500">
                Select a
                recording
              </div>
            ) : (
              <>
                {selectedRecording.file_path ? (
                  <video
                    controls
                    className="w-full aspect-video bg-black rounded-lg"
                  >
                    <source
                      src={api.getPlaybackUrl(selectedRecording.file_path)}
                      type="video/mp4"
                    />
                  </video>
                ) : (
                  <div className="aspect-video bg-black rounded-lg flex items-center justify-center text-slate-500">
                    Video not available
                  </div>
                )}

                <div className="mt-6 space-y-3">

                  <InfoRow
                    label="Camera"
                    value={
                      selectedCameraName
                    }
                  />

                  <InfoRow
                    label="Start"
                    value={
                      selectedRecording.start_time
                    }
                  />

                  <InfoRow
                    label="End"
                    value={
                      selectedRecording.end_time
                    }
                  />

                  <InfoRow
                    label="Size"
                    value={
                      String(
                        selectedRecording.file_size ||
                          0
                      )
                    }
                  />

                </div>

                <div className="mt-6 flex gap-3">

                  <button
                    onClick={() =>
                      void exportClip()
                    }
                    className="px-4 py-2 bg-green-600 hover:bg-green-500 rounded text-white flex items-center gap-2"
                  >
                    <Download
                      size={16}
                    />
                    Export
                  </button>

                </div>
              </>
            )}

          </div>

        </div>

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
