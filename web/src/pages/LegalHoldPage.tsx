import { useState, useEffect } from "react"
import { apiClient } from "../api/client"

interface LegalHold {
  id: string
  camera_id: string
  reason: string
  created_by: string
  created_at: string
  released_at: string | null
}

export function LegalHoldPage() {
  const [holds, setHolds] = useState<LegalHold[]>([])
  const [showDialog, setShowDialog] = useState(false)
  const [cameraId, setCameraId] = useState("")
  const [reason, setReason] = useState("")
  const [loading, setLoading] = useState(true)

  const fetchHolds = async () => {
    try {
      const res = await apiClient.fetch("/api/legal-holds")
      const data = await res.json()
      setHolds(data.legal_holds || [])
    } catch (e) {
      console.error("Failed to fetch legal holds", e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchHolds()
  }, [])

  const handleCreate = async () => {
    if (!cameraId || !reason) return
    try {
      await apiClient.fetch("/api/legal-holds", {
        method: "POST",
        body: JSON.stringify({ camera_id: cameraId, reason, created_by: "admin" }),
        headers: { "Content-Type": "application/json" },
      })
      setShowDialog(false)
      setCameraId("")
      setReason("")
      await fetchHolds()
    } catch (e) {
      console.error("Failed to create legal hold", e)
    }
  }

  const handleRelease = async (id: string) => {
    if (!confirm("Are you sure you want to release this legal hold?")) return
    try {
      await apiClient.fetch(`/api/legal-holds/${id}/release`, { method: "POST" })
      await fetchHolds()
    } catch (e) {
      console.error("Failed to release legal hold", e)
    }
  }

  if (loading) return <div className="p-6">Loading legal holds...</div>

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-4">
        <h2 className="text-xl font-semibold">Legal Holds</h2>
        <button
          onClick={() => setShowDialog(true)}
          className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          New Legal Hold
        </button>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full border-collapse">
          <thead>
            <tr className="bg-gray-100">
              <th className="text-left p-2 border">Camera ID</th>
              <th className="text-left p-2 border">Reason</th>
              <th className="text-left p-2 border">Created By</th>
              <th className="text-left p-2 border">Created At</th>
              <th className="text-left p-2 border">Status</th>
              <th className="text-left p-2 border">Actions</th>
            </tr>
          </thead>
          <tbody>
            {holds.map((hold) => (
              <tr key={hold.id} className="border-b hover:bg-gray-50">
                <td className="p-2 border">{hold.camera_id}</td>
                <td className="p-2 border">{hold.reason}</td>
                <td className="p-2 border">{hold.created_by}</td>
                <td className="p-2 border">{new Date(hold.created_at).toLocaleString()}</td>
                <td className="p-2 border">
                  <span className={`px-2 py-1 rounded text-xs ${hold.released_at ? "bg-gray-200" : "bg-red-100 text-red-800"}`}>
                    {hold.released_at ? "Released" : "Active"}
                  </span>
                </td>
                <td className="p-2 border">
                  {!hold.released_at && (
                    <button
                      onClick={() => handleRelease(hold.id)}
                      className="px-3 py-1 bg-yellow-500 text-white rounded text-sm hover:bg-yellow-600"
                    >
                      Release
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {showDialog && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-96">
            <h3 className="text-lg font-semibold mb-4">New Legal Hold</h3>
            <div className="space-y-3">
              <input
                placeholder="Camera ID"
                value={cameraId}
                onChange={e => setCameraId(e.target.value)}
                className="w-full p-2 border rounded"
              />
              <textarea
                placeholder="Reason for legal hold"
                value={reason}
                onChange={e => setReason(e.target.value)}
                className="w-full p-2 border rounded"
                rows={3}
              />
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <button onClick={() => setShowDialog(false)} className="px-4 py-2 border rounded">Cancel</button>
              <button onClick={handleCreate} className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700">Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
