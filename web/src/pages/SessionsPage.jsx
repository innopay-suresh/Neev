import React, { useState, useEffect, useCallback } from 'react'
import { Clock, Activity, RefreshCw } from 'lucide-react'
import { apiFetch } from '../lib/api.js'
import styles from './SessionsPage.module.css'

function fmtTime(ts) {
  if (!ts) return '—'
  const t = Date.parse(ts)
  if (Number.isNaN(t)) return '—'
  return new Date(t).toLocaleString()
}

function duration(s) {
  const start = Date.parse(s.started_at)
  const end = s.status === 'active' ? Date.now() : Date.parse(s.ended_at)
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return '—'
  const sec = Math.floor((end - start) / 1000)
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}

export function SessionsPage() {
  const [sessions, setSessions] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      const res = await apiFetch('/api/v1/dashboard/sessions')
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setSessions(data.sessions || [])
      setError('')
    } catch {
      setError('Could not load sessions from the server.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [load])

  const active = sessions.filter(s => s.status === 'active').length
  const ended = sessions.length - active

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Sessions</h1>
          <p className="page-subtitle">{active} active · {ended} completed</p>
        </div>
        <button className="btn-secondary" onClick={load}><RefreshCw size={14} /> Refresh</button>
      </div>

      <div className={styles.list}>
        {loading ? (
          <div className="card" style={{ padding: 24, textAlign: 'center', color: 'var(--text-muted)' }}>Loading sessions…</div>
        ) : error ? (
          <div className="card" style={{ padding: 24, textAlign: 'center', color: 'var(--text-muted)' }}>{error}</div>
        ) : sessions.length === 0 ? (
          <div className="card" style={{ padding: 32, textAlign: 'center', color: 'var(--text-muted)' }}>
            No sessions yet. Connect to a device to start one.
          </div>
        ) : sessions.map(s => {
          const isActive = s.status === 'active'
          return (
            <div key={s.id} className={`card ${styles.row}`}>
              <div className={styles.rowLeft}>
                <div className={`${styles.typeIcon} ${isActive ? styles.active : ''}`}>
                  <Activity size={14} />
                </div>
                <div>
                  <div className={styles.device}>{s.agent_id || s.target_id || s.id}</div>
                  <div className={styles.meta}>{s.controller_ip || s.controller_id || '—'}</div>
                </div>
              </div>
              <div className={styles.rowMid}>
                <Clock size={12} />
                <span>{fmtTime(s.started_at)}</span>
                <span className={styles.duration}>{duration(s)}</span>
              </div>
              <div className={styles.rowRight}>
                <span className={`badge ${isActive ? 'badge-success' : 'badge-muted'}`}>
                  {isActive ? '● Active' : (s.status || 'Ended')}
                </span>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
