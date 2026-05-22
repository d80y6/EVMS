import React from 'react';

const Dashboard = () => {
  return (
    <div className="min-h-screen bg-slate-900 text-white p-8">
      <header className="flex justify-between items-center mb-8">
        <h1 className="text-3xl font-bold tracking-tight">DAM VMS</h1>
        <div className="flex gap-4">
          <span className="px-3 py-1 bg-green-500/20 text-green-400 rounded-full text-sm font-medium border border-green-500/30">
            System Online
          </span>
        </div>
      </header>

      <main>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {/* Camera Card Placeholder */}
          <div className="aspect-video bg-slate-800 rounded-lg border border-slate-700 flex items-center justify-center group cursor-pointer hover:border-slate-500 transition-all">
            <span className="text-slate-500 group-hover:text-slate-300">Camera Feed Loading...</span>
          </div>
          <div className="aspect-video bg-slate-800 rounded-lg border border-slate-700 flex items-center justify-center group cursor-pointer hover:border-slate-500 transition-all">
            <span className="text-slate-500 group-hover:text-slate-300">Camera Feed Loading...</span>
          </div>
        </div>
      </main>
    </div>
  );
};

export default Dashboard;
