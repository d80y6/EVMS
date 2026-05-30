import React, { useEffect, useState } from 'react';
import { api, Recording } from '../api/client';

export default function RecordingsPage() {
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getRecordings()
      .then((data) => setRecordings(data.recordings))
      .catch((err) => setError(err.message))
      .finally(() => setIsLoading(false));
  }, []);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-slate-400 animate-pulse">Loading recordings...</p>
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

  if (recordings.length === 0) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-slate-500">No recordings found.</p>
      </div>
    );
  }

  return (
    <div>
      <h2 className="text-lg font-semibold text-slate-200 mb-6">Recordings</h2>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
              <th className="text-left pb-3 pr-4">Camera</th>
              <th className="text-left pb-3 pr-4">Start Time</th>
              <th className="text-left pb-3 pr-4">End Time</th>
              <th className="text-left pb-3 pr-4">Size</th>
              <th className="text-left pb-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {recordings.map((rec, i) => (
              <tr key={i} className="border-b border-slate-800/50 text-slate-300">
                <td className="py-3 pr-4">{rec.camera_id}</td>
                <td className="py-3 pr-4">{new Date(rec.start_time).toLocaleString()}</td>
                <td className="py-3 pr-4">{new Date(rec.end_time).toLocaleString()}</td>
                <td className="py-3 pr-4">{(rec.file_size / 1024 / 1024).toFixed(1)} MB</td>
                <td className="py-3">
                  <a
                    href={api.getPlaybackUrl(rec.file_path)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-indigo-400 hover:text-indigo-300 transition-colors"
                  >
                    Play
                  </a>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
