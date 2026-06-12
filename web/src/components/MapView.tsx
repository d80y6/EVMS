import { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import type { CameraMapPosition } from '../hooks/useMapCameras';

interface HeatmapCell {
  lat: number;
  lng: number;
  intensity: number;
}

interface MapViewProps {
  positions: CameraMapPosition[];
  onCameraClick: (cameraId: string) => void;
  onPositionChange: (cameraId: string, lat: number, lng: number) => void;
  floorPlanUrl?: string;
  floorPlanBounds?: [[number, number], [number, number]];
  showHeatmap?: boolean;
  heatmapData?: HeatmapCell[];
}

const entityMap: Record<string, string> = {
  '<': '&lt;', '>': '&gt;', '&': '&amp;', '"': '&quot;', "'": '&#39;',
};

function escapeHtml(s: string): string {
  return s.replace(/[<>&"']/g, c => entityMap[c] || c);
}

export default function MapView({
  positions,
  onCameraClick,
  onPositionChange,
  floorPlanUrl,
  floorPlanBounds,
  showHeatmap,
  heatmapData = [],
}: MapViewProps) {
  const mapRef = useRef<L.Map | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const floorPlanLayerRef = useRef<L.ImageOverlay | null>(null);
  const heatmapLayerRef = useRef<L.LayerGroup | null>(null);
  const markerLayerRef = useRef<L.LayerGroup | null>(null);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    mapRef.current = L.map(containerRef.current).setView([40.7128, -74.006], 13);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap contributors',
    }).addTo(mapRef.current);

    markerLayerRef.current = L.layerGroup().addTo(mapRef.current);
    heatmapLayerRef.current = L.layerGroup().addTo(mapRef.current);
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    if (floorPlanLayerRef.current) {
      map.removeLayer(floorPlanLayerRef.current);
      floorPlanLayerRef.current = null;
    }

    if (floorPlanUrl && floorPlanBounds) {
      floorPlanLayerRef.current = L.imageOverlay(floorPlanUrl, floorPlanBounds, {
        opacity: 0.8,
      }).addTo(map);
      map.fitBounds(floorPlanBounds);
    }
  }, [floorPlanUrl, floorPlanBounds]);

  useEffect(() => {
    const layer = markerLayerRef.current;
    if (!layer) return;
    layer.clearLayers();

    positions.forEach(pos => {
      const color = pos.status === 'online' ? '#22c55e' : pos.status === 'error' ? '#ef4444' : '#6b7280';
      const icon = L.divIcon({
        className: '',
        html: `<div style="width:16px;height:16px;border-radius:50%;background:${color};border:2px solid white;cursor:pointer;box-shadow:0 0 6px rgba(0,0,0,0.4)"></div>`,
        iconSize: [16, 16],
        iconAnchor: [8, 8],
      });
      const marker = L.marker([pos.lat, pos.lng], { icon, draggable: true })
        .addTo(layer)
        .bindPopup(`<b>${escapeHtml(pos.name)}</b><br/>Status: ${escapeHtml(pos.status)}`);

      marker.on('click', () => onCameraClick(pos.cameraId));
      marker.on('dragend', () => {
        const ll = marker.getLatLng();
        onPositionChange(pos.cameraId, ll.lat, ll.lng);
      });
    });
  }, [positions, onCameraClick, onPositionChange]);

  useEffect(() => {
    const layer = heatmapLayerRef.current;
    if (!layer) return;
    layer.clearLayers();

    if (showHeatmap && heatmapData.length > 0) {
      const maxIntensity = Math.max(...heatmapData.map(d => d.intensity), 1);
      heatmapData.forEach(cell => {
        const radius = Math.max(5, (cell.intensity / maxIntensity) * 30);
        const opacity = Math.min(0.8, 0.2 + (cell.intensity / maxIntensity) * 0.6);
        L.circle([cell.lat, cell.lng], {
          radius,
          color: '#ef4444',
          fillColor: '#ef4444',
          fillOpacity: opacity,
          weight: 0,
        }).addTo(layer);
      });
    }
  }, [showHeatmap, heatmapData]);

  return <div ref={containerRef} className="w-full h-full rounded" />;
}
