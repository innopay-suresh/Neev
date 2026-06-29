import React, { useState, useMemo, useEffect, useCallback } from 'react'
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid,
  PieChart, Pie, Cell, Legend
} from 'recharts'
import { jsPDF } from 'jspdf'
import {
  Download, Monitor, Activity, Server, Users, FileText, RefreshCw,
  ChevronUp, ChevronDown
} from 'lucide-react'
import { apiFetch } from '../lib/api.js'
import styles from './AnalyticsPage.module.css'

/* ── Mini Sparkline ─────────────────────────────────────────────────────────── */
function MiniSparkline({ data, color }) {
  if (!data || data.length < 2) return <svg width={120} height={40} />
  const max = Math.max(...data)
  const min = Math.min(...data)
  const range = max - min || 1
  const W = 120, H = 40
  const pts = data.map((v, i) => {
    const x = (i / (data.length - 1)) * W
    const y = H - ((v - min) / range) * H
    return `${x},${y}`
  }).join(' ')
  return (
    <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} style={{ overflow: 'visible' }}>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function CustomTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null
  return (
    <div className={styles.tooltip}>
      <div className={styles.tooltipLabel}>{label}</div>
      {payload.map((p, i) => (
        <div key={i} className={styles.tooltipRow} style={{ color: p.color }}>
          <span>{p.name}</span><span>{p.value}</span>
        </div>
      ))}
    </div>
  )
}

const OUTCOME_COLORS = { Accepted: '#22C55E', Denied: '#EF4444', 'Rate limited': '#F59E0B' }
const HEALTH_COLORS = { Online: '#22C55E', Offline: '#94A3B8' }

function withColor(arr, map) {
  return (arr || []).map(d => ({ ...d, color: map[d.name] || '#4F8CFF' }))
}

/* ── Top Devices Table ──────────────────────────────────────────────────────── */
function TopDevicesTable({ data, sortDir, onSort }) {
  return (
    <div className={styles.topTable}>
      <div className={styles.topTableHead}>
        <span className={styles.colDevice}>Device</span>
        <span className={`${styles.sortable} ${styles.sortActive}`} onClick={onSort}>
          Sessions {sortDir === 'asc' ? <ChevronUp size={11} /> : <ChevronDown size={11} />}
        </span>
      </div>
      {data.length === 0 ? (
        <div className={styles.topTableRow} style={{ color: 'var(--text-muted)' }}>
          <span className={styles.colDevice}>No session history yet</span><span />
        </div>
      ) : data.map((d, i) => (
        <div key={d.name + i} className={styles.topTableRow}>
          <span className={styles.colDevice}>
            <Monitor size={12} className={styles.deviceIcon} /> {d.name}
          </span>
          <span className={styles.colNum}>{d.sessions}</span>
        </div>
      ))}
    </div>
  )
}

const EMPTY = { trend: [], top_devices: [], outcomes: [], health: [], summary: {} }

