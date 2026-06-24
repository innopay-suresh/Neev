import React, { useState, useEffect, useRef, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Sparkles, Monitor, AlertCircle, FileText, Lightbulb, Zap,
  Send, X, ChevronDown, Shield, Cpu, HardDrive, Wifi,
  Clock, CheckCircle2, ArrowRight, RefreshCw
} from 'lucide-react'
import styles from './AIAssistantPage.module.css'

/* ── Mock data ─────────────────────────────────────────────────────────────── */
const DEVICES = [
  { id: 1, name: 'MacBook Pro 16"',         os: 'macOS 14.5',    cpu: 78, ram: 61, disk: 43, uptime: '4d 7h',  network: 'wifi',     risk: 'low' },
  { id: 2, name: 'Dell XPS 15 Developer',   os: 'Ubuntu 22.04',  cpu: 45, ram: 72, disk: 58, uptime: '12d 2h', network: 'ethernet', risk: 'low' },
  { id: 3, name: 'Ubuntu 22.04 Server',     os: 'Ubuntu 22.04',  cpu: 92, ram: 88, disk: 81, uptime: '89d 1h', network: 'ethernet', risk: 'high' },
  { id: 4, name: 'iMac 27" Retina',         os: 'macOS 14.4',    cpu: 23, ram: 45, disk: 67, uptime: '1d 0h',  network: 'wifi',    risk: 'low' },
  { id: 5, name: 'ThinkPad X1 Carbon',      os: 'Windows 11',    cpu: 67, ram: 83, disk: 74, uptime: '3d 14h', network: 'wifi',    risk: 'medium' },
  { id: 6, name: 'Mac Mini M2',             os: 'macOS 13.6',    cpu: 31, ram: 29, disk: 22, uptime: '7d 3h',  network: 'ethernet', risk: 'low' },
  { id: 7, name: 'Windows Desktop Rig',     os: 'Windows 10',    cpu: 89, ram: 91, disk: 77, uptime: '2d 8h',  network: 'ethernet', risk: 'high' },
  { id: 8, name: 'Raspberry Pi Cluster',    os: 'Raspbian 12',   cpu: 55, ram: 67, disk: 34, uptime: '34d 6h', network: 'ethernet', risk: 'medium' },
]

const MOCK_RESPONSES = {
  device: [
    "Based on the device analysis, this system shows moderate CPU load. The RAM utilization is within acceptable bounds. I recommend scheduling a memory cleanup during off-hours.",
    "The disk usage pattern suggests temporary files are accumulating. A disk cleanup utility run is advised. The network configuration looks stable.",
    "System uptime of 89 days is quite long — consider scheduling a planned reboot to refresh system state and clear memory fragmentation.",
    "This device is performing within normal parameters. All critical metrics (CPU, RAM, disk) are below threshold. No immediate action required.",
  ],
  troubleshoot: {
    default: [
      "Let me analyze that issue step by step. First, check the system event logs for any error entries around the time the issue began. Run: Get-EventLog -LogName System -EntryType Error -Newest 50",
      "Based on the symptoms described, the most likely cause is a service configuration issue. Try restarting the relevant service from Task Manager or via CLI.",
      "Network-related issues often stem from DNS cache or adapter driver problems. Flushing the DNS cache (ipconfig /flushdns) and disabling auto-tuning can help.",
    ],
    slow: [
      "Performance degradation can have several causes. Check Task Manager for any runaway processes. Also verify no Windows updates are pending in the background.",
      "Run a network speed test and check for packet loss. If on Wi-Fi, proximity to the router can affect speeds significantly.",
    ],
    printer: [
      "Printer issues are often resolved by checking the Spooler service. Run: net stop spooler && net start spooler from an elevated prompt.",
      "Clear the print queue from the Printers & Scanners settings. Remove stuck jobs and restart the Print Spooler service.",
    ],
    crash: [
      "Application crashes generate dump files in C:\\ProgramData\\Microsoft\\Windows\\WER\\ReportArchive. Check for recent .dmp files to identify the failing module.",
      "Corrupted user profile data can cause crashes. Try creating a new Windows profile and moving data across to test.",
    ],
    vpn: [
      "VPN failures are commonly caused by time sync issues or conflicting network drivers. Verify the local system time is accurate to the second.",
      "Reinstall the VPN client adapter or update network drivers. Also check if split tunneling is misconfigured.",
    ],
  },
  copilot: [
    "I can see several applications running on the remote desktop including a web browser and terminal. The screen appears responsive.",
    "The remote system appears to have multiple windows open. For better performance, closing unused applications would help.",
    "I observe the Task Manager is open showing the Performance tab. CPU usage seems stable at the current load level.",
    "The desktop shows normal activity. No unusual processes detected in the foreground.",
  ],
  session: [
    { action: 'Connected to MacBook Pro 16"', detail: 'WebRTC session established via TURN relay. Avg latency 42ms, bitrate 2.1 Mbps.', time: '14:22' },
    { action: 'Installed macOS security update', detail: '2024-005 Security Update for macOS Sonoma installed successfully. Restart recommended.', time: '14:31' },
    { action: 'Cleared browser cache', detail: 'Freed 847 MB from Safari browser cache and local storage.', time: '14:38' },
    { action: 'Ran disk health check', detail: 'SSD SMART status: Good. No reallocated sectors. Wear level: 12%.', time: '14:45' },
    { action: 'Session disconnected', detail: 'Session lasted 47 minutes. All actions logged.', time: '15:09' },
  ],
}

