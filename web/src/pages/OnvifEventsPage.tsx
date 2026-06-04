import { useState, useEffect } from 'react';
import { api } from '../api/client';

interface OnvifSubscription {
  id: string;
  camera_id: string;
  onvif_device_url: string;
  created_at: string;
}

export default function OnvifEventsPage() {
  const [subscriptions, setSubscriptions] = useState<OnvifSubscription[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showDialog, setShowDialog] = useState(false);
  const [cameraId, setCameraId] = useState('');
  const [deviceUrl, setDeviceUrl] = useState('');

  const fetchSubs = async () => {
    try {
      const data = await api.listOnvifSubscriptions();
      setSubscriptions(data.subscriptions || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSubs();
    const interval = setInterval(fetchSubs, 30000);
    return () => clearInterval(interval);
  }, []);

  const handleSubscribe = async () => {
    if (!cameraId || !deviceUrl) return;
    try {
      await api.subscribeOnvifEvents(cameraId, deviceUrl);
      setShowDialog(false);
      setCameraId('');
      setDeviceUrl('');
      await fetchSubs();
    } catch {
      setError('Failed to subscribe');
    }
  };

  const handleUnsubscribe = async (camId: string) => {
    if (!confirm('Unsubscribe from ONVIF events on this camera?')) return;
    try {
      await api.unsubscribeOnvifEvents(camId);
      await fetchSubs();
    } catch {
      setError('Failed to unsubscribe');
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading ONVIF subscriptions...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">ONVIF Events</h2>
        <button onClick={() => setShowDialog(true)}
          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">+ Subscribe</button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {subscriptions.length === 0 && <p className="p-6 text-sm text-slate-500">No ONVIF subscriptions.</p>}
        {subscriptions.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Camera ID</th><th className="p-3">Device URL</th><th className="p-3">Created</th><th className="p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {subscriptions.map(s => (
                <tr key={s.id} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300">{s.camera_id}</td>
                  <td className="p-3 text-slate-300 font-mono text-xs">{s.onvif_device_url}</td>
                  <td className="p-3 text-slate-300 text-xs">{new Date(s.created_at).toLocaleString()}</td>
                  <td className="p-3">
                    <button onClick={() => handleUnsubscribe(s.camera_id)}
                      className="text-xs px-3 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">Unsubscribe</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showDialog && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-md space-y-4">
            <h3 className="text-sm font-medium text-slate-300">Subscribe to ONVIF Events</h3>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Camera ID</label>
              <input type="text" value={cameraId} onChange={e => setCameraId(e.target.value)}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">ONVIF Device URL</label>
              <input type="text" value={deviceUrl} onChange={e => setDeviceUrl(e.target.value)} placeholder="e.g. http://192.168.1.100/onvif/device_service"
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div className="flex justify-end gap-2">
              <button onClick={() => setShowDialog(false)}
                className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Cancel</button>
              <button onClick={handleSubscribe} disabled={!cameraId || !deviceUrl}
                className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">Subscribe</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
