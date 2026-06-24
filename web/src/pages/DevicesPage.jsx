import React, { useState, useRef, useMemo, useCallback } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  Monitor, Search, Filter, Plus, ArrowUpDown, ArrowUp, ArrowDown,
  Wifi, WifiOff, Clock, Cpu, HardDrive, MemoryStick, Tag, X
} from 'lucide-react'
import styles from './DevicesPage.module.css'

/* ── Mock data: 10,000 devices for performance testing ────────────────── */
const OS_LIST = ['macOS', 'Windows', 'Linux', 'Raspbian']
const OS_ICONS = { macOS: '🍎', Windows: '🪟', Linux: '🐧', Raspbian: '🍓' }
const MOCK_DEVICES = Array.from({ length: 10000 }, (_, i) => {
  const os = OS_LIST[Math.floor(Math.random() * OS_LIST.length)]
  const osName = os === 'macOS' ? `${os} ${['Sonoma', 'Ventura', 'Monterey'][i % 3]}` : os === 'Windows' ? `${os} ${['11 Pro', '10 Pro', '11 Enterprise'][i % 3]}` : os
  return {
    id: i + 1,
    name: ['MacBook Pro 16"', 'Windows Desktop', 'Ubuntu Server', 'Mac Mini', 'Windows Laptop', 'Raspberry Pi', 'iMac', 'Chromebook', 'Fedora Workstation', 'pop!_OS'][i % 10],
    hostname: `host-${String(i + 1).padStart(3, '0')}.local`,
    ip: `192.168.${Math.floor(i / 254)}.${(i % 254) + 1}`,
    os: osName,
    cpu: Math.floor(Math.random() * 95),
    ram: Math.floor(Math.random() * 95),
    disk: Math.floor(Math.random() * 95),
    status: Math.random() > 0.25 ? 'online' : 'offline',
    lastSeen: Math.random() > 0.5 ? 'Just now' : ['2 min ago', '5 min ago', '1h ago', '3h ago', '1 day ago'][Math.floor(Math.random() * 5)],
    agentVersion: `1.${Math.floor(Math.random() * 5)}.${Math.floor(Math.random() * 20)}`,
    uptime: Math.random() > 0.5 ? `${Math.floor(Math.random() * 30)}d ${Math.floor(Math.random() * 24)}h` : '—',
    department: ['Engineering', 'Design', 'Sales', 'IT', 'HR', 'Finance'][i % 6],
    tags: [['laptop', 'primary'], ['desktop', 'workstation'], ['server', 'production'], ['iot', 'sensor']][i % 4],
  }
})

/* ── Stat Bar ─────────────────────────────────────────────────────────── */
function StatBar({ value }) {
  const color = value > 80 ? 'var(--danger)' : value > 60 ? 'var(--warning)' : 'var(--success)'
  return (
    <div className={styles.statBar}>
      <div className={styles.statBarFill} style={{ width: `${value}%`, background: color }} />
      <span className={styles.statVal} style={{ color }}>{value}%</span>
    </div>
  )
}

