import { useState, useEffect } from 'react';
import { api, getCSRFToken } from '../api/client';

export default function CsrfPage() {
  const [token, setToken] = useState<string | null>(getCSRFToken());
  const [enabled, setEnabled] = useState(true);
  const [createdAt, setCreatedAt] = useState('');
  const [loading, setLoading] = useState(true);
  const [regenerating, setRegenerating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  useEffect(() => {
    api.getCSRFStatus()
      .then((data) => {
        setToken(data.token);
        setEnabled(data.enabled);
        setCreatedAt(data.created_at);
      })
      .catch(() => setToken(getCSRFToken()))
      .finally(() => setLoading(false));
  }, []);

  const handleRegenerate = async () => {
    if (!window.confirm('Regenerate CSRF token? This may break active forms.')) return;
    setRegenerating(true);
    setError(null);
    setSuccess(null);
    try {
      const data = await api.regenerateCSRFToken();
      setToken(data.token);
      setSuccess('CSRF token regenerated successfully');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to regenerate token');
    } finally {
      setRegenerating(false);
    }
  };

  const handleCopy = () => {
    if (token) {
      navigator.clipboard.writeText(token);
      setSuccess('Token copied to clipboard');
    }
  };

  if (loading) return <div className="p-4 text-slate-400">Loading CSRF settings...</div>;

  return (
    <div className="max-w-2xl space-y-6">
      <h2 className="text-lg font-semibold text-slate-200">CSRF Token Management</h2>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-xl p-4">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}
      {success && (
        <div className="bg-green-900/20 border border-green-800 rounded-xl p-4">
          <p className="text-sm text-green-400">{success}</p>
        </div>
      )}

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-slate-400">CSRF Protection</h3>
            <p className="text-xs text-slate-500 mt-1">
              Cross-Site Request Forgery protection status
            </p>
          </div>
          <span className={`px-3 py-1 text-xs font-medium rounded-full ${
            enabled ? 'bg-green-900/30 text-green-400' : 'bg-red-900/30 text-red-400'
          }`}>
            {enabled ? 'Protected' : 'Disabled'}
          </span>
        </div>

        <div className="bg-slate-800 rounded-lg p-4 space-y-2">
          <label className="text-xs text-slate-500">Current CSRF Token</label>
          <div className="flex items-center gap-2">
            <code className="flex-1 text-xs text-indigo-300 font-mono bg-slate-900 rounded px-3 py-2 select-all break-all">
              {token || 'No token available'}
            </code>
            <button onClick={handleCopy} disabled={!token}
              className="px-3 py-2 bg-slate-700 hover:bg-slate-600 disabled:bg-slate-800 text-white text-xs rounded transition-colors shrink-0">
              Copy
            </button>
          </div>
        </div>

        {createdAt && (
          <div className="text-xs text-slate-500">
            Token created: {new Date(createdAt).toLocaleString()}
          </div>
        )}

        <button
          onClick={handleRegenerate}
          disabled={regenerating}
          className="px-4 py-2 bg-amber-600 hover:bg-amber-500 disabled:bg-amber-800 text-white text-sm font-medium rounded-lg transition-colors"
        >
          {regenerating ? 'Regenerating...' : 'Regenerate Token'}
        </button>
      </div>
    </div>
  );
}
