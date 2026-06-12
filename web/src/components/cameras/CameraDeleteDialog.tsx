interface CameraDeleteDialogProps {
  open: boolean;

  title?: string;

  message: string;

  deleting?: boolean;

  onConfirm: () => Promise<void>;

  onClose: () => void;
}

export default function CameraDeleteDialog({
  open,
  title = 'Delete Camera',
  message,
  deleting = false,
  onConfirm,
  onClose,
}: CameraDeleteDialogProps) {
  if (!open) {
    return null;
  }

  return (
    <div className="fixed inset-0 bg-black/70 z-50 flex items-center justify-center p-4">

      <div
        role="dialog"
        aria-modal="true"
        className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-xl"
      >

        <div className="px-6 py-4 border-b border-slate-800">

          <h2 className="text-lg font-semibold text-slate-200">
            {title}
          </h2>

        </div>

        <div className="p-6">

          <p className="text-sm text-slate-400">
            {message}
          </p>

        </div>

        <div className="px-6 py-4 border-t border-slate-800 flex justify-end gap-2">

          <button
            onClick={onClose}
            disabled={deleting}
            className="px-4 py-2 rounded bg-slate-700 hover:bg-slate-600 text-white text-sm"
          >
            Cancel
          </button>

          <button
            onClick={() => void onConfirm()}
            disabled={deleting}
            className="px-4 py-2 rounded bg-red-700 hover:bg-red-600 disabled:bg-red-900 text-white text-sm"
          >
            {deleting
              ? 'Deleting...'
              : 'Delete'}
          </button>

        </div>

      </div>

    </div>
  );
}