/* ── Virtualized Table ─────────────────────────────────────────────────── */
export function DevicesPage() {
  const parentRef = useRef(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all') // all | online | offline
  const [osFilter, setOsFilter] = useState('all')
  const [sortKey, setSortKey] = useState('name')
  const [sortDir, setSortDir] = useState('asc')
  const [selected, setSelected] = useState(null)
  const [showTag, setShowTag] = useState(null)

  /* Filter + sort */
  const filtered = useMemo(() => {
    let list = MOCK_DEVICES
    if (search) {
      const q = search.toLowerCase()
      list = list.filter(d =>
        d.name.toLowerCase().includes(q) ||
        d.hostname.toLowerCase().includes(q) ||
        d.ip.includes(q) ||
        d.department.toLowerCase().includes(q)
      )
    }
    if (statusFilter !== 'all') list = list.filter(d => d.status === statusFilter)
    if (osFilter !== 'all') list = list.filter(d => d.os.startsWith(osFilter))
    list = [...list].sort((a, b) => {
      let av = a[sortKey], bv = b[sortKey]
      if (typeof av === 'string') av = av.toLowerCase()
      if (typeof bv === 'string') bv = bv.toLowerCase()
      if (av < bv) return sortDir === 'asc' ? -1 : 1
      if (av > bv) return sortDir === 'asc' ? 1 : -1
      return 0
    })
    return list
  }, [search, statusFilter, osFilter, sortKey, sortDir])

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

  const onlineCount = MOCK_DEVICES.filter(d => d.status === 'online').length

  return (
    <div className={styles.page}>

      {/* Header */}
      <div className={styles.pageHeader}>
        <div>
          <h1 className={styles.pageTitle}>Devices</h1>
          <p className={styles.pageSubtitle}>{MOCK_DEVICES.length} devices · {onlineCount} online</p>
        </div>
        <button className="btn-primary"><Plus size={14} /> Add Device</button>
      </div>

      {/* Toolbar */}
      <div className={styles.toolbar}>
        <div className={styles.searchBox}>
          <Search size={14} />
          <input
            type="text"
            placeholder="Search name, hostname, IP, department…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          {search && (
            <button className={styles.clearSearch} onClick={() => setSearch('')}>
              <X size={13} />
            </button>
          )}
        </div>

        <div className={styles.filterPills}>
          <button className={`${styles.pill} ${statusFilter === 'all' ? styles.active : ''}`} onClick={() => setStatusFilter('all')}>All</button>
          <button className={`${styles.pill} ${statusFilter === 'online' ? styles.active : ''}`} onClick={() => setStatusFilter('online')}>
            <span className={styles.statusDot} style={{ background: 'var(--success)' }} />
            Online
          </button>
          <button className={`${styles.pill} ${statusFilter === 'offline' ? styles.active : ''}`} onClick={() => setStatusFilter('offline')}>
            <span className={styles.statusDot} style={{ background: 'var(--text-muted)' }} />
            Offline
          </button>
        </div>

        <div className={styles.osPills}>
          {['all', ...OS_LIST].map(os => (
            <button key={os} className={`${styles.pill} ${osFilter === os ? styles.active : ''}`} onClick={() => setOsFilter(os)}>
              {os === 'all' ? 'All OS' : `${OS_ICONS[os]} ${os}`}
            </button>
          ))}
        </div>

        <div className={styles.resultCount}>
          {filtered.length} result{filtered.length !== 1 ? 's' : ''}
        </div>
      </div>

      {/* Table */}
      <div className={styles.tableWrap}>
        {/* Table header */}
        <div className={styles.tableHead}>
          <div className={styles.colStatus}>Status</div>
          <div className={styles.colName}>
            <button className={styles.sortBtn} onClick={() => handleSort('name')}>
              Name <SortIcon k="name" />
            </button>
          </div>
          <div className={styles.colIp}>IP Address</div>
          <div className={styles.colOs}>OS</div>
          <div className={styles.colDept}>Department</div>
          <div className={styles.colCpu}>
            <button className={styles.sortBtn} onClick={() => handleSort('cpu')}>
              CPU <SortIcon k="cpu" />
            </button>
          </div>
          <div className={styles.colRam}>
            <button className={styles.sortBtn} onClick={() => handleSort('ram')}>
              RAM <SortIcon k="ram" />
            </button>
          </div>
          <div className={styles.colDisk}>
            <button className={styles.sortBtn} onClick={() => handleSort('disk')}>
              Disk <SortIcon k="disk" />
            </button>
          </div>
          <div className={styles.colLast}>Last Seen</div>
          <div className={styles.colActions}>Actions</div>
        </div>

        {/* Virtualized rows */}
        <div ref={parentRef} className={styles.tableBody}>
          <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
            {virtualizer.getVirtualItems().map(vRow => {
              const d = filtered[vRow.index]
              return (
                <div
                  key={d.id}
                  className={`${styles.tableRow} ${selected === d.id ? styles.selected : ''}`}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    height: vRow.size,
                    transform: `translateY(${vRow.start}px)`,
                  }}
                  onClick={() => setSelected(d.id)}
                >
                  <div className={styles.colStatus}>
                    <span className={`${styles.pip} ${d.status === 'online' ? styles.pipOnline : styles.pipOffline}`} />
                  </div>
                  <div className={styles.colName}>
                    <div className={styles.deviceIcon}>{OS_ICONS[d.os.split(' ')[0]] || '💻'}</div>
                    <div>
                      <div className={styles.deviceName}>{d.name}</div>
                      <div className={styles.deviceHostname}>{d.hostname}</div>
                    </div>
                  </div>
                  <div className={styles.colIp}>{d.ip}</div>
                  <div className={styles.colOs}>{d.os}</div>
                  <div className={styles.colDept}>
                    <span className={styles.deptTag}>{d.department}</span>
                  </div>
                  <div className={styles.colCpu}><StatBar value={d.cpu} /></div>
                  <div className={styles.colRam}><StatBar value={d.ram} /></div>
                  <div className={styles.colDisk}><StatBar value={d.disk} /></div>
                  <div className={styles.colLast}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: 'var(--text-muted)' }}>
                      <Clock size={11} />
                      {d.lastSeen}
                    </span>
                  </div>
                  <div className={styles.colActions}>
                    <button
                      className="btn-primary"
                      style={{ padding: '4px 10px', fontSize: 11 }}
                      disabled={d.status === 'offline'}
                      onClick={e => { e.stopPropagation() }}
                    >
                      Connect
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      </div>

      {/* Pagination hint */}
      {filtered.length > 50 && (
        <div className={styles.paginationHint}>
          Showing {Math.min(50, filtered.length)} of {filtered.length} devices — scroll to load more
        </div>
      )}
    </div>
  )
}