import React, { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Monitor, Wifi, Clock, ChevronRight, Clipboard, Settings } from 'lucide-react'
import styles from './ConnectScreen.module.css'

const isNative = typeof window !== 'undefined' && !!window.go?.backend?.App

/**
 * ConnectScreen — refactored to match AnyDesk split dashboard style.
 * Displays "This Desk" local agent info in the left panel, and
 * "Remote Desk" in the right panel.
 */
export function ConnectScreen({ onConnect, recents = [], relayURL, onSetRelay, onOpenLogs, getSettings, saveSettings, version, localAgent, relayConnected }) {
  const [agentID, setAgentID] = useState('')
  const [password, setPassword] = useState('')
  const [connecting, setConnecting] = useState(false)
  const [error, setError] = useState('')
  const [showSettings, setShowSettings] = useState(false)
  const [newRelay, setNewRelay] = useState(relayURL)
  const [unattendedPass, setUnattendedPass] = useState('')
  const [copied, setCopied] = useState(false)
  const idRef = useRef(null)

  useEffect(() => { 
    idRef.current?.focus() 
    if (getSettings) {
      getSettings().then(s => {
        if (s) setUnattendedPass(s.unattended_password || '')
      })
    }
  }, [getSettings])

  useEffect(() => {
    setNewRelay(relayURL)
  }, [relayURL])

  const formatID = (raw) => {
    if (!raw) return ''
    const digits = String(raw).replace(/\D/g, '').slice(0, 9)
    if (digits.length > 6) return `${digits.slice(0,3)} ${digits.slice(3,6)} ${digits.slice(6)}`
    if (digits.length > 3) return `${digits.slice(0,3)} ${digits.slice(3)}`
    return digits
  }

  const handleIDChange = (e) => {
    setAgentID(formatID(e.target.value))
    setError('')
  }

  const rawDigits = agentID.replace(/\D/g, '')
  const isValid = rawDigits.length === 9

  const handleConnect = async () => {
    if (!isValid || connecting) return
    setConnecting(true)
    setError('')
    const targetID = `${rawDigits.slice(0,3)}-${rawDigits.slice(3,6)}-${rawDigits.slice(6)}`
    const err = await onConnect(targetID, password)
    if (err) {
      setError(err)
      setConnecting(false)
    }
  }

  const handleRecentClick = (id) => {
    setAgentID(formatID(id.replace(/\s/g, '')))
    idRef.current?.focus()
  }

  const handleCopyID = () => {
    if (localAgent?.id) {
      navigator.clipboard.writeText(formatID(localAgent.id))
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  const allDevices = recents.map(r => ({
    agent_id: r.agent_id,
    label: r.label || `Recent Desk (${r.agent_id})`,
    os: 'windows',
    status: 'offline'
  }))

  return (
    <div className={styles.root}>
      <div className={styles.container}>
        
        {/* TOP SPLIT VIEW */}
        <div className={styles.splitView}>
          
          {/* LEFT PANEL: This Desk */}
          <div className={styles.panel}>
            <div className={styles.panelHeader}>
              <h2>This Desk</h2>
              <div className={`${styles.statusBadge} ${relayConnected ? styles.online : styles.offline}`}>
                <span className={styles.statusDot} />
                {relayConnected ? 'Online' : 'Offline'}
              </div>
            </div>
            
            <div className={styles.panelBody}>
              <div className={styles.infoGroup}>
                <label>Your Address</label>
                <div 
                  className={styles.massiveDisplay} 
                  onClick={handleCopyID} 
                  title="Click to copy"
                >
                  {localAgent ? formatID(localAgent.id) : '--- --- ---'}
                  {copied && <span className={styles.copyTooltip}>Copied!</span>}
                </div>
              </div>

              <div className={styles.infoGroup}>
                <label>Access Password</label>
                <div className={styles.passwordDisplay}>
                  {localAgent ? localAgent.password : '••••••••'}
                </div>
              </div>
              
              <div className={styles.panelActions}>
                <button className={styles.btnGhost} onClick={onOpenLogs}>View Logs</button>
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
                  >
                    <label>Relay Server</label>
                    <input
                      className={styles.input}
                      value={newRelay}
                      onChange={e => setNewRelay(e.target.value)}
                    />
                    {isNative && (
                      <>
                        <label>Unattended Access Password</label>
                        <input
                          className={styles.input}
                          type="password"
                          value={unattendedPass}
                          onChange={e => setUnattendedPass(e.target.value)}
                          placeholder="Optional"
                        />
                      </>
                    )}
                    <button
                      className={styles.btnSecondary}
                      onClick={() => { 
                        onSetRelay(newRelay)
                        if (saveSettings) saveSettings({ unattended_password: unattendedPass })
                        setShowSettings(false) 
                      }}
                    >
                      Save Changes
                    </button>
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
                  value={agentID}
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
                className={`${styles.btnPrimary} ${connecting ? styles.btnLoading : ''}`}
                onClick={handleConnect}
                disabled={!isValid || connecting}
              >
                {connecting ? <><span className={styles.spinner} /> Connecting…</> : <><Wifi size={16} /> Connect</>}
              </button>
            </div>
          </div>
        </div>

        {/* BOTTOM PANEL: Devices */}
        <div className={styles.devicesSection}>
          <h3 className={styles.devicesTitle}>Recent Sessions</h3>
          {allDevices.length === 0 ? (
            <div className={styles.emptyState}>No recent sessions. Connect to a device to see it here.</div>
          ) : (
            <div className={styles.devicesGrid}>
              {allDevices.map((dev) => (
                <div 
                  key={dev.agent_id}
                  className={styles.deviceCard}
                  onClick={() => handleRecentClick(dev.agent_id)}
                >
                  <div className={styles.deviceIcon}>
                    {dev.os === 'darwin' ? '🍎' : dev.os === 'windows' ? '🪟' : '🐧'}
                  </div>
                  <div className={styles.deviceInfo}>
                    <h4>{dev.label}</h4>
                    <span>{formatID(dev.agent_id)}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

      </div>
    </div>
  )
}
