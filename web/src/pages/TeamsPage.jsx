import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  Users, Plus, Search, MoreHorizontal, Mail, Shield,
  UserCheck, UserX, Crown, X, ChevronDown, Building2,
  ArrowRight, Trash2, Settings
} from 'lucide-react'
import styles from './TeamsPage.module.css'

/* ── Mock data ─────────────────────────────────────────────────────────────── */
const INITIAL_TEAMS = [
  {
    id: 1, name: 'IT Operations', icon: '💻', color: '#4F8CFF',
    members: [
      { id: 1, name: 'Sarah Chen',      email: 'sarah.chen@acme.com',      role: 'Admin',  status: 'active',   avatar: 'SC' },
      { id: 2, name: 'Marcus Rodriguez', email: 'marcus.r@acme.com',       role: 'Admin',  status: 'active',   avatar: 'MR' },
      { id: 3, name: 'Emily Watson',     email: 'emily.watson@acme.com',   role: 'Member', status: 'active',   avatar: 'EW' },
      { id: 4, name: 'James Kim',        email: 'james.kim@acme.com',      role: 'Member', status: 'pending',  avatar: 'JK' },
    ],
  },
  {
    id: 2, name: 'DevOps', icon: '🔧', color: '#22c55e',
    members: [
      { id: 5, name: 'Alex Thompson',    email: 'alex.t@acme.com',        role: 'Admin',  status: 'active',   avatar: 'AT' },
      { id: 6, name: 'Priya Sharma',     email: 'priya.sharma@acme.com',  role: 'Member', status: 'active',   avatar: 'PS' },
      { id: 7, name: 'David Lee',        email: 'david.lee@acme.com',     role: 'Viewer', status: 'active',   avatar: 'DL' },
    ],
  },
  {
    id: 3, name: 'Help Desk', icon: '🎧', color: '#f59e0b',
    members: [
      { id: 8,  name: 'Rachel Green',    email: 'rachel.g@acme.com',      role: 'Member', status: 'active',   avatar: 'RG' },
      { id: 9,  name: 'Tom Wilson',      email: 'tom.wilson@acme.com',    role: 'Member', status: 'active',   avatar: 'TW' },
      { id: 10, name: 'Nina Patel',      email: 'nina.patel@acme.com',    role: 'Viewer', status: 'pending',  avatar: 'NP' },
    ],
  },
  {
    id: 4, name: 'Security Team', icon: '🔒', color: '#ef4444',
    members: [
      { id: 11, name: 'Alex Thompson',   email: 'alex.t@acme.com',        role: 'Admin',  status: 'active',   avatar: 'AT' },
      { id: 12, name: 'Sarah Chen',      email: 'sarah.chen@acme.com',    role: 'Admin',  status: 'active',   avatar: 'SC' },
    ],
  },
]

/* ── Role badge ────────────────────────────────────────────────────────────── */
function RoleBadge({ role }) {
  var cls = role === 'Admin' ? styles.roleAdmin : role === 'Viewer' ? styles.roleViewer : styles.roleMember
  return <span className={cls}>{role}</span>
}

/* ── Status dot ────────────────────────────────────────────────────────────── */
function StatusDot({ status }) {
  return <span className={styles.statusDot + ' ' + (status === 'active' ? styles.statusActive : styles.statusPending)} />
}

/* ── Invite modal ──────────────────────────────────────────────────────────── */
function InviteModal({ team, onClose, onInvite }) {
  var _useState = useState('')
  var email     = _useState[0]
  var setEmail  = _useState[1]
  var _useState2 = useState('Member')
  var role      = _useState2[0]
  var setRole   = _useState2[1]

  function handleSubmit(e) {
    e.preventDefault()
    if (!email.trim()) return
    onInvite(team.id, email.trim(), role)
    onClose()
  }

  return (
    <motion.div className={styles.modalOverlay} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={onClose}>
      <motion.div className={styles.modal} initial={{ opacity: 0, scale: 0.95, y: 10 }} animate={{ opacity: 1, scale: 1, y: 0 }} exit={{ opacity: 0 }} onClick={function(e) { e.stopPropagation() }}>
        <div className={styles.modalHeader}>
          <h3>Invite to {team.name}</h3>
          <button onClick={onClose} className={styles.modalClose}><X size={15} /></button>
        </div>
        <form onSubmit={handleSubmit} className={styles.modalBody}>
          <div className={styles.modalField}>
            <label>Email address</label>
            <div className={styles.emailInput}>
              <Mail size={14} />
              <input type="email" value={email} onChange={function(e) { setEmail(e.target.value) }} placeholder="colleague@acme.com" required autoFocus />
            </div>
          </div>
          <div className={styles.modalField}>
            <label>Role</label>
            <div className={styles.selectWrap}>
              <Shield size={13} />
              <select value={role} onChange={function(e) { setRole(e.target.value) }}>
                <option>Member</option>
                <option>Admin</option>
                <option>Viewer</option>
              </select>
              <ChevronDown size={12} />
            </div>
          </div>
          <div className={styles.modalActions}>
            <button type="button" onClick={onClose} className="btn-secondary">Cancel</button>
            <button type="submit" className="btn-primary"><Mail size={13} /> Send Invite</button>
          </div>
        </form>
      </motion.div>
    </motion.div>
  )
}

