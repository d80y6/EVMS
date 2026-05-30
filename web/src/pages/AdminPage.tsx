import React, { useEffect, useState } from 'react';
import { api } from '../api/client';
import { useAuth } from '../context/AuthContext';

interface UserItem {
  id: string;
  username: string;
  role: string;
  active: boolean;
  created_at: string;
}

export default function AdminPage() {
  const { role } = useAuth();
  const [users, setUsers] = useState<UserItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newUser, setNewUser] = useState({ username: '', password: '', role: 'viewer' });

  const fetchUsers = () => {
    setIsLoading(true);
    api.getUsers()
      .then((data) => setUsers(data.users))
      .catch((err) => setError(err.message))
      .finally(() => setIsLoading(false));
  };

  useEffect(() => {
    fetchUsers();
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
      await api.deleteUser(userId);
      fetchUsers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to deactivate user');
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
    </div>
  );
}
