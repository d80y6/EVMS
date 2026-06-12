import {
  LayoutGrid,
  Table,
} from 'lucide-react';

export type CameraViewMode =
  | 'table'
  | 'cards';

interface ViewModeToggleProps {
  value: CameraViewMode;

  onChange: (
    mode: CameraViewMode
  ) => void;
}

export default function ViewModeToggle({
  value,
  onChange,
}: ViewModeToggleProps) {
  return (
    <div className="flex rounded-lg overflow-hidden border border-slate-700">

      <button
        onClick={() =>
          onChange('table')
        }
        className={`px-3 py-2 flex items-center gap-2 text-sm ${
          value === 'table'
            ? 'bg-indigo-600 text-white'
            : 'bg-slate-800 text-slate-400'
        }`}
      >
        <Table size={16} />
        Table
      </button>

      <button
        onClick={() =>
          onChange('cards')
        }
        className={`px-3 py-2 flex items-center gap-2 text-sm ${
          value === 'cards'
            ? 'bg-indigo-600 text-white'
            : 'bg-slate-800 text-slate-400'
        }`}
      >
        <LayoutGrid
          size={16}
        />
        Cards
      </button>

    </div>
  );
}