/* ── Team card ─────────────────────────────────────────────────────────────── */
function TeamCard({ team, onInvite }) {
  var _useState3 = useState(false)
  var expanded   = _useState3[0]
  var setExpanded = _useState3[1]
  var _useState4 = useState(false)
  var menuOpen   = _useState4[0]
  var setMenuOpen = _useState4[1]

  var activeCount = team.members.filter(function(m) { return m.status === 'active' }).length

  return (
    <motion.div className={styles.teamCard} layout initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}>
      <div className={styles.teamHeader}>
        <div className={styles.teamLeft}>
          <div className={styles.teamIcon} style={{ background: team.color + '18', border: '1px solid ' + team.color + '30' }}>
            <span style={{ fontSize: 18 }}>{team.icon}</span>
          </div>
          <div>
            <div className={styles.teamName}>{team.name}</div>
            <div className={styles.teamMeta}>{team.members.length} members · {activeCount} active</div>
          </div>
        </div>
        <div className={styles.teamActions}>
          <button className={styles.inviteBtn} onClick={function() { onInvite(team) }}>
            <Plus size={13} /> Invite
          </button>
          <div className={styles.menuWrapper}>
            <button className={styles.menuBtn} onClick={function() { setMenuOpen(!menuOpen) }}>
              <MoreHorizontal size={15} />
            </button>
            <AnimatePresence>
              {menuOpen && (
                <motion.div className={styles.menu} initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0 }}>
                  <button onClick={function() { setMenuOpen(false); setExpanded(!expanded) }}>
                    <Users size={13} /> {expanded ? 'Hide' : 'Show'} members
                  </button>
                  <button onClick={function() { setMenuOpen(false) }}>
                    <Settings size={13} /> Team settings
                  </button>
                  <button className={styles.menuDanger} onClick={function() { setMenuOpen(false) }}>
                    <Trash2 size={13} /> Delete team
                  </button>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </div>
      </div>

      {/* Member preview */}
      <div className={styles.memberPreview} onClick={function() { setExpanded(!expanded) }}>
        {team.members.slice(0, 4).map(function(m) { return (
          <div key={m.id} className={styles.avatar} style={{ background: m.status === 'active' ? '#4F8CFF22' : '#9ca3af22' }} title={m.name}>
            {m.avatar}
          </div>
        )})}
        {team.members.length > 4 && (
          <div className={styles.avatarMore}>+{team.members.length - 4}</div>
        )}
        <ArrowRight size={12} className={styles.expandArrow + (expanded ? (' ' + styles.expandArrowOpen) : '')} />
      </div>

      {/* Expanded member list */}
      <AnimatePresence>
        {expanded && (
          <motion.div className={styles.memberList} initial={{ opacity: 0, height: 0 }} animate={{ opacity: 1, height: 'auto' }} exit={{ opacity: 0, height: 0 }}>
            {team.members.map(function(m) { return (
              <div key={m.id} className={styles.memberRow}>
                <div className={styles.memberAvatar} style={{ background: m.status === 'active' ? '#4F8CFF22' : '#9ca3af22', color: m.status === 'active' ? '#4F8CFF' : '#9ca3af' }}>
                  {m.avatar}
                </div>
                <div className={styles.memberInfo}>
                  <div className={styles.memberName}>
                    {m.name}
                    {m.role === 'Admin' && <Crown size={11} className={styles.crownIcon} />}
                  </div>
                  <div className={styles.memberEmail}>{m.email}</div>
                </div>
                <RoleBadge role={m.role} />
                <div className={styles.memberStatus}>
                  <StatusDot status={m.status} />
                  <span>{m.status === 'active' ? 'Active' : 'Pending'}</span>
                </div>
                <button className={styles.memberAction} title={m.status === 'pending' ? 'Resend invite' : 'Remove'}>
                  {m.status === 'pending' ? <Mail size={13} /> : <UserX size={13} />}
                </button>
              </div>
            )})}
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  )
}

/* ── Create team modal ─────────────────────────────────────────────────────── */
function CreateTeamModal({ onClose, onCreate }) {
  var _useState5 = useState('')
  var name       = _useState5[0]
  var setName    = _useState5[1]
  var _useState6 = useState('')
  var icon       = _useState6[0]
  var setIcon    = _useState6[1]

  function handleSubmit(e) {
    e.preventDefault()
    if (!name.trim()) return
    onCreate({ name: name.trim(), icon: icon.trim() || '📁' })
    onClose()
  }

  return (
    <motion.div className={styles.modalOverlay} initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={onClose}>
      <motion.div className={styles.modal} initial={{ opacity: 0, scale: 0.95, y: 10 }} animate={{ opacity: 1, scale: 1, y: 0 }} onClick={function(e) { e.stopPropagation() }}>
        <div className={styles.modalHeader}>
          <h3>Create Team</h3>
          <button onClick={onClose} className={styles.modalClose}><X size={15} /></button>
        </div>
        <form onSubmit={handleSubmit} className={styles.modalBody}>
          <div className={styles.modalField}>
            <label>Team name</label>
            <div className={styles.emailInput}>
              <Building2 size={14} />
              <input type="text" value={name} onChange={function(e) { setName(e.target.value) }} placeholder="e.g. Security Operations" required autoFocus />
            </div>
          </div>
          <div className={styles.modalField}>
            <label>Icon (emoji)</label>
            <input type="text" value={icon} onChange={function(e) { setIcon(e.target.value) }} placeholder="💻" maxLength={2} style={{ width: 60 }} />
          </div>
          <div className={styles.modalActions}>
            <button type="button" onClick={onClose} className="btn-secondary">Cancel</button>
            <button type="submit" className="btn-primary"><Plus size={13} /> Create Team</button>
          </div>
        </form>
      </motion.div>
    </motion.div>
  )
}

/* ── Main TeamsPage ────────────────────────────────────────────────────────── */
export function TeamsPage() {
  var _useState7 = useState(INITIAL_TEAMS)
  var teams      = _useState7[0]
  var setTeams   = _useState7[1]
  var _useState8 = useState('')
  var search     = _useState8[0]
  var setSearch  = _useState8[1]
  var _useState9 = useState(null)
  var inviteTeam = _useState9[0]
  var setInviteTeam = _useState9[1]
  var _useState10 = useState(false)
  var showCreate = _useState10[0]
  var setShowCreate = _useState10[1]

  function handleInvite(teamId, email, role) {
    setTeams(function(prev) {
      return prev.map(function(t) {
        if (t.id !== teamId) return t
        var newMember = {
          id: Date.now(),
          name: email.split('@')[0].replace(/[._]/g, ' ').replace(/\b\w/g, function(c) { return c.toUpperCase() }),
          email: email,
          role: role,
          status: 'pending',
          avatar: email.slice(0, 2).toUpperCase(),
        }
        return Object.assign({}, t, { members: t.members.concat([newMember]) })
      })
    })
  }

  function handleCreateTeam(data) {
    var colors = ['#4F8CFF', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4']
    var newTeam = {
      id: Date.now(),
      name: data.name,
      icon: data.icon,
      color: colors[Math.floor(Math.random() * colors.length)],
      members: [],
    }
    setTeams(function(prev) { return prev.concat([newTeam]) })
  }

  var filtered = teams.filter(function(t) { return t.name.toLowerCase().includes(search.toLowerCase()) })

  var totalMembers = teams.reduce(function(acc, t) { return acc + t.members.length }, 0)
  var activeMembers = teams.reduce(function(acc, t) { return acc + t.members.filter(function(m) { return m.status === 'active' }).length }, 0)

  return (
    <div className={styles.page}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Teams</h1>
          <p className="page-subtitle">{teams.length} teams · {totalMembers} total members · {activeMembers} active</p>
        </div>
        <button className="btn-primary" onClick={function() { setShowCreate(true) }}>
          <Plus size={14} /> Create Team
        </button>
      </div>

      {/* Search */}
      <div className={styles.searchBar}>
        <Search size={14} />
        <input type="text" placeholder="Search teams…" value={search} onChange={function(e) { setSearch(e.target.value) }} />
      </div>

      {/* Team cards */}
      <div className={styles.teamGrid}>
        {filtered.map(function(team) { return (
          <TeamCard key={team.id} team={team} onInvite={function(t) { setInviteTeam(t) }} />
        )})}
        {filtered.length === 0 && (
          <div className={styles.emptyState}>
            <Users size={32} strokeWidth={1.5} />
            <p>No teams found</p>
          </div>
        )}
      </div>

      {/* Modals */}
      <AnimatePresence>
        {inviteTeam && (
          <InviteModal team={inviteTeam} onClose={function() { setInviteTeam(null) }} onInvite={handleInvite} />
        )}
        {showCreate && (
          <CreateTeamModal onClose={function() { setShowCreate(false) }} onCreate={handleCreateTeam} />
        )}
      </AnimatePresence>
    </div>
  )
}