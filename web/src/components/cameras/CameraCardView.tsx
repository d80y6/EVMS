import { Camera, Edit, Trash2, Eye, MapPin } from 'lucide-react';
import { Camera as CameraModel } from '../../api/client';
import CameraSnapshot from './CameraSnapshot';

interface Site {
  id: string;
  name: string;
  location: string;
}

interface CameraCardViewProps {
  cameras: CameraModel[];
  sites: Site[];

  selected: string[];

  onSelect: (id: string) => void;

  onEdit: (camera: CameraModel) => void;

  onDelete: (camera: CameraModel) => void;

  onDetails: (camera: CameraModel) => void;
}

export default function CameraCardView({
  cameras,
  sites,
  selected,
  onSelect,
  onEdit,
  onDelete,
  onDetails,
}: CameraCardViewProps) {
  const siteMap = Object.fromEntries(
    sites.map((site) => [
      site.id,
      site,
    ])
  );

  if (cameras.length === 0) {
    return (
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-12 text-center">
        <Camera
          className="mx-auto mb-4 text-slate-600"
          size={40}
        />

        <p className="text-slate-500">
          No cameras found
        </p>
      </div>
    );
  }

  return (
    <div className="grid gap-4 grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">

      {cameras.map((camera) => {
        const site =
          siteMap[camera.site_id];

        const isSelected =
          selected.includes(
            camera.id
          );

        return (
          <div
            key={camera.id}
            className={`rounded-xl border transition-all ${
              isSelected
                ? 'border-indigo-500 bg-slate-850'
                : 'border-slate-800 bg-slate-900 hover:border-slate-700'
            }`}
          >
          <CameraSnapshot
            cameraId={camera.id}
            className="mb-4"
          />

            <div className="p-4">

              <div className="flex items-start justify-between">

                <div className="flex gap-3">

                  <input
                    type="checkbox"
                    checked={
                      isSelected
                    }
                    onChange={() =>
                      onSelect(
                        camera.id
                      )
                    }
                    className="mt-1"
                  />

                  <div>

                    <h3 className="font-semibold text-slate-200">
                      {camera.name}
                    </h3>

                    {camera.description && (
                      <p className="mt-1 text-xs text-slate-500 line-clamp-2">
                        {
                          camera.description
                        }
                      </p>
                    )}

                  </div>

                </div>

                <span
                  className={`px-2 py-1 rounded-full text-xs font-medium ${
                    camera.status ===
                    'online'
                      ? 'bg-green-900/40 text-green-400'
                      : 'bg-red-900/40 text-red-400'
                  }`}
                >
                  {
                    camera.status
                  }
                </span>

              </div>

              <div className="mt-4 space-y-3">

                <div className="flex justify-between">

                  <span className="text-slate-500 text-sm">
                    PTZ
                  </span>

                  <span className="text-slate-300 text-sm">
                    {
                      camera.ptz_protocol
                    }
                  </span>

                </div>

                <div className="flex justify-between">

                  <span className="text-slate-500 text-sm">
                    Retention
                  </span>

                  <span className="text-slate-300 text-sm">
                    {
                      camera.retention_days
                    }
                    d
                  </span>

                </div>

                <div className="flex justify-between">

                  <span className="text-slate-500 text-sm">
                    Site
                  </span>

                  <span className="text-slate-300 text-sm">
                    {site?.name ||
                      camera.site_id}
                  </span>

                </div>

                {site?.location && (
                  <div className="flex gap-2 text-xs text-slate-500">

                    <MapPin
                      size={14}
                    />

                    <span>
                      {
                        site.location
                      }
                    </span>

                  </div>
                )}

              </div>

            </div>

            <div className="border-t border-slate-800 p-3">

              <div className="flex gap-2">

                <button
                  onClick={() =>
                    onDetails(
                      camera
                    )
                  }
                  className="flex-1 px-3 py-2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm flex items-center justify-center gap-2"
                >
                  <Eye size={16} />
                  Details
                </button>

                <button
                  onClick={() =>
                    onEdit(
                      camera
                    )
                  }
                  className="px-3 py-2 rounded bg-indigo-600 hover:bg-indigo-500 text-white"
                >
                  <Edit
                    size={16}
                  />
                </button>

                <button
                  onClick={() =>
                    onDelete(
                      camera
                    )
                  }
                  className="px-3 py-2 rounded bg-red-700 hover:bg-red-600 text-white"
                >
                  <Trash2
                    size={16}
                  />
                </button>

              </div>

            </div>

          </div>
        );
      })}

    </div>
  );
}
