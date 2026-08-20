# Neev Remote

Cross-platform remote-desktop app: Flutter client (macOS + Windows) over a Go
signaling relay, with a Go agent/daemon for unattended + session-switch hosting.

**Read `PROJECT_MEMORY.md` before changing anything.** It holds the Locked
Decisions (LD-1..LD-14), the per-platform-pair working-feature status, and the
change log. Windows-to-Windows is the stable baseline and must never regress.

## Design System

Always read `DESIGN.md` before making any visual or UI decision.
All font choices, colors, spacing, radii and aesthetic direction are defined there.
Do not deviate without explicit user approval.
In QA mode, flag any code that doesn't match `DESIGN.md`.

**Data Honesty Rule (binding):** never render a metric the app does not actually
measure. No placeholder counts, no invented latency/uptime figures. If the data
source doesn't exist yet, omit the tile. See `DESIGN.md` for which tiles are
blocked on roadmap Phase 2 (audit log) and Phase 4 (central console).

## Skill routing

When the user's request matches an available skill, invoke it via the Skill tool. When in doubt, invoke the skill.

Key routing rules:
- Product ideas/brainstorming → invoke /office-hours
- Strategy/scope → invoke /plan-ceo-review
- Architecture → invoke /plan-eng-review
- Design system/plan review → invoke /design-consultation or /plan-design-review
- Full review pipeline → invoke /autoplan
- Bugs/errors → invoke /investigate
- QA/testing site behavior → invoke /qa or /qa-only
- Code review/diff check → invoke /review
- Visual polish → invoke /design-review
- Ship/deploy/PR → invoke /ship or /land-and-deploy
- Save progress → invoke /context-save
- Resume context → invoke /context-restore
- Author a backlog-ready spec/issue → invoke /spec
