/**
 * useWails — abstracts the Wails Go↔JS bridge.
 *
 * In production (inside Wails), window.go is populated with all bound
 * Go methods. In dev (Vite), we return stubs so the UI can run standalone.
 */

import { useCallback, useEffect } from 'react'

// Wails injects this global in the webview.
const go = typeof window !== 'undefined' && window.go?.backend?.App
  ? window.go.backend.App
  : null

// Polyfill implementations for browser dev mode.
const polyfill = {
  GetVersion: async () => '1.0.0-web',
  GetRelayURL: async () => {
    return localStorage.getItem('remote_agent_relay') || `ws://${window.location.hostname}:8080/ws`
  },
  SetRelayURL: async (url) => {
    localStorage.setItem('remote_agent_relay', url)
  },
  GetSettings: async () => ({ unattended_password: '' }),
  SaveSettings: async (s) => {},
  GetLogs: async () => [],
  ClearLogs: async () => {},
  GetRecentConnections: async () => {
    try {
      const stored = localStorage.getItem('remote_agent_recents')
      return stored ? JSON.parse(stored) : []
    } catch {
      return []
    }
  },
  SaveRecentConnection: async (agentID) => {
    try {
      const stored = localStorage.getItem('remote_agent_recents')
      let recents = stored ? JSON.parse(stored) : []
      recents = recents.filter(r => r.agent_id !== agentID)
      recents.unshift({ agent_id: agentID, label: agentID, last_used: new Date().toISOString() })
      if (recents.length > 10) recents = recents.slice(0, 10)
      localStorage.setItem('remote_agent_recents', JSON.stringify(recents))
    } catch (e) {
      console.error(e)
    }
  },
  GetLocalAgent: async () => {
    return null // Pure browser has no local agent host
  },
  GetLocalIPs: async () => [],
  Connect: async (id, pass) => {
    return '' // empty string = success
  },
  Disconnect: async () => {},
  SendInputEvent: async () => '',
  GetSessionInfo: async () => ({
    agent_id: '',
    state: 'connected',
    latency_ms: 0,
    bitrate_kbps: 0,
    fps: 0,
    started_at: new Date().toISOString(),
  }),
}

const bridge = go || polyfill

export function useWails() {
  const getVersion         = useCallback(() => bridge.GetVersion(), [])
  const getRelayURL        = useCallback(() => bridge.GetRelayURL(), [])
  const setRelayURL        = useCallback((url) => bridge.SetRelayURL(url), [])
  const getSettings        = useCallback(() => bridge.GetSettings(), [])
  const saveSettings       = useCallback((s) => bridge.SaveSettings(s), [])
  const getLogs            = useCallback(() => bridge.GetLogs(), [])
  const clearLogs          = useCallback(() => bridge.ClearLogs(), [])
  const getRecents         = useCallback(() => bridge.GetRecentConnections(), [])
  const saveRecent         = useCallback((id) => {
    if (bridge.SaveRecentConnection) return bridge.SaveRecentConnection(id)
  }, [])
  const getLocalAgent      = useCallback(() => bridge.GetLocalAgent(), [])
  const getLocalIPs        = useCallback(() => bridge.GetLocalIPs(), [])
  const connect            = useCallback((id, pass) => bridge.Connect(id, pass), [])
  const disconnect         = useCallback(() => bridge.Disconnect(), [])
  const sendInput          = useCallback((ev) => bridge.SendInputEvent(JSON.stringify(ev)), [])
  const getSessionInfo     = useCallback(() => bridge.GetSessionInfo(), [])

  return { getVersion, getRelayURL, setRelayURL, getSettings, saveSettings, getLogs, clearLogs, getRecents, saveRecent, getLocalAgent, getLocalIPs, connect, disconnect, sendInput, getSessionInfo }
}

/**
 * useWailsEvents — subscribe to Wails backend events.
 * Falls back to a no-op in browser dev mode.
 */
const wailsRuntime = typeof window !== 'undefined' && window.runtime

export function useWailsEvent(eventName, handler) {
  useEffect(() => {
    if (!wailsRuntime) return
    wailsRuntime.EventsOn(eventName, handler)
    return () => wailsRuntime.EventsOff(eventName)
  }, [eventName, handler])
}
