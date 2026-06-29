import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Shield, Users, KeyRound, FileText, AlertTriangle, CheckCircle2,
  Copy, Check, Clock, ShieldCheck, X, ChevronDown, ChevronRight,
  RefreshCw, Smartphone, Lock, Sliders, AlertOctagon, Search, Filter
} from 'lucide-react'
import { apiFetch } from '../lib/api.js'
import styles from './SecurityPage.module.css'

/** Maps a backend audit event to the row shape the audit table renders. */
function toAuditRow(e, i) {
  const ts = e.created_at ? new Date(e.created_at) : new Date()
  const level = (e.outcome === 'denied' || e.outcome === 'error') ? 'error'
    : (e.outcome === 'rate_limited' || (e.type || '').includes('revoke')) ? 'warn'
    : 'info'
  const detailParts = []
  if (e.target) detailParts.push(`target=${e.target}`)
  if (e.outcome) detailParts.push(`outcome=${e.outcome}`)
  if (e.details) detailParts.push(JSON.stringify(e.details))
  return {
    id: e.id || i,
    time: ts.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
    date: ts.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    user: e.actor || '—',
    action: e.type || 'event',
    ip: e.ip || '—',
    level,
    source: (e.type || '').split('.')[0] || 'system',
    details: detailParts.join(' · ') || (e.type || ''),
  }
}

/* ── QR Code generator (no library) ──────────────────────────────────────── */
function generateQRMatrix(text) {
  const size = 25
  const matrix = Array.from({ length: size }, () => Array(size).fill(false))
  const addFinder = (row, col) => {
    for (let r = -1; r <= 7; r++) {
      for (let c = -1; c <= 7; c++) {
        const R = row + r, C = col + c
        if (R < 0 || R >= size || C < 0 || C >= size) continue
        if (r >= 0 && r <= 6 && c >= 0 && c <= 6) {
          matrix[R][C] = (r === 0 || r === 6 || c === 0 || c === 6 ||
            (r >= 2 && r <= 4 && c >= 2 && c <= 4)) && !(r === 2 && c === 2)
        } else if ((r === -1 || r === 7) && (c >= -1 && c <= 7)) {
          matrix[R][C] = c >= 0 && c <= 7
        } else if ((c === -1 || c === 7) && (r >= -1 && r <= 7)) {
          matrix[R][C] = r >= 0 && r <= 7
        }
      }
    }
  }
  addFinder(0, 0)
  addFinder(0, size - 8)
  addFinder(size - 8, 0)
  for (let i = 8; i < size - 8; i++) {
    matrix[6][i] = i % 2 === 0
    matrix[i][6] = i % 2 === 0
  }
  const ax = size - 9, ay = size - 9
  for (let r = -4; r <= 4; r++) {
    for (let c = -4; c <= 4; c++) {
      const R = ay + r, C = ax + c
      if (R >= 0 && R < size && C >= 0 && C < size) {
        matrix[R][C] = Math.abs(r) === 4 || Math.abs(c) === 4 || (Math.abs(r) <= 2 && Math.abs(c) <= 2)
      }
    }
  }
  let hash = 0
  for (let i = 0; i < text.length; i++) hash = ((hash << 5) - hash + text.charCodeAt(i)) | 0
  const ds = 9, de = size - 9
  for (let r = ds; r < de; r++) {
    for (let c = ds; c < de; c++) {
      if (!matrix[r][c]) {
        const idx = (r - ds) * (de - ds) + (c - ds)
        matrix[r][c] = ((hash >> (idx % 32)) & 1) === 1
      }
    }
  }
  return matrix
}

function drawQR(canvas, text) {
  const ctx = canvas.getContext('2d')
  canvas.width = 200; canvas.height = 200
  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, 200, 200)
  const matrix = generateQRMatrix(text)
  const mSize = Math.floor(200 / matrix.length)
  ctx.fillStyle = '#0F172A'
  for (let r = 0; r < matrix.length; r++) {
    for (let c = 0; c < matrix[r].length; c++) {
      if (matrix[r][c]) ctx.fillRect(c * mSize, r * mSize, mSize - 1, mSize - 1)
    }
  }
}

/* ── Helpers ───────────────────────────────────────────────────────────────── */
function copyToClipboard(text) { navigator.clipboard.writeText(text).catch(() => {}) }

