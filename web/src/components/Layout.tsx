import React, { useEffect, useState } from 'react';
import { NavLink, useSearchParams } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { api } from '../api/client';

interface NavItem {
  to: string;
  label: string;
  icon: string;
  adminOnly?: boolean;
}

interface SiteItem {
  id: string;
  name: string;
  location: string;
}

const navItems: NavItem[] = [
  { to: '/', label: 'Live View', icon: '■' },
  { to: '/recordings', label: 'Recordings', icon: '▶' },
  { to: '/events', label: 'Events', icon: '!' },
  { to: '/map', label: 'Map', icon: '◉' },
  { to: '/search', label: 'Search', icon: '⌕' },
  { to: '/health', label: 'Health', icon: '♥' },
  { to: '/storage', label: 'Storage', icon: '💾' },
  { to: '/admin', label: 'Admin', icon: '⚙', adminOnly: true },
  { to: '/settings', label: 'Settings', icon: '⚙' },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const { logout, role, username } = useAuth();
  const [sites, setSites] = useState<SiteItem[]>([]);
  const [searchParams, setSearchParams] = useSearchParams();
  const [showCamerasSub, setShowCamerasSub] = useState(false);
  const [showMonitoringSub, setShowMonitoringSub] = useState(false);
  const selectedSite = searchParams.get('site') || '';

  useEffect(() => {
    api.getSites()
      .then((data) => setSites(data.sites))
      .catch(() => {});
  }, []);

  const handleSiteClick = (siteId: string) => {
    const params = new URLSearchParams(searchParams);
    if (params.get('site') === siteId) {
      params.delete('site');
    } else {
      params.set('site', siteId);
    }
    setSearchParams(params);
  };

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 font-sans selection:bg-indigo-500/30">
      <div className="flex h-screen">
        <aside className="w-64 border-r border-slate-800 flex flex-col">
          <div className="p-6 pb-4">
            <div className="flex items-center gap-3 px-2">
              <div className="w-8 h-8 bg-indigo-600 rounded-lg flex items-center justify-center font-bold text-lg">D</div>
              <h1 className="text-xl font-bold tracking-tight">DAM VMS</h1>
            </div>
          </div>

          <nav className="flex flex-col gap-1 px-4 pb-4 border-b border-slate-800">
            {navItems
              .filter((item) => !item.adminOnly || role === 'admin')
              .map((item) => (
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

          <div className="flex flex-col gap-1 px-4 pb-2 border-b border-slate-800">
            <button onClick={() => setShowCamerasSub(!showCamerasSub)}
              className="flex items-center gap-3 px-4 py-2 rounded-md text-sm font-medium text-slate-500 hover:text-slate-300 hover:bg-slate-900 transition-colors">
              <span className="w-5 text-center text-xs">{showCamerasSub ? '▾' : '▸'}</span>
              Cameras & Retention
            </button>
            {showCamerasSub && (
              <div className="flex flex-col gap-0.5 ml-4">
                <NavLink to="/cameras" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">📷</span>Cameras
                </NavLink>
                <NavLink to="/legal-holds" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">⚖</span>Legal Holds
                </NavLink>
                <NavLink to="/discovery" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🔍</span>Discovery
                </NavLink>
                <NavLink to="/onvif-events" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🔔</span>ONVIF Events
                </NavLink>
                <NavLink to="/onvif-recordings" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">📼</span>ONVIF Recordings
                </NavLink>
                <NavLink to="/imaging" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🎨</span>Imaging
                </NavLink>
                <NavLink to="/devices" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🌐</span>Device/Network
                </NavLink>
                <NavLink to="/bookmarks" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🔖</span>Bookmarks
                </NavLink>
                <NavLink to="/export" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">📤</span>Export
                </NavLink>
              </div>
            )}
            <button onClick={() => setShowMonitoringSub(!showMonitoringSub)}
              className="flex items-center gap-3 px-4 py-2 rounded-md text-sm font-medium text-slate-500 hover:text-slate-300 hover:bg-slate-900 transition-colors">
              <span className="w-5 text-center text-xs">{showMonitoringSub ? '▾' : '▸'}</span>
              Monitoring
            </button>
            {showMonitoringSub && (
              <div className="flex flex-col gap-0.5 ml-4">
                <NavLink to="/alerts" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">⚠</span>Alerts & Rules
                </NavLink>
                <NavLink to="/analytics" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">📊</span>Analytics
                </NavLink>
                <NavLink to="/audit" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">📋</span>Audit Chain
                </NavLink>
                <NavLink to="/webhooks" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🔗</span>Webhooks
                </NavLink>
                <NavLink to="/pos" className={({ isActive }) => `px-4 py-1.5 rounded-md text-xs font-medium transition-colors flex items-center gap-3 ${isActive ? 'bg-slate-800 text-indigo-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'}`}>
                  <span className="w-4 text-center">🛒</span>POS Transactions
                </NavLink>
              </div>
            )}
          </div>

          <div className="flex-1 overflow-y-auto px-4 py-4 space-y-1">
            <h3 className="text-[10px] uppercase tracking-widest text-slate-600 px-2 pb-2 font-medium">Sites</h3>
            {sites.length === 0 && (
              <p className="text-xs text-slate-700 px-2">No sites configured</p>
            )}
            {sites.map((site) => (
              <button
                key={site.id}
                onClick={() => handleSiteClick(site.id)}
                className={`w-full px-4 py-1.5 rounded-md text-xs font-medium transition-colors text-left flex items-center gap-2 ${
                  selectedSite === site.id
                    ? 'bg-slate-800 text-indigo-400'
                    : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'
                }`}
              >
                <span className="text-[10px]">■</span>
                <span>{site.name}</span>
                {site.location && (
                  <span className="text-[9px] text-slate-700 ml-auto">{site.location}</span>
                )}
              </button>
            ))}
            <button
              onClick={() => {
                const params = new URLSearchParams(searchParams);
                params.delete('site');
                setSearchParams(params);
              }}
              className={`w-full px-4 py-1.5 rounded-md text-xs font-medium transition-colors text-left ${
                !selectedSite
                  ? 'bg-slate-800 text-indigo-400'
                  : 'text-slate-500 hover:text-slate-300 hover:bg-slate-900'
              }`}
            >
              All Sites
            </button>
          </div>

          <div className="p-4 border-t border-slate-800 space-y-2">
            <div className="px-2 text-xs text-slate-600">
              <span className="block">{username}</span>
              <span className="block uppercase tracking-wider text-[10px]">{role}</span>
            </div>
            <button
              onClick={logout}
              className="w-full px-4 py-2 text-sm font-medium text-slate-500 hover:text-red-400 transition-colors text-left rounded-md"
            >
              Sign Out
            </button>
          </div>
        </aside>

        <main className="flex-1 flex flex-col">
          <header className="h-16 border-b border-slate-800 px-8 flex items-center justify-between bg-slate-950/50 backdrop-blur-md sticky top-0 z-10">
            <div className="flex items-center gap-4">
              <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-widest">
                Global Operations Center
              </h2>
              <span className="w-1.5 h-1.5 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.6)]" />
              {selectedSite && (
                <span className="text-xs text-indigo-400 bg-indigo-900/30 px-2 py-0.5 rounded-full">
                  {sites.find(s => s.id === selectedSite)?.name || selectedSite}
                </span>
              )}
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
