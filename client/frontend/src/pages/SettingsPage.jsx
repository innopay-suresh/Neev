import React, { useState } from 'react'
import { Settings, Globe, Shield, Monitor, Sparkles, User, Bell, Save } from 'lucide-react'
import styles from './SettingsPage.module.css'

const SECTIONS = [
  { id: 'general',   label: 'Application', icon: Settings,  description: 'General, updates, notifications' },
  { id: 'network',   label: 'Connection',  icon: Globe,     description: 'Network, relay, bandwidth' },
  { id: 'security',  label: 'Security',    icon: Shield,    description: 'Access, permissions, privacy, MFA' },
  { id: 'display',   label: 'Sessions',    icon: Monitor,   description: 'Display, audio, recording, clipboard' },
  { id: 'ai',        label: 'AI',          icon: Sparkles,  description: 'Providers, models, prompts' },
  { id: 'account',   label: 'Account',     icon: User,      description: 'Profile, license, organization' },
]

export function SettingsPage() {
  const [activeSection, setActiveSection] = useState('general')

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Settings</h1>
          <p className="page-subtitle">Configure application, connection, and security preferences</p>
        </div>
        <button className="btn-primary"><Save size={14} /> Save Changes</button>
      </div>

      <div className={styles.layout}>
        {/* Side nav */}
        <div className={styles.settingsNav}>
          {SECTIONS.map(({ id, label, icon: Icon, description }) => (
            <button
              key={id}
              className={`${styles.navItem} ${activeSection === id ? styles.active : ''}`}
              onClick={() => setActiveSection(id)}
            >
              <Icon size={15} />
              <div>
                <div className={styles.navLabel}>{label}</div>
                <div className={styles.navDesc}>{description}</div>
              </div>
            </button>
          ))}
        </div>

        {/* Settings panel */}
        <div className={`card ${styles.panel}`}>
          {activeSection === 'general' && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>Application</h2>
              <div className={styles.group}>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Start on system boot</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Launch Neev Remote when you log in</span>
                    <input type="checkbox" className={styles.toggle} defaultChecked />
                  </div>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Auto-update</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Automatically download and install updates</span>
                    <input type="checkbox" className={styles.toggle} defaultChecked />
                  </div>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Notification level</label>
                  <select style={{ maxWidth: 220 }}>
                    <option>All notifications</option>
                    <option>Only important</option>
                    <option>None</option>
                  </select>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'network' && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>Connection</h2>
              <div className={styles.group}>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Relay server</label>
                  <input type="text" defaultValue="relay.remoteagent.io" style={{ maxWidth: 320 }} />
                  <span className={styles.fieldHint}>STUN/TURN relay server for NAT traversal</span>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Bandwidth limit</label>
                  <select style={{ maxWidth: 220 }}>
                    <option>Unlimited</option>
                    <option>5 Mbps</option>
                    <option>10 Mbps</option>
                    <option>25 Mbps</option>
                  </select>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Connection timeout</label>
                  <input type="number" defaultValue={30} style={{ maxWidth: 100 }} />
                  <span className={styles.fieldHint}>Seconds to wait before falling back to relay</span>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'security' && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>Security & Privacy</h2>
              <div className={styles.group}>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Require authentication</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Require password before accepting session</span>
                    <input type="checkbox" className={styles.toggle} defaultChecked />
                  </div>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Session approval</label>
                  <select style={{ maxWidth: 220 }}>
                    <option>Always ask</option>
                    <option>Ask first time only</option>
                    <option>Never ask</option>
                  </select>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Clipboard sync</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Allow clipboard sharing during sessions</span>
                    <input type="checkbox" className={styles.toggle} defaultChecked />
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'display' && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>Sessions</h2>
              <div className={styles.group}>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Default quality</label>
                  <select style={{ maxWidth: 220 }}>
                    <option>Adaptive (recommended)</option>
                    <option>Ultra (lossless)</option>
                    <option>High (60 fps)</option>
                    <option>Medium (30 fps)</option>
                    <option>Low (15 fps)</option>
                  </select>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Record sessions</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Save session recordings to local storage</span>
                    <input type="checkbox" className={styles.toggle} />
                  </div>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Audio streaming</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Stream remote audio to local speaker</span>
                    <input type="checkbox" className={styles.toggle} />
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'ai' && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>AI Configuration</h2>
              <div className={styles.group}>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>AI Provider</label>
                  <select style={{ maxWidth: 220 }}>
                    <option>OpenAI</option>
                    <option>Anthropic</option>
                    <option>Local model</option>
                    <option>Disabled</option>
                  </select>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Model</label>
                  <select style={{ maxWidth: 220 }}>
                    <option>GPT-4o</option>
                    <option>GPT-4o-mini</option>
                    <option>Claude 3.5 Sonnet</option>
                  </select>
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>AI session summary</label>
                  <div className={styles.toggleRow}>
                    <span className={styles.fieldDesc}>Auto-generate summary after each session</span>
                    <input type="checkbox" className={styles.toggle} defaultChecked />
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeSection === 'account' && (
            <div className={styles.section}>
              <h2 className={styles.sectionTitle}>Account</h2>
              <div className={styles.group}>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Display name</label>
                  <input type="text" defaultValue="Suresh" style={{ maxWidth: 260 }} />
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>Email</label>
                  <input type="email" defaultValue="suresh@example.com" style={{ maxWidth: 260 }} />
                </div>
                <div className={styles.field}>
                  <label className={styles.fieldLabel}>License key</label>
                  <input type="text" defaultValue="RA-XXXX-XXXX-XXXX" style={{ maxWidth: 260, fontFamily: 'var(--mono)', fontSize: 12 }} />
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}