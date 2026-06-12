import { useEffect, useMemo, useState } from 'react';
import {
  Camera,
  CheckCircle,
  AlertTriangle,
  MapPin,
  Shield,
  HardDrive,
  RefreshCw,
} from 'lucide-react';

import { api, Camera as CameraModel } from '../api/client';

interface Site {
  id: string;
  name: string;
  location: string;
}

export default function CameraHealthPage() {
  const [loading, setLoading] =
    useState(true);

  const [error, setError] =
    useState<string | null>(null);

  const [cameras, setCameras] =
    useState<CameraModel[]>([]);

  const [sites, setSites] =
    useState<Site[]>([]);

  const loadData = async () => {
    try {
      setLoading(true);

      const [
        cameraData,
        siteData,
      ] = await Promise.all([
        api.listCameras(),
        api.getSites(),
      ]);

      setCameras(cameraData);

      setSites(
        siteData.sites || []
      );

      setError(null);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to load health data'
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, []);

  const stats =
    useMemo(() => {
      const total =
        cameras.length;

      const online =
        cameras.filter(
          (c) =>
            c.status ===
            'online'
        ).length;

      const offline =
        cameras.filter(
          (c) =>
            c.status !==
            'online'
        ).length;

      const ptz =
        cameras.filter(
          (c) =>
            c.ptz_protocol &&
            c.ptz_protocol !==
              'none'
        ).length;

      const retention =
        cameras.reduce(
          (
            sum,
            cam
          ) =>
            sum +
            (cam.retention_days ||
              0),
          0
        );

      return {
        total,
        online,
        offline,
        ptz,
        retentionAvg:
          total > 0
            ? Math.round(
                retention /
                  total
              )
            : 0,
      };
    }, [cameras]);

  const siteMap =
    useMemo(() => {
      const map: Record<string, string> = {};
      sites.forEach(s => { map[s.id] = s.name; });
      return map;
    }, [sites]);

  const siteStats =
    useMemo(() => {
      return sites.map(
        (site) => ({
          ...site,
          count:
            cameras.filter(
              (
                camera
              ) =>
                camera.site_id ===
                site.id
            ).length,
        })
      );
    }, [
      sites,
      cameras,
    ]);

  if (loading) {
    return (
      <div className="p-6 text-slate-400">
        Loading camera health...
      </div>
    );
  }

  return (
    <div className="space-y-6">

      {/* Header */}

      <div className="flex items-center justify-between">

        <div>

          <h1 className="text-2xl font-bold text-slate-200">
            Camera Health
          </h1>

          <p className="text-slate-500">
            Fleet overview and
            operational status
          </p>

        </div>

        <button
          onClick={() =>
            void loadData()
          }
          className="px-3 py-2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 flex items-center gap-2"
        >
          <RefreshCw
            size={16}
          />

          Refresh
        </button>

      </div>

      {error && (
        <div className="border border-red-800 bg-red-950/20 rounded-xl p-4 text-red-400">
          {error}
        </div>
      )}

      {/* KPI Cards */}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">

        <StatCard
          title="Total Cameras"
          value={
            stats.total
          }
          icon={Camera}
        />

        <StatCard
          title="Online"
          value={
            stats.online
          }
          icon={
            CheckCircle
          }
          color="green"
        />

        <StatCard
          title="Offline"
          value={
            stats.offline
          }
          icon={
            AlertTriangle
          }
          color="red"
        />

        <StatCard
          title="PTZ Enabled"
          value={stats.ptz}
          icon={Shield}
        />

        <StatCard
          title="Avg Retention"
          value={`${stats.retentionAvg}d`}
          icon={
            HardDrive
          }
        />

      </div>

      {/* Health Ratio */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6">

        <h2 className="text-lg font-semibold text-slate-200 mb-4">
          Health Overview
        </h2>

        <div className="space-y-3">

          <div className="flex justify-between text-sm">

            <span className="text-slate-500">
              Online
            </span>

            <span className="text-slate-300">
              {
                stats.online
              }{' '}
              /{' '}
              {
                stats.total
              }
            </span>

          </div>

          <div className="w-full bg-slate-800 rounded-full h-4">

            <div
              className="h-4 rounded-full bg-green-500"
              style={{
                width: `${
                  stats.total
                    ? (stats.online /
                        stats.total) *
                      100
                    : 0
                }%`,
              }}
            />

          </div>

        </div>

      </div>

      {/* Sites */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">

        <div className="px-6 py-4 border-b border-slate-800">

          <h2 className="text-lg font-semibold text-slate-200">
            Site Distribution
          </h2>

        </div>

        <table className="w-full text-sm">

          <thead>

            <tr className="border-b border-slate-800 text-left text-slate-400">

              <th className="p-4">
                Site
              </th>

              <th className="p-4">
                Location
              </th>

              <th className="p-4">
                Cameras
              </th>

            </tr>

          </thead>

          <tbody>

            {siteStats.map(
              (site) => (
                <tr
                  key={
                    site.id
                  }
                  className="border-b border-slate-800 hover:bg-slate-800/40"
                >

                  <td className="p-4 text-slate-200">
                    {
                      site.name
                    }
                  </td>

                  <td className="p-4 text-slate-400">

                    <div className="flex items-center gap-2">

                      <MapPin
                        size={
                          14
                        }
                      />

                      {
                        site.location
                      }

                    </div>

                  </td>

                  <td className="p-4 text-slate-300">
                    {
                      site.count
                    }
                  </td>

                </tr>
              )
            )}

          </tbody>

        </table>

      </div>

      {/* Offline Cameras */}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">

        <div className="px-6 py-4 border-b border-slate-800">

          <h2 className="text-lg font-semibold text-slate-200">
            Offline Cameras
          </h2>

        </div>

        {cameras.filter(
          (c) =>
            c.status !==
            'online'
        ).length ===
        0 ? (
          <div className="p-6 text-green-400">
            All cameras are
            online.
          </div>
        ) : (
          <table className="w-full text-sm">

            <thead>

              <tr className="border-b border-slate-800 text-left text-slate-400">

                <th className="p-4">
                  Name
                </th>

                <th className="p-4">
                  Site
                </th>

                <th className="p-4">
                  Status
                </th>

              </tr>

            </thead>

            <tbody>

              {cameras
                .filter(
                  (c) =>
                    c.status !==
                    'online'
                )
                .map(
                  (
                    camera
                  ) => (
                    <tr
                      key={
                        camera.id
                      }
                      className="border-b border-slate-800"
                    >

                      <td className="p-4 text-slate-200">
                        {
                          camera.name
                        }
                      </td>

                      <td className="p-4 text-slate-400">
                        {siteMap[camera.site_id] || camera.site_id}
                      </td>

                      <td className="p-4">

                        <span className="px-2 py-1 rounded bg-red-900/30 text-red-400 text-xs">
                          {
                            camera.status
                          }
                        </span>

                      </td>

                    </tr>
                  )
                )}

            </tbody>

          </table>
        )}

      </div>

    </div>
  );
}

function StatCard({
  title,
  value,
  icon: Icon,
  color,
}: {
  title: string;
  value: string | number;
  icon: any;
  color?: 'green' | 'red';
}) {
  const colorClass =
    color === 'green'
      ? 'text-green-400'
      : color === 'red'
      ? 'text-red-400'
      : 'text-indigo-400';

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">

      <div className="flex justify-between items-center">

        <div>

          <p className="text-sm text-slate-500">
            {title}
          </p>

          <p className="text-2xl font-bold text-slate-200 mt-2">
            {value}
          </p>

        </div>

        <Icon
          size={24}
          className={
            colorClass
          }
        />

      </div>

    </div>
  );
}
