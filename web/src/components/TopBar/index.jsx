import React, { useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  Search,
  Bell,
  User,
  Moon,
  Sun,
  ChevronRight,
  Command,
} from 'lucide-react'
import styles from './TopBar.module.css'

const BREADCRUMBS = {
  '/dashboard': ['Dashboard'],
  '/devices':   ['Devices'],
  '/remote':    ['Remote Access'],
  '/sessions':  ['Sessions'],
  '/security':  ['Security'],
  '/ai':        ['AI Assistant'],
  '/analytics': ['Analytics'],
  '/settings':  ['Settings'],
}

export function TopBar() {
  const location = useLocation()
  const navigate = useNavigate()
  const [theme, setTheme] = useState('dark')
  const [searchFocused, setSearchFocused] = useState(false)

  const crumbs = BREADCRUMBS[location.pathname] || [location.pathname.slice(1)]
  const isDark = theme === 'dark'

  const toggleTheme = () => {
    const next = isDark ? 'light' : 'dark'
    setTheme(next)
    document.documentElement.setAttribute('data-theme', next)
  }

  return (
    <header className={styles.topbar}>
      {/* Breadcrumb */}
      <div className={styles.breadcrumb}>
        {crumbs.map((crumb, i) => (
          <React.Fragment key={crumb}>
            {i > 0 && <ChevronRight size={12} className={styles.sep} />}
            <span className={i === crumbs.length - 1 ? styles.current : styles.parent}>
              {crumb}
            </span>
          </React.Fragment>
        ))}
      </div>

      {/* Search */}
      <div className={`${styles.searchWrap} ${searchFocused ? styles.focused : ''}`}>
        <Search size={14} className={styles.searchIcon} />
        <input
          type="text"
          placeholder="Search devices, sessions…"
          className={styles.searchInput}
          onFocus={() => setSearchFocused(true)}
          onBlur={() => setSearchFocused(false)}
        />
        <kbd className={styles.kbd}>⌘K</kbd>
      </div>

      {/* Right actions */}
      <div className={styles.actions}>
        {/* Theme toggle */}
        <button
          className={`btn-icon ${styles.iconBtn}`}
          onClick={toggleTheme}
          title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
        >
          {isDark ? <Sun size={16} /> : <Moon size={16} />}
        </button>

        {/* Notifications */}
        <button
          className={`btn-icon ${styles.iconBtn}`}
          title="Notifications"
        >
          <Bell size={16} />
          <span className={styles.notifDot} />
        </button>

        {/* User avatar */}
        <button
          className={styles.avatar}
          title="Account"
          onClick={() => navigate('/settings/account')}
        >
          <User size={14} />
        </button>
      </div>
    </header>
  )
}