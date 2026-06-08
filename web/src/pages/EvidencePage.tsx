import { useState, useEffect } from 'react';
import { api } from '../api/client';

interface EvidenceCase {
  id: string;
  name: string;
  case_number: string;
  description: string;
  tags: string[];
  status: string;
  created_at: string;
  updated_at: string;
  item_count: number;
}

interface EvidenceItem {
  id: string;
  name: string;
  file_path?: string;
  notes?: string;
  recording_id?: string;
  camera_id?: string;
  timestamp?: string;
  created_at: string;
}

export default function EvidencePage() {
  const [cases, setCases] = useState<EvidenceCase[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [selectedCase, setSelectedCase] = useState<string | null>(null);
  const [caseDetail, setCaseDetail] = useState<any>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newCase, setNewCase] = useState({ name: '', case_number: '', description: '', tags: '' });
  const [shareEmail, setShareEmail] = useState('');
  const [shareExpiry, setShareExpiry] = useState('');
  const [sharing, setSharing] = useState(false);
  const [exporting, setExporting] = useState(false);

  const fetchCases = () => {
    api.getEvidenceCases()
      .then((data) => setCases(data.cases || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchCases(); }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError(null);
      await api.createEvidenceCase({
        name: newCase.name,
        case_number: newCase.case_number,
        description: newCase.description,
        tags: newCase.tags.split(',').map((t) => t.trim()).filter(Boolean),
      });
      setShowCreate(false);
      setNewCase({ name: '', case_number: '', description: '', tags: '' });
      fetchCases();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create case');
    }
  };

  const handleSelectCase = async (id: string) => {
    try {
      setError(null);
      const detail = await api.getEvidenceCase(id);
      setCaseDetail(detail);
      setSelectedCase(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load case details');
    }
  };

  const handleShare = async () => {
    if (!shareExpiry) return;
    setSharing(true);
    try {
      const res = await api.shareEvidence(selectedCase!, { expires_at: new Date(shareExpiry).toISOString(), email: shareEmail || undefined });
      await navigator.clipboard.writeText(res.share_url);
      setShareEmail('');
      setShareExpiry('');
      setSharing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to share');
      setSharing(false);
    }
  };

  const handleExport = async () => {
    setExporting(true);
    try {
      const res = await api.exportEvidenceBundle(selectedCase!);
      const a = document.createElement('a');
      a.href = res.file_path;
      a.download = res.file_path.split('/').pop() || 'evidence-export.zip';
      a.click();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Export failed');
    } finally {
      setExporting(false);
    }
  };

  const filtered = cases.filter((c) =>
    !search || c.name.toLowerCase().includes(search.toLowerCase()) ||
    c.case_number.toLowerCase().includes(search.toLowerCase())
  );

  if (loading) return <div className="p-4 text-slate-400">Loading evidence cases...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Evidence Management</h2>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors"
        >
          {showCreate ? 'Cancel' : 'New Case'}
        </button>
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-xl p-4">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h3 className="text-sm font-medium text-slate-400">New Evidence Case</h3>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Case Name</label>
              <input type="text" value={newCase.name} onChange={(e) => setNewCase({ ...newCase, name: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" required />
            </div>
            <div className="space-y-2">
              <label className="text-xs text-slate-500">Case Number</label>
              <input type="text" value={newCase.case_number} onChange={(e) => setNewCase({ ...newCase, case_number: e.target.value })}
                className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" required />
            </div>
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Description</label>
            <textarea value={newCase.description} onChange={(e) => setNewCase({ ...newCase, description: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" rows={3} />
          </div>
          <div className="space-y-2">
            <label className="text-xs text-slate-500">Tags (comma separated)</label>
            <input type="text" value={newCase.tags} onChange={(e) => setNewCase({ ...newCase, tags: e.target.value })}
              className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" placeholder="theft, vandalism, evidence" />
          </div>
          <button type="submit"
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium rounded-lg transition-colors">
            Create Case
          </button>
        </form>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-1">
          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            <div className="p-3 border-b border-slate-800">
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search cases..."
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
            {filtered.length === 0 && (
              <p className="p-6 text-sm text-slate-500">No cases found.</p>
            )}
            {filtered.map((c) => (
              <button
                key={c.id}
                onClick={() => handleSelectCase(c.id)}
                className={`w-full text-left p-4 border-b border-slate-800 hover:bg-slate-800/50 transition-colors ${
                  selectedCase === c.id ? 'bg-slate-800' : ''
                }`}
              >
                <div className="text-sm font-medium text-slate-300">{c.name}</div>
                <div className="text-xs text-slate-500 mt-1">{c.case_number}</div>
                <div className="flex items-center gap-2 mt-2">
                  <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${
                    c.status === 'open' ? 'bg-green-900/30 text-green-400' :
                    c.status === 'closed' ? 'bg-slate-700 text-slate-400' :
                    'bg-yellow-900/30 text-yellow-400'
                  }`}>{c.status}</span>
                  <span className="text-[10px] text-slate-600">{c.item_count} items</span>
                </div>
              </button>
            ))}
          </div>
        </div>

        <div className="lg:col-span-2">
          {!selectedCase && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-12 flex items-center justify-center">
              <p className="text-sm text-slate-500">Select a case to view details</p>
            </div>
          )}

          {selectedCase && caseDetail && (
            <div className="space-y-4">
              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-lg font-medium text-slate-200">{caseDetail.name}</h3>
                    <p className="text-xs text-slate-500">{caseDetail.case_number}</p>
                  </div>
                  <div className="flex gap-2">
                    <button onClick={handleExport} disabled={exporting}
                      className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">
                      {exporting ? 'Exporting...' : 'Export Bundle'}
                    </button>
                    <button onClick={async () => {
                      try {
                        await api.deleteEvidenceCase(selectedCase);
                        setSelectedCase(null);
                        setCaseDetail(null);
                        fetchCases();
                      } catch (err) {
                        setError(err instanceof Error ? err.message : 'Failed to delete');
                      }
                    }} className="text-xs px-3 py-1 bg-red-600 hover:bg-red-500 text-white rounded transition-colors">
                      Delete
                    </button>
                  </div>
                </div>

                {caseDetail.description && (
                  <p className="text-sm text-slate-400">{caseDetail.description}</p>
                )}

                {caseDetail.tags && caseDetail.tags.length > 0 && (
                  <div className="flex gap-1 flex-wrap">
                    {caseDetail.tags.map((tag: string, i: number) => (
                      <span key={i} className="text-[10px] px-2 py-0.5 bg-indigo-900/30 text-indigo-300 rounded-full">{tag}</span>
                    ))}
                  </div>
                )}
              </div>

              {caseDetail.items && caseDetail.items.length > 0 && (
                <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-3">
                  <h4 className="text-sm font-medium text-slate-400">Evidence Items</h4>
                  {caseDetail.items.map((item: EvidenceItem) => (
                    <div key={item.id} className="bg-slate-800 rounded-lg p-3 flex items-center justify-between">
                      <div>
                        <div className="text-sm text-slate-300">{item.name}</div>
                        {item.notes && <div className="text-xs text-slate-500 mt-0.5">{item.notes}</div>}
                        <div className="text-[10px] text-slate-600 mt-1">{new Date(item.created_at).toLocaleString()}</div>
                      </div>
                      {item.file_path && (
                        <a href={item.file_path} target="_blank" rel="noopener noreferrer"
                          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 text-white rounded transition-colors">
                          Download
                        </a>
                      )}
                    </div>
                  ))}
                </div>
              )}

              {caseDetail.chain_of_custody && caseDetail.chain_of_custody.length > 0 && (
                <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-3">
                  <h4 className="text-sm font-medium text-slate-400">Chain of Custody</h4>
                  {caseDetail.chain_of_custody.map((entry: any, i: number) => (
                    <div key={i} className="flex items-start gap-3 text-sm">
                      <div className="w-2 h-2 mt-1.5 rounded-full bg-indigo-500 shrink-0" />
                      <div>
                        <p className="text-slate-300">{entry.action}</p>
                        <p className="text-xs text-slate-500">{entry.actor} - {new Date(entry.timestamp).toLocaleString()}</p>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
                <h4 className="text-sm font-medium text-slate-400">Share Case</h4>
                <div className="flex items-center gap-3">
                  <input type="email" value={shareEmail} onChange={(e) => setShareEmail(e.target.value)}
                    placeholder="Email (optional)"
                    className="flex-1 bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500" />
                  <input type="datetime-local" value={shareExpiry} onChange={(e) => setShareExpiry(e.target.value)}
                    className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-white focus:outline-none focus:ring-2 focus:ring-indigo-500" />
                  <button onClick={handleShare} disabled={sharing || !shareExpiry}
                    className="text-xs px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">
                    {sharing ? 'Sharing...' : 'Share & Copy Link'}
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
