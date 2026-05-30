import React, { useEffect, useState } from 'react';
import { api, Recording } from '../api/client';
import TimelineScrubber from '../components/TimelineScrubber';

export default function RecordingsPage() {
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedCamera, setSelectedCamera] = useState<string>('demo_cam');
  const [playingUrl, setPlayingUrl] = useState<string | null>(null);

  useEffect(() => {
    api.getRecordings()
      .then((data) => setRecordings(data.recordings))
      .catch((err) => setError(err.message))
      .finally(() => setIsLoading(false));
  }, []);

  const handleSeek = (timestamp: string) => {
    console.log('Seek to:', timestamp);
  };

  const handlePlay = (filePath: string) => {
    setPlayingUrl(api.getPlaybackUrl(filePath));
  };

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

  return (
    <div className="space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">Recordings</h2>

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        <div className="aspect-video bg-slate-950 flex items-center justify-center">
          {playingUrl ? (
            <video
              src={playingUrl}
              controls
              autoPlay
              className="w-full h-full"
            />
          ) : (
            <p className="text-slate-600 text-sm">Select a recording to play</p>
          )}
        </div>
      </div>

      <TimelineScrubber
        cameraId={selectedCamera}
        onSeek={handleSeek}
        events={[]}
      />

      <div className="flex items-center gap-3">
        <label className="text-sm text-slate-400">Camera:</label>
        <select
          value={selectedCamera}
          onChange={(e) => setSelectedCamera(e.target.value)}
          className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        >
          <option value="demo_cam">Front Entrance</option>
          <option value="parking_lot">Parking Lot</option>
          <option value="warehouse">Main Warehouse</option>
        </select>
      </div>

      {recordings.length === 0 ? (
        <div className="flex items-center justify-center h-32">
          <p className="text-slate-500">No recordings found.</p>
        </div>
      ) : (
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
                    <button
                      onClick={() => handlePlay(rec.file_path)}
                      className="text-indigo-400 hover:text-indigo-300 transition-colors"
                    >
                      Play
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
