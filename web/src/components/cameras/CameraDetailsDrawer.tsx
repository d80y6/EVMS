import { useEffect, useState } from 'react';
import {
  Activity,
  Database,
  Info,
  Network,
  RefreshCw,
  Shield,
  Video,
  X,
} from 'lucide-react';

import { api, Camera } from '../../api/client';
import CameraSnapshot from './CameraSnapshot';

interface CameraDetailsDrawerProps {
  open: boolean;
  camera: Camera | null;
  onClose: () => void;
}

type TabKey =
  | 'general'
  | 'streams'
  | 'network'
  | 'diagnostics'
  | 'recording'
  | 'onvif';

function InfoRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex justify-between gap-4 py-2 border-b border-slate-800">
      <span className="text-slate-500">
        {label}
      </span>

      <span className="text-slate-200 text-right break-all">
        {value ?? '-'}
      </span>
    </div>
  );
}

export default function CameraDetailsDrawer({
  open,
  camera,
  onClose,
}: CameraDetailsDrawerProps) {
  const [activeTab, setActiveTab] =
    useState<TabKey>('general');

  const [loading, setLoading] =
    useState(false);

  const [error, setError] =
    useState<string | null>(null);

  const [general, setGeneral] =
    useState<any>(null);

  const [streams, setStreams] =
    useState<any>(null);

  const [network, setNetwork] =
    useState<any>(null);

  const [diagnostics, setDiagnostics] =
    useState<any>(null);

  const [recording, setRecording] =
    useState<any>(null);

  const [onvif, setOnvif] =
    useState<any>(null);

  useEffect(() => {
    if (!open || !camera) {
      return;
    }

    void loadTab(activeTab);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, camera, activeTab]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const handler = (
      e: KeyboardEvent
    ) => {
      if (e.key === 'Escape') {
        onClose();
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
  }, [open, onClose]);

  const loadTab = async (
    tab: TabKey
  ) => {
    if (!camera) {
      return;
    }

    try {
      setLoading(true);
      setError(null);

      switch (tab) {
        case 'general': {
          if (general) {
            break;
          }

          const [
            info,
            capabilities,
          ] = await Promise.all([
            api.getDeviceInfo(
              camera.id
            ),
            api.getDeviceCapabilities(
              camera.id
            ),
          ]);

          setGeneral({
            info,
            capabilities,
          });

          break;
        }

        case 'streams': {
          if (streams) {
            break;
          }

          const [
            profiles,
            videoSources,
            audioSources,
          ] = await Promise.all([
            api.getProfiles(
              camera.id
            ),
            api.getVideoSources(
              camera.id
            ),
            api.getAudioSources(
              camera.id
            ),
          ]);

          setStreams({
            profiles,
            videoSources,
            audioSources,
          });

          break;
        }

        case 'network': {
          if (network) {
            break;
          }

          const result =
            await api.getNetworkInterfaces(
              camera.id
            );

          setNetwork(result);

          break;
        }

        case 'diagnostics': {
          if (diagnostics) {
            break;
          }

          const result =
            await api.getDeviceDiagnostics(
              camera.id
            );

          setDiagnostics(
            result
          );

          break;
        }

        case 'recording': {
          if (recording) {
            break;
          }

          const result =
            await api.getCameraRecording(
              camera.id
            );

          setRecording(
            result
          );

          break;
        }

        case 'onvif': {
          if (onvif) {
            break;
          }

          const result =
            await api.getCameraOnvif(
              camera.id
            );

          setOnvif(result);

          break;
        }
      }
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to load camera data'
      );
    } finally {
      setLoading(false);
    }
  };

  const refreshCurrentTab =
    async () => {
      switch (activeTab) {
        case 'general':
          setGeneral(null);
          break;

        case 'streams':
          setStreams(null);
          break;

        case 'network':
          setNetwork(null);
          break;

        case 'diagnostics':
          setDiagnostics(null);
          break;

        case 'recording':
          setRecording(null);
          break;

        case 'onvif':
          setOnvif(null);
          break;
      }

      await loadTab(
        activeTab
      );
    };

  if (!open || !camera) {
    return null;
  }

  const tabs = [
    {
      key: 'general',
      label: 'General',
      icon: Info,
    },
    {
      key: 'streams',
      label: 'Streams',
      icon: Video,
    },
    {
      key: 'network',
      label: 'Network',
      icon: Network,
    },
    {
      key: 'diagnostics',
      label: 'Diagnostics',
      icon: Activity,
    },
    {
      key: 'recording',
      label: 'Recording',
      icon: Database,
    },
    {
      key: 'onvif',
      label: 'ONVIF',
      icon: Shield,
    },
  ] as const;

  return (
    <div className="fixed inset-0 z-50">

      <div
        className="absolute inset-0 bg-black/60"
        onClick={onClose}
      />

      <div className="absolute top-0 right-0 h-full w-full max-w-3xl bg-slate-900 border-l border-slate-800 shadow-2xl">

        <div className="h-full flex flex-col">

          {/* Header */}

          <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800">

            <div>
              <h2 className="text-lg font-semibold text-slate-200">
                Camera Details
              </h2>

              <p className="text-sm text-slate-500">
                {camera.name}
              </p>
            </div>

            <div className="flex gap-2">

              <button
                onClick={() =>
                  void refreshCurrentTab()
                }
                className="p-2 rounded bg-slate-800 hover:bg-slate-700"
              >
                <RefreshCw
                  size={16}
                />
              </button>

              <button
                onClick={onClose}
                className="p-2 rounded bg-slate-800 hover:bg-slate-700"
              >
                <X size={16} />
              </button>

            </div>

          </div>

          {/* Tabs */}

          <div className="flex border-b border-slate-800 overflow-x-auto">

            {tabs.map((tab) => {
              const Icon =
                tab.icon;

              return (
                <button
                  key={tab.key}
                  onClick={() =>
                    setActiveTab(
                      tab.key
                    )
                  }
                  className={`px-4 py-3 text-sm flex items-center gap-2 border-b-2 whitespace-nowrap ${
                    activeTab ===
                    tab.key
                      ? 'border-indigo-500 text-white'
                      : 'border-transparent text-slate-400 hover:text-slate-200'
                  }`}
                >
                  <Icon
                    size={16}
                  />

                  {tab.label}
                </button>
              );
            })}

          </div>

          {/* Content */}

          <div className="flex-1 overflow-auto p-6">

            {loading && (
              <div className="text-slate-400">
                Loading...
              </div>
            )}

            {error && (
              <div className="border border-red-800 bg-red-950/20 rounded-lg p-4 text-red-400">
                {error}
              </div>
            )}

            {!loading &&
              !error &&
              activeTab ===
                'general' && (
                <div className="space-y-2">
                  <CameraSnapshot
                    cameraId={camera.id}
                    autoRefresh
                  />

                  <InfoRow
                    label="Name"
                    value={
                      camera.name
                    }
                  />

                  <InfoRow
                    label="Description"
                    value={
                      camera.description ||
                      '-'
                    }
                  />

                  <InfoRow
                    label="Status"
                    value={
                      camera.status
                    }
                  />

                  <InfoRow
                    label="Site"
                    value={
                      camera.site_id
                    }
                  />

                  <div className="mt-6">

                    <h3 className="text-sm font-semibold text-slate-300 mb-2">
                      Device Information
                    </h3>

                    <pre className="bg-slate-950 rounded p-4 text-xs overflow-auto">
                      {JSON.stringify(
                        general,
                        null,
                        2
                      )}
                    </pre>

                  </div>

                </div>
              )}

            {!loading &&
              !error &&
              activeTab ===
                'streams' && (
                <pre className="bg-slate-950 rounded p-4 text-xs overflow-auto">
                  {JSON.stringify(
                    streams,
                    null,
                    2
                  )}
                </pre>
              )}

            {!loading &&
              !error &&
              activeTab ===
                'network' && (
                <div className="space-y-2">

                  <InfoRow
                    label="IP Address"
                    value={
                      network?.ip_address
                    }
                  />

                  <InfoRow
                    label="RTSP Port"
                    value={
                      network?.rtsp_port
                    }
                  />

                  <InfoRow
                    label="ONVIF Port"
                    value={
                      network?.onvif_port
                    }
                  />

                  <InfoRow
                    label="ONVIF"
                    value={
                      camera?.config
                        ? (() => {
                            try {
                              const cfg = JSON.parse(camera.config);
                              return cfg.is_onvif !== false
                                ? `Enabled (Port ${cfg.onvif_port || 8000})`
                                : 'Disabled';
                            } catch {
                              return 'Enabled (Port 80)';
                            }
                          })()
                        : 'Enabled (Port 80)'
                    }
                  />

                  <InfoRow
                    label="HTTP Port"
                    value={
                      network?.http_port
                    }
                  />

                  <InfoRow
                    label="DHCP"
                    value={
                      network?.dhcp
                        ? 'Enabled'
                        : 'Disabled'
                    }
                  />

                  {network?.interfaces
                    ?.length >
                    0 && (
                    <div className="mt-6">

                      <h3 className="text-sm font-semibold text-slate-300 mb-2">
                        Interfaces
                      </h3>

                      <pre className="bg-slate-950 rounded p-4 text-xs overflow-auto">
                        {JSON.stringify(
                          network.interfaces,
                          null,
                          2
                        )}
                      </pre>

                    </div>
                  )}

                </div>
              )}

            {!loading &&
              !error &&
              activeTab ===
                'diagnostics' && (
                <div className="space-y-2">

                  <InfoRow
                    label="Reachable"
                    value={
                      diagnostics?.reachable
                        ? 'Yes'
                        : 'No'
                    }
                  />

                  <InfoRow
                    label="RTSP"
                    value={
                      diagnostics?.rtsp
                        ? 'OK'
                        : 'Failed'
                    }
                  />

                  <InfoRow
                    label="ONVIF"
                    value={
                      diagnostics?.onvif_enabled === false
                        ? 'N/A (disabled)'
                        : diagnostics?.onvif
                          ? 'OK'
                          : 'Failed'
                    }
                  />

                  <InfoRow
                    label="Status"
                    value={
                      diagnostics?.status
                    }
                  />

                  <InfoRow
                    label="Latency"
                    value={`${diagnostics?.latency_ms ?? 0} ms`}
                  />

                  <InfoRow
                    label="Response Time"
                    value={`${diagnostics?.response_time_ms ?? 0} ms`}
                  />

                  <InfoRow
                    label="Uptime"
                    value={`${diagnostics?.uptime_pct ?? 0}%`}
                  />

                </div>
              )}

            {!loading &&
              !error &&
              activeTab ===
                'recording' && (
                <div className="space-y-2">

                  <InfoRow
                    label="Retention Days"
                    value={
                      recording?.retention_days
                    }
                  />

                  <InfoRow
                    label="Recordings"
                    value={
                      recording?.recordings_count
                    }
                  />

                  <InfoRow
                    label="Oldest Recording"
                    value={
                      recording?.oldest_recording ||
                      '-'
                    }
                  />

                  <InfoRow
                    label="Latest Recording"
                    value={
                      recording?.latest_recording ||
                      '-'
                    }
                  />

                  <InfoRow
                    label="Recording Enabled"
                    value={
                      recording?.recording_enabled
                        ? 'Yes'
                        : 'No'
                    }
                  />

                  <InfoRow
                    label="Storage Used"
                    value={
                      recording?.storage_used_bytes
                    }
                  />

                </div>
              )}

            {!loading &&
              !error &&
              activeTab ===
                'onvif' && (
                <div className="space-y-2">

                  <InfoRow
                    label="Username"
                    value={
                      onvif?.username
                    }
                  />

                  <InfoRow
                    label="Device URI"
                    value={
                      onvif?.device_uri
                    }
                  />

                  <InfoRow
                    label="Events"
                    value={
                      onvif?.events_supported
                        ? 'Supported'
                        : 'Unsupported'
                    }
                  />

                  <InfoRow
                    label="Analytics"
                    value={
                      onvif?.analytics_supported
                        ? 'Supported'
                        : 'Unsupported'
                    }
                  />

                  <InfoRow
                    label="PTZ"
                    value={
                      onvif?.ptz
                        ? 'Supported'
                        : 'Unsupported'
                    }
                  />

                  <InfoRow
                    label="Imaging"
                    value={
                      onvif?.imaging
                        ? 'Supported'
                        : 'Unsupported'
                    }
                  />

                  <div className="mt-6">

                    <h3 className="text-sm font-semibold text-slate-300 mb-2">
                      Capabilities
                    </h3>

                    <pre className="bg-slate-950 rounded p-4 text-xs overflow-auto">
                      {JSON.stringify(
                        onvif?.capabilities ||
                          {},
                        null,
                        2
                      )}
                    </pre>

                  </div>

                </div>
              )}

          </div>

        </div>

      </div>

    </div>
  );
}
