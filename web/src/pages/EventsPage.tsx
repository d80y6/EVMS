import { useEffect, useMemo, useState } from 'react';
import {
  AlertTriangle,
  RefreshCw,
  Activity,
} from 'lucide-react';

import {
  api,
  AIEvent,
  Camera,
} from '../api/client';

export default function EventsPage() {
  const [loading, setLoading] =
    useState(true);

  const [events, setEvents] =
    useState<AIEvent[]>([]);

  const [cameras, setCameras] =
    useState<Camera[]>([]);

  const [selectedEvent, setSelectedEvent] =
    useState<AIEvent | null>(null);

  const [cameraFilter, setCameraFilter] =
    useState('');

  const [typeFilter, setTypeFilter] =
    useState('');

  const [confidenceFilter, setConfidenceFilter] =
    useState(0);

  const [error, setError] =
    useState<string | null>(null);

  const loadData = async () => {
    try {
      setLoading(true);

      const [
        eventData,
        cameraData,
      ] = await Promise.all([
        api.getEvents(),
        api.listCameras(),
      ]);

      setEvents(
        eventData.events || []
      );

      setCameras(
        cameraData
      );

      setError(null);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to load events'
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, []);

  const filteredEvents =
    useMemo(() => {
      return events.filter(
        (event) => {
          const cameraMatch =
            !cameraFilter ||
            event.camera_id ===
              cameraFilter;

          const typeMatch =
            !typeFilter ||
            event.object_type ===
              typeFilter;

          const confidenceMatch =
            event.confidence >=
            confidenceFilter;

          return (
            cameraMatch &&
            typeMatch &&
            confidenceMatch
          );
        }
      );
    }, [
      events,
      cameraFilter,
      typeFilter,
      confidenceFilter,
    ]);

  const eventTypes =
    useMemo(() => {
      return [
        ...new Set(
          events.map(
            (event) =>
              event.object_type
          )
        ),
      ];
    }, [events]);

  const stats =
    useMemo(() => {
      const total =
        events.length;

      const highConfidence =
        events.filter(
          (event) =>
            event.confidence >=
            0.8
        ).length;

      return {
        total,
        highConfidence,
      };
    }, [events]);

  if (loading) {
    return (
      <div className="p-6 text-slate-400">
        Loading events...
      </div>
    );
  }

  return (
    <div className="space-y-6">

      {/* Header */}

      <div className="flex justify-between items-center">

        <div>

          <h1 className="text-2xl font-bold text-slate-200">
            Events
          </h1>

          <p className="text-slate-500">
            AI detections and
            ONVIF events
          </p>

        </div>

        <button
          onClick={() =>
            void loadData()
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

      {/* Stats */}

      <div className="grid md:grid-cols-2 gap-4">

        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">

          <div className="flex justify-between">

            <div>

              <div className="text-slate-500 text-sm">
                Total Events
              </div>

              <div className="text-3xl font-bold text-slate-200 mt-2">
                {stats.total}
              </div>

            </div>

            <Activity className="text-indigo-400" />

          </div>

        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">

          <div className="flex justify-between">

            <div>

              <div className="text-slate-500 text-sm">
                High Confidence
              </div>

              <div className="text-3xl font-bold text-slate-200 mt-2">
                {
                  stats.highConfidence
                }
              </div>

            </div>

            <AlertTriangle className="text-yellow-400" />

          </div>

        </div>

      </div>

      {/* Filters */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">

        <div className="grid md:grid-cols-4 gap-4">

          <div>

            <label className="block text-sm text-slate-500 mb-2">
              Camera
            </label>

            <select
              value={
                cameraFilter
              }
              onChange={(e) =>
                setCameraFilter(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            >

              <option value="">
                All Cameras
              </option>

              {cameras.map(
                (camera) => (
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
              Event Type
            </label>

            <select
              value={
                typeFilter
              }
              onChange={(e) =>
                setTypeFilter(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            >

              <option value="">
                All Types
              </option>

              {eventTypes.map(
                (type) => (
                  <option
                    key={type}
                    value={type}
                  >
                    {type}
                  </option>
                )
              )}

            </select>

          </div>

          <div>

            <label className="block text-sm text-slate-500 mb-2">
              Confidence
            </label>

            <input
              type="range"
              min="0"
              max="1"
              step="0.1"
              value={
                confidenceFilter
              }
              onChange={(e) =>
                setConfidenceFilter(
                  Number(
                    e.target.value
                  )
                )
              }
              className="w-full"
            />

          </div>

          <div className="flex items-end">

            <div className="w-full bg-slate-800 rounded px-3 py-2 text-center text-slate-300">
              ≥{' '}
              {
                confidenceFilter
              }
            </div>

          </div>

        </div>

      </div>

      {/* Main Layout */}

      <div className="grid xl:grid-cols-3 gap-6">

        {/* Event List */}

        <div className="xl:col-span-1">

          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">

            <div className="px-4 py-3 border-b border-slate-800">
              <h2 className="font-semibold text-slate-200">
                Events
              </h2>
            </div>

            <div className="max-h-[700px] overflow-auto">

              {filteredEvents.map(
                (event) => (
                  <button
                    key={
                      event.id
                    }
                    onClick={() =>
                      setSelectedEvent(
                        event
                      )
                    }
                    className={`w-full text-left p-4 border-b border-slate-800 hover:bg-slate-800/40 ${
                      selectedEvent?.id ===
                      event.id
                        ? 'bg-indigo-900/20'
                        : ''
                    }`}
                  >

                    <div className="font-medium text-slate-200">
                      {
                        event.object_type
                      }
                    </div>

                    <div className="text-xs text-slate-500 mt-1">
                      {
                        event.event_time
                      }
                    </div>

                  </button>
                )
              )}

            </div>

          </div>

        </div>

        {/* Details */}

        <div className="xl:col-span-2">

          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">

            {!selectedEvent ? (
              <div className="h-96 flex items-center justify-center text-slate-500">
                Select an event
              </div>
            ) : (
              <div className="space-y-4">

                <InfoRow
                  label="ID"
                  value={
                    selectedEvent.id
                  }
                />

                <InfoRow
                  label="Camera"
                  value={
                    selectedEvent.camera_id
                  }
                />

                <InfoRow
                  label="Type"
                  value={
                    selectedEvent.object_type
                  }
                />

                <InfoRow
                  label="Confidence"
                  value={String(
                    selectedEvent.confidence
                  )}
                />

                <InfoRow
                  label="Time"
                  value={
                    selectedEvent.event_time
                  }
                />

              </div>
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
