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
import { Download, Package, Shield, RefreshCw, ExternalLink, Copy } from 'lucide-react'
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

export function DownloadsPage() {
  const [installers, setInstallers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [lastUpdated, setLastUpdated] = useState('')

  const loadInstallers = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const response = await apiFetch('/api/v1/public/installers')
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

  const platformOrder = ['windows', 'macos-desktop', 'macos-agent', 'linux', 'macos', 'other']

  const platformLabel = useCallback((platform) => {
    switch (platform) {
      case 'windows':
        return 'Windows'
      case 'macos-desktop':
        return 'macOS Desktop App'
      case 'macos-agent':
        return 'macOS Agent Package'
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
          <span>Public Downloads</span>
        </div>
        <h1>Install Neev Remote without login</h1>
        <p>
          Share this portal with your users. They can download the correct installer for their laptop:
          use the <strong>desktop app</strong> on the support machine and the <strong>agent package</strong> on each remote laptop.
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
          No installers are published yet. Build packages into <code>dist/packages</code> and mount that
          directory into the server as <code>/app/downloads</code>.
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
                        <div className={styles.cardIcon}><Download size={16} /></div>
                        <div className={styles.cardMeta}>
                          <strong>{installer.description || 'Installer'}</strong>
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
                      {(platform === 'macos' || platform === 'macos-desktop') && (
                        <div className={styles.troubleNote}>
                          <strong>macOS Gatekeeper:</strong> If Apple blocks opening, open <em>System Settings ➔ Privacy &amp; Security</em> and click <strong>Open Anyway</strong>.
                          {platform === 'macos-desktop' && installer.filename.endsWith('.dmg') && (
                            <div className={styles.hintText}>
                              <strong>DMG installer (recommended):</strong> Open the <code>.dmg</code> file and drag <code>neev_remote.app</code> to the <strong>Applications</strong> folder (choose <strong>Replace</strong> if an older version is already there).
                              If Gatekeeper blocks, run: <code>xattr -dr com.apple.quarantine /Applications/neev_remote.app</code>
                            </div>
                          )}
                          {platform === 'macos-desktop' && installer.filename.endsWith('.zip') && (
                            <div className={styles.hintText}>
                              <strong>ZIP package:</strong> Unzip, then double-click <code>Install.command</code> to automatically install to <strong>/Applications</strong> and launch.
                              Or manually: <code>xattr -dr com.apple.quarantine neev_remote.app</code> then drag to Applications.
                            </div>
                          )}
                          {platform === 'macos-agent' && (
                            <div className={styles.hintText}>
                              This is the <strong>agent package</strong>. Install it on the remote laptop so it runs in the background.
                            </div>
                          )}
                        </div>
                      )}
                      {platform === 'windows' && (
                        <div className={styles.troubleNote}>
                          <strong>Windows SmartScreen:</strong> If blocked, click <em><strong>More info</strong></em> and select <strong>Run anyway</strong>.
                          <div className={styles.hintText}>
                            After installation, find <strong>Neev Remote</strong> on your Desktop or in Start Menu → Neev Remote.
                          </div>
                        </div>
                      )}
                      {platform === 'linux' && (
                        <div className={styles.troubleNote}>
                          <strong>Installation:</strong> Install via <code>sudo dpkg -i {installer.filename}</code>, then start the service.
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
        <h3>How to publish installers</h3>
        <ol>
          <li>Run <code>scripts/publish-installers.sh</code> on macOS or Linux to publish the current platform installer plus the macOS desktop app bundle.</li>
          <li>Or build packages with <code>scripts/release-packages.sh</code> / <code>scripts/release-packages.ps1</code> and place them in <code>dist/packages</code>.</li>
          <li>Keep the server mounted to that folder, and users can download without signing in.</li>
        </ol>
        <p className={styles.platformNote}>
          The portal now separates <strong>macOS Desktop App</strong> from <strong>macOS Agent Package</strong>, so users don’t try to open a service installer as an app.
          The agent packages run headless in the background; they do not open a visible window when launched.
          Windows downloads now use the full server URL so they open directly from the portal.
        </p>
      </div>
    </div>
  )
}
