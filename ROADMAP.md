# RemoteAgent — AI-Powered Remote Operations Platform

## Project Overview

Transform the existing RemoteAgent application into a premium enterprise-grade AI-powered remote operations platform, surpassing solutions like AnyDesk, TeamViewer, Splashtop, Tailscale, and NinjaOne in UX, AI capability, and enterprise management.

---

## Design System

### Visual Style
- **Inspired by**: Linear, Notion, Arc Browser, Cloudflare Dashboard, Tailscale, Raycast
- **Theme**: Dark mode first (with light mode option)
- **Corners**: 12–16px border-radius
- **Shadows**: Soft, layered — `0 4px 24px rgba(0,0,0,0.4)`
- **Glassmorphism**: Frosted glass panels with `backdrop-filter: blur(20px)`
- **Typography**: Inter (UI), JetBrains Mono (code/mono)
- **Spacing**: 4px base unit, generous whitespace
- **Animations**: Spring-based easing, 200–350ms transitions

### Color Palette

| Token | Hex | Usage |
|---|---|---|
| `--bg-primary` | `#0F1117` | Main background |
| `--bg-secondary` | `#161A23` | Panel/card surfaces |
| `--bg-tertiary` | `#1E2330` | Elevated surfaces, hover |
| `--accent` | `#4F8CFF` | Primary actions, links, focus |
| `--accent-hover` | `#6BA0FF` | Accent hover state |
| `--success` | `#22C55E` | Online, connected, success |
| `--warning` | `#F59E0B` | Caution, medium severity |
| `--danger` | `#EF4444` | Error, disconnect, critical |
| `--text-primary` | `#FFFFFF` | Headings, primary content |
| `--text-secondary` | `#9CA3AF` | Labels, secondary info |
| `--text-muted` | `#6B7280` | Placeholders, disabled |
| `--border` | `#2A2F3C` | Subtle borders |
| `--border-active` | `#4F8CFF40` | Focus/active borders |

### CSS Variable Defaults

```css
--radius-sm: 8px;
--radius-md: 12px;
--radius-lg: 16px;
--radius-full: 9999px;
--shadow-sm: 0 1px 3px rgba(0,0,0,0.3);
--shadow-md: 0 4px 16px rgba(0,0,0,0.4);
--shadow-lg: 0 8px 32px rgba(0,0,0,0.5);
--shadow-glow: 0 0 24px rgba(79,140,255,0.25);
--t-fast: 0.15s cubic-bezier(0.4,0,0.2,1);
--t-med: 0.25s cubic-bezier(0.4,0,0.2,1);
--t-slow: 0.4s cubic-bezier(0.4,0,0.2,1);
--t-spring: 0.35s cubic-bezier(0.34,1.56,0.64,1);
```

---

## Main Application Layout

### Global Shell
- **Left sidebar** (64px collapsed / 240px expanded): icon + label navigation
- **Top bar** (56px): breadcrumb, search, notifications, user avatar
- **Content area**: full-height, scrollable, max-width contained

### Sidebar Navigation

| Icon | Label | Route |
|---|---|---|
| Grid | Dashboard | `/dashboard` |
| Monitor | Devices | `/devices` |
| Remote | Remote Access | `/remote` |
| Clock | Sessions | `/sessions` |
| Shield | Security | `/security` |
| Sparkles | AI Assistant | `/ai` |
| BarChart2 | Analytics | `/analytics` |
| Settings | Settings | `/settings` |

### Dashboard Widgets (2-column grid)

- **Online Devices** — count + sparkline
- **Active Sessions** — count + duration
- **Device Health** — average score + distribution bar
- **Security Alerts** — count + severity breakdown
- **Recent Activity** — timeline list
- **AI Recommendations** — prioritized action list

### Device Cards (Grid)

Each card shows: hostname, OS icon, CPU/RAM/Disk bars, last seen, status dot.
Actions on hover: Connect, File Transfer, Terminal, Restart, Shutdown.

---

## Module Roadmap

### Phase 1 — Design Foundation
- [ ] Replace `globals.css` with new design system tokens
- [ ] Update `NavBar` to use new sidebar (expandable)
- [ ] Rebuild `App.jsx` shell with sidebar + topbar layout
- [ ] Migrate all pages to new CSS module system
- [ ] Add `react-router-dom` routes for all sections
- [ ] Dark theme toggle (dark default, light option)

### Phase 2 — Dashboard & Device Management
- [ ] Redesign Dashboard page with widget grid
- [ ] Build Devices page with virtualized table (react-virtual)
- [ ] Device detail drawer/modal
- [ ] Filters: Online/Offline/OS/Tag/Department
- [ ] Search functionality
- [ ] Device grouping and tagging
- [ ] Device Health Score algorithm and display

### Phase 3 — Remote Session Redesign
- [ ] Replace `SessionView` toolbar with floating dock
- [ ] Auto-hide toolbar on inactivity
- [ ] Privacy Mode panel (blank screen, disable input)
- [ ] Multi-monitor selector
- [ ] Session notes panel
- [ ] Session timeline
- [ ] Fullscreen toggle