function generateBackupCodes(count = 8) {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  return Array.from({ length: count }, () => {
    let code = ''
    for (let i = 0; i < 8; i++) code += chars[Math.floor(Math.random() * chars.length)]
    return code.slice(0, 4) + '-' + code.slice(4)
  })
}

/* ── Permission data ───────────────────────────────────────────────────────── */
const PERMISSIONS = [
  { key: 'device_access',   label: 'Device Access',     desc: 'View and connect to devices' },
  { key: 'file_transfer',   label: 'File Transfer',      desc: 'Send and receive files' },
  { key: 'clipboard',       label: 'Clipboard Sync',     desc: 'Sync clipboard with remote' },
  { key: 'chat',            label: 'Chat',               desc: 'Send messages to host' },
  { key: 'logs_view',       label: 'View Logs',          desc: 'Access activity logs' },
  { key: 'wo_lan',          label: 'Wake on LAN',        desc: 'Wake sleeping devices' },
  { key: 'session_approve', label: 'Session Approval',   desc: 'Approve pending sessions' },
  { key: 'user_management', label: 'User Management',    desc: 'Invite and manage users' },
]

const ROLES = [
  { id: 'admin',    name: 'Admin',      color: '#4F8CFF', desc: 'Full system access',         defaultPermissions: PERMISSIONS.map(p => p.key) },
  { id: 'operator', name: 'Operator',   color: '#22C55E', desc: 'Day-to-day operations',      defaultPermissions: ['device_access', 'file_transfer', 'clipboard', 'chat', 'logs_view', 'wo_lan'] },
  { id: 'viewer',   name: 'Viewer',     color: '#94A3B8', desc: 'Read-only access',           defaultPermissions: ['device_access', 'logs_view'] },
  { id: 'helpdesk', name: 'Helpdesk',   color: '#F59E0B', desc: 'Limited support access',     defaultPermissions: ['device_access', 'chat', 'logs_view', 'session_approve'] },
]

/* ── Mock audit log ────────────────────────────────────────────────────────── */
function makeAuditEntry(i) {
  const events = [
    { action: 'User login',             level: 'info',  src: 'auth' },
    { action: 'User logout',            level: 'info',  src: 'auth' },
    { action: 'Session started',        level: 'info',  src: 'session' },
    { action: 'Session ended',          level: 'info',  src: 'session' },
    { action: 'Permission changed',     level: 'warn',  src: 'rbac' },
    { action: 'Failed auth attempt',    level: 'error', src: 'auth' },
    { action: 'MFA enrolled',           level: 'info',  src: 'mfa' },
    { action: 'Session approved',       level: 'info',  src: 'session' },
    { action: 'Session rejected',       level: 'warn',  src: 'session' },
    { action: 'Role assigned',          level: 'info',  src: 'rbac' },
    { action: 'File transfer completed', level: 'info', src: 'session' },
    { action: 'Admin login',            level: 'warn',  src: 'auth' },
  ]
  const users = ['alex@co.com', 'sam@co.com', 'admin@co.com', 'jordan@co.com', 'taylor@co.com', 'dev@co.com']
  const ips = ['203.0.113.1', '198.51.100.5', '192.0.2.10', '10.0.0.42', '172.16.0.3']
  const ev = events[i % events.length]
  const ts = new Date(Date.now() - i * 7 * 60000)
  return {
    id: i,
    time: ts.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
    date: ts.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    user: users[i % users.length],
    action: ev.action,
    ip: ips[i % ips.length],
    level: ev.level,
    source: ev.src,
    details: `${ev.action} by ${users[i % users.length]} from ${ips[i % ips.length]}`,
  }
}

const AUDIT_LOG = Array.from({ length: 50 }, (_, i) => makeAuditEntry(i))

/* ── Mock pending approvals ────────────────────────────────────────────────── */
function makePending(id) {
  const users = ['alex@co.com', 'sam@co.com', 'jordan@co.com']
  const devices = ['MacBook Pro 16"', 'Dell XPS 15', 'iMac 27"', 'ThinkPad X1']
  const reasons = ['Software installation support', 'System troubleshooting', 'File recovery', 'Performance diagnostics']
  return {
    id,
    user: users[id % users.length],
    device: devices[id % devices.length],
    reason: reasons[id % reasons.length],
    ts: new Date(Date.now() - id * 60000).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
    countdown: (4 + id) * 60,
  }
}

