import { useEffect, useRef } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import type { CameraMapPosition } from '../hooks/useMapCameras';

interface MapViewProps {
  positions: CameraMapPosition[];
  onCameraClick: (cameraId: string) => void;
  onPositionChange: (cameraId: string, lat: number, lng: number) => void;
}

export default function MapView({ positions, onCameraClick, onPositionChange }: MapViewProps) {
  const mapRef = useRef<L.Map | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;
    mapRef.current = L.map(containerRef.current).setView([40.7128, -74.006], 13);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap contributors',
    }).addTo(mapRef.current);
  }, []);

  useEffect(() => {
    if (!mapRef.current) return;
    const map = mapRef.current;
    map.eachLayer(layer => {
      if (layer instanceof L.Marker) layer.remove();
    });

    positions.forEach(pos => {
      const color = pos.status === 'online' ? '#22c55e' : pos.status === 'error' ? '#ef4444' : '#6b7280';
      const icon = L.divIcon({
        className: '',
        html: `<div style="width:16px;height:16px;border-radius:50%;background:${color};border:2px solid white;cursor:pointer;box-shadow:0 0 6px rgba(0,0,0,0.4)"></div>`,
        iconSize: [16, 16],
        iconAnchor: [8, 8],
      });
      const marker = L.marker([pos.lat, pos.lng], { icon, draggable: true })
        .addTo(map)
        .bindPopup(`<b>${pos.name}</b><br/>Status: ${pos.status}`);

      marker.on('click', () => onCameraClick(pos.cameraId));
      marker.on('dragend', () => {
        const ll = marker.getLatLng();
        onPositionChange(pos.cameraId, ll.lat, ll.lng);
      });
    });
  }, [positions, onCameraClick, onPositionChange]);

  return <div ref={containerRef} className="w-full h-full rounded" />;
}
