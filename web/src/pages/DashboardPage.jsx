import React, { useState, useEffect, useCallback } from 'react'
import { motion } from 'framer-motion'
import {
  Monitor, Users, Activity, Wifi, Clock, Shield, Copy,
  RefreshCw, Circle, TrendingUp, Server, Zap, AlertTriangle,
  Sparkles, TrendingDown, ArrowUpRight, ArrowDownRight, MonitorCheck
} from 'lucide-react'
import {
  AreaChart, Area, XAxis, YAxis,
  Tooltip, ResponsiveContainer, CartesianGrid
} from 'recharts'
import { useAppLogs } from '../logs/AppLogsContext.jsx'
import { apiFetch, clearAuthToken, setAuthToken } from '../lib/api.js'
import styles from './DashboardPage.module.css'

const genSparkData = (n = 20, base = 100, variance = 40) =>
  Array.from({ length: n }, (_, i) => ({
    t: i,
    v: Math.max(0, base + Math.sin(i * 0.5) * variance + (Math.random() - 0.5) * variance * 0.5),
  }))

/* ── Mini Sparkline SVG ────────────────────────────────────────────────── */
function Sparkline({ data, color = 'var(--accent)', height = 36 }) {
  const max = Math.max(...data.map(d => d.v))
  const min = Math.min(...data.map(d => d.v))
  const range = max - min || 1
  const W = 80
  const pts = data.map((d, i) => {
    const x = (i / (data.length - 1)) * W
    const y = height - ((d.v - min) / range) * height
    return `${x},${y}`
  }).join(' ')
  return (
    <svg width={W} height={height} viewBox={`0 0 ${W} ${height}`} style={{ overflow: 'visible' }}>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

/* ── Stat Widget Card ─────────────────────────────────────────────────── */
function StatWidget({ icon: Icon, label, value, trend, trendLabel, sparkData, color = 'accent' }) {
  const isUp = trend > 0
  const isDown = trend < 0
  return (
    <div className={`${styles.statWidget} ${styles[color]}`}>
      <div className={styles.statWidgetTop}>
        <div className={`${styles.statIcon} ${styles[color]}`}><Icon size={16} /></div>
        {trend !== undefined && (
          <span className={`${styles.statTrend} ${isUp ? styles.up : isDown ? styles.down : styles.neutral}`}>
            {isUp ? <ArrowUpRight size={11} /> : isDown ? <ArrowDownRight size={11} /> : null}
            {Math.abs(trend)}{trendLabel || ''}
          </span>
        )}
      </div>
      <div className={styles.statValue}>{value}</div>
      <div className={styles.statLabel}>{label}</div>
      {sparkData && (
        <div className={styles.statSparkline}>
          <Sparkline data={sparkData} color={`var(--${color === 'accent' ? 'accent' : color === 'success' ? 'success' : color === 'warning' ? 'warning' : 'danger'})`} />
        </div>
      )}
    </div>
  )
}

/* ── Dashboard Page ────────────────────────────────────────────────────── */
export function DashboardPage() {
  const [agents, setAgents] = useState([])
  const [sessions, setSessions] = useState([])
  const [auditEvents, setAuditEvents] = useState([])
  const [stats, setStats] = useState({ agents_total: 0, agents_online: 0, sessions_active: 0, sessions_total: 0 })
  const [enrollment, setEnrollment] = useState(null)
  const [bootstrap, setBootstrap] = useState(null)
  const [trustBundle, setTrustBundle] = useState(null)
  const [agentActionMessage, setAgentActionMessage] = useState('')
  const [lastAgentCertBundle, setLastAgentCertBundle] = useState(null)
  const [lastAgentCertAgentId, setLastAgentCertAgentId] = useState('')
  const [authUser, setAuthUser] = useState(null)
  const [authState, setAuthState] = useState('checking')
  const [authError, setAuthError] = useState('')
  const [loginEmail, setLoginEmail] = useState('')
  const [loginPassword, setLoginPassword] = useState('')
  const [loginOTP, setLoginOTP] = useState('')
  const [userList, setUserList] = useState([])
  const [userDrafts, setUserDrafts] = useState({})
  const [newUserEmail, setNewUserEmail] = useState('')
  const [newUserPassword, setNewUserPassword] = useState('')
  const [newUserRole, setNewUserRole] = useState('viewer')
  const [newUserError, setNewUserError] = useState('')
  const [mfaPassword, setMfaPassword] = useState('')
  const [mfaSecret, setMfaSecret] = useState('')
  const [mfaUri, setMfaUri] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [mfaMessage, setMfaMessage] = useState('')
  const [trafficData] = useState(() => genSparkData(24, 1200, 600))
  const [latencyData] = useState(() => genSparkData(24, 30, 20))
  const [refreshing, setRefreshing] = useState(false)
  const [lastRefresh, setLastRefresh] = useState(new Date())
  const [loadError, setLoadError] = useState('')
  const { log } = useAppLogs()

  const onlineCount = agents.filter(a => a.online).length
  const activeSessionCount = sessions.filter(s => s.status === 'active').length
  const avgLatency = agents.filter(a => a.latency_ms > 0).length
    ? Math.round(agents.filter(a => a.latency_ms > 0).reduce((sum, a) => sum + a.latency_ms, 0) / agents.filter(a => a.latency_ms > 0).length)
    : 0

  const loadDashboard = useCallback(async (role = authUser?.role || 'admin') => {
    setRefreshing(true)
    setLoadError('')
    try {
      const [agentsRes, sessionsRes, statsRes] = await Promise.all([
        apiFetch('/api/v1/dashboard/agents'),
        apiFetch('/api/v1/dashboard/sessions'),
        apiFetch('/api/v1/dashboard/stats'),
      ])

      if (!agentsRes.ok) throw new Error(`agents:${agentsRes.status}`)
      if (!sessionsRes.ok) throw new Error(`sessions:${sessionsRes.status}`)
      if (!statsRes.ok) throw new Error(`stats:${statsRes.status}`)

      const [agentsJson, sessionsJson, statsJson] = await Promise.all([
        agentsRes.json(),
        sessionsRes.json(),
        statsRes.json(),
      ])
      let enrollmentJson = null
      let auditJson = null
      let bootstrapJson = null
      let usersJson = null
      let trustBundleJson = null

      if (role === 'admin') {
        const [enrollmentRes, auditRes, bootstrapRes, trustBundleRes] = await Promise.all([
          apiFetch('/api/v1/admin/enrollment'),
          apiFetch('/api/v1/dashboard/audit'),
          apiFetch('/api/v1/admin/bootstrap'),
          apiFetch('/api/v1/admin/trust-bundle'),
        ])
        if (enrollmentRes.ok) enrollmentJson = await enrollmentRes.json()
        if (auditRes.ok) auditJson = await auditRes.json()
        if (bootstrapRes.ok) bootstrapJson = await bootstrapRes.json()
        if (trustBundleRes.ok) trustBundleJson = await trustBundleRes.json()
      }

      let users = []
      if (role === 'admin') {
        const usersRes = await apiFetch('/api/v1/admin/users')
        if (usersRes.ok) {
          const u = await usersRes.json()
          users = u?.users || []
        }
      }

      setAgents(agentsJson?.agents || [])
      setSessions(sessionsJson?.sessions || [])
      setStats(statsJson || {})
      setAuditEvents(auditJson?.events || [])
      setUserList(users)
      setEnrollment(enrollmentJson)
      setBootstrap(bootstrapJson)
      setTrustBundle(trustBundleJson)
      setLastRefresh(new Date())
    } catch (error) {
      console.error('[dashboard] load error', error)
      setLoadError(String(error))
      setAgents([])
      setSessions([])
      setAuditEvents([])
      setUserList([])
      setTrustBundle(null)
      setLastAgentCertBundle(null)
      setLastAgentCertAgentId('')
      setUserDrafts({})
      setEnrollment(null)
      setBootstrap(null)
      setAgentActionMessage('')
      setStats({ agents_total: 0, agents_online: 0, sessions_active: 0, sessions_total: 0 })
      log('error', 'dashboard', 'failed to load dashboard data', { error: String(error) })
    } finally {
      setRefreshing(false)
    }
  }, [authUser?.role, log])

  useEffect(() => {
    let cancelled = false
    const verify = async () => {
      try {
        const response = await apiFetch('/api/v1/auth/me')
        if (!response.ok) throw new Error(`me:${response.status}`)
        const payload = await response.json()
        if (cancelled) return
        if (payload?.enabled === false) {
          setAuthUser(null)
          setAuthState('ready')
          setAuthError('')
          return
        }
        setAuthUser(payload?.user || null)
        setAuthState('ready')
        setAuthError('')
      } catch (error) {
        clearAuthToken()
        if (!cancelled) {
          setAuthUser(null)
          setAuthState('login')
          setAuthError('Please sign in to continue.')
        }
      }
    }
    verify()
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (authState !== 'ready') return undefined
    loadDashboard(authUser?.role || 'admin')
    const interval = setInterval(() => { if (authUser) loadDashboard(authUser.role) }, 30000)
    return () => clearInterval(interval)
  }, [authState, authUser, loadDashboard])

  const refresh = useCallback(() => loadDashboard(authUser?.role || 'admin'), [authUser?.role, loadDashboard])

  const handleLogin = useCallback(async (event) => {
    event.preventDefault()
    setAuthError('')
    setAuthState('checking')
    try {
      const response = await apiFetch('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email: loginEmail, password: loginPassword }),
        headers: { 'Content-Type': 'application/json' },
      })
      const payload = await response.json().catch(() => ({}))
      if (response.status === 429) throw new Error('Too many attempts — please wait.')
      if (response.status === 422) throw new Error('Missing email or password.')
      if (!response.ok) {
        if (payload?.mfa_required) { setAuthState('mfa'); return }
        throw new Error(payload?.error || `login:${response.status}`)
      }
      if (payload?.mfa_required) { setAuthState('mfa'); return }
      setAuthUser(payload?.user || null)
      setAuthState('ready')
      setAuthError('')
      if (payload?.token) setAuthToken(payload.token)
      log('info', 'auth', 'login success', { email: loginEmail })
    } catch (error) {
      setAuthError(String(error.message || error))
      setAuthState('login')
      log('error', 'auth', 'login failed', { error: String(error(error)) })
    }
  }, [loginEmail, loginPassword, log])

  const handleLogout = useCallback(() => {
    setAuthUser(null)
    setAuthState('login')
    setLoginEmail('')
    setLoginPassword('')
    setLoginOTP('')
    setMfaPassword('')
    log('info', 'auth', 'dashboard logout')
  }, [log])

  const handleMfaSetup = useCallback(async (event) => {
    event.preventDefault()
    setMfaMessage('')
    try {
      const response = await apiFetch('/api/v1/auth/mfa/setup', {
        method: 'POST',
        body: JSON.stringify({ current_password: mfaPassword }),
        headers: { 'Content-Type': 'application/json' },
      })
      const payload = await response.json().catch(() => ({}))
      if (!response.ok) throw new Error(payload?.error || `setup:${response.status}`)
      setMfaSecret(payload.secret || '')
      setMfaUri(payload.otpauth_uri || '')
      setMfaMessage('Scan the QR / add the secret in your authenticator app, then confirm with a code below.')
      log('info', 'auth', 'mfa setup started', { email: authUser?.email })
    } catch (error) {
      setMfaMessage(`MFA setup failed: ${String(error.message || error)}`)
      log('error', 'auth', 'mfa setup failed', { error: String(error) })
    }
  }, [authUser?.email, log, mfaPassword])

  const handleMfaConfirm = useCallback(async (event) => {
    event.preventDefault()
    if (!mfaSecret || !mfaCode) return
    try {
      const response = await apiFetch('/api/v1/auth/mfa/confirm', {
        method: 'POST',
        body: JSON.stringify({ secret: mfaSecret, otp_code: mfaCode }),
        headers: { 'Content-Type': 'application/json' },
      })
      const payload = await response.json().catch(() => ({}))
      if (!response.ok) throw new Error(payload?.error || `confirm:${response.status}`)
      setAuthUser(payload?.user || authUser)
      setMfaSecret('')
      setMfaUri('')
      setMfaCode('')
      setMfaMessage('Two-factor authentication enabled.')
      log('info', 'auth', 'mfa enabled', { email: authUser?.email })
    } catch (error) {
      setMfaMessage(`MFA confirmation failed: ${String(error.message || error)}`)
      log('error', 'auth', 'mfa confirmation failed', { error: String(error) })
    }
  }, [authUser, mfaCode, mfaSecret, log])

  const handleMfaDisable = useCallback(async () => {
    try {
      const response = await apiFetch('/api/v1/auth/mfa', { method: 'DELETE' })
      const payload = await response.json().catch(() => ({}))
      if (!response.ok) throw new Error(payload?.error || `disable:${response.status}`)
      setAuthUser(payload?.user || authUser)
      setMfaSecret('')
      setMfaUri('')
      setMfaCode('')
      setMfaMessage('Two-factor authentication disabled.')
      log('info', 'auth', 'mfa disabled', { email: authUser?.email })
    } catch (error) {
      setMfaMessage(`MFA disable failed: ${String(error.message || error)}`)
      log('error', 'auth', 'mfa disable failed', { error: String(error) })
    }
  }, [authUser, log])

  const handleCreateUser = useCallback(async (event) => {
    event.preventDefault()
    setNewUserError('')
    try {
      const response = await apiFetch('/api/v1/admin/users', {
        method: 'POST',
        body: JSON.stringify({ email: newUserEmail, password: newUserPassword, role: newUserRole }),
        headers: { 'Content-Type': 'application/json' },
      })
      const payload = await response.json().catch(() => ({}))
      if (!response.ok) throw new Error(payload?.error || `create:${response.status}`)
      setNewUserEmail('')
      setNewUserPassword('')
      setNewUserRole('viewer')
      log('info', 'admin', 'user created', { email: payload?.user?.email, role: payload?.user?.role })
      await loadDashboard(authUser?.role || 'admin')
    } catch (error) {
      setNewUserError(String(error.message || error))
      log('error', 'admin', 'user creation failed', { error: String(error) })
    }
  }, [authUser?.role, newUserEmail, newUserPassword, newUserRole, loadDashboard, log])

  const handleDeleteUser = useCallback(async (email) => {
    if (!email) return
    try {
      const response = await apiFetch(`/api/v1/admin/users/${encodeURIComponent(email)}`, { method: 'DELETE' })
      const payload = await response.json().catch(() => ({}))
      if (!response.ok) throw new Error(payload?.error || `delete:${response.status}`)
      log('info', 'admin', 'user deleted', { email })
      await loadDashboard(authUser?.role || 'admin')
    } catch (error) {
      log('error', 'admin', 'user deletion failed', { error: String(error) })
    }
  }, [authUser?.role, loadDashboard, log])

  /* ── Auth Gate ──────────────────────────────────────────────────────────── */
  if (authState === 'checking') {
    return (
      <div className={styles.page}>
        <div className={styles.authPlaceholder}>
          <div className="spinner" style={{ width: 32, height: 32 }} />
        </div>
      </div>
    )
  }

  if (authState === 'login' || authState === 'mfa') {
    return (
      <div className={styles.page}>
        <div className={styles.authWrap}>
          <div className={styles.authCard}>
            <div className={styles.authLogo}>
              <Zap size={22} strokeWidth={2.5} />
            </div>
            <div className={styles.authTitle}>
              <Shield size={14} />
              <span>{authState === 'mfa' ? 'Two-factor required' : 'Admin sign in'}</span>
            </div>
            <h2>Neev Remote Dashboard</h2>
            <p>{authState === 'mfa' ? 'Enter the one-time code from your authenticator app.' : 'Sign in with your company account to view devices, sessions, and rollout tools.'}</p>
            <form className={styles.authForm} onSubmit={handleLogin}>
              <label className={styles.authField}>
                <span>Email</span>
                <input type="email" value={loginEmail} onChange={(e) => setLoginEmail(e.target.value)} placeholder="admin@company.com" autoComplete="email" />
              </label>
              <label className={styles.authField}>
                <span>Password</span>
                <input type="password" value={loginPassword} onChange={(e) => setLoginPassword(e.target.value)} placeholder="••••••••" autoComplete="current-password" />
              </label>
              {authState === 'mfa' && (
                <label className={styles.authField}>
                  <span>Authenticator code</span>
                  <input type="text" value={loginOTP} onChange={(e) => setLoginOTP(e.target.value)} placeholder="123456" inputMode="numeric" autoComplete="one-time-code" />
                </label>
              )}
              {authError && <div className={styles.authError}>{authError}</div>}
              <button className={styles.authButton} type="submit" disabled={refreshing}>
                {refreshing ? 'Signing in…' : authState === 'mfa' ? 'Verify code' : 'Sign in'}
              </button>
            </form>
          </div>
        </div>
      </div>
    )
  }

  const stagger = { animate: { transition: { staggerChildren: 0.07 } } }
  const item    = { initial:{opacity:0,y:12}, animate:{opacity:1,y:0,transition:{duration:0.3}} }

  /* ── Recent Activity ─────────────────────────────────────────────────────── */
  const recentActivity = [
    { id: 1, title: 'MacBook Pro — Session ended', meta: 'Duration: 24m · 820 MB transferred', time: '10 min ago', dot: 'dotOnline' },
    { id: 2, title: 'Windows Desktop — Connected', meta: 'User: admin@DESKTOP', time: '32 min ago', dot: 'dotOnline' },
    { id: 3, title: 'Ubuntu Server — Session ended', meta: 'Duration: 1h 12m', time: '2h ago', dot: 'dotOffline' },
    { id: 4, title: 'High CPU on Windows Desktop', meta: 'Alert · 92% CPU usage', time: '1h ago', dot: 'dotWarning' },
    { id: 5, title: 'New device enrolled: Mac Mini', meta: 'Agent v1.0.0 · macOS Ventura', time: '3h ago', dot: 'dotInfo' },
  ]

  /* ── AI Recommendations ──────────────────────────────────────────────────── */
  const aiRecs = [
    { id: 1, title: 'High memory pressure on Windows Desktop', desc: 'Chrome consuming 4.2 GB RAM. Consider restarting browser.', priority: 'high' },
    { id: 2, title: 'Disk space critically low on Ubuntu Server', desc: '/dev/sda1 at 91% capacity. Archive old logs.', priority: 'high' },
    { id: 3, title: 'Schedule maintenance window for Mac Mini', desc: 'Device has been offline 3 days. Agent update available.', priority: 'med' },
    { id: 4, title: 'Network latency spike detected', desc: 'Ubuntu Server avg RTT increased 40% over last hour.', priority: 'med' },
  ]

  return (
    <div className={styles.page}>

      {/* ── New Premium Header ─────────────────────────────────────────── */}
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <div className={styles.greeting}>Good {new Date().getHours() < 12 ? 'morning' : new Date().getHours() < 17 ? 'afternoon' : 'evening'}</div>
          <h1 className={styles.title}>Operations Dashboard</h1>
          <p className={styles.subtitle}>Last updated {lastRefresh.toLocaleTimeString()}{authUser ? ` · ${authUser.email}` : ''}</p>
          {loadError && <p style={{ fontSize: 12, color: 'var(--danger)', marginTop: 4 }}>{loadError}</p>}
          {agentActionMessage && <p className={styles.subtitle}>{agentActionMessage}</p>}
        </div>
        <div className={styles.headerActions}>
          <button className={styles.refreshBtn} onClick={refresh} disabled={refreshing}>
            <RefreshCw size={14} className={refreshing ? styles.spinning : ''} />
            {refreshing ? 'Refreshing…' : 'Refresh'}
          </button>
          {authUser && <button className={styles.logoutBtn} onClick={handleLogout}>Logout</button>}
        </div>
      </div>

      {/* ── Stat Widgets ────────────────────────────────────────────────── */}
      <div className={styles.statsGrid}>
        <StatWidget icon={Server}      label="Total Devices"     value={stats.agents_total || agents.length} trend={3}  trendLabel="%"  sparkData={genSparkData(12, 24, 8)}   color="accent"  />
        <StatWidget icon={Circle}      label="Online Now"        value={onlineCount}                      trend={1}           sparkData={genSparkData(12, 3, 2)}    color="success" />
        <StatWidget icon={Activity}    label="Active Sessions"   value={activeSessionCount}              trend={2}           sparkData={genSparkData(12, 2, 1)}    color="warning" />
        <StatWidget icon={Zap}         label="Avg Latency"       value={`${avgLatency}ms`}               trend={-5}  trendLabel="ms"                     color="danger"  />
      </div>

      {/* ── Main Content Grid ───────────────────────────────────────────── */}
      <div className={styles.mainGrid}>

        {/* Device Health */}
        <div className={styles.card}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle}>Device Health</span>
            <button className={styles.cardAction}>View all</button>
          </div>
          <div className={styles.healthScore}>
            <div className={styles.healthRing}>
              <svg width="56" height="56" viewBox="0 0 56 56">
                <circle cx="28" cy="28" r="22" fill="none" stroke="var(--bg-tertiary)" strokeWidth="5" />
                <circle
                  cx="28" cy="28" r="22" fill="none"
                  stroke="var(--success)"
                  strokeWidth="5"
                  strokeLinecap="round"
                  strokeDasharray={`${2 * Math.PI * 22}`}
                  strokeDashoffset={`${2 * Math.PI * 22 * (1 - 0.85)}`}
                />
              </svg>
            </div>
            <div>
              <div className={styles.healthScoreVal}>85%</div>
              <div className={styles.healthScoreLabel}>Overall health score</div>
            </div>
            <div className={styles.healthBreakdown}>
              <div className={styles.healthRow}><span className={styles.healthRowLabel}>Online</span><span className={styles.healthRowVal} style={{ color: 'var(--success)' }}>{onlineCount}</span></div>
              <div className={styles.healthRow}><span className={styles.healthRowLabel}>Offline</span><span className={styles.healthRowVal} style={{ color: 'var(--text-muted)' }}>{agents.length - onlineCount}</span></div>
              <div className={styles.healthRow}><span className={styles.healthRowLabel}>At risk</span><span className={styles.healthRowVal} style={{ color: 'var(--warning)' }}>1</span></div>
            </div>
          </div>
        </div>

        {/* Recent Activity */}
        <div className={styles.card}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle}>Recent Activity</span>
            <button className={styles.cardAction}>View all</button>
          </div>
          <div className={styles.activityList}>
            {recentActivity.map(a => (
              <div key={a.id} className={styles.activityItem}>
                <span className={`${styles.activityDot} ${styles[a.dot]}`} />
                <div className={styles.activityContent}>
                  <div className={styles.activityTitle}>{a.title}</div>
                  <div className={styles.activityMeta}>
                    <span>{a.meta}</span>
                    <span className={styles.activityTime}>{a.time}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* AI Recommendations */}
        <div className={styles.card}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <Sparkles size={14} style={{ color: 'var(--accent)' }} />
              AI Recommendations
            </span>
            <button className={styles.cardAction}>View all</button>
          </div>
          <div className={styles.aiList}>
            {aiRecs.map(r => (
              <div key={r.id} className={styles.aiItem}>
                <div className={styles.aiIcon}><AlertTriangle size={13} /></div>
                <div className={styles.aiContent}>
                  <div className={styles.aiTitle}>{r.title}</div>
                  <div className={styles.aiDesc}>{r.desc}</div>
                </div>
                <span className={`${styles.aiPriority} ${r.priority === 'high' ? styles.priorityHigh : r.priority === 'med' ? styles.priorityMed : styles.priorityLow}`}>
                  {r.priority}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* ── Existing Charts + Sessions (below) ─────────────────────────── */}
      {/* KPI cards */}
      <motion.div className={styles.kpiGrid} variants={stagger} initial="initial" animate="animate">
        <KPICard icon={<Server size={18} />}  label="Total Agents"      value={stats.agents_total || agents.length} sub={`${onlineCount} online`}       color="blue"    variants={item} />
        <KPICard icon={<Circle size={18} />}  label="Online"            value={onlineCount}            sub="connected now"                  color="green"   variants={item} />
        <KPICard icon={<Users size={18} />}   label="Active Sessions"   value={activeSessionCount}     sub="in progress"                    color="purple"  variants={item} />
        <KPICard icon={<Zap size={18} />}     label="Avg Latency"       value={`${avgLatency}ms`}      sub="across online agents"           color="yellow"  variants={item} />
      </motion.div>

      {/* Charts row */}
      <div className={styles.chartRow}>
        <div className={styles.chartCard}>
          <div className={styles.chartHeader}>
            <TrendingUp size={15} />
            <span>Bandwidth (Kbps)</span>
            <span className={styles.chartBadge}>24h</span>
          </div>
          <ResponsiveContainer width="100%" height={130}>
            <AreaChart data={trafficData} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
              <defs>
                <linearGradient id="bwGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor="#3b82f6" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#3b82f6" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
              <XAxis dataKey="t" tick={false} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 10, fill: 'var(--text-muted)' }} axisLine={false} tickLine={false} width={30} />
              <Tooltip
                contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }}
                labelStyle={{ color: 'var(--text-muted)' }}
                itemStyle={{ color: 'var(--text-primary)' }}
              />
              <Area type="monotone" dataKey="v" stroke="#3b82f6" fill="url(#bwGrad)" strokeWidth={2} dot={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>

        <div className={styles.chartCard}>
          <div className={styles.chartHeader}>
            <Clock size={15} />
            <span>Latency (ms)</span>
            <span className={styles.chartBadge}>24h</span>
          </div>
          <ResponsiveContainer width="100%" height={130}>
            <AreaChart data={latencyData} margin={{ top: 4, right: 4, left: -20, bottom: 0 }}>
              <defs>
                <linearGradient id="latGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%"  stopColor="#22c55e" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#22c55e" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
              <XAxis dataKey="t" tick={false} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 10, fill: 'var(--text-muted)' }} axisLine={false} tickLine={false} width={30} />
              <Tooltip
                contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }}
                labelStyle={{ color: 'var(--text-muted)' }}
                itemStyle={{ color: 'var(--text-primary)' }}
              />
              <Area type="monotone" dataKey="v" stroke="#22c55e" fill="url(#latGrad)" strokeWidth={2} dot={false} />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Device + Session rows */}
      <div className={styles.bottomRow}>
        <div className={styles.card} style={{ flex: 1 }}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle}>Devices</span>
            <button className={styles.cardAction} onClick={() => window.location.href = '/devices'}>Manage →</button>
          </div>
          {agents.slice(0, 5).map(agent => (
            <div key={agent.id || agent.agent_id} className={styles.agentRow}>
              <span className={`${styles.agentDot} ${agent.online ? styles.agentOnline : styles.agentOffline}`} />
              <span className={styles.agentName}>{agent.hostname || agent.name || 'Unknown'}</span>
              <span className={styles.agentId}>{agent.id || agent.agent_id || ''}</span>
              <span className={styles.agentStatus}>{agent.online ? 'Online' : 'Offline'}</span>
            </div>
          ))}
          {agents.length === 0 && <p style={{ padding: '16px 20px', color: 'var(--text-muted)', fontSize: 13 }}>No devices enrolled yet.</p>}
        </div>

        <div className={styles.card} style={{ flex: 1 }}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle}>Sessions</span>
            <button className={styles.cardAction} onClick={() => window.location.href = '/sessions'}>View all →</button>
          </div>
          {sessions.slice(0, 5).map(session => (
            <div key={session.id} className={styles.sessionRow}>
              <span className={`${styles.sessionDot} ${session.status === 'active' ? styles.sessionActive : styles.sessionEnded}`} />
              <div>
                <div className={styles.sessionName}>{session.device_name || session.agent_id || 'Session'}</div>
                <div className={styles.sessionMeta}>{session.started_at ? new Date(session.started_at).toLocaleTimeString() : ''}</div>
              </div>
              <span className={`${styles.sessionBadge} ${session.status === 'active' ? styles.sessionBadgeActive : styles.sessionBadgeEnded}`}>
                {session.status}
              </span>
            </div>
          ))}
          {sessions.length === 0 && <p style={{ padding: '16px 20px', color: 'var(--text-muted)', fontSize: 13 }}>No sessions yet.</p>}
        </div>
      </div>

      {/* Admin section */}
      {authUser?.role === 'admin' && (
        <>
          <div className={styles.divider} />
          <div className={styles.adminSection}>
            <h3 className={styles.adminTitle}>Admin</h3>

            {/* Enrollment + Bootstrap */}
            <div className={styles.adminGrid}>
              <div className={styles.card}>
                <div className={styles.cardHeader}>
                  <span className={styles.cardTitle}>Device Enrollment</span>
                </div>
                <div className={styles.cardBody}>
                  {enrollment?.enrollment_link ? (
                    <div className={styles.enrollmentLink} onClick={() => { navigator.clipboard.writeText(enrollment.enrollment_link); log('info', 'admin', 'enrollment link copied') }}>
                        <code>{enrollment.enrollment_link}</code>
                        <Copy size={13} />
                      </div>
                  ) : <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>No enrollment link generated yet.</p>}
                  <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
                    <button className="btn-primary" style={{ fontSize: 12 }} onClick={async () => {
                      const res = await apiFetch('/api/v1/admin/enrollment', { method: 'POST' })
                      const json = await res.json().catch(() => ({}))
                      if (res.ok) { setEnrollment(json); log('info', 'admin', 'enrollment link generated') }
                      else log('error', 'admin', 'enrollment failed', { error: json?.error })
                    }}>Generate Link</button>
                  </div>
                </div>
              </div>

              <div className={styles.card}>
                <div className={styles.cardHeader}>
                  <span className={styles.cardTitle}>Bootstrap Token</span>
                </div>
                <div className={styles.cardBody}>
                  {bootstrap?.token ? (
                    <div className={styles.enrollmentLink} onClick={() => { navigator.clipboard.writeText(bootstrap.token); log('info', 'admin', 'bootstrap token copied') }}>
                      <code style={{ fontSize: 11 }}>{bootstrap.token.slice(0, 24)}…</code>
                      <Copy size={13} />
                    </div>
                  ) : <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>No bootstrap token. Generate one to provision agents.</p>}
                  <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
                    <button className="btn-primary" style={{ fontSize: 12 }} onClick={async () => {
                      const res = await apiFetch('/api/v1/admin/bootstrap', { method: 'POST' })
                      const json = await res.json().catch(() => ({}))
                      if (res.ok) { setBootstrap(json); log('info', 'admin', 'bootstrap token generated') }
                      else log('error', 'admin', 'bootstrap failed', { error: json?.error })
                    }}>Generate Token</button>
                    {bootstrap?.token && (
                      <button className="btn-danger" style={{ fontSize: 12 }} onClick={async () => {
                        await apiFetch('/api/v1/admin/bootstrap', { method: 'DELETE' })
                        setBootstrap(null)
                        log('info', 'admin', 'bootstrap token revoked')
                      }}>Revoke</button>
                    )}
                  </div>
                </div>
              </div>

              <div className={styles.card}>
                <div className={styles.cardHeader}>
                  <span className={styles.cardTitle}>Trust Bundle</span>
                </div>
                <div className={styles.cardBody}>
                  {trustBundle?.bundle_url ? (
                    <p style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                      Expiry: {trustBundle?.not_after ? new Date(trustBundle.not_after).toLocaleDateString() : 'unknown'}
                    </p>
                  ) : <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>No trust bundle uploaded.</p>}
                  <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
                    <button className="btn-primary" style={{ fontSize: 12 }} onClick={async () => {
                      const file = window.prompt('Paste the base64-encoded PEM trust bundle:')
                      if (!file) return
                      const res = await apiFetch('/api/v1/admin/trust-bundle', {
                        method: 'POST',
                        body: JSON.stringify({ bundle_pem: file }),
                        headers: { 'Content-Type': 'application/json' },
                      })
                      const json = await res.json().catch(() => ({}))
                      if (res.ok) { setTrustBundle(json); log('info', 'admin', 'trust bundle uploaded') }
                      else log('error', 'admin', 'trust bundle failed', { error: json?.error })
                    }}>Upload Bundle</button>
                  </div>
                </div>
              </div>
            </div>

            {/* User Management */}
            <div className={styles.card} style={{ marginTop: 16 }}>
              <div className={styles.cardHeader}>
                <span className={styles.cardTitle}>Team Members</span>
                <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{userList.length} user{userList.length !== 1 ? 's' : ''}</span>
              </div>
              <div>
                {userList.map(u => (
                  <div key={u.email} className={styles.userRow}>
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600 }}>{u.email}</div>
                      <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>{u.role} · {u.mfa_enabled ? '🔥 MFA on' : 'MFA off'}</div>
                    </div>
                    <div style={{ display: 'flex', gap: 6, marginLeft: 'auto' }}>
                      {u.email !== authUser?.email && (
                        <button className="btn-danger" style={{ fontSize: 11, padding: '3px 10px' }} onClick={() => handleDeleteUser(u.email)}>Remove</button>
                      )}
                    </div>
                  </div>
                ))}
                <form className={styles.userForm} onSubmit={handleCreateUser}>
                  <input type="email" placeholder="New user email" value={newUserEmail} onChange={e => setNewUserEmail(e.target.value)} style={{ flex: 1 }} />
                  <input type="password" placeholder="Password" value={newUserPassword} onChange={e => setNewUserPassword(e.target.value)} style={{ width: 140 }} />
                  <select value={newUserRole} onChange={e => setNewUserRole(e.target.value)} style={{ width: 110 }}>
                    <option value="viewer">Viewer</option>
                    <option value="admin">Admin</option>
                  </select>
                  <button type="submit" className="btn-primary" style={{ fontSize: 12 }}>Invite</button>
                </form>
                {newUserError && <p style={{ padding: '8px 20px', color: 'var(--danger)', fontSize: 12 }}>{newUserError}</p>}
              </div>
            </div>

            {/* MFA */}
            <div className={styles.card} style={{ marginTop: 16 }}>
              <div className={styles.cardHeader}>
                <span className={styles.cardTitle}>Two-Factor Authentication</span>
                <span className="badge badge-accent">Recommended</span>
              </div>
              <div className={styles.cardBody}>
                {authUser?.mfa_enabled ? (
                  <div>
                    <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 12 }}>MFA is active on your account.</p>
                    <button className="btn-danger" style={{ fontSize: 12 }} onClick={handleMfaDisable}>Disable MFA</button>
                  </div>
                ) : (
                  <div>
                    <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 12 }}>
                      Protect your account with an authenticator app.
                    </p>
                    {mfaUri && (
                      <div style={{ marginBottom: 12 }}>
                        <img src={mfaUri} alt="MFA QR" style={{ display: 'block', width: 160, marginBottom: 8, borderRadius: 8, border: '1px solid var(--border)' }} />
                        <code style={{ fontSize: 11, wordBreak: 'break-all', color: 'var(--text-secondary)' }}>{mfaSecret}</code>
                      </div>
                    )}
                    {mfaMessage && <p style={{ fontSize: 12, color: mfaMessage.includes('failed') ? 'var(--danger)' : 'var(--success)', marginBottom: 8 }}>{mfaMessage}</p>}
                    {!mfaSecret && (
                      <form onSubmit={handleMfaSetup} style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
                        <input type="password" placeholder="Current password" value={mfaPassword} onChange={e => setMfaPassword(e.target.value)} style={{ width: 180 }} />
                        <button type="submit" className="btn-primary" style={{ fontSize: 12 }}>Setup MFA</button>
                      </form>
                    )}
                    {mfaSecret && (
                      <form onSubmit={handleMfaConfirm} style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
                        <input type="text" placeholder="123456" value={mfaCode} onChange={e => setMfaCode(e.target.value)} style={{ width: 100 }} inputMode="numeric" />
                        <button type="submit" className="btn-primary" style={{ fontSize: 12 }}>Confirm</button>
                        <button type="button" className="btn-ghost" style={{ fontSize: 12 }} onClick={() => { setMfaSecret(''); setMfaCode('') }}>Cancel</button>
                      </form>
                    )}
                  </div>
                )}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

/* ── KPICard (kept from original for backward compat) ─────────────────── */
function KPICard({ icon, label, value, sub, color, variants }) {
  const colorMap = { blue: 'var(--accent)', green: 'var(--success)', purple: '#a855f7', yellow: 'var(--warning)', red: 'var(--danger)' }
  return (
    <motion.div className={styles.kpiCard} variants={variants} style={{ '--kpi-accent': colorMap[color] || colorMap.blue }}>
      <div className={styles.kpiIconWrap} style={{ background: `${colorMap[color] || colorMap.blue}18`, color: colorMap[color] || colorMap.blue }}>
        {icon}
      </div>
      <div className={styles.kpiValue}>{value}</div>
      <div className={styles.kpiLabel}>{label}</div>
      <div className={styles.kpiSub}>{sub}</div>
    </motion.div>
  )
}