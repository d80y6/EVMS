import { useState, useEffect } from 'react';

interface HealthResult {
  status: string;
  timestamp: string;
  checks?: Record<string, string>;
}

export default function HealthPage() {
  const [results, setResults] = useState<Record<string, HealthResult>>({});

  useEffect(() => {
    const check = async () => {
      try {
        const resp = await fetch('/api/health?check=db,nats,storage');
        const data = await resp.json();
        setResults(prev => ({ ...prev, gateway: data }));
      } catch {
        setResults(prev => ({ ...prev, gateway: { status: 'error', timestamp: '' } }));
      }
    };
    check();
    const interval = setInterval(check, 15000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="p-4">
      <h1 className="text-xl font-bold mb-4">System Health</h1>
      <div className="grid gap-4">
        {Object.entries(results).map(([service, data]) => (
          <div key={service} className="bg-slate-800 p-4 rounded-xl border border-slate-700">
            <div className="flex items-center gap-2">
              <span className={`w-3 h-3 rounded-full ${data.status === 'ok' ? 'bg-green-500' : 'bg-red-500'}`} />
              <span className="font-medium capitalize">{service}</span>
              <span className="text-sm text-slate-400">{data.status}</span>
            </div>
            {data.checks && (
              <div className="mt-2 ml-5 space-y-1">
                {Object.entries(data.checks).map(([check, status]) => (
                  <div key={check} className="flex gap-2 text-sm">
                    <span className="w-20 text-slate-400">{check}:</span>
                    <span className={status === 'ok' ? 'text-green-400' : 'text-red-400'}>{status}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
