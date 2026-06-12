interface Site {
  id: string;
  name: string;
  location: string;
}

type StatusFilter =
  | 'all'
  | 'online'
  | 'offline';

interface CameraFiltersProps {
  search: string;

  siteFilter: string;

  statusFilter: StatusFilter;

  sites: Site[];

  onSearchChange: (
    value: string
  ) => void;

  onSiteChange: (
    value: string
  ) => void;

  onStatusChange: (
    value: StatusFilter
  ) => void;
}

export default function CameraFilters({
  search,
  siteFilter,
  statusFilter,
  sites,
  onSearchChange,
  onSiteChange,
  onStatusChange,
}: CameraFiltersProps) {
  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl p-4">

      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">

        <input
          type="text"
          value={search}
          placeholder="Search cameras..."
          onChange={(e) =>
            onSearchChange(
              e.target.value
            )
          }
          className="bg-slate-800 border border-slate-700 rounded px-3 py-2 text-sm text-slate-300"
        />

        <select
          value={siteFilter}
          onChange={(e) =>
            onSiteChange(
              e.target.value
            )
          }
          className="bg-slate-800 border border-slate-700 rounded px-3 py-2 text-sm text-slate-300"
        >
          <option value="">
            All Sites
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

        <select
          value={statusFilter}
          onChange={(e) =>
            onStatusChange(
              e.target
                .value as StatusFilter
            )
          }
          className="bg-slate-800 border border-slate-700 rounded px-3 py-2 text-sm text-slate-300"
        >
          <option value="all">
            All Status
          </option>

          <option value="online">
            Online
          </option>

          <option value="offline">
            Offline
          </option>

        </select>

      </div>

    </div>
  );
}
