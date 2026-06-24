import React, { useState, useRef, useMemo, useCallback } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Monitor, Search, Plus, ArrowUpDown, ArrowUp, ArrowDown, X, Clock } from 'lucide-react'
import styles from './DevicesPage.module.css'

const OS_ICONS = { macOS: '🍎', Windows: '🪟', Linux: '🐧', Raspbian: '🍓' }
const MOCK_DEVICES = Array.from({ length: 10000 }, (_, i) => {
  const osList = ['macOS', 'Windows', 'Linux']
  const os = osList[i % 3]
  return {
    id: i + 1,
    name: ['MacBook Pro 16"', 'Windows Desktop', 'Ubuntu Server', 'Mac Mini', 'Windows Laptop', 'iMac', 'Chromebook'][i % 7],
    hostname: `host-${String(i + 1).padStart(3, '0')}.local`,
    ip: `192.168.${Math.floor(i / 254)}.${(i % 254) + 1}`,
    os: os === 'macOS' ? 'macOS Sonoma' : os === 'Windows' ? 'Windows 11 Pro' : 'Ubuntu 22.04 LTS',
    cpu: Math.floor(Math.random() * 90),
    ram: Math.floor(Math.random() * 90),
    disk: Math.floor(Math.random() * 90),
    status: Math.random() > 0.25 ? 'online' : 'offline',
    lastSeen: ['Just now', '2 min ago', '5 min ago', '1h ago', '3h ago'][i % 5],
    department: ['Engineering', 'Design', 'IT', 'Sales'][i % 4],
  }
})

function StatBar({ value }) {
  const color = value > 80 ? 'var(--danger)' : value > 60 ? 'var(--warning)' : 'var(--success)'
  return (
    <div className={styles.statBar}>
      <div className={styles.statBarFill} style={{ width: `${value}%`, background: color }} />
      <span className={styles.statVal} style={{ color }}>{value}%</span>
    </div>
  )
}

export function DevicesPage() {
  const parentRef = useRef(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [sortKey, setSortKey] = useState('name')
  const [sortDir, setSortDir] = useState('asc')
  const [selected, setSelected] = useState(null)

  const filtered = useMemo(() => {
    let list = MOCK_DEVICES
    if (search) {
      const q = search.toLowerCase()
      list = list.filter(d => d.name.toLowerCase().includes(q) || d.hostname.toLowerCase().includes(q) || d.ip.includes(q))
    }
    if (statusFilter !== 'all') list = list.filter(d => d.status === statusFilter)
    list = [...list].sort((a, b) => {
      let av = a[sortKey], bv = b[sortKey]
      if (typeof av === 'string') { av = av.toLowerCase(); bv = bv.toLowerCase() }
      if (av < bv) return sortDir === 'asc' ? -1 : 1
      if (av > bv) return sortDir === 'asc' ? 1 : -1
      return 0
    })
    return list
  }, [search, statusFilter, sortKey, sortDir])

  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 52,
    overscan: 8,
  })

  const handleSort = useCallback((key) => {
    if (sortKey === key) setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('asc') }
  }, [sortKey])

  const SortIcon = ({ k }) => sortKey !== k
    ? <ArrowUpDown size={11} style={{ opacity: 0.3 }} />
    : sortDir === 'asc' ? <ArrowUp size={11} /> : <ArrowDown size={11} />

  const onlineCount = MOCK_DEVICES.filter(d => d.status === 'online').length

  return (
    <div className={styles.page}>
      <div className={styles.pageHeader}>
        <div>
          <h1 className={styles.pageTitle}>Devices</h1>
          <p className={styles.pageSubtitle}>{MOCK_DEVICES.length} devices · {onlineCount} online</p>
        </div>
        <button className="btn-primary"><Plus size={14} /> Add Device</button>
      </div>

      <div className={styles.toolbar}>
        <div className={styles.searchBox}>
          <Search size={14} />
          <input type="text" placeholder="Search name, hostname, IP…" value={search} onChange={e => setSearch(e.target.value)} />
          {search && <button className={styles.clearSearch} onClick={() => setSearch('')}><X size={12} /></button>}
        </div>
        <div className={styles.filterPills}>
          <button className={`${styles.pill} ${statusFilter === 'all' ? styles.active : ''}`} onClick={() => setStatusFilter('all')}>All</button>
          <button className={`${styles.pill} ${statusFilter === 'online' ? styles.active : ''}`} onClick={() => setStatusFilter('online')}>Online</button>
          <button className={`${styles.pill} ${statusFilter === 'offline' ? styles.active : ''}`} onClick={() => setStatusFilter('offline')}>Offline</button>
        </div>
        <div className={styles.resultCount}>{filtered.length} results</div>
      </div>

      <div className={styles.tableWrap}>
        <div className={styles.tableHead}>
          <div className={styles.colStatus}>Status</div>
          <div className={styles.colName}>
            <button className={styles.sortBtn} onClick={() => handleSort('name')}>Name <SortIcon k="name" /></button>
          </div>
          <div className={styles.colIp}>IP Address</div>
          <div className={styles.colOs}>OS</div>
          <div className={styles.colCpu}><button className={styles.sortBtn} onClick={() => handleSort('cpu')}>CPU <SortIcon k="cpu" /></button></div>
          <div className={styles.colRam}><button className={styles.sortBtn} onClick={() => handleSort('ram')}>RAM <SortIcon k="ram" /></button></div>
          <div className={styles.colDisk}><button className={styles.sortBtn} onClick={() => handleSort('disk')}>Disk <SortIcon k="disk" /></button></div>
          <div className={styles.colLast}>Last Seen</div>
          <div className={styles.colActions}>Actions</div>
        </div>

        <div ref={parentRef} className={styles.tableBody}>
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
                  <div className={styles.colStatus}><span className={`${styles.pip} ${d.status === 'online' ? styles.pipOnline : styles.pipOffline}`} /></div>
                  <div className={styles.colName}>
                    <span style={{ fontSize: 16 }}>{OS_ICONS[d.os.split(' ')[0]] || '💻'}</span>
                    <div>
                      <div className={styles.deviceName}>{d.name}</div>
                      <div className={styles.deviceHostname}>{d.hostname}</div>
                    </div>
                  </div>
                  <div className={styles.colIp}>{d.ip}</div>
                  <div className={styles.colOs}>{d.os}</div>
                  <div className={styles.colCpu}><StatBar value={d.cpu} /></div>
                  <div className={styles.colRam}><StatBar value={d.ram} /></div>
                  <div className={styles.colDisk}><StatBar value={d.disk} /></div>
                  <div className={styles.colLast}><Clock size={11} /> {d.lastSeen}</div>
                  <div className={styles.colActions}>
                    <button className="btn-primary" style={{ padding: '4px 10px', fontSize: 11 }} disabled={d.status === 'offline'}>Connect</button>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}