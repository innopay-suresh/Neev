import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Building2, Key, Users, Globe, Lock, CheckCircle2, AlertCircle,
  ExternalLink, RefreshCw, X, ChevronRight, Shield, Database,
  Link2, Eye, EyeOff, Plus, Trash2
} from 'lucide-react'
import styles from './EnterprisePage.module.css'

/* ── SSO Provider configs ──────────────────────────────────────────────────── */
var SSO_PROVIDERS = [
  {
    id: 'entra',
    name: 'Microsoft Entra ID',
    color: '#4F8CFF',
    description: 'Authenticate using your Microsoft organization accounts',
    fields: [
      { key: 'tenant_id',     label: 'Tenant ID',      placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' },
      { key: 'client_id',     label: 'Client ID',       placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' },
      { key: 'client_secret', label: 'Client Secret',   placeholder: 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'   },
    ],
    scopes: ['openid', 'profile', 'email', 'User.Read'],
  },
  {
    id: 'okta',
    name: 'Okta',
    color: '#007dc1',
    description: 'Enterprise SSO with Okta Identity Platform',
    fields: [
      { key: 'domain',        label: 'Okta Domain',     placeholder: 'your-org.okta.com'                },
      { key: 'client_id',     label: 'Client ID',        placeholder: '0oaXXXXXXXXXXXXX'                  },
      { key: 'client_secret', label: 'Client Secret',    placeholder: 'xxxxxxxxxxxxxxxxxxxxxxxx'          },
    ],
    scopes: ['openid', 'profile', 'email', 'groups'],
  },
  {
    id: 'google',
    name: 'Google Workspace',
    color: '#4285F4',
    description: 'Sign in with Google Workspace accounts',
    fields: [
      { key: 'client_id',     label: 'OAuth Client ID',  placeholder: 'xxxxxxxxxxxx-xxxxxxxxxxxxxxxx.apps.googleusercontent.com' },
      { key: 'client_secret', label: 'Client Secret',    placeholder: 'GOCSPX-xxxxxxxxxxxxxxxxxxxxxxxxx'                        },
    ],
    scopes: ['openid', 'profile', 'email'],
  },
]

/* ── SSO Provider icons (inline SVG) ──────────────────────────────────────── */
function EntraIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 21 21" fill="none">
      <rect x="1" y="1" width="9" height="9" fill="#f35325"/>
      <rect x="11" y="1" width="9" height="9" fill="#81bc06"/>
      <rect x="1" y="11" width="9" height="9" fill="#05a6f0"/>
      <rect x="11" y="11" width="9" height="9" fill="#ffba08"/>
    </svg>
  )
}

function OktaIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="10" stroke="#007dc1" strokeWidth="2"/>
      <path d="M12 6C9 6 7 8 7 11C7 14 9 16 12 16C15 16 17 14 17 11C17 8 15 6 12 6Z" stroke="#007dc1" strokeWidth="2"/>
    </svg>
  )
}

function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
      <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
      <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
    </svg>
  )
}

var PROVIDER_ICONS = { entra: EntraIcon, okta: OktaIcon, google: GoogleIcon }

