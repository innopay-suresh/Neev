# Neev Remote — PROJECT MEMORY

Living record of decisions, working features, known problems, and change log for
the Neev Remote (Flutter host+viewer + Go/native helpers) remote-desktop tool.
Update this file with every substantive change. Keep it honest: a feature only
moves to **Working Features** after it is confirmed working on real hardware.

---

## Architecture (as of 2026-07-08)

- **Flutter app** (`neev_remote/`, `neev_remote.exe`) — runs in a **user session**.
  Is BOTH the host (screen capture via WebRTC `getDisplayMedia` → VP8, input
  forwarding, clipboard) and the viewer. Holds the **transport**: signaling
  WebSocket to the relay + WebRTC peer connection(s) + data channels
  (`webrtc_service.dart`, `signaling_service.dart`, `remote_service.dart`).
- **SYSTEM helper service** (`neev_remote/windows/service/neev_helper.cpp`,
  `neev_helper.exe`) — LOCAL SYSTEM, session 0. Does what a user-session process
  can't: capture the **secure desktop** (Winlogon: UAC / sign-in / lock) via GDI,
  inject into it, follow the active console session, mint/hold the machine-wide
  id+password, run a user-context clipboard agent, and (ServiceHost mode) launch
  the Flutter host into the active session. Talks to the app over localhost TCP
  `127.0.0.1:47921`; clipboard agent on `47922`.
- **Relay/signaling server** — Go, at `172.17.17.77:8080` (see deploy notes).
- **Old Go/pion agent** (`agent/`) — a prior session-independent host (pion
  WebRTC + DXGI capture). Superseded by the Flutter host, but the code remains
  and is the basis for the planned service-resident transport (see Locked
  Decisions → transport-in-service).

---

## Locked Decisions

- **LD-1 — The transport connection lives in a user-session process, so it
  cannot survive a user switch.** A user switch destroys the session and tears
  the Flutter host (and its WebRTC transport) down → disconnect. True seamless
  survival would require moving the transport into the persistent LocalSystem
  service (native Go/pion or C++), with capture as a swappable frame source.
  **DECISION (revised 2026-07-08): do NOT rewrite the transport in Go / do not
  change the Flutter agent for this.** Instead accept a brief (~2-3 s) drop and
  make the VIEWER auto-reconnect to the same machine-id across the switch
  (Dart-only). If true zero-disconnect is ever required, revisit the native
  service-transport — it is the only way, and it is a major re-architecture.
- **LD-2 — Secure-desktop capture stays in the SYSTEM helper (GDI on Winlogon).**
  It is proven and must not be rebuilt from scratch. The helper log confirms it
  captures + sends every secure-desktop frame correctly.
- **LD-3 — Normal input goes through the in-app injector; only the SYSTEM helper
  can reach the secure desktop / elevated windows.** Helper *normal-desktop*
  input routing was unreliable in the field and is disabled
  (`_kRouteNormalInputViaHelper=false`); input is force-routed through the helper
  only while the host is on a secure desktop.
- **LD-4 — Clipboard: announce-on-copy → deliver-on-paste for files** (no bytes on
  copy). Requires native delayed-render (Windows COM `IDataObject`), not pure
  Dart. Text/images sync-on-copy, paste with Ctrl+V. Master on/off toggle exists.
- **LD-5 — Input injection must run at SYSTEM/elevated integrity or it cannot
  type into elevated windows (UIPI).** A Medium-integrity injector is silently
  blocked from High-IL (admin) windows. Honored by routing input through the
  SYSTEM helper agent whenever the host's foreground window is elevated (helper
  detects it via `IsForegroundElevated`; app sets `_hostElevatedActive`).
- **LD-6 — For TRUE seamless survival of a user switch, the transport must live
  in the SYSTEM service and follow the new session id.** Not currently done
  (transport is in the user-session Flutter host — see LD-1). Until/unless that
  native re-architecture happens, user switches are handled by viewer
  auto-reconnect (brief drop, not seamless).

