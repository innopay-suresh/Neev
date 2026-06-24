import React, { useCallback, useEffect, useMemo, useState } from 'react'

const IST_OPTIONS = {
  timeZone: 'Asia/Kolkata',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: true,
}

const IST_DATE_OPTIONS = {
  timeZone: 'Asia/Kolkata',
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: true,
}

import { Download, Package, Shield, RefreshCw, ExternalLink, Copy, Monitor, Smartphone } from 'lucide-react'
import { apiFetch } from '../lib/api.js'
import styles from './DownloadsPage.module.css'

function formatBytes(bytes) {
  if (!bytes && bytes !== 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`
}

export function FlutterDownloadsPage() {
  const [installers, setInstallers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [lastUpdated, setLastUpdated] = useState('')

  const loadInstallers = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const response = await apiFetch('/api/v1/public/flutter-installers')
      const payload = await response.json().catch(() => ({}))
      if (!response.ok) {
        throw new Error(payload?.error || `installers:${response.status}`)
      }
      setInstallers(Array.isArray(payload?.installers) ? payload.installers : [])
      setLastUpdated(new Date().toLocaleTimeString('en-IN', IST_OPTIONS))
    } catch (err) {
      setError(`Could not load installers: ${String(err.message || err)}`)
      setInstallers([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadInstallers()
  }, [loadInstallers])

  useEffect(() => {
    const timer = setInterval(() => {
      loadInstallers()
    }, 15000)
    return () => clearInterval(timer)
  }, [loadInstallers])

  const grouped = useMemo(() => {
    return installers.reduce((acc, item) => {
      const key = item.platform || 'other'
      acc[key] = acc[key] || []
      acc[key].push(item)
      return acc
    }, {})
  }, [installers])

  const platformOrder = ['windows', 'macos', 'linux', 'other']

  const platformLabel = useCallback((platform) => {
    switch (platform) {
      case 'windows':
        return 'Windows'
      case 'macos':
        return 'macOS'
      case 'linux':
        return 'Linux'
      default:
        return 'Other'
    }
  }, [])

  const copyText = useCallback(async (text) => {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
    } catch (err) {
      console.warn('[downloads] copy failed', err)
    }
  }, [])

  return (
    <div className={styles.page}>
      <div className={styles.hero}>
        <div className={styles.heroBadge}>
          <Package size={14} />
          <span>Neev Remote Desktop</span>
        </div>
        <h1>Download Cross-Platform Remote Desktop</h1>
        <p>
          <strong>All-in-one app</strong> — works as both Viewer and Agent on any platform.
          No separate packages needed. Download once, use for remote access or to allow remote connections.
        </p>
        <div className={styles.heroActions}>
          <button type="button" className={styles.primaryBtn} onClick={loadInstallers}>
            <RefreshCw size={14} /> Refresh list
          </button>
          <button type="button" className={styles.secondaryBtn} onClick={() => copyText(window.location.href)}>
            <Copy size={14} /> Copy portal link
          </button>
        </div>
      </div>

      {error && <div className={styles.errorBox}>{error}</div>}

      <div className={styles.sectionHeader}>
        <Shield size={14} />
        <span>Available installers</span>
        <span className={styles.countBadge}>{installers.length}</span>
        {lastUpdated && <span className={styles.updatedBadge}>Updated {lastUpdated}</span>}
      </div>

      {loading ? (
        <div className={styles.emptyState}>Loading installer list…</div>
      ) : installers.length === 0 ? (
        <div className={styles.emptyState}>
          No installers are published yet. Builds are automatically created via GitHub Actions CI.
          Check back after the first release tag is pushed.
        </div>
      ) : (
        <div className={styles.grid}>
          {platformOrder.map((platform) => (
            grouped[platform]?.length ? (
              <div key={platform} className={styles.platformGroup}>
                <h3>{platformLabel(platform)}</h3>
                <div className={styles.cardGrid}>
                  {grouped[platform].map((installer) => (
                    <div key={installer.filename} className={styles.card}>
                      <div className={styles.cardTop}>
                        <div className={styles.cardIcon}>
                          {platform === 'macos' ? <Monitor size={16} /> : <Download size={16} />}
                        </div>
                        <div className={styles.cardMeta}>
                          <strong>{installer.description || 'Neev Remote'}</strong>
                          <span>{installer.filename}</span>
                        </div>
                      </div>
                      <div className={styles.cardStats}>
                        <span>{formatBytes(installer.size)}</span>
                        <span>{installer.modified_at ? new Date(installer.modified_at).toLocaleString('en-IN', IST_DATE_OPTIONS) : '—'}</span>
                      </div>
                      <div className={styles.cardActions}>
                        <a className={styles.downloadBtn} href={installer.download_url}>
                          <Download size={14} /> Download
                        </a>
                        <button className={styles.copyBtn} type="button" onClick={() => copyText(installer.download_url)}>
                          <ExternalLink size={14} />
                        </button>
                      </div>
                      
                      <div className={styles.featureList}>
                        <span><Monitor size={12} /> Viewer + Agent in one app</span>
                        <span><Smartphone size={12} /> Cross-platform</span>
                      </div>

                      {platform === 'macos' && (
                        <div className={styles.troubleNote}>
                          <strong>macOS Gatekeeper:</strong> If Apple blocks opening, run:
                          <code>sudo xattr -dr com.apple.quarantine /Applications/NeevRemote.app</code>
                          <div className={styles.hintText}>
                            Or open <em>System Settings ➔ Privacy &amp; Security</em> and click <strong>Open Anyway</strong>.
                          </div>
                        </div>
                      )}
                      {platform === 'windows' && (
                        <div className={styles.troubleNote}>
                          <strong>Windows SmartScreen:</strong> If blocked, click <em><strong>More info</strong></em> and select <strong>Run anyway</strong>.
                          <div className={styles.hintText}>
                            After installation, find <strong>Neev Remote</strong> in Start Menu.
                          </div>
                        </div>
                      )}
                      {platform === 'linux' && (
                        <div className={styles.troubleNote}>
                          <strong>Installation:</strong>
                          <code>sudo dpkg -i {installer.filename}</code>
                          <div className={styles.hintText}>
                            Or: <code>sudo apt install ./{installer.filename}</code>
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ) : null
          ))}
        </div>
      )}

      <div className={styles.tipBox}>
        <h3>Quick Start</h3>
        <ol>
          <li><strong>Download</strong> and install the app for your platform</li>
          <li><strong>Launch</strong> the app — you'll see Agent ID displayed</li>
          <li><strong>To allow remote access:</strong> Share your Agent ID with the person helping you</li>
          <li><strong>To connect remotely:</strong> Enter the other person's Agent ID in the Viewer</li>
        </ol>
        <p className={styles.platformNote}>
          No accounts, no passwords. Just install and connect using Agent IDs.
          The app works as both <strong>Viewer</strong> (control remote screen) and <strong>Agent</strong> (allow remote access).
        </p>
      </div>
    </div>
  )
}

export default FlutterDownloadsPage