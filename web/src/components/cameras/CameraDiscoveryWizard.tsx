import { useEffect, useMemo, useState } from 'react';
import {
Search,
Loader2,
CheckCircle,
Camera,
Plus,
X,
} from 'lucide-react';
import { api } from '../../api/client';

interface Site {
id: string;
name: string;
location: string;
}

interface DiscoveryDevice {
ip: string;
manufacturer?: string;
model?: string;
serial_number?: string;
onvif?: boolean;
rtsp?: boolean;
}

interface CameraDiscoveryWizardProps {
open: boolean;
sites: Site[];
onClose: () => void;
onImported: () => Promise<void>;
}

export default function CameraDiscoveryWizard({
open,
sites,
onClose,
onImported,
}: CameraDiscoveryWizardProps) {
const [step, setStep] = useState(1);

const [loading, setLoading] =
useState(false);

const [testing, setTesting] =
useState(false);

const [error, setError] =
useState<string | null>(null);

const [scanId, setScanId] =
useState('');

const [subnet, setSubnet] =
useState(
'192.168.1.0/24'
);

const [devices, setDevices] =
useState<
DiscoveryDevice[]
>([]);

const [selected, setSelected] =
useState<string[]>([]);

const [siteId, setSiteId] =
useState('');

const [username, setUsername] =
useState('admin');

const [password, setPassword] =
useState('');

const [testResult, setTestResult] =
useState<any>(null);

useEffect(() => {
if (!open) {
resetWizard();
}
}, [open]);

const resetWizard = () => {
setStep(1);
setScanId('');
setDevices([]);
setSelected([]);
setSiteId('');
setError(null);
setTestResult(null);
};

const selectedDevices =
useMemo(
() =>
devices.filter((d) =>
selected.includes(
d.ip
)
),
[devices, selected]
);

const startScan =
async () => {
try {
setLoading(true);
setError(null);

      const scan =
        await api.startDiscoveryScan(
          {
            subnets: [subnet], site_id: siteId,
          }
        );

      const id =
        scan.id;

    setScanId(id);

    let attempts = 0;
    let completed =
      false;

    while (
      !completed &&
      attempts < 30
    ) {
      const status =
        await api.getDiscoveryScan(
          id
        );

      if (
        status.status ===
          'completed'
      ) {
        completed =
          true;
        break;
      }

      await new Promise(
        (resolve) =>
          setTimeout(
            resolve,
            2000
          )
      );

      attempts++;
    }

    const results =
      await api.getDiscoveryResults(
        id
      );

    const discovered =
      (results.results || []).map(
        (r) => ({
          ip: r.ip_address,
          manufacturer: r.manufacturer || '',
          model: r.model || '',
          serial_number: r.serial_number || '',
          onvif: !!(r.capabilities?.onvif),
          rtsp: !!(r.capabilities?.rtsp),
        })
      );

    setDevices(
      discovered
    );

    setSelected(
      discovered.map(
        (d) => d.ip
      )
    );

    setStep(2);
  } catch (err) {
    setError(
      err instanceof Error
        ? err.message
        : 'Discovery failed'
    );
  } finally {
    setLoading(false);
  }
};


const toggleDevice = (
ip: string
) => {
setSelected((prev) =>
prev.includes(ip)
? prev.filter(
(x) => x !== ip
)
: [...prev, ip]
);
};

const testCredentials =
async () => {
if (
selectedDevices.length ===
0
) {
return;
}

  try {
    setTesting(true);
    setError(null);

      const result =
        await api.testOnvifCredentials(
          {
            ip: selectedDevices[0]
              .ip,
            port: 80,
            username,
            password,
          }
        );

    setTestResult(
      result
    );

    setStep(4);
  } catch (err) {
    setError(
      err instanceof Error
        ? err.message
        : 'Credential validation failed'
    );
  } finally {
    setTesting(false);
  }
};


const importDevices =
async () => {
try {
setLoading(true);
setError(null);

      await api.importDiscoveryResults(
        scanId,
        {
          result_ids: selected,
          credentials: selected.map(ip => ({
            result_id: ip,
            username,
            password,
          })),
        }
      );

    await onImported();

    onClose();
  } catch (err) {
    setError(
      err instanceof Error
        ? err.message
        : 'Import failed'
    );
  } finally {
    setLoading(false);
  }
};

if (!open) {
return null;
}

return ( <div className="fixed inset-0 z-50">


  <div
    className="absolute inset-0 bg-black/70"
    onClick={onClose}
  />

  <div className="absolute inset-x-0 top-8 mx-auto w-full max-w-5xl bg-slate-900 border border-slate-800 rounded-xl shadow-2xl">

    {/* Header */}

    <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800">

      <div>

        <h2 className="text-lg font-semibold text-slate-200">
          Camera Discovery
        </h2>

        <p className="text-sm text-slate-500">
          Discover and
          import ONVIF
          cameras
        </p>

      </div>

      <button
        onClick={onClose}
        className="p-2 rounded bg-slate-800 hover:bg-slate-700"
      >
        <X size={16} />
      </button>

    </div>

    {/* Steps */}

    <div className="px-6 py-4 border-b border-slate-800 flex gap-6">

      <Step
        current={step}
        value={1}
        label="Scan"
      />

      <Step
        current={step}
        value={2}
        label="Select"
      />

      <Step
        current={step}
        value={3}
        label="Credentials"
      />

      <Step
        current={step}
        value={4}
        label="Import"
      />

    </div>

    {/* Body */}

    <div className="p-6 min-h-[500px]">

      {error && (
        <div className="mb-4 border border-red-800 bg-red-950/20 rounded-lg p-4 text-red-400">
          {error}
        </div>
      )}

      {step === 1 && (
        <div className="space-y-6">

          <div>

            <label className="block text-sm text-slate-400 mb-2">
              Subnet
            </label>

            <input
              value={subnet}
              onChange={(e) =>
                setSubnet(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            />

          </div>

          <button
            onClick={
              startScan
            }
            disabled={
              loading
            }
            className="px-4 py-2 rounded bg-indigo-600 hover:bg-indigo-500 text-white flex items-center gap-2"
          >
            {loading ? (
              <Loader2 className="animate-spin" />
            ) : (
              <Search />
            )}

            Start Discovery
          </button>

        </div>
      )}

      {step === 2 && (
        <div>

          <div className="flex justify-between mb-4">

            <div className="text-slate-400">
              {
                devices.length
              }{' '}
              devices found
            </div>

            <button
              onClick={() =>
                setSelected(
                  devices.map(
                    (
                      d
                    ) =>
                      d.ip
                  )
                )
              }
              className="text-indigo-400"
            >
              Select All
            </button>

          </div>

          <div className="space-y-2 max-h-[350px] overflow-auto">

            {devices.map(
              (
                device
              ) => (
                <div
                  key={
                    device.ip
                  }
                  className="flex items-center gap-4 border border-slate-800 rounded-lg p-3"
                >

                  <input
                    type="checkbox"
                    checked={selected.includes(
                      device.ip
                    )}
                    onChange={() =>
                      toggleDevice(
                        device.ip
                      )
                    }
                  />

                  <Camera />

                  <div className="flex-1">

                    <div className="text-slate-200 font-medium">
                      {
                        device.manufacturer
                      }{' '}
                      {
                        device.model
                      }
                    </div>

                    <div className="text-xs text-slate-500">
                      {
                        device.ip
                      }
                    </div>

                  </div>

                </div>
              )
            )}

          </div>

          <button
            onClick={() =>
              setStep(3)
            }
            className="mt-6 px-4 py-2 rounded bg-indigo-600 text-white"
          >
            Continue
          </button>

        </div>
      )}

      {step === 3 && (
        <div className="space-y-4">

          <input
            value={
              username
            }
            onChange={(e) =>
              setUsername(
                e.target.value
              )
            }
            placeholder="Username"
            className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
          />

          <input
            type="password"
            value={
              password
            }
            onChange={(e) =>
              setPassword(
                e.target.value
              )
            }
            placeholder="Password"
            className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
          />

          <button
            disabled={
              testing
            }
            onClick={
              testCredentials
            }
            className="px-4 py-2 rounded bg-indigo-600 text-white flex items-center gap-2"
          >
            {testing ? (
              <Loader2 className="animate-spin" />
            ) : (
              <CheckCircle />
            )}

            Validate
            Credentials
          </button>

        </div>
      )}

      {step === 4 && (
        <div className="space-y-6">

          {testResult && (
            <div className="border border-green-800 bg-green-950/20 rounded-lg p-4">

              <div className="flex items-center gap-2 text-green-400">

                <CheckCircle />

                Credentials
                validated

              </div>

            </div>
          )}

          <div>

            <label className="block text-sm text-slate-400 mb-2">
              Site
            </label>

            <select
              value={
                siteId
              }
              onChange={(e) =>
                setSiteId(
                  e.target.value
                )
              }
              className="w-full bg-slate-800 border border-slate-700 rounded px-3 py-2"
            >

              <option value="">
                Select Site
              </option>

              {sites.map(
                (
                  site
                ) => (
                  <option
                    key={
                      site.id
                    }
                    value={
                      site.id
                    }
                  >
                    {
                      site.name
                    }
                  </option>
                )
              )}

            </select>

          </div>

          <div className="border border-slate-800 rounded-lg p-4">

            <div className="font-medium text-slate-200">
              Import Summary
            </div>

            <div className="text-sm text-slate-500 mt-2">
              {
                selected.length
              }{' '}
              devices selected
            </div>

          </div>

          <button
            disabled={
              loading ||
              !siteId
            }
            onClick={
              importDevices
            }
            className="px-4 py-2 rounded bg-green-600 hover:bg-green-500 text-white flex items-center gap-2"
          >
            <Plus />

            Import Cameras
          </button>

        </div>
      )}

    </div>

  </div>

</div>


);
}

function Step({
current,
value,
label,
}: {
current: number;
value: number;
label: string;
}) {
return (
<div
className={`flex items-center gap-2 ${
        current >= value
          ? 'text-indigo-400'
          : 'text-slate-500'
      }`}
> <div className="w-6 h-6 rounded-full border flex items-center justify-center">
{value} </div>


  <span>{label}</span>
</div>


);
}
