import React, { useEffect, useState } from 'react';
import { api, APIKey } from '../api/client';
import { useAuth } from '../context/AuthContext';
import { LegalHoldPage } from './LegalHoldPage';

interface UserItem {
  id: string;
  username: string;
  role: string;
  active: boolean;
  created_at: string;
}

export default function AdminPage() {
  const { role } = useAuth();
  const [tab, setTab] = useState<'users' | 'legal-holds' | 'api-keys'>('users');
  const [users, setUsers] = useState<UserItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newUser, setNewUser] = useState({ username: '', password: '', role: 'viewer' });
  const [sites, setSites] = useState<{ id: string; name: string; location: string }[]>([]);
  const [showSiteDialog, setShowSiteDialog] = useState(false);
  const [siteName, setSiteName] = useState('');
  const [siteLocation, setSiteLocation] = useState('');

  // API Key state
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [showCreateKey, setShowCreateKey] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyScopes, setNewKeyScopes] = useState('read');
  const [newKeyExpiry, setNewKeyExpiry] = useState('');
  const [createdKey, setCreatedKey] = useState<{key: string; id: string} | null>(null);

  const fetchUsers = () => {
    setIsLoading(true);
    api.getUsers()
      .then((data) => setUsers(data.users))
      .catch((err) => setError(err.message))
      .finally(() => setIsLoading(false));
  };

  const fetchAPIKeys = () => {
    api.getAPIKeys()
      .then((data) => setApiKeys(data.api_keys))
      .catch(() => {});
  };

  useEffect(() => {
    fetchUsers();
    api.getSites().then(d => setSites(d.sites || [])).catch(() => {});
    fetchAPIKeys();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.createUser(newUser.username, newUser.password, newUser.role);
      setShowCreate(false);
      setNewUser({ username: '', password: '', role: 'viewer' });
      fetchUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create user');
    }
  };

  const handleRoleChange = async (userId: string, newRole: string) => {
    try {
      await api.updateUser(userId, { role: newRole });
      fetchUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update user');
    }
  };

  const handleDeactivate = async (userId: string) => {
    try {
      await api.updateUser(userId, { role: 'viewer' });
      fetchUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to deactivate user');
    }
  };

  const handleCreateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const data: {name: string; scopes?: string; expires_in?: string} = { name: newKeyName, scopes: newKeyScopes };
      if (newKeyExpiry) {
        data.expires_in = newKeyExpiry;
      }
      const res = await api.createAPIKey(data);
      setCreatedKey({ key: res.key, id: res.id });
      setShowCreateKey(false);
      setNewKeyName('');
      setNewKeyScopes('read');
      setNewKeyExpiry('');
      fetchAPIKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create API key');
    }
  };

  const handleRevokeKey = async (keyId: string) => {
    if (!window.confirm('Revoke this API key? This action cannot be undone.')) return;
    try {
      await api.revokeAPIKey(keyId);
      fetchAPIKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke API key');
    }
  };

  if (role !== 'admin') {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-red-400">Access denied. Admin role required.</p>
      </div>
    );
  }

  return (
    <div className="max-w-4xl">
      <div className="flex gap-4 border-b border-slate-800 mb-6">
        <button
          onClick={() => setTab('users')}
          className={`pb-2 text-sm font-medium transition-colors ${
            tab === 'users' ? 'text-indigo-400 border-b-2 border-indigo-400' : 'text-slate-500 hover:text-slate-300'
          }`}
        >
          User Administration
        </button>
        <button
          onClick={() => setTab('api-keys')}
          className={`pb-2 text-sm font-medium transition-colors ${
            tab === 'api-keys' ? 'text-indigo-400 border-b-2 border-indigo-400' : 'text-slate-500 hover:text-slate-300'
          }`}
        >
          API Keys
        </button>
        <button
          onClick={() => setTab('legal-holds')}
          className={`pb-2 text-sm font-medium transition-colors ${
            tab === 'legal-holds' ? 'text-indigo-400 border-b-2 border-indigo-400' : 'text-slate-500 hover:text-slate-300'
          }`}
        >
          Legal Holds
        </button>
      </div>

      {tab === 'users' && (
        <>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold text-slate-200">User Administration</h2>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors"
        >
          {showCreate ? 'Cancel' : 'Create User'}
        </button>
      </div>

      {error && (
        <div className="mb-4 bg-red-900/30 border border-red-800 rounded-lg p-3 text-sm text-red-400">
          {error}
        </div>
      )}

      {showCreate && (
        <form onSubmit={handleCreate} className="mb-6 bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h3 className="text-sm font-medium text-slate-400">New User</h3>
          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Username</label>
              <input
                type="text"
                value={newUser.username}
                onChange={(e) => setNewUser({ ...newUser, username: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500"
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Password</label>
              <input
                type="password"
                value={newUser.password}
                onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500"
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Role</label>
              <select
                value={newUser.role}
                onChange={(e) => setNewUser({ ...newUser, role: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500"
              >
                <option value="viewer">Viewer</option>
                <option value="operator">Operator</option>
                <option value="admin">Admin</option>
              </select>
            </div>
          </div>
          <button
            type="submit"
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors"
          >
            Create
          </button>
        </form>
      )}

      {isLoading ? (
        <div className="flex items-center justify-center h-32">
          <p className="text-slate-400 animate-pulse">Loading users...</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                <th className="text-left pb-3 pr-4">Username</th>
                <th className="text-left pb-3 pr-4">Role</th>
                <th className="text-left pb-3 pr-4">Status</th>
                <th className="text-left pb-3 pr-4">Created</th>
                <th className="text-left pb-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} className="border-b border-slate-800/50 text-slate-300">
                  <td className="py-3 pr-4">{u.username}</td>
                  <td className="py-3 pr-4">
                    <select
                      value={u.role}
                      onChange={(e) => handleRoleChange(u.id, e.target.value)}
                      className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-300 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    >
                      <option value="viewer">Viewer</option>
                      <option value="operator">Operator</option>
                      <option value="admin">Admin</option>
                    </select>
                  </td>
                  <td className="py-3 pr-4">
                    <span className={`text-xs px-2 py-0.5 rounded-full ${
                      u.active ? 'bg-green-900/30 text-green-400' : 'bg-red-900/30 text-red-400'
                    }`}>
                      {u.active ? 'Active' : 'Inactive'}
                    </span>
                  </td>
                  <td className="py-3 pr-4 text-slate-500">
                    {new Date(u.created_at).toLocaleDateString()}
                  </td>
                  <td className="py-3">
                    {u.active && (
                      <button
                        onClick={() => handleDeactivate(u.id)}
                        className="text-red-400 hover:text-red-300 text-xs transition-colors"
                      >
                        Deactivate
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
        </>
      )}

      {tab === 'api-keys' && (
        <div className="space-y-6">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-slate-200">API Key Management</h2>
            <button
              onClick={() => { setShowCreateKey(true); setCreatedKey(null); }}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors"
            >
              Create API Key
            </button>
          </div>

          {error && (
            <div className="mb-4 bg-red-900/30 border border-red-800 rounded-lg p-3 text-sm text-red-400">
              {error}
            </div>
          )}

          {createdKey && (
            <div className="bg-amber-900/30 border border-amber-700 rounded-xl p-4 space-y-2">
              <h3 className="text-sm font-medium text-amber-300">API Key Created - Copy it now!</h3>
              <p className="text-xs text-amber-400">This key will not be shown again.</p>
              <div className="bg-slate-900 rounded-lg p-3 font-mono text-sm text-amber-200 break-all select-all">
                {createdKey.key}
              </div>
              <button
                onClick={() => {
                  navigator.clipboard.writeText(createdKey.key);
                  setCreatedKey(null);
                }}
                className="text-xs px-3 py-1 bg-amber-600 hover:bg-amber-500 text-white rounded transition-colors"
              >
                Copy & Dismiss
              </button>
            </div>
          )}

          {showCreateKey && (
            <form onSubmit={handleCreateKey} className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
              <h3 className="text-sm font-medium text-slate-400">New API Key</h3>
              <div className="grid grid-cols-3 gap-4">
                <div className="space-y-2">
                  <label className="text-xs text-slate-500">Name</label>
                  <input
                    type="text"
                    value={newKeyName}
                    onChange={(e) => setNewKeyName(e.target.value)}
                    className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    required
                    placeholder="e.g. Automation"
                  />
                </div>
                <div className="space-y-2">
                  <label className="text-xs text-slate-500">Scopes</label>
                  <select
                    value={newKeyScopes}
                    onChange={(e) => setNewKeyScopes(e.target.value)}
                    className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  >
                    <option value="read">Read Only</option>
                    <option value="read,write">Read & Write</option>
                    <option value="admin">Admin</option>
                  </select>
                </div>
                <div className="space-y-2">
                  <label className="text-xs text-slate-500">Expires In (optional)</label>
                  <select
                    value={newKeyExpiry}
                    onChange={(e) => setNewKeyExpiry(e.target.value)}
                    className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  >
                    <option value="">Never</option>
                    <option value="24h">24 hours</option>
                    <option value="720h">30 days</option>
                    <option value="8760h">1 year</option>
                  </select>
                </div>
              </div>
              <div className="flex gap-2">
                <button
                  type="submit"
                  className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors"
                >
                  Generate Key
                </button>
                <button
                  type="button"
                  onClick={() => setShowCreateKey(false)}
                  className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white text-sm font-medium rounded-lg transition-colors"
                >
                  Cancel
                </button>
              </div>
            </form>
          )}

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-800 text-slate-500 uppercase text-xs tracking-wider">
                  <th className="text-left pb-3 pr-4">Name</th>
                  <th className="text-left pb-3 pr-4">Key Prefix</th>
                  <th className="text-left pb-3 pr-4">Scopes</th>
                  <th className="text-left pb-3 pr-4">Status</th>
                  <th className="text-left pb-3 pr-4">Created</th>
                  <th className="text-left pb-3 pr-4">Last Used</th>
                  <th className="text-left pb-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {apiKeys.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-slate-500 text-sm">
                      No API keys configured.
                    </td>
                  </tr>
                )}
                {apiKeys.map((k) => (
                  <tr key={k.id} className="border-b border-slate-800/50 text-slate-300">
                    <td className="py-3 pr-4 font-medium">{k.name}</td>
                    <td className="py-3 pr-4">
                      <code className="text-xs bg-slate-800 px-2 py-1 rounded text-slate-400">{k.key_prefix}...</code>
                    </td>
                    <td className="py-3 pr-4 text-xs text-slate-400">{k.scopes}</td>
                    <td className="py-3 pr-4">
                      <span className={`text-xs px-2 py-0.5 rounded-full ${
                        k.active ? 'bg-green-900/30 text-green-400' : 'bg-red-900/30 text-red-400'
                      }`}>
                        {k.active ? 'Active' : 'Revoked'}
                      </span>
                    </td>
                    <td className="py-3 pr-4 text-slate-500 text-xs">
                      {new Date(k.created_at).toLocaleDateString()}
                    </td>
                    <td className="py-3 pr-4 text-slate-500 text-xs">
                      {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'Never'}
                    </td>
                    <td className="py-3">
                      {k.active && (
                        <button
                          onClick={() => handleRevokeKey(k.id)}
                          className="text-red-400 hover:text-red-300 text-xs transition-colors"
                        >
                          Revoke
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab === 'legal-holds' && <LegalHoldPage />}

      {/* Sites Section */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4 mt-6">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-slate-400">Sites</h3>
          <button onClick={() => setShowSiteDialog(true)}
            className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">+ Add Site</button>
        </div>
        {sites.length === 0 && <p className="text-sm text-slate-500">No sites configured.</p>}
        {sites.map(s => (
          <div key={s.id} className="flex items-center justify-between bg-slate-800 rounded-lg p-3">
            <div><span className="text-sm text-slate-300">{s.name}</span><span className="text-xs text-slate-600 ml-2">{s.location}</span></div>
          </div>
        ))}
        {showSiteDialog && (
          <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 w-full max-w-md space-y-4">
              <h4 className="text-sm font-medium text-slate-300">Add Site</h4>
              <input type="text" placeholder="Site name" value={siteName} onChange={e => setSiteName(e.target.value)}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
              <input type="text" placeholder="Location" value={siteLocation} onChange={e => setSiteLocation(e.target.value)}
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
              <div className="flex justify-end gap-2">
                <button onClick={() => setShowSiteDialog(false)}
                  className="text-xs px-3 py-1 bg-slate-700 hover:bg-slate-600 text-white rounded transition-colors">Cancel</button>
                <button onClick={async () => {
                  if (!siteName) return;
                  await api.createSite(siteName, siteLocation);
                  const freshSites = await api.getSites();
                  setSites(freshSites.sites || []);
                  setShowSiteDialog(false); setSiteName(''); setSiteLocation('');
                }} disabled={!siteName}
                  className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">Create</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