/* ── Helpers ───────────────────────────────────────────────────────────────── */
function getMockResponse(category, keyword) {
  let pool
  if (category === 'troubleshoot') {
    const key = Object.keys(MOCK_RESPONSES.troubleshoot).find(function(k) { return keyword && keyword.toLowerCase().includes(k) })
    pool = MOCK_RESPONSES.troubleshoot[key || 'default']
  } else {
    pool = MOCK_RESPONSES[category] || MOCK_RESPONSES.device
  }
  return pool[Math.floor(Math.random() * pool.length)]
}

function getColor(value, warnThreshold, critThreshold) {
  if (value >= critThreshold) return 'var(--danger)'
  if (value >= warnThreshold) return 'var(--warning)'
  return 'var(--success)'
}

function TypingDots() {
  return (
    <div className={styles.typingDots}>
      <span /><span /><span />
    </div>
  )
}

/* ── Device Analysis Tab ────────────────────────────────────────────────────── */
function DeviceAnalysisTab() {
  const [selected, setSelected] = useState(DEVICES[0])
  const [analyzing, setAnalyzing] = useState(false)
  const [result, setResult]       = useState(null)

  function handleAnalyze() {
    setAnalyzing(true)
    setResult(null)
    setTimeout(function() {
      setAnalyzing(false)
      var d = selected
      setResult({
        risk:   d.risk,
        cpu:    d.cpu,
        ram:    d.ram,
        disk:   d.disk,
        uptime: d.uptime,
        network: d.network,
        recs: [
          d.cpu > 80  ? 'High CPU usage detected — investigate runaway processes' : 'CPU usage within normal range',
          d.ram > 80  ? 'Memory pressure elevated — consider closing unused applications' : 'Memory utilization acceptable',
          d.disk > 70 ? 'Disk space running low — free up at least 15% free space recommended' : 'Disk space adequate',
        ],
      })
    }, 1800)
  }

  return (
    <div className={styles.tabPane}>
      <div className={styles.deviceSelect}>
        <label>Select a device to analyze</label>
        <div className={styles.selectWrap}>
          <Monitor size={14} />
          <select value={selected.id} onChange={function(e) { setSelected(DEVICES.find(function(d) { return d.id === parseInt(e.target.value, 10) })); setResult(null) }}>
            {DEVICES.map(function(d) { return <option key={d.id} value={d.id}>{d.name} — {d.os}</option> })}
          </select>
          <ChevronDown size={13} />
        </div>
      </div>

      <button className={"btn-primary " + styles.analyzeBtn} onClick={handleAnalyze} disabled={analyzing}>
        {analyzing
          ? <><RefreshCw size={13} className={styles.spin} /> Analyzing…</>
          : <><Sparkles size={13} /> Analyze Device</>}
      </button>

      {result && (
        <motion.div className={styles.analysisCard} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}>
          <div className={styles.analysisHeader}>
            <div className={styles.analysisTitle}>
              <Shield size={16} />
              <span>Analysis Report — {selected.name}</span>
            </div>
            <span className={styles.riskBadge + ' ' + styles['risk_' + result.risk]}>
              {result.risk === 'high' ? 'High Risk' : result.risk === 'medium' ? 'Medium Risk' : 'Low Risk'}
            </span>
          </div>

          <div className={styles.specGrid}>
            <div className={styles.specItem}>
              <Cpu size={13} /><span>CPU</span>
              <div className={styles.specBar}><div className={styles.specFill} style={{ width: (result.cpu) + '%', background: getColor(result.cpu, 60, 80) }} /></div>
              <span>{result.cpu}%</span>
            </div>
            <div className={styles.specItem}>
              <HardDrive size={13} /><span>RAM</span>
              <div className={styles.specBar}><div className={styles.specFill} style={{ width: (result.ram) + '%', background: getColor(result.ram, 60, 80) }} /></div>
              <span>{result.ram}%</span>
            </div>
            <div className={styles.specItem}>
              <HardDrive size={13} /><span>Disk</span>
              <div className={styles.specBar}><div className={styles.specFill} style={{ width: (result.disk) + '%', background: getColor(result.disk, 50, 70) }} /></div>
              <span>{result.disk}%</span>
            </div>
            <div className={styles.specItem}>
              <Clock size={13} /><span>Uptime</span>
              <span className={styles.specValue}>{result.uptime}</span>
            </div>
            <div className={styles.specItem}>
              <Wifi size={13} /><span>Network</span>
              <span className={styles.specValue}>{result.network}</span>
            </div>
          </div>

          <div className={styles.recList}>
            <div className={styles.recHeader}><Lightbulb size={13} /><span>Recommendations</span></div>
            {result.recs.map(function(r, i) { return (
              <div key={i} className={styles.recItem}><ArrowRight size={11} /><span>{r}</span></div>
            )})}
          </div>
        </motion.div>
      )}
    </div>
  )
}

