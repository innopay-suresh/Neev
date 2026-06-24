import React from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Monitor, LayoutDashboard, FileText, Download } from 'lucide-react'
import styles from './NavBar.module.css'

export function NavBar({ logsCount = 0, onToggleLogs, logsOpen = false }) {
  const { pathname } = useLocation()
  const isActive = (path) => pathname === path ? styles.tabActive : ''

  return (
    <nav className={styles.nav}>
      <div className={styles.brand}>
        <img src="/favicon.png" alt="Neev Remote" className={styles.brandImage} />
        <span className={styles.brandName}>Neev Remote</span>
        <span className={styles.brandBadge}>Web</span>
      </div>

      <div className={styles.tabs}>
        <Link to="/viewer" className={`${styles.tab} ${isActive('/viewer')}`}>
          <Monitor size={14} />
          <span>Viewer</span>
        </Link>
        <Link to="/downloads" className={`${styles.tab} ${isActive('/downloads')}`}>
          <Download size={14} />
          <span>Downloads</span>
        </Link>
        <Link to="/dashboard" className={`${styles.tab} ${isActive('/dashboard')}`}>
          <LayoutDashboard size={14} />
          <span>Dashboard</span>
        </Link>
      </div>

      <div className={styles.right}>
        <button
          type="button"
          className={`${styles.logsBtn} ${logsOpen ? styles.logsBtnActive : ''}`}
          onClick={onToggleLogs}
        >
          <FileText size={14} />
          <span>Logs</span>
          {logsCount > 0 && <span className={styles.logsBadge}>{logsCount}</span>}
        </button>
        <div className={styles.statusDot} title="Relay connected" />
      </div>
    </nav>
  )
}
