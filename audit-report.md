# Neev Remote — Full Application Audit

**Date:** 2026-08-20 · **Auditor:** Accio (automated + code review) · **Scope:** code, design, security, features, gaps, market comparison, launch readiness

**Stack audited:** Go signaling relay + host agent (`server/`, `agent/` ~22.6K LOC), Flutter host/viewer (`neev_remote/` ~18.4K LOC Dart), React web dashboard (`web/` ~11.7K LOC), Wails desktop client (`client/` ~6.5K LOC), packaging + deploy.

---

## 1. Executive Summary

**Verdict: NOT market-ready for public launch. The engineering core is strong and the feature set is beyond MVP — but a set of hard security issues, secrets hygiene problems, and unfinished product surfaces must be fixed first.**

What is genuinely good:
- Real E2E encryption (WebRTC DTLS-SRTP, relay cannot see screen data), P2P-first with TURN fallback, mTLS device certificates with reissue/revoke endpoints.
- Password brute-force lockout in the relay; Argon2id hashing; TOTP MFA; RBAC (admin/support/viewer); audit events logged.
- Deep, hard-won platform correctness: Windows secure-desktop (UAC/login/lock) capture via SYSTEM helper, macOS TCC handling, DPI-aware capture, user-switch session continuity, receiver-acked large-file flow control. This is the hardest part of a remote-desktop product and it works.
- Disciplined engineering culture: 27 locked decisions, regression checklist, working-features tracking, CI release workflows.

What blocks launch (P0):
1. **`neev-signing.p12` (a private key bundle) is committed to git** — the project's own memory says the repo is public and keys must never be committed. Must be purged from history and the cert rotated/revoked.
2. **Machine password stored in plaintext** at `C:\ProgramData\NeevRemote\machine.dat` (world-readable on Windows). Any local user can steal the machine ID + password and take over unattended remote access.
3. **TURN credentials are static and handed to anonymous callers** via the public `/api/v1/session/ice-servers` endpoint — relay bandwidth abuse / free-riding vector.
4. **Demo/mock code in production surfaces:** web Security page accepts MFA code `123456`, AI Assistant is 100% mock, Settings "Save" does nothing, dashboard widgets render mock arrays.
5. **Auth is OFF by default** (`AUTH_ENABLED=false` → `requireRole` middleware passes everyone through), and the documented prod deploy (`docker-compose.prod.yml`) mounts `config.prod.yaml` **which does not exist** — the prod path is untested/broken as documented.

---

## 2. Code Quality Audit

### 2.1 Go backend (server + agent)

| Severity | Finding | Location |
|---|---|---|
| Medium | Swallowed send errors throughout the signaling hub (send failures ignored → state divergence) | `server/signaling/hub.go:359,365,383,412` |
| Medium | One goroutine per connected agent every 30s for pings — bursts at 10k+ agents | `server/signaling/hub.go:944` |
| Medium | God-files: `agent/session/transport.go` (1378 lines: WebRTC + IPC + consent + input + audio), `agent/session/consentwin_windows.go` (1019 lines GDI) | `agent/session/` |
| Low | `go vet` warning: mDNS address format breaks on IPv6 | `agent/network/mdns.go:114` |
| Low | `go.sum` is in `.gitignore` — breaks reproducible builds | `.gitignore` |

**Tests:** `server/` has **one** tested package (`signaling`); `server/api`, `server/auth`, `server/config`, `server/session` (registry, audit, certificates) have **zero tests** — the security-critical packages are untested. `agent/` is better: 23 test files covering audio (opus/pcmu/echo/mix), record/WebM, ICE, IPC, consent, file receive.

### 2.2 Flutter client (`neev_remote/`)

| Severity | Finding | Location |
|---|---|---|
| Medium | 50+ empty `catch (_) {}` blocks in `remote_service.dart` / `webrtc_service.dart` — critical failures silenced, field-debugging impossible | `lib/data/services/` |
| Medium | Heavy use of `late final` on native-plugin-backed services — `LateInitializationError` risk on partial teardown | `remote_service.dart` |
| Low | Known dead code: `CommandActivityPanel` (nothing instantiates it), `lib/window_manager.dart` is a stub | `lib/presentation/` |
| Low | `sprintf` for ID generation (buffer-overflow risk if format changes) | `windows/service/neev_helper.cpp:170` |

