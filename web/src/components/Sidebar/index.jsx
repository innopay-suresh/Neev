import React, { useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Monitor,
  ScreenShare,
  Download,
  Clock,
  Shield,
  Sparkles,
  BarChart2,
  Settings,
  ChevronLeft,
  ChevronRight,
  Zap,
  Users,
  Building2,
} from 'lucide-react'
import styles from './Sidebar.module.css'

const NAV_ITEMS = [
  { to: '/dashboard', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/devices',   icon: Monitor,          label: 'Devices'    },
  { to: '/remote',    icon: ScreenShare,      label: 'Remote Access' },
  { to: '/downloads', icon: Download,         label: 'Downloads'   },
  { to: '/sessions',  icon: Clock,            label: 'Sessions'   },
  { to: '/security',   icon: Shield,           label: 'Security'     },
  { to: '/ai',        icon: Sparkles,         label: 'AI Assistant'},
  { to: '/teams',     icon: Users,            label: 'Teams'       },
  { to: '/enterprise', icon: Building2,       label: 'Enterprise'   },
  { to: '/analytics', icon: BarChart2,        label: 'Analytics'   },
  { to: '/settings',  icon: Settings,         label: 'Settings'   },
]

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(false)
  const location = useLocation()

  return (
    <aside className={`${styles.sidebar} ${collapsed ? styles.collapsed : ''}`}>
      {/* Logo */}
      <div className={styles.logo}>
        <div className={styles.logoIcon} style={{ background: 'transparent' }}>
          <img src="/favicon.png" alt="Neev Remote" style={{ width: '22px', height: '22px', display: 'block' }} />
        </div>
        {!collapsed && (
          <span className={styles.logoText}>Neev Remote</span>
        )}
      </div>

      {/* Navigation */}
      <nav className={styles.nav}>
        {NAV_ITEMS.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            className={({ isActive }) =>
              `${styles.navItem} ${isActive ? styles.active : ''}`
            }
            title={collapsed ? label : undefined}
          >
            <span className={styles.navIcon}>
              <Icon size={18} strokeWidth={1.8} />
            </span>
            {!collapsed && <span className={styles.navLabel}>{label}</span>}
            {location.pathname === to && (
              <span className={styles.activePip} />
            )}
          </NavLink>
        ))}
      </nav>

      {/* Collapse toggle */}
      <button
        className={styles.collapseBtn}
        onClick={() => setCollapsed(c => !c)}
        title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      >
        {collapsed
          ? <ChevronRight size={15} />
          : <ChevronLeft size={15} />
        }
        {!collapsed && <span>Collapse</span>}
      </button>
    </aside>
  )
}