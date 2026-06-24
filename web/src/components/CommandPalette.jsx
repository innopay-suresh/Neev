import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Search, LayoutDashboard, Monitor, ScreenShare, Clock, Shield,
  Sparkles, BarChart2, Settings, Download, X, ArrowRight,
  Zap, Activity, Users, Building2, ArrowUpDown
} from 'lucide-react'
import styles from './CommandPalette.module.css'

/* ── Command registry ──────────────────────────────────────────────────────── */
var COMMANDS = [
  /* Pages */
  { id: 'nav-dashboard',   label: 'Go to Dashboard',    group: 'Navigation', icon: LayoutDashboard, action: 'navigate', target: '/dashboard' },
  { id: 'nav-devices',     label: 'Go to Devices',      group: 'Navigation', icon: Monitor,         action: 'navigate', target: '/devices'   },
  { id: 'nav-remote',      label: 'Go to Remote Access', group: 'Navigation', icon: ScreenShare,     action: 'navigate', target: '/remote'    },
  { id: 'nav-sessions',    label: 'Go to Sessions',     group: 'Navigation', icon: Clock,           action: 'navigate', target: '/sessions'  },
  { id: 'nav-security',    label: 'Go to Security',     group: 'Navigation', icon: Shield,          action: 'navigate', target: '/security'  },
  { id: 'nav-ai',          label: 'Go to AI Assistant', group: 'Navigation', icon: Sparkles,        action: 'navigate', target: '/ai'        },
  { id: 'nav-teams',       label: 'Go to Teams',        group: 'Navigation', icon: Users,           action: 'navigate', target: '/teams'     },
  { id: 'nav-enterprise',  label: 'Go to Enterprise',   group: 'Navigation', icon: Building2,       action: 'navigate', target: '/enterprise'},
  { id: 'nav-analytics',   label: 'Go to Analytics',    group: 'Navigation', icon: BarChart2,       action: 'navigate', target: '/analytics' },
  { id: 'nav-downloads',   label: 'Go to Downloads',    group: 'Navigation', icon: Download,        action: 'navigate', target: '/downloads' },
  { id: 'nav-settings',    label: 'Go to Settings',     group: 'Navigation', icon: Settings,        action: 'navigate', target: '/settings'  },
  /* Actions */
  { id: 'act-new-session', label: 'Start new remote session', group: 'Actions', icon: ScreenShare, action: 'navigate', target: '/remote' },
  { id: 'act-refresh-dev', label: 'Refresh devices',           group: 'Actions', icon: Monitor,    action: 'refresh' },
  { id: 'act-export',      label: 'Export device list (CSV)',  group: 'Actions', icon: Download,   action: 'export' },
  { id: 'act-ai-analytics',label: 'Open AI Analytics insight', group: 'Actions', icon: Sparkles,   action: 'navigate', target: '/ai' },
  { id: 'act-dark-mode',   label: 'Toggle dark mode (stub)',   group: 'Actions', icon: Zap,         action: 'darkmode' },
]

/* ── Fuzzy filter ──────────────────────────────────────────────────────────── */
function fuzzyMatch(query, text) {
  query = query.toLowerCase()
  text  = text.toLowerCase()
  var qi = 0
  for (var ti = 0; ti < text.length && qi < query.length; ti++) {
    if (text[ti] === query[qi]) qi++
  }
  return qi === query.length
}

function highlightMatch(query, text) {
  if (!query) return text
  var q = query.toLowerCase()
  var result = []
  var inHighlight = false
  var qi = 0
  for (var i = 0; i < text.length; i++) {
    var c = text[i]
    var lc = c.toLowerCase()
    if (qi < q.length && lc === q[qi]) {
      if (!inHighlight) { result.push({ t: c, h: true }); inHighlight = true }
      else result.push({ t: c, h: true })
      qi++
    } else {
      if (inHighlight) { result.push({ t: c, h: false }); inHighlight = false }
      else result.push({ t: c, h: false })
    }
  }
  return result
}

/* ── Highlighted label ─────────────────────────────────────────────────────── */
function HighlightLabel(props) {
  var parts = highlightMatch(props.query, props.text)
  return (
    <span>
      {parts.map(function(p, i) { return p.h ? <mark key={i} className={styles.hl}>{p.t}</mark> : <span key={i}>{p.t}</span> })}
    </span>
  )
}