---

## Working Features (confirmed)

- Normal remote control host↔viewer (~20 fps), Windows/macOS/Linux.
- Clicks/drags correct (no click-becomes-drag; no dead clicks; no stuck-Alt →
  double-click opens files, not Properties).
- Discovery shows real machine names (LAN UDP + relay-assisted).
- File **copy** no longer becomes **move** (Preferred DropEffect = Copy).
- Clipboard text/image sync; clipboard sync on/off toggle.
- SYSTEM helper: secure-desktop capture + send (helper log verified 2026-07-08).

## Known Problems (open)

- **KP-1 — UAC prompt not shown on viewer (regression). FIX IMPLEMENTED
  2026-07-08 (pending hardware verify).** Root cause: NOT capture (helper log
  proves capture+send work every time). The UAC frames reached whichever single
  host held the helper's pipe, but the viewer may be connected (via the relay
  machine-id) to a *different* host process — ServiceHost mode launches its own
  host, and a user-opened app is a second host. Introduced by `17bdb0f`. Fix:
  the helper now broadcasts frames to ALL connected pipe clients
  (`neev_helper.cpp`: `g_client` → `g_clients` + per-client reader threads), so
  whichever host the viewer watches gets the overlay. No Flutter/app change.
- **KP-2 — Full disconnect on user switch. FIX IMPLEMENTED 2026-07-08 (pending
  hardware verify).** The service does `TerminateProcess(host)` + relaunch on
  session change (log: `active session X -> Y; relaunching host`); the transport
  lives in that host, so it dies. Per LD-1 (revised: no Go rewrite), fixed in
  Dart: the viewer now enables auto-reconnect after a successful connect and
  re-dials the SAME machine-id when the peer drops (fast 2 s retries, then 5 s).
  The relaunched host re-registers the machine-id, so the viewer reconnects into
  the new user's desktop. Result: brief ~2-5 s drop, NOT seamless (true seamless
  would need the native service-transport, deliberately not done).
- **KP-3 — Clipboard files on-paste (delayed render) is v1, unverified on
  hardware.** Compiles; paste correctness / large / multi-file / timeout need
  real-Windows testing before it becomes a Working Feature.

---

## Change Log

- **2026-07-08 — DECISION: build industry-standard transport-in-SYSTEM-service
  (reverses "no Go").** To compete with AnyDesk/TeamViewer (zero-drop user
  switch, always-on unattended), the transport must live in a persistent
  LocalSystem service with capture as a swappable per-session worker. Survey of
  `agent/` (Go/pion) found ~80% already exists: full pion WebRTC host, signaling
  (id+password, reconnect, mTLS), DXGI+GDI capture, VP8 encode w/ ABR, input,
  clipboard. Gap = service/session layer, most of which `neev_helper.cpp` has
  (WTS session follow, CreateProcessAsUser). Plan: combine the two halves.
  Phase 0 PoC milestones: (1) Go host builds in CI ✓, (2) split Go into
  persistent `--transport` + per-session `--capture-worker` over local IPC,
  (3) neev_helper launches transport once + swaps worker on session change,
  (4) prove one live frame surviving a user switch. Guardrail: shipping Flutter
  host is untouched and stays default until parity + user-approved cutover.
  **M1 DONE 2026-07-08:** isolated `.github/workflows/agent-windows.yml` builds
  the Go host on Windows (CGO + bundled `agent/encode/windows_lib/lib/libvpx.a`;
  mingw gcc path resolved dynamically). The old build.yml failure was a
  misconfigured workflow (pkg-config for ffmpeg/x264 the agent never uses).
- **2026-07-08 — Issue: host "app closes, doesn't return" on user switch — root
  cause = DUAL HOST.** Helper log (17:55:39 switch) proved the service relaunches
  the host fine in the new session AND that elevated input works (`inject-fwd:
  key … sent=1`). The real problem: a manually-opened app window AND the service
  host both register the machine-id (two `tcp: client connected` per event).
  On a switch the visible manual host is stranded in the old (backgrounded)
  session while the service host moves on, and the viewer's reconnect ping-pongs
  between them. FIX: new `neev_remote/hostmode` channel (runner `host_mode.cpp`)
  reports {serviceInstance, serviceHostMode (HKLM reg)}; `HostMode.shouldAutoHost`
  makes a manually-opened window NOT host when ServiceHost mode is on — only the
  service instance hosts. Eliminates the dual host (also underlay KP-1 / elevated
  input). Native (runner + registry) + small Dart.
- **2026-07-08 — Issue: can't type in elevated windows (UIPI) — FIX (native +
  Dart).** Helper `neev_helper.cpp` now detects an elevated foreground window
  (`IsForegroundElevated`, `TokenElevation`) and sends state msg `'e'` (1/0) to
  the app. `remote_service.dart` sets `_hostElevatedActive` and routes ALL input
  through the SYSTEM helper while elevated (or on secure desktop), so admin
  windows receive input. Normal windows keep the fast in-app injector. See LD-5.
- **2026-07-08 — Issue: user switch disconnected FOREVER on r28 — FIX (Dart).**
  Root cause: the viewer's WebRTC peer entered ICE `disconnected` (not `failed`)
  when the host was killed on session change, and the reconnect only triggered
  on `failed`/`closed` → stuck. Now a 3 s grace on `disconnected` then treat as
  lost → the existing auto-reconnect re-dials the machine-id. Still brief-drop,
  not seamless (LD-6).
- **2026-07-08 — KP-2 fix: viewer auto-reconnect across user switch.**
  `remote_service.dart`: enable `autoReconnect` on successful connect (was
  reboot-only) + faster initial retries. Re-dials the same machine-id when the
  host is relaunched by the service on session change. Dart-only, no Go.
- **2026-07-08 — KP-1 fix: helper multi-client broadcast.** `neev_helper.cpp`
  pipe server now accepts multiple hosts and broadcasts secure-desktop frames to
  all of them (per-client reader threads). Restores UAC-on-viewer regardless of
  which host the viewer is connected to. Decided (LD-1 revised) NOT to move
  transport to Go; Issue 2 (user-switch) to be handled by viewer auto-reconnect.
- **2026-07-08 — Diagnosed KP-1 & KP-2 from helper log.** Helper capture/send
  confirmed flawless. Both issues traced to the host/transport being a
  duplicatable, session-bound process (regression `17bdb0f`). Approved: full
  transport-in-service (LD-1) via a PoC-first path; created this file.
- **2026-07-08 — Clipboard increment 2 (v1):** files announce-on-copy →
  deliver-on-paste via native COM delayed-render (`remote_file_drop.cpp`,
  dedicated STA thread) + eager fallback off-Windows. Ships, needs hardware test.
- **2026-07-08 — Clipboard increment 1:** master clipboard-sync on/off toggle.
- **2026-07-07 — r25:** release stuck modifier keys on focus loss (double-click →
  Properties fix). **r24:** file paste-as-move fix (Preferred DropEffect=Copy) +
  route input via helper while host on secure desktop. **r23:** stop routing
  normal input via helper (dead clicks) + mac accessibility re-check.
  **r22:** click-becomes-drag (cross-injector reordering) + discovery real names.
- **2026-07-07 — CI/publishing unblocked:** repo made public (free Actions) +
  purged 4 GB artifacts. NOTE: builds 07-03→07-06 never ran, so "r17–r21"
  installers were OLD code — bug reports from that window describe stale builds.

---

## Notes for future changes

- Never mark a feature "working" until confirmed on hardware (user request).
- The dev Mac cannot build/run the Windows native (helper, runner C++, Go/pion)
  or test secure-desktop/session behavior — those go via CI + user hardware.
- When a bug report contradicts the code, verify which build (commit SHA) the
  installer under test was actually built from.
