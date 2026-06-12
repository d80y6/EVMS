import { useEffect, useMemo, useState } from 'react';

import { api, Camera } from '../api/client';

import CameraGrid from '../components/cameras/CameraGrid';
import CameraCardView from '../components/cameras/CameraCardView';
import CameraDialog from '../components/cameras/CameraDialog';
import CameraDeleteDialog from '../components/cameras/CameraDeleteDialog';
import CameraBulkActions from '../components/cameras/CameraBulkActions';
import CameraFilters from '../components/cameras/CameraFilters';
import CameraDetailsDrawer from '../components/cameras/CameraDetailsDrawer';
import CameraDiscoveryWizard from '../components/cameras/CameraDiscoveryWizard';
import ViewModeToggle, {
  CameraViewMode,
} from '../components/cameras/ViewModeToggle';

interface Site {
  id: string;
  name: string;
  location: string;
}

type StatusFilter =
  | 'all'
  | 'online'
  | 'offline';

export default function CamerasPage() {
  const [loading, setLoading] =
    useState(true);

  const [refreshing, setRefreshing] =
    useState(false);

  const [error, setError] =
    useState<string | null>(null);

  const [cameras, setCameras] =
    useState<Camera[]>([]);

  const [sites, setSites] =
    useState<Site[]>([]);

  const [selected, setSelected] =
    useState<string[]>([]);

  const [search, setSearch] =
    useState('');

  const [siteFilter, setSiteFilter] =
    useState('');

  const [statusFilter, setStatusFilter] =
    useState<StatusFilter>('all');

  const [viewMode, setViewMode] =
    useState<CameraViewMode>('table');

  const [dialogOpen, setDialogOpen] =
    useState(false);

  const [editingCamera, setEditingCamera] =
    useState<Camera | null>(null);

  const [detailsOpen, setDetailsOpen] =
    useState(false);

  const [detailsCamera, setDetailsCamera] =
    useState<Camera | null>(null);

  const [discoveryOpen, setDiscoveryOpen] =
    useState(false);

  const [deleteOpen, setDeleteOpen] =
    useState(false);

  const [deleteTarget, setDeleteTarget] =
    useState<Camera | null>(null);

  const [deleting, setDeleting] =
    useState(false);

  const loadData = async (
    silent = false
  ) => {
    try {
      if (silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }

      const [cameraData, siteData] =
        await Promise.all([
          api.listCameras(),
          api.getSites(),
        ]);

      setCameras(cameraData);
      setSites(siteData.sites || []);
      setError(null);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Failed to load cameras'
      );
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    void loadData();
  }, []);

  const filteredCameras =
    useMemo(() => {
      return cameras.filter(
        (camera) => {
          const matchesSearch =
            !search ||
            camera.name
              .toLowerCase()
              .includes(
                search.toLowerCase()
              );

          const matchesSite =
            !siteFilter ||
            camera.site_id ===
              siteFilter;

          const matchesStatus =
            statusFilter === 'all' ||
            camera.status ===
              statusFilter;

          return (
            matchesSearch &&
            matchesSite &&
            matchesStatus
          );
        }
      );
    }, [
      cameras,
      search,
      siteFilter,
      statusFilter,
    ]);

  const toggleSelect = (
    id: string
  ) => {
    setSelected((prev) =>
      prev.includes(id)
        ? prev.filter(
            (v) => v !== id
          )
        : [...prev, id]
    );
  };

  const toggleSelectAll = () => {
    if (
      selected.length ===
      filteredCameras.length
    ) {
      setSelected([]);
      return;
    }

    setSelected(
      filteredCameras.map(
        (camera) => camera.id
      )
    );
  };

  const handleAdd = () => {
    setEditingCamera(null);
    setDialogOpen(true);
  };

  const handleEdit = (
    camera: Camera
  ) => {
    setEditingCamera(camera);
    setDialogOpen(true);
  };

  const handleDetails = (
    camera: Camera
  ) => {
    setDetailsCamera(camera);
    setDetailsOpen(true);
  };

  const handleDelete = (
    camera: Camera
  ) => {
    setDeleteTarget(camera);
    setDeleteOpen(true);
  };

  const confirmDelete =
    async () => {
      if (!deleteTarget) {
        return;
      }

      try {
        setDeleting(true);

        await api.deleteCamera(
          deleteTarget.id
        );

        await loadData(true);

        setDeleteOpen(false);
        setDeleteTarget(null);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Delete failed'
        );
      } finally {
        setDeleting(false);
      }
    };

  const handleBulkDelete =
    async () => {
      await Promise.all(
        selected.map((id) =>
          api.deleteCamera(id)
        )
      );

      setSelected([]);

      await loadData(true);
    };

  if (loading) {
    return (
      <div className="p-6 text-slate-400">
        Loading cameras...
      </div>
    );
  }

  return (
    <div className="space-y-6">

      {/* Header */}

      <div className="flex items-center justify-between">

        <div>
          <h1 className="text-xl font-semibold text-slate-200">
            Cameras
          </h1>

          <p className="text-sm text-slate-500">
            Camera inventory and
            device management
          </p>
        </div>

        <div className="flex gap-2">

          <ViewModeToggle
            value={viewMode}
            onChange={
              setViewMode
            }
          />

          <button
            onClick={() =>
              loadData(true)
            }
            className="px-3 py-2 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-sm"
          >
            {refreshing
              ? 'Refreshing...'
              : 'Refresh'}
          </button>

          <button
            onClick={() =>
              setDiscoveryOpen(
                true
              )
            }
            className="px-3 py-2 rounded bg-green-600 hover:bg-green-500 text-white text-sm"
          >
            Discovery
          </button>

          <button
            onClick={
              handleAdd
            }
            className="px-3 py-2 rounded bg-indigo-600 hover:bg-indigo-500 text-white text-sm"
          >
            + Add Camera
          </button>

        </div>

      </div>

      {error && (
        <div className="border border-red-800 bg-red-950/20 rounded-xl p-4 text-red-400">
          {error}
        </div>
      )}

      <CameraFilters
        search={search}
        siteFilter={siteFilter}
        statusFilter={
          statusFilter
        }
        sites={sites}
        onSearchChange={
          setSearch
        }
        onSiteChange={
          setSiteFilter
        }
        onStatusChange={
          setStatusFilter
        }
      />

      <CameraBulkActions
        selectedCount={
          selected.length
        }
        onDelete={
          handleBulkDelete
        }
        onClearSelection={() =>
          setSelected([])
        }
      />

      {viewMode ===
      'table' ? (
        <CameraGrid
          cameras={
            filteredCameras
          }
          sites={sites}
          selected={selected}
          onSelect={
            toggleSelect
          }
          onSelectAll={
            toggleSelectAll
          }
          onEdit={
            handleEdit
          }
          onDelete={
            handleDelete
          }
          onDetails={
            handleDetails
          }
        />
      ) : (
        <CameraCardView
          cameras={
            filteredCameras
          }
          sites={sites}
          selected={selected}
          onSelect={
            toggleSelect
          }
          onEdit={
            handleEdit
          }
          onDelete={
            handleDelete
          }
          onDetails={
            handleDetails
          }
        />
      )}

      <CameraDialog
        open={dialogOpen}
        camera={
          editingCamera
        }
        sites={sites}
        onClose={() =>
          setDialogOpen(
            false
          )
        }
        onSaved={async () => {
          await loadData(
            true
          );
        }}
      />

      <CameraDeleteDialog
        open={deleteOpen}
        deleting={deleting}
        title="Delete Camera"
        message={
          deleteTarget
            ? `Delete "${deleteTarget.name}"?`
            : ''
        }
        onConfirm={
          confirmDelete
        }
        onClose={() => {
          setDeleteOpen(
            false
          );
          setDeleteTarget(
            null
          );
        }}
      />

      <CameraDetailsDrawer
        open={detailsOpen}
        camera={
          detailsCamera
        }
        onClose={() =>
          setDetailsOpen(
            false
          )
        }
      />

      <CameraDiscoveryWizard
        open={
          discoveryOpen
        }
        sites={sites}
        onClose={() =>
          setDiscoveryOpen(
            false
          )
        }
        onImported={async () => {
          await loadData(
            true
          );
        }}
      />

    </div>
  );
}
