import { useState } from 'react';
import { api } from '../api/client';

interface DiscoveredDevice {
  url: string;
  manufacturer: string;
  model: string;
  firmware_version: string;
  scopes: string[];
}

export default function DiscoveryPage() {
  const [scanning, setScanning] = useState(false);
  const [devices, setDevices] = useState<DiscoveredDevice[]>([]);
  const [error, setError] = useState<string | null>(null);

  const handleScan = async () => {
    setScanning(true);
    setError(null);
    setDevices([]);
    try {
      await api.scanDiscovery();
      setTimeout(async () => {
        try {
          const data = await api.getDiscoveryResults();
          setDevices(data.devices || []);
        } catch {
          setError('Failed to get discovery results');
        } finally {
          setScanning(false);
        }
      }, 3000);
    } catch {
      setError('Failed to start discovery scan');
      setScanning(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-200">Network Discovery</h2>
        <button onClick={handleScan} disabled={scanning}
          className="text-xs px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:bg-indigo-800 text-white rounded transition-colors">
          {scanning ? 'Scanning...' : 'Scan Network'}
        </button>
      </div>

      {error && <div className="bg-red-900/20 border border-red-800 rounded-xl p-4"><p className="text-sm text-red-400">{error}</p></div>}

      {scanning && <div className="text-sm text-slate-400 animate-pulse">Scanning network, please wait...</div>}

      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
        {devices.length === 0 && !scanning && <p className="p-6 text-sm text-slate-500">No devices discovered. Click "Scan Network" to start.</p>}
        {devices.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-slate-400 border-b border-slate-800 text-left">
                <th className="p-3">URL</th><th className="p-3">Manufacturer</th><th className="p-3">Model</th><th className="p-3">Firmware</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((d, i) => (
                <tr key={i} className="border-b border-slate-800 hover:bg-slate-800/50">
                  <td className="p-3 text-slate-300 font-mono text-xs">{d.url}</td>
                  <td className="p-3 text-slate-300">{d.manufacturer}</td>
                  <td className="p-3 text-slate-300">{d.model}</td>
                  <td className="p-3 text-slate-300">{d.firmware_version}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
