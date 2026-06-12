import { useEffect, useMemo, useState } from 'react';
import { api, Camera } from '../../api/client';

interface Site {
  id: string;
  name: string;
  location: string;
}

interface CameraDialogProps {
  open: boolean;
  camera?: Camera | null;
  sites: Site[];
  onClose: () => void;
  onSaved: () => Promise<void>;
}

const emptyForm = {
  site_id: '',
  name: '',
  description: '',
  connection_url: '',
  substream_url: '',
  ptz_protocol: 'none',
  retention_days: 30,
  onvif_username: '',
  onvif_password: '',
};

export default function CameraDialog({
  open,
  camera,
  sites,
  onClose,
  onSaved,
}: CameraDialogProps) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] =
    useState(emptyForm);

  const [config, setConfig] = useState<{ onvif_port: number; is_onvif: boolean }>({
    onvif_port: 8000,
    is_onvif: true,
  });

  useEffect(() => {
    if (!camera) {
      setForm(emptyForm);
      setConfig({ onvif_port: 8000, is_onvif: true });
      return;
    }

    setForm({
      site_id: camera.site_id,
      name: camera.name,
      description:
        camera.description || '',
      connection_url:
        camera.connection_url,
      substream_url:
        camera.substream_url || '',
      ptz_protocol:
        camera.ptz_protocol || 'none',
      retention_days:
        camera.retention_days || 30,
      onvif_username:
        camera.onvif_username || '',
      onvif_password: '',
    });

    if (camera.config) {
      try {
        const parsed = JSON.parse(camera.config);
        setConfig({
          onvif_port: parsed.onvif_port || 8000,
          is_onvif: parsed.is_onvif !== false,
        });
      } catch {
        setConfig({ onvif_port: 8000, is_onvif: true });
      }
    } else {
      setConfig({ onvif_port: 8000, is_onvif: true });
    }
  }, [camera]);

  useEffect(() => {
    if (!open) return;

    const handler = (
      e: KeyboardEvent
    ) => {
      if (e.key === 'Escape') {
        onClose();
      }

      if (
        e.key === 'Enter' &&
        e.ctrlKey
      ) {
        void handleSave();
      }
    };

    window.addEventListener(
      'keydown',
      handler
    );

    return () =>
      window.removeEventListener(
        'keydown',
        handler
      );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, form]);

  const valid = useMemo(
    () =>
      form.site_id &&
      form.name.trim() &&
      form.connection_url.trim(),
    [form]
  );

  const handleSave = async () => {
    if (!valid) return;

    try {
      setSaving(true);
      setError(null);

      const payload: any = {
        site_id: form.site_id,
        name: form.name,
        description:
          form.description,
        connection_url:
          form.connection_url,
        substream_url:
          form.substream_url,
        ptz_protocol:
          form.ptz_protocol,
        retention_days:
          form.retention_days,
        onvif_username:
          form.onvif_username,
      };

      if (
        form.onvif_password.trim()
      ) {
        payload.onvif_password =
          form.onvif_password;
      }

      payload.config = JSON.stringify({ ...(camera?.config ? JSON.parse(camera.config) : {}), ...config });

      if (camera?.id) {
        await api.updateCamera(
          camera.id,
          payload
        );
      } else {
        await api.createCamera({
          ...payload,
          onvif_password:
            form.onvif_password,
        });
      }

      await onSaved();

      onClose();
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Save failed'
      );
    } finally {
      setSaving(false);
    }
  };

  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4">

      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="camera-dialog-title"
        className="w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-xl"
      >

        <div className="border-b border-slate-800 px-6 py-4">
          <h2
            id="camera-dialog-title"
            className="text-lg font-semibold text-slate-200"
          >
            {camera
              ? 'Edit Camera'
              : 'Add Camera'}
          </h2>
        </div>

        <div className="p-6 space-y-4">

          {error && (
            <div className="rounded-lg border border-red-800 bg-red-950/30 p-3 text-sm text-red-400">
              {error}
            </div>
          )}

          <div>
            <label className="block text-xs text-slate-500 mb-1">
              Site
            </label>

            <select
              value={form.site_id}
              onChange={(e) =>
                setForm({
                  ...form,
                  site_id:
                    e.target.value,
                })
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
            >
              <option value="">
                Select Site
              </option>

              {sites.map((site) => (
                <option
                  key={site.id}
                  value={site.id}
                >
                  {site.name}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs text-slate-500 mb-1">
              Camera Name
            </label>

            <input
              value={form.name}
              onChange={(e) =>
                setForm({
                  ...form,
                  name:
                    e.target.value,
                })
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
            />
          </div>

          <div>
            <label className="block text-xs text-slate-500 mb-1">
              Description
            </label>

            <textarea
              rows={3}
              value={form.description}
              onChange={(e) =>
                setForm({
                  ...form,
                  description:
                    e.target.value,
                })
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
            />
          </div>

          <div>
            <label className="block text-xs text-slate-500 mb-1">
              Main Stream URL
            </label>

            <input
              value={
                form.connection_url
              }
              onChange={(e) =>
                setForm({
                  ...form,
                  connection_url:
                    e.target.value,
                })
              }
              placeholder="rtsp://..."
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
            />
          </div>

          <div>
            <label className="block text-xs text-slate-500 mb-1">
              Sub Stream URL
            </label>

            <input
              value={
                form.substream_url
              }
              onChange={(e) =>
                setForm({
                  ...form,
                  substream_url:
                    e.target.value,
                })
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                PTZ Protocol
              </label>

              <select
                value={
                  form.ptz_protocol
                }
                onChange={(e) =>
                  setForm({
                    ...form,
                    ptz_protocol:
                      e.target.value,
                  })
                }
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
              >
                <option value="none">
                  None
                </option>

                <option value="onvif">
                  ONVIF
                </option>

                <option value="pelco_d">
                  Pelco D
                </option>

                <option value="pelco_p">
                  Pelco P
                </option>
              </select>
            </div>

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                Retention Days
              </label>

              <input
                type="number"
                min={1}
                max={365}
                value={
                  form.retention_days
                }
                onChange={(e) =>
                  setForm({
                    ...form,
                    retention_days:
                      Number(
                        e.target.value
                      ) || 30,
                  })
                }
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
              />
            </div>

          </div>

          <div className="grid grid-cols-2 gap-4">

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                ONVIF Enabled
              </label>

              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={config.is_onvif}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      is_onvif: e.target.checked,
                    })
                  }
                  className="w-4 h-4 rounded border-slate-600 bg-slate-800 text-indigo-600 focus:ring-indigo-500"
                />
                <span className="text-sm text-slate-400">
                  {config.is_onvif ? 'Enabled' : 'Disabled'}
                </span>
              </label>
            </div>

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                ONVIF Port
              </label>

              <input
                type="number"
                min={1}
                max={65535}
                value={config.onvif_port}
                disabled={!config.is_onvif}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    onvif_port:
                      Number(e.target.value) || 8000,
                  })
                }
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300 disabled:opacity-50 disabled:cursor-not-allowed"
              />
            </div>

          </div>

          {form.ptz_protocol === 'onvif' && !config.is_onvif && (
            <div className="rounded-lg border border-amber-800 bg-amber-950/30 p-3 text-sm text-amber-400">
              PTZ protocol is set to ONVIF but ONVIF is disabled. PTZ will not work.
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                ONVIF Username
              </label>

              <input
                value={
                  form.onvif_username
                }
                onChange={(e) =>
                  setForm({
                    ...form,
                    onvif_username:
                      e.target.value,
                  })
                }
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
              />
            </div>

            <div>
              <label className="block text-xs text-slate-500 mb-1">
                ONVIF Password
              </label>

              <input
                type="password"
                value={
                  form.onvif_password
                }
                onChange={(e) =>
                  setForm({
                    ...form,
                    onvif_password:
                      e.target.value,
                  })
                }
                placeholder={
                  camera
                    ? 'Leave blank to keep current password'
                    : ''
                }
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2 text-slate-300"
              />
            </div>

          </div>

        </div>

        <div className="border-t border-slate-800 px-6 py-4 flex justify-end gap-2">

          <button
            onClick={onClose}
            className="px-4 py-2 rounded bg-slate-700 hover:bg-slate-600 text-white text-sm"
          >
            Cancel
          </button>

          <button
            disabled={
              !valid || saving
            }
            onClick={handleSave}
            className="px-4 py-2 rounded bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-900 text-white text-sm"
          >
            {saving
              ? 'Saving...'
              : camera
              ? 'Update Camera'
              : 'Create Camera'}
          </button>

        </div>

      </div>

    </div>
  );
}
