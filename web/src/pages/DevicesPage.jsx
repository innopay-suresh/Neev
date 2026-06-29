import React, { useState, useRef, useMemo, useCallback, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Search, ArrowUpDown, ArrowUp, ArrowDown, Clock, X, RefreshCw, Monitor
} from 'lucide-react'
import { apiFetch } from '../lib/api.js'
import styles from './DevicesPage.module.css'

const OS_ICONS = { macos: '🍎', windows: '🪟', linux: '🐧', web: '🌐' }
const osKey = (os) => (os || '').toLowerCase().split(/[\s\d]/)[0]
const osIcon = (os) => OS_ICONS[osKey(os)] || '💻'

function relativeTime(ts) {
  if (!ts) return '—'
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!t || Number.isNaN(t)) return '—'
  const s = Math.floor((Date.now() - t) / 1000)
  if (s < 10) return 'just now'
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

/** Maps a backend agent record to the row shape the table renders. */
function toDevice(a) {
  const online = !!a.status && a.status !== 'offline'
  return {
    id: a.id,
    name: a.hostname || a.id,
    hostname: a.id,
    os: a.os || 'Unknown',
    version: a.version || '—',
    group: a.device_group || a.org_id || '—',
    sessions: a.sessions || 0,
    status: online ? 'online' : 'offline',
    lastSeenRaw: a.last_seen,
    lastSeen: relativeTime(a.last_seen),
  }
}

