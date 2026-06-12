import { useState, useEffect } from 'react';
import { api } from '../api/client';

const SEVERITY_COLORS: Record<string, string> = {
  low: 'bg-slate-700 text-slate-400',
  medium: 'bg-yellow-900/30 text-yellow-400',
  high: 'bg-orange-900/30 text-orange-400',
  critical: 'bg-red-900/30 text-red-400',
};

const STATUS_FLOW = ['open', 'investigating', 'resolved', 'closed'] as const;

export default function IncidentsPage() {
  const [incidents, setIncidents] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filterStatus, setFilterStatus] = useState('');
  const [filterSeverity, setFilterSeverity] = useState('');
  const [selectedIncident, setSelectedIncident] = useState<any>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newIncident, setNewIncident] = useState({ title: '', description: '', severity: 'medium', camera_ids: '' });
  const [noteContent, setNoteContent] = useState('');
  const [assignUserId, setAssignUserId] = useState('');

  const fetchIncidents = () => {
    setLoading(true);
    api.getIncidents({ status: filterStatus || undefined, severity: filterSeverity || undefined })
      .then((data) => { setIncidents(data.incidents || []); setTotal(data.total || 0); })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { fetchIncidents(); }, [filterStatus, filterSeverity]);

  const handleSelect = async (id: string) => {
    try {
      const detail = await api.getIncident(id);
      setSelectedIncident(detail);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load incident');
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError(null);
      await api.createIncident({
        title: newIncident.title,
        description: newIncident.description,
        severity: newIncident.severity,
        camera_ids: newIncident.camera_ids.split(',').map((s) => s.trim()).filter(Boolean),
      });
      setShowCreate(false);
      setNewIncident({ title: '', description: '', severity: 'medium', camera_ids: '' });
      fetchIncidents();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create incident');
    }
  };

  const handleStatusChange = async (newStatus: string) => {
    if (!selectedIncident) return;
    try {
      await api.updateIncidentStatus(selectedIncident.id, newStatus);
      setSelectedIncident({ ...selectedIncident, status: newStatus });
      fetchIncidents();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update status');
    }
  };

  const handleAssign = async () => {
    if (!assignUserId || !selectedIncident) return;
    try {
      await api.assignIncident(selectedIncident.id, assignUserId);
      setSelectedIncident({ ...selectedIncident, assigned_to: assignUserId });
      setAssignUserId('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to assign');
    }
  };

  const handleAddNote = async () => {
    if (!noteContent || !selectedIncident) return;
    try {
      const res = await api.addIncidentNote(selectedIncident.id, noteContent);
      const newNote = { id: res.id, content: noteContent, created_by: 'current_user', created_at: new Date().toISOString() };
      setSelectedIncident({
        ...selectedIncident,
        notes: [...(selectedIncident.notes || []), newNote],
      });
      setNoteContent('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add note');
    }
  };

  const handleEscalate = async () => {
    if (!selectedIncident) return;
    try {
      await api.escalateIncident(selectedIncident.id);
      fetchIncidents();
      handleSelect(selectedIncident.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to escalate');
    }
  };

  if (loading && incidents.length === 0) return <div className="p-4 text-slate-400">Loading incidents...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Incident Management</h2>
        <button onClick={() => setShowCreate(!showCreate)}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
          {showCreate ? 'Cancel' : 'New Incident'}
        </button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h3 className="text-sm font-medium text-slate-400">New Incident</h3>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Title</label>
              <input type="text" value={newIncident.title} onChange={(e) => setNewIncident({ ...newIncident, title: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Severity</label>
              <select value={newIncident.severity} onChange={(e) => setNewIncident({ ...newIncident, severity: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500">
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Description</label>
            <textarea value={newIncident.description} onChange={(e) => setNewIncident({ ...newIncident, description: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" rows={3} />
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Related Camera IDs (comma separated)</label>
            <input type="text" value={newIncident.camera_ids} onChange={(e) => setNewIncident({ ...newIncident, camera_ids: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" placeholder="cam-001, cam-002" />
          </div>
          <button type="submit" className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">Create Incident</button>
        </form>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-1">
          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            <div className="p-3 border-b border-slate-800 flex gap-2">
              <select value={filterStatus} onChange={(e) => setFilterStatus(e.target.value)}
                className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-300">
                <option value="">All Status</option>
                {STATUS_FLOW.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
              <select value={filterSeverity} onChange={(e) => setFilterSeverity(e.target.value)}
                className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-300">
                <option value="">All Severity</option>
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>
            {incidents.length === 0 && <p className="p-6 text-sm text-slate-500">No incidents found.</p>}
            {incidents.map((inc) => (
              <button key={inc.id} onClick={() => handleSelect(inc.id)}
                className={`w-full text-left p-4 border-b border-slate-800 hover:bg-slate-800/50 transition-colors ${selectedIncident?.id === inc.id ? 'bg-slate-800' : ''}`}>
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-slate-300">{inc.title}</span>
                  <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${SEVERITY_COLORS[inc.severity] || SEVERITY_COLORS.medium}`}>{inc.severity}</span>
                </div>
                <div className="text-xs text-slate-500 mt-1">
                  <span className={`px-1.5 py-0.5 rounded ${inc.status === 'open' ? 'text-green-400' : inc.status === 'closed' ? 'text-slate-400' : 'text-yellow-400'}`}>{inc.status}</span>
                  {inc.assigned_to && <span className="ml-2">assignee: {inc.assigned_to}</span>}
                </div>
              </button>
            ))}
            {total > incidents.length && (
              <p className="p-3 text-xs text-slate-600 text-center">{total} total incidents</p>
            )}
          </div>
        </div>

        <div className="lg:col-span-2">
          {!selectedIncident && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-12 flex items-center justify-center">
              <p className="text-sm text-slate-500">Select an incident to view details</p>
            </div>
          )}

          {selectedIncident && (
            <div className="space-y-4">
              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-lg font-medium text-slate-200">{selectedIncident.title}</h3>
                    <p className="text-xs text-slate-500">Created {new Date(selectedIncident.created_at).toLocaleString()}</p>
                  </div>
                  <div className="flex gap-2">
                    <button onClick={handleEscalate}
                      className="text-xs px-3 py-1 bg-orange-600 hover:bg-orange-500 text-white rounded transition-colors">
                      Escalate
                    </button>
                  </div>
                </div>

                {selectedIncident.description && (
                  <p className="text-sm text-slate-400">{selectedIncident.description}</p>
                )}

                <div className="flex items-center gap-2">
                  <span className="text-xs text-slate-500">Status:</span>
                  <select value={selectedIncident.status} onChange={(e) => handleStatusChange(e.target.value)}
                    className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-slate-300">
                    {STATUS_FLOW.map((s) => (
                      <option key={s} value={s} disabled={
                        (selectedIncident.status === 'resolved' && s === 'investigating') ||
                        (selectedIncident.status === 'closed' && s !== 'closed')
                      }>{s}</option>
                    ))}
                  </select>
                  <span className={`text-xs px-2 py-0.5 rounded-full ${SEVERITY_COLORS[selectedIncident.severity] || ''}`}>{selectedIncident.severity}</span>
                </div>

                <div className="flex items-center gap-2">
                  <span className="text-xs text-slate-500">Assign to:</span>
                  <input type="text" value={assignUserId} onChange={(e) => setAssignUserId(e.target.value)}
                    placeholder="User ID" className="bg-slate-800 border border-slate-700 rounded px-2 py-1 text-xs text-white w-40" />
                  <button onClick={handleAssign} disabled={!assignUserId}
                    className="text-xs px-2 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">Assign</button>
                </div>
              </div>

              {selectedIncident.timeline && selectedIncident.timeline.length > 0 && (
                <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-3">
                  <h4 className="text-sm font-medium text-slate-400">Timeline</h4>
                  {selectedIncident.timeline.map((entry: any, i: number) => (
                    <div key={i} className="flex items-start gap-3">
                      <div className="w-2 h-2 mt-1.5 rounded-full bg-indigo-500 shrink-0" />
                      <div>
                        <p className="text-sm text-slate-300">{entry.action}</p>
                        <p className="text-xs text-slate-500">{entry.actor} - {new Date(entry.timestamp).toLocaleString()}</p>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-3">
                <h4 className="text-sm font-medium text-slate-400">Notes</h4>
                {selectedIncident.notes && selectedIncident.notes.length > 0 && (
                  <div className="space-y-2">
                    {selectedIncident.notes.map((note: any, i: number) => (
                      <div key={i} className="bg-slate-800 rounded-lg p-3">
                        <p className="text-sm text-slate-300">{note.content}</p>
                        <p className="text-xs text-slate-500 mt-1">{note.created_by} - {new Date(note.created_at).toLocaleString()}</p>
                      </div>
                    ))}
                  </div>
                )}
                <div className="flex gap-2">
                  <textarea value={noteContent} onChange={(e) => setNoteContent(e.target.value)}
                    placeholder="Add a note..."
                    className="flex-1 bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500" rows={2} />
                  <button onClick={handleAddNote} disabled={!noteContent}
                    className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white text-xs font-medium rounded transition-colors self-end">
                    Add Note
                  </button>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
