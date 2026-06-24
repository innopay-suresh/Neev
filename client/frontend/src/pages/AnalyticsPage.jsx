import React, { useState, useMemo } from 'react'
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid,
  PieChart, Pie, Cell, Legend, BarChart, Bar
} from 'recharts'
import { jsPDF } from 'jspdf'
import {
  Download, TrendingUp, Monitor, Clock, Activity, HardDrive,
  ArrowUp, ArrowDown, ChevronUp, ChevronDown, FileText
} from 'lucide-react'
import styles from './AnalyticsPage.module.css'

/* ── Helpers ───────────────────────────────────────────────────────────────── */
function formatSizeMB(mb) {
  if (mb >= 1000) return `${(mb / 1024).toFixed(1)} GB`
  return `${Math.round(mb)} MB`
}

/* ── Mock data generators ──────────────────────────────────────────────────── */
function generate30DayTrend() {
  const days = []
  const base = new Date()
  for (let i = 29; i >= 0; i--) {
    const d = new Date(base)
    d.setDate(d.getDate() - i)
    const label = d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
    days.push({
      label,
      sessions:    8 + Math.floor(Math.random() * 18),
      durationMin: 120 + Math.floor(Math.random() * 280),
      dataMB:      200 + Math.floor(Math.random() * 800),
    })
  }
  return days
}

const TREND_DATA = generate30DayTrend()

const CONNECTION_DATA = [
  { name: 'Direct', value: 62, color: '#22C55E' },
  { name: 'STUN',   value: 28, color: '#F59E0B' },
  { name: 'Relay',  value: 10, color: '#EF4444' },
]

const HEALTH_DATA = [
  { name: 'Healthy',  value: 73, color: '#22C55E' },
  { name: 'Warning',  value: 18, color: '#F59E0B' },
  { name: 'Critical', value:  9, color: '#EF4444' },
]

const TOP_DEVICES = [
  { id: 1, name: 'MacBook Pro 16"',         sessions: 24, totalMin: 892, dataMB: 4821 },
  { id: 2, name: 'Dell XPS 15 Developer',  sessions: 19, totalMin: 741, dataMB: 3204 },
  { id: 3, name: 'Ubuntu 22.04 Server',    sessions: 16, totalMin: 1102, dataMB: 8920 },
  { id: 4, name: 'iMac 27" Retina',        sessions: 12, totalMin: 398, dataMB: 1820 },
  { id: 5, name: 'ThinkPad X1 Carbon',     sessions: 10, totalMin: 312, dataMB: 987 },
  { id: 6, name: 'Mac Mini M2',            sessions:  8, totalMin: 245, dataMB: 634 },
  { id: 7, name: 'Windows Desktop Rig',    sessions:  7, totalMin: 521, dataMB: 4102 },
  { id: 8, name: 'Raspberry Pi Cluster',   sessions:  5, totalMin: 189, dataMB: 312 },
]

const STATS = [
  {
    label: 'Active Devices',
    value: '24',
    delta: '+3',
    icon: Monitor,
    trend: [12, 15, 14, 18, 16, 20, 22, 19, 24, 21, 23, 24],
    colorVar: '--accent',
  },
  {
    label: 'Sessions Today',
    value: '12',
    delta: '+5',
    icon: Activity,
    trend: [5, 7, 6, 8, 9, 8, 10, 11, 9, 12, 10, 12],
    colorVar: '--success',
  },
  {
    label: 'Avg Duration',
    value: '34m',
    delta: '+8m',
    icon: Clock,
    trend: [28, 30, 29, 32, 31, 33, 35, 34, 36, 33, 35, 34],
    colorVar: '--warning',
  },
  {
    label: 'Data Transferred',
    value: '2.4 GB',
    delta: '+0.8 GB',
    icon: HardDrive,
    trend: [1200, 1400, 1350, 1600, 1550, 1800, 1700, 2000, 1900, 2100, 2200, 2400],
    colorVar: '#4F8CFF',
  },
]

