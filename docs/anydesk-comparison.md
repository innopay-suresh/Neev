# Neev Remote vs AnyDesk — Feature Comparison & Phased Roadmap

**Purpose:** planning input for next phases.
**Target user:** IT support / MSP technicians (weighting below reflects this).
**Focus dimensions (all selected):** Enterprise/security, Performance/UX, Collaboration, Reach/platforms.
**Baseline:** derived from a code inventory of `neev_remote/` (Flutter app), `server/` (Go signaling + API), `agent/` (Go host agent), `web/` (React portal). Date: 2026-07-01.

Legend — Status: ✅ Have · 🟡 Partial · ⛔ Gap. Priority (for MSP): **P0** table-stakes · **P1** important · **P2** nice-to-have. Effort: **S** ≈ days · **M** ≈ 1–2 wks · **L** ≈ weeks · **XL** ≈ 1–2 months.

---

## 1. TL;DR

**Where Neev already competes (often ahead of AnyDesk's free tier):**
- End-to-end encryption (WebRTC DTLS-SRTP), TLS signaling, **certificate-based device trust + revocation**, **MFA (TOTP)**, **RBAC**, **audit logging**, **unattended enrollment with org/device groups**. This is a genuinely strong, enterprise-grade control plane.
- Working P2P + TURN relay, session-password auth (Argon2id) with brute-force lockout, cross-platform host (Win/mac/Linux), a real management dashboard, and the just-shipped **UAC / elevated / secure-desktop** control.

**Where AnyDesk is clearly ahead (the upgrade targets):**
1. **File transfer** — ⛔ none. *Single biggest MSP gap.*
2. **Remote reboot + auto-reconnect** — ⛔ none. MSP-critical for patching.
3. **Session recording** — ⛔ none. Needed for audit/compliance/MSP proof-of-work.
4. **In-session chat** — ⛔ none. Talking to the end user.
5. **Mobile apps (iOS/Android)** — ⛔ web-browser only. Technicians on the go.
6. **Performance codec** — 🟡 VP8 fixed, no adaptive bitrate, no H.264/HW encode.
7. **Privacy mode** (blank remote monitor + lock local input) — ⛔ none.
8. Multi-monitor in-session switching, clipboard files/images, Wake-on-LAN wiring, remote print, TCP tunneling, custom-branded MSI + mass deployment, Wayland.

**Recommended thrust:** Neev's differentiator is the security/management plane — keep leaning into it — but it will not feel like a real AnyDesk alternative to an MSP until **file transfer, remote reboot, session recording, and chat** exist. Those anchor Phase 1–2.

---

## 2. Feature comparison matrix

### Connectivity & performance
| Feature | Neev | AnyDesk | Gap / note |
|---|---|---|---|
| P2P with relay fallback | ✅ WebRTC + TURN | ✅ | At parity |
| Codec | 🟡 VP8 only, forced | ✅ DeskRT + H.264 (HW accel) | No HW encode → higher CPU/latency at 4K |
| Adaptive bitrate / quality scaling | ⛔ fixed 30fps, 1920×1200 cap | ✅ dynamic ABR | Feels worse on poor links |
| Multi-monitor: capture selection | 🟡 source select at start | ✅ | Works, but no in-session switch |
| Multi-monitor: in-session switch / "show all" | ⛔ | ✅ | P1 |
| LAN discovery (mDNS) | ⛔ always relay/ID | ✅ | P2 |
| Connection quality indicator / stats | ✅ fps/rtt/kbps/codec | ✅ | At parity |

### Access & session control
| Feature | Neev | AnyDesk | Gap / note |
|---|---|---|---|
| Session ID + password | ✅ Argon2id, rotating | ✅ | At parity |
| Unattended access | ✅ hash + enrollment | ✅ | At parity (UX can improve) |
| Address book / device groups | 🟡 dashboard device list | ✅ + favorites/recent/aliases | Needs "my devices" connect flow, favorites |
| Permission profiles / ACL (what a controller may do) | ⛔ | ✅ granular | P1 for enterprise |
| View-only / read-only session | 🟡 protocol allows, no UI | ✅ | Easy win |
| Privacy mode (blank remote screen + lock input) | ⛔ | ✅ | P1, MSP privacy |
| 2FA | ✅ dashboard TOTP | ✅ | At parity |

### Productivity / collaboration
| Feature | Neev | AnyDesk | Gap / note |
|---|---|---|---|
| File transfer / file manager | ⛔ | ✅ manager + drag-drop | **P0** |
| Clipboard: text | ✅ | ✅ | At parity |
| Clipboard: files/images | ⛔ text only | ✅ | P1 |
| In-session chat | ⛔ | ✅ + offline messages | **P0** |
| Session recording | ⛔ | ✅ auto/manual | **P0** (audit) |
| Remote reboot + auto-reconnect | ⛔ | ✅ incl. safe-mode | **P0** |
| Wake-on-LAN | 🟡 `agent/wol` stub, unwired | ✅ | P1 |
| Remote print | ⛔ | ✅ | P2 |
| Whiteboard / annotation | ⛔ | ✅ | P2 |
| Multiple simultaneous viewers | ✅ | ✅ | At parity |

### Reach / platforms
| Feature | Neev | AnyDesk | Gap / note |
|---|---|---|---|
| Host: Windows / macOS / Linux(X11) | ✅ | ✅ | At parity |
| Host: Linux Wayland | ⛔ | ✅ | P2 (growing importance) |
| Desktop viewer (Win/mac/Linux) | ✅ Flutter | ✅ | At parity |
| Web viewer | ✅ | 🟡 limited | Neev slightly ahead |
| Mobile apps (iOS/Android) | ⛔ browser only | ✅ full clients | **P0/P1** for MSP mobility |
| TCP tunnel / port-forward / VPN | ⛔ | ✅ | P2 (enterprise) |

### Security & management (Neev's strong suit)
| Feature | Neev | AnyDesk | Gap / note |
|---|---|---|---|
| E2E encryption | ✅ DTLS-SRTP | ✅ | At parity |
| Certificate-based device trust + revocation | ✅ | 🟡 (Enterprise only) | **Neev ahead** |
| RBAC + audit log | ✅ | 🟡 (paid tiers) | **Neev ahead / at parity** |
| Management console (devices/sessions/users) | ✅ | ✅ (paid) | At parity |
| Custom-branded client / MSI / namespace | 🟡 env-var branding | ✅ client generator | P1 for MSP white-label |
| Mass deployment (GPO/RMM) | 🟡 installer env | ✅ | P1 |
| REST API for automation | ⛔ | ✅ | P1 (MSP/RMM integration) |
| SSO / SAML / SCIM | ⛔ | ✅ (Enterprise) | P2 |
| On-premises / self-hosted | ✅ (already self-hosted) | 🟡 (Enterprise add-on) | **Neev ahead** |
| Auto-update | 🟡 stubs, not wired | ✅ | P1 |

---

## 3. Prioritized gap list (MSP lens)

**Must-have to be a credible AnyDesk alternative (P0):**
1. File transfer (bidirectional, drag-drop, queue).
2. Remote reboot + auto-reconnect (normal; safe-mode later).
3. Session recording (local capture → file; later cloud/audit link).
4. In-session chat.

**Strong differentiators / important (P1):**
5. Mobile viewer apps (Flutter already → iOS/Android is incremental).
6. Adaptive bitrate + H.264/hardware encode.
7. Privacy mode (blank monitor + lock remote input) + view-only toggle.
8. Address book UX: favorites, groups, "connect to my devices," online presence.
9. Clipboard files/images; multi-monitor in-session switch.
10. Custom-branded client + mass deployment + REST API (MSP white-label & RMM).
11. Auto-update; Wake-on-LAN wiring.

**Nice-to-have (P2):** Wayland host, remote print, TCP tunnel/VPN, whiteboard, mDNS LAN discovery, SSO/SAML.

---

## 4. Phased roadmap

### Phase 1 — "MSP table-stakes" (make it usable for real support work)
| Item | Effort | Why |
|---|---|---|
| **File transfer** over a dedicated WebRTC data channel (chunked, resume, progress) | L | #1 MSP gap; reuse the chunking pattern already built for UAC frames |
| **Remote reboot + auto-reconnect** (agent command + viewer re-dials same ID) | M | Patching workflow; agent is a service already |
| **View-only toggle** + **privacy mode** (blank remote monitor, lock local input) | M | Trust/consent; protocol already nearly supports view-only |
| **Clipboard: images/files** (extend existing clip channel) | S–M | Frequent in support |
| **In-session monitor switch** ("show all" / cycle) | M | Selection exists; add live switch |

### Phase 2 — "Trust & collaboration"
| Item | Effort | Why |
|---|---|---|
| **Session recording** (local MP4/frames → optional upload + audit link in dashboard) | L | Compliance/proof-of-work; MSP differentiator |
| **In-session chat** (+ offline message to device) | M | Talk to end user |
| **Address book UX**: favorites, groups, online presence, one-click "connect to my devices" | M | Turns the device list into a real MSP console |
| **Wake-on-LAN** wiring (dashboard button → agent/peer relays magic packet) | S–M | `agent/wol` stub already exists |
| **Permission profiles / ACL** (per-controller: control vs view, file, clipboard, reboot) | M | Enterprise/MSP governance |
| **Auto-update** (agent + app self-update, staged rollout) | M | Fleet hygiene |

### Phase 3 — "Reach & performance"
| Item | Effort | Why |
|---|---|---|
| **Mobile viewer apps (iOS/Android)** from the existing Flutter codebase (touch input, gestures) | L | Technician mobility; incremental from Flutter |
| **Adaptive bitrate** + **H.264 / hardware encode** path (keep VP8 fallback) | L | Matches AnyDesk "feels local," lowers CPU |
| **Wayland host** capture + input (PipeWire portal / libei) | L | Modern Linux desktops |
| **Web viewer touch/mobile optimization** | S–M | Better mobile-browser fallback |

### Phase 4 — "Enterprise & scale"
| Item | Effort | Why |
|---|---|---|
| **Custom-branded client generator** + **MSI** + mass deployment (GPO/RMM templates) | L | MSP white-label & fleet rollout |
| **REST API** (devices, sessions, connect tokens, audit) | M–L | RMM/PSA integration |
| **TCP tunnel / port-forward** (RDP/SSH over the P2P link) | L | Enterprise access |
| **SSO / SAML / SCIM**, license/billing, on-prem hardening | XL | Enterprise sales |
| **Remote print**, whiteboard/annotation | M each | Round out parity |

---

## 5. Effort vs impact (quick reference)

- **Highest impact, do first:** File transfer, Remote reboot, Session recording, Chat.
- **High impact, lower effort (quick wins):** View-only toggle, clipboard files/images, WoL wiring, in-session monitor switch, auto-update.
- **Strategic bets (bigger):** Mobile apps, adaptive/H.264, custom-branded MSI + REST API.
- **Lean on existing strength:** the security/management plane is ahead of AnyDesk free — market it, and layer ACL/permission profiles + REST API on top for MSP/enterprise deals.

---

## 6. Open items to verify before committing scope

1. **Two host paths?** There's a Flutter host (`neev_remote/`) *and* a Go host agent (`agent/`). Confirm which is canonical for hosting so features land in the right place (recording/file-transfer/reboot must target the shipping host).
2. **Dashboard "mock" data:** recent-activity timeline and AI recommendations appear to be placeholder — confirm which dashboard widgets are wired to real data.
3. **VP8-forced rationale:** VP8 was forced to fix a Windows→Windows blank-video bug. Any H.264 work must preserve that fix (feature-detect, don't regress).
4. **Existing `ROADMAP.md`:** reconcile this comparison with the repo's existing roadmap phases (this doc is AnyDesk-parity-focused; the repo roadmap may sequence differently).
