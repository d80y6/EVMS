import React from 'react';
import CameraView from './CameraView';

const Dashboard = () => {
  const cameras = [
    { id: 'demo_cam', name: 'Front Entrance' },
    { id: 'parking_lot', name: 'Parking Lot' },
    { id: 'warehouse', name: 'Main Warehouse' },
  ];

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans selection:bg-indigo-500/30">
      {/* Sidebar / Nav Placeholder */}
      <div className="flex h-screen">
        <aside className="w-64 border-r border-slate-800 p-6 flex flex-col gap-8">
          <div className="flex items-center gap-3 px-2">
            <div className="w-8 h-8 bg-indigo-600 rounded-lg flex items-center justify-center font-bold text-lg">D</div>
            <h1 className="text-xl font-bold tracking-tight">DAM VMS</h1>
          </div>

          <nav className="flex flex-col gap-2">
            <a href="#" className="px-4 py-2 bg-slate-800 rounded-md text-sm font-medium text-indigo-400">Live View</a>
            <a href="#" className="px-4 py-2 hover:bg-slate-900 rounded-md text-sm font-medium text-slate-400 transition-colors">Recordings</a>
            <a href="#" className="px-4 py-2 hover:bg-slate-900 rounded-md text-sm font-medium text-slate-400 transition-colors">Events</a>
            <a href="#" className="px-4 py-2 hover:bg-slate-900 rounded-md text-sm font-medium text-slate-400 transition-colors">Settings</a>
          </nav>
        </aside>

        <main className="flex-1 flex flex-col">
          <header className="h-16 border-b border-slate-800 px-8 flex items-center justify-between bg-slate-950/50 backdrop-blur-md sticky top-0 z-10">
            <div className="flex items-center gap-4">
              <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-widest">Global Operations Center</h2>
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]" />
            </div>

            <div className="flex items-center gap-6">
              <div className="text-right">
                <p className="text-xs font-bold text-slate-300">ADMIN-SYS-01</p>
                <p className="text-[10px] text-slate-500 uppercase font-medium">Enterprise Tier</p>
              </div>
              <div className="w-10 h-10 bg-slate-800 rounded-full border border-slate-700" />
            </div>
          </header>

          <div className="p-8 overflow-y-auto">
            <div className="grid grid-cols-1 xl:grid-cols-2 2xl:grid-cols-3 gap-8">
              {cameras.map((cam) => (
                <div key={cam.id} className="space-y-3">
                  <CameraView cameraId={cam.id} streamUrl="http://localhost:8082" />
                  <div className="flex justify-between items-center px-1">
                    <h3 className="text-sm font-bold text-slate-200">{cam.name}</h3>
                    <div className="flex gap-2">
                      <span className="text-[10px] px-2 py-0.5 bg-slate-800 text-slate-400 rounded-md font-bold border border-slate-700">H.264</span>
                      <span className="text-[10px] px-2 py-0.5 bg-slate-800 text-slate-400 rounded-md font-bold border border-slate-700">1080P</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </main>
      </div>
    </div>
  );
};

export default Dashboard;
