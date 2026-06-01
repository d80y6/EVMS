import { useState, useEffect } from 'react'
import { apiClient } from '../api/client'

interface StorageEstimate {
  camera_id: string
  camera_name: string
  retention_days: number
  daily_usage_gb: number
  current_usage_gb: number
  estimated_total_gb: number
  days_remaining: number
}

export default function StoragePage() {
  const [estimates, setEstimates] = useState<StorageEstimate[]>([])
  const [totals, setTotals] = useState({ total_daily_gb: 0, total_storage_gb: 0 })
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    apiClient.fetch('/api/storage/estimates')
      .then(r => r.json())
      .then(data => {
        setEstimates(data.estimates || [])
        setTotals({ total_daily_gb: data.total_daily_gb, total_storage_gb: data.total_storage_gb })
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const totalEstimated = estimates.reduce((s, e) => s + e.estimated_total_gb, 0)

  if (loading) return <div className="p-4">Loading storage estimates...</div>

  return (
    <div className="p-4">
      <h1 className="text-xl font-bold mb-4">Storage Planning</h1>

      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-slate-800 p-4 rounded-lg">
          <div className="text-sm text-slate-400">Daily Ingest</div>
          <div className="text-2xl font-bold text-white">{totals.total_daily_gb.toFixed(1)} GB</div>
        </div>
        <div className="bg-slate-800 p-4 rounded-lg">
          <div className="text-sm text-slate-400">Current Usage</div>
          <div className="text-2xl font-bold text-white">{totals.total_storage_gb.toFixed(1)} GB</div>
        </div>
        <div className="bg-slate-800 p-4 rounded-lg">
          <div className="text-sm text-slate-400">Estimated @ Retention</div>
          <div className="text-2xl font-bold text-white">{totalEstimated.toFixed(1)} GB</div>
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-slate-400 border-b border-slate-700">
              <th className="text-left p-2">Camera</th>
              <th className="text-right p-2">Retention</th>
              <th className="text-right p-2">Daily</th>
              <th className="text-right p-2">Current</th>
              <th className="text-right p-2">Est. Total</th>
              <th className="text-right p-2">Days Left</th>
            </tr>
          </thead>
          <tbody>
            {estimates.map(e => (
              <tr key={e.camera_id} className="border-b border-slate-800 hover:bg-slate-800/50">
                <td className="p-2 text-white">{e.camera_name}</td>
                <td className="p-2 text-right text-slate-300">{e.retention_days}d</td>
                <td className="p-2 text-right text-slate-300">{e.daily_usage_gb.toFixed(1)} GB</td>
                <td className="p-2 text-right text-slate-300">{e.current_usage_gb.toFixed(1)} GB</td>
                <td className="p-2 text-right text-slate-300">{e.estimated_total_gb.toFixed(1)} GB</td>
                <td className="p-2 text-right text-slate-300">{e.days_remaining.toFixed(0)}d</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
