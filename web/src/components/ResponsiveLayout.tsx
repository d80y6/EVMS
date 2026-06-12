import { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { useMediaQuery } from '../hooks/useMediaQuery';

const mobileNavItems = [
  { to: '/', label: 'Live', icon: '■' },
  { to: '/recordings', label: 'Recordings', icon: '▶' },
  { to: '/events', label: 'Events', icon: '!' },
  { to: '/map', label: 'Map', icon: '◉' },
  { to: '/settings', label: 'Settings', icon: '⚙' },
];

export default function ResponsiveLayout({ children }: { children: React.ReactNode }) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const isTablet = useMediaQuery('(min-width: 768px) and (max-width: 1024px)');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const location = useLocation();
  const isLoginPage = location.pathname === '/login';

  if (isMobile && !isLoginPage) {
    return (
      <>
        <div className="flex flex-col h-screen bg-slate-950 text-slate-50">
          <main className="flex-1 overflow-auto pb-16">
            {children}
          </main>
          <nav className="fixed bottom-0 left-0 right-0 z-50 bg-slate-900 border-t border-slate-800">
            <div className="flex justify-around items-center h-16 px-2">
              {mobileNavItems.map(item => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to === '/'}
                  className={({ isActive }) =>
                    `flex flex-col items-center gap-0.5 px-3 py-1.5 rounded-md text-[10px] font-medium transition-colors ${
                      isActive ? 'text-indigo-400' : 'text-slate-500 hover:text-slate-300'
                    }`
                  }
                >
                  <span className="text-lg">{item.icon}</span>
                  {item.label}
                </NavLink>
              ))}
            </div>
          </nav>
        </div>
        <style>{`
          @media (max-width: 767px) {
            #app-sidebar, #app-header { display: none !important; }
          }
        `}</style>
      </>
    );
  }

  if (isTablet && !isLoginPage) {
    return (
      <>
        <div data-sidebar-state={sidebarOpen ? 'open' : 'closed'}>
          {children}
        </div>
        <button
          onClick={() => setSidebarOpen(!sidebarOpen)}
          className="fixed left-4 bottom-4 z-50 w-10 h-10 bg-indigo-600 rounded-full flex items-center justify-center text-white shadow-lg hover:bg-indigo-500 transition-colors text-sm"
        >
          {sidebarOpen ? '◁' : '▷'}
        </button>
        <style>{`
          [data-sidebar-state="closed"] #app-sidebar { display: none !important; }
        `}</style>
      </>
    );
  }

  return <>{children}</>;
}
