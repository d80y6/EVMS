import { useState, useEffect } from 'react';
import { api } from '../api/client';
import { useAuth } from '../context/AuthContext';

interface SSOProvider {
  id: string;
  name: string;
  provider_type: string;
  enabled: boolean;
  client_id?: string;
  issuer_url?: string;
  created_at: string;
}

export default function SsoPage() {
  const { role } = useAuth();
  const [providers, setProviders] = useState<SSOProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [editProvider, setEditProvider] = useState<SSOProvider | null>(null);
  const [form, setForm] = useState({
    name: '',
    provider_type: 'oidc',
    client_id: '',
    client_secret: '',
    issuer_url: '',
    redirect_uri: '',
    enabled: true,
  });

  const fetchProviders = () => {
    setLoading(true);
    api.getSSOProviders()
      .then((data) => setProviders(data.providers || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchProviders(); }, []);

  if (role !== 'admin') {
    return <div className="flex items-center justify-center h-64"><p className="text-red-400">Access denied. Admin role required.</p></div>;
  }

  const resetForm = () => {
    setForm({ name: '', provider_type: 'oidc', client_id: '', client_secret: '', issuer_url: '', redirect_uri: '', enabled: true });
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError(null);
      const payload: any = {
        name: form.name,
        provider_type: form.provider_type,
        client_id: form.client_id,
        issuer_url: form.issuer_url,
        redirect_uri: form.redirect_uri || undefined,
        enabled: form.enabled,
      };
      if (form.client_secret) payload.client_secret = form.client_secret;

      if (editProvider) {
        await api.updateSSOProvider(editProvider.id, payload);
      } else {
        await api.createSSOProvider(payload);
      }
      setSuccess(editProvider ? 'Provider updated' : 'Provider created');
      setShowCreate(false);
      setEditProvider(null);
      resetForm();
      fetchProviders();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save provider');
    }
  };

  const handleDelete = async (id: string) => {
    if (!window.confirm('Delete this SSO provider?')) return;
    try {
      await api.deleteSSOProvider(id);
      setProviders((prev) => prev.filter((p) => p.id !== id));
      setSuccess('Provider deleted');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  const handleTest = async (id: string) => {
    try {
      setError(null);
      const res = await api.testSSOProvider(id);
      setSuccess(res.message || 'Test successful');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Test failed');
    }
  };

  const handleEdit = (p: SSOProvider) => {
    setEditProvider(p);
    setForm({
      name: p.name,
      provider_type: p.provider_type,
      client_id: p.client_id || '',
      client_secret: '',
      issuer_url: p.issuer_url || '',
      redirect_uri: '',
      enabled: p.enabled,
    });
    setShowCreate(true);
  };

  const handleToggle = async (id: string, enabled: boolean) => {
    try {
      await api.updateSSOProvider(id, { enabled: !enabled });
      setProviders((prev) => prev.map((p) => (p.id === id ? { ...p, enabled: !enabled } : p)));
      setSuccess(`Provider ${!enabled ? 'enabled' : 'disabled'}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to toggle');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">SSO Provider Management</h2>
        <button onClick={() => { setShowCreate(!showCreate); setEditProvider(null); resetForm(); }}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
          {showCreate ? 'Cancel' : '+ Add Provider'}
        </button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}
      {success && <div className="bg-green-900/20 border border-green-800 rounded-xl p-4"><p className="text-sm text-green-400">{success}</p></div>}

      {showCreate && (
        <form onSubmit={handleSave} className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h3 className="text-sm font-medium text-slate-400">{editProvider ? 'Edit Provider' : 'New SSO Provider'}</h3>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Provider Name</label>
              <input type="text" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Type</label>
              <select value={form.provider_type} onChange={(e) => setForm({ ...form, provider_type: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
                <option value="oidc">OpenID Connect (OIDC)</option>
                <option value="saml">SAML</option>
              </select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Client ID</label>
              <input type="text" value={form.client_id} onChange={(e) => setForm({ ...form, client_id: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Client Secret {editProvider ? '(leave blank to keep current)' : ''}</label>
              <input type="password" value={form.client_secret} onChange={(e) => setForm({ ...form, client_secret: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white"
                required={!editProvider} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Issuer URL</label>
              <input type="url" value={form.issuer_url} onChange={(e) => setForm({ ...form, issuer_url: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Redirect URI (optional)</label>
              <input type="url" value={form.redirect_uri} onChange={(e) => setForm({ ...form, redirect_uri: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white" />
            </div>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Enabled</label>
            <select value={String(form.enabled)} onChange={(e) => setForm({ ...form, enabled: e.target.value === 'true' })}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white">
              <option value="true">Yes</option>
              <option value="false">No</option>
            </select>
          </div>
          <div className="flex gap-2">
            <button type="submit" className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
              {editProvider ? 'Update' : 'Create'}
            </button>
            <button type="button" onClick={() => { setShowCreate(false); setEditProvider(null); resetForm(); }}
              className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-colors">
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {loading && <p className="p-4 text-sm text-slate-400">Loading providers...</p>}
        {!loading && providers.length === 0 && <p className="p-6 text-sm text-slate-500">No SSO providers configured.</p>}
        {!loading && providers.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left p-3">Name</th>
                <th className="text-left p-3">Type</th>
                <th className="text-left p-3">Client ID</th>
                <th className="text-left p-3">Issuer URL</th>
                <th className="text-left p-3">Enabled</th>
                <th className="text-left p-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {providers.map((p) => (
                <tr key={p.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="p-3 font-medium">{p.name}</td>
                  <td className="p-3">
                    <span className="text-xs px-2 py-0.5 rounded bg-indigo-900/30 text-indigo-300 uppercase">
                      {p.provider_type}
                    </span>
                  </td>
                  <td className="p-3 text-xs text-slate-400 font-mono">{p.client_id || '-'}</td>
                  <td className="p-3 text-xs text-slate-400 max-w-[200px] truncate">{p.issuer_url || '-'}</td>
                  <td className="p-3">
                    <button onClick={() => handleToggle(p.id, p.enabled)}
                      className={`text-xs px-3 py-1 rounded transition-colors ${
                        p.enabled ? 'bg-green-600 text-white' : 'bg-slate-700 text-slate-400'
                      }`}>
                      {p.enabled ? 'ON' : 'OFF'}
                    </button>
                  </td>
                  <td className="p-3">
                    <div className="flex gap-2">
                      <button onClick={() => handleTest(p.id)}
                        className="text-xs px-2 py-1 bg-green-700 hover:bg-green-600 text-white rounded transition-colors">Test</button>
                      <button onClick={() => handleEdit(p)}
                        className="text-xs px-2 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Edit</button>
                      <button onClick={() => handleDelete(p.id)}
                        className="text-xs px-2 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">Delete</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