### Phase 4 — File Transfer Module
- [ ] Drag-and-drop file manager UI
- [ ] Transfer queue with pause/resume/cancel
- [ ] Progress indicators per file
- [ ] Transfer history log
- [ ] Multi-file and folder transfer

### Phase 5 — Security Module
- [ ] Access Policies page (RBAC rules editor)
- [ ] Roles: Admin, Support Engineer, Read Only, Auditor
- [ ] MFA setup page (TOTP)
- [ ] Session approval workflows
- [ ] Audit log viewer with filters and export

### Phase 6 — Analytics Module
- [ ] Usage trends chart (recharts)
- [ ] Connection trends chart
- [ ] Device health trends
- [ ] Engineer activity report
- [ ] Export to CSV/PDF

### Phase 7 — AI Module
- [ ] AI Assistant page with tabbed interface
- [ ] **AI Device Analysis**: pre-connection system scan → insight card
- [ ] **AI Troubleshooting**: issue selector → log analysis → root cause → fix steps
- [ ] **AI Session Copilot**: natural language → command suggestions during session
- [ ] **AI Session Summary**: auto-generate post-session report → export PDF
- [ ] Configurable AI providers (OpenAI, Anthropic, local)

### Phase 8 — Settings Restructure
- [ ] Settings → Application (General, Updates, Notifications)
- [ ] Settings → Connection (Network, Relay, Bandwidth)
- [ ] Settings → Security (Access, Permissions, Privacy, MFA)
- [ ] Settings → Sessions (Display, Audio, Recording, Clipboard)
- [ ] Settings → AI (Providers, Models, Prompts)
- [ ] Settings → Account (Profile, License, Organization)

### Phase 9 — Enterprise Features
- [ ] Organization management (teams, departments, device groups)
- [ ] SSO integration stubs (Entra ID, Okta, Google Workspace)
- [ ] LDAP/Active Directory connector page
- [ ] Invite/remove team members
- [ ] Device assignment to departments

### Phase 10 — Performance & Polish
- [ ] Virtualized device tables (10k+ devices)
- [ ] Lazy loading for analytics charts
- [ ] Background sync with stale-while-revalidate
- [ ] Service worker for offline shell
- [ ] Keyboard shortcuts (Raycast-style command palette)
- [ ] Final animation polish pass

---

## Existing Pages to Replace

| Current | Replacement |
|---|---|
| DashboardPage | New Dashboard with widgets |
| ViewerPage | Remote Access page + redesigned session view |
| DownloadsPage | File Transfer module |
| (missing) | Devices page (new) |
| (missing) | Sessions page (new) |
| (missing) | Security page (new) |
| (missing) | AI Assistant page (new) |
| (missing) | Analytics page (new) |
| (missing) | Settings pages (new) |

---

## Technical Notes

### Performance Targets
- Virtualized table: handle 10,000 rows at 60fps
- Session toolbar: render within 16ms frame budget
- AI insights: stream response, don't wait for full generation
- Lazy chunks: ViewerPage and Analytics split into separate bundles

### Key Libraries
- `react-router-dom` v6 — routing
- `recharts` — charts
- `@tanstack/react-virtual` — virtualized lists
- `framer-motion` — animations (already in use)
- `zustand` or `jotai` — global state (consider replacing Context)
- `react-hot-toast` or `sonner` — toast notifications
- `@radix-ui/react-*` — accessible primitives (dialog, dropdown, popover)

### File Structure (proposed)

```
web/src/
├── App.jsx                    # Shell with sidebar + router
├── components/
│   ├── Sidebar/
│   ├── TopBar/
│   ├── CommandPalette/        # Raycast-style Cmd+K
│   └── ui/                    # Shared design system components
├── pages/
│   ├── Dashboard/
│   ├── Devices/
│   ├── RemoteAccess/
│   ├── Sessions/
│   ├── Security/
│   ├── AIAssistant/
│   ├── Analytics/
│   └── Settings/
│       ├── Application/
│       ├── Connection/
│       ├── Security/
│       ├── Sessions/
│       ├── AI/
│       └── Account/
├── hooks/
├── stores/                    # Zustand/Jotai stores
├── lib/
├── styles/
│   └── globals.css            # Design system tokens
└── types/
```

---

## Success Criteria

- [ ] All 9 sidebar sections have functional pages
- [ ] Dark theme applied consistently across all components
- [ ] Device list renders 10k rows without jank
- [ ] Floating session toolbar with all 11 actions
- [ ] Privacy Mode blanks remote screen instantly
- [ ] AI Device Analysis shows pre-connection health check
- [ ] AI Troubleshooting generates root cause from logs
- [ ] File transfer UI shows queue with progress
- [ ] Audit log viewer with export
- [ ] SSO configuration UI complete
- [ ] Lighthouse performance score ≥ 90