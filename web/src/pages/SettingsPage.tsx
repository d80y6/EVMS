import React from 'react';

export default function SettingsPage() {
  return (
    <div className="max-w-lg">
      <h2 className="text-lg font-semibold text-slate-200 mb-6">Settings</h2>

      <div className="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-6">
        <div>
          <h3 className="text-sm font-medium text-slate-400 mb-2">Profile</h3>
          <p className="text-slate-500 text-sm">
            User profile and account settings will be available in a future update.
          </p>
        </div>

        <div className="border-t border-slate-800 pt-6">
          <h3 className="text-sm font-medium text-slate-400 mb-2">Notifications</h3>
          <p className="text-slate-500 text-sm">
            Notification preferences (email, webhook, push) are configured via the backend services.
          </p>
        </div>

        <div className="border-t border-slate-800 pt-6">
          <h3 className="text-sm font-medium text-slate-400 mb-2">Streaming</h3>
          <p className="text-slate-500 text-sm">
            WebRTC and recording settings are managed through the camera management service.
          </p>
        </div>

        <div className="border-t border-slate-800 pt-6">
          <h3 className="text-sm font-medium text-slate-400 mb-2">System</h3>
          <div className="text-xs text-slate-600 space-y-1">
            <p>DAM VMS v0.1.0</p>
            <p>React + Vite + Tailwind CSS</p>
            <p>WebRTC via Pion</p>
          </div>
        </div>
      </div>
    </div>
  );
}