/* ── SSO Provider card ─────────────────────────────────────────────────────── */
function SSOProviderCard(props) {
  var provider = props.provider
  var config   = props.config
  var onChange = props.onChange
  var onToggle = props.onToggle

  var _useState = useState(false)
  var expanded  = _useState[0]
  var setExpanded = _useState[1]

  var isEnabled = config && config.enabled
  var Icon      = PROVIDER_ICONS[provider.id]

  function handleFieldChange(key, value) {
    onChange(provider.id, Object.assign({}, config, { settings: Object.assign({}, (config && config.settings) || {}, { [key]: value }) }))
  }

  function getFieldValue(key) {
    return (config && config.settings && config.settings[key]) || ''
  }

  return (
    <motion.div className={styles.ssoCard} layout style={{ borderColor: isEnabled ? provider.color + '40' : undefined }}>
      <div className={styles.ssoHeader}>
        <div className={styles.ssoLeft}>
          <div className={styles.ssoIconWrap} style={{ background: provider.color + '15' }}>
            <Icon />
          </div>
          <div>
            <div className={styles.ssoName}>{provider.name}</div>
            <div className={styles.ssoDesc}>{provider.description}</div>
          </div>
        </div>
        <div className={styles.ssoRight}>
          {isEnabled && (
            <span className={styles.enabledBadge} style={{ color: provider.color, background: provider.color + '15' }}>
              Configured
            </span>
          )}
          <button
            className={styles.ssoToggle}
            onClick={function() { onToggle(provider.id, !isEnabled) }}
            style={{ background: isEnabled ? provider.color : 'var(--bg-tertiary)', borderColor: isEnabled ? provider.color : 'var(--border)' }}
          >
            <span className={styles.ssoToggleThumb} style={{ transform: isEnabled ? 'translateX(18px)' : 'translateX(2px)' }} />
          </button>
        </div>
      </div>

      <button className={styles.ssoExpandBtn} onClick={function() { setExpanded(!expanded) }}>
        <span>{expanded ? 'Hide' : 'Configure'} connection</span>
        <ChevronRight size={13} style={{ transform: expanded ? 'rotate(90deg)' : 'none', transition: 'transform 0.15s' }} />
      </button>

      <AnimatePresence>
        {expanded && (
          <motion.div className={styles.ssoFields} initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }}>
            <SSOFieldGroup fields={provider.fields} getFieldValue={getFieldValue} onFieldChange={handleFieldChange} />
            <div className={styles.fieldGroup}>
              <label className={styles.fieldLabel}>OAuth Scopes</label>
              <div className={styles.scopeList}>
                {provider.scopes.map(function(s) { return <span key={s} className={styles.scopeChip}>{s}</span> })}
              </div>
            </div>
            <div className={styles.ssoActions}>
              <button className="btn-secondary" style={{ fontSize: 12, padding: '6px 14px' }}>Test Connection</button>
              <button className="btn-primary" style={{ fontSize: 12, padding: '6px 14px' }}>Save {provider.name}</button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  )
}

/* ── SSO field group (non-JSX-map helper) ──────────────────────────────────── */
function SSOFieldGroup(props) {
  var fields        = props.fields
  var getFieldValue = props.getFieldValue
  var onFieldChange = props.onFieldChange

  var rows = []
  for (var i = 0; i < fields.length; i++) {
    var f = fields[i]
    rows.push(
      <div key={f.key} className={styles.fieldGroup}>
        <label className={styles.fieldLabel}>{f.label}</label>
        <div className={styles.fieldInputWrap}>
          <input
            type="password"
            placeholder={f.placeholder}
            value={getFieldValue(f.key)}
            onChange={function(e) { onFieldChange(f.key, e.target.value) }}
            className={styles.fieldInput}
          />
          <Eye size={13} className={styles.fieldEye} />
        </div>
      </div>
    )
  }
  return <>{rows}</>
}

