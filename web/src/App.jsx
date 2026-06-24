import React, { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useNavigate } from 'react-router-dom'
import { AppLogsProvider } from './logs/AppLogsContext.jsx'
import { Sidebar } from './components/Sidebar/index.jsx'
import { TopBar } from './components/TopBar/index.jsx'
import { LogsOverlay } from './components/LogsOverlay.jsx'
import { CommandPalette } from './components/CommandPalette.jsx'

/* Pages */
import { DashboardPage } from './pages/DashboardPage.jsx'
import { ViewerPage } from './pages/ViewerPage.jsx'
import { DownloadsPage } from './pages/DownloadsPage.jsx'
import { FlutterDownloadsPage } from './pages/FlutterDownloadsPage.jsx'
import { DevicesPage } from './pages/DevicesPage.jsx'
import { SessionsPage } from './pages/SessionsPage.jsx'
import { SecurityPage } from './pages/SecurityPage.jsx'
import { AIAssistantPage } from './pages/AIAssistantPage.jsx'
import { TeamsPage } from './pages/TeamsPage.jsx'
import { EnterprisePage } from './pages/EnterprisePage.jsx'
import { AnalyticsPage } from './pages/AnalyticsPage.jsx'
import { SettingsPage } from './pages/SettingsPage.jsx'

import './styles/globals.css'

function Shell() {
  var navigate    = useNavigate()
  var _useState   = useState(false)
  var paletteOpen = _useState[0]
  var setPaletteOpen = _useState[1]

  /* Global Cmd+K shortcut */
  useEffect(function() {
    function onKey(e) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setPaletteOpen(function(v) { return !v })
      }
    }
    window.addEventListener('keydown', onKey)
    return function() { window.removeEventListener('keydown', onKey) }
  }, [])

  function handleNavigate(target) { navigate(target) }

  return (
    <>
      <CommandPalette open={paletteOpen} onClose={function() { setPaletteOpen(false) }} navigate={handleNavigate} />
      <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <Sidebar />
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <TopBar />
        <main style={{
          flex: 1,
          overflow: 'auto',
          padding: '28px 28px',
          background: 'var(--bg-primary)',
        }}>
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/devices" element={<DevicesPage />} />
            <Route path="/remote" element={<ViewerPage />} />
            <Route path="/sessions" element={<SessionsPage />} />
            <Route path="/security" element={<SecurityPage />} />
            <Route path="/ai" element={<AIAssistantPage />} />
            <Route path="/analytics" element={<AnalyticsPage />} />
            <Route path="/teams" element={<TeamsPage />} />
            <Route path="/enterprise" element={<EnterprisePage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/downloads" element={<DownloadsPage />} />
            <Route path="/download-flutter" element={<FlutterDownloadsPage />} />
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </main>
      </div>
    </div>
    </>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AppLogsProvider>
        <Shell />
      </AppLogsProvider>
    </BrowserRouter>
  )
}