/* ── CommandPalette ────────────────────────────────────────────────────────── */
export function CommandPalette(props) {
  var open     = props.open
  var onClose  = props.onClose
  var navigate = props.navigate

  var _useState = useState('')
  var query     = _useState[0]
  var setQuery  = _useState[1]
  var _useState2 = useState(0)
  var selected  = _useState2[0]
  var setSelected = _useState2[1]

  var inputRef = useRef(null)

  /* Filtered + grouped commands */
  var results = useMemo(function() {
    if (!query) return COMMANDS
    return COMMANDS.filter(function(c) { return fuzzyMatch(query, c.label) })
  }, [query])

  /* Group results */
  var grouped = useMemo(function() {
    var groups = {}
    for (var i = 0; i < results.length; i++) {
      var g = results[i].group
      if (!groups[g]) groups[g] = []
      groups[g].push(results[i])
    }
    return groups
  }, [results])

  /* Flat list for keyboard nav */
  var flat = results

  /* Focus input when opened */
  useEffect(function() {
    if (open) {
      setQuery('')
      setSelected(0)
      setTimeout(function() { inputRef.current && inputRef.current.focus() }, 50)
    }
  }, [open])

  /* Keyboard navigation */
  useEffect(function() {
    if (!open) return
    function handleKey(e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setSelected(function(s) { return Math.min(s + 1, flat.length - 1) }) }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setSelected(function(s) { return Math.max(s - 1, 0) }) }
      else if (e.key === 'Enter') {
        var cmd = flat[selected]
        if (cmd) execute(cmd)
      }
      else if (e.key === 'Escape') { onClose() }
    }
    window.addEventListener('keydown', handleKey)
    return function() { window.removeEventListener('keydown', handleKey) }
  }, [open, selected, flat])

  function execute(cmd) {
    if (cmd.action === 'navigate') navigate(cmd.target)
    else if (cmd.action === 'refresh') window.location.reload()
    else if (cmd.action === 'export') alert('Exporting device list as CSV…')
    else if (cmd.action === 'darkmode') alert('Dark mode toggle (stub)')
    onClose()
  }

  /* Scroll selected into view */
  var listRef = useRef(null)
  useEffect(function() {
    var el = listRef.current
    if (!el) return
    var item = el.children[selected]
    if (item) item.scrollIntoView({ block: 'nearest' })
  }, [selected])

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            className={styles.backdrop}
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={onClose}
          />
          <motion.div
            className={styles.palette}
            initial={{ opacity: 0, scale: 0.96, y: -10 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: -10 }}
            transition={{ duration: 0.15 }}
          >
            {/* Search input */}
            <div className={styles.searchBar}>
              <Search size={15} className={styles.searchIcon} />
              <input
                ref={inputRef}
                type="text"
                placeholder="Search commands, pages, actions…"
                value={query}
                onChange={function(e) { setQuery(e.target.value); setSelected(0) }}
                className={styles.searchInput}
              />
              <kbd className={styles.escHint}>ESC</kbd>
            </div>

            {/* Results */}
            <div className={styles.resultList} ref={listRef}>
              {flat.length === 0 && (
                <div className={styles.empty}>No results for "{query}"</div>
              )}
              {Object.keys(grouped).map(function(groupName) {
                var cmds = grouped[groupName]
                var groupStartIndex = flat.indexOf(cmds[0])
                return (
                  <div key={groupName} className={styles.group}>
                    <div className={styles.groupLabel}>{groupName}</div>
                    {cmds.map(function(cmd) {
                      var flatIdx = flat.indexOf(cmd)
                      var Icon = cmd.icon
                      return (
                        <button
                          key={cmd.id}
                          className={styles.resultItem + (flatIdx === selected ? (' ' + styles.resultItemSelected) : '')}
                          onClick={function() { execute(cmd) }}
                          onMouseEnter={function() { setSelected(flatIdx) }}
                        >
                          <div className={styles.resultIcon}><Icon size={14} /></div>
                          <span className={styles.resultLabel}><HighlightLabel query={query} text={cmd.label} /></span>
                          {flatIdx === selected && <ArrowRight size={12} className={styles.resultArrow} />}
                        </button>
                      )
                    })}
                  </div>
                )
              })}
            </div>

            {/* Footer hints */}
            <div className={styles.footer}>
              <span><kbd>↑↓</kbd> navigate</span>
              <span><kbd>↵</kbd> select</span>
              <span><kbd>ESC</kbd> close</span>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}