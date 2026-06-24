import React from 'react'
import { motion } from 'framer-motion'
import {
  Monitor, Activity, Clock, Shield, Zap, Server, Circle, Users,
  TrendingUp, AlertTriangle, Sparkles, ArrowUpRight, ArrowDownRight,
  RefreshCw, Wifi, MonitorCheck
} from 'lucide-react'
import styles from './DashboardPage.module.css'

function Sparkline({ data, color = 'var(--accent)', height = 32 }) {
  const max = Math.max(...data.map(d => d.v))
  const min = Math.min(...data.map(d => d.v))
  const range = max - min || 1
  const W = 72
  const pts = data.map((d, i) => {
    const x = (i / (data.length - 1)) * W
    const y = height - ((d.v - min) / range) * height
    return `${x},${y}`
  }).join(' ')
  return (
    <svg width={W} height={height} viewBox={`0 0 ${W} ${height}`} style={{ overflow: 'visible' }}>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}

const genSpark = (n = 10, base = 100, v = 30) =>
  Array.from({ length: n }, (_, i) => ({ v: Math.max(0, base + (Math.random() - 0.5) * v) }))

function StatWidget({ icon: Icon, label, value, trend, color = 'accent' }) {
  const isUp = trend > 0
  return (
    <div className={`${styles.statWidget} ${styles[color]}`}>
      <div className={styles.statWidgetTop}>
        <div className={`${styles.statIcon} ${styles[color]}`}><Icon size={15} /></div>
        {trend !== undefined && (
          <span className={`${styles.statTrend} ${isUp ? styles.up : styles.down}`}>
            {isUp ? <ArrowUpRight size={11} /> : <ArrowDownRight size={11} />}
            {Math.abs(trend)}{trend > 0 ? '%' : ''}
          </span>
        )}
      </div>
      <div className={styles.statValue}>{value}</div>
      <div className={styles.statLabel}>{label}</div>
    </div>
  )
}

export function DashboardPage({ localAgent }) {
  const recent = [
    { id: 1, title: 'Session ended · MacBook Pro', meta: 'Duration: 24m · 820 MB', time: '10 min ago', dot: styles.dotOnline },
    { id: 2, title: 'Connected · Windows Desktop', meta: 'User: admin', time: '32 min ago', dot: styles.dotOnline },
    { id: 3, title: 'Session ended · Ubuntu Server', meta: 'Duration: 1h 12m', time: '2h ago', dot: styles.dotOffline },
    { id: 4, title: 'High CPU alert · Windows Desktop', meta: 'Alert · 92% usage', time: '1h ago', dot: styles.dotWarning },
  ]

  const aiRecs = [
    { id: 1, title: 'High memory on Windows Desktop', desc: 'Chrome consuming 4.2 GB. Restart recommended.', priority: styles.priorityHigh },
    { id: 2, title: 'Disk space low on Ubuntu Server', desc: '/dev/sda1 at 91%. Archive old logs.', priority: styles.priorityHigh },
    { id: 3, title: 'Mac Mini maintenance due', desc: 'Offline 3 days. Agent update available.', priority: styles.priorityMed },
  ]

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <div className={styles.greeting}>Overview</div>
          <h1 className={styles.title}>Dashboard</h1>
          <p className={styles.subtitle}>Remote operations at a glance</p>
        </div>
        <div className={styles.headerActions}>
          <button className={styles.refreshBtn}>
            <RefreshCw size={13} /> Refresh
          </button>
        </div>
      </div>

      <div className={styles.statsGrid}>
        <StatWidget icon={Server}    label="Total Devices"  value="6"       trend={3}   color="accent"  />
        <StatWidget icon={Circle}    label="Online Now"     value="3"       trend={1}   color="success" />
        <StatWidget icon={Activity}  label="Active Sessions" value="2"      trend={2}   color="warning" />
        <StatWidget icon={Zap}       label="Avg Latency"    value="28ms"    trend={-5}  color="danger"  />
      </div>

      <div className={styles.mainGrid}>
        {/* This device */}
        <div className={`card ${styles.card}`}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle}>This Device</span>
            <span className="badge badge-success"><Wifi size={10} /> Online</span>
          </div>
          <div className={styles.thisDevice}>
            <div className={styles.thisDeviceIcon}><MonitorCheck size={22} /></div>
            <div>
              <div className={styles.thisDeviceName}>{localAgent?.hostname || 'Loading…'}</div>
              <div className={styles.thisDeviceId}>ID: {localAgent?.id || '— — — — — —'}</div>
            </div>
          </div>
        </div>

        {/* Recent Activity */}
        <div className={`card ${styles.card}`}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle}>Recent Activity</span>
          </div>
          <div className={styles.activityList}>
            {recent.map(a => (
              <div key={a.id} className={styles.activityItem}>
                <span className={`${styles.activityDot} ${a.dot}`} />
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
        <div className={`card ${styles.card}`}>
          <div className={styles.cardHeader}>
            <span className={styles.cardTitle} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <Sparkles size={14} style={{ color: 'var(--accent)' }} />
              AI Recommendations
            </span>
          </div>
          <div className={styles.aiList}>
            {aiRecs.map(r => (
              <div key={r.id} className={styles.aiItem}>
                <div className={styles.aiIcon}><AlertTriangle size={13} /></div>
                <div className={styles.aiContent}>
                  <div className={styles.aiTitle}>{r.title}</div>
                  <div className={styles.aiDesc}>{r.desc}</div>
                </div>
                <span className={`${styles.aiPriority} ${r.priority}`}>
                  {r.priority === styles.priorityHigh ? 'high' : 'med'}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}