/* ── LDAP/AD Connector ─────────────────────────────────────────────────────── */
function LDAPConnector() {
  var s = { server: 'ldap.acme.com', port: 636, use_ssl: true, bind_dn: '', bind_password: '', base_dn: 'dc=acme,dc=com', user_filter: '(objectClass=user)', sync_interval: 60, connected: false }
  var _useState2 = useState(s)
  var config     = _useState2[0]
  var setConfig  = _useState2[1]
  var _useState3 = useState(false)
  var testing    = _useState3[0]
  var setTesting = _useState3[1]
  var _useState4 = useState(false)
  var showPw     = _useState4[0]
  var setShowPw  = _useState4[1]

  function handleTest() {
    setTesting(true)
    setTimeout(function() {
      setTesting(false)
      setConfig(function(c) { return Object.assign({}, c, { connected: true }) })
    }, 2000)
  }

  function sc(key, val) { setConfig(function(c) { var n = Object.assign({}, c); n[key] = val; return n }) }

  return (
    <div className={styles.ldapSection}>
      <div className={styles.ldapHeader}>
        <div className={styles.ldapTitle}><Database size={16} /><span>LDAP / Active Directory</span></div>
        <span className={config.connected ? styles.connBadge : styles.disconnBadge}>
          {config.connected
            ? <><CheckCircle2 size={11} /> Connected</>
            : <><AlertCircle size={11} /> Not connected</>}
        </span>
      </div>

      <div className={styles.ldapGrid}>
        <div className={styles.fieldGroup}>
          <label>LDAP Server</label>
          <div className={styles.fieldRow}>
            <input type="text" value={config.server} onChange={function(e) { sc('server', e.target.value) }} className={styles.fieldInput} />
            <input type="number" value={config.port} onChange={function(e) { sc('port', parseInt(e.target.value, 10)) }} className={styles.fieldInput} style={{ width: 80 }} placeholder="636" />
          </div>
        </div>
        <div className={styles.fieldGroup}>
          <label>Bind DN</label>
          <input type="text" value={config.bind_dn} onChange={function(e) { sc('bind_dn', e.target.value) }} placeholder="CN=service,CN=Users,DC=acme,DC=com" className={styles.fieldInput} />
        </div>
        <div className={styles.fieldGroup}>
          <label>Bind Password</label>
          <div className={styles.fieldInputWrap}>
            <input type={showPw ? 'text' : 'password'} value={config.bind_password} onChange={function(e) { sc('bind_password', e.target.value) }} placeholder="Password" className={styles.fieldInput} />
            <button onClick={function() { setShowPw(!showPw) }} className={styles.fieldEye} style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex', color: 'var(--text-muted)' }}>
              {showPw ? <EyeOff size={13} /> : <Eye size={13} />}
            </button>
          </div>
        </div>
        <div className={styles.fieldGroup}>
          <label>Base DN</label>
          <input type="text" value={config.base_dn} onChange={function(e) { sc('base_dn', e.target.value) }} placeholder="DC=acme,DC=com" className={styles.fieldInput} />
        </div>
        <div className={styles.fieldGroup}>
          <label>User Search Filter</label>
          <input type="text" value={config.user_filter} onChange={function(e) { sc('user_filter', e.target.value) }} placeholder="(objectClass=user)" className={styles.fieldInput} />
        </div>
        <div className={styles.fieldGroup}>
          <label>Sync Interval (minutes)</label>
          <input type="number" value={config.sync_interval} onChange={function(e) { sc('sync_interval', parseInt(e.target.value, 10)) }} className={styles.fieldInput} style={{ width: 100 }} />
        </div>
      </div>

      <div className={styles.ldapActions}>
        <div className={styles.sslToggle}>
          <input type="checkbox" checked={config.use_ssl} onChange={function(e) { sc('use_ssl', e.target.checked) }} className={styles.toggle} id="ldap_ssl" />
          <label htmlFor="ldap_ssl">Use LDAPS (SSL/TLS)</label>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn-secondary" onClick={handleTest} disabled={testing} style={{ fontSize: 12, padding: '6px 14px' }}>
            {testing ? <><RefreshCw size={12} className={styles.spin} /> Testing…</> : 'Test Connection'}
          </button>
          <button className="btn-primary" style={{ fontSize: 12, padding: '6px 14px' }}>Save LDAP Config</button>
        </div>
      </div>
    </div>
  )
}

