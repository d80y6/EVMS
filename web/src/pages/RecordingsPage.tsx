import { useEffect, useState, useCallback } from 'react';
import { api, Camera, Recording, AIEvent, type POSTransaction } from '../api/client';
import TimelineScrubber from '../components/TimelineScrubber';
import SyncPlaybackView from '../components/SyncPlaybackView';
import { useSyncPlayback } from '../hooks/useSyncPlayback';

export default function RecordingsPage() {
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedCamera, setSelectedCamera] = useState<string>('');
  const [events, setEvents] = useState<AIEvent[]>([]);
  const [playingUrl, setPlayingUrl] = useState<string | null>(null);
  const [selectedCameras, setSelectedCameras] = useState<string[]>([]);
  const sync = useSyncPlayback();

  const [posOverlay, setPosOverlay] = useState(false);
  const [currentTx, setCurrentTx] = useState<POSTransaction | null>(null);

  const fetchPOSTxns = useCallback(async () => {
    if (!selectedCamera) return;
    const start = new Date(sync.state.currentTime - 10000).toISOString();
    const end = new Date(sync.state.currentTime + 10000).toISOString();
    try {
      const data = await api.getPOSTransactions({ camera_id: selectedCamera, start_time: start, end_time: end });
      const transactions = data.transactions;
      const match = transactions.find(t => {
        const txnTime = new Date(t.timestamp).getTime();
        return Math.abs(txnTime - sync.state.currentTime) < 5000;
      });
      setCurrentTx(match || null);
    } catch {
      setCurrentTx(null);
    }
  }, [selectedCamera, sync.state.currentTime]);

  useEffect(() => {
    if (posOverlay) fetchPOSTxns();
  }, [posOverlay, fetchPOSTxns]);

  useEffect(() => {
    Promise.all([
      api.getRecordings(),
      api.listCameras(),
      api.getEvents(),
    ])
      .then(([recData, camData, evData]) => {
        setRecordings(recData.recordings || []);
        setCameras(camData || []);
        setEvents(evData.events || []);
        if (camData.length > 0 && !selectedCamera) {
          setSelectedCamera(camData[0].id);
        }
      })
      .catch((err) => setError(err.message))
      .finally(() => setIsLoading(false));
  }, []);

  const toggleCamera = (id: string) => {
    setSelectedCameras(prev =>
      prev.includes(id) ? prev.filter(c => c !== id) : [...prev, id]
    );
  };

  const handleSeek = (timestamp: string) => {
    sync.seek(new Date(timestamp).getTime());
  };

  const [exporting, setExporting] = useState(false);

  const handleExport = async () => {
    const cameraId = selectedCamera;
    if (!cameraId) return;
    const start = new Date(Date.now() - 3600000).toISOString();
    const end = new Date().toISOString();
    setExporting(true);
    try {
      const result = await api.exportRecording(cameraId, start, end, true);
      alert(`Export complete: ${result.sha256}`);
    } catch (err: any) {
      alert(`Export failed: ${err.message}`);
    } finally {
      setExporting(false);
    }
  };

  const handlePlay = (filePath: string) => {
    setPlayingUrl(api.getPlaybackUrl(filePath));
  };

  const formatTime = (ms: number) => {
    const d = new Date(ms);
    return d.toLocaleTimeString();
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

      <div className="flex flex-wrap gap-2">
        {cameras.map(cam => (
          <label
            key={cam.id}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm cursor-pointer transition-colors ${
              selectedCameras.includes(cam.id)
                ? 'bg-indigo-900/50 text-indigo-300 border border-indigo-700'
                : 'bg-slate-800 text-slate-400 border border-slate-700 hover:border-slate-600'
            }`}
          >
            <input
              type="checkbox"
              checked={selectedCameras.includes(cam.id)}
              onChange={() => toggleCamera(cam.id)}
              className="sr-only"
            />
            {cam.name}
          </label>
        ))}
      </div>

      {selectedCameras.length > 1 && (
        <>
          <div className="flex items-center gap-3 p-3 bg-slate-900 border border-slate-800 rounded-xl">
            <button
              onClick={() => sync.play(Date.now() - 60000)}
              className="bg-indigo-600 hover:bg-indigo-500 px-4 py-1.5 rounded-lg text-sm font-medium transition-colors"
            >
              ▶ Play
            </button>
            <button
              onClick={sync.pause}
              className="bg-slate-700 hover:bg-slate-600 px-4 py-1.5 rounded-lg text-sm font-medium transition-colors"
            >
              ⏸ Pause
            </button>
            <div className="flex-1 flex items-center gap-2">
              <span className="text-xs text-slate-500">{formatTime(sync.state.currentTime - 60000)}</span>
              <input
                type="range"
                min={-86400000}
                max={0}
                value={Date.now() - sync.state.currentTime}
                onChange={e => sync.seek(Date.now() - Number(e.target.value))}
                className="flex-1 accent-indigo-500"
              />
              <span className="text-xs text-slate-500">{formatTime(sync.state.currentTime)}</span>
            </div>
            <div className="flex items-center gap-1">
              {[0.5, 1, 2, 4].map(s => (
                <button
                  key={s}
                  onClick={() => sync.setSpeed(s)}
                  className={`px-2 py-1 rounded text-xs font-medium transition-colors ${
                    sync.state.speed === s
                      ? 'bg-indigo-700 text-white'
                      : 'bg-slate-700 text-slate-400 hover:bg-slate-600'
                  }`}
                >
                  {s}x
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            {selectedCameras.map(id => {
              const cam = cameras.find(c => c.id === id);
              return (
                <SyncPlaybackView
                  key={id}
                  cameraId={id}
                  cameraName={cam?.name || id}
                  sync={sync}
                />
              );
            })}
          </div>
        </>
      )}

      <div className="relative bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
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

        {posOverlay && currentTx && (
          <div className="absolute top-4 right-4 bg-slate-900/95 border border-slate-700 rounded-xl p-4 shadow-2xl backdrop-blur-sm w-72 z-50">
            <button onClick={() => setPosOverlay(false)} className="absolute top-2 right-2 text-slate-500 hover:text-slate-300 text-lg leading-none">&times;</button>
            <div className="text-sm font-semibold text-slate-200 mb-2">
              POS Transaction
            </div>
            <div className="text-xs text-slate-400 space-y-1">
              <div className="flex justify-between">
                <span>Register:</span>
                <span className="text-slate-300">{currentTx.register_id}</span>
              </div>
              <div className="flex justify-between">
                <span>Transaction:</span>
                <span className="text-slate-300">#{currentTx.transaction_id}</span>
              </div>
              <div className="flex justify-between">
                <span>Tender:</span>
                <span className="text-slate-300">{currentTx.tender_type}</span>
              </div>
              {currentTx.items.map((item: { description: string; quantity: number; total: number }, i: number) => (
                <div key={i} className="flex justify-between border-t border-slate-800 pt-1">
                  <span className="truncate max-w-[140px]">{item.description}</span>
                  <span className="text-slate-300">x{item.quantity} ${item.total.toFixed(2)}</span>
                </div>
              ))}
              <div className="flex justify-between border-t border-slate-800 pt-1 font-semibold text-slate-200">
                <span>Total</span>
                <span>${currentTx.total.toFixed(2)}</span>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="flex items-center gap-3">
        <label className="flex items-center gap-2 text-sm text-slate-400 cursor-pointer">
          <input
            type="checkbox"
            checked={posOverlay}
            onChange={e => setPosOverlay(e.target.checked)}
            className="accent-indigo-500"
          />
          POS Overlay
        </label>
        {currentTx && (
          <span className="text-xs text-green-400">
            Transaction ${currentTx.total.toFixed(2)} — {currentTx.items.length} item(s)
          </span>
        )}
      </div>

      <TimelineScrubber
        cameraId={selectedCamera}
        onSeek={handleSeek}
        events={events.map(e => ({ timestamp: e.event_time, type: e.object_type }))}
      />

      <div className="flex items-center gap-3">
        <label className="text-sm text-slate-400">Camera:</label>
        <select
          value={selectedCamera}
          onChange={(e) => setSelectedCamera(e.target.value)}
          className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-sm text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        >
          {cameras.map(cam => (
            <option key={cam.id} value={cam.id}>{cam.name}</option>
          ))}
        </select>
        <button
          onClick={handleExport}
          disabled={exporting}
          className="bg-green-700 hover:bg-green-600 disabled:bg-green-800 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ml-auto"
        >
          {exporting ? 'Exporting...' : 'Export with SHA-256'}
        </button>
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
