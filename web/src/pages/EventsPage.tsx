import React, { useEffect, useState } from 'react';
import { api, AIEvent } from '../api/client';

export default function EventsPage() {
  const [events, setEvents] = useState<AIEvent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getEvents()
      .then((data) => setEvents(data.events))
      .catch((err) => setError(err.message))
      .finally(() => setIsLoading(false));
  }, []);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-slate-400 animate-pulse">Loading events...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-red-400">{error}</p>
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-slate-500">No AI events detected yet.</p>
      </div>
    );
  }

  const confidenceColor = (c: number) => {
    if (c >= 0.8) return 'text-green-400';
    if (c >= 0.5) return 'text-yellow-400';
    return 'text-slate-400';
  };

  return (
    <div>
      <h2 className="text-lg font-semibold text-slate-200 mb-6">AI Events</h2>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
              <th className="text-left pb-3 pr-4">Time</th>
              <th className="text-left pb-3 pr-4">Camera</th>
              <th className="text-left pb-3 pr-4">Object</th>
              <th className="text-left pb-3">Confidence</th>
            </tr>
          </thead>
          <tbody>
            {events.map((ev, i) => (
              <tr key={i} className="border-b border-slate-800/50 text-slate-300">
                <td className="py-3 pr-4">{new Date(ev.event_time).toLocaleString()}</td>
                <td className="py-3 pr-4">{ev.camera_id}</td>
                <td className="py-3 pr-4 capitalize">{ev.object_type}</td>
                <td className={`py-3 font-medium ${confidenceColor(ev.confidence)}`}>
                  {(ev.confidence * 100).toFixed(0)}%
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
