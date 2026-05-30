import React from 'react';
import { NavLink } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';

const navItems = [
  { to: '/', label: 'Live View', icon: '■' },
  { to: '/recordings', label: 'Recordings', icon: '▶' },
  { to: '/events', label: 'Events', icon: '!' },
  { to: '/settings', label: 'Settings', icon: '⚙' },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { logout } = useAuth();

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans selection:bg-indigo-500/30">
      <div className="flex h-screen">
        <aside className="w-64 border-r border-slate-800 p-6 flex flex-col gap-8">
          <div className="flex items-center gap-3 px-2">
            <div className="w-8 h-8 bg-indigo-600 rounded-lg flex items-center justify-center font-bold text-lg">D</div>
            <h1 className="text-xl font-bold tracking-tight">DAM VMS</h1>
          </div>

          <nav className="flex flex-col gap-2 flex-1">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) =>
                  `px-4 py-2 rounded-md text-sm font-medium transition-colors flex items-center gap-3 ${
                    isActive
                      ? 'bg-slate-800 text-indigo-400'
                      : 'text-slate-400 hover:bg-slate-900 hover:text-slate-300'
                  }`
                }
              >
                <span className="w-5 text-center">{item.icon}</span>
                {item.label}
              </NavLink>
            ))}
          </nav>

          <button
            onClick={logout}
            className="px-4 py-2 text-sm font-medium text-slate-500 hover:text-red-400 transition-colors text-left"
          >
            Sign Out
          </button>
        </aside>

        <main className="flex-1 flex flex-col">
          <header className="h-16 border-b border-slate-800 px-8 flex items-center justify-between bg-slate-950/50 backdrop-blur-md sticky top-0 z-10">
            <div className="flex items-center gap-4">
              <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-widest">
                Global Operations Center
              </h2>
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]" />
            </div>
          </header>

          <div className="p-8 overflow-y-auto flex-1">
            {children}
          </div>
        </main>
      </div>
    </div>
  );
}