/* ── Tab components ─────────────────────────────────────────────────────────── */
function RbacTab({ permissions, onToggle, onSave }) {
  return (
    <div className={styles.tabContent}>
      <div className={styles.sectionHead}>
        <h3>Roles &amp; Permissions</h3>
        <p>Define what each role can do. Changes apply immediately to all members.</p>
      </div>
      <div className={styles.rolesGrid}>
        {ROLES.map(role => (
          <div key={role.id} className={styles.roleCard}>
            <div className={styles.roleCardHeader} style={{ borderLeftColor: role.color }}>
              <div>
                <div className={styles.roleName}>{role.name}</div>
                <div className={styles.roleDesc}>{role.desc}</div>
              </div>
              <div className={styles.rolePermCount}>
                {Object.values(permissions[role.id] || {}).filter(Boolean).length}
                <span>/{PERMISSIONS.length}</span>
              </div>
            </div>
            <div className={styles.permissionsList}>
              {PERMISSIONS.map(perm => {
                const has = permissions[role.id]?.[perm.key] ?? role.defaultPermissions.includes(perm.key)
                return (
                  <div key={perm.key} className={styles.permRow}>
                    <div className={styles.permInfo}>
                      <span className={styles.permLabel}>{perm.label}</span>
                      <span className={styles.permDesc}>{perm.desc}</span>
                    </div>
                    <button
                      className={`${styles.toggle} ${has ? styles.toggleOn : ''}`}
                      onClick={() => onToggle(role.id, perm.key, !has)}
                      role="switch" aria-checked={has}
                    >
                      <span className={styles.toggleThumb} />
                    </button>
                  </div>
                )
              })}
            </div>
          </div>
        ))}
      </div>
      <div className={styles.saveBar}>
        <button className="btn-primary" onClick={onSave}>
          <CheckCircle2 size={13} /> Save Changes
        </button>
        <span className={styles.saveHint}>Changes take effect immediately</span>
      </div>
    </div>
  )
}

