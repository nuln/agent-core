import React from 'react'
import { Link, useLocation } from 'react-router-dom'
import { LayoutGrid, Brain, MessageSquare, Settings, Activity } from 'lucide-react'
import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

interface WebRoute {
  path: string
  label: string
  icon: string
  entry: string
}

interface UIAbility {
  routes: WebRoute[]
}

interface MainLayoutProps {
  children: React.ReactNode
  manifest: Record<string, UIAbility>
}

const ICON_MAP: Record<string, React.ReactNode> = {
  'layout-grid': <LayoutGrid size={20} />,
  'brain': <Brain size={20} />,
  'message-square': <MessageSquare size={20} />,
  'settings': <Settings size={20} />,
  'activity': <Activity size={20} />,
}

export default function MainLayout({ children, manifest }: MainLayoutProps) {
  const location = useLocation()

  return (
    <div className="flex h-screen w-full bg-[var(--surface-page)] text-[var(--text-primary)]">
      {/* Sidebar */}
      <aside className="w-64 flex-col border-r border-[var(--border-subtle)] bg-[var(--surface-card)] flex transition-[width] duration-200">
        <div className="flex h-16 items-center border-b border-[var(--border-subtle)] px-6">
          <span className="text-xl font-bold tracking-tight text-[var(--text-primary)]">AI Agent</span>
        </div>

        <nav className="flex-1 space-y-0.5 p-0 py-4 overflow-y-auto">
          <NavItem to="/" icon="layout-grid" label="Dashboard" active={location.pathname === '/'} />
          <NavItem to="/channels" icon="activity" label="Channels" active={location.pathname === '/channels'} />
          
          <div className="mt-8 mb-2 px-4 text-[10px] font-bold uppercase tracking-wider text-[var(--text-tertiary)]">
            Plugins
          </div>
          
          {Object.entries(manifest).map(([pluginId, ability]) => (
            ability.routes.map(route => (
              <NavItem 
                key={`${pluginId}-${route.path}`}
                to={route.path}
                icon={route.icon}
                label={route.label}
                active={location.pathname === route.path}
              />
            ))
          ))}
        </nav>

        <div className="border-t border-[var(--border-subtle)] p-0">
          <NavItem to="/settings" icon="settings" label="Settings" active={location.pathname === '/settings'} />
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col overflow-hidden">
        <header className="flex h-16 items-center border-b border-[var(--border-subtle)] bg-[var(--surface-card)] px-8">
          <h2 className="text-xs font-bold text-[var(--text-secondary)] uppercase tracking-[0.15em]">
            {location.pathname === '/' ? 'System Overview' : 'Plugin Console'}
          </h2>
        </header>
        <section className="flex-1 overflow-y-auto p-8">
          <div className="mx-auto max-w-6xl">
            {children}
          </div>
        </section>
      </main>
    </div>
  )
}

function NavItem({ to, icon, label, active }: { to: string; icon: string; label: string; active?: boolean }) {
  return (
    <Link
      to={to}
      className={cn(
        "flex items-center gap-3 px-4 py-2 text-sm font-medium transition-colors",
        active 
          ? "bg-[var(--color-gray-100)] text-[var(--color-accent)] border-r-2 border-[var(--color-accent)]" 
          : "text-[var(--text-secondary)] hover:bg-[var(--color-gray-50)] hover:text-[var(--text-primary)]"
      )}
    >
      <span className={cn(active ? "text-[var(--color-accent)]" : "text-[var(--text-tertiary)]")}>
        {ICON_MAP[icon] || <Activity size={20} />}
      </span>
      {label}
    </Link>
  )
}
