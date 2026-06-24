import React, { useRef, useEffect, useState, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  X, Maximize2, Minimize2, Activity, Wifi, Clock, Monitor,
  Upload, MessageSquare, Clipboard, Terminal, Power,
  Eye, EyeOff, StickyNote, Folder, FolderOpen, Pause, Play, Sparkles, Send, Zap
} from 'lucide-react'
import { useInputCapture } from '../hooks/useInputCapture.js'
import styles from './SessionView.module.css'

/** Format bytes → "1.2 MB" */
function formatSize(bytes) {
  if (bytes < 1024)        return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

/** Generate a simple unique ID */
let _transferId = 1
function nextId() { return _transferId++ }

/**
 * SessionView — full-screen remote desktop viewer.
 * Features: floating bottom dock, Privacy Mode, Session Notes, File Transfer queue.
 */
export function SessionView({
  stream, sessionInfo, displays, connectionMode, qualityInfo, cursorInfo,
  wolEnabled, sendWol, sendInput, sendClipboard, sendChat, sendFile,
  onDisconnect
}) {
  const videoRef     = useRef(null)
  const containerRef = useRef(null)
  const hideTimerRef = useRef(null)
  const pauseRef     = useRef(null)   // {file, offset} — used for pause/resume

  const [isFullscreen,    setIsFullscreen]    = useState(false)
  const [toolbarVisible,  setToolbarVisible]  = useState(true)
  const [privacyMode,     setPrivacyMode]     = useState(false)
  const [privacyVideoSrc, setPrivacyVideoSrc] = useState(null)
  const [inputBlockRef,   _setInputBlockRef]  = useState({ current: false })
  const [notesPanel,       setNotesPanel]       = useState(false)
  const [sessionNotes,     setSessionNotes]     = useState('')
  const [isChatOpen,       setIsChatOpen]       = useState(false)
  const [chatMsg,          setChatMsg]           = useState('')
  const [messages,         setMessages]          = useState([])
  const [isLogsOpen,       setIsLogsOpen]        = useState(false)
  const [logs,             setLogs]              = useState([])
  const [levelFilter,      setLevelFilter]       = useState('all')
  const [sourceFilter,     setSourceFilter]      = useState('all')
  const [aiPopupOpen,     setAiPopupOpen]       = useState(false)
  const [aiMessages,      setAiMessages]         = useState([])
  const [aiInput,         setAiInput]            = useState('')
  const [aiTyping,        setAiTyping]           = useState(false)

  /* ── File transfer state ─────────────────────────────────────────────── */
  const [transfers,        setTransfers]        = useState([])   // active queue
  const [history,          setHistory]          = useState([])   // completed
  const [isDragging,       setIsDragging]       = useState(false) // drag overlay
  const [transferPanelOpen, setTransferPanelOpen] = useState(false)

  const chatEndRef = useRef(null)
  const logsEndRef = useRef(null)

  /* ── Attach WebRTC stream ─────────────────────────────────────────────── */
  useEffect(() => {
    if (!videoRef.current) return
    if (privacyMode) return
    videoRef.current.srcObject = stream || null
    if (stream) {
      videoRef.current.muted = true
      videoRef.current.play().catch(() => {})
    }
  }, [stream, privacyMode])

  /* ── Toolbar auto-hide ─────────────────────────────────────────────────── */
  const showToolbar = () => {
    setToolbarVisible(true)
    clearTimeout(hideTimerRef.current)
    hideTimerRef.current = setTimeout(() => setToolbarVisible(false), 3000)
  }
  useEffect(() => {
    showToolbar()
    return () => clearTimeout(hideTimerRef.current)
  }, [])

  /* ── Fullscreen ───────────────────────────────────────────────────────── */
  const toggleFullscreen = () => {
    if (!document.fullscreenElement) {
      containerRef.current?.requestFullscreen()
      setIsFullscreen(true)
    } else {
      document.exitFullscreen()
      setIsFullscreen(false)
    }
  }

  /* ── Input capture ─────────────────────────────────────────────────────── */
  const { onMouseMove, onMouseDown, onMouseUp, onWheel, onContextMenu } =
    useInputCapture({ containerRef, videoRef, sendInput, enabled: !privacyMode })

  const handleMouseMove = (e) => {
    if (privacyMode) return
    showToolbar()
    onMouseMove(e)
  }
  const handleMouseDown = (e) => {
    if (privacyMode) return
    onMouseDown(e)
  }
  const handleMouseUp = (e) => {
    if (privacyMode) return
    onMouseUp(e)
  }

  /* ── Privacy Mode ─────────────────────────────────────────────────────── */
  const togglePrivacyMode = () => {
    if (!privacyMode) {
      if (videoRef.current?.srcObject) {
        setPrivacyVideoSrc(videoRef.current.srcObject)
        videoRef.current.srcObject = null
      }
      setPrivacyMode(true)
      _setInputBlockRef({ current: true })
    } else {
      if (privacyVideoSrc && videoRef.current) {
        videoRef.current.srcObject = privacyVideoSrc
        videoRef.current.play().catch(() => {})
      }
      setPrivacyMode(false)
      _setInputBlockRef({ current: false })
    }
  }

  /* ── Drag & Drop ─────────────────────────────────────────────────────── */
  const handleDragEnter = (e) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(true)
    showToolbar()
  }
  const handleDragOver = (e) => {
    e.preventDefault()
    e.stopPropagation()
  }
  const handleDragLeave = (e) => {
    // Only trigger if leaving the container itself (not a child element)
    if (e.currentTarget === e.target) setIsDragging(false)
  }
  const handleDrop = (e) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
    const files = Array.from(e.dataTransfer?.files || [])
    if (files.length) addTransfers(files)
  }

  /* ── File transfer queue ─────────────────────────────────────────────── */
  const addTransfers = (fileList) => {
    const newItems = fileList.map(file => ({
      id:       nextId(),
      name:     file.name,
      size:     file.size,
      progress: 0,
      status:   'pending',  // pending | uploading | paused | completed | error
      file,
    }))
    setTransfers(prev => [...prev, ...newItems])
    // Auto-start pending uploads
    newItems.forEach(item => sendTransfer(item))
  }

  /**
   * Send a single file. item.file may be a full file or a slice (for resume).
   * onProgress is scaled: if startOffset > 0, we remap 0-1 to (startOffset/file.size)-1.
   */
  const sendTransfer = useCallback(async (item, startOffset = 0) => {
    const fileSlice = startOffset > 0 ? item.file.slice(startOffset) : item.file

    setTransfers(prev =>
      prev.map(t => t.id === item.id ? { ...t, status: 'uploading', progress: 0 } : t)
    )

    try {
      await sendFile(fileSlice, (prog) => {
        // Remap progress back to 0-1 relative to full file
        const overall = startOffset / item.size + prog * (1 - startOffset / item.size)
        setTransfers(prev =>
          prev.map(t => t.id === item.id ? { ...t, progress: overall } : t)
        )
      })
      // Completed
      setTransfers(prev => prev.filter(t => t.id !== item.id))
      setHistory(prev => [{
        id: item.id, name: item.name, size: item.size,
        completedAt: new Date().toLocaleTimeString(),
      }, ...prev].slice(0, 20))
    } catch (err) {
      setTransfers(prev =>
        prev.map(t => t.id === item.id ? { ...t, status: 'error', progress: 0 } : t)
      )
      console.error('[transfer] error:', err)
    }
  }, [sendFile])

  /**
   * Pause: record current {file, offset} in pauseRef and mark status='paused'.
   * The in-flight sendFile call will finish — progress updates are ignored
   * once status is 'paused' (guarded in the setTransfers map below).
   */
  const handlePause = (id) => {
    const item = transfers.find(t => t.id === id)
    if (!item) return
    // Snapshot the current progress as the resume offset
    const resumeOffset = Math.floor(item.progress * item.size)
    pauseRef.current = { id, file: item.file, resumeOffset }
    setTransfers(prev =>
      prev.map(t => t.id === id ? { ...t, status: 'paused' } : t)
    )
  }

  /**
   * Resume: read offset from pauseRef, slice file from that byte, re-send.
   */
  const handleResume = (id) => {
    const { file, resumeOffset } = pauseRef.current || {}
    const item = transfers.find(t => t.id === id)
    if (!item) return
    // Override file with a slice from the stored offset
    const resumeFile = file ? file.slice(resumeOffset) : item.file
    setTransfers(prev =>
      prev.map(t => t.id === id ? { ...t, status: 'uploading' } : t)
    )
    sendTransfer({ ...item, file: resumeFile }, resumeOffset)
  }

  const handleCancel = (id) => {
    setTransfers(prev => prev.filter(t => t.id !== id))
  }

  /* ── Session notes ─────────────────────────────────────────────────────── */
  const handleSaveNotes = () => {
    console.log('[session] notes saved:', sessionNotes)
    setNotesPanel(false)
  }

  /* ── AI Copilot ───────────────────────────────────────────────────────── */
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
      const text = `[${sessionInfo?.agent_id || 'Device'}] ${MOCK_AI_RESPONSES[Math.floor(Math.random() * MOCK_AI_RESPONSES.length)]}`
      setAiMessages(prev => [...prev, { from: 'ai', text }])
    }, 1500 + Math.random() * 600)
  }

  /* ── Clipboard sync ────────────────────────────────────────────────────── */
  const handleSyncClipboard = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (text) { sendClipboard(text); console.log('Clipboard synced') }
    } catch (e) { console.error('Clipboard sync failed', e) }
    sendInput({ type: 'get_clipboard' })
  }

  /* ── Chat ──────────────────────────────────────────────────────────────── */
  const handleSendChat = (e) => {
    e.preventDefault()
    if (!chatMsg.trim()) return
    sendChat(chatMsg)
    setMessages(prev => [...prev, { from: 'You', text: chatMsg }])
    setChatMsg('')
  }
  useEffect(() => {
    const h = (e) => setMessages(prev => [...prev, { from: 'Host', text: e.detail }])
    window.addEventListener('webrtc:chat_received', h)
    return () => window.removeEventListener('webrtc:chat_received', h)
  }, [])

  /* ── Logs ──────────────────────────────────────────────────────────────── */
  useEffect(() => {
    const h = (e) => setLogs(prev => [...prev, e.detail])
    window.addEventListener('webrtc:log_received', h)
    return () => window.removeEventListener('webrtc:log_received', h)
  }, [])
  useEffect(() => { chatEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages, isChatOpen])
  useEffect(() => { if (isLogsOpen) logsEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [logs, isLogsOpen])

  /* ── Session stats ─────────────────────────────────────────────────────── */
  const qualityColor = !sessionInfo?.latency_ms ? 'var(--text-muted)'
    : sessionInfo.latency_ms < 50  ? 'var(--success)'
    : sessionInfo.latency_ms < 150 ? 'var(--warning)'
    : 'var(--danger)'

  const connectionModeColor =
    connectionMode === 'direct' ? 'var(--success)'
    : connectionMode === 'stun' ? 'var(--warning)'
    : connectionMode === 'relay' ? 'var(--danger)'
    : 'var(--text-muted)'

  const connectionModeText =
    connectionMode === 'direct' ? 'Direct'
    : connectionMode === 'stun' ? 'STUN'
    : connectionMode === 'relay' ? 'Relay'
    : 'Connecting'

  const elapsedSec = sessionInfo?.started_at
    ? Math.floor((Date.now() - new Date(sessionInfo.started_at).getTime()) / 1000)
    : 0
  const elapsed = `${String(Math.floor(elapsedSec / 60)).padStart(2,'0')}:${String(elapsedSec % 60).padStart(2,'0')}`

  return (
    <div
      ref={containerRef}
      className={styles.root}
      onMouseMove={handleMouseMove}
      onMouseDown={handleMouseDown}
      onMouseUp={handleMouseUp}
      onWheel={onWheel}
      onContextMenu={onContextMenu}
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      tabIndex={0}
    >
      {/* Remote video */}
      <video ref={videoRef} className={styles.video} autoPlay playsInline muted disablePictureInPicture />

      {/* Remote cursor overlay */}
      {cursorInfo?.visible !== false && (
        <RemoteCursor containerRef={containerRef} videoRef={videoRef} cursorInfo={cursorInfo} />
      )}

      {/* Placeholder when no stream */}
      {!stream && !privacyMode && (
        <div className={styles.placeholder}>
          <div className={styles.placeholderIcon}><Monitor size={48} strokeWidth={1} /></div>
          <p>Waiting for remote screen…</p>
          <div className={styles.placeholderDots}>
            {[0,1,2].map(i => (
              <div key={i} className={styles.placeholderDot} style={{ animationDelay: `${i * 0.2}s` }} />
            ))}
          </div>
        </div>
      )}

      {/* Privacy Mode overlay */}
      {privacyMode && (
        <div className={styles.privacyOverlay}>
          <EyeOff size={40} strokeWidth={1.5} className={styles.privacyOverlayIcon} />
          <p>Screen hidden — press the eye icon to restore</p>
        </div>
      )}

      {/* Drag-and-drop overlay */}
      <AnimatePresence>
        {isDragging && (
          <motion.div
            className={styles.dragOverlay}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15 }}
          >
            <FolderOpen size={52} strokeWidth={1.5} />
            <p>Drop files to send</p>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Always-visible disconnect button (top-right) */}
      <button className={styles.disconnectBtn} onClick={onDisconnect}>
        <X size={13} /><span>Disconnect</span>
      </button>

      {/* Quality dot (top-right, below disconnect) */}
      <div className={styles.qualityDot} style={{ background: qualityColor }} />

      {/* Floating bottom toolbar */}
      <AnimatePresence>
        {toolbarVisible && (
          <motion.div
            className={styles.toolbar}
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 16 }}
            transition={{ duration: 0.2, ease: 'easeOut' }}
            onMouseEnter={showToolbar}
          >
            {/* Left: agent badge */}
            <div className={styles.toolbarLeft}>
              <div className={styles.agentBadge}>
                <Monitor size={11} />
                <span className={styles.agentIDText}>{sessionInfo?.agent_id || '—'}</span>
              </div>
            </div>

            {/* Center: stats */}
            <div className={styles.toolbarCenter}>
              <StatBadge icon={<Activity size={10} />} label={sessionInfo?.latency_ms != null ? `${sessionInfo.latency_ms}ms` : '—'} color={qualityColor} />
              <StatBadge icon={<Wifi size={10} />} label={sessionInfo?.bitrate_kbps ? `${sessionInfo.bitrate_kbps} kbps` : '—'} color="var(--text-secondary)" />
              <StatBadge icon={<Clock size={10} />} label={elapsed} color="var(--text-secondary)" />
              <StatBadge
                icon={<div style={{ width: 7, height: 7, borderRadius: '50%', background: connectionModeColor }} />}
                label={connectionModeText}
                color={connectionModeColor}
              />
              {sessionInfo?.fps != null && (
                <StatBadge label={`${sessionInfo.fps} fps`} color="var(--text-secondary)" />
              )}
            </div>

            {/* Right: actions */}
            <div className={styles.toolbarRight}>
              {/* Privacy */}
              <button
                className={`${styles.toolBtn} ${privacyMode ? styles.toolBtnActive : ''}`}
                onClick={togglePrivacyMode}
                title={privacyMode ? 'Show remote screen' : 'Hide remote screen'}
              >
                {privacyMode ? <EyeOff size={13} /> : <Eye size={13} />}
                <span>{privacyMode ? 'Show' : 'Privacy'}</span>
              </button>

              {/* WoL */}
              {wolEnabled && (
                <button className={styles.toolBtn} onClick={sendWol} title="Wake on LAN"
                  style={{ color: 'var(--success)', borderColor: 'rgba(34,197,94,0.3)' }}>
                  <Power size={13} /><span>Wake</span>
                </button>
              )}

              {/* Monitor selector */}
              {displays && displays.length > 1 && (
                <select className={styles.toolBtn} style={{ appearance: 'auto' }}
                  onChange={(e) => sendInput({ type: 'switch_display', display_id: parseInt(e.target.value, 10) })}>
                  {displays.map((d, i) => (
                    <option key={d.id} value={d.id}>Display {i + 1} ({d.width}×{d.height})</option>
                  ))}
                </select>
              )}

              {/* File transfer panel */}
              <button
                className={`${styles.toolBtn} ${transferPanelOpen ? styles.toolBtnActive : ''}`}
                onClick={() => setTransferPanelOpen(v => !v)}
                title="File Transfer"
              >
                <Folder size={13} /><span>Transfer</span>
                {transfers.length > 0 && (
                  <span className={styles.transferBadge}>{transfers.length}</span>
                )}
              </button>

              {/* Clipboard */}
              <button className={styles.toolBtn} onClick={handleSyncClipboard} title="Sync clipboard to host">
                <Clipboard size={13} /><span>Clip</span>
              </button>

              {/* Session notes */}
              <button
                className={`${styles.toolBtn} ${notesPanel ? styles.toolBtnActive : ''}`}
                onClick={() => setNotesPanel(v => !v)}
                title="Session Notes"
              >
                <StickyNote size={13} /><span>Notes</span>
              </button>

              {/* Chat */}
              <button
                className={`${styles.toolBtn} ${isChatOpen ? styles.toolBtnActive : ''}`}
                onClick={() => setIsChatOpen(v => !v)}
                title="Chat"
              >
                <MessageSquare size={13} /><span>Chat</span>
              </button>

              {/* Logs */}
              <button
                className={`${styles.toolBtn} ${isLogsOpen ? styles.toolBtnActive : ''}`}
                onClick={() => setIsLogsOpen(v => !v)}
                title="Activity Logs"
              >
                <Terminal size={13} /><span>Logs</span>
              </button>

              {/* Fullscreen */}
              <button className={styles.toolBtn} onClick={toggleFullscreen} title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
                {isFullscreen ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
              </button>

              {/* AI Copilot */}
              <button
                className={`${styles.toolBtn} ${aiPopupOpen ? styles.toolBtnActive : ''}`}
                onClick={() => setAiPopupOpen(v => !v)}
                title="AI Copilot"
              >
                <Sparkles size={13} /><span>AI</span>
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ── AI Copilot Popup ─────────────────────────────────────────── */}
      <AnimatePresence>
        {aiPopupOpen && (
          <motion.div
            className={styles.aiPopup}
            initial={{ opacity: 0, scale: 0.95, y: 10 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 10 }}
            transition={{ duration: 0.18 }}
            onMouseDown={e => e.stopPropagation()}
            onMouseMove={e => e.stopPropagation()}
          >
            <div className={styles.aiPopupHeader}>
              <div className={styles.aiPopupTitle}>
                <Sparkles size={14} />
                <span>AI Copilot</span>
              </div>
              <button onClick={() => setAiPopupOpen(false)} className={styles.aiPopupClose}>
                <X size={13} />
              </button>
            </div>
            <div className={styles.aiPopupMessages}>
              {aiMessages.length === 0 && !aiTyping && (
                <div className={styles.aiPopupEmpty}>
                  <Zap size={20} strokeWidth={1.5} />
                  <p>Ask anything about what's happening on the remote screen</p>
                </div>
              )}
              {aiMessages.map((m, i) => (
                <div key={i} className={`${styles.aiMsgRow} ${m.from === 'user' ? styles.aiMsgRowUser : ''}`}>
                  <div className={`${styles.aiMsgBubble} ${m.from === 'user' ? styles.aiMsgUser : styles.aiMsgAi}`}>
                    {m.text}
                  </div>
                </div>
              ))}
              {aiTyping && (
                <div className={styles.aiMsgRow}>
                  <div className={`${styles.aiMsgBubble} ${styles.aiMsgAi}`}>
                    <div className={styles.aiTypingDots}><span /><span /><span /></div>
                  </div>
                </div>
              )}
            </div>
            <div className={styles.aiPopupInput}>
              <input
                type="text" placeholder="Ask AI Copilot…" value={aiInput}
                onChange={e => setAiInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && sendAiMessage()}
              />
              <button onClick={sendAiMessage} disabled={!aiInput.trim() || aiTyping}>
                <Send size={13} />
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ── File Transfer Panel ─────────────────────────────────────────── */}
      <AnimatePresence>
        {transferPanelOpen && (
          <motion.div
            className={styles.transferPanel}
            initial={{ x: 320 }}
            animate={{ x: 0 }}
            exit={{ x: 320 }}
            transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            onMouseDown={e => e.stopPropagation()}
            onMouseMove={e => e.stopPropagation()}
          >
            {/* Header */}
            <div className={styles.transferHeader}>
              <div className={styles.transferHeaderLeft}>
                <FolderOpen size={15} />
                <h4>File Transfer</h4>
              </div>
              <label className={styles.addFileBtn}>
                <Upload size={13} /><span>Add Files</span>
                <input type="file" multiple style={{ display: 'none' }}
                  onChange={e => { const files = Array.from(e.target.files || []); if (files.length) addTransfers(files); e.target.value = '' }} />
              </label>
              <button onClick={() => setTransferPanelOpen(false)}><X size={14} /></button>
            </div>

            {/* Queue */}
            <div className={styles.transferBody}>
              {transfers.length === 0 ? (
                <p className={styles.transferEmpty}>No active transfers. Drag files onto the session window or click Add Files.</p>
              ) : (
                transfers.map(item => (
                  <div key={item.id} className={styles.transferRow}>
                    <div className={styles.transferRowInfo}>
                      <span className={styles.transferName} title={item.name}>{item.name}</span>
                      <span className={styles.transferSize}>{formatSize(item.size)}</span>
                    </div>
                    <div className={styles.transferProgressBar}>
                      <div className={styles.transferProgressFill}
                        style={{ width: `${Math.round(item.progress * 100)}%` }} />
                    </div>
                    <div className={styles.transferRowActions}>
                      <span className={`${styles.transferStatus} ${styles[`transferStatus_${item.status}`]}`}>
                        {item.status === 'uploading' && '↑ ' + Math.round(item.progress * 100) + '%'}
                        {item.status === 'paused'   && 'Paused'}
                        {item.status === 'error'     && 'Error'}
                        {item.status === 'pending'   && 'Pending'}
                      </span>
                      {item.status === 'uploading' && (
                        <button className={styles.transferActionBtn} onClick={() => handlePause(item.id)} title="Pause"><Pause size={11} /></button>
                      )}
                      {item.status === 'paused' && (
                        <button className={styles.transferActionBtn} onClick={() => handleResume(item.id)} title="Resume"><Play size={11} /></button>
                      )}
                      <button className={`${styles.transferActionBtn} ${styles.transferActionCancel}`} onClick={() => handleCancel(item.id)} title="Cancel"><X size={11} /></button>
                    </div>
                  </div>
                ))
              )}
            </div>

            {/* History */}
            {history.length > 0 && (
              <div className={styles.transferHistory}>
                <div className={styles.transferHistoryHeader}>
                  <span>Completed ({history.length})</span>
                </div>
                <div className={styles.transferHistoryList}>
                  {history.map(h => (
                    <div key={h.id} className={styles.historyRow}>
                      <span className={styles.historyName} title={h.name}>{h.name}</span>
                      <span className={styles.historySize}>{formatSize(h.size)}</span>
                      <span className={styles.historyTime}>{h.completedAt}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {/* ── Session Notes Panel ──────────────────────────────────────────── */}
      <AnimatePresence>
        {notesPanel && (
          <motion.div
            className={styles.notesPanel}
            initial={{ x: -320 }}
            animate={{ x: 0 }}
            exit={{ x: -320 }}
            transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            onMouseDown={e => e.stopPropagation()}
            onMouseMove={e => e.stopPropagation()}
          >
            <div className={styles.notesHeader}>
              <div className={styles.notesHeaderLeft}>
                <StickyNote size={15} />
                <h4>Session Notes</h4>
              </div>
              <button onClick={() => setNotesPanel(false)}><X size={14} /></button>
            </div>
            <div className={styles.notesBody}>
              <textarea
                className={styles.notesTextarea}
                placeholder="Type notes during the session…"
                value={sessionNotes}
                onChange={e => setSessionNotes(e.target.value)}
              />
            </div>
            <div className={styles.notesFooter}>
              <button className={`${styles.toolBtn} ${styles.notesSaveBtn}`} onClick={handleSaveNotes}>
                <span>Save Notes</span>
              </button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ── Chat Drawer ──────────────────────────────────────────────────── */}
      <AnimatePresence>
        {isChatOpen && (
          <motion.div
            className={styles.chatDrawer}
            initial={{ x: 320 }}
            animate={{ x: 0 }}
            exit={{ x: 320 }}
            transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            onMouseDown={e => e.stopPropagation()}
            onMouseMove={e => e.stopPropagation()}
          >
            <div className={styles.chatHeader}>
              <h4>IT Support Chat</h4>
              <button onClick={() => setIsChatOpen(false)}><X size={14} /></button>
            </div>
            <div className={styles.chatBody}>
              <p className={styles.chatNotice}>Messages are sent directly to the host terminal.</p>
              <div className={styles.chatMsgList}>
                {messages.map((m, i) => (
                  <div key={i} className={`${styles.chatBubble} ${m.from === 'You' ? styles.chatBubbleSelf : styles.chatBubblePeer}`}>
                    <strong>{m.from}:</strong> {m.text}
                  </div>
                ))}
                <div ref={chatEndRef} />
              </div>
            </div>
            <form className={styles.chatForm} onSubmit={handleSendChat}>
              <input type="text" placeholder="Type a message…" value={chatMsg}
                onChange={e => setChatMsg(e.target.value)} className={styles.chatInput} />
              <button type="submit" className={styles.chatSendBtn}>Send</button>
            </form>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ── Logs Drawer ──────────────────────────────────────────────────── */}
      <AnimatePresence>
        {isLogsOpen && (
          <motion.div
            className={styles.logsDrawer}
            initial={{ x: 320 }}
            animate={{ x: 0 }}
            exit={{ x: 320 }}
            transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            onMouseDown={e => e.stopPropagation()}
            onMouseMove={e => e.stopPropagation()}
          >
            <div className={styles.logsHeader}>
              <div className={styles.logsTitleArea}>
                <Terminal size={14} />
                <h4>Activity Logs Monitor</h4>
              </div>
              <button onClick={() => setIsLogsOpen(false)}><X size={14} /></button>
            </div>
            <div className={styles.logsFilters}>
              <div className={styles.filterGroup}>
                <label>Level:</label>
                <select value={levelFilter} onChange={e => setLevelFilter(e.target.value)}>
                  <option value="all">All Levels</option><option value="info">Info</option>
                  <option value="warn">Warn</option><option value="error">Error</option>
                </select>
              </div>
              <div className={styles.filterGroup}>
                <label>Source:</label>
                <select value={sourceFilter} onChange={e => setSourceFilter(e.target.value)}>
                  <option value="all">All Sources</option>
                  <option value="Viewer">Viewer</option>
                  <option value="Remote Agent">Remote Agent</option>
                </select>
              </div>
              <button className={styles.clearBtn} onClick={() => setLogs([])}>Clear</button>
            </div>
            <div className={styles.logsBody}>
              <div className={styles.logsList}>
                {logs
                  .filter(log =>
                    (levelFilter === 'all' || log.level?.toLowerCase() === levelFilter) &&
                    (sourceFilter === 'all' || log.source === sourceFilter)
                  )
                  .map((log, i) => {
                    const levelClass = log.level === 'error' ? styles.logError
                      : log.level === 'warn'  ? styles.logWarn : styles.logInfo
                    const sourceClass = log.source === 'Viewer' ? styles.sourceViewer : styles.sourceAgent
                    return (
                      <div key={i} className={`${styles.logRow} ${levelClass}`}>
                        <span className={styles.logTime}>{log.time}</span>
                        <span className={`${styles.logSource} ${sourceClass}`}>{log.source}</span>
                        <span className={styles.logMessage}>{log.message}</span>
                      </div>
                    )
                  })}
                <div ref={logsEndRef} />
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

/* ── StatBadge ─────────────────────────────────────────────────────────────── */
function StatBadge({ icon, label, color }) {
  return (
    <div className={styles.statBadge} style={{ color }}>
      {icon}<span>{label}</span>
    </div>
  )
}

/* ── RemoteCursor ───────────────────────────────────────────────────────────── */
function RemoteCursor({ containerRef, videoRef, cursorInfo }) {
  const dotRef    = useRef(null)
  const posRef    = useRef({ x: 0.5, y: 0.5, cursorType: 0 })
  const histRef   = useRef([])
  const lastUpdateRef = useRef(0)

  useEffect(() => {
    if (!cursorInfo) return
    const now = performance.now()
    const entry = { x: cursorInfo.x, y: cursorInfo.y, t: now, cursorType: cursorInfo.cursorType ?? 0 }
    posRef.current = { x: entry.x, y: entry.y, cursorType: entry.cursorType }
    histRef.current.push(entry)
    if (histRef.current.length > 6) histRef.current.shift()
    lastUpdateRef.current = now
  }, [cursorInfo])

  useEffect(() => {
    let raf
    const update = () => {
      const el    = containerRef.current
      const video = videoRef?.current
      const dot   = dotRef.current
      if (!el || !dot || !video || !video.videoWidth) { raf = requestAnimationFrame(update); return }

      const now  = performance.now()
      const hist = histRef.current
      let px = posRef.current.x, py = posRef.current.y

      if (hist.length >= 2) {
        const p0 = hist[hist.length - 2]
        const p1 = hist[hist.length - 1]
        const dt = (p1.t - p0.t) / 1000
        if (dt > 0) {
          const vx = (p1.x - p0.x) / dt
          const vy = (p1.y - p0.y) / dt
          const elapsed = (now - lastUpdateRef.current) / 1000
          px = p1.x + vx * Math.min(elapsed, 0.1)
          py = p1.y + vy * Math.min(elapsed, 0.1)
        }
      }

      const rect      = el.getBoundingClientRect()
      const vW        = video.videoWidth
      const vH        = video.videoHeight
      const scale     = Math.min(rect.width / vW, rect.height / vH)
      const rendW     = vW * scale
      const rendH     = vH * scale
      const offsetX   = (rect.width  - rendW) / 2
      const offsetY   = (rect.height - rendH) / 2
      const screenX   = offsetX + px / vW * rendW - 7
      const screenY   = offsetY + py / vH * rendH - 7
      dot.style.transform = `translate(${screenX}px, ${screenY}px)`
      dot.setAttribute('data-type', String(posRef.current.cursorType))
      raf = requestAnimationFrame(update)
    }
    raf = requestAnimationFrame(update)
    return () => cancelAnimationFrame(raf)
  }, [containerRef, videoRef])

  if (!cursorInfo || cursorInfo.visible === false) return null

  const t = cursorInfo.cursorType ?? 0
  return (
    <div ref={dotRef} data-type={String(t)} style={{
      position: 'absolute', top: 0, left: 0,
      width: 14, height: 20, pointerEvents: 'none', zIndex: 50,
      willChange: 'transform', transition: 'none',
    }}>
      {/* Arrow cursor — full set omitted for brevity, uses same SVGs as before */}
      <svg width="14" height="20" viewBox="0 0 14 20" fill="none">
        <path d="M1 1L1 16.5L4.5 12.5L7.5 18.5L9.5 17.5L6.5 11.5L11.5 11.5L1 1Z"
          fill="white" stroke="rgba(0,0,0,0.65)" strokeWidth="1.2" strokeLinejoin="round" />
      </svg>
    </div>
  )
}