**Tests:** 18 Dart test files — good coverage of view-only, unattended access, privacy teardown. Gaps: no tests for `FileStore` path handling or signaling retry backoff.

### 2.3 Web dashboard (`web/`)

- Solid React structure (12 pages), virtualized device table, real API calls for Devices/Sessions/Analytics/Viewer.
- **Mock/incomplete:** Dashboard activity + AI recommendations (`genSparkData`), SecurityPage RBAC (UI-only), MFA (demo stub), AIAssistant (all `MOCK_RESPONSES` + simulated timers), Settings (Save → `console.log`). These violate the project's own Data Honesty Rule in the web layer.

### 2.4 Wails client (`client/`)

- Clean Go backend (`pion/webrtc`), reuses web frontend via `sync-frontend.js`. **Architecture risk:** it is a second, parallel implementation alongside the Flutter app — two host/viewer codebases to maintain and keep feature-parity.

---

## 3. Design Audit

**Verdict: good system, drifted implementation, broken brand consistency.**

| Severity | Finding | Evidence |
|---|---|---|
| Medium | Theme tokens drifted from spec: code uses background `#0F1115`, accent `#FF6B00`; DESIGN.md v4 "Obsidian" specifies canvas `#131210`, ember accent `#FF6A32` | `lib/core/theme/app_theme.dart:55-65` vs `DESIGN.md` |
| Medium | DESIGN.md is internally contradictory: header declares Obsidian "active, user-approved" but the Decisions Log (2026-07-24) says Obsidian was **reverted** to v3 light — and the code is a third variant | `DESIGN.md` lines 3-13 vs 89 |
| Medium | Brand split: app icon/logo is **green**, theme accent is **orange** — documented as an open decision in PROJECT_MEMORY (2026-08-03) | `PROJECT_MEMORY.md` change log |
| Low | Data Honesty Rule: the documented connect-screen stage-checkmark violation (cosmetic 420ms timer) has been fixed (timer removed, replaced by real viewer phase) — PROJECT_MEMORY not yet updated; web layer still violates the rule with mock widgets | `home_command_center.dart` |

Positives: bundled fonts (Space Grotesk/Inter/JetBrains Mono) fix a real macOS typography bug; consent card is a designed native surface on both platforms; Data Honesty discipline exists in the Flutter app.

---

## 4. Security Audit

### 4.1 Security claims — verified against code

| Claim (README) | Status | Evidence |
|---|---|---|
| E2E DTLS-SRTP, relay blind | ✅ | WebRTC/pion transport, relay only forwards SDP/ICE |
| Argon2id hashing | ✅ (weak params, see below) | `agent/auth/auth.go` |
| JWT dashboard auth + RBAC | ✅ (auth off by default) | `server/api/auth.go:87-103` |
| MFA TOTP | ✅ real server-side verification | `server/auth/totp.go`, `store.go:268-272` |
| Agent mTLS + managed device trust (issue/rotate/revoke) | ✅ | `server/api/admin.go:222,252`, `server/auth/client_ca.go` |
| Audit trail in Redis | ✅ | 11 `AddAuditEvent` call sites |
| Brute-force protection | ✅ on agent IDs (5-strike) | `server/signaling/hub.go:169` |

### 4.2 Critical / High findings

