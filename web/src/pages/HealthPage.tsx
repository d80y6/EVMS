import { useState, useEffect } from 'react';

interface ServiceHealth {
  name: string;
  status: string;
  error?: string;
}

interface SystemHealth {
  status: string;
  timestamp: string;
  services: ServiceHealth[];
}

export default function HealthPage() {
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const check = async () => {
      try {
        const resp = await fetch('/api/health/system');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data: SystemHealth = await resp.json();
        setHealth(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      }
    };
    check();
    const interval = setInterval(check, 15000);
    return () => clearInterval(interval);
  }, []);

  const statusColor = (status: string) => {
    switch (status) {
      case 'ok': return 'bg-green-500';
      case 'degraded': return 'bg-yellow-500';
      default: return 'bg-red-500';
    }
  };

  const statusBg = (status: string) => {
    switch (status) {
      case 'ok': return 'bg-green-900/20 border-green-800';
      case 'degraded': return 'bg-yellow-900/20 border-yellow-800';
      default: return 'bg-red-900/20 border-red-800';
    }
  };

  if (!health && !error) {
    return <div className="p-4 text-slate-400">Loading system health...</div>;
  }

  return (
    <div className="p-4">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-bold">System Health</h1>
        {health && (
          <div className="flex items-center gap-2">
            <span className={`w-3 h-3 rounded-full ${statusColor(health.status)}`} />
            <span className="text-sm text-slate-400 capitalize">{health.status}</span>
            <span className="text-xs text-slate-600">
              {new Date(health.timestamp).toLocaleTimeString()}
            </span>
          </div>
        )}
      </div>

      {error && (
        <div className="bg-red-900/20 border border-red-800 rounded-xl p-4 mb-4">
          <p className="text-red-400 text-sm">Failed to fetch health: {error}</p>
        </div>
      )}

      <div className="grid gap-3">
        {health?.services.map((svc) => (
          <div
            key={svc.name}
            className={`flex items-center justify-between p-4 rounded-xl border ${statusBg(svc.status)}`}
          >
            <div className="flex items-center gap-3">
              <span className={`w-2.5 h-2.5 rounded-full ${statusColor(svc.status)}`} />
              <span className="font-medium capitalize">{svc.name}</span>
            </div>
            <div className="flex items-center gap-2">
              <span className={`text-xs px-2 py-0.5 rounded-full ${
                svc.status === 'ok' ? 'text-green-400 bg-green-900/40' :
                svc.status === 'degraded' ? 'text-yellow-400 bg-yellow-900/40' :
                'text-red-400 bg-red-900/40'
              }`}>
                {svc.status}
              </span>
              {svc.error && (
                <span className="text-xs text-red-400 max-w-48 truncate" title={svc.error}>
                  {svc.error}
                </span>
              )}
            </div>
          </div>
        ))}
      </div>

      {health && health.services.length === 0 && (
        <div className="text-center py-12 text-slate-600">
          No services reported
        </div>
      )}
    </div>
  );
}
