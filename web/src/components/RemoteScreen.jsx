import React, { useRef, useEffect, useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { X, Maximize2, Minimize2, Activity, Wifi, Monitor } from 'lucide-react'
import styles from './RemoteScreen.module.css'

/**
 * RemoteScreen — renders the live WebRTC video stream and captures
 * all mouse/keyboard input, forwarding via sendInput(event).
 *
 * Identical in spirit to the desktop client's SessionView but lives
 * in the browser and auto-hides the overlay toolbar.
 */
export function RemoteScreen({ stream, sendInput, onDisconnect, sessionInfo }) {
  const videoRef     = useRef(null)
  const containerRef = useRef(null)
  const [toolbar, setToolbar]     = useState(true)
  const [fullscreen, setFullscreen] = useState(false)
  const hideTimer = useRef(null)

  // Attach stream.
  useEffect(() => {
    if (videoRef.current && stream) videoRef.current.srcObject = stream
  }, [stream])

  // Auto-hide toolbar.
  const showToolbar = () => {
    setToolbar(true)
    clearTimeout(hideTimer.current)
    hideTimer.current = setTimeout(() => setToolbar(false), 3000)
  }
  useEffect(() => { showToolbar(); return () => clearTimeout(hideTimer.current) }, [])

  // Fullscreen.
  const toggleFS = () => {
    if (!document.fullscreenElement) { containerRef.current?.requestFullscreen(); setFullscreen(true) }
    else { document.exitFullscreen(); setFullscreen(false) }
  }

  // Normalize mouse coordinates to 0–1 relative to the actual video frame.
  const norm = (e) => {
    const video = videoRef.current
    const container = containerRef.current
    if (!video || !video.videoWidth) {
      const rect = container?.getBoundingClientRect()
      if (!rect) return { x: 0, y: 0 }
      return {
        x: Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)),
        y: Math.max(0, Math.min(1, (e.clientY - rect.top)  / rect.height)),
      }
    }
    
    const rect = video.getBoundingClientRect()
    const videoRatio = video.videoWidth / video.videoHeight
    const boxRatio = rect.width / rect.height
    
    let drawWidth = rect.width
    let drawHeight = rect.height
    let drawX = rect.left
    let drawY = rect.top
    
    if (videoRatio > boxRatio) {
      drawHeight = rect.width / videoRatio
      drawY = rect.top + (rect.height - drawHeight) / 2
    } else {
      drawWidth = rect.height * videoRatio
      drawX = rect.left + (rect.width - drawWidth) / 2
    }
    
    return {
      x: Math.max(0, Math.min(1, (e.clientX - drawX) / drawWidth)),
      y: Math.max(0, Math.min(1, (e.clientY - drawY) / drawHeight))
    }
  }

  const lastMoveRef = useRef(0)
  const lastEventRef = useRef({ time: 0, type: '', x: 0, y: 0 })
  
  const checkLoop = (type, x, y) => {
    const now = Date.now()
    
    if (type === 'move') {
      if (now - lastMoveRef.current < 25) return true // Throttle to ~40fps
      lastMoveRef.current = now
      return false
    }

    const last = lastEventRef.current
    if (last.type === type && last.x === x && last.y === y && (now - last.time) < 50) return true
    lastEventRef.current = { time: now, type, x, y }
    return false
  }

  const onMouseMove  = (e) => { showToolbar(); if (!sendInput) return; const coords = norm(e); if (checkLoop('move', coords.x, coords.y)) return; sendInput({ type: 'mouse_move', ...coords }) }
  const onMouseDown  = (e) => { e.preventDefault(); if (!sendInput) return; const coords = norm(e); if (checkLoop('down', coords.x, coords.y)) return; sendInput({ type: 'mouse_down', ...coords, button: e.button }) }
  const onMouseUp    = (e) => { if (!sendInput) return; const coords = norm(e); if (checkLoop('up', coords.x, coords.y)) return; sendInput({ type: 'mouse_up', ...coords, button: e.button }) }
  const onWheel      = (e) => { e.preventDefault(); if (sendInput) sendInput({ type: 'mouse_scroll', dx: e.deltaX, dy: e.deltaY }) }
  const onCtxMenu    = (e) => e.preventDefault()

  // Keyboard (global).
  useEffect(() => {
    const down = (e) => { e.preventDefault(); sendInput?.({ type:'key_down', key_code:e.keyCode, code:e.code, char:e.key.length===1?e.key:'', modifiers:(e.shiftKey?1:0)|(e.ctrlKey?2:0)|(e.altKey?4:0)|(e.metaKey?8:0) }) }
    const up   = (e) => { e.preventDefault(); sendInput?.({ type:'key_up', key_code:e.keyCode, code:e.code }) }
    window.addEventListener('keydown', down)
    window.addEventListener('keyup',   up)
    return () => { window.removeEventListener('keydown', down); window.removeEventListener('keyup', up) }
  }, [sendInput])

  const latColor = !sessionInfo?.latency_ms ? 'var(--text-dim)'
    : sessionInfo.latency_ms < 50 ? 'var(--success)'
    : sessionInfo.latency_ms < 150 ? 'var(--warning)'
    : 'var(--danger)'

  return (
    <div
      ref={containerRef}
      className={styles.root}
      onMouseMove={onMouseMove}
      onMouseDown={onMouseDown}
      onMouseUp={onMouseUp}
      onWheel={onWheel}
      onContextMenu={onCtxMenu}
      tabIndex={0}
    >
      <video ref={videoRef} className={styles.video} autoPlay playsInline muted />

      {!stream && (
        <div className={styles.waiting}>
          <Monitor size={40} strokeWidth={1} style={{ color: 'rgba(255,255,255,.2)' }} />
          <p>Waiting for stream…</p>
        </div>
      )}

      {/* Toolbar */}
      <AnimatePresence>
        {toolbar && (
          <motion.div className={styles.toolbar}
            initial={{ opacity:0, y:-8 }} animate={{ opacity:1, y:0 }}
            exit={{ opacity:0, y:-8 }} transition={{ duration:.18 }}>
            <div className={styles.tbLeft}>
              <div className={styles.agentBadge}>
                <Monitor size={12} />
                <span>{sessionInfo?.agentID || 'Remote'}</span>
              </div>
            </div>
            <div className={styles.tbCenter}>
              {sessionInfo?.latency_ms != null && (
                <span className={styles.stat} style={{ color: latColor }}>
                  <Activity size={11} /> {sessionInfo.latency_ms}ms
                </span>
              )}
              {sessionInfo?.fps && (
                <span className={styles.stat}><Wifi size={11} /> {sessionInfo.fps}fps</span>
              )}
            </div>
            <div className={styles.tbRight}>
              <button className={styles.tbBtn} onClick={toggleFS} title="Fullscreen">
                {fullscreen ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
              </button>
              <button className={`${styles.tbBtn} ${styles.tbBtnDanger}`} onClick={onDisconnect}>
                <X size={13} /> Disconnect
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Quality dot */}
      <div className={styles.qualityDot} style={{ background: latColor }} />
    </div>
  )
}