export function DevicesPage() {
  const navigate = useNavigate()
  const parentRef = useRef(null)
  const [devices, setDevices] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [sortKey, setSortKey] = useState('name')
  const [sortDir, setSortDir] = useState('asc')
  const [selected, setSelected] = useState(null)

  const load = useCallback(async () => {
    try {
      const res = await apiFetch('/api/v1/dashboard/agents')
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setDevices((data.agents || []).map(toDevice))
      setError('')
    } catch (e) {
      setError('Could not load devices from the server.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 5000) // live status
    return () => clearInterval(t)
  }, [load])

  const filtered = useMemo(() => {
    let list = devices
    if (search) {
      const q = search.toLowerCase()
      list = list.filter(d =>
        d.name.toLowerCase().includes(q) ||
        d.hostname.toLowerCase().includes(q) ||
        d.group.toLowerCase().includes(q) ||
        d.os.toLowerCase().includes(q))
    }
    if (statusFilter !== 'all') list = list.filter(d => d.status === statusFilter)
    list = [...list].sort((a, b) => {
      let av = a[sortKey], bv = b[sortKey]
      if (typeof av === 'string') av = av.toLowerCase()
      if (typeof bv === 'string') bv = bv.toLowerCase()
      if (av < bv) return sortDir === 'asc' ? -1 : 1
      if (av > bv) return sortDir === 'asc' ? 1 : -1
      return 0
    })
    return list
  }, [devices, search, statusFilter, sortKey, sortDir])

  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 56,
    overscan: 10,
  })

  const handleSort = useCallback((key) => {
    if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('asc') }
  }, [sortKey])

  const SortIcon = ({ k }) => {
    if (sortKey !== k) return <ArrowUpDown size={12} style={{ opacity: 0.3 }} />
    return sortDir === 'asc' ? <ArrowUp size={12} /> : <ArrowDown size={12} />
  }

  const connect = useCallback((d) => {
    if (d.status === 'offline') return
    navigate(`/remote?agent=${encodeURIComponent(d.id)}`)
  }, [navigate])

  const onlineCount = devices.filter(d => d.status === 'online').length

  return (
    <div className={styles.page}>
      <div className={styles.pageHeader}>
        <div>
          <h1 className={styles.pageTitle}>Devices</h1>
          <p className={styles.pageSubtitle}>
            {devices.length} device{devices.length !== 1 ? 's' : ''} · {onlineCount} online
          </p>
        </div>
        <button className="btn-primary" onClick={load}><RefreshCw size={14} /> Refresh</button>
      </div>

      <div className={styles.toolbar}>
        <div className={styles.searchBox}>
          <Search size={14} />
          <input
            type="text"
            placeholder="Search hostname, ID, group, OS…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          {search && (
            <button className={styles.clearSearch} onClick={() => setSearch('')}><X size={13} /></button>
          )}
        </div>
        <div className={styles.filterPills}>
          <button className={`${styles.pill} ${statusFilter === 'all' ? styles.active : ''}`} onClick={() => setStatusFilter('all')}>All</button>
          <button className={`${styles.pill} ${statusFilter === 'online' ? styles.active : ''}`} onClick={() => setStatusFilter('online')}>
            <span className={styles.statusDot} style={{ background: 'var(--success)' }} /> Online
          </button>
          <button className={`${styles.pill} ${statusFilter === 'offline' ? styles.active : ''}`} onClick={() => setStatusFilter('offline')}>
            <span className={styles.statusDot} style={{ background: 'var(--text-muted)' }} /> Offline
          </button>
        </div>
        <div className={styles.resultCount}>
          {filtered.length} result{filtered.length !== 1 ? 's' : ''}
        </div>
      </div>

      <div className={styles.tableWrap}>
        <div className={styles.tableHead}>
          <div className={styles.colStatus}>Status</div>
          <div className={styles.colName}>
            <button className={styles.sortBtn} onClick={() => handleSort('name')}>Device <SortIcon k="name" /></button>
          </div>
          <div className={styles.colOs}>OS</div>
          <div className={styles.colVer}>Version</div>
          <div className={styles.colGroup}>Group</div>
          <div className={styles.colSess}>Sessions</div>
          <div className={styles.colLast}>Last Seen</div>
          <div className={styles.colActions}>Actions</div>
        </div>

        <div ref={parentRef} className={styles.tableBody}>
          {loading ? (
            <div className={styles.emptyState}>Loading devices…</div>
          ) : error ? (
            <div className={styles.emptyState}>{error}</div>
          ) : filtered.length === 0 ? (
            <div className={styles.emptyState}>
              <Monitor size={40} style={{ opacity: 0.3 }} />
              <p>No devices yet. Install the app on a machine and start sharing — it appears here.</p>
            </div>
          ) : (
            <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
              {virtualizer.getVirtualItems().map(vRow => {
                const d = filtered[vRow.index]
                return (
                  <div
                    key={d.id}
                    className={`${styles.tableRow} ${selected === d.id ? styles.selected : ''}`}
                    style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: vRow.size, transform: `translateY(${vRow.start}px)` }}
                    onClick={() => setSelected(d.id)}
                  >
                    <div className={styles.colStatus}>
                      <span className={`${styles.pip} ${d.status === 'online' ? styles.pipOnline : styles.pipOffline}`} />
                    </div>
                    <div className={styles.colName}>
                      <div className={styles.deviceIcon}>{osIcon(d.os)}</div>
                      <div>
                        <div className={styles.deviceName}>{d.name}</div>
                        <div className={styles.deviceHostname}>{d.hostname}</div>
                      </div>
                    </div>
                    <div className={styles.colOs}>{d.os}</div>
                    <div className={styles.colVer}>{d.version}</div>
                    <div className={styles.colGroup}><span className={styles.deptTag}>{d.group}</span></div>
                    <div className={styles.colSess}>{d.sessions}</div>
                    <div className={styles.colLast}>
                      <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: 'var(--text-muted)' }}>
                        <Clock size={11} /> {d.lastSeen}
                      </span>
                    </div>
                    <div className={styles.colActions}>
                      <button
                        className="btn-primary"
                        style={{ padding: '4px 10px', fontSize: 11 }}
                        disabled={d.status === 'offline'}
                        onClick={e => { e.stopPropagation(); connect(d) }}
                      >
                        Connect
                      </button>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
