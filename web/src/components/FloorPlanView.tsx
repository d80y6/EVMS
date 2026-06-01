import { useEffect, useRef, useState } from 'react'
import { api, Camera } from '../api/client'

interface FloorPlanProps {
  imageUrl: string
  cameras: Camera[]
  siteId: string
  onCameraClick: (id: string) => void
}

export function FloorPlanView({ imageUrl, cameras, siteId: _siteId, onCameraClick }: FloorPlanProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [positions, setPositions] = useState<Record<string, { x: number; y: number }>>({})

  useEffect(() => {
    const pos: Record<string, { x: number; y: number }> = {}
    cameras.forEach(cam => {
      try {
        const config = cam.config ? JSON.parse(cam.config) : {}
        if (config.floor_plan_position) {
          pos[cam.id] = config.floor_plan_position
        }
      } catch { /* ignore parse errors */ }
    })
    setPositions(pos)
  }, [cameras])

  const savePosition = async (cameraId: string, x: number, y: number) => {
    const updated = { ...positions, [cameraId]: { x, y } }
    setPositions(updated)
    try {
      const cam = cameras.find(c => c.id === cameraId)
      const config = cam?.config ? JSON.parse(cam.config) : {}
      config.floor_plan_position = { x, y }
      await api.updateCameraConfig(cameraId, { config })
    } catch (e) {
      console.error('Failed to save position', e)
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online': return 'bg-green-500'
      case 'error': return 'bg-red-500'
      default: return 'bg-gray-500'
    }
  }

  return (
    <div ref={containerRef} className="relative w-full h-[600px] bg-gray-900 rounded overflow-hidden">
      <img src={imageUrl} alt="Floor Plan" className="w-full h-full object-contain" />
      {cameras.map(cam => {
        const pos = positions[cam.id]
        if (!pos) return null
        return (
          <div
            key={cam.id}
            className={`absolute w-6 h-6 rounded-full ${getStatusColor(cam.status)} border-2 border-white cursor-pointer flex items-center justify-center text-xs font-bold shadow-lg hover:scale-125 transition-transform`}
            style={{ left: `${pos.x * 100}%`, top: `${pos.y * 100}%`, transform: 'translate(-50%, -50%)' }}
            onClick={() => onCameraClick(cam.id)}
            draggable
            onDragEnd={(e) => {
              if (!containerRef.current) return
              const rect = containerRef.current.getBoundingClientRect()
              const x = (e.clientX - rect.left) / rect.width
              const y = (e.clientY - rect.top) / rect.height
              savePosition(cam.id, Math.max(0, Math.min(1, x)), Math.max(0, Math.min(1, y)))
            }}
          >
            <span className="text-white text-[10px]">{cam.name.charAt(0).toUpperCase()}</span>
          </div>
        )
      })}
    </div>
  )
}