/* ── Team Sync ─────────────────────────────────────────────────────────────── */
function TeamSyncTab() {
  return (
    <div className={styles.tabPane}>
      <div className={styles.sectionHeader}>
        <div>
          <h3>Team Directory Sync</h3>
          <p>Automatically sync user teams and roles from your identity provider to Neev Remote.</p>
        </div>
        <button className="btn-primary" style={{ fontSize: 12, padding: '6px 14px' }}>
          <RefreshCw size={12} /> Sync Now
        </button>
      </div>
      <div className={styles.syncCard}>
        <div className={styles.syncStatus}>
          <div className={styles.syncDot + ' ' + styles.syncDotGreen} />
          <span>Last synced 2 hours ago</span>
        </div>
        <div className={styles.syncStats}>
          <div className={styles.syncStat}><span className={styles.syncStatNum}>847</span><span className={styles.syncStatLabel}>Users synced</span></div>
          <div className={styles.syncStat}><span className={styles.syncStatNum}>12</span><span className={styles.syncStatLabel}>Groups mapped</span></div>
          <div className={styles.syncStat}><span className={styles.syncStatNum}>3</span><span className={styles.syncStatLabel}>New since last sync</span></div>
        </div>
        <div className={styles.syncMappings}>
          <div className={styles.syncMappingTitle}>Active directory group mappings</div>
          {[['CN=IT-Ops,OU=Groups,DC=acme,DC=com', 'IT Operations'],
            ['CN=DevOps,OU=Groups,DC=acme,DC=com', 'DevOps'],
            ['CN=HelpDesk,OU=Groups,DC=acme,DC=com', 'Help Desk']].map(function(m, i) { return (
            <div key={i} className={styles.syncMapRow}>
              <code className={styles.syncMapAd}>{m[0]}</code>
              <ArrowRight size={12} className={styles.syncMapArrow} />
              <span className={styles.syncMapRa}>{m[1]}</span>
              <span className={styles.syncMapBadge}>Active</span>
            </div>
          )})}
        </div>
      </div>
    </div>
  )
}

/* ── Main EnterprisePage ───────────────────────────────────────────────────── */
var TABS = [
  { id: 'sso',   label: 'SSO Providers', icon: Key     },
  { id: 'ldap',  label: 'LDAP / AD',     icon: Database},
  { id: 'teams', label: 'Team Sync',     icon: Users   },
]

export function EnterprisePage() {
  var _useState5 = useState('sso')
  var activeTab    = _useState5[0]
  var setActiveTab = _useState5[1]
  var _useState6   = useState({})
  var ssoConfigs   = _useState6[0]
  var setSsoConfigs = _useState6[1]

  function handleSSOToggle(id, enabled) {
    setSsoConfigs(function(prev) { return Object.assign({}, prev, { [id]: Object.assign({}, prev[id] || {}, { enabled: enabled }) }) })
  }

  function handleSSOChange(id, cfg) {
    setSsoConfigs(function(prev) { return Object.assign({}, prev, { [id]: cfg }) })
  }

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Enterprise</h1>
          <p className="page-subtitle">SSO, LDAP/AD integration, and team directory sync</p>
        </div>
      </div>

      <div className={styles.tabBar}>
        {TABS.map(function(tab) {
          var Icon = tab.icon
          return (
            <button
              key={tab.id}
              className={styles.tabItem + (activeTab === tab.id ? (' ' + styles.tabActive) : '')}
              onClick={function() { setActiveTab(tab.id) }}
            >
              <Icon size={13} /><span>{tab.label}</span>
            </button>
          )
        })}
      </div>

      <AnimatePresence mode="wait">
        <motion.div key={activeTab} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }}>
          {activeTab === 'sso' && (
            <div className={styles.tabPane}>
              <div className={styles.sectionHeader}>
                <div>
                  <h3>Single Sign-On Providers</h3>
                  <p>Configure SSO to let users authenticate with your identity provider. SAML 2.0 and OIDC are supported.</p>
                </div>
              </div>
              <div className={styles.ssoGrid}>
                {SSO_PROVIDERS.map(function(p) { return (
                  <SSOProviderCard
                    key={p.id}
                    provider={p}
                    config={ssoConfigs[p.id]}
                    onChange={handleSSOChange}
                    onToggle={handleSSOToggle}
                  />
                )})}
              </div>
            </div>
          )}
          {activeTab === 'ldap' && (
            <div className={styles.tabPane}>
              <div className={styles.sectionHeader}>
                <div>
                  <h3>LDAP / Active Directory</h3>
                  <p>Connect Neev Remote to your on-premises LDAP or Active Directory server for user authentication and group sync.</p>
                </div>
              </div>
              <div className={styles.ldapCard}><LDAPConnector /></div>
            </div>
          )}
          {activeTab === 'teams' && <TeamSyncTab />}
        </motion.div>
      </AnimatePresence>
    </div>
  )
}