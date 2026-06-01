import { useEffect, useRef, useState } from 'react';
import { api, Camera, Tour, TourStep } from '../api/client';
import { useAuth } from '../context/AuthContext';
import { FloorPlanView } from '../components/FloorPlanView';

export default function SettingsPage() {
  const { username, role } = useAuth();
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [retentions, setRetentions] = useState<Record<string, number>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [tours, setTours] = useState<Tour[]>([]);
  const [showTourDialog, setShowTourDialog] = useState(false);
  const [tourName, setTourName] = useState('');
  const [tourInterval, setTourInterval] = useState(10);
  const [tourSteps, setTourSteps] = useState<TourStep[]>([{ camera_id: '', preset_token: '', dwell_seconds: 5 }]);
  const [editingTour, setEditingTour] = useState<Tour | null>(null);
  const [floorPlanImage, setFloorPlanImage] = useState<string | null>(null);
  const [floorPlanSiteId, setFloorPlanSiteId] = useState<string>('default');
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.getCameras()
      .then((data) => {
        if (data.cameras && data.cameras.length > 0) {
          setCameras(data.cameras);
          const ret: Record<string, number> = {};
          data.cameras.forEach((c) => { ret[c.id] = c.retention_days; });
          setRetentions(ret);
        }
      })
      .catch(() => {});
    api.listTours()
      .then((data) => setTours(data.tours || []))
      .catch(() => {});
  }, []);

  const handleStartTour = async (id: string) => {
    try {
      await api.startTour(id);
      setTours((prev) => prev.map((t) => t.id === id ? { ...t, enabled: true } : t));
    } catch {}
  };

  const handleStopTour = async (id: string) => {
    try {
      await api.stopTour(id);
      setTours((prev) => prev.map((t) => t.id === id ? { ...t, enabled: false } : t));
    } catch {}
  };

  const handleDeleteTour = async (id: string) => {
    try {
      await api.deleteTour(id);
      setTours((prev) => prev.filter((t) => t.id !== id));
    } catch {}
  };

  const handleCreateTour = async () => {
    try {
      const res = await api.createTour({ name: tourName, steps: tourSteps, interval: tourInterval });
      setTours((prev) => [...prev, { id: res.id, name: tourName, enabled: false, steps: tourSteps, interval: tourInterval, created_at: new Date().toISOString() }]);
      setShowTourDialog(false);
      setTourName('');
      setTourInterval(10);
      setTourSteps([{ camera_id: '', preset_token: '', dwell_seconds: 5 }]);
    } catch {}
  };

  const addStep = () => {
    setTourSteps((prev) => [...prev, { camera_id: '', preset_token: '', dwell_seconds: 5 }]);
  };

  const updateStep = (idx: number, field: keyof TourStep, value: string | number) => {
    setTourSteps((prev) => prev.map((s, i) => i === idx ? { ...s, [field]: value } : s));
  };

  const removeStep = (idx: number) => {
    setTourSteps((prev) => prev.filter((_, i) => i !== idx));
  };

  const handleRetentionChange = (cameraId: string, days: number) => {
    setRetentions((prev) => ({ ...prev, [cameraId]: days }));
  };

  const saveRetention = async (cameraId: string) => {
    setSaving(cameraId);
    try {
      await new Promise((resolve) => setTimeout(resolve, 300));
      setSaving(null);
    } catch {
      setSaving(null);
    }
  };

  return (
    <div className="max-w-2xl space-y-8">
      <h2 className="text-lg font-semibold text-slate-200">Settings</h2>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Profile</h3>
        <div className="text-sm text-slate-300 space-y-1">
          <p><span className="text-slate-500">Username:</span> {username}</p>
          <p><span className="text-slate-500">Role:</span> <span className="uppercase tracking-wider text-xs">{role}</span></p>
        </div>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-6">
        <h3 className="text-sm font-medium text-slate-400">Recording Retention</h3>
        <p className="text-xs text-slate-500">Configure per-camera retention period (7&ndash;90 days).</p>

        {cameras.length === 0 && (
          <p className="text-sm text-slate-500">No cameras configured.</p>
        )}

        {cameras.map((cam) => (
          <div key={cam.id} className="space-y-2">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-slate-300">{cam.name}</span>
                <span className="text-xs text-slate-600 ml-2">({cam.id})</span>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-sm text-slate-400 w-8 text-right font-mono">
                  {retentions[cam.id] ?? cam.retention_days}d
                </span>
                <button
                  onClick={() => saveRetention(cam.id)}
                  disabled={saving === cam.id}
                  className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors"
                >
                  {saving === cam.id ? 'Saving...' : 'Save'}
                </button>
              </div>
            </div>
            <input
              type="range"
              min="7"
              max="90"
              step="1"
              value={retentions[cam.id] ?? cam.retention_days}
              onChange={(e) => handleRetentionChange(cam.id, parseInt(e.target.value))}
              className="w-full h-1.5 bg-slate-700 rounded-full appearance-none cursor-pointer accent-indigo-500"
            />
            <div className="flex justify-between text-[10px] text-slate-600">
              <span>7 days</span>
              <span>90 days</span>
            </div>
          </div>
        ))}
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Notifications</h3>
        <p className="text-sm text-slate-500">
          Notification preferences (email, webhook, push) are configured via the backend services.
        </p>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Privacy Masking</h3>
        <p className="text-xs text-slate-500">Configure blur regions per camera.</p>
        {cameras.map((cam) => {
          const masks = cam.config ? (() => { try { return JSON.parse(cam.config).privacy_masks || []; } catch { return []; } })() : [];
          return (
            <div key={cam.id} className="space-y-2">
              <div className="text-sm text-slate-300">{cam.name}</div>
              {masks.length === 0 && <p className="text-xs text-slate-600">No masks configured.</p>}
              {masks.map((mask: any, i: number) => (
                <div key={i} className="bg-slate-800 p-2 rounded flex items-center gap-2 text-xs text-slate-400">
                  <span>{mask.label || `Mask ${i + 1}`}</span>
                  <span className="text-slate-600">({mask.points?.length || 0} points)</span>
                </div>
              ))}
            </div>
          );
        })}
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Streaming</h3>
        <p className="text-sm text-slate-500">
          WebRTC and recording settings are managed through the camera management service.
        </p>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-6">
        <h3 className="text-sm font-medium text-slate-400">Archive Tiering</h3>
        <p className="text-xs text-slate-500">Configure storage tiering thresholds (requires environment variable configuration).</p>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-xs text-slate-500 block mb-1">Hot→Warm after (days)</label>
            <input type="number" defaultValue={7} min={1} max={90}
                   className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
          </div>
          <div>
            <label className="text-xs text-slate-500 block mb-1">Warm→Cold after (days)</label>
            <input type="number" defaultValue={30} min={1} max={365}
                   className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
          </div>
        </div>
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">I/O Ports</h3>
        <p className="text-xs text-slate-500">ONVIF relay control — shown when an ONVIF camera is selected.</p>
        {cameras.filter(c => c.ptz_protocol === 'onvif').length === 0 && (
          <p className="text-xs text-slate-600">No ONVIF cameras configured.</p>
        )}
        {cameras.filter(c => c.ptz_protocol === 'onvif').map(cam => (
          <div key={cam.id} className="text-sm text-slate-300">Relays for {cam.name} — use camera's ONVIF interface.</div>
        ))}
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-slate-400">PTZ Tours</h3>
          <button onClick={() => setShowTourDialog(true)} className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">
            + New Tour
          </button>
        </div>

        {tours.length === 0 && (
          <p className="text-xs text-slate-600">No tours configured.</p>
        )}

        {tours.map((tour) => (
          <div key={tour.id} className="bg-slate-800 rounded-lg p-3 space-y-2">
            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-slate-300">{tour.name}</span>
                <span className="text-xs text-slate-600 ml-2">({tour.steps.length} steps, {tour.interval}s interval)</span>
              </div>
              <div className="flex items-center gap-2">
                {tour.enabled ? (
                  <button onClick={() => handleStopTour(tour.id)} className="text-xs px-2 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">Stop</button>
                ) : (
                  <button onClick={() => handleStartTour(tour.id)} className="text-xs px-2 py-1 bg-green-600 hover:bg-green-500 text-white rounded transition-colors">Start</button>
                )}
                <button onClick={() => handleDeleteTour(tour.id)} className="text-xs px-2 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Delete</button>
              </div>
            </div>
            <div className="text-xs text-slate-500 space-y-1">
              {tour.steps.map((step, i) => (
                <div key={i} className="text-slate-500">
                  Step {i + 1}: {step.camera_id}{step.preset_token ? ` → preset ${step.preset_token}` : ''} (dwell: {step.dwell_seconds}s)
                </div>
              ))}
            </div>
          </div>
        ))}

        {showTourDialog && (
          <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-lg space-y-4 max-h-[80vh] overflow-y-auto">
              <h4 className="text-sm font-medium text-slate-300">New Tour</h4>

              <div>
                <label className="text-xs text-slate-500 block mb-1">Tour Name</label>
                <input type="text" value={tourName} onChange={(e) => setTourName(e.target.value)}
                       className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
              </div>

              <div>
                <label className="text-xs text-slate-500 block mb-1">Interval (seconds between steps)</label>
                <input type="number" value={tourInterval} onChange={(e) => setTourInterval(parseInt(e.target.value) || 10)} min={1}
                       className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="text-xs text-slate-500">Steps</label>
                  <button onClick={addStep} className="text-xs px-2 py-0.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">+ Add Step</button>
                </div>
                {tourSteps.map((step, i) => (
                  <div key={i} className="bg-slate-800 rounded p-2 space-y-2">
                    <div className="flex items-center justify-between">
                      <span className="text-xs text-slate-400">Step {i + 1}</span>
                      {tourSteps.length > 1 && (
                        <button onClick={() => removeStep(i)} className="text-xs text-red-400 hover:text-red-300">Remove</button>
                      )}
                    </div>
                    <div className="grid grid-cols-3 gap-2">
                      <div>
                        <label className="text-[10px] text-slate-600 block">Camera ID</label>
                        <input type="text" value={step.camera_id} onChange={(e) => updateStep(i, 'camera_id', e.target.value)}
                               className="w-full bg-slate-700 border border-slate-600 rounded px-2 py-1 text-xs text-slate-300" />
                      </div>
                      <div>
                        <label className="text-[10px] text-slate-600 block">Preset Token</label>
                        <input type="text" value={step.preset_token || ''} onChange={(e) => updateStep(i, 'preset_token', e.target.value)}
                               className="w-full bg-slate-700 border border-slate-600 rounded px-2 py-1 text-xs text-slate-300" />
                      </div>
                      <div>
                        <label className="text-[10px] text-slate-600 block">Dwell (s)</label>
                        <input type="number" value={step.dwell_seconds} onChange={(e) => updateStep(i, 'dwell_seconds', parseInt(e.target.value) || 5)} min={1}
                               className="w-full bg-slate-700 border border-slate-600 rounded px-2 py-1 text-xs text-slate-300" />
                      </div>
                    </div>
                  </div>
                ))}
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button onClick={() => setShowTourDialog(false)} className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Cancel</button>
                <button onClick={handleCreateTour} disabled={!tourName || tourSteps.length === 0}
                        className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">Create</button>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">Floor Plan</h3>
        <p className="text-xs text-slate-500">Upload a floor plan image and drag camera markers to their positions.</p>

        <div className="flex items-center gap-3">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) {
                const reader = new FileReader()
                reader.onload = (ev) => setFloorPlanImage(ev.target?.result as string)
                reader.readAsDataURL(file)
              }
            }}
            className="text-xs text-slate-400 file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:text-xs file:bg-indigo-600 file:text-white hover:file:bg-indigo-500"
          />
          {floorPlanImage && (
            <button
              onClick={() => { setFloorPlanImage(null); if (fileInputRef.current) fileInputRef.current.value = ''; }}
              className="text-xs px-2 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors"
            >
              Clear
            </button>
          )}
        </div>

        {cameras.length > 0 && (
          <div>
            <label className="text-xs text-slate-500 block mb-1">Site ID</label>
            <select
              value={floorPlanSiteId}
              onChange={(e) => setFloorPlanSiteId(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300"
            >
              {[...new Set(cameras.map(c => c.site_id))].map(sid => (
                <option key={sid} value={sid}>{sid}</option>
              ))}
            </select>
          </div>
        )}

        {floorPlanImage && (
          <FloorPlanView
            imageUrl={floorPlanImage}
            cameras={cameras.filter(c => c.site_id === floorPlanSiteId)}
            siteId={floorPlanSiteId}
            onCameraClick={(id) => console.log('Camera clicked:', id)}
          />
        )}

        {!floorPlanImage && (
          <div className="h-[200px] bg-slate-800 rounded flex items-center justify-center">
            <p className="text-xs text-slate-600">No floor plan uploaded. Select an image above.</p>
          </div>
        )}
      </div>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
        <h3 className="text-sm font-medium text-slate-400">System</h3>
        <div className="text-xs text-slate-600 space-y-1">
          <p>DAM VMS v0.1.0</p>
          <p>React + Vite + Tailwind CSS</p>
          <p>WebRTC via Pion</p>
          <p>Cameras: {cameras.length}</p>
        </div>
      </div>
    </div>
  );
}
