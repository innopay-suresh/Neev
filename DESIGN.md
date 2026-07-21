# Design System — Neev Remote

## Product Context
- **What this is:** Cross-platform remote-desktop control app (Flutter, macOS + Windows). Viewer connects to a host by 9-digit device ID + password; screen, input, clipboard and file transfer ride WebRTC.
- **Who it's for:** IT admins and support staff running unattended and attended remote sessions.
- **Space/industry:** Remote access / remote support. Peers: AnyDesk, TeamViewer, Splashtop.
- **Project type:** Desktop application (not a website). Dense, utility-first, used daily.

## Aesthetic Direction
- **Direction:** Warm bento dashboard. Cream canvas, white cards, coral accent, session thumbnails as the primary object.
- **Decoration level:** intentional — layered surfaces, real borders, one dark promo band. No gradients as decoration, no glassmorphism.
- **Mood:** Calm, warm, information-dense. Should feel like a tool that respects the operator's time, not a marketing page.
- **Why not the category default:** AnyDesk/TeamViewer/Splashtop all ship cool-grey utilitarian chrome. The warm cream canvas (`#EEEAE0`) is the deliberate departure — it is instantly recognisable next to competitors and makes white cards read as cards without heavy shadows.

## Typography
- **Display/Hero:** Space Grotesk 500/600 — page titles, section titles, card titles, stat values.
- **Body/UI:** Inter 400/500/600 — labels, meta, nav, buttons.
- **Data/IDs:** JetBrains Mono 500/600 — device IDs, passwords, build stamps, byte counts. Must use tabular figures.
- **Loading:** bundled as app assets in `pubspec.yaml` (NOT system fonts). This also fixes a real bug: the old theme requested `Segoe UI Variable Text`, which does not exist on macOS, so the Mac build had no typographic identity.
- **Scale:** 9/10/10.5/11/11.5/12/12.5/13/13.5/14/15/16/19/24 px. Titles 19 (page) and 15 (section); IDs 14; meta 10.5–11.5.

## Color
- **Approach:** restrained — one coral/orange accent, warm neutrals, three semantic hues used only for status. Retuned 2026-07-21 for the "Command Center" redesign (values below); token NAMES unchanged so widgets keep compiling.
- **Canvas:** `#F5F1E8` — warm cream page background.
- **Surface:** `#FFFEFB` — cards, sidebar, top bar; secondary surface `#F8F5EE`.
- **Ink (text):** `#171714`; secondary `#777266`; tertiary `#9A9385`.
- **Primary (orange):** `#F05A28`; deep/pressed `#C94418`; tint `#FCE5D9`.
- **Device grounds (card backgrounds, muted, never bright blue):** navy `#243B53`, forest `#294B3A`, plum `#543246`, walnut `#554332` (opt: slate `#3A4148`, teal `#2D4A49`, burgundy `#56363B`, olive `#4B4C36`). White/warm-grey device models over these.
- **Teal/green (success/online):** `#198764`, tint `#DDEFE7`.
- **Amber (favorite/warning):** `#D78A18`.
- **Error:** `#D8493F`. **Info slate:** `#53616D`.
- **Borders:** `#DED6C8` (hairline), `#D0C6AC` (strong / inputs).
- **Dark mode:** warm charcoal — canvas `#151512`, surface `#1E1E1A`, elevated `#282720`, border `#38362E`, text `#F6F2E8`/`#AAA395`/`#777166`, orange `#FF6A32`. Device grounds deeper + slightly desaturated. Never pure black/white.

## Spacing
- **Base unit:** 4px.
- **Density:** compact — this is an operator tool, not a marketing site.
- **Scale:** 2(2) xs(4) sm(8) md(12) lg(16) xl(22) 2xl(28) 3xl(34).
- **Sidebar width:** 216px. **Content padding:** 26px 34px. **Card padding:** 12–22px.

## Layout
- **Approach:** grid-disciplined bento.
- **Shell:** fixed 216px sidebar + fluid main, max content width 1220px.
- **Sidebar carries the device's OWN ID + password panel.** This is what removes the dead space that previously sat under a "This computer" card.
- **Main column order:** connect bar → session tile grid (`1.7fr 1fr 1fr 1fr`, featured tile first) → bento bottom (`1.7fr 1fr`) → promo band → footer.
- **Border radius:** sm 5–6px (chips, copy buttons), md 8–9px (inputs, nav, buttons), lg 12px (panels), xl 15px (cards, bands).
- **Shadows:** `sm: 0 1px 2px rgba(24,17,8,.04), 0 4px 10px -4px rgba(24,17,8,.06)`; `md: 0 2px 4px rgba(24,17,8,.05), 0 16px 32px -14px rgba(24,17,8,.14)`.

## Motion
- **Approach:** minimal-functional.
- **Easing:** enter ease-out, exit ease-in, move ease-in-out.
- **Duration:** micro 80ms (hover), short 160ms (press, nav), medium 240ms (panel/route). No entrance choreography, no scroll-driven animation.

## Data Honesty Rule (binding)
**Never render a metric the app does not actually measure.** Tiles are added only when their data source exists:
- Available now: recent connections (id, name, last-connected), favorites, session thumbnails (captured frame), unattended state, build stamp, security facts.
- Blocked on **roadmap Phase 2** (session audit log): sessions today, data transferred, uptime, session-activity chart.
- Blocked on **roadmap Phase 4** (central console): team presence.
- Needs wiring: per-device online/offline dot (relay presence), live latency (WebRTC `getStats`).
Placeholder or invented numbers are not acceptable in shipped builds.

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-07-20 | Initial design system created | /design-consultation. Direction supplied by user as a bento mockup; tokens extracted from it. |
| 2026-07-20 | Warm cream canvas `#EEEAE0` over cool grey | Old `#F8F8F9` sat ~3% off white so cards never read as cards; warm canvas also differentiates from AnyDesk/TeamViewer/Splashtop. |
| 2026-07-20 | Bundle Space Grotesk / Inter / JetBrains Mono as assets | `Segoe UI Variable Text` does not exist on macOS — the Mac build was falling back to a generic sans. |
| 2026-07-20 | Device ID is a designed object (JetBrains Mono, grouped, tabular) | It is the product's core noun and is read aloud over the phone. |
| 2026-07-20 | Own ID/password panel moves into the sidebar | Removes the ~300px of dead space under the old "This computer" card. |
| 2026-07-20 | Metric tiles omitted until their data exists | Data Honesty Rule; fabricated dashboard numbers destroy trust on first check. |
| 2026-07-21 | "Command Center" redesign direction approved (prototype) | User-supplied premium spec. Three-column shell: 88→240px expandable nav rail · connection-dock-led workspace · collapsible live-activity panel. Subtle 3D device cards, connection-sequence animation, command palette. Prototype: claude.ai/code/artifact/44999bc8. |
| 2026-07-21 | Palette retuned coral→orange `#E8622C`→`#F05A28`, canvas `#EEEAE0`→`#F5F1E8`, warm-dark mode defined | Aligns with the approved Command Center spec; token names kept stable so no widget breaks. |
| 2026-07-21 | Redesign honors Data Honesty Rule: latency/FPS/bandwidth/codec/CPU/MEM tiles gated on WebRTC getStats + host telemetry; multi-session/terminal/WoL/recording/MFA/per-permission gated on their (absent) backends | Same rule as before — the layout ships, fabricated-metric tiles and dead feature controls do not. |
