import React, { useRef, useEffect, useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { X, Maximize2, Minimize2, Activity, Wifi, Clock, Monitor, Upload, MessageSquare, Clipboard, Terminal, Power, Sparkles, Send, Zap, ChevronLeft } from 'lucide-react'
import { useInputCapture } from '../hooks/useInputCapture.js'
import styles from './SessionView.module.css'

/**
 * SessionView — the full-screen remote desktop viewer.
 *
 * Displays the WebRTC video stream in a <video> element and captures
 * all mouse/keyboard events, forwarding them to the host via Wails.
 */
export function SessionView({ stream, sessionInfo, displays, cursorInfo, connectionMode, qualityInfo, wolEnabled, sendWol, sendInput, sendClipboard, sendChat, sendFile, onDisconnect }) {
  const videoRef = useRef(null)
  const containerRef = useRef(null)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [toolbarVisible, setToolbarVisible] = useState(true)
  const hideTimerRef = useRef(null)
  const [aiPopupOpen, setAiPopupOpen] = useState(false)
  const [aiMessages, setAiMessages]   = useState([])
  const [aiInput, setAiInput]         = useState('')
  const [aiTyping, setAiTyping]       = useState(false)

  // Attach WebRTC stream to video element and play.
  useEffect(() => {
    if (videoRef.current && stream) {
      videoRef.current.srcObject = stream
      videoRef.current.muted = true
      const playPromise = videoRef.current.play()
      if (playPromise !== undefined) {
        playPromise.catch(err => console.warn('[session] autoplay blocked:', err))
      }
    }
  }, [stream])

  const showToolbar = () => {
    setToolbarVisible(true)
    clearTimeout(hideTimerRef.current)
    hideTimerRef.current = setTimeout(() => setToolbarVisible(false), 3000)
  }
  useEffect(() => {
    showToolbar()
    return () => clearTimeout(hideTimerRef.current)
  }, [])

  const toggleFullscreen = () => {
    if (!document.fullscreenElement) {
      containerRef.current?.requestFullscreen()
      setIsFullscreen(true)
    } else {
      document.exitFullscreen()
      setIsFullscreen(false)
    }
  }

  const MOCK_AI_RESPONSES = [
    "I can see the remote screen shows a desktop environment. Several application windows appear to be open.",
    "The Task Manager is visible showing performance metrics. CPU usage appears stable at the current load.",
    "The desktop shows normal activity. No unusual processes detected in the foreground applications.",
    "I observe a web browser is open on the remote machine. The browser appears to be responsive.",
  ]
  const sendAiMessage = () => {
    if (!aiInput.trim() || aiTyping) return
    const userMsg = { from: 'user', text: aiInput.trim() }
    setAiMessages(prev => [...prev, userMsg])
    const keyword = aiInput
    setAiInput('')
    setAiTyping(true)
    setTimeout(() => {
      setAiTyping(false)
      const text = '[' + (sessionInfo?.agent_id || 'Device') + '] ' + MOCK_AI_RESPONSES[Math.floor(Math.random() * MOCK_AI_RESPONSES.length)]
      setAiMessages(prev => [...prev, { from: 'ai', text }])
    }, 1500 + Math.random() * 600)
  }

  const { onMouseMove, onMouseDown, onMouseUp, onWheel, onContextMenu } =
    useInputCapture({ containerRef, videoRef, sendInput, enabled: true })

  const handleMouseMove = (e) => {
    showToolbar()
    onMouseMove(e)
  }

  const qualityColor = sessionInfo?.latency_ms < 50 ? 'var(--col-success)'
    : sessionInfo?.latency_ms < 150 ? 'var(--col-warning)'
    : 'var(--col-danger)'

  const connectionModeColor =
    connectionMode === 'direct' ? 'rgba(76,209,76,0.9)'
    : connectionMode === 'stun' ? 'rgba(255,200,0,0.9)'
    : connectionMode === 'relay' ? 'rgba(255,80,80,0.9)'
    : 'rgba(180,180,180,0.9)'
  const connectionModeText = connectionMode === 'direct' ? 'Direct'
    : connectionMode === 'stun' ? 'STUN'
    : connectionMode === 'relay' ? 'Relay'
    : 'Connecting'

  const elapsedSec = sessionInfo?.started_at
    ? Math.floor((Date.now() - new Date(sessionInfo.started_at).getTime()) / 1000)
    : 0
  const elapsed = `${String(Math.floor(elapsedSec / 60)).padStart(2,'0')}:${String(elapsedSec % 60).padStart(2,'0')}`

  const [isChatOpen, setIsChatOpen] = useState(false)
  const [chatMsg, setChatMsg] = useState('')
  const [messages, setMessages] = useState([])
  const chatEndRef = useRef(null)

  const [isLogsOpen, setIsLogsOpen] = useState(false)
  const [logs, setLogs] = useState([])
  const [levelFilter, setLevelFilter] = useState('all')
  const [sourceFilter, setSourceFilter] = useState('all')
  const logsEndRef = useRef(null)

  useEffect(() => {
    const h = (e) => setMessages(prev => [...prev, { from: 'Host', text: e.detail }])
    window.addEventListener('webrtc:chat_received', h)
    return () => window.removeEventListener('webrtc:chat_received', h)
  }, [])

  useEffect(() => {
    const h = (e) => setLogs(prev => [...prev, e.detail])
    window.addEventListener('webrtc:log_received', h)
    return () => window.removeEventListener('webrtc:log_received', h)
  }, [])

  useEffect(() => { chatEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages, isChatOpen])
  useEffect(() => { if (isLogsOpen) logsEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [logs, isLogsOpen])

  const handleSyncClipboard = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text) { sendClipboard(text); console.log('Clipboard synced to remote host') }
    } catch (e) { console.error('Clipboard sync failed', e) }
    sendInput({ type: 'get_clipboard' })
  }

  const handleSendChat = (e) => {
    e.preventDefault()
    if (!chatMsg.trim()) return
    sendChat(chatMsg)
    setMessages(prev => [...prev, { from: 'You', text: chatMsg }])
    setChatMsg('')
  }

  const fileInputRef = useRef(null)
  const [uploadProgress, setUploadProgress] = useState(0)
  const [isUploading, setIsUploading] = useState(false)

  const handleFileChange = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    setIsUploading(true)
    setUploadProgress(0)
    try {
      await sendFile(file, setUploadProgress)
      setTimeout(() => setIsUploading(false), 2000)
    } catch (err) { console.error(err); setIsUploading(false) }
  }

  return (
    <div
      ref={containerRef}
      className={styles.root}
      onMouseMove={handleMouseMove}
      onMouseDown={onMouseDown}
      onMouseUp={onMouseUp}
      onWheel={onWheel}
      onContextMenu={onContextMenu}
      tabIndex={0}
    >
      <video
        ref={videoRef}
        className={styles.video}
        autoPlay
        playsInline
        muted
        disablePictureInPicture
      />

      {/* Remote cursor overlay — positioned from agent's cursor_info via control channel */}
      {cursorInfo?.visible !== false && (
        <RemoteCursor containerRef={containerRef} videoRef={videoRef} cursorInfo={cursorInfo} />
      )}

      {!stream && (
        <div className={styles.placeholder}>
          <div className={styles.placeholderIcon}><Monitor size={48} strokeWidth={1} /></div>
          <p>Waiting for remote screen…</p>
          <div className={styles.placeholderDots}>
            {[0,1,2].map(i => <div key={i} className={styles.placeholderDot} style={{ animationDelay: `${i * 0.2}s` }} />)}
          </div>
        </div>
      )}

      <AnimatePresence>
        {toolbarVisible && (
          <motion.div className={styles.toolbar} initial={{ opacity: 0, y: -8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -8 }} transition={{ duration: 0.18 }}>
            <div className={styles.toolbarLeft}>
              <div className={styles.agentBadge}>
                <Monitor size={12} />
                <span className={styles.agentIDText}>{sessionInfo?.agent_id || '—'}</span>
              </div>
            </div>
            <div className={styles.toolbarCenter}>
              <StatBadge icon={<Activity size={11} />} label={sessionInfo?.latency_ms != null ? `${sessionInfo.latency_ms}ms` : '—'} color={qualityColor} />
              <StatBadge icon={<Wifi size={11} />} label={sessionInfo?.bitrate_kbps ? `${sessionInfo.bitrate_kbps} kbps` : '—'} color="rgba(255, 255, 255, 0.65)" />
              <StatBadge icon={<Clock size={11} />} label={elapsed} color="rgba(255, 255, 255, 0.65)" />
              <StatBadge
                icon={<div style={{width:8,height:8,borderRadius:'50%',background:connectionModeColor}} />}
                label={connectionModeText}
                color={connectionModeColor}
              />
              {sessionInfo?.fps != null && <StatBadge label={`${sessionInfo.fps} fps`} color="rgba(255, 255, 255, 0.65)" />}
            </div>
            <div className={styles.toolbarRight}>
              {wolEnabled && (
                <button className={styles.toolBtn} onClick={sendWol} title="Wake on LAN"
                  style={{ color: '#16a34a', borderColor: 'rgba(22,163,74,0.25)', background: 'rgba(22,163,74,0.08)' }}>
                  <Power size={14} />
                  <span>Wake</span>
                </button>
              )}
              {displays && displays.length > 1 && (
                <select className={styles.toolBtn} style={{ appearance: 'auto', background: 'transparent', color: 'white', border: 'none', outline: 'none' }}
                  onChange={(e) => sendInput({ type: 'switch_display', display_id: parseInt(e.target.value, 10) })}>
                  {displays.map((d, i) => <option key={d.id} value={d.id} style={{ color: 'black' }}>Display {i + 1} ({d.width}x{d.height})</option>)}
                </select>
              )}
              <input type="file" ref={fileInputRef} style={{ display: 'none' }} onChange={handleFileChange} />
              <button className={styles.toolBtn} onClick={() => fileInputRef.current?.click()} title="Send File">
                {isUploading ? <span>{Math.round(uploadProgress * 100)}%</span> : <span><Upload size={14} style={{ display: 'inline-block', verticalAlign: '-2px' }}/> Send File</span>}
              </button>
              <button className={styles.toolBtn} onClick={handleSyncClipboard} title="Sync Clipboard to Host">
                <Clipboard size={14} style={{ display: 'inline-block', verticalAlign: '-2px' }} /><span>Sync</span>
              </button>
              <button className={styles.toolBtn} onClick={() => setIsChatOpen(!isChatOpen)} title="Open Chat">
                <MessageSquare size={14} style={{ display: 'inline-block', verticalAlign: '-2px' }} /><span>Chat</span>
              </button>
              <button className={styles.toolBtn} onClick={() => setIsLogsOpen(!isLogsOpen)} title="Open Logs Monitor">
                <Terminal size={14} style={{ display: 'inline-block', verticalAlign: '-2px' }} /><span>Logs</span>
              </button>
              <button id="fullscreen-btn" className={styles.toolBtn} onClick={toggleFullscreen} title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
                {isFullscreen ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
              </button>
              <button className={styles.toolBtn} onClick={() => setAiPopupOpen(!aiPopupOpen)} title="AI Copilot"
                style={aiPopupOpen ? { color: '#4F8CFF', borderColor: 'rgba(79,140,255,0.35)', background: 'rgba(79,140,255,0.1)' } : {}}>
                <Sparkles size={14} style={{ display: 'inline-block', verticalAlign: '-2px' }} />
                <span>AI</span>
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <div style={{ position: 'absolute', top: 20, left: 20, zIndex: 1000 }}>
        <button id="disconnect-btn" className={styles.toolBtn} onClick={onDisconnect} title="Back to Dashboard" style={{ padding: '8px 16px', background: 'rgba(0,0,0,0.6)', color: '#fff', border: '1px solid rgba(255,255,255,0.2)', backdropFilter: 'blur(8px)' }}>
          <ChevronLeft size={16} /><span>Back to Dashboard</span>
        </button>
      </div>

      <AnimatePresence>
        {isChatOpen && (
          <motion.div className={styles.chatDrawer} initial={{ x: '100%' }} animate={{ x: 0 }} exit={{ x: '100%' }} transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            onMouseDown={e => e.stopPropagation()} onMouseMove={e => e.stopPropagation()}>
            <div className={styles.chatHeader}><h4>IT Support Chat</h4><button onClick={() => setIsChatOpen(false)}><X size={14}/></button></div>
            <div className={styles.chatBody}>
              <p className={styles.chatNotice}>Messages are sent directly to the host machine terminal.</p>
              <div className={styles.chatMsgList}>
                {messages.map((m, i) => <div key={i} className={`${styles.chatBubble} ${m.from === 'You' ? styles.chatBubbleSelf : styles.chatBubblePeer}`}><strong>{m.from}:</strong> {m.text}</div>)}
                <div ref={chatEndRef} />
              </div>
            </div>
            <form onSubmit={handleSendChat} className={styles.chatForm}>
              <input type="text" placeholder="Type a message..." value={chatMsg} onChange={e => setChatMsg(e.target.value)} className={styles.chatInput} />
              <button type="submit" className={styles.chatSendBtn}>Send</button>
            </form>
          </motion.div>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {isLogsOpen && (
          <motion.div className={styles.logsDrawer} initial={{ x: '100%' }} animate={{ x: 0 }} exit={{ x: '100%' }} transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            onMouseDown={e => e.stopPropagation()} onMouseMove={e => e.stopPropagation()}>
            <div className={styles.logsHeader}>
              <div className={styles.logsTitleArea}><Terminal size={14} style={{ marginRight: '4px' }} /><h4>Activity Logs Monitor</h4></div>
              <button onClick={() => setIsLogsOpen(false)}><X size={14}/></button>
            </div>
            <div className={styles.logsFilters}>
              <div className={styles.filterGroup}><label>Level:</label><select value={levelFilter} onChange={e => setLevelFilter(e.target.value)}><option value="all">All Levels</option><option value="info">Info</option><option value="warn">Warn</option><option value="error">Error</option></select></div>
              <div className={styles.filterGroup}><label>Source:</label><select value={sourceFilter} onChange={e => setSourceFilter(e.target.value)}><option value="all">All Sources</option><option value="Viewer">Viewer</option><option value="Neev Remote Agent">Neev Remote Agent</option></select></div>
              <button className={styles.clearBtn} onClick={() => setLogs([])}>Clear</button>
            </div>
            <div className={styles.logsBody}>
              <div className={styles.logsList}>
                {logs.filter(log => (levelFilter === 'all' || log.level?.toLowerCase() === levelFilter) && (sourceFilter === 'all' || log.source === sourceFilter)).map((log, i) => {
                  const levelClass = log.level === 'error' ? styles.logError : log.level === 'warn' ? styles.logWarn : styles.logInfo
                  const sourceClass = log.source === 'Viewer' ? styles.sourceViewer : styles.sourceAgent
                  return <div key={i} className={`${styles.logRow} ${levelClass}`}><span className={styles.logTime}>{log.time}</span><span className={`${styles.logSource} ${sourceClass}`}>{log.source}</span><span className={styles.logMessage}>{log.message}</span></div>
                })}
                <div ref={logsEndRef} />
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* AI Copilot Popup */}
      <AnimatePresence>
        {aiPopupOpen && (
          <motion.div
            initial={{ opacity: 0, scale: 0.95, y: 10 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 10 }}
            transition={{ duration: 0.18 }}
            onMouseDown={e => e.stopPropagation()}
            onMouseMove={e => e.stopPropagation()}
            style={{ position: 'fixed', bottom: 90, right: 24, width: 360, maxHeight: 480, zIndex: 1000, background: 'rgba(20,24,32,0.98)', border: '1px solid rgba(255,255,255,0.1)', borderRadius: 14, boxShadow: '0 20px 60px rgba(0,0,0,0.5)', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}
          >
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 14px', borderBottom: '1px solid rgba(255,255,255,0.08)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 7, fontSize: 13, fontWeight: 700, color: '#4F8CFF' }}>
                <Sparkles size={14} />
                <span>AI Copilot</span>
              </div>
              <button onClick={() => setAiPopupOpen(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'rgba(255,255,255,0.4)', padding: 4, borderRadius: 6, display: 'flex' }}>
                <X size={13} />
              </button>
            </div>
            <div style={{ flex: 1, overflowY: 'auto', padding: 12, display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 320 }}>
              {aiMessages.length === 0 && !aiTyping && (
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8, padding: '20px 10px', color: 'rgba(255,255,255,0.35)', textAlign: 'center' }}>
                  <Zap size={20} strokeWidth={1.5} />
                  <p style={{ fontSize: 12, margin: 0 }}>Ask anything about the remote screen</p>
                </div>
              )}
              {aiMessages.map((m, i) => (
                <div key={i} style={{ display: 'flex', justifyContent: m.from === 'user' ? 'flex-end' : 'flex-start' }}>
                  <div style={{ maxWidth: '85%', padding: '8px 12px', borderRadius: 12, fontSize: 12, lineHeight: 1.5, wordBreak: 'break-word', background: m.from === 'user' ? '#4F8CFF' : 'rgba(255,255,255,0.06)', color: m.from === 'user' ? '#fff' : 'rgba(255,255,255,0.85)', border: m.from === 'user' ? 'none' : '1px solid rgba(255,255,255,0.08)', borderBottomLeftRadius: m.from === 'ai' ? 4 : 12, borderBottomRightRadius: m.from === 'user' ? 4 : 12 }}>
                    {m.text}
                  </div>
                </div>
              ))}
              {aiTyping && (
                <div style={{ display: 'flex' }}>
                  <div style={{ padding: '8px 12px', borderRadius: 12, background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.08)', borderBottomLeftRadius: 4 }}>
                    <div style={{ display: 'flex', gap: 4 }}>
                      {[0, 0.2, 0.4].map((delay, idx) => (
                        <span key={idx} style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: 'rgba(255,255,255,0.4)', animation: 'typingBounce 1.2s ease infinite ' + delay + 's' }} />
                      ))}
                    </div>
                  </div>
                </div>
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 7, padding: '10px 12px', borderTop: '1px solid rgba(255,255,255,0.08)' }}>
              <input
                type="text" placeholder="Ask AI Copilot…" value={aiInput}
                onChange={e => setAiInput(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') sendAiMessage() }}
                style={{ flex: 1, background: 'none', border: 'none', outline: 'none', color: 'rgba(255,255,255,0.85)', fontSize: 13, fontFamily: 'inherit', height: 24 }}
              />
              <button onClick={sendAiMessage} disabled={!aiInput.trim() || aiTyping}
                style={{ width: 28, height: 28, borderRadius: 7, background: '#4F8CFF', color: '#fff', border: 'none', display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', opacity: (!aiInput.trim() || aiTyping) ? 0.35 : 1 }}>
                <Send size={13} />
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <div className={styles.qualityDot} style={{ background: qualityColor }} />
    </div>
  )
}

function StatBadge({ icon, label, color }) {
  return <div className={styles.statBadge} style={{ color }}>{icon}<span>{label}</span></div>
}

/**
 * RemoteCursor — renders the host's system cursor as an SVG overlay.
 * Position is computed in the same coordinate space as useInputCapture's
 * normalize() formula so the cursor hot-spot aligns with where the
 * remote cursor actually is on the remote host's screen.
 */
function RemoteCursor({ containerRef, videoRef, cursorInfo }) {
  const posRef = useRef({ x: 0.5, y: 0.5, cursorType: 0 })
  const dotRef = useRef(null)
  // Ring buffer for velocity prediction: last 6 positions with timestamps
  const histRef = useRef([])
  const lastUpdateRef = useRef(0)

  // Update position and cursor type via direct DOM manipulation on cursorInfo changes
  useEffect(() => {
    if (!cursorInfo) return
    const now = performance.now()
    const entry = { x: cursorInfo.x, y: cursorInfo.y, t: now, cursorType: cursorInfo.cursorType ?? 0 }
    posRef.current = { x: entry.x, y: entry.y, cursorType: entry.cursorType }
    // Push to ring buffer, keep last 6
    const hist = histRef.current
    hist.push(entry)
    if (hist.length > 6) hist.shift()
    lastUpdateRef.current = now
  }, [cursorInfo])

  // RAF loop for smooth 60fps cursor tracking with velocity prediction
  useEffect(() => {
    let raf
    let lastTime = performance.now()
    const update = () => {
      const el = containerRef.current
      const video = videoRef?.current
      const dot = dotRef.current
      if (!el || !dot || !video || !video.videoWidth) {
        raf = requestAnimationFrame(update)
        return
      }
      const now = performance.now()
      const dt = (now - lastTime) / 1000 // seconds since last frame
      lastTime = now

      const hist = histRef.current
      let px = posRef.current.x
      let py = posRef.current.y

      // Linear velocity prediction if we have at least 2 history samples
      if (hist.length >= 2) {
        const n = hist.length
        const p0 = hist[n - 2]
        const p1 = hist[n - 1]
        const dtHist = (p1.t - p0.t) / 1000
        if (dtHist > 0) {
          const vx = (p1.x - p0.x) / dtHist // pixels per second (remote coords)
          const vy = (p1.y - p0.y) / dtHist
          // Predict: pos += velocity * elapsed since last update
          const elapsed = (now - lastUpdateRef.current) / 1000
          px = p1.x + vx * Math.min(elapsed, 0.1) // cap at 100ms prediction
          py = p1.y + vy * Math.min(elapsed, 0.1)
        }
      }

      const rect = el.getBoundingClientRect()
      const vW = video.videoWidth
      const vH = video.videoHeight
      const scale = Math.min(rect.width / vW, rect.height / vH)
      const renderedW = vW * scale
      const renderedH = vH * scale
      const offsetX = (rect.width - renderedW) / 2
      const offsetY = (rect.height - renderedH) / 2
      // Convert normalized remote coords to pixel position within video
      const screenX = offsetX + px / vW * renderedW - 7
      const screenY = offsetY + py / vH * renderedH - 7
      dot.style.transform = `translate(${screenX}px, ${screenY}px)`
      // Swap SVG cursor shape based on cursorType
      dot.setAttribute('data-type', String(posRef.current.cursorType))
      raf = requestAnimationFrame(update)
    }
    raf = requestAnimationFrame(update)
    return () => cancelAnimationFrame(raf)
  }, [containerRef, videoRef])

  if (!cursorInfo || cursorInfo.visible === false) return null

  // Cursor type: 0=arrow, 1=ibeam, 2=cross, 3=wait/hourglass, 4=resize, 5=hand
  const t = cursorInfo.cursorType ?? 0
  const size = t === 1 ? '16x24' : t === 2 || t === 3 ? '20x20' : '14x20'

  return (
    <div ref={dotRef} data-type={String(t)} style={{
      position: 'absolute', top: 0, left: 0,
      width: 14, height: 20, pointerEvents: 'none', zIndex: 50,
      willChange: 'transform', transition: 'none',
    }}>
      {t === 0 && (
        <svg width="14" height="20" viewBox="0 0 14 20" fill="none">
          <path d="M1 1L1 16.5L4.5 12.5L7.5 18.5L9.5 17.5L6.5 11.5L11.5 11.5L1 1Z"
            fill="white" stroke="rgba(0,0,0,0.65)" strokeWidth="1.2" strokeLinejoin="round" />
        </svg>
      )}
      {t === 1 && (
        <svg width="16" height="24" viewBox="0 0 16 24" fill="none">
          <line x1="8" y1="0" x2="8" y2="24" stroke="rgba(0,0,0,0.7)" strokeWidth="1.5" />
          <line x1="8" y1="0" x2="8" y2="24" stroke="white" strokeWidth="0.8" />
        </svg>
      )}
      {t === 2 && (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <line x1="10" y1="2" x2="10" y2="18" stroke="rgba(0,0,0,0.7)" strokeWidth="1.5" />
          <line x1="2" y1="10" x2="18" y2="10" stroke="rgba(0,0,0,0.7)" strokeWidth="1.5" />
          <line x1="10" y1="2" x2="10" y2="18" stroke="white" strokeWidth="0.8" />
          <line x1="2" y1="10" x2="18" y2="10" stroke="white" strokeWidth="0.8" />
        </svg>
      )}
      {t === 3 && (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <circle cx="10" cy="10" r="8" fill="white" stroke="rgba(0,0,0,0.7)" strokeWidth="1.5" />
          <line x1="4" y1="4" x2="16" y2="16" stroke="rgba(0,0,0,0.7)" strokeWidth="2" />
        </svg>
      )}
      {t === 4 && (
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <line x1="10" y1="1" x2="10" y2="19" stroke="rgba(0,0,0,0.7)" strokeWidth="1.5" />
          <line x1="1" y1="10" x2="19" y2="10" stroke="rgba(0,0,0,0.7)" strokeWidth="1.5" />
          <line x1="10" y1="1" x2="10" y2="19" stroke="white" strokeWidth="0.8" />
          <line x1="1" y1="10" x2="19" y2="10" stroke="white" strokeWidth="0.8" />
        </svg>
      )}
      {t === 5 && (
        <svg width="18" height="22" viewBox="0 0 18 22" fill="none">
          <path d="M2 2C2 2 2 12 9 12C9 12 2 12 2 18L2 20L16 20L16 2L2 2Z"
            fill="white" stroke="rgba(0,0,0,0.7)" strokeWidth="1.2" strokeLinejoin="round" />
          <path d="M2 2L9 12L16 2" fill="rgba(0,0,0,0.15)" />
        </svg>
      )}
      {t >= 6 && (
        <svg width="14" height="20" viewBox="0 0 14 20" fill="none">
          <path d="M1 1L1 16.5L4.5 12.5L7.5 18.5L9.5 17.5L6.5 11.5L11.5 11.5L1 1Z"
            fill="white" stroke="rgba(0,0,0,0.65)" strokeWidth="1.2" strokeLinejoin="round" />
        </svg>
      )}
    </div>
  )
}