| # | Severity | Finding | Evidence |
|---|---|---|---|
| S1 | **Critical** | `neev-signing.p12` committed to git (private key); `.gitignore` doesn't exclude it; project memory says repo is PUBLIC and keys must only live in GH secrets. If this cert was ever used for signing/notarization it must be treated as compromised | `git ls-files` → `neev-signing.p12` |
| S2 | **Critical** | Machine ID + password stored **plaintext** in `C:\ProgramData\NeevRemote\machine.dat`; ProgramData is world-readable by default → any local user can read credentials for unattended remote takeover. (macOS side is correctly 0600.) | `neev_helper.cpp:56,87,124` |
| S3 | **High** | Static TURN credentials (`agent`/`changeme_in_prod`) served to **anonymous** callers via public `GET /api/v1/session/ice-servers` — no short-lived TURN REST creds; free relay bandwidth for anyone + leaked credential | `server/api/server.go:146-165`, `deploy/turnserver.conf` |
| S4 | **High** | Dashboard login has **no rate limiting** (only agent-ID lockout exists) | `server/api/auth.go` login |
| S5 | **High** | Auth bypass-by-default: `requireRole` calls `c.Next()` when `AUTH_ENABLED=false` (the default) → all `/admin/*` and `/dashboard/*` routes open on a default deploy | `server/api/auth.go:87-103`, README env table |
| S6 | **High** | Documented prod deploy broken: `docker-compose.prod.yml` mounts `deploy/config.prod.yaml` — **file does not exist** | `deploy/` listing |
| S7 | **High** | WebSocket relay has no message-size limit → memory DoS on the signaling server | `server/api/server.go:84` |
| S8 | **High** | Argon2id `time=1` — below OWASP minimum (t=3, m=64MB) for credential hashing | `agent/auth/auth.go:13` |
| S9 | **High** | No JWT revocation (no jti blocklist); tokens live in `localStorage` in the web app (XSS-exposed) | `server/auth/token.go`, `web/src/lib/api.js:2` |
| S10 | **High** | Localhost IPC (`127.0.0.1:47921` UAC, `47922` clipboard) is **unauthenticated** — any local process can inject input or read clipboard data | `neev_helper.cpp` |
| S11 | **High** | Prebuilt binary committed: `agent/encode/windows_lib/lib/libvpx.a` — unverifiable supply chain; build from source in CI instead | `git ls-files` |
| S12 | **Medium** | `dump.rdb` (Redis snapshot) committed to git — potential session/registry data leak | `git ls-files` |
| S13 | **Medium** | Weak defaults in config: JWT `CHANGE_ME_IN_PRODUCTION`, TURN `changeme`, DB DSN with credentials + `sslmode=disable` | `server/config/config.go:107-117`, `deploy/config.dev.yaml` |
| S14 | **Medium** | Web MFA demo stub accepts `123456`; `placeholder="123456"` in login forms | `web/src/pages/SecurityPage.jsx:249,254` |
| S15 | **Medium** | Hardcoded internal IP `172.17.17.77` in UI text, CI workflow, and comments | `home_command_center.dart:1178,1205`, `.github/workflows/flutter.yml:18` |
| S16 | **Low** | `file_store_io.dart` sanitizer doesn't explicitly reject `..` (exploitability limited by exclusive-create-on-directory failing, but should be defense-in-depth) | `neev_remote/lib/data/services/file_store_io.dart:39-42` |
| S17 | **Low** | `web/` API base URL defaults to `localhost:8080` when `VITE_API_URL` missing — production build misconfig risk | `web/src/lib/api.js:28-32` |
| S18 | **Open question** | Windows binary signing: installer scripts build NSIS packages; no evidence of an EV/authenticode cert — unsigned Windows binaries trigger SmartScreen warnings that kill conversion. **Verify before launch.** | `packaging/windows/` |

---

## 5. Features & Gaps

### 5.1 Implemented (confirmed working / built)

Screen + input remote control (Win/macOS/Linux-X11) · unattended access + consent prompts (native, both hosts) · clipboard text/image/file (delayed-render paste) · bidirectional file transfer with ack flow control (large files HW-confirmed) · in-session chat · voice (PCMU 8kHz, host+viewer) · session recording (VP8→WebM, host-side) · privacy mode (blank + input lock, both OS) · secure-desktop/UAC/login-window capture (Win SYSTEM helper; macOS daemon) · user-switch session continuity (viewer auto-reconnect / TransportMode) · lock/logoff/reboot/SAS commands · LAN mDNS discovery · web viewer · dashboard: devices/sessions/analytics/audit log/downloads portal · RBAC + TOTP MFA · agent mTLS + fleet enrollment + device groups · installers (NSIS/.pkg/.deb) · self-hosted relay + TURN + Redis + Postgres.

### 5.2 Gaps vs ROADMAP.md (enterprise vision)

