import { useEffect, useState } from 'react';
import { api } from '../api/client';

interface Camera { id: string; name: string; }

export default function ImagingPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [selectedCamera, setSelectedCamera] = useState('');
  const [settings, setSettings] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [focusSpeed, setFocusSpeed] = useState(0.5);
  const [exposureMode, setExposureMode] = useState('AUTO');
  const [wbMode, setWbMode] = useState('AUTO');
  const [irCutMode, setIrCutMode] = useState('AUTO');

  useEffect(() => {
    api.getCameras().then(d => setCameras(d.cameras)).catch(() => {});
  }, []);

  useEffect(() => {
    if (!selectedCamera) return;
    setLoading(true);
    api.getImagingSettings(selectedCamera).then(d => {
      const s = d.settings || d;
      setSettings(s);
      setExposureMode(s.exposure?.mode || 'AUTO');
      setWbMode(s.white_balance?.mode || 'AUTO');
      setIrCutMode(s.ir_cut_filter || 'AUTO');
    }).catch(() => setSettings(null)).finally(() => setLoading(false));
  }, [selectedCamera]);

  const updateField = (field: string, value: any) => {
    setSettings({ ...settings, [field]: value });
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const payload = {
        ...settings,
        exposure: { ...(settings.exposure || {}), mode: exposureMode },
        white_balance: { ...(settings.white_balance || {}), mode: wbMode },
        ir_cut_filter: irCutMode,
      };
      await api.setImagingSettings(selectedCamera, '', payload);
      setMessage('Settings saved');
    } catch { setMessage('Save failed'); }
    setSaving(false);
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Imaging Settings</h1>

      <select value={selectedCamera} onChange={e => setSelectedCamera(e.target.value)}
        className="bg-slate-800 text-white rounded px-3 py-2 text-sm">
        <option value="">Select camera...</option>
        {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
      </select>

      {loading && <p className="text-sm text-slate-400">Loading settings...</p>}

      {settings && (
        <div className="grid grid-cols-2 gap-6 max-w-2xl">
          {/* Basic Controls */}
          <div className="bg-slate-900 p-4 rounded-lg space-y-3">
            <h3 className="text-sm font-medium">Basic</h3>
            {['Brightness', 'Contrast', 'Sharpness', 'Saturation'].map(field => (
              <div key={field}>
                <label className="text-xs text-slate-400 flex justify-between">
                  <span>{field}</span>
                  <span className="text-slate-500">{settings[field.toLowerCase()] ?? 50}</span>
                </label>
                <input type="range" min="0" max="100"
                  value={settings[field.toLowerCase()] ?? 50}
                  onChange={e => updateField(field.toLowerCase(), Number(e.target.value))}
                  className="w-full" />
              </div>
            ))}
          </div>

          {/* Exposure */}
          <div className="bg-slate-900 p-4 rounded-lg space-y-3">
            <h3 className="text-sm font-medium">Exposure</h3>
            <select value={exposureMode} onChange={e => setExposureMode(e.target.value)}
              className="w-full bg-slate-800 text-white rounded px-2 py-1 text-xs">
              <option value="AUTO">Auto</option><option value="MANUAL">Manual</option>
              <option value="APERTURE_PRIORITY">Aperture Priority</option><option value="SHUTTER_PRIORITY">Shutter Priority</option>
            </select>
            <div>
              <label className="text-xs text-slate-400">Exposure Time (µs)</label>
              <input type="number" value={settings.exposure?.exposure_time || 8333}
                onChange={e => updateField('exposure', { ...settings.exposure, exposure_time: Number(e.target.value) })}
                className="w-full bg-slate-800 text-white rounded px-2 py-1 text-xs" />
            </div>
            <div>
              <label className="text-xs text-slate-400">Gain (dB)</label>
              <input type="number" step="0.1" value={settings.exposure?.gain || 0}
                onChange={e => updateField('exposure', { ...settings.exposure, gain: Number(e.target.value) })}
                className="w-full bg-slate-800 text-white rounded px-2 py-1 text-xs" />
            </div>
            <div>
              <label className="text-xs text-slate-400">Iris</label>
              <input type="range" min="0" max="100" value={settings.exposure?.iris || 50}
                onChange={e => updateField('exposure', { ...settings.exposure, iris: Number(e.target.value) })}
                className="w-full" />
            </div>
          </div>

          {/* White Balance */}
          <div className="bg-slate-900 p-4 rounded-lg space-y-3">
            <h3 className="text-sm font-medium">White Balance</h3>
            <select value={wbMode} onChange={e => setWbMode(e.target.value)}
              className="w-full bg-slate-800 text-white rounded px-2 py-1 text-xs">
              <option value="AUTO">Auto</option><option value="MANUAL">Manual</option>
              <option value="DAYLIGHT">Daylight</option><option value="FLUORESCENT">Fluorescent</option>
              <option value="INCANDESCENT">Incandescent</option>
            </select>
            {wbMode === 'MANUAL' && (
              <>
                <div>
                  <label className="text-xs text-slate-400">Red Gain</label>
                  <input type="range" min="0" max="100" value={settings.white_balance?.red_gain || 50}
                    onChange={e => updateField('white_balance', { ...settings.white_balance, red_gain: Number(e.target.value) })}
                    className="w-full" />
                </div>
                <div>
                  <label className="text-xs text-slate-400">Blue Gain</label>
                  <input type="range" min="0" max="100" value={settings.white_balance?.blue_gain || 50}
                    onChange={e => updateField('white_balance', { ...settings.white_balance, blue_gain: Number(e.target.value) })}
                    className="w-full" />
                </div>
              </>
            )}
          </div>

          {/* Advanced */}
          <div className="bg-slate-900 p-4 rounded-lg space-y-3">
            <h3 className="text-sm font-medium">Advanced</h3>
            <div>
              <label className="text-xs text-slate-400">WDR Level</label>
              <input type="range" min="0" max="100" value={settings.wdr?.level || 0}
                onChange={e => updateField('wdr', { ...settings.wdr, level: Number(e.target.value) })}
                className="w-full" />
            </div>
            <div className="flex items-center gap-2">
              <input type="checkbox" checked={settings.backlight_compensation?.enabled || false}
                onChange={e => updateField('backlight_compensation', { ...settings.backlight_compensation, enabled: e.target.checked })}
                className="rounded" />
              <label className="text-xs text-slate-400">Backlight Compensation</label>
            </div>
            <div>
              <label className="text-xs text-slate-400">IR Cut Filter</label>
              <select value={irCutMode} onChange={e => setIrCutMode(e.target.value)}
                className="w-full bg-slate-800 text-white rounded px-2 py-1 text-xs mt-1">
                <option value="AUTO">Auto</option><option value="ON">On</option><option value="OFF">Off</option>
              </select>
            </div>
          </div>

          {/* Focus */}
          <div className="bg-slate-900 p-4 rounded-lg col-span-2 space-y-3">
            <h3 className="text-sm font-medium">Focus</h3>
            <div className="flex items-center gap-3">
              <input type="range" min="-1" max="1" step="0.1" value={focusSpeed}
                onChange={e => setFocusSpeed(Number(e.target.value))} className="flex-1" />
              <span className="text-xs text-slate-400 w-12">{focusSpeed.toFixed(1)}</span>
              <button onClick={() => api.moveFocus(selectedCamera, focusSpeed)}
                className="px-3 py-1 text-xs bg-indigo-600 rounded hover:bg-indigo-500">Move</button>
              <button onClick={() => api.stopFocus(selectedCamera)}
                className="px-3 py-1 text-xs bg-red-600 rounded hover:bg-red-500">Stop</button>
            </div>
          </div>

          <div className="col-span-2 flex items-center gap-4">
            <button onClick={handleSave} disabled={saving}
              className="px-4 py-2 text-sm bg-indigo-600 rounded hover:bg-indigo-500 disabled:opacity-50">
              {saving ? 'Saving...' : 'Save Settings'}
            </button>
            {message && <span className="text-sm text-green-400">{message}</span>}
          </div>
        </div>
      )}
    </div>
  );
}
