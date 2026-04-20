import React, { useEffect, useState, Suspense } from 'react'
import { Routes, Route } from 'react-router-dom'
import MainLayout from './layout/MainLayout'
import axios from 'axios'
import { PLUGIN_COMPONENTS } from './generated/plugin-registry'
import Dashboard from './pages/Dashboard'
import Channels from './pages/Channels'

interface WebRoute {
  path: string
  label: string
  icon: string
  entry: string
}

interface UIAbility {
  routes: WebRoute[]
}

export default function App() {
  const [manifest, setManifest] = useState<Record<string, UIAbility>>({})

  useEffect(() => {
    // Fetch plugin manifest from Go Backend
    axios.get('/api/v1/ui/manifest')
      .then(res => setManifest(res.data))
      .catch(err => console.error('Failed to load UI manifest', err))
  }, [])

  return (
    <MainLayout manifest={manifest}>
      <Suspense fallback={<div className="flex h-full items-center justify-center text-slate-400">Loading...</div>}>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/channels" element={<Channels />} />
          {/* Plugin routes dynamically registered */}
          {Object.entries(manifest).map(([pluginId, ability]) => {
            const Component = PLUGIN_COMPONENTS[pluginId];
            return ability.routes.map(route => (
              <Route 
                key={`${pluginId}-${route.path}`} 
                path={route.path} 
                element={Component ? <Component /> : <div>Plugin component not found: {pluginId}</div>} 
              />
            ))
          })}
        </Routes>
      </Suspense>
    </MainLayout>
  )
}
