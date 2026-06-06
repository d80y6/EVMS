import { useState, useEffect, useCallback } from 'react';
import { api, ScanRecord, ResultRecord } from '../api/client';

export default function DiscoveryPage() {
  const [mode, setMode] = useState<'launcher' | 'history' | 'results'>('launcher');
  const [sites, setSites] = useState<{ id: string; name: string }[]>([]);
  const [selectedSite, setSelectedSite] = useState('');
  const [methods, setMethods] = useState<string[]>(['ws-discovery', 'ip-range']);
  const [subnets, setSubnets] = useState('');
  const [ports, setPorts] = useState('80,554,8080');
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Scan history
  const [scans, setScans] = useState<ScanRecord[]>([]);
  const [scanTotal, setScanTotal] = useState(0);
  const [scanPage, setScanPage] = useState(1);

  // Current scan results
  const [currentScanId, setCurrentScanId] = useState<string | null>(null);
  const [results, setResults] = useState<ResultRecord[]>([]);
  const [resultTotal, setResultTotal] = useState(0);
  const [resultPage, setResultPage] = useState(1);
  const [resultQuery, setResultQuery] = useState('');
  const [selectedResults, setSelectedResults] = useState<Set<string>>(new Set());
  const [credentials] = useState<Record<string, { username: string; password: string }>>({});
  const [importing, setImporting] = useState(false);
  const [importResults, setImportResults] = useState<{ result_id: string; status: string; error?: string }[]>([]);

  useEffect(() => {
    api.getSites().then(d => setSites(d.sites || [])).catch(() => {});
  }, []);

  const handleStartScan = async () => {
    if (!selectedSite) { setError('Select a site'); return; }
    setScanning(true);
    setError(null);
    try {
      const scan = await api.startDiscoveryScan({
        site_id: selectedSite,
        methods,
        subnets: subnets ? subnets.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        ports: ports.split(',').map(p => parseInt(p.trim())).filter(p => !isNaN(p)),
      });
      setCurrentScanId(scan.id);
      const poll = setInterval(async () => {
        try {
          const s = await api.getDiscoveryScan(scan.id);
          if (s.status !== 'running' && s.status !== 'pending') {
            clearInterval(poll);
            setScanning(false);
            setMode('results');
            loadResults(scan.id, 1, '');
          }
        } catch {
          clearInterval(poll);
          setScanning(false);
        }
      }, 1000);
    } catch (e: any) {
      setError(e.message || 'Failed to start scan');
      setScanning(false);
    }
  };

  const loadScans = useCallback(async (page: number) => {
    try {
      const data = await api.getDiscoveryScans({ site_id: selectedSite || undefined, page, per_page: 20 });
      setScans(data.scans);
      setScanTotal(data.total);
      setScanPage(page);
    } catch { setError('Failed to load scans'); }
  }, [selectedSite]);

  const loadResults = useCallback(async (scanId: string, page: number, query: string) => {
    try {
      const data = await api.getDiscoveryResults(scanId, { page, per_page: 20, query: query || undefined });
      setResults(data.results);
      setResultTotal(data.total);
      setResultPage(page);
    } catch { setError('Failed to load results'); }
  }, []);

  const toggleResult = (id: string) => {
    setSelectedResults(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const toggleAllResults = () => {
    if (selectedResults.size === results.length) {
      setSelectedResults(new Set());
    } else {
      setSelectedResults(new Set(results.map(r => r.id)));
    }
  };

  const handleImport = async () => {
    if (!currentScanId || selectedResults.size === 0) return;
    setImporting(true);
    setImportResults([]);
    try {
      const credsList = Array.from(selectedResults)
        .filter(id => credentials[id])
        .map(id => ({ result_id: id, ...credentials[id] }));
      const res = await api.importDiscoveryResults(currentScanId, {
        result_ids: Array.from(selectedResults),
        credentials: credsList.length > 0 ? credsList : undefined,
      });
      setImportResults([
        ...Array.from({ length: res.imported }, () => ({ result_id: '', status: 'imported' as const })),
        ...res.failed.map(f => ({ result_id: f.result_id, status: 'failed' as const, error: f.error })),
      ]);
      loadResults(currentScanId, 1, '');
    } catch (e: any) {
      setError(e.message || 'Import failed');
    }
    setImporting(false);
  };

  const viewScan = (id: string) => {
    setCurrentScanId(id);
    setMode('results');
    loadResults(id, 1, '');
  };

  const renderMethodCheckboxes = () => {
    const allMethods = [
      { id: 'ws-discovery', label: 'WS-Discovery' },
      { id: 'ip-range', label: 'IP Range Scan' },
      { id: 'mdns', label: 'mDNS' },
      { id: 'manual', label: 'Manual IPs' },
    ];
    return (
      <div className="flex flex-wrap gap-3">
        {allMethods.map(m => (
          <label key={m.id} className="flex items-center gap-1.5 text-xs text-slate-300">
            <input type="checkbox" checked={methods.includes(m.id)}
              onChange={() => setMethods(prev =>
                prev.includes(m.id) ? prev.filter(x => x !== m.id) : [...prev, m.id])} />
            {m.label}
          </label>
        ))}
      </div>
    );
  };

  const capColor = (cap: string) => {
    const colors: Record<string, string> = {
      ptz: 'bg-blue-900/40 text-blue-300',
      analytics: 'bg-purple-900/40 text-purple-300',
      imaging: 'bg-green-900/40 text-green-300',
      events: 'bg-yellow-900/40 text-yellow-300',
      media: 'bg-indigo-900/40 text-indigo-300',
      recording: 'bg-red-900/40 text-red-300',
    };
    return colors[cap] || 'bg-slate-700 text-slate-400';
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Network Discovery</h2>
        <div className="flex gap-2">
          <button onClick={() => setMode('launcher')}
            className={`text-xs px-3 py-1 rounded transition-colors ${mode === 'launcher' ? 'bg-indigo-600 text-white' : 'bg-slate-700 text-slate-300 hover:bg-slate-600'}`}>
            New Scan
          </button>
          <button onClick={() => { setMode('history'); loadScans(1); }}
            className={`text-xs px-3 py-1 rounded transition-colors ${mode === 'history' ? 'bg-indigo-600 text-white' : 'bg-slate-700 text-slate-300 hover:bg-slate-600'}`}>
            Scan History
          </button>
        </div>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {/* Scan Launcher */}
      {mode === 'launcher' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-4 space-y-4">
          <div>
            <label className="text-xs text-slate-500 block mb-1">Site</label>
            <select value={selectedSite} onChange={e => setSelectedSite(e.target.value)}
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300">
              <option value="">Select site</option>
              {sites.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </div>
          <div>
            <label className="text-xs text-slate-500 block mb-1">Discovery Methods</label>
            {renderMethodCheckboxes()}
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-xs text-slate-500 block mb-1">Subnets (CIDR, comma-separated)</label>
              <input type="text" value={subnets} onChange={e => setSubnets(e.target.value)}
                placeholder="10.0.0.0/24, 192.168.1.0/24"
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
            <div>
              <label className="text-xs text-slate-500 block mb-1">Ports (comma-separated)</label>
              <input type="text" value={ports} onChange={e => setPorts(e.target.value)}
                placeholder="80, 554, 8080"
                className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300" />
            </div>
          </div>
          <button onClick={handleStartScan} disabled={scanning || !selectedSite}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded text-sm transition-colors">
            {scanning ? 'Scanning...' : 'Start Scan'}
          </button>
        </div>
      )}

      {/* Scan History */}
      {mode === 'history' && (
        <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">Date</th>
                <th className="p-3">Methods</th>
                <th className="p-3">Subnets</th>
                <th className="p-3">Found</th>
                <th className="p-3">Status</th>
              </tr>
            </thead>
            <tbody>
              {scans.map(s => (
                <tr key={s.id} className="border-b border-slate-800 hover:bg-slate-800/50 cursor-pointer transition-colors"
                  onClick={() => viewScan(s.id)}>
                  <td className="p-3 text-slate-300 text-xs">{new Date(s.created_at).toLocaleString()}</td>
                  <td className="p-3 text-slate-300 text-xs">{s.methods.join(', ')}</td>
                  <td className="p-3 text-slate-300 text-xs">{(s.subnets || []).join(', ') || '-'}</td>
                  <td className="p-3 text-slate-300">{s.total_found}</td>
                  <td className="p-3">
                    <span className={`text-[10px] px-2 py-0.5 rounded uppercase font-medium ${
                      s.status === 'completed' ? 'bg-green-900/40 text-green-300' :
                      s.status === 'running' ? 'bg-blue-900/40 text-blue-300' :
                      s.status === 'failed' ? 'bg-red-900/40 text-red-300' :
                      s.status === 'cancelled' ? 'bg-yellow-900/40 text-yellow-300' :
                      'bg-slate-700 text-slate-400'
                    }`}>{s.status}</span>
                  </td>
                </tr>
              ))}
              {scans.length === 0 && (
                <tr><td colSpan={5} className="p-6 text-sm text-slate-500 text-center">No scans yet</td></tr>
              )}
            </tbody>
          </table>
          {scanTotal > 20 && (
            <div className="flex justify-center gap-2 p-3">
              <button disabled={scanPage <= 1} onClick={() => loadScans(scanPage - 1)}
                className="text-xs px-2 py-1 bg-slate-700 disabled:bg-slate-800 text-slate-300 rounded">Previous</button>
              <span className="text-xs text-slate-500 py-1">Page {scanPage} of {Math.ceil(scanTotal / 20)}</span>
              <button disabled={scanPage >= Math.ceil(scanTotal / 20)} onClick={() => loadScans(scanPage + 1)}
                className="text-xs px-2 py-1 bg-slate-700 disabled:bg-slate-800 text-slate-300 rounded">Next</button>
            </div>
          )}
        </div>
      )}

      {/* Results View */}
      {mode === 'results' && (
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            <input type="text" value={resultQuery} onChange={e => setResultQuery(e.target.value)}
              placeholder="Search results..." className="bg-slate-800 border border-slate-700 rounded px-3 py-1.5 text-sm text-slate-300 w-64"
              onKeyDown={e => { if (e.key === 'Enter' && currentScanId) loadResults(currentScanId, 1, resultQuery); }} />
            <button onClick={() => { if (currentScanId) loadResults(currentScanId, 1, resultQuery); }}
              className="text-xs px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-slate-300 rounded">Search</button>
            <button onClick={() => setMode('history')}
              className="text-xs px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-slate-300 rounded ml-auto">Back</button>
          </div>

          {results.length > 0 && (
            <div className="flex flex-wrap gap-2 items-center">
              <label className="flex items-center gap-2 text-xs text-slate-400">
                <input type="checkbox" checked={selectedResults.size === results.length} onChange={toggleAllResults} className="rounded" />
                Select All ({results.length} found)
              </label>
              <button onClick={handleImport} disabled={selectedResults.size === 0 || importing}
                className="text-xs px-3 py-1 bg-green-600 hover:bg-green-500 disabled:bg-green-800 text-white rounded transition-colors">
                {importing ? 'Importing...' : `Import Selected (${selectedResults.size})`}
              </button>
            </div>
          )}

          {importResults.length > 0 && (
            <div className="bg-slate-900 border border-slate-800 rounded-xl p-4">
              <h3 className="text-sm font-medium text-slate-300 mb-2">Import Results</h3>
              <div className="text-xs space-y-1 text-slate-400">
                {importResults.map((r, i) => (
                  <div key={i} className={r.status === 'imported' ? 'text-green-400' : 'text-red-400'}>
                    {r.status === 'imported' ? '✓ Imported' : `✗ Failed: ${r.error}`}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            {results.length === 0 && (
              <p className="p-6 text-sm text-slate-500">No devices found in this scan.</p>
            )}
            {results.length > 0 && (
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-slate-400 border-b border-slate-800 text-left">
                    <th className="p-3 w-8"><input type="checkbox" checked={selectedResults.size === results.length} onChange={toggleAllResults} className="rounded" /></th>
                    <th className="p-3">IP:Port</th>
                    <th className="p-3">Manufacturer</th>
                    <th className="p-3">Model</th>
                    <th className="p-3">Firmware</th>
                    <th className="p-3">Serial</th>
                    <th className="p-3">Hostname</th>
                    <th className="p-3">Capabilities</th>
                    <th className="p-3">In DB</th>
                  </tr>
                </thead>
                <tbody>
                  {results.map(r => (
                    <tr key={r.id} className={`border-b border-slate-800 hover:bg-slate-800/50 transition-colors ${selectedResults.has(r.id) ? 'bg-indigo-900/20' : ''}`}>
                      <td className="p-3"><input type="checkbox" checked={selectedResults.has(r.id)} onChange={() => toggleResult(r.id)} className="rounded" /></td>
                      <td className="p-3 text-slate-300 font-mono text-xs">{r.ip_address}{r.port ? `:${r.port}` : ''}</td>
                      <td className="p-3 text-slate-300">{r.manufacturer || '-'}</td>
                      <td className="p-3 text-slate-300">{r.model || '-'}</td>
                      <td className="p-3 text-slate-300">{r.firmware || '-'}</td>
                      <td className="p-3 text-slate-300 text-xs">{r.serial_number || '-'}</td>
                      <td className="p-3 text-slate-300 text-xs">{r.hostname || '-'}</td>
                      <td className="p-3">
                        <div className="flex flex-wrap gap-1">
                          {r.capabilities && Object.entries(r.capabilities).filter(([,v]) => v).map(([k]) => (
                            <span key={k} className={`text-[9px] px-1.5 py-0.5 rounded uppercase font-medium ${capColor(k)}`}>{k}</span>
                          ))}
                          {(!r.capabilities || Object.keys(r.capabilities).length === 0) && <span className="text-[9px] text-slate-600">-</span>}
                        </div>
                      </td>
                      <td className="p-3">
                        {r.already_in_db ? <span className="text-[10px] px-1.5 py-0.5 rounded bg-green-900/40 text-green-300">Yes</span>
                          : <span className="text-[10px] text-slate-600">No</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {resultTotal > 20 && (
            <div className="flex justify-center gap-2">
              <button disabled={resultPage <= 1} onClick={() => currentScanId && loadResults(currentScanId, resultPage - 1, resultQuery)}
                className="text-xs px-2 py-1 bg-slate-700 disabled:bg-slate-800 text-slate-300 rounded">Previous</button>
              <span className="text-xs text-slate-500 py-1">Page {resultPage} of {Math.ceil(resultTotal / 20)}</span>
              <button disabled={resultPage >= Math.ceil(resultTotal / 20)} onClick={() => currentScanId && loadResults(currentScanId, resultPage + 1, resultQuery)}
                className="text-xs px-2 py-1 bg-slate-700 disabled:bg-slate-800 text-slate-300 rounded">Next</button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