| ROADMAP item | Status |
|---|---|
| Dashboard widgets (stats, sparklines) | 🟡 partial — activity/AI-recs mock |
| Devices page, virtualized 10k | ✅ implemented |
| Remote session redesign (floating dock, monitor switch) | 🟡 partial — session view exists, in-session monitor switch absent |
| File transfer UI (queue, pause/resume, drag-drop manager) | 🟡 basic queue; no pause/resume, no folder transfer |
| Security module (policies editor, roles, MFA page, audit viewer) | 🟡 MFA/audit real; RBAC editor UI-only |
| Analytics module | ✅ charts real (usage trends) |
| AI Assistant (analysis/troubleshooting/copilot/summary) | ⛔ 100% mock |
| Settings restructure | ⛔ shell only |
| Enterprise: SSO/LDAP/SCIM, org/teams management | ⛔ missing (Teams page partial) |
| Performance: H.264/HW encode, adaptive bitrate | ⛔ VP8 only, fixed rate |
| Wayland host, iOS/Android apps | ⛔ |
| Auto-update | ⛔ stubs |
| REST API for automation | 🟡 dashboard API exists; no public automation API |
| Remote print, TCP tunnel, whiteboard | ⛔ |

### 5.3 Gaps vs core remote-desktop parity (docs/anydesk-comparison.md is stale — dated 2026-07-01)

The comparison doc's "P0 gaps" (file transfer, recording, chat, reboot) have since been **closed** per PROJECT_MEMORY working features. Remaining real parity gaps: **H.264/hardware encode + adaptive bitrate** (biggest quality gap), **Wayland**, **mobile apps**, **multi-monitor in-session switching**, **auto-update**, **REST automation API**, **SSO**.

---

## 6. Market Comparison (data fetched 2026-08-20)

### 6.1 Pricing

