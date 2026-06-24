import React, { useEffect, useMemo, useRef, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { FileText, X, Trash2 } from 'lucide-react'
import { useWails } from '../hooks/useWails.js'
import styles from './LogsOverlay.module.css'

function normalizeLog(entry) {
  if (typeof entry === 'string') {
    return {
      time: new Date().toISOString(),
      level: 'info',
      message: entry,
      raw: entry,
    }
  }

  return {
    time: entry?.time || new Date().toISOString(),
    level: entry?.level || 'info',
    message: entry?.message || entry?.raw || 'Log entry',
    raw: entry?.raw || '',
  }
}

export function LogsOverlay() {
  const { getLogs, clearLogs } = useWails()
  const [logs, setLogs] = useState([])
  const [isOpen, setIsOpen] = useState(false)
  const endRef = useRef(null)

  useEffect(() => {
    getLogs().then((entries) => {
      setLogs(Array.isArray(entries) ? entries.map(normalizeLog) : [])
    }).catch(() => setLogs([]))
  }, [getLogs])

  useEffect(() => {
    if (typeof window === 'undefined' || !window.runtime) return

    const handler = (entry) => {
      setLogs((prev) => {
        const next = [...prev, normalizeLog(entry)]
        return next.length > 250 ? next.slice(next.length - 250) : next
      })
      setIsOpen(true)
    }

    window.runtime.EventsOn('app:log_received', handler)
    const openHandler = () => setIsOpen(true)
    const closeHandler = () => setIsOpen(false)
    const toggleHandler = () => setIsOpen((value) => !value)
    window.addEventListener('remote:open_logs', openHandler)
    window.addEventListener('remote:close_logs', closeHandler)
    window.addEventListener('remote:toggle_logs', toggleHandler)

    return () => {
      window.runtime.EventsOff('app:log_received')
      window.removeEventListener('remote:open_logs', openHandler)
      window.removeEventListener('remote:close_logs', closeHandler)
      window.removeEventListener('remote:toggle_logs', toggleHandler)
    }
  }, [])

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs, isOpen])

  const levelCounts = useMemo(() => {
    return logs.reduce((counts, entry) => {
      counts[entry.level] = (counts[entry.level] || 0) + 1
      return counts
    }, {})
  }, [logs])

  const handleClear = async () => {
    setLogs([])
    try {
      await clearLogs()
    } catch (err) {
      console.error(err)
    }
  }

  return (
    <>
      <AnimatePresence>
        {!isOpen && (
          <motion.button
            className={styles.fab}
            onClick={() => setIsOpen(true)}
            initial={{ scale: 0 }}
            animate={{ scale: 1 }}
            exit={{ scale: 0 }}
            title="Open Logs"
          >
            <FileText size={18} />
            <span>Logs</span>
            {logs.length > 0 && <span className={styles.badge}>{logs.length}</span>}
          </motion.button>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {isOpen && (
          <motion.div
            className={styles.overlay}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 20 }}
          >
            <div className={styles.header}>
              <div>
                <h4>System Logs</h4>
                <p>{logs.length} entries</p>
              </div>
              <div className={styles.headerActions}>
                <button onClick={handleClear} title="Clear logs">
                  <Trash2 size={14} />
                </button>
                <button onClick={() => setIsOpen(false)} title="Close logs">
                  <X size={14} />
                </button>
              </div>
            </div>

            <div className={styles.summary}>
              <span>{levelCounts.error || 0} errors</span>
              <span>{levelCounts.warn || 0} warnings</span>
              <span>{levelCounts.info || 0} info</span>
            </div>

            <div className={styles.body}>
              {logs.length === 0 ? (
                <div className={styles.emptyState}>
                  Waiting for log activity…
                </div>
              ) : (
                logs.map((entry, index) => (
                  <div key={`${entry.time}-${index}`} className={styles.row}>
                    <div className={`${styles.level} ${styles[`level_${entry.level}`] || styles.level_info}`}>
                      {entry.level}
                    </div>
                    <div className={styles.content}>
                      <div className={styles.meta}>{entry.time}</div>
                      <div className={styles.message}>{entry.message}</div>
                    </div>
                  </div>
                ))
              )}
              <div ref={endRef} />
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  )
}