/* ── Chat Tab ───────────────────────────────────────────────────────────────── */
function ChatTab(props) {
  var _props = props || {}
  var title        = _props.title
  var TitleIcon    = _props.icon
  var placeholder  = _props.placeholder
  var sessionContext = _props.sessionContext
  var mockCategory = _props.mockCategory

  var _useState = useState([])
  var messages  = _useState[0]
  var setMessages = _useState[1]
  var _useState2 = useState('')
  var input     = _useState2[0]
  var setInput  = _useState2[1]
  var _useState3 = useState(false)
  var typing    = _useState3[0]
  var setTyping = _useState3[1]
  var bottomRef = useRef(null)
  var inputRef  = useRef(null)

  useEffect(function() {
    bottomRef.current && bottomRef.current.scrollIntoView({ behavior: 'smooth' })
  }, [messages, typing])

  useEffect(function() {
    inputRef.current && inputRef.current.focus()
  }, [])

  var sendMessage = useCallback(function() {
    if (!input.trim() || typing) return
    var userMsg = { from: 'user', text: input.trim() }
    setMessages(function(prev) { return prev.concat([userMsg]) })
    var keyword = input
    setInput('')
    setTyping(true)
    setTimeout(function() {
      setTyping(false)
      var prefix = sessionContext ? '[' + sessionContext + '] ' : ''
      var aiText = prefix + getMockResponse(mockCategory, keyword)
      setMessages(function(prev) { return prev.concat([{ from: 'ai', text: aiText }]) })
    }, 1600 + Math.random() * 600)
  }, [input, typing, mockCategory, sessionContext])

  return (
    <div className={styles.chatTab}>
      {sessionContext && (
        <div className={styles.contextBanner}>
          <Zap size={12} /><span>Session context: {sessionContext}</span>
        </div>
      )}

      <div className={styles.messageList}>
        {messages.length === 0 && !typing && (
          <div className={styles.chatEmpty}>
            <TitleIcon size={28} strokeWidth={1.5} />
            <p>{placeholder}</p>
          </div>
        )}

        <AnimatePresence>
          {messages.map(function(m, i) { return (
            <motion.div
              key={i}
              className={styles.messageRow + (m.from === 'user' ? (' ' + styles.messageRowUser) : '')}
              initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }}
            >
              <div className={styles.messageBubble + (m.from === 'user' ? (' ' + styles.messageUser) : (' ' + styles.messageAi))}>
                {m.text}
              </div>
            </motion.div>
          )})}
        </AnimatePresence>

        {typing && (
          <motion.div className={styles.messageRow} initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
            <div className={styles.messageBubble + ' ' + styles.messageAi}>
              <TypingDots />
            </div>
          </motion.div>
        )}

        <div ref={bottomRef} />
      </div>

      <div className={styles.chatInputBar}>
        <input
          ref={inputRef}
          type="text" placeholder={placeholder} value={input}
          onChange={function(e) { setInput(e.target.value) }}
          onKeyDown={function(e) { if (e.key === 'Enter') sendMessage() }}
        />
        <button className={styles.sendBtn} onClick={sendMessage} disabled={!input.trim() || typing}>
          <Send size={14} />
        </button>
      </div>
    </div>
  )
}

/* ── Session Summary Tab ────────────────────────────────────────────────────── */
function SessionSummaryTab() {
  var _useState4 = useState(null)
  var summary    = _useState4[0]
  var setSummary = _useState4[1]
  var _useState5 = useState(false)
  var generating = _useState5[0]
  var setGenerating = _useState5[1]

  function handleGenerate() {
    setGenerating(true)
    setSummary(null)
    setTimeout(function() {
      setGenerating(false)
      setSummary({
        device: 'MacBook Pro 16"',
        os: 'macOS 14.5',
        duration: '47 minutes',
        agentVersion: 'v1.4.2',
        actions: MOCK_RESPONSES.session,
      })
    }, 2200)
  }

  return (
    <div className={styles.tabPane}>
      {!summary && !generating && (
        <div className={styles.summaryPrompt}>
          <FileText size={36} strokeWidth={1.5} className={styles.summaryIcon} />
          <h3>Generate Session Summary</h3>
          <p>Automatically create a detailed report of actions taken during the session, including timestamps, changes made, and diagnostics run.</p>
          <button className="btn-primary" onClick={handleGenerate}><FileText size={13} /> Generate Summary</button>
        </div>
      )}

      {generating && (
        <div className={styles.summaryPrompt}>
          <div className={styles.spinner} />
          <p>Generating summary…</p>
        </div>
      )}

      {summary && (
        <motion.div className={styles.summaryOutput} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}>
          <div className={styles.summaryHeader}>
            <div>
              <h3>Session Summary</h3>
              <p>{summary.device} · {summary.os} · {summary.duration} · {summary.agentVersion}</p>
            </div>
            <CheckCircle2 size={20} color="var(--success)" />
          </div>

          <div className={styles.timeline}>
            {summary.actions.map(function(a, i) { return (
              <div key={i} className={styles.timelineItem}>
                <div className={styles.timelineDot} />
                <div className={styles.timelineContent}>
                  <div className={styles.timelineHeader}>
                    <span className={styles.timelineAction}>{a.action}</span>
                    <span className={styles.timelineTime}>{a.time}</span>
                  </div>
                  <p className={styles.timelineDetail}>{a.detail}</p>
                </div>
              </div>
            )})}
          </div>
        </motion.div>
      )}
    </div>
  )
}

/* ── Main Page ─────────────────────────────────────────────────────────────── */
var TABS = [
  { id: 'device',       label: 'Device Analysis',    icon: Monitor },
  { id: 'troubleshoot', label: 'AI Troubleshooting', icon: AlertCircle },
  { id: 'copilot',      label: 'Session Copilot',    icon: Zap },
  { id: 'summary',      label: 'Session Summary',    icon: FileText },
]

export function AIAssistantPage() {
  var _useState6 = useState('device')
  var activeTab  = _useState6[0]
  var setActiveTab = _useState6[1]

  function renderTab() {
    if (activeTab === 'device')       return <DeviceAnalysisTab />
    if (activeTab === 'troubleshoot') return <ChatTab title="AI Troubleshooting" icon={AlertCircle} placeholder="Describe your issue…" mockCategory="troubleshoot" />
    if (activeTab === 'copilot')      return <ChatTab title="Session Copilot" icon={Zap} placeholder="Ask about what's happening on screen…" sessionContext="MacBook Pro · macOS 14.5 · Session 47min" mockCategory="copilot" />
    if (activeTab === 'summary')      return <SessionSummaryTab />
    return null
  }

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">AI Assistant</h1>
          <p className="page-subtitle">AI-powered troubleshooting, analysis, and automation</p>
        </div>
        <div className={styles.aiBadge}>
          <Sparkles size={13} />
          <span>Powered by Neev Remote AI</span>
        </div>
      </div>

      <div className={styles.tabBar}>
        {TABS.map(function(tab) { return (
          <button
            key={tab.id}
            className={styles.tabItem + (activeTab === tab.id ? (' ' + styles.tabActive) : '')}
            onClick={function() { setActiveTab(tab.id) }}
          >
            <tab.icon size={13} />
            <span>{tab.label}</span>
          </button>
        )})}
      </div>

      <AnimatePresence mode="wait">
        <motion.div
          key={activeTab}
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.18 }}
          className={styles.tabContent}
        >
          {renderTab()}
        </motion.div>
      </AnimatePresence>
    </div>
  )
}