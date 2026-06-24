import React, { useState, useCallback, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Monitor, Wifi, ChevronRight, Settings } from 'lucide-react'
import { useSignaling } from '../hooks/useSignaling.js'
import { useWebRTC } from '../hooks/useWebRTC.js'
import { SessionView } from '../components/SessionView.jsx'
import { useAppLogs } from '../logs/AppLogsContext.jsx'
import { apiUrl } from '../lib/api.js'
import styles from './ViewerPage.module.css'

const RELAY_URL = import.meta.env.VITE_RELAY_URL || 
  (location.port === '3000' || location.port === '5173' 
    ? `ws://${location.hostname}:8080/ws` 
    : `ws://${location.host}/ws`)

async function fetchICEServers() {
  const response = await fetch(apiUrl('/api/v1/session/ice-servers'))
  if (!response.ok) throw new Error(`ICE server request failed: ${response.status}`)
  const data = await response.json()
  return Array.isArray(data?.ice_servers) ? data.ice_servers : []
}

export function ViewerPage() {
  const [targetID, setTargetID] = useState('')
  const [password, setPassword] = useState('')
  const [phase, setPhase] = useState('idle')
  const [error, setError] = useState('')
  const [ownID, setOwnID] = useState('')
  const [peerID, setPeerID] = useState('')
  const [copied, setCopied] = useState(false)
  const [iceServers, setIceServers] = useState([])
  const [showSettings, setShowSettings] = useState(false)
  const idRef = useRef(null)
  const { log } = useAppLogs()

  const { connected, send, on } = useSignaling(RELAY_URL, { log })

  useEffect(() => {
    let cancelled = false
    fetchICEServers()
      .then((servers) => { if (!cancelled) setIceServers(servers) })
      .catch((err) => {
        console.warn('[viewer] failed to load ICE servers, using STUN defaults', err)
        if (!cancelled) setIceServers([])
      })
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    const parseHash = () => {
      const parts = location.hash.split('?')
      if (parts.length > 1) {
        const query = new URLSearchParams(parts[1])
        const idParam = query.get('id')
        if (idParam) {
          const digits = idParam.replace(/\D/g, '').slice(0, 9)
          if (digits.length === 9) setTargetID(`${digits.slice(0,3)} ${digits.slice(3,6)} ${digits.slice(6)}`)
          else setTargetID(idParam)
        }
      }
    }
    parseHash()
    window.addEventListener('hashchange', parseHash)
    return () => window.removeEventListener('hashchange', parseHash)
  }, [])

  useEffect(() => on('registered', (msg) => {
    setOwnID(msg.payload?.agent_id || '')
    log('info', 'viewer', 'browser agent registered', { agentID: msg.payload?.agent_id || '' })
  }), [on, log])

  useEffect(() => on('session.connect', (msg) => {
    if (msg.error) {
      setError(msg.error === 'agent not found' ? 'Agent is offline or unreachable' : msg.error)
      setPhase('error')
      log('error', 'viewer', 'signaling error', { error: msg.error })
    }
  }), [on, log])

  const { start, stop, stream, connectionState, displays, cursorInfo, sendInput, sendClipboard, sendChat, sendFile } = useWebRTC({ send, on, agentID: ownID, peerID, log, iceServers })

  const rawDigits = targetID.replace(/\D/g, '')
  const isValid   = rawDigits.length === 9 && connected

  const formatID = (raw) => {
    if (!raw) return ''
    const digits = String(raw).replace(/\D/g, '').slice(0, 9)
    if (digits.length > 6) return `${digits.slice(0,3)} ${digits.slice(3,6)} ${digits.slice(6)}`
    if (digits.length > 3) return `${digits.slice(0,3)} ${digits.slice(3)}`
    return digits
  }

  const handleIDChange = (e) => {
    setTargetID(formatID(e.target.value))
    setError('')
  }

  const handleConnect = useCallback(async () => {
    if (!isValid || phase === 'connecting') return
    setError('')
    setPhase('connecting')
    
    // Convert spaced ID (114 454 514) to hyphens (114-454-514) for the backend
    const fmt = `${rawDigits.slice(0,3)}-${rawDigits.slice(3,6)}-${rawDigits.slice(6)}`
    setPeerID(fmt)
    log('info', 'viewer', 'connect requested', { targetID: fmt })

    const cleanupSuccess = on('connect', async () => {
      setPhase('active')
      log('info', 'viewer', 'controller connect accepted', { targetID: fmt })
      try {
        await start(fmt)
      } catch (err) {
        console.error('[viewer] start error', err)
        setError('Failed to start WebRTC session')
        setPhase('error')
      }
      cleanup()
    })
    
    const cleanupError = on('session.connect', (msg) => {
      if (msg.error) {
        setError(msg.error === 'agent not found' ? 'Agent is offline or unreachable' : msg.error)
        setPhase('error')
        cleanup()
      }
    })

    const cleanup = () => { cleanupSuccess(); cleanupError() }

    send({ type: 'connect', payload: { target_id: fmt, password_hash: password } })

    setTimeout(() => {
      setPhase((prev) => {
        if (prev === 'connecting') {
          setError('Connection timed out')
          cleanup()
          return 'error'
        }
        return prev
      })
    }, 10000)
  }, [isValid, phase, rawDigits, password, send, on, start, log])

  const handleDisconnect = useCallback(() => {
    stop(peerID)
    setPhase('idle')
    setPeerID('')
    log('info', 'viewer', 'disconnect requested', { targetID: peerID })
  }, [stop, peerID, log])

  const handleCopyBrowserID = () => {
    if (ownID) {
      navigator.clipboard.writeText(formatID(ownID))
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
      log('info', 'viewer', 'browser ID copied to clipboard', { agentID: ownID })
    }
  }

  if (phase === 'active') {
    return (
      <div className={styles.fullscreen} style={{ width: '100vw', height: '100vh', background: '#000' }}>
        <SessionView 
          stream={stream}
          sessionInfo={{ agent_id: peerID, state: connectionState, started_at: new Date().toISOString() }}
          displays={displays}
          cursorInfo={cursorInfo}
          connectionMode={connectionState}
          qualityInfo={null}
          sendInput={sendInput} 
          sendClipboard={sendClipboard}
          sendChat={sendChat}
          sendFile={sendFile}
          log={log}
          onDisconnect={handleDisconnect} 
        />
        <div style={{ position: 'absolute', top: 10, left: 10, background: 'rgba(0,0,0,0.5)', color: '#fff', padding: '4px 8px', borderRadius: 4, zIndex: 10 }}>
          {connectionState}
        </div>
      </div>
    )
  }

  return (
    <div className={styles.root}>
      <div className={styles.container}>
        <div className={styles.splitView}>
          
          {/* LEFT PANEL: Browser Desk */}
          <div className={styles.panel}>
            <div className={styles.panelHeader}>
              <h2>Browser Desk</h2>
              <div className={`${styles.statusBadge} ${connected ? styles.online : styles.offline}`}>
                <span className={styles.statusDot} />
                {connected ? 'Relay Connected' : 'Offline'}
              </div>
            </div>
            
            <div className={styles.panelBody}>
              <div className={styles.infoGroup}>
                <label>Your Browser Address</label>
                <div 
                  className={styles.massiveDisplay} 
                  onClick={handleCopyBrowserID} 
                  title="Click to copy"
                >
                  {ownID ? formatID(ownID) : '--- --- ---'}
                  {copied && <span className={styles.copyTooltip}>Copied!</span>}
                </div>
              </div>

              <div className={styles.infoGroup}>
                <label>Access Password</label>
                <div className={styles.passwordDisplay}>
                  Not applicable for browser
                </div>
              </div>
              
              <div className={styles.panelActions}>
                <a href="/downloads" className={styles.btnGhost} style={{ textDecoration: 'none' }}>
                  Download Agents <ChevronRight size={14} />
                </a>
                <button className={styles.btnGhost} onClick={() => setShowSettings(!showSettings)}>
                  <Settings size={14} /> Settings
                </button>
              </div>

              <AnimatePresence>
                {showSettings && (
                  <motion.div
                    className={styles.settingsPanel}
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: 'auto', opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    style={{ overflow: 'hidden' }}
                  >
                    <label>Relay Server</label>
                    <input
                      className={styles.input}
                      value={RELAY_URL}
                      disabled
                    />
                    <p style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 8 }}>
                      Relay URL is determined by the environment.
                    </p>
                  </motion.div>
                )}
              </AnimatePresence>
            </div>
          </div>

          {/* RIGHT PANEL: Remote Desk */}
          <div className={styles.panel}>
            <div className={styles.panelHeader}>
              <h2>Remote Desk</h2>
            </div>
            
            <div className={styles.panelBody}>
              <div className={styles.infoGroup}>
                <label>Remote Address</label>
                <input
                  ref={idRef}
                  className={`${styles.massiveInput} ${error ? styles.inputError : ''}`}
                  type="text"
                  placeholder="000 000 000"
                  value={targetID}
                  onChange={handleIDChange}
                  onKeyDown={e => e.key === 'Enter' && handleConnect()}
                  maxLength={11}
                  autoComplete="off"
                />
              </div>

              <div className={styles.infoGroup}>
                <label>Password</label>
                <input
                  className={styles.standardInput}
                  type="password"
                  placeholder="Enter password..."
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleConnect()}
                />
              </div>

              <AnimatePresence>
                {error && (
                  <motion.div
                    className={styles.errorBox}
                    initial={{ opacity: 0, height: 0 }}
                    animate={{ opacity: 1, height: 'auto' }}
                    exit={{ opacity: 0, height: 0 }}
                  >
                    ⚠ {error}
                  </motion.div>
                )}
              </AnimatePresence>

              <div style={{ flex: 1 }} />

              <button
                className={`${styles.btnPrimary} ${phase === 'connecting' ? styles.btnLoading : ''}`}
                onClick={handleConnect}
                disabled={!isValid || phase === 'connecting'}
              >
                {phase === 'connecting' ? <><span className={styles.spinner} /> Connecting…</> : <><Wifi size={16} /> Connect</>}
              </button>
            </div>
          </div>
        </div>

      </div>
    </div>
  )
}
