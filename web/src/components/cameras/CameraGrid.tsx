import { Eye, Edit, Trash2 } from 'lucide-react';
import { Camera } from '../../api/client';

interface Site {
  id: string;
  name: string;
  location: string;
}

interface CameraGridProps {
  cameras: Camera[];
  sites: Site[];

  selected: string[];

  onSelect: (id: string) => void;

  onSelectAll: () => void;

  onDetails: (camera: Camera) => void;

  onEdit: (camera: Camera) => void;

  onDelete: (camera: Camera) => void;
}

export default function CameraGrid({
  cameras,
  sites,
  selected,
  onSelect,
  onSelectAll,
  onDetails,
  onEdit,
  onDelete,
}: CameraGridProps) {
  const siteMap = Object.fromEntries(
    sites.map((site) => [
      site.id,
      site,
    ])
  );

  if (cameras.length === 0) {
    return (
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-8 text-center text-slate-500">
        No cameras found
      </div>
    );
  }

  const allSelected =
    cameras.length > 0 &&
    selected.length ===
      cameras.length;

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">

      <table className="w-full text-sm">

        <thead>

          <tr className="border-b border-slate-800 text-left text-slate-400">

            <th className="p-3 w-10">

              <input
                type="checkbox"
                checked={
                  allSelected
                }
                onChange={
                  onSelectAll
                }
              />

            </th>

            <th className="p-3">
              Name
            </th>

            <th className="p-3">
              Site
            </th>

            <th className="p-3">
              Status
            </th>

            <th className="p-3">
              PTZ
            </th>

            <th className="p-3">
              Retention
            </th>

            <th className="p-3 text-right">
              Actions
            </th>

          </tr>

        </thead>

        <tbody>

          {cameras.map(
            (camera) => {
              const site =
                siteMap[
                  camera.site_id
                ];

              return (
                <tr
                  key={
                    camera.id
                  }
                  className="border-b border-slate-800 hover:bg-slate-800/40"
                >

                  <td className="p-3">

                    <input
                      type="checkbox"
                      checked={selected.includes(
                        camera.id
                      )}
                      onChange={() =>
                        onSelect(
                          camera.id
                        )
                      }
                    />

                  </td>

                  <td className="p-3">

                    <div>

                      <button
                        onClick={() =>
                          onDetails(
                            camera
                          )
                        }
                        className="font-medium text-slate-200 hover:text-indigo-400"
                      >
                        {
                          camera.name
                        }
                      </button>

                      {camera.description && (
                        <div className="text-xs text-slate-500 mt-1">
                          {
                            camera.description
                          }
                        </div>
                      )}

                    </div>

                  </td>

                  <td className="p-3 text-slate-300">
                    {site?.name ||
                      camera.site_id}
                  </td>

                  <td className="p-3">

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

                  </td>

                  <td className="p-3 text-slate-300">
                    {
                      camera.ptz_protocol
                    }
                  </td>

                  <td className="p-3 text-slate-300">
                    {
                      camera.retention_days
                    }
                    d
                  </td>

                  <td className="p-3">

                    <div className="flex justify-end gap-2">

                      <button
                        onClick={() =>
                          onDetails(
                            camera
                          )
                        }
                        className="p-2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300"
                        title="Details"
                      >
                        <Eye
                          size={
                            16
                          }
                        />
                      </button>

                      <button
                        onClick={() =>
                          onEdit(
                            camera
                          )
                        }
                        className="p-2 rounded bg-indigo-600 hover:bg-indigo-500 text-white"
                        title="Edit"
                      >
                        <Edit
                          size={
                            16
                          }
                        />
                      </button>

                      <button
                        onClick={() =>
                          onDelete(
                            camera
                          )
                        }
                        className="p-2 rounded bg-red-700 hover:bg-red-600 text-white"
                        title="Delete"
                      >
                        <Trash2
                          size={
                            16
                          }
                        />
                      </button>

                    </div>

                  </td>

                </tr>
              );
            }
          )}

        </tbody>

      </table>

    </div>
  );
}
