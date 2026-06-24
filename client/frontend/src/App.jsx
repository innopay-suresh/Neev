import React, { useState, useEffect, useCallback, useRef } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { BrowserRouter, Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { ConnectScreen } from './components/ConnectScreen.jsx'
import { SessionView } from './components/SessionView.jsx'
import { HostChatOverlay } from './components/HostChatOverlay.jsx'
import { LogsOverlay } from './components/LogsOverlay.jsx'
import { useWails } from './hooks/useWails.js'
import { useSignaling } from './hooks/useSignaling.js'
import { useWebRTC } from './hooks/useWebRTC.js'
import { Sidebar } from './components/Sidebar/index.jsx'
import { TopBar } from './components/TopBar/index.jsx'
import { CommandPalette } from './components/CommandPalette.jsx'

/* Pages */
import { DashboardPage } from './pages/DashboardPage.jsx'
import { DevicesPage } from './pages/DevicesPage.jsx'
import { RemoteAccessPage } from './pages/RemoteAccessPage.jsx'
import { SessionsPage } from './pages/SessionsPage.jsx'
import { SecurityPage } from './pages/SecurityPage.jsx'
import { AIAssistantPage } from './pages/AIAssistantPage.jsx'
import { TeamsPage } from './pages/TeamsPage.jsx'
import { AnalyticsPage } from './pages/AnalyticsPage.jsx'
import { SettingsPage } from './pages/SettingsPage.jsx'

import './styles/globals.css'

const isNative = typeof window !== 'undefined' && !!window.go?.backend?.App

async function fetchICEServers(relayURL) {
  const base = relayURL.startsWith('wss://') ? 'https' : 'http'
  const host = relayURL.replace(/^wss?:\/\//, '').replace(/\/ws\/?$/, '')
  const endpoint = base + '://' + host + '/api/v1/session/ice-servers'
  const response = await fetch(endpoint)
  if (!response.ok) throw new Error(`ICE server request failed: ${response.status}`)
  const data = await response.json()
  return Array.isArray(data?.ice_servers) ? data.ice_servers : []
}

/* ── App Shell (with sidebar layout) ──────────────────────────────────────── */
function Shell({ children, onOpenLogs }) {
  var navigate = useNavigate()
  var _useState = useState(false)
  var paletteOpen = _useState[0]
  var setPaletteOpen = _useState[1]

  useEffect(function() {
    function onKey(e) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') { e.preventDefault(); setPaletteOpen(function(v) { return !v }) }
    }
    window.addEventListener('keydown', onKey)
    return function() { window.removeEventListener('keydown', onKey) }
  }, [])

  function handleNavigate(t) { navigate(t) }

  return (
    <>
      <CommandPalette open={paletteOpen} onClose={function() { setPaletteOpen(false) }} navigate={handleNavigate} />
      <div style={{ display: 'flex', height: '100vh', overflow: 'hidden' }}>
      <Sidebar />
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <TopBar onOpenLogs={onOpenLogs} />
        <main style={{
          flex: 1, overflow: 'auto',
          padding: '28px 28px',
          background: 'var(--bg-primary)',
        }}>
          {children}
        </main>
      </div>
    </div>
    </>
  )
}

/* ── Remote Session (full-screen, no sidebar) ──────────────────────────────── */
function SessionScreen({ peerID, stopWebRTC, stream, connectionState, displays, cursorInfo, qualityInfo, sendInput, sendClipboard, sendChat, sendFile, onDisconnected }) {
  const handleDisconnect = useCallback(async () => {
    stopWebRTC(peerID)
    onDisconnected()
  }, [stopWebRTC, peerID, onDisconnected])

  return (
    <div style={{ height: '100vh', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <SessionView
        stream={stream}
        sessionInfo={{ state: connectionState }}
        displays={displays}
        qualityInfo={qualityInfo}
        sendInput={sendInput}
        sendClipboard={sendClipboard}
        sendChat={sendChat}
        sendFile={sendFile}
        onDisconnect={handleDisconnect}
        cursorInfo={cursorInfo}
      />
    </div>
  )
}

/* ── Main App (browser / localhost) ───────────────────────────────────────── */
export default function App() {
  const [version, setVersion] = useState('')
  const [recents, setRecents] = useState([])
  const [localAgent, setLocalAgent] = useState(null)
  const [logsOpen, setLogsOpen] = useState(false)

  const {
    getVersion, getRelayURL, setRelayURL: setWailsRelayURL,
    getSettings, saveSettings, getRecents, saveRecent,
    getLocalAgent: getLocalWailsAgent, getLocalIPs
  } = useWails()

  const browserOrigin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080'
  const defaultRelay = `${browserOrigin.replace(/^http/, 'ws')}/ws`
  const [relayURL, setRelayURLState] = useState(defaultRelay)
  const [iceServers, setIceServers] = useState([])

  const [peerID, setPeerID] = useState('')
  const [phase, setPhase] = useState('idle') // idle | connecting | active | error
  const [sessionInfo, setSessionInfo] = useState(null)

  const { connected: sigConnected, send, on } = useSignaling(relayURL)
  const { start: startWebRTC, stop: stopWebRTC, stream, connectionState, displays, cursorInfo, qualityInfo, sendInput, sendClipboard, sendChat, sendFile } = useWebRTC({ send, on, agentID: '', peerID, iceServers, getLocalIPs })

  /* ── Initialize ─────────────────────────────────────────────────────────── */
  useEffect(() => {
    getVersion().then(setVersion)
    if (isNative) {
      getRelayURL().then(url => { if (url) setRelayURLState(url) })
    }
    getRecents().then(res => setRecents(res || []))
    let interval
    const pollAgent = () => {
      getLocalWailsAgent().then(agent => {
        if (agent?.id) { setLocalAgent(agent); if (interval) clearInterval(interval) }
        else setLocalAgent(null)
      }).catch(() => setLocalAgent(null))
    }
    pollAgent()
    if (isNative) interval = setInterval(pollAgent, 2000)
    return () => { if (interval) clearInterval(interval) }
  }, [getVersion, getRelayURL, getRecents, getLocalWailsAgent])

  useEffect(() => {
    let cancelled = false
    fetchICEServers(relayURL)
      .then(servers => { if (!cancelled) setIceServers(servers) })
      .catch(() => { if (!cancelled) setIceServers([]) })
    return () => { cancelled = true }
  }, [relayURL])

  /* ── Session lifecycle ──────────────────────────────────────────────────── */
  const handleConnect = useCallback(async (agentID, password) => {
    if (phase === 'connecting') return 'Already connecting'
    setPhase('connecting')
    setPeerID(agentID)

    return new Promise((resolve) => {
      let isActive = false
      const cleanupSuccess = on('connect', async () => {
        isActive = true
        setPhase('active')
        setSessionInfo({ agent_id: agentID, started_at: new Date().toISOString() })
        try {
          await startWebRTC(agentID)
          if (!isNative && saveRecent) saveRecent(agentID)
          resolve(null)
        } catch (err) {
          resolve('Failed to start session')
          handleDisconnected()
        }
        cleanup()
      })
      const cleanupError = on('session.connect', (msg) => {
        if (msg.error) {
          isActive = true
          resolve(msg.error === 'agent not found' ? 'Agent is offline or unreachable' : msg.error)
          handleDisconnected()
          cleanup()
        }
      })

      const cleanup = () => {
        cleanupSuccess()
        cleanupError()
      }

      send({ type: 'connect', payload: { target_id: agentID, password_hash: password } })
      setTimeout(() => {
        cleanup()
        if (!isActive) { resolve('Connection timed out'); handleDisconnected() }
      }, 10000)
    })
  }, [phase, on, send, startWebRTC, saveRecent])

  const handleDisconnected = useCallback(() => {
    setPhase('idle')
    setPeerID('')
    setSessionInfo(null)
    getRecents().then(res => setRecents(res || []))
  }, [getRecents])

  const handleSetRelay = useCallback((url) => {
    setRelayURLState(url)
    if (isNative) setWailsRelayURL(url)
    else localStorage.setItem('remote_agent_relay', url)
  }, [setWailsRelayURL])

  const handleOpenLogs = useCallback(() => setLogsOpen(o => !o), [])

  /* ── Remote session active → full-screen session view ────────────────────── */
  if (phase === 'active' && peerID) {
    return (
      <>
        <SessionScreen
          peerID={peerID}
          stopWebRTC={stopWebRTC}
          stream={stream}
          connectionState={connectionState}
          displays={displays}
          cursorInfo={cursorInfo}
          qualityInfo={qualityInfo}
          sendInput={sendInput}
          sendClipboard={sendClipboard}
          sendChat={sendChat}
          sendFile={sendFile}
          onDisconnected={handleDisconnected}
        />
        <LogsOverlay />
      </>
    )
  }

  /* ── Normal app shell with sidebar ───────────────────────────────────────── */
  return (
    <BrowserRouter>
      <Routes>
        {/* Shell wrapper for all pages */}
        <Route path="/*" element={
          <Shell onOpenLogs={handleOpenLogs}>
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<DashboardPage localAgent={localAgent} />} />
              <Route path="/devices" element={<DevicesPage />} />
              <Route path="/remote" element={<Navigate to="/connect" replace />} />
              <Route path="/connect" element={
                <ConnectScreen
                  onConnect={handleConnect}
                  recents={recents}
                  relayURL={relayURL}
                  onSetRelay={handleSetRelay}
                  onOpenLogs={handleOpenLogs}
                  getSettings={getSettings}
                  saveSettings={saveSettings}
                  version={version}
                  localAgent={localAgent}
                  relayConnected={true}
                />
              } />
              <Route path="/sessions" element={<SessionsPage />} />
              <Route path="/security" element={<SecurityPage />} />
              <Route path="/ai" element={<AIAssistantPage />} />
              <Route path="/analytics" element={<AnalyticsPage />} />
              <Route path="/teams" element={<TeamsPage />} />
              <Route path="/settings" element={<SettingsPage getSettings={getSettings} saveSettings={saveSettings} version={version} />} />
              <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
          </Shell>
        } />
      </Routes>
      <LogsOverlay open={logsOpen} onClose={() => setLogsOpen(false)} />
    </BrowserRouter>
  )
}