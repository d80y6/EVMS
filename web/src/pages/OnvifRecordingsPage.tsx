import { useEffect, useState } from 'react';
import { api } from '../api/client';

interface Camera { id: string; name: string; }

export default function OnvifRecordingsPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [selectedCamera, setSelectedCamera] = useState('');
  const [recordings, setRecordings] = useState<any[]>([]);
  const [tracks, setTracks] = useState<any[]>([]);
  const [replayUri, setReplayUri] = useState('');
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState('');

  useEffect(() => {
    api.getCameras().then(d => setCameras(d.cameras)).catch(() => {});
  }, []);

  useEffect(() => {
    if (!selectedCamera) return;
    setLoading(true);
    setReplayUri('');
    setTracks([]);
    api.listOnvifRecordings(selectedCamera)
      .then(d => setRecordings(d.recordings || [])).catch(() => setRecordings([]))
      .finally(() => setLoading(false));
  }, [selectedCamera]);

  const handleDelete = async (token: string) => {
    if (!confirm('Delete this recording from the device?')) return;
    try {
      await api.deleteOnvifRecording(selectedCamera, token);
      setRecordings(prev => prev.filter(r => r.token !== token));
      setMessage('Recording deleted');
    } catch { setMessage('Delete failed'); }
  };

  const handleShowTracks = async (token: string) => {
    try {
      const d = await api.getRecordingTracks(selectedCamera, token);
      setTracks(d.tracks || []);
    } catch { setMessage('Failed to load tracks'); }
  };

  const handleGetReplayUri = async (recordingToken: string) => {
    try {
      const d = await api.getReplayUri(selectedCamera, recordingToken);
      setReplayUri(d.replay_uri || d.uri || '');
    } catch { setMessage('Failed to get replay URI'); }
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">ONVIF Device Recordings</h1>

      <select value={selectedCamera} onChange={e => setSelectedCamera(e.target.value)}
        className="bg-slate-800 text-white rounded px-3 py-2 text-sm">
        <option value="">Select camera...</option>
        {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
      </select>

      {message && <p className="text-sm text-green-400">{message}</p>}
      {loading && <p className="text-sm text-slate-400">Loading recordings...</p>}

      {!loading && selectedCamera && recordings.length === 0 && (
        <p className="text-sm text-slate-500">No ONVIF recordings on this device.</p>
      )}

      {recordings.length > 0 && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead><tr className="text-slate-400 border-b border-slate-800 text-left">
              <th className="p-3">Token</th><th className="p-3">Source</th><th className="p-3">Content</th><th className="p-3">Tracks</th><th className="p-3">Actions</th>
            </tr></thead>
            <tbody>
              {recordings.map((r: any) => (
                <tr key={r.token} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300 text-xs font-mono">{r.token}</td>
                  <td className="p-3 text-slate-300 text-xs">{r.source?.source_id || r.source?.name || '-'}</td>
                  <td className="p-3 text-slate-300 text-xs">{r.content || '-'}</td>
                  <td className="p-3 text-slate-300 text-xs">{r.tracks?.length || 0}</td>
                  <td className="p-3 flex gap-1">
                    <button onClick={() => handleShowTracks(r.token)}
                      className="text-xs px-2 py-1 bg-slate-700 rounded hover:bg-slate-600">Tracks</button>
                    <button onClick={() => handleGetReplayUri(r.token)}
                      className="text-xs px-2 py-1 bg-indigo-700 rounded hover:bg-indigo-600">Replay</button>
                    <button onClick={() => handleDelete(r.token)}
                      className="text-xs px-2 py-1 bg-red-800 rounded hover:bg-red-700">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tracks.length > 0 && (
        <div className="bg-slate-900 p-4 rounded-lg">
          <h3 className="text-sm font-medium mb-2">Tracks</h3>
          <div className="space-y-1">
            {tracks.map((t: any, i: number) => (
              <p key={i} className="text-xs text-slate-300">
                {t.track_type || t.type}: {t.encoding || t.codec || '-'}
                ({t.bitrate || '?'} kbps)
              </p>
            ))}
          </div>
        </div>
      )}

      {replayUri && (
        <div className="bg-slate-900 p-4 rounded-lg">
          <h3 className="text-sm font-medium mb-2">Replay URI</h3>
          <p className="text-xs text-slate-300 font-mono break-all">{replayUri}</p>
        </div>
      )}
    </div>
  );
}
