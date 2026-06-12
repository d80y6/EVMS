import { useState } from 'react';

interface CameraBulkActionsProps {
  selectedCount: number;

  onDelete: () => Promise<void>;

  onClearSelection: () => void;
}

export default function CameraBulkActions({
  selectedCount,
  onDelete,
  onClearSelection,
}: CameraBulkActionsProps) {
  const [deleting, setDeleting] =
    useState(false);

  const handleDelete =
    async () => {
      if (
        selectedCount === 0
      ) {
        return;
      }

      const confirmed =
        window.confirm(
          `Delete ${selectedCount} selected camera(s)?`
        );

      if (!confirmed) {
        return;
      }

      try {
        setDeleting(true);

        await onDelete();
      } finally {
        setDeleting(false);
      }
    };

  if (selectedCount === 0) {
    return null;
  }

  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl px-4 py-3 flex items-center justify-between">

      <div className="text-sm text-slate-400">

        {selectedCount}{' '}
        camera(s) selected

      </div>

      <div className="flex gap-2">

        <button
          onClick={
            onClearSelection
          }
          className="px-3 py-2 rounded bg-slate-700 hover:bg-slate-600 text-white text-sm"
        >
          Clear
        </button>

        <button
          disabled={deleting}
          onClick={
            handleDelete
          }
          className="px-3 py-2 rounded bg-red-700 hover:bg-red-600 disabled:bg-red-900 text-white text-sm"
        >
          {deleting
            ? 'Deleting...'
            : 'Delete Selected'}
        </button>

      </div>

    </div>
  );
}
