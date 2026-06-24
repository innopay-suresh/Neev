# Settings Restructure

## Goal
Redesign SettingsPage with 6 categories and section navigation in both `web/src/pages/Settings/` and `client/frontend/src/pages/Settings/`.

## Summary
Restructure the flat SettingsPage into a categorized settings system with 6 distinct categories: Application, Connection, Security, Sessions, AI, and Account. Each category becomes its own section with dedicated navigation.

## Current State
- `SettingsPage.jsx` is a single monolithic component (~10,400 lines)
- All settings are in one scrollable page
- No category-based navigation or filtering

## Target Structure

```
web/src/pages/Settings/
├── SettingsPage.jsx              # Main container with sidebar navigation
├── SettingsPage.module.css       # Shared styles
├── Application/
│   ├── ApplicationSettings.jsx   # General, Updates, Notifications
│   └── ApplicationSettings.module.css
├── Connection/
│   ├── ConnectionSettings.jsx    # Network, Relay, Bandwidth
│   └── ConnectionSettings.module.css
├── Security/
│   ├── SecuritySettings.jsx      # Access, Permissions, Privacy, MFA
│   └── SecuritySettings.module.css
├── Sessions/
│   ├── SessionSettings.jsx       # Display, Audio, Recording, Clipboard
│   └── SessionSettings.module.css
├── AI/
│   ├── AISettings.jsx            # Providers, Models, Prompts
│   └── AISettings.module.css
└── Account/
    ├── AccountSettings.jsx       # Profile, License, Organization
    └── AccountSettings.module.css
```

## 6 Categories (from ROADMAP.md Phase 8)

### 1. Application
- General (startup, language, theme)
- Updates (auto-update, check for updates)
- Notifications (push settings, email alerts)

### 2. Connection
- Network (timeout, retry logic)
- Relay (relay server URL, fallback)
- Bandwidth (quality presets, custom limits)

### 3. Security
- Access (session approval, connection limits)
- Permissions (RBAC roles)
- Privacy (screen capture permissions)
- MFA (TOTP setup)

### 4. Sessions
- Display (resolution, DPI scaling)
- Audio (input/output device selection)
- Recording (local recording, storage)
- Clipboard (sync direction, history)

### 5. AI
- Providers (OpenAI, Anthropic, local)
- Models (model selection per provider)
- Prompts (custom system prompts)

### 6. Account
- Profile (name, email, avatar)
- License (plan info, upgrade)
- Organization (team, department)

## Implementation Steps

1. Create `Settings/` folder structure in both `web/` and `client/frontend/`
2. Create `SettingsPage.jsx` container with:
   - Left sidebar showing 6 category icons/labels
   - Content area that renders selected category component
   - Active state highlighting
3. Extract current settings into 6 category components
4. Create CSS modules for each component using ROADMAP design tokens
5. Sync changes between `web/` and `client/frontend/`

## Files to Create/Modify

### web/src/pages/Settings/
- [ ] `SettingsPage.jsx` (refactor to container)
- [ ] `SettingsPage.module.css`
- [ ] `Application/ApplicationSettings.jsx`
- [ ] `Application/ApplicationSettings.module.css`
- [ ] `Connection/ConnectionSettings.jsx`
- [ ] `Connection/ConnectionSettings.module.css`
- [ ] `Security/SecuritySettings.jsx`
- [ ] `Security/SecuritySettings.module.css`
- [ ] `Sessions/SessionSettings.jsx`
- [ ] `Sessions/SessionSettings.module.css`
- [ ] `AI/AISettings.jsx`
- [ ] `AI/AISettings.module.css`
- [ ] `Account/AccountSettings.jsx`
- [ ] `Account/AccountSettings.module.css`

### client/frontend/src/pages/Settings/
- [ ] Same structure as above (sync after web/)

## Design System (from ROADMAP.md)
- Dark mode first with light option
- Border-radius: 12-16px
- Spacing: 4px base unit
- Typography: Inter (UI), JetBrains Mono (code)
- CSS tokens: `--bg-primary`, `--accent`, `--border`, etc.

## Next Steps
- Extract all existing settings fields from current `SettingsPage.jsx`
- Categorize each field into one of 6 categories
- Build container component with navigation
- Implement each category component