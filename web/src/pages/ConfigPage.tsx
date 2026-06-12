import { useState, useEffect } from 'react';
import { api } from '../api/client';
import { useAuth } from '../context/AuthContext';

const CONFIG_CATEGORIES = ['general', 'retention', 'security', 'notifications', 'storage', 'ai'];

export default function ConfigPage() {
  const { role } = useAuth();
  const [category, setCategory] = useState('general');
  const [config, setConfig] = useState<Record<string, { value: any; type: string; description: string }>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [editValues, setEditValues] = useState<Record<string, string>>({});
  const [history, setHistory] = useState<any[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const [importData, setImportData] = useState('');
  const [showImport, setShowImport] = useState(false);
  const [showExport, setShowExport] = useState(false);
  const [exportData, setExportData] = useState('');

  useEffect(() => {
    setLoading(true);
    setError(null);
    api.getConfigCategory(category)
      .then((data) => {
        setConfig(data.config || {});
        const vals: Record<string, string> = {};
        Object.entries(data.config || {}).forEach(([k, v]: [string, any]) => {
          vals[k] = String(v.value ?? '');
        });
        setEditValues(vals);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [category]);

  if (role !== 'admin') {
    return <div className="flex items-center justify-center h-64"><p className="text-red-400">Access denied. Admin role required.</p></div>;
  }

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const updates: Record<string, any> = {};
      Object.entries(editValues).forEach(([k, v]) => {
        const entry = config[k];
        if (entry) {
          if (entry.type === 'number' || entry.type === 'integer') updates[k] = Number(v);
          else if (entry.type === 'boolean') updates[k] = v === 'true';
          else updates[k] = v;
        }
      });
      await api.updateConfig(category, updates);
      setSuccess('Configuration saved successfully');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const handleExport = async () => {
    try {
      const data = await api.exportConfig();
      setExportData(JSON.stringify(data.config, null, 2));
      setShowExport(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Export failed');
    }
  };

  const handleImport = async () => {
    try {
      const parsed = JSON.parse(importData);
      const res = await api.importConfig(parsed);
      setSuccess(`Imported ${res.count} configuration values`);
      setShowImport(false);
      setImportData('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed');
    }
  };

  const handleViewHistory = async () => {
    try {
      const data = await api.getConfigHistory(category);
      setHistory(data.entries || []);
      setShowHistory(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load history');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">System Configuration</h2>
        <div className="flex gap-2">
          <button onClick={() => setShowImport(true)}
            className="px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-white text-xs font-medium rounded-lg transition-colors">
            Import
          </button>
          <button onClick={handleExport}
            className="px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-white text-xs font-medium rounded-lg transition-colors">
            Export
          </button>
        </div>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}
      {success && <div className="bg-green-900/20 border border-green-800 rounded-xl p-4"><p className="text-sm text-green-400">{success}</p></div>}

      <div className="flex gap-6">
        {/* Categories Sidebar */}
        <div className="w-48 shrink-0 space-y-1">
          {CONFIG_CATEGORIES.map((cat) => (
            <button key={cat} onClick={() => { setCategory(cat); setShowHistory(false); }}
              className={`w-full text-left px-4 py-2 rounded-lg text-sm font-medium transition-colors capitalize ${
                category === cat ? 'bg-indigo-600 text-white' : 'text-slate-400 hover:bg-slate-800 hover:text-slate-300'
              }`}>
              {cat}
            </button>
          ))}
        </div>

        {/* Config Editor */}
        <div className="flex-1">
          {loading ? (
            <div className="p-4 text-slate-400">Loading configuration...</div>
          ) : (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
              <h3 className="text-sm font-medium text-slate-400 capitalize">{category} Settings</h3>

              {Object.keys(config).length === 0 && (
                <p className="text-sm text-slate-500">No configuration options in this category.</p>
              )}

              {Object.entries(config).map(([key, entry]) => (
                <div key={key} className="space-y-1.5">
                  <label className="text-xs text-slate-400 capitalize">{key.replace(/_/g, ' ')}</label>
                  {entry.description && (
                    <p className="text-[10px] text-slate-600">{entry.description}</p>
                  )}
                  {entry.type === 'boolean' ? (
                    <select value={editValues[key] || 'false'} onChange={(e) => setEditValues({ ...editValues, [key]: e.target.value })}
                      className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white w-full">
                      <option value="true">True</option>
                      <option value="false">False</option>
                    </select>
                  ) : entry.type === 'number' || entry.type === 'integer' ? (
                    <input type="number" value={editValues[key] || ''} onChange={(e) => setEditValues({ ...editValues, [key]: e.target.value })}
                      className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" />
                  ) : (
                    <input type="text" value={editValues[key] || ''} onChange={(e) => setEditValues({ ...editValues, [key]: e.target.value })}
                      className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" />
                  )}
                </div>
              ))}

              <div className="flex gap-2 pt-2">
                <button onClick={handleSave} disabled={saving || Object.keys(config).length === 0}
                  className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-sm font-medium rounded-lg transition-colors">
                  {saving ? 'Saving...' : 'Save'}
                </button>
                <button onClick={handleViewHistory}
                  className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-colors">
                  View Change History
                </button>
              </div>
            </div>
          )}

          {/* Change History Modal */}
          {showHistory && (
            <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-2xl max-h-[80vh] overflow-y-auto space-y-4">
                <div className="flex items-center justify-between">
                  <h4 className="text-sm font-medium text-slate-300">Change History - {category}</h4>
                  <button onClick={() => setShowHistory(false)} className="text-xs text-slate-500 hover:text-slate-300">Close</button>
                </div>
                {history.length === 0 && <p className="text-sm text-slate-500">No changes recorded.</p>}
                {history.map((entry: any, i: number) => (
                  <div key={i} className="bg-slate-800 rounded-lg p-3 text-xs space-y-1">
                    <div className="text-slate-300">{entry.key}: <span className="text-red-400">{String(entry.old_value)}</span> → <span className="text-green-400">{String(entry.new_value)}</span></div>
                    <div className="text-slate-500">{entry.changed_by} - {new Date(entry.changed_at).toLocaleString()}</div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Import Modal */}
          {showImport && (
            <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-lg space-y-4">
                <h4 className="text-sm font-medium text-slate-300">Import Configuration</h4>
                <textarea value={importData} onChange={(e) => setImportData(e.target.value)}
                  className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white font-mono" rows={10}
                  placeholder="Paste JSON configuration..." />
                <div className="flex justify-end gap-2">
                  <button onClick={() => setShowImport(false)}
                    className="px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white text-xs rounded transition-colors">Cancel</button>
                  <button onClick={handleImport} disabled={!importData}
                    className="px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-xs rounded transition-colors">Import</button>
                </div>
              </div>
            </div>
          )}

          {/* Export Modal */}
          {showExport && (
            <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-lg space-y-4">
                <h4 className="text-sm font-medium text-slate-300">Exported Configuration</h4>
                <textarea readOnly value={exportData}
                  className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white font-mono" rows={10} />
                <div className="flex justify-end gap-2">
                  <button onClick={() => { navigator.clipboard.writeText(exportData); }}
                    className="px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white text-xs rounded transition-colors">Copy</button>
                  <button onClick={() => setShowExport(false)}
                    className="px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white text-xs rounded transition-colors">Done</button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
