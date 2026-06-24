import React, { useEffect, useMemo, useRef } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { FileText, Trash2, X } from 'lucide-react'
import { useAppLogs } from '../logs/AppLogsContext.jsx'
import styles from './LogsOverlay.module.css'

export function LogsOverlay() {
  const { logs, isOpen, toggleLogs, closeLogs, clearLogs } = useAppLogs()
  const endRef = useRef(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs, isOpen])

  const summary = useMemo(() => logs.reduce((counts, entry) => {
    counts[entry.level] = (counts[entry.level] || 0) + 1
    return counts
  }, {}), [logs])

  return (
    <>
      <AnimatePresence>
        {!isOpen && (
          <motion.button
            className={styles.fab}
            onClick={toggleLogs}
            initial={{ scale: 0 }}
            animate={{ scale: 1 }}
            exit={{ scale: 0 }}
            title="Open Logs"
          >
            <FileText size={18} />
            {logs.length > 0 && <span className={styles.badge}>{logs.length}</span>}
          </motion.button>
        )}
      </AnimatePresence>

      <AnimatePresence>
        {isOpen && (
          <motion.aside
            className={styles.panel}
            initial={{ opacity: 0, x: 20 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: 20 }}
          >
            <div className={styles.header}>
              <div>
                <h4>Activity Logs</h4>
                <p>{logs.length} events</p>
              </div>
              <div className={styles.actions}>
                <button onClick={clearLogs} title="Clear logs">
                  <Trash2 size={14} />
                </button>
                <button onClick={closeLogs} title="Close logs">
                  <X size={14} />
                </button>
              </div>
            </div>

            <div className={styles.summary}>
              <span>{summary.error || 0} errors</span>
              <span>{summary.warn || 0} warnings</span>
              <span>{summary.info || 0} info</span>
            </div>

            <div className={styles.body}>
              {logs.length === 0 ? (
                <div className={styles.empty}>No activity yet.</div>
              ) : (
                logs.map((entry) => (
                  <div key={entry.id} className={styles.row}>
                    <div className={`${styles.level} ${styles[`level_${entry.level}`] || styles.level_info}`}>
                      {entry.level}
                    </div>
                    <div className={styles.content}>
                      <div className={styles.meta}>
                        <span>{entry.time}</span>
                        <span>{entry.source}</span>
                      </div>
                      <div className={styles.message}>{entry.message}</div>
                      {entry.details ? (
                        <pre className={styles.details}>{JSON.stringify(entry.details, null, 2)}</pre>
                      ) : null}
                    </div>
                  </div>
                ))
              )}
              <div ref={endRef} />
            </div>
          </motion.aside>
        )}
      </AnimatePresence>
    </>
  )
}