function MfaTab() {
  const [phase, setPhase] = useState('idle')
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState([])
  const [copied, setCopied] = useState(null)
  const [error, setError] = useState('')
  const canvasRef = useRef(null)
  const otpUri = 'otpauth://totp/Neev Remote:admin@co.com?secret=JBSWY3DPEHPK3PXP&issuer=Neev Remote&algorithm=SHA1&digits=6&period=30'

  useEffect(() => {
    if (phase === 'scanning' && canvasRef.current) drawQR(canvasRef.current, otpUri)
  }, [phase])

  const handleVerify = () => {
    if (code === '123456') {
      setCodes(generateBackupCodes())
      setPhase('active')
      setError('')
    } else {
      setError('Invalid code. Try 123456 for demo.')
    }
  }

  const handleCopy = (c, i) => {
    copyToClipboard(c)
    setCopied(i)
    setTimeout(() => setCopied(null), 1500)
  }

  if (phase === 'idle') {
    return (
      <div className={styles.tabContent}>
        <div className={styles.mfaSetupCard}>
          <div className={styles.mfaIconWrap}><Smartphone size={36} strokeWidth={1.5} /></div>
          <h3>Set Up Authenticator App</h3>
          <p>Secure your account with two-factor authentication using an authenticator app.</p>
          <div className={styles.mfaSteps}>
            {['Download Google Authenticator, Microsoft Authenticator, or Authy', 'Scan the QR code when you proceed', 'Enter the 6-digit code to verify setup'].map((s, i) => (
              <div key={i} className={styles.mfaStep}>
                <span className={styles.mfaStepNum}>{i + 1}</span>
                <span>{s}</span>
              </div>
            ))}
          </div>
          <button className="btn-primary" onClick={() => setPhase('scanning')}>
            <KeyRound size={13} /> Start Setup
          </button>
        </div>
      </div>
    )
  }

  if (phase === 'scanning' || phase === 'verifying') {
    return (
      <div className={styles.tabContent}>
        <div className={styles.mfaSetupCard}>
          <div className={styles.mfaQrSection}>
            <div className={styles.mfaQrLabel}>Scan this QR code with your authenticator app</div>
            <canvas ref={canvasRef} className={styles.qrCanvas} width={200} height={200} />
            <div className={styles.mfaManualKey}>
              Or enter this key manually: <code>JBSWY3DPEHPK3PXP</code>
              <button className={styles.copyKeyBtn} onClick={() => copyToClipboard('JBSWY3DPEHPK3PXP')}>
                <Copy size={11} /> Copy
              </button>
            </div>
          </div>
          <div className={styles.mfaVerifySection}>
            <h4>Enter the 6-digit code</h4>
            <p>Open your authenticator app and type the code shown for <strong>Neev Remote</strong></p>
            <input
              type="text" className={styles.codeInput} placeholder="000000" maxLength={6} value={code}
              onChange={e => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              onKeyDown={e => e.key === 'Enter' && handleVerify()}
            />
            {error && <div className={styles.codeError}>{error}</div>}
            <div className={styles.mfaHint}>Demo: use code <strong>123456</strong></div>
            <div className={styles.mfaActions}>
              <button className="btn-ghost" onClick={() => { setPhase('idle'); setCode(''); setError('') }}>Back</button>
              <button className="btn-primary" onClick={handleVerify} disabled={code.length < 6}>
                <ShieldCheck size={13} /> Verify Code
              </button>
            </div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className={styles.tabContent}>
      <div className={styles.mfaActiveCard}>
        <div className={styles.mfaActiveHeader}>
          <CheckCircle2 size={22} color="var(--success)" />
          <div>
            <h3>MFA Active</h3>
            <p>Your account is protected with two-factor authentication.</p>
          </div>
          <button className="btn-ghost" onClick={() => { setPhase('idle'); setCode(''); setCodes([]) }}>
            <RefreshCw size={12} /> Reset
          </button>
        </div>
        <div className={styles.backupSection}>
          <div className={styles.backupHeader}>
            <Lock size={14} />
            <h4>Backup Codes</h4>
            <span className={styles.backupNote}>Save these somewhere safe. Each code can only be used once.</span>
          </div>
          <div className={styles.backupCodesGrid}>
            {codes.map((c, i) => (
              <div key={i} className={styles.backupCode} onClick={() => handleCopy(c, i)}>
                {copied === i ? <Check size={11} /> : null}
                <span>{c}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function ApprovalsTab() {
  const [pending, setPending] = useState([makePending(0), makePending(1), makePending(2)])
  const [approved, setApproved] = useState([])
  const [rejected, setRejected] = useState([])
  const [rejectReason, setRejectReason] = useState('')
  const [rejectTarget, setRejectTarget] = useState(null)
  const [toast, setToast] = useState(null)

  useEffect(() => {
    const id = setInterval(() => {
      setPending(prev => prev.map(s => s.countdown <= 1 ? null : { ...s, countdown: s.countdown - 1 }).filter(Boolean))
    }, 1000)
    return () => clearInterval(id)
  }, [])

  const showToast = (msg) => { setToast(msg); setTimeout(() => setToast(null), 3000) }
  const fmt = (s) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`

  const handleApprove = (id) => {
    const item = pending.find(s => s.id === id)
    if (!item) return
    setPending(prev => prev.filter(s => s.id !== id))
    setApproved(prev => [{ ...item, outcome: 'approved' }, ...prev])
    showToast(`Session for ${item.user} approved`)
  }

  const handleRejectConfirm = () => {
    const item = pending.find(s => s.id === rejectTarget)
    if (!item) return
    setPending(prev => prev.filter(s => s.id !== rejectTarget))
    setRejected(prev => [{ ...item, reason: rejectReason || 'No reason provided', outcome: 'rejected' }, ...prev])
    setRejectReason('')
    setRejectTarget(null)
    showToast(`Session for ${item.user} rejected`)
  }

  return (
    <div className={styles.tabContent}>
      <div className={styles.sectionHead}>
        <h3>Session Approvals</h3>
        <p>Review and approve pending session requests. Unapproved sessions auto-expire in 5 minutes.</p>
      </div>
      <AnimatePresence>
        {toast && (
          <motion.div className={styles.toast} initial={{ opacity: 0, y: -10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}>
            <CheckCircle2 size={13} /> {toast}
          </motion.div>
        )}
      </AnimatePresence>

      {pending.length === 0 ? (
        <div className={styles.emptyState}>
          <CheckCircle2 size={40} strokeWidth={1} color="var(--success)" />
          <h4>All clear</h4>
          <p>No pending session requests</p>
        </div>
      ) : (
        <div className={styles.approvalsList}>
          <div className={styles.approvalsHeader}>
            <span>User</span><span>Device</span><span>Reason</span><span>Time</span><span>Expires</span><span>Action</span>
          </div>
          {pending.map(s => (
            <div key={s.id} className={styles.pendingRow}>
              <span className={styles.cellUser}>{s.user}</span>
              <span className={styles.cellDevice}>{s.device}</span>
              <span className={styles.cellReason}>{s.reason}</span>
              <span className={styles.cellTime}>{s.ts}</span>
              <span className={`${styles.cellCountdown} ${s.countdown < 60 ? styles.urgent : ''}`}>
                <Clock size={10} /> {fmt(s.countdown)}
              </span>
              <span className={styles.cellActions}>
                <button className={styles.approveBtn} onClick={() => handleApprove(s.id)}>
                  <Check size={12} /> Approve
                </button>
                <button className={styles.rejectBtn} onClick={() => setRejectTarget(s.id)}>
                  <X size={12} /> Reject
                </button>
              </span>
            </div>
          ))}
        </div>
      )}

      <AnimatePresence>
        {rejectTarget !== null && (
          <motion.div className={styles.modalBackdrop} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={() => setRejectTarget(null)}>
            <motion.div className={styles.modal} initial={{ scale: 0.95, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} onClick={e => e.stopPropagation()}>
              <h4>Reject Session</h4>
              <p>Optionally provide a reason for the requester.</p>
              <textarea className={styles.modalTextarea} placeholder="Reason (optional)" value={rejectReason} onChange={e => setRejectReason(e.target.value)} rows={3} />
              <div className={styles.modalActions}>
                <button className="btn-ghost" onClick={() => setRejectTarget(null)}>Cancel</button>
                <button className={styles.rejectBtn} onClick={handleRejectConfirm}><X size={12} /> Confirm Rejection</button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {(approved.length > 0 || rejected.length > 0) && (
        <div className={styles.approvalsHistory}>
          <h4>Recent Decisions</h4>
          {[...approved, ...rejected].slice(0, 10).map(s => (
            <div key={s.id + '_' + s.outcome} className={styles.historyRow}>
              <span>{s.user}</span><span>{s.device}</span>
              <span className={s.outcome === 'approved' ? styles.hApprove : styles.hReject}>
                {s.outcome === 'approved' ? <Check size={11} /> : <X size={11} />}
                {s.outcome}
              </span>
              <span className={styles.hReason}>{s.reason || '—'}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function AuditTab() {
  const [query, setQuery] = useState('')
  const [levelFilter, setLevelFilter] = useState('all')
  const [expanded, setExpanded] = useState(null)
  const [events, setEvents] = useState([])
  const parentRef = useRef(null)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const res = await apiFetch('/api/v1/dashboard/audit')
        if (!res.ok) return
        const data = await res.json()
        if (!cancelled) setEvents((data.events || []).map(toAuditRow))
      } catch { /* keep empty */ }
    }
    load()
    const t = setInterval(load, 8000)
    return () => { cancelled = true; clearInterval(t) }
  }, [])

  const filtered = useMemo(() => {
    let entries = events
    if (levelFilter !== 'all') entries = entries.filter(e => e.level === levelFilter)
    if (query.trim()) {
      const q = query.toLowerCase()
      entries = entries.filter(e =>
        e.user.toLowerCase().includes(q) || e.action.toLowerCase().includes(q) ||
        e.ip.toLowerCase().includes(q) || e.details.toLowerCase().includes(q)
      )
    }
    return entries
  }, [query, levelFilter, events])

  const rowVirtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 48,
    overscan: 5,
  })

  const levelBadge = (l) => l === 'error' ? 'danger' : l === 'warn' ? 'warning' : 'default'
  const levelColor  = (l) => l === 'error' ? 'var(--danger)' : l === 'warn' ? 'var(--warning)' : 'var(--text-secondary)'

  return (
    <div className={styles.tabContent}>
      <div className={styles.sectionHead}>
        <h3>Audit Log</h3>
        <p>All security events across your organization. Click any row to expand details.</p>
      </div>
      <div className={styles.auditControls}>
        <div className={styles.auditSearch}>
          <Search size={13} />
          <input type="text" placeholder="Search user, action, IP…" value={query} onChange={e => setQuery(e.target.value)} />
          {query && <button onClick={() => setQuery('')}><X size={12} /></button>}
        </div>
        <div className={styles.auditFilter}>
          <Filter size={13} />
          <select value={levelFilter} onChange={e => setLevelFilter(e.target.value)}>
            <option value="all">All Levels</option>
            <option value="info">Info</option>
            <option value="warn">Warning</option>
            <option value="error">Error</option>
          </select>
        </div>
        <div className={styles.auditCount}>{filtered.length} events</div>
      </div>
      <div className={styles.auditTableWrap}>
        <div className={styles.auditTableHeader}>
          <span style={{ flex: 1.5 }}>Time</span>
          <span style={{ flex: 1.5 }}>User</span>
          <span style={{ flex: 2 }}>Action</span>
          <span style={{ flex: 1.2 }}>IP</span>
          <span style={{ flex: 0.8 }}>Level</span>
          <span style={{ flex: 0.4 }} />
        </div>
        <div ref={parentRef} className={styles.auditScroll}>
          {filtered.length === 0 ? (
            <div className={styles.emptyState}>
              <AlertTriangle size={32} strokeWidth={1} />
              <p>No events match your filters</p>
            </div>
          ) : (
            <div style={{ height: rowVirtualizer.getTotalSize() + 'px', position: 'relative' }}>
              {rowVirtualizer.getVirtualItems().map(vrow => {
                const entry = filtered[vrow.index]
                const isOpen = expanded === entry.id
                return (
                  <div key={entry.id} style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: vrow.size + 'px', transform: `translateY(${vrow.start}px)` }}>
                    <div className={`${styles.auditRow} ${isOpen ? styles.auditRowOpen : ''}`} onClick={() => setExpanded(isOpen ? null : entry.id)}>
                      <span style={{ flex: 1.5, fontSize: 12, color: 'var(--text-secondary)' }}>{entry.date} {entry.time}</span>
                      <span style={{ flex: 1.5, fontSize: 12, fontWeight: 500 }}>{entry.user}</span>
                      <span style={{ flex: 2, fontSize: 12 }}>{entry.action}</span>
                      <span style={{ flex: 1.2, fontSize: 12, color: 'var(--text-muted)', fontFamily: 'monospace' }}>{entry.ip}</span>
                      <span style={{ flex: 0.8 }}><span className={`badge badge-${levelBadge(entry.level)}`} style={{ fontSize: 10 }}>{entry.level}</span></span>
                      <span style={{ flex: 0.4, display: 'flex', justifyContent: 'flex-end' }}>{isOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}</span>
                    </div>
                    {isOpen && (
                      <div className={styles.auditDetail}>
                        <div className={styles.auditDetailRow}><span>Full Details</span><span>{entry.details}</span></div>
                        <div className={styles.auditDetailRow}><span>Source</span><span style={{ textTransform: 'capitalize' }}>{entry.source}</span></div>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

/* ── Main SecurityPage ─────────────────────────────────────────────────────── */
export function SecurityPage() {
  const [activeTab, setActiveTab] = useState('rbac')
  const [permissions, setPermissions] = useState(() =>
    Object.fromEntries(ROLES.map(r => [r.id, Object.fromEntries(r.defaultPermissions.map(p => [p, true]))]))
  )
  const handleToggle = useCallback((roleId, permKey, val) => {
    setPermissions(prev => ({ ...prev, [roleId]: { ...prev[roleId], [permKey]: val } }))
  }, [])
  const handleSave = () => console.log('[security] permissions updated:', permissions)

  const TABS = [
    { id: 'rbac',      label: 'RBAC',          icon: <Shield size={13} /> },
    { id: 'mfa',       label: 'MFA',            icon: <KeyRound size={13} /> },
    { id: 'approvals', label: 'Approvals',      icon: <CheckCircle2 size={13} /> },
    { id: 'audit',     label: 'Audit Log',      icon: <FileText size={13} /> },
  ]

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Security</h1>
          <p className="page-subtitle">Access control, MFA, and audit logs</p>
        </div>
      </div>
      <div className={styles.tabBar}>
        {TABS.map(tab => (
          <button key={tab.id} className={`${styles.tabItem} ${activeTab === tab.id ? styles.tabActive : ''}`} onClick={() => setActiveTab(tab.id)}>
            {tab.icon}<span>{tab.label}</span>
          </button>
        ))}
      </div>
      <AnimatePresence mode="wait">
        <motion.div key={activeTab} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }}>
          {activeTab === 'rbac'      && <RbacTab      permissions={permissions} onToggle={handleToggle} onSave={handleSave} />}
          {activeTab === 'mfa'       && <MfaTab       />}
          {activeTab === 'approvals' && <ApprovalsTab />}
          {activeTab === 'audit'     && <AuditTab     />}
        </motion.div>
      </AnimatePresence>
    </div>
  )
}