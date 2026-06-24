import React, { createContext, useCallback, useContext, useMemo, useState } from 'react'

const AppLogsContext = createContext(null)
const MAX_LOGS = 250

export function AppLogsProvider({ children }) {
  const [logs, setLogs] = useState([])
  const [isOpen, setIsOpen] = useState(false)

  const log = useCallback((level, source, message, details = null) => {
    const entry = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      time: new Date().toISOString(),
      level,
      source,
      message,
      details,
    }

    setLogs((prev) => {
      const next = [...prev, entry]
      return next.length > MAX_LOGS ? next.slice(next.length - MAX_LOGS) : next
    })
  }, [])

  const clearLogs = useCallback(() => setLogs([]), [])
  const openLogs = useCallback(() => setIsOpen(true), [])
  const closeLogs = useCallback(() => setIsOpen(false), [])
  const toggleLogs = useCallback(() => setIsOpen((value) => !value), [])

  const value = useMemo(() => ({
    logs,
    log,
    clearLogs,
    isOpen,
    openLogs,
    closeLogs,
    toggleLogs,
    setIsOpen,
  }), [logs, log, clearLogs, isOpen, openLogs, closeLogs, toggleLogs])

  return (
    <AppLogsContext.Provider value={value}>
      {children}
    </AppLogsContext.Provider>
  )
}

export function useAppLogs() {
  const context = useContext(AppLogsContext)
  if (!context) {
    throw new Error('useAppLogs must be used inside AppLogsProvider')
  }
  return context
}
