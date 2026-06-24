import React from 'react'
import { Clock, Activity, Download } from 'lucide-react'
import styles from './SessionsPage.module.css'

const SESSIONS = [
  { id: 's1', device: 'MacBook Pro 16"', user: 'suresh@mac', started: '10:32 AM', duration: '24m', status: 'active', type: 'remote' },
  { id: 's2', device: 'Windows Desktop', user: 'admin@DESKTOP', started: '09:15 AM', duration: '1h 41m', status: 'active', type: 'file_transfer' },
  { id: 's3', device: 'Ubuntu Server', user: 'root', started: 'Yesterday 3:00 PM', duration: '—', status: 'ended', type: 'remote' },
  { id: 's4', device: 'Mac Mini', user: 'office@mac', started: 'Yesterday 11:00 AM', duration: '38m', status: 'ended', type: 'remote' },
]

export function SessionsPage() {
  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Sessions</h1>
          <p className="page-subtitle">2 active · 2 completed</p>
        </div>
        <button className="btn-secondary"><Download size={14} /> Export Log</button>
      </div>

      <div className={styles.list}>
        {SESSIONS.map(s => (
          <div key={s.id} className={`card ${styles.row}`}>
            <div className={styles.rowLeft}>
              <div className={`${styles.typeIcon} ${s.status === 'active' ? styles.active : ''}`}>
                <Activity size={14} />
              </div>
              <div>
                <div className={styles.device}>{s.device}</div>
                <div className={styles.meta}>{s.user}</div>
              </div>
            </div>
            <div className={styles.rowMid}>
              <Clock size={12} />
              <span>{s.started}</span>
              <span className={styles.duration}>{s.duration}</span>
            </div>
            <div className={styles.rowRight}>
              <span className={`badge ${s.status === 'active' ? 'badge-success' : 'badge-muted'}`}>
                {s.status === 'active' ? '● Active' : 'Ended'}
              </span>
              {s.status === 'active'
                ? <button className="btn-danger" style={{ padding: '4px 12px', fontSize: 12 }}>End Session</button>
                : <button className="btn-secondary" style={{ padding: '4px 12px', fontSize: 12 }}>View Report</button>
              }
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}