| Vendor | Free tier | Paid pricing | Source |
|---|---|---|---|
| **AnyDesk** | Personal use, free | Solo ₹1,080/mo; Standard ₹1,536.80/mo (1 conn); Advanced ₹3,428.80/mo (2 conns) — annual billing, ex-GST (IN) | [anydesk.com/en/pricing](https://www.anydesk.com/en/pricing) |
| **TeamViewer** | Non-commercial only; limited (3 managed devices, 1 file at a time, no WOL) | Remote Access $24.90/mo; Business $50.90/mo (1 user, 200 devices); Premium $112.90/mo (15 users, 300 devices) — billed annually | PCMag TeamViewer review, 2026; teamviewer.com |
| **RustDesk** | **OSS, fully free, self-hosted** | Server Pro subscriptions (Individual / Basic / Customized; +$1.20/user, +$0.12/device) | [rustdesk.com/pricing](https://rustdesk.com/pricing/) |
| **Splashtop** | Personal, free | ~$5/user/mo billed annually (~$60/user/yr) | ITQlick pricing page, 2026 |
| **Zoho Assist** | Free edition (limited) | Remote Support: ₹400/tech/mo (Std), ₹600 (Pro), ₹960 (Ent); Unattended: ₹400–600 / 25 computers/mo — annual billing | [zoho.com/assist/pricing](https://www.zoho.com/assist/pricing.html) |
| **Chrome Remote Desktop** | Free (Google) | Free; org features via Google Workspace | PCMag/Google |
| *Reference: RemotePC* | — | Individual from $29.50/yr; 5 PCs $79.50/yr | PCMag review |

### 6.2 Positioning analysis

- **Direct open-source threat: RustDesk** (113K+ GitHub stars, self-hosted server, web client, Wayland support, mobile apps, custom branding, 90+ config options). Neev's self-hosted story collides with a free, far more mature alternative. **Neev's edge over RustDesk:** managed device trust (mTLS + CA rotation/revocation), RBAC + MFA + audit log, secure-desktop/UAC capture, session recording, voice, and a real management dashboard — RustDesk's OSS server has none of the management plane.
- **vs AnyDesk/TeamViewer/Splashtop:** Neev's control plane (RBAC/MFA/mTLS/audit) beats the free tiers and matches paid tiers; quality (VP8 fixed-rate, no HW encode) and reach (no mobile, no Wayland) lag; brand/track record absent.
- **Price whitespace:** self-hosted + control plane + ₹/INR-friendly pricing could undercut AnyDesk's ₹1,080–3,400/mo and Zoho's ₹400–960/tech/mo for MSPs that want on-prem data control. But **no pricing/GTM exists yet** (no license model, no billing, no LICENSE file despite README claiming MIT).

---

## 7. Market-Ready Launch Status

**Readiness: 3 / 10 — "can't launch yet".**

### P0 — Must fix before any public launch
1. **Purge `neev-signing.p12` and `dump.rdb` from git history** (git-filter-repo/BFG); rotate/revoke the cert; add ignore rules + a CI secrets scan. (S1, S12)
2. **Protect machine credentials at rest** — DPAPI-encrypt `machine.dat` on Windows (or restrict ACLs to SYSTEM); verify macOS 0600 permissions everywhere. (S2)
3. **Fix TURN credential model** — coturn REST API time-limited credentials; never serve the static secret anonymously. (S3)
4. **Remove demo/mock code from production web paths** — MFA `123456` stub, AI mock, Settings console.log, dashboard mock arrays; gate behind dev flag or delete. (S14, §2.3)
5. **Default auth ON for production** and make the prod compose actually work (create `config.prod.yaml`, add login rate limiting). (S5, S6, S4)
6. **Wire-format hardening:** WebSocket message size cap on the relay. (S7)

### P1 — Before scaling beyond a pilot
7. JWT revocation (jti + Redis blocklist) + auth middleware tests. (S9)
8. Authenticate localhost IPC (shared token/handshake on 47921/47922). (S10)
9. Argon2id params → t=3/m=64MB; add login rate limiter. (S8, S4)
10. Tests for `server/auth` + `server/api` (auth is the most critical untested code). (§2.1)
11. Resolve branding: one accent (icon/theme/spec) + fix DESIGN.md contradiction. (§3)
12. Confirm Windows code signing (SmartScreen) + macOS notarization path for the Flutter app. (S18)
13. Remove stray `test_*.go` root files and hardcoded `172.17.17.77` references. (S15)
14. Decide the two-client question: Flutter vs Wails — one canonical host/viewer. (§2.4)

### P2 — Differentiators worth building next
15. Adaptive bitrate + H.264/HW encode (biggest UX gap vs AnyDesk/TeamViewer).
16. Mobile viewer (Flutter makes iOS/Android incremental) — MSP mobility.
17. Wayland host (PipeWire/libei); in-session monitor switching; folder transfer.
18. Auto-update; REST automation API; SSO/LDAP; LICENSE file + pricing page.
19. Web-layer Data Honesty cleanup (mock widgets) before the dashboard is customer-facing.

---

## 8. Conclusion

The core transport (P2P WebRTC, secure-desktop capture, file transfer, consent model, user-switch continuity) is genuinely strong and beyond MVP — this is the hard 80% of a remote-desktop product, done. What stands between this and launch is **security hygiene and product finish, not engineering capability**: fix the five P0 items above (roughly 1–2 focused sprints), and the product is launchable for a self-hosted/IT-support niche — with RustDesk as the main open-source competitor and the management plane (mTLS + RBAC + MFA + audit + secure-desktop) as the honest differentiator against it.

---

## Appendix A — Verified claims & test inventory
- `go test ./...` (server): signaling ✅ passes; 4 packages without tests.
- `go vet`: 1 warning (mDNS IPv6).
- Flutter: 18 test files; Go agent: 23 test files; server: 1 package.
- Git: 3 CI workflows (`build.yml`, `flutter.yml`, `agent-windows.yml`); macOS signing secrets referenced from GH secrets (good) but `.p12` also committed (bad).
- No LICENSE file present despite README MIT claim.

## Appendix B — Sources
- AnyDesk pricing: https://www.anydesk.com/en/pricing (fetched 2026-08-20)
- TeamViewer pricing/features: PCMag review https://www.pcmag.com/reviews/teamviewer (2026) + teamviewer.com pricing overview (JS-gated, numbers from PCMag)
- RustDesk: https://rustdesk.com/pricing/ (fetched 2026-08-20); rustdesk.com
- Splashtop: https://www.itqlick.com/splashtop-remote-desktop/pricing (2026)
- Zoho Assist: https://www.zoho.com/assist/pricing.html (fetched 2026-08-20)
- Project docs: README.md, DESIGN.md, ROADMAP.md, PROJECT_MEMORY.md, docs/anydesk-comparison.md (2026-07-01, stale)
