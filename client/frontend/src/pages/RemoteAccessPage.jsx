import React from 'react'
import { ScreenShare, ArrowRight, Clock, Activity } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import styles from './RemoteAccessPage.module.css'

const RECENTS = [
  { id: '000111222', name: 'MacBook Pro 16"', os: 'macOS', last: '10 min ago', status: 'online' },
  { id: '333444555', name: 'Windows Desktop', os: 'Windows', last: '32 min ago', status: 'online' },
  { id: '666777888', name: 'Ubuntu Server', os: 'Linux', last: '2h ago', status: 'offline' },
]

export function RemoteAccessPage() {
  const navigate = useNavigate()

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Remote Access</h1>
          <p className="page-subtitle">Connect to a remote device using its Agent ID</p>
        </div>
      </div>

      <div className={styles.layout}>
        {/* Connect form */}
        <div className={`card ${styles.connectCard}`}>
          <div className={styles.connectIcon}>
            <ScreenShare size={20} />
          </div>
          <h2 className={styles.connectTitle}>Connect to Device</h2>
          <p className={styles.connectDesc}>Enter the 9-digit Agent ID shown on the remote device.</p>
          <input
            type="text"
            className={styles.idInput}
            placeholder="000-000-000"
            maxLength={11}
            onKeyDown={e => {
              if (e.key === 'Enter') navigate('/connect')
            }}
          />
          <button className="btn-primary" style={{ width: '100%', justifyContent: 'center', marginTop: 'var(--space-4)', padding: '10px' }}>
            Connect <ArrowRight size={14} />
          </button>
        </div>

        {/* Recent devices */}
        <div className={`card ${styles.recentsCard}`}>
          <div className="section-header">
            <span className="section-title">Recent Connections</span>
          </div>
          <div className={styles.recentsList}>
            {RECENTS.map(d => (
              <div key={d.id} className={styles.recentItem}>
                <div className={styles.recentLeft}>
                  <span className={`${styles.recentStatus} ${d.status === 'online' ? styles.online : styles.offline}`} />
                  <div>
                    <div className={styles.recentName}>{d.name}</div>
                    <div className={styles.recentId}>{d.id}</div>
                  </div>
                </div>
                <div className={styles.recentRight}>
                  <span className={`badge ${d.status === 'online' ? 'badge-success' : 'badge-muted'}`}>
                    {d.status}
                  </span>
                  <button
                    className="btn-secondary"
                    style={{ padding: '4px 10px', fontSize: 11 }}
                    onClick={() => navigate('/connect')}
                    disabled={d.status === 'offline'}
                  >
                    Connect
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}