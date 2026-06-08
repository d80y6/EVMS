import { useEffect, useState } from 'react';
import { api } from '../api/client';

interface Camera { id: string; name: string; }

function DiagnosticsCard({ title, data }: { title: string; data: any }) {
  if (!data) return null;
  return (
    <details className="bg-slate-900 rounded-lg">
      <summary className="text-sm font-medium p-4 cursor-pointer hover:text-indigo-400 transition-colors">{title}</summary>
      <div className="px-4 pb-4">
        <pre className="text-xs text-slate-400 overflow-auto max-h-60">{JSON.stringify(data, null, 2)}</pre>
      </div>
    </details>
  );
}

export default function DevicePage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [selectedCamera, setSelectedCamera] = useState('');
  const [info, setInfo] = useState<any>(null);
  const [caps, setCaps] = useState<any>(null);
  const [interfaces, setInterfaces] = useState<any[]>([]);
  const [dns, setDns] = useState<any>(null);
  const [ntp, setNtp] = useState<any>(null);
  const [hostname, setHostname] = useState('');
  const [newHostname, setNewHostname] = useState('');
  const [dnsServers, setDnsServers] = useState('');
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState('');
  const [diagnostics, setDiagnostics] = useState<any>(null);
  const [serviceDebug, setServiceDebug] = useState<any>(null);

  useEffect(() => {
    api.getCameras().then(d => setCameras(d.cameras)).catch(() => {});
  }, []);

  useEffect(() => {
    if (!selectedCamera) return;
    setLoading(true);
    Promise.all([
      api.getDeviceInfo(selectedCamera).then(d => setInfo(d)).catch(() => setInfo(null)),
      api.getDeviceCapabilities(selectedCamera).then(d => setCaps(d)).catch(() => setCaps(null)),
      api.getNetworkInterfaces(selectedCamera).then(d => setInterfaces(d.interfaces || [])).catch(() => setInterfaces([])),
      api.getDNS(selectedCamera).then(d => { setDns(d); setDnsServers((d.dns_servers || []).join(', ')); }).catch(() => setDns(null)),
      api.getNTP(selectedCamera).then(d => setNtp(d)).catch(() => setNtp(null)),
      api.getHostname(selectedCamera).then(d => { setHostname(d.hostname || ''); setNewHostname(d.hostname || ''); }).catch(() => {}),
      api.getDeviceDiagnostics(selectedCamera).then(d => setDiagnostics(d)).catch(() => setDiagnostics(null)),
    ]).finally(() => setLoading(false));
    api.getServiceDebug().then(d => setServiceDebug(d)).catch(() => {});
  }, [selectedCamera]);

  const handleSetHostname = async () => {
    try {
      await api.setHostname(selectedCamera, newHostname);
      setHostname(newHostname);
      setMessage('Hostname updated');
    } catch { setMessage('Failed to set hostname'); }
  };

  const handleSetDNS = async () => {
    try {
      await api.setDNS(selectedCamera, false, dnsServers.split(',').map(s => s.trim()).filter(Boolean));
      setMessage('DNS updated');
    } catch { setMessage('Failed to set DNS'); }
  };

  const handleReboot = async () => {
    if (!window.confirm('Reboot the device?')) return;
    try {
      await api.rebootDevice(selectedCamera);
      setMessage('Reboot command sent');
    } catch { setMessage('Reboot failed'); }
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Device & Network</h1>

      <select value={selectedCamera} onChange={e => setSelectedCamera(e.target.value)}
        className="bg-slate-800 text-white rounded px-3 py-2 text-sm">
        <option value="">Select camera...</option>
        {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
      </select>

      {loading && <p className="text-sm text-slate-400">Loading device info...</p>}
      {message && <p className="text-sm text-green-400">{message}</p>}

      {info && (
        <div className="bg-slate-900 p-4 rounded-lg space-y-1">
          <h3 className="text-sm font-medium mb-2">Device Information</h3>
          {Object.entries(info).filter(([k]) => !k.startsWith('_')).map(([k, v]) => (
            <p key={k} className="text-xs text-slate-300"><span className="text-slate-500 capitalize">{k}:</span> {String(v)}</p>
          ))}
        </div>
      )}

      {caps && (
        <div className="bg-slate-900 p-4 rounded-lg">
          <h3 className="text-sm font-medium mb-2">Capabilities</h3>
          <div className="flex flex-wrap gap-2">
            {Object.entries(caps).filter(([_, v]) => v === true).map(([k]) => (
              <span key={k} className="text-[10px] px-2 py-0.5 bg-indigo-900/50 text-indigo-300 rounded">{k}</span>
            ))}
          </div>
        </div>
      )}

      <div className="grid grid-cols-2 gap-4">
        <div className="bg-slate-900 p-4 rounded-lg space-y-3">
          <h3 className="text-sm font-medium">Hostname</h3>
          <p className="text-xs text-slate-400">Current: {hostname || 'N/A'}</p>
          <input value={newHostname} onChange={e => setNewHostname(e.target.value)}
            className="w-full bg-slate-800 text-white rounded px-3 py-2 text-sm" placeholder="New hostname" />
          <button onClick={handleSetHostname} className="px-3 py-1 text-xs bg-indigo-600 rounded hover:bg-indigo-500">
            Set Hostname
          </button>
        </div>

        <div className="bg-slate-900 p-4 rounded-lg space-y-3">
          <h3 className="text-sm font-medium">DNS</h3>
          {dns && <p className="text-xs text-slate-400">DHCP: {dns.from_dhcp ? 'Yes' : 'No'}</p>}
          <input value={dnsServers} onChange={e => setDnsServers(e.target.value)}
            className="w-full bg-slate-800 text-white rounded px-3 py-2 text-sm" placeholder="DNS servers (comma-separated)" />
          <button onClick={handleSetDNS} className="px-3 py-1 text-xs bg-indigo-600 rounded hover:bg-indigo-500">
            Set DNS
          </button>
        </div>

        <div className="bg-slate-900 p-4 rounded-lg">
          <h3 className="text-sm font-medium mb-2">NTP</h3>
          {ntp ? (
            <div className="text-xs text-slate-300 space-y-1">
              {ntp.from_dhcp !== undefined && <p>From DHCP: {String(ntp.from_dhcp)}</p>}
              {(ntp.ntp_servers || []).length > 0 && <p>Servers: {(ntp.ntp_servers || []).join(', ')}</p>}
            </div>
          ) : <p className="text-xs text-slate-500">No NTP info</p>}
        </div>

        <div className="bg-slate-900 p-4 rounded-lg">
          <h3 className="text-sm font-medium mb-2">Actions</h3>
          <button onClick={handleReboot} className="px-3 py-1 text-xs bg-red-700 rounded hover:bg-red-600">
            Reboot Device
          </button>
        </div>
      </div>

      {interfaces.length > 0 && (
        <div className="bg-slate-900 p-4 rounded-lg">
          <h3 className="text-sm font-medium mb-2">Network Interfaces</h3>
          <div className="space-y-2">
            {interfaces.map((iface: any, i: number) => (
              <div key={i} className="text-xs text-slate-300 border-b border-slate-800 pb-2">
                <p><span className="text-slate-500">Name:</span> {iface.name || iface.interface_name || 'N/A'}</p>
                {iface.mac_address && <p><span className="text-slate-500">MAC:</span> {iface.mac_address}</p>}
                {iface.ipv4?.address && <p><span className="text-slate-500">IPv4:</span> {iface.ipv4.address}</p>}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="space-y-2">
        <h3 className="text-sm font-medium">Diagnostics</h3>
        <DiagnosticsCard title="Device Diagnostics" data={diagnostics} />
        <DiagnosticsCard title="Service Debug" data={serviceDebug} />
      </div>
    </div>
  );
}