/* ── Mini Sparkline ─────────────────────────────────────────────────────────── */
function MiniSparkline({ data, color }) {
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

/* ── Custom Tooltip ─────────────────────────────────────────────────────────── */
function CustomTooltip({ active, payload, label }) {
  if (!active || !payload?.length) return null
  return (
    <div className={styles.chartTooltip}>
      <div className={styles.tooltipLabel}>{label}</div>
      {payload.map((p, i) => (
        <div key={i} className={styles.tooltipRow} style={{ color: p.color }}>
          <span>{p.name}</span>
          <span>{typeof p.value === 'number' && p.value > 100 ? formatSizeMB(p.value) : p.value}</span>
        </div>
      ))}
    </div>
  )
}

const PIE_COLORS = ['#22C55E', '#F59E0B', '#EF4444', '#4F8CFF', '#8B5CF6']

/* ── Export functions ───────────────────────────────────────────────────────── */
function exportCSV() {
  const rows = [
    ['Device', 'Sessions', 'Total Time (min)', 'Data (MB)'],
    ...TOP_DEVICES.map(d => [d.name, d.sessions, d.totalMin, d.dataMB]),
  ]
  const csv = rows.map(r => r.join(',')).join('\n')
  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `remote-agent-analytics-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

function exportPDF() {
  const doc = new jsPDF()
  const title = 'Neev Remote — Analytics Report'
  const date = new Date().toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' })
  doc.setFontSize(18)
  doc.text(title, 14, 20)
  doc.setFontSize(10)
  doc.setTextColor(100)
  doc.text(date, 14, 28)
  doc.setDrawColor(200)
  doc.line(14, 32, 196, 32)
  doc.setTextColor(0)
  doc.setFontSize(13)
  doc.text('Top Devices by Session Count', 14, 42)
  doc.setFontSize(10)
  const headers = [['#', 'Device', 'Sessions', 'Total Time', 'Data']]
  const dataRows = TOP_DEVICES.map((d, i) => [i + 1, d.name, d.sessions, `${d.totalMin}m`, `${d.dataMB} MB`])
  doc.autoTable({ head: headers, body: dataRows, startY: 48, styles: { fontSize: 9 } })
  const totalSessions = TOP_DEVICES.reduce((s, d) => s + d.sessions, 0)
  const totalData = TOP_DEVICES.reduce((s, d) => s + d.dataMB, 0)
  const finalY = doc.lastAutoTable.finalY + 8
  doc.text(`Total sessions: ${totalSessions}  |  Total data: ${formatSizeMB(totalData)}`, 14, finalY)
  doc.save(`remote-agent-report-${new Date().toISOString().slice(0, 10)}.pdf`)
}

/* ── Top Devices Table ──────────────────────────────────────────────────────── */
function TopDevicesTable({ data, sortKey, sortDir, onSort }) {
  return (
    <div className={styles.topTable}>
      <div className={styles.topTableHead}>
        <span className={styles.colDevice}>Device</span>
        <span className={`${styles.sortable} ${sortKey === 'sessions' ? styles.sortActive : ''}`} onClick={() => onSort('sessions')}>
          Sessions {sortKey === 'sessions' ? (sortDir === 'asc' ? <ChevronUp size={11} /> : <ChevronDown size={11} />) : null}
        </span>
        <span className={`${styles.sortable} ${sortKey === 'totalMin' ? styles.sortActive : ''}`} onClick={() => onSort('totalMin')}>
          Total Time {sortKey === 'totalMin' ? (sortDir === 'asc' ? <ChevronUp size={11} /> : <ChevronDown size={11} />) : null}
        </span>
        <span className={`${styles.sortable} ${sortKey === 'dataMB' ? styles.sortActive : ''}`} onClick={() => onSort('dataMB')}>
          Data {sortKey === 'dataMB' ? (sortDir === 'asc' ? <ChevronUp size={11} /> : <ChevronDown size={11} />) : null}
        </span>
      </div>
      {data.map((d) => (
        <div key={d.id} className={styles.topTableRow}>
          <span className={styles.colDevice}>
            <Monitor size={12} className={styles.deviceIcon} />
            {d.name}
          </span>
          <span className={styles.colNum}>{d.sessions}</span>
          <span className={styles.colNum}>{Math.floor(d.totalMin / 60)}h {d.totalMin % 60}m</span>
          <span className={styles.colNum}>{formatSizeMB(d.dataMB)}</span>
        </div>
      ))}
    </div>
  )
}

/* ── Main AnalyticsPage ─────────────────────────────────────────────────────── */
export function AnalyticsPage() {
  const [sortKey, setSortKey] = useState('sessions')
  const [sortDir, setSortDir] = useState('desc')

  const sortedDevices = useMemo(() => {
    return [...TOP_DEVICES].sort((a, b) => {
      const mult = sortDir === 'asc' ? 1 : -1
      return (a[sortKey] - b[sortKey]) * mult
    })
  }, [sortKey, sortDir])

  const handleSort = (key) => {
    if (key === sortKey) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('desc') }
  }

  const totalSessions = TOP_DEVICES.reduce((s, d) => s + d.sessions, 0)
  const totalDataMB   = TOP_DEVICES.reduce((s, d) => s + d.dataMB, 0)
  const avgDuration   = Math.round(TOP_DEVICES.reduce((s, d) => s + d.totalMin, 0) / TOP_DEVICES.length)

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Analytics</h1>
          <p className="page-subtitle">Usage trends and session insights</p>
        </div>
        <div className={styles.exportBar}>
          <button className="btn-secondary" onClick={exportCSV}>
            <Download size={14} /> CSV
          </button>
          <button className="btn-secondary" onClick={exportPDF}>
            <FileText size={14} /> PDF
          </button>
        </div>
      </div>

      {/* Stat cards */}
      <div className={styles.statsGrid}>
        {STATS.map(({ label, value, delta, icon: Icon, trend, colorVar }) => (
          <div key={label} className={`card ${styles.statCard}`}>
            <div className={styles.statTop}>
              <span className={styles.statIcon} style={{ color: `var(${colorVar.replace('var(', '').replace(')', '')})` }}>
                <Icon size={14} />
              </span>
              <span className={styles.statDelta}>{delta}</span>
            </div>
            <div className={styles.statValue}>{value}</div>
            <div className={styles.statLabel}>{label}</div>
            <MiniSparkline data={trend} color={`var(${colorVar.replace('var(', '').replace(')', '')})`} />
          </div>
        ))}
      </div>

      {/* Usage Trends */}
      <div className={`card ${styles.chartCard}`}>
        <div className="section-header">
          <span className="section-title">Session Volume — Last 30 Days</span>
          <span className={styles.badge}>30d</span>
        </div>
        <div className={styles.areaChartWrap}>
          <ResponsiveContainer width="100%" height={220}>
            <AreaChart data={TREND_DATA} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
              <defs>
                <linearGradient id="sessionsGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#4F8CFF" stopOpacity={0.25} />
                  <stop offset="95%" stopColor="#4F8CFF" stopOpacity={0.02} />
                </linearGradient>
                <linearGradient id="durationGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#22C55E" stopOpacity={0.25} />
                  <stop offset="95%" stopColor="#22C55E" stopOpacity={0.02} />
                </linearGradient>
                <linearGradient id="dataGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#F59E0B" stopOpacity={0.2} />
                  <stop offset="95%" stopColor="#F59E0B" stopOpacity={0.02} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
              <XAxis
                dataKey="label" tick={{ fontSize: 11, fill: 'var(--text-muted)' }}
                tickLine={false} axisLine={{ stroke: 'var(--border)' }}
                interval={4}
              />
              <YAxis
                yAxisId="left" orientation="left"
                tick={{ fontSize: 11, fill: 'var(--text-muted)' }}
                tickLine={false} axisLine={false}
              />
              <YAxis
                yAxisId="right" orientation="right"
                tick={{ fontSize: 11, fill: 'var(--text-muted)' }}
                tickLine={false} axisLine={false} tickFormatter={v => v > 1000 ? `${(v/1024).toFixed(0)}G` : v}
              />
              <Tooltip content={<CustomTooltip />} />
              <Area type="monotone" yAxisId="left" dataKey="sessions" stroke="#4F8CFF" fill="url(#sessionsGrad)" strokeWidth={2} dot={false} name="Sessions" />
              <Area type="monotone" yAxisId="left" dataKey="durationMin" stroke="#22C55E" fill="url(#durationGrad)" strokeWidth={2} dot={false} name="Duration (min)" />
              <Area type="monotone" yAxisId="right" dataKey="dataMB" stroke="#F59E0B" fill="url(#dataGrad)" strokeWidth={2} dot={false} name="Data (MB)" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
        <div className={styles.chartLegend}>
          <span className={styles.legendItem}><span style={{ background: '#4F8CFF' }} className={styles.legendDot} />Sessions</span>
          <span className={styles.legendItem}><span style={{ background: '#22C55E' }} className={styles.legendDot} />Duration (min)</span>
          <span className={styles.legendItem}><span style={{ background: '#F59E0B' }} className={styles.legendDot} />Data (MB)</span>
        </div>
      </div>

      {/* Connection + Health PieCharts */}
      <div className={styles.pieRow}>
        <div className={`card ${styles.pieCard}`}>
          <div className="section-header">
            <span className="section-title">Connection Type</span>
          </div>
          <div className={styles.pieWrap}>
            <ResponsiveContainer width="100%" height={200}>
              <PieChart>
                <Pie
                  data={CONNECTION_DATA}
                  cx="50%"
                  cy="50%"
                  innerRadius={55}
                  outerRadius={85}
                  paddingAngle={3}
                  dataKey="value"
                >
                  {CONNECTION_DATA.map((entry, i) => (
                    <Cell key={i} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip
                  formatter={(v, name) => [`${v}%`, name]}
                  contentStyle={{ background: 'var(--bg-primary)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }}
                />
                <Legend
                  formatter={(value) => <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{value}</span>}
                  iconType="circle" iconSize={8}
                />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className={`card ${styles.pieCard}`}>
          <div className="section-header">
            <span className="section-title">Device Health</span>
          </div>
          <div className={styles.pieWrap}>
            <ResponsiveContainer width="100%" height={200}>
              <PieChart>
                <Pie
                  data={HEALTH_DATA}
                  cx="50%"
                  cy="50%"
                  innerRadius={55}
                  outerRadius={85}
                  paddingAngle={3}
                  dataKey="value"
                >
                  {HEALTH_DATA.map((entry, i) => (
                    <Cell key={i} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip
                  formatter={(v, name) => [`${v}%`, name]}
                  contentStyle={{ background: 'var(--bg-primary)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }}
                />
                <Legend
                  formatter={(value) => <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>{value}</span>}
                  iconType="circle" iconSize={8}
                />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>

      {/* Top Devices */}
      <div className={`card ${styles.tableCard}`}>
        <div className="section-header">
          <div>
            <span className="section-title">Most Active Devices</span>
            <div className={styles.tableMeta}>
              <span>{totalSessions} total sessions</span>
              <span>·</span>
              <span>{formatSizeMB(totalDataMB)} transferred</span>
              <span>·</span>
              <span>~{avgDuration}m avg</span>
            </div>
          </div>
        </div>
        <TopDevicesTable
          data={sortedDevices}
          sortKey={sortKey}
          sortDir={sortDir}
          onSort={handleSort}
        />
      </div>
    </div>
  )
}