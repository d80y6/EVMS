import { useState, useRef, useEffect, useCallback } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { api, Camera } from '../api/client';

interface ImportedFeature {
  id: string;
  name: string;
  lat: number;
  lng: number;
}

function parseKML(text: string): GeoJSON.FeatureCollection {
  const parser = new DOMParser();
  const doc = parser.parseFromString(text, 'text/xml');
  const placemarks = doc.querySelectorAll('Placemark');
  const features: GeoJSON.Feature[] = [];
  placemarks.forEach(pm => {
    const name = pm.querySelector('name')?.textContent || '';
    const point = pm.querySelector('Point');
    if (point) {
      const coordsText = point.querySelector('coordinates')?.textContent;
      if (coordsText) {
        const parts = coordsText.trim().split(',').map(Number);
        if (parts.length >= 2) {
          features.push({
            type: 'Feature',
            geometry: { type: 'Point', coordinates: [parts[0], parts[1]] },
            properties: { name },
          });
        }
      }
    }
  });
  return { type: 'FeatureCollection', features };
}

export default function GISPage() {
  const mapRef = useRef<L.Map | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const geoLayerRef = useRef<L.GeoJSON | null>(null);
  const [features, setFeatures] = useState<ImportedFeature[]>([]);
  const [fileError, setFileError] = useState<string | null>(null);
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [assigningId, setAssigningId] = useState<string | null>(null);
  const [assignStatus, setAssignStatus] = useState<string | null>(null);

  useEffect(() => {
    api.listCameras().then(setCameras).catch(() => {});
  }, []);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    mapRef.current = L.map(containerRef.current).setView([40.7128, -74.006], 2);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap contributors',
    }).addTo(mapRef.current);
  }, []);

  const handleFile = useCallback(async (file: File) => {
    setFileError(null);
    const text = await file.text();
    let geojson: GeoJSON.FeatureCollection;

    if (file.name.endsWith('.kml') || file.name.endsWith('.kmz')) {
      geojson = parseKML(text);
    } else if (file.name.endsWith('.geojson') || file.name.endsWith('.json')) {
      geojson = JSON.parse(text);
    } else {
      setFileError('Unsupported file format. Please upload GeoJSON or KML files.');
      return;
    }

    if (!geojson.features || geojson.features.length === 0) {
      setFileError('No features found in the file.');
      return;
    }

    const imported: ImportedFeature[] = [];
    geojson.features.forEach((f, i) => {
      if (f.geometry?.type === 'Point') {
        const coords = (f.geometry as GeoJSON.Point).coordinates;
        imported.push({
          id: `feature-${i}`,
          name: (f.properties as Record<string, unknown>)?.name as string || `Feature ${i + 1}`,
          lat: coords[1],
          lng: coords[0],
        });
      }
    });

    setFeatures(imported);

    if (mapRef.current) {
      if (geoLayerRef.current) mapRef.current.removeLayer(geoLayerRef.current);
      geoLayerRef.current = L.geoJSON(geojson as unknown as GeoJSON.GeoJSON, {
        pointToLayer: (_, latlng) => L.circleMarker(latlng, {
          radius: 8, fillColor: '#22c55e', color: '#fff', weight: 2, fillOpacity: 0.8,
        }),
      }).addTo(mapRef.current);
      mapRef.current.fitBounds(geoLayerRef.current.getBounds());
    }
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  }, [handleFile]);

  const handleAssign = async (feature: ImportedFeature, cameraId: string) => {
    setAssignStatus(`Assigning ${feature.name}...`);
    try {
      await api.updateCameraConfig(cameraId, { map_position: { lat: feature.lat, lng: feature.lng } });
      setAssignStatus('Position assigned successfully!');
      setAssigningId(null);
    } catch {
      setAssignStatus('Failed to assign position.');
    }
    setTimeout(() => setAssignStatus(null), 3000);
  };

  return (
    <div className="h-full p-4 flex flex-col">
      <h2 className="text-lg font-semibold text-slate-200 mb-4">GIS Data Import</h2>

      <div
        onDrop={handleDrop}
        onDragOver={(e) => e.preventDefault()}
        className="border-2 border-dashed border-slate-700 rounded-lg p-8 text-center mb-4 hover:border-indigo-500 transition-colors cursor-pointer"
        onClick={() => document.getElementById('gis-file-input')?.click()}
      >
        <p className="text-slate-400 text-sm">Drop GeoJSON or KML file here, or click to browse</p>
        <input
          id="gis-file-input"
          type="file"
          accept=".geojson,.kml,.kmz,.json"
          className="hidden"
          onChange={(e) => e.target.files?.[0] && handleFile(e.target.files[0])}
        />
      </div>

      {fileError && <p className="text-red-400 text-sm mb-4">{fileError}</p>}
      {assignStatus && <p className="text-indigo-400 text-sm mb-4">{assignStatus}</p>}

      <div className="flex gap-4 flex-1 min-h-0">
        <div ref={containerRef} className="flex-1 rounded-lg overflow-hidden border border-slate-800" />

        {features.length > 0 && (
          <div className="w-80 bg-slate-900 rounded-lg border border-slate-800 p-4 overflow-y-auto shrink-0">
            <h3 className="text-sm font-semibold text-slate-300 mb-3">Imported Features ({features.length})</h3>
            <div className="space-y-2">
              {features.map((f) => (
                <div key={f.id} className="bg-slate-800 rounded p-3">
                  <p className="text-xs text-slate-300 font-medium">{f.name}</p>
                  <p className="text-[10px] text-slate-500">{f.lat.toFixed(6)}, {f.lng.toFixed(6)}</p>
                  {assigningId === f.id ? (
                    <select
                      className="mt-2 w-full text-xs bg-slate-700 text-slate-200 rounded px-2 py-1"
                      onChange={(e) => e.target.value && handleAssign(f, e.target.value)}
                      defaultValue=""
                    >
                      <option value="" disabled>Select camera...</option>
                      {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
                    </select>
                  ) : (
                    <button
                      onClick={() => { setAssigningId(f.id); setAssignStatus(null); }}
                      className="mt-2 text-xs text-indigo-400 hover:text-indigo-300"
                    >
                      Assign to Camera
                    </button>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