/* ── Main AnalyticsPage ─────────────────────────────────────────────────────── */
export function AnalyticsPage() {
  const [data, setData] = useState(EMPTY)
  const [sortDir, setSortDir] = useState('desc')

  const load = useCallback(async () => {
    try {
      const res = await apiFetch('/api/v1/dashboard/analytics')
      if (!res.ok) return
      setData(await res.json())
    } catch { /* keep last */ }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [load])

  const trend = data.trend || []
  const summary = data.summary || {}
  const outcomes = useMemo(() => withColor(data.outcomes, OUTCOME_COLORS), [data.outcomes])
  const health = useMemo(() => withColor(data.health, HEALTH_COLORS), [data.health])

  const sortedDevices = useMemo(() => {
    const list = [...(data.top_devices || [])]
    list.sort((a, b) => sortDir === 'asc' ? a.sessions - b.sessions : b.sessions - a.sessions)
    return list
  }, [data.top_devices, sortDir])

  const sparkSessions = trend.slice(-12).map(d => d.sessions)
  const totalSessions = (data.top_devices || []).reduce((s, d) => s + d.sessions, 0)

  const stats = [
    { label: 'Active Devices',  value: summary.active_devices ?? 0, icon: Monitor,  colorVar: '--accent'  },
    { label: 'Total Devices',   value: summary.total_devices ?? 0,  icon: Server,   colorVar: '--success' },
    { label: 'Sessions Today',  value: summary.sessions_today ?? 0,  icon: Activity, colorVar: '--warning' },
    { label: 'Total Sessions',  value: summary.sessions_total ?? 0,  icon: Users,    colorVar: '#4F8CFF'   },
  ]

  const exportCSV = () => {
    const rows = [['Device', 'Sessions'], ...sortedDevices.map(d => [d.name, d.sessions])]
    const csv = rows.map(r => r.join(',')).join('\n')
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }))
    const a = document.createElement('a')
    a.href = url; a.download = `neev-analytics-${new Date().toISOString().slice(0, 10)}.csv`; a.click()
    URL.revokeObjectURL(url)
  }

  const exportPDF = () => {
    const doc = new jsPDF()
    doc.setFontSize(18); doc.text('Neev Remote — Analytics Report', 14, 20)
    doc.setFontSize(10); doc.setTextColor(100)
    doc.text(new Date().toLocaleString(), 14, 28)
    doc.setTextColor(0); doc.setFontSize(11)
    doc.text(`Active devices: ${summary.active_devices ?? 0}   Sessions today: ${summary.sessions_today ?? 0}   Total sessions: ${summary.sessions_total ?? 0}`, 14, 40)
    doc.setFontSize(13); doc.text('Most Active Devices', 14, 54)
    let y = 62; doc.setFontSize(10)
    sortedDevices.forEach((d, i) => { doc.text(`${i + 1}. ${d.name} — ${d.sessions} sessions`, 14, y); y += 7 })
    doc.save(`neev-analytics-${new Date().toISOString().slice(0, 10)}.pdf`)
  }

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Analytics</h1>
          <p className="page-subtitle">Live usage from your server</p>
        </div>
        <div className={styles.exportBar}>
          <button className="btn-secondary" onClick={load}><RefreshCw size={14} /> Refresh</button>
          <button className="btn-secondary" onClick={exportCSV}><Download size={14} /> CSV</button>
          <button className="btn-secondary" onClick={exportPDF}><FileText size={14} /> PDF</button>
        </div>
      </div>

      {/* Stat cards */}
      <div className={styles.statsGrid}>
        {stats.map(({ label, value, icon: Icon, colorVar }) => {
          const color = colorVar.startsWith('--') ? `var(${colorVar})` : colorVar
          return (
            <div key={label} className={`card ${styles.statCard}`}>
              <div className={styles.statTop}>
                <span className={styles.statIcon} style={{ color }}><Icon size={14} /></span>
              </div>
              <div className={styles.statValue}>{value}</div>
              <div className={styles.statLabel}>{label}</div>
              <MiniSparkline data={sparkSessions} color={color} />
            </div>
          )
        })}
      </div>

      {/* Session volume trend */}
      <div className={`card ${styles.chartCard}`}>
        <div className="section-header">
          <span className="section-title">Session Volume — Last 30 Days</span>
          <span className={styles.badge}>30d</span>
        </div>
        <div className={styles.areaChartWrap}>
          <ResponsiveContainer width="100%" height={220}>
            <AreaChart data={trend} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
              <defs>
                <linearGradient id="sessionsGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#4F8CFF" stopOpacity={0.25} />
                  <stop offset="95%" stopColor="#4F8CFF" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
              <XAxis dataKey="label" tick={{ fontSize: 11, fill: 'var(--text-muted)' }} tickLine={false} axisLine={{ stroke: 'var(--border)' }} interval={4} />
              <YAxis allowDecimals={false} tick={{ fontSize: 11, fill: 'var(--text-muted)' }} tickLine={false} axisLine={false} />
              <Tooltip content={<CustomTooltip />} />
              <Area type="monotone" dataKey="sessions" stroke="#4F8CFF" fill="url(#sessionsGrad)" strokeWidth={2} dot={false} name="Sessions" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Outcomes + Health */}
      <div className={styles.pieRow}>
        {[
          { title: 'Connection Outcomes', data: outcomes },
          { title: 'Device Health', data: health },
        ].map(({ title, data: pie }) => {
          const total = pie.reduce((s, d) => s + (d.value || 0), 0)
          return (
            <div key={title} className={`card ${styles.pieCard}`}>
              <div className="section-header"><span className="section-title">{title}</span></div>
              <div className={styles.pieWrap}>
                {total === 0 ? (
                  <div style={{ height: 200, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)', fontSize: 13 }}>
                    No data yet
                  </div>
                ) : (
                  <ResponsiveContainer width="100%" height={200}>
                    <PieChart>
                      <Pie data={pie} cx="50%" cy="50%" innerRadius={55} outerRadius={85} paddingAngle={3} dataKey="value">
                        {pie.map((entry, i) => <Cell key={i} fill={entry.color} />)}
                      </Pie>
                      <Tooltip
                        formatter={(v, name) => [v, name]}
                        contentStyle={{ background: 'var(--bg-primary)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }}
                      />
                      <Legend formatter={(value) => <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{value}</span>} iconType="circle" iconSize={8} />
                    </PieChart>
                  </ResponsiveContainer>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* Top devices */}
      <div className={`card ${styles.tableCard}`}>
        <div className="section-header">
          <div>
            <span className="section-title">Most Active Devices</span>
            <div className={styles.tableMeta}><span>{totalSessions} total connections</span></div>
          </div>
        </div>
        <TopDevicesTable data={sortedDevices} sortDir={sortDir} onSort={() => setSortDir(d => d === 'asc' ? 'desc' : 'asc')} />
      </div>
    </div>
  )
}
