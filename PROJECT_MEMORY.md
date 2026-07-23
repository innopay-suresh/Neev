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
- **LD-7 — After any user-profile switch, the service spawns a worker into the
  new session and hands the viewer full screen + control automatically, with no
  prompt and no disconnect** (the AnyDesk model; session ends only on deliberate
  end-session or network loss). Implemented via the opt-in `TransportMode`
  seamless backend (built 2026-07-09, Phase A — pending hardware validation):
  the Go transport runs persistently in session 0 as SYSTEM (owns the WebRTC
  connection, never dies on a switch), and a per-session capture+**input**
  worker is spawned into the active session via `WTSQueryUserToken` →
  `DuplicateTokenEx` → `CreateProcessAsUser` on `winsta0\default` (runs AS the
  logged-in user, so SendInput lands on that user's desktop). On a profile
  switch only the worker is swapped; the viewer's peer + RTP stream continue, so
  no disconnect. The transport is the WebRTC OFFERER and auto-accepts (no
  consent dialog). Requires the native session-0 transport — NOT achievable in
  pure Dart (a user-session transport dies on the switch by definition, see
  LD-1). Clipboard/files over this path are Phase B (not yet carried).
- **LD-8 — Relay registration and the WebRTC connection happen ONCE in the
  SYSTEM service and persist across session changes — NEVER re-register on a
  switch.** In TransportMode the Go transport (session 0) registers the
  machine id+password one time and keeps that connection for its whole lifetime;
  a user switch must not run `startHosting` / re-register. (The default Flutter
  path violates this — it relaunches + re-registers the host per switch, which
  is the black-screen/login-break failure mode LD-8 exists to prevent. That path
  stays only as the non-seamless default.)
- **LD-11 — Exactly ONE host identity per machine, owned by the SYSTEM service;
  the user app is UI-only and never a connectable host.** In service-owned mode
  (TransportMode) the Flutter app must not register as a second host by ANY path
  — the guard lives inside `startHosting` (keyed off `HostMode.serviceOwnsHosting`),
  so auto-host, settings-reconnect, the Share button, and fixed-password all stay
  UI-only. The app still shows the machine id+password (from the helper) so users
  dial the single service transport. Prevents the split-brain where a viewer
  lands on a user-app host that has screen but no SYSTEM input.
- **LD-12 — The published machine ID is stable and service-owned — identical
  across users, account types, and app launches; never per-launch or
  per-profile.** The SYSTEM service mints `machine.dat` (`EnsureMachineId`) at
  startup BEFORE launching the transport, so the transport always advertises that
  id (never a relay-assigned fallback), on every laptop incl. first boot. The
  only per-install id (`_persistentAgentId`) is confined to the legacy
  Flutter-host mode, which is never used when the service owns hosting.
- **LD-10 — The capture worker must RETRY connecting to the transport (never
  fatally exit on dial-refused); session-swap must wait for the new worker to
  attach before retiring the old one — no zero-producer window.** The transport
  (session 0) may not be accepting at the instant the service spawns a new worker
  on a switch; a single connection-refused used to `log.Fatal` the worker,
  leaving the transport with no frame producer (frozen/black while input still
  worked via the agent/secure-bridge pipe). Fixed: `ipc.DialRetry` (worker
  retries ~300 ms up to 15 s); the transport distributes frames only from the
  CURRENT worker (single-producer guard, safe overlap); and the service spawns
  the new worker first and defers killing the old (`prevWorker`) to the next
  loop, so the old keeps producing until the new attaches.
- **LD-9 — On session change, swap the capture/input SOURCE behind the live
  connection; never restart hosting. Exactly ONE owner holds capture+input at a
  time.** The transport picks its frame source and input target live: the SYSTEM
  helper's secure-desktop bridge while a secure/elevated desktop is up (UAC /
  lock / another user's login — only SYSTEM can inject there), otherwise the
  per-session worker. Sources never interleave on one decoder (a keyframe is
  forced on every source switch), and input is routed to that single owner, so
  two workers can never contend (the login-screen input-break cause).
- **LD-13 — Cross-platform uses a common OS-agnostic wire format + per-OS
  implementations + a translation layer; the Windows-to-Windows path is
  platform-guarded and must never be altered by cross-platform changes.** The
  wire is OS-neutral (mouse normalized 0..1, keys = USB-HID usages, JSON for
  clip/ft/cmd). Each OS keeps its own native impl (Windows: `ClipboardWriter` /
  `ClipAgentBridge` / Go clipboard+input; macOS: `ClipboardMonitor.swift` using
  NSPasteboard.changeCount + CoreGraphics/Cocoa). Format differences that cross a
  boundary are translated (text LF↔CRLF, image PNG, file COPY-effect) — never by
  forcing one OS's format on the other. Every macOS/cross-platform branch sits
  behind a `TargetPlatform.macOS` guard (`NativeClipboardMonitor.supported`) or a
  source-OS check, so a Windows↔Windows session takes the exact same code path it
  did before. Do NOT modify a shared Win↔Win function (input_windows.go,
  command_windows.go, clipimg_windows.go, ClipboardWriter, the secure-desktop
  bridge) in a way that changes its Win↔Win behavior.

- **LD-14 — On macOS, the daemon must FOLLOW the active console session on user
  switch and re-point capture/input to the on-console session — viewer always
  matches the host's physical screen.** macOS fast-user-switch keeps EVERY user's
  session alive, so multiple per-session capture workers (the `Aqua`+`LoginWindow`
  LaunchAgent) run at once, each capturing ITS OWN session's framebuffer. Without
  an on-console check the transport streams whichever worker attached last — often
  the backgrounded previous user — so viewer and host diverge (D-4). Every worker
  MUST gate on `CGSessionCopyCurrentDictionary()` → `kCGSessionOnConsoleKey`: block
  until on-console before attaching, and stop/exit the moment it leaves the console
  (launchd KeepAlive respawns it to wait again). Exactly ONE on-console producer.
  This is the macOS analogue of the Windows `WTSQueryUserToken` spawn-into-active-
  session rule (LD-7) — never regress it into "last worker wins".
- **LD-15 — File-transfer resources (SCTP send buffer / handles / channels) must
  drain/release immediately after each transfer completes or fails — never
  accumulate. Both directions share the single `file` channel per peer, so a leak
  in one blocks the other.** The sender must pace against the REAL buffered amount
  for its actual send direction (host: max across viewers; viewer: the host peer),
  drain to a small high-water (512 KB, well under the ~16 MB SCTP cap), and on a
  stall ABORT the transfer — NEVER force-send into a full buffer (that saturates
  the shared channel and wedges both directions until reconnect, the "fails at
  file 5" bug). Go receiver releases every `*os.File` on end/cancel/teardown.
- **LD-21 — Transport↔worker IPC writes go through ONE writer goroutine draining
  three priority lanes (hi > bulk > droppable); no producer ever holds a lock
  across a blocking socket write. File transfer is backpressured end-to-end so a
  file larger than the pipe streams steadily and can never deadlock the lane;
  input/capture/clipboard stay live throughout.** `ipc.Conn` supersedes the r69
  write-mutex: `WriteMessage` = hi (input/control/acks/clip-control/chat/keyframe/
  video-info), `WriteBulk` = bounded reliable (file + clipboard-file BYTES →
  backpressure to the sender), `WriteDroppable` = video (drop-oldest, keyframe
  recovers). One goroutine writes, so frames never interleave (keeps LD-19
  integrity); hi always beats bulk, so a large transfer can't head-of-line-block
  input. The r69 bug was: a producer blocked in `WriteMessage` while holding the
  mutex → input starved and the bidirectional pipe deadlocked on a >~16 MB file.
  pion runs a per-channel read goroutine (network/peer.go), so blocking the file
  channel on `WriteBulk` backpressure never blocks the control (input) channel.
  Do NOT put bulk file/clipboard bytes on the hi lane or reintroduce a write
  mutex held across the socket write.
- **LD-22 — In TransportMode the CONSENT gate lives in the Go transport, not the
  Flutter app.** The SYSTEM-service transport (session 0) owns hosting and used to
  auto-accept every viewer (`onConnect`→`CreateAgentOffer`, LD-7); the Flutter
  app's in-app consent dialog is on the SUPPRESSED `startHosting` path, so it can
  never fire there. Consent now: the app mirrors the "Ask before allowing
  connections" toggle to `%ProgramData%\NeevRemote\consent.txt` (`consent_flag.dart`
  shim, Windows only); the transport reads it per connect (`consentRequired`), and
  when on, asks the per-session worker (`KindConsentRequest`) to show a modal
  Accept/Deny (`consent_windows.go` `MessageBoxW` on the interactive desktop),
  waits ≤30 s for `KindConsentReply`, and only offers on Accept. Deny / 30 s
  timeout / NO worker attached (lock screen / unattended) → refuse (no offer) —
  the literal meaning of "ask before allowing". The two consent IPC kinds are the
  first request/response pair over the worker IPC. Flutter-hosted (non-Transport)
  boxes still use the in-app dialog (LD-17). macOS daemon consent is a later port
  (`consent_other.go` returns true; the flag is Windows-only).
- **LD-16 — Every incoming file transfer gets a UNIQUE destination — never a
  shared/reused path or handle — and "Sent" status is only shown after the host
  confirms the file was fully and uniquely saved.** The receiver reserves a
  unique path the moment the `offer` arrives (Dart: `FileStore.reserveUnique`
  atomically `create(exclusive:true)` a placeholder before the next offer is
  handled; Go: `os.Create(uniquePath)` synchronously at offer on the single
  reader goroutine), keyed off the transfer — so rapid back-to-back sends can
  never resolve to the same name and clobber. The sender marks a transfer
  `done` ONLY on an explicit receiver→sender `{k:'ft',t:'saved',id,path}` ack;
  until then it is `sent` ("Delivered — confirming…"), never a false success.
  Both the Dart receiver and the Go worker send the ack. Do NOT reintroduce a
  save that picks its destination at `end` time via check-then-write (the TOCTOU
  that let 4 same-named files overwrite one slot and all report "Sent").
- **LD-17 — The "Ask before allowing connections" setting is authoritative and
  read LIVE by `startHosting()`; it is NOT clamped by unattended access.** An
  unattended/fixed password governs REACHABILITY; this toggle governs PROMPTING —
  the two are independent. `startHosting` reads `askOnConnect` from prefs at the
  moment hosting starts (not via a widget build), and the UI keeps it updated for
  mid-session toggles. Verify via `app.log`: `promptOnConnect` must match the
  actual toggle state, never be permanently `false`. Do NOT reintroduce the
  `&& !unattendedEnabled` clamp (it made the toggle inert on every always-on host
  that had an unattended password). Consent-on-by-default (`askOnConnect` default
  `true`) stands; silent unattended access requires explicitly turning it OFF.
- **LD-18 — File-transfer confirmation/acknowledgment is tracked per-transfer by
  unique ID — never a single shared slot/callback/completer. Every transfer in a
  batch must be able to confirm (or fail) independently.** Incoming state is
  `_incoming[id]`; the reserved destination is `inc.reserved` (one Future per id);
  the `{t:'saved',id}` / `{t:'failed',id}` acks and the sender's ack timers
  (`_ackTimers[id]`) are all keyed by id. A send never spins "confirming…"
  forever: it settles on `saved` (done), `failed` (error), or a per-id timeout
  ("Delivered (unconfirmed)"). Do NOT reintroduce any single "current transfer"
  reference — it strands transfer 2..N when transfer 1 holds the slot.
- **LD-19 — File transfer (esp. large files) runs OFF the input-injection/capture
  execution path and can never block or freeze remote control; all writes to a
  shared IPC connection are SERIALIZED; interrupted transfers are explicitly
  reported and their partial files deleted — never silently truncated.** The
  transport↔worker `net.Conn` is written by many goroutines (input, file chunks,
  keyframe reqs; video frames, clipboard, chat, export); a framed message is two
  writes (header+payload), so concurrent writers interleave and corrupt the
  stream → the reader errors/blocks → input + capture wedge forever (a 71 MB file
  racing with live input reproduced it). Fix: `ipc.Conn` wraps the conn with a
  write mutex (`NewConn`; all writes go through `conn.WriteMessage`); reads stay
  lock-free (one reader per direction). The worker also hands `KindFileData` to a
  DEDICATED drain goroutine (buffered chan), so disk writes never delay
  `KindInput`. `filerecv` tracks announced-size vs written and, on `end`, acks
  `saved` only if complete — else deletes the partial and sends `{t:'failed'}`;
  `closeAll`/create-error/write-error do the same. Do NOT write to the worker
  conn with the bare `ipc.WriteMessage(conn,…)` package func — always the
  `*ipc.Conn` method, or the interleave bug returns.
- **LD-20 — Worker IPC serializes only WRITES (stream integrity, LD-19) and
  per-lane ordering; clipboard-file and file-transfer run on INDEPENDENT
  non-blocking lanes, and any whole-file stream runs on its own goroutine — so no
  operation can stall another.** `KindFileData` multiplexes file transfers
  ({k:ft}) AND clipboard-file ops ({k:clipf*}); the worker reader routes them to
  two separate drain goroutines (`fileCh`/`clipCh`) by a cheap kind peek
  (`isFileTransferMsg`). `serveBytes` (clipboard SOURCE serving a whole file) and
  the `finishFile` clipagent write run on their OWN goroutines, so a large paste
  or a slow helper never blocks the next pull or a file-transfer ack. Concurrent
  serves are safe (per-message `ipc.Conn` write mutex + token/index/seq demux on
  the viewer). Input/capture stay on the reader goroutine, off both lanes (LD-19).
  Do NOT re-merge the two lanes or call a whole-file stream inline on a drain
  goroutine — that reintroduces the r69 head-of-line block (clipboard paste never
  completing + file acks stuck "unconfirmed").

---

## Working Features (confirmed)

- Normal remote control host↔viewer (~20 fps), Windows/macOS/Linux.
- Clicks/drags correct (no click-becomes-drag; no dead clicks; no stuck-Alt →
  double-click opens files, not Properties).
- Discovery shows real machine names (LAN UDP + relay-assisted).
- File **copy** no longer becomes **move** (Preferred DropEffect = Copy).
- Clipboard text/image sync; clipboard sync on/off toggle.
- Multi-file selection and queued transfer, both directions (r65): pick many
  files at once (export and import), sent sequentially through the fixed file
  channel with per-file progress; one failure is isolated and the queue continues.
- Viewer captures TRACKPAD two-finger scroll (`PointerPanZoom`) in addition to the
  mouse wheel (`PointerScroll`), forwarded through the existing scroll pipeline to
  the existing host injection (r58; mouse-wheel win-win/mac-win already confirmed).
- SYSTEM helper: secure-desktop capture + send (helper log verified 2026-07-08).
- TransportMode capture shows the FULL host screen on scaled displays — DPI-aware
  worker (`setProcessDpiAware`, r30). **Hardware-confirmed 2026-07-13** (user: "screen
  layout fixed"; also text copy/paste, UAC dialog, switch-user all working on r30).
- Viewer shows the FULL host screen by default (r20 behavior: `objectFit: Contain`
  off the host's actual frame size — correct across resolutions + Windows DPI),
  with an optional Fit/Fill toggle in the session toolbar (Fill = `Cover`/crop).
  Restored 2026-07-13 (`r28-viewfix`) after the `r27-view` hand-rolled-geometry
  regression; do NOT reintroduce manual video sizing.

## Working Features — per platform pair (status)

Legend: ✅ hardware-confirmed · 🟡 built + local-build-validated, awaiting hardware
test · ❌ known gap.

HARDWARE-TESTED 2026-07-15 (user's "Neev remote Test 1.xlsx", r53 Mac + Jul-14 Win):
- **Win → Win** ✅ ALL pass (control, clipboard text/image/file, transfer, lock,
  UAC/secure-desktop). EXCEPT Ctrl+Alt+Del ❌ — pre-existing: the Go TransportMode
  worker consumes `sas` but never executes it (command_windows.go); NOT a
  regression. Optional fix: SYSTEM SendSAS in the Go transport.
- **Mac → Win** ✅ ALL 11 pass: control, multi-line text (LF→CRLF ✅), image both
  ways repeatable (B1/A2 ✅), clipboard files, lock action, file transfer all
  types, **click-after-user-switch WORKS** (A3 was never broken for Mac→Win),
  keyboard capture. (MW-7 "Win+L" is a menu item = lock command; no physical Win+L
  on a Mac — not a bug.)
- **Win → Mac** mostly ✅ (control, clipboard text/image/file repeatable — wedge
  gone; file COPY stays in source = B2 ✅; same-user unlock recovery = r49 ✅).
  Fixed in **r55**: scroll (InputInjector `.pixel`→`.line`), import (activate host
  before openFile picker). WM-6 privacy button was just a STALE Windows viewer
  (Jul-14, predates the remoteHostOs=='macos' gate) — fixed by publishing Win r55.
  Switch-to-DIFFERENT-user still freezes (needs daemon+TCC — expected).
- **Mac → Mac** 🟡: not yet tested by user.

r55 published BOTH macOS + Windows to the portal (first Windows publish since
Jul-14 — the viewer-side r53 fixes only reach the user once Windows is updated).
All macOS work is platform-guarded (LD-13); Win→Win byte-for-byte unchanged +
hardware-confirmed intact.

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
- **KP-2 — Full disconnect on user switch. 07-08 fix FAILED IN FIELD 2026-07-09
  (build 11:37 IST); real root cause found, FIX v2 IMPLEMENTED 2026-07-09
  (pending hardware verify).** The 07-08 viewer auto-reconnect never got the
  chance to run: when the service kills the host on a session change, the
  host's relay websocket drops and the relay sends a synthetic `bye` to the
  viewer (`server/signaling/hub.go` `disconnect()`); the viewer treated ANY
  `bye` as a deliberate session end → `disconnectViewer()` with
  `autoReconnect=false` → reconnect disarmed + grace timer cancelled →
  permanent disconnect. Exactly the reported symptom: switch-user password
  page visible in viewer (secure-desktop path works), host app closes on
  login, viewer stays open but never reconnects. Second latent hole: while the
  new host is still re-registering (password entry + profile load can take
  30-60 s), the relay answers a re-dial with `error` "agent disconnected" /
  "agent not found or offline", which set status failed WITHOUT rescheduling.
  Fix v2: (a) `remote_service.dart` `bye` handler — while autoReconnect is
  armed and reason ≠ `peer_left`, treat bye as connection lost and keep
  re-dialing; (b) relay `error` replies reschedule the retry, EXCEPT
  password / too-many-attempts which now hard-stop autoReconnect (so retries
  can't trip the relay's 5-strike lockout); (c) `hub.go` — synthetic bye now
  carries reason `peer_left` (client sent an explicit bye, e.g. host rejected
  consent) vs `peer_dropped` (socket died, e.g. host killed on switch). The
  Dart fix also works against the OLD deployed relay (bare bye + armed →
  reconnect; rejections arrive before arming, so they still end cleanly);
  relay redeploy only needed for the reason tags.
  **STILL FAILED IN FIELD 2026-07-09 (build ~12:39 IST).** User's helper logs
  (`helper - Host.log` / `helper -viewer.log`) show native side perfect
  (secure-desktop capture, session poll `1->2` relaunches host in the new
  session, new host attaches to the local pipe). BUT those are HELPER logs —
  they contain ZERO Flutter/WebRTC/relay events, so they can't show whether the
  viewer reconnect ran. Blocking discovery: **the Dart app has NO file logging
  (only `debugPrint`, dropped in release)** → the whole transport layer is
  invisible in the field. Two structural facts that matter for the fix:
  (1) the service launches the host with a **duplicated SYSTEM token**
  (`neev_helper.cpp` `LaunchProcessInSession`, ~L318) — the host runs as
  LOCAL SYSTEM, so its SharedPreferences live in the SYSTEM profile, SEPARATE
  from the user-profile settings the person configures in the visible window;
  (2) the service-host runs **headless/hidden** (`flutter_window.cpp` ~L73).
  So if the SYSTEM/headless host has `promptOnConnect=true` (SYSTEM-profile
  never had unattended enabled), a reconnecting viewer triggers an INVISIBLE
  consent dialog nobody can accept → hangs. That's the leading hypothesis but
  UNCONFIRMED. ACTION 2026-07-09: shipped a **diagnostic build**
  (`lib/core/diag_log.dart` → `C:\ProgramData\NeevRemote\app.log`, build stamp
  `2026-07-09-diag1`) instrumenting host register, incoming-connect +
  promptOnConnect, viewer connect/bye/error/peer-state, and reconnect
  scheduling. Next: user retests switch-user, sends `app.log` from BOTH
  machines → pinpoint exact failure, then fix precisely (do NOT ship more blind
  behavior changes).
- **KP-3 — Clipboard files on-paste (delayed render) is v1, unverified on
  hardware.** Compiles; paste correctness / large / multi-file / timeout need
  real-Windows testing before it becomes a Working Feature.

---

## Change Log

- **2026-07-23 — Clipboard/chat/file regressions + card rebuild (r75).**
  • **IPC writeLoop deadlock (root cause of clipboard host→viewer + chat replies +
    file 'saved' acks all breaking together, "1st file ok, 2nd unconfirmed, 3rd
    stuck"):** `agent/ipc/ipc.go` writeLoop returned on ANY socket write error
    WITHOUT closing `done`, so every later WriteMessage/WriteBulk buffered then
    blocked FOREVER (and never returned an error, so the worker's transport-gone
    respawn never fired). FIX: on write error close `done` via closeOnce → producers
    get ErrConnClosed, the reader sees the dead conn, session reconnects instead of
    silently wedging.
  • **Consent thread-desktop leak (viewer→host clipboard while consent ON):**
    `consent_windows.go` did LockOSThread + bindInputDesktop + UnlockOSThread
    without restoring/closing the desktop → returned a desktop-polluted thread to
    the Go pool; a later clipboard call landing on it ran under the wrong desktop.
    FIX: new `bindInputDesktopSaved()` (deskbind_windows.go) saves the prior thread
    desktop + closes the opened HDESK on return; consent uses it.
  • **Chat window not displaying (host got the message, no popup):** the boot-time
    chat window could be behind/minimised. `chatEnsureShown` now SW_RESTORE +
    BringWindowToTop + SetForegroundWindow. (If it persists it's a wrong-desktop
    creation at boot — check worker.log "chat window created/create failed".)
  • **Consent prompt wording** cleaned (strip internal "ctrl-" prefix, group the id
    as XXX XXX XXX). Full AnyDesk-style custom window is a later follow-up.
  • **Device cards rebuilt** (Command Center): smaller (~108px thumbnail, ~240px,
    2–6 cols), premium — online dot + name + favorite in the body, OS·ID on one
    mono line, compact Connect; LIGHT tinted placeholder + small tilting glyph
    instead of the heavy dark ground so real screenshots stand out. Single device
    profile (removed nav-rail + top-bar "This PC" chips; kept the activity-panel
    "This device" card) — r74b.
- **2026-07-21 — Discovery flicker/slow-refresh fix + consent gate in
  TransportMode (r73). Implements LD-22.**
  • **Discovery (Dart, viewer-side):** flaky "shows then vanishes" + "refresh
    takes long to rediscover". ROOT CAUSE: the refresh button hard-cleared BOTH
    sources (`_devices.clear()` UDP + `_serverPeers.clear()` relay) → list blinked
    empty → spinner → slow repopulate at 3 s/5 s cadence; plus a tight 12 s UDP
    stale window flickered devices under lossy broadcast; plus the relay list
    full-replaced on any transient/empty poll. FIX: refresh no longer clears
    (re-announce/re-poll, let stale-prune remove gone devices); UDP announce 3→2 s,
    stale 12→20 s (tolerates ~9 lost packets); relay eviction now needs 2
    consecutive misses (`_serverPeerMiss`) so one empty reply can't wipe the list.
  • **Consent in TransportMode:** the "Ask before allowing connections" Accept/Deny
    dialog never popped on SYSTEM-service hosts. ROOT CAUSE: the Go transport
    auto-accepts (`onConnect`→`CreateAgentOffer`, no gate) and the Flutter consent
    dialog is on the suppressed startHosting path; the transport had no knowledge
    of the toggle. FIX (LD-22): app writes `%ProgramData%\NeevRemote\consent.txt`
    (`consent_flag*.dart`); transport `consentRequired()` reads it and, when on,
    `askConsent()` sends `KindConsentRequest` to the worker, which shows a
    `MessageBoxW` Accept/Deny on the interactive desktop (`consent_windows.go`) and
    replies `KindConsentReply`; the offer is deferred until Accept — Deny/30 s
    timeout/no-session → refuse. New IPC kinds 0x0A/0x0B (first request/response
    pair). Windows-first (macOS stub). Pending hardware validation of the modal +
    deny path. Go builds (darwin + windows cross-compile) + Dart analyzes clean.
- **2026-07-21 — Large file aborted mid-send (false stall); progress-based drain
  timeout + cancel-on-abort (r72).** After r71 killed the file-lane DEADLOCK
  (confirmed: a stalled large file no longer wedges the lane — every file after it
  finishes), one gap remained: an individual large file (~>12 MB) intermittently
  aborted mid-transfer (host wrote ~8–16 MB, never finished), and it worked again
  only after a reconnect. ROOT CAUSE (viewer-side, file_transfer_service.dart):
  `sendFile`'s drain wait used a FIXED 30 s timeout — `while buffered()>highWater`
  for 30 s → abort. r71's bulk-lane backpressure legitimately PAUSES the sender
  when the import competes with the live video stream for bandwidth; once the SCTP
  buffers fill (after several transfers) that pause exceeds 30 s and the fixed
  timeout misread "receiving slowly" as "peer dead" → false abort. Fresh SCTP
  buffers (after reconnect) are empty, so the first big file slips through — hence
  "works after reconnect". FIX: the stall timer now RESETS whenever the buffer
  actually drains; it only fires after the window with ZERO drain progress (peer
  truly stopped). A large file over a slow/contended link now completes (drains
  steadily, just slowly). Also: on a real abort the viewer sends `{t:'cancel',id}`
  so the host deletes the partial immediately (was leaked until worker teardown).
  Refines LD-15 (drain pacing is progress-based, not fixed-duration). Viewer-only
  Dart change; no Go/wire change. Analyzes clean.
- **2026-07-21 — Large file (>~16 MB) deadlocked the whole file lane; writer-
  goroutine IPC redesign (r71). Implements LD-21.** Logs (r70): viewer sent a
  23.8 MB import fully (`sent end`) → host logged only the offer, never finished;
  then EVERY later file (63 KB, 194 KB) + all export requests `ack TIMEOUT`, until
  a manual reconnect. Clipboard lane stayed alive (r70 split holds) — only the
  FILE lane wedged. ROOT CAUSE: the r69 per-conn write MUTEX was held across a
  blocking socket write — on a file bigger than the pipe (fileCh 256 + socket
  buffers) the transport's file-forward `WriteMessage` blocked holding the mutex,
  starving input and deadlocking the single bidirectional transport↔worker pipe;
  it never self-cleared. Export uses the same lane → same wedge → no picker. FIX:
  replaced the mutex with a single WRITER GOROUTINE per `ipc.Conn` draining three
  priority lanes — `WriteMessage` (hi: input/control/acks/chat/keyframe/video-info),
  `WriteBulk` (bounded reliable: file + clipboard-file bytes = real backpressure),
  `WriteDroppable` (video: drop-oldest). No producer holds a lock across a socket
  write; hi always beats bulk (input never behind file data); one writer keeps
  frame integrity (LD-19). Bulk backpressure paces the sender via pion's
  per-channel read goroutine, which (confirmed in network/peer.go) means blocking
  the file channel never blocks the input channel. Call sites: transport
  file-forward → WriteBulk; worker video → WriteDroppable; clipboard image +
  clipfdat bytes + export data chunks → WriteBulk; everything else → hi. Also:
  host receive-progress log every 8 MB + export request/picker logs (a stall is
  no longer invisible); `fileCh` 256→512. Pure Go, no wire change; Win↔Win
  capture/input/secure-desktop untouched. Builds + vets clean.
- **2026-07-21 — r69 side effect: clipboard-file + file-transfer shared one lane
  and blocked each other (r70). Implements LD-20.** After r69 (`r69-ipc-serialize`)
  a test showed clipboard Ctrl+C/Ctrl+V never completing (worker.log: 3 `announcing
  host clipboard files` h1/h2/h3, no pull) and one export file stuck "Delivered
  (unconfirmed)". ROOT CAUSE: r69 correctly moved `KindFileData` off the input
  goroutine, but funnelled BOTH file transfers ({k:ft}) and clipboard-file ops
  ({k:clipf*}) onto ONE `fileCh` drain goroutine — and `serveBytes` (viewer pasting
  a host file) streamed the WHOLE file SYNCHRONOUSLY on it. So a clipboard serve
  blocked file-transfer acks, and a file transfer blocked clipboard pulls — one
  shared serial lane, both symptoms. FIX (keeps r68 anti-freeze + r69 write mutex):
  (1) reader routes KindFileData to two independent lanes `fileCh`/`clipCh` by a
  cheap kind peek (`isFileTransferMsg`); (2) `serveBytes` now runs on its own
  goroutine (`go cf.serveBytes`), like serveExport, so a big paste never blocks its
  lane; (3) the `finishFile` clipagent write (host-destination staging, ~2s helper
  round-trip) runs async too; (4) added pull/serve logs (`viewer pulling host
  clipboard file` / `served host clipboard file`) — the missing completion
  instrumentation. Three independent lanes now: capture/input (reader), file
  transfer, clipboard — no shared serial choke. Pure Go, no wire change; Win↔Win
  capture/input/secure-desktop untouched. Builds + vets clean.
- **2026-07-21 — Large file froze remote control: IPC write race in the Go
  transport/worker (r69). Implements LD-19.** Log evidence: host `worker.log`
  logged `receiving file …SADP__EN.zip size=71581150` then NOTHING ever again;
  viewer logged 3 files `ack TIMEOUT` + 2 min of `input mv dropped — no live
  viewer peer` starting the instant the big file began; the file landed truncated
  at 756 KB. ROOT CAUSE (not "blocking write starves input" — sharper): the single
  transport↔worker `net.Conn` is written by many goroutines, and `ipc.WriteMessage`
  emits header+payload as two unsynchronized `Write`s. A 71 MB transfer = ~2000
  chunk writes racing with live input + keyframe reqs → interleaved partial
  messages corrupt the frame stream → the worker's `ReadMessage` reads a bogus
  length → reader loop errors/blocks → input AND file processing wedge forever.
  Small files (few chunks) rarely collided, so 1–3 worked. FIX (4 parts): (1)
  `ipc.Conn` wrapper serializes all writes with a mutex (`agent/ipc/ipc.go`);
  every worker-conn write on both sides now goes through `conn.WriteMessage`
  (transport.go, worker.go, clipboard.go, clipfiles_windows.go, filerecv.go).
  (2) worker hands `KindFileData` to a dedicated drain goroutine (buffered chan)
  so disk I/O never delays `KindInput` injection (worker.go). (3) `filerecv`
  tracks size-vs-written and reports `{t:'failed'}` + deletes the partial on
  truncation / create-error / write-error / session-teardown (`closeAll`) —
  never a silent 756 KB truncation. (4) r68's client 30 s ack-timeout stays as
  the backstop. Platform-guarded: pure Go serialization, no wire-format/logic
  change; Win↔Win capture/input/secure-desktop paths unchanged (it FIXES a
  Win↔Win freeze); macOS daemon shares the IPC and benefits too. Builds + vets
  clean locally.
- **2026-07-21 — File transfer r67 follow-up: no-hang confirmation + per-id
  diagnostics (r68). Implements LD-18.** Reported: viewer sends 5 files, host
  saves file 1, files 2–5 stuck at "Delivered — confirming…" forever, never on
  disk; `worker.log` silent (so the receive path is the Dart `FileTransferManager`,
  NOT the Go worker — confirmed). INVESTIGATION: read every layer (sender queue,
  receiver, `_finishIncoming`, ack handler, file-channel `onMessage` wiring on
  both offerer+answerer) and REPRODUCED concurrent `reserveUnique` (5 same-name
  after 7 pre-existing → 5 distinct paths, no hang). The ack path is ALREADY
  per-id — the "single shared slot" theory does not match the source. The true
  drop (offers 2–5 leaving no placeholder ⇒ not reaching/completing on the host)
  is only pinnable from a real run, so per the KP-2 "no blind behavior changes"
  rule we did NOT invent a fix. Shipped: (A) per-id diag logs on both ends
  (`ft` tag): recv offer / reserved / recv end / wrote / ack saved / recv
  saved|failed, sender sent-end / ack timeout — the next 8-file run's app.log
  shows exactly where 2–5 die. (B) HARDENING so it can never hang silently:
  `_finishIncoming` now emits `{t:'failed',id,err}` on any exception; the sender
  arms a per-id 30 s ack timeout (`_ackTimers[id]`) that settles a stuck send as
  "Delivered (unconfirmed)" instead of an infinite spinner; new `failed` handler
  + `FileTransfer.unconfirmed`. Everything keyed by transfer id (LD-18). Part C
  (precise fix at the real drop point) follows once the instrumented log is in.
- **2026-07-21 — File transfer: fixed silent overwrite (only last file survived)
  + false "Sent"; consent toggle actually wired (r67). Implements LD-16 + LD-17.**
  • **Overwrite (data loss):** the Flutter-host receive path allocated the
    destination at `end` time via a check-then-write dedup loop in
    `saveToDownloads`, and `_finishIncoming` was fired un-awaited — so N same-named
    transfers finishing close together all evaluated "does `foo.png` exist?" before
    any had written, all picked the identical path, and the last write won (4
    silent losses looked like 5 "Sent"). FIX: `FileStore.reserveUnique` atomically
    `create(exclusive:true)` a unique placeholder the MOMENT the `offer` arrives
    (`file_store_io.dart`); `_Incoming` carries that reserved path; `_finishIncoming`
    writes to it via `writeReserved`. Race-free regardless of same-name. The Go
    worker path (`filerecv.go`) already created the file synchronously at offer on
    one goroutine — safe, unchanged behavior.
  • **False "Sent":** the sender set `done` purely when its SCTP buffer drained —
    no host confirmation existed on the wire. FIX: new `{k:'ft',t:'saved',id,path}`
    ack sent by BOTH receivers (Dart `_finishIncoming`, Go `filerecv.go` on `end`).
    New `FileStatus.sent` = "Delivered — confirming…"; a send flips to `done`
    ("Saved on host") ONLY on the ack. If a host never acks it stays "Delivered"
    (honest), never a false success. `clearFinished`/`anyDone` keep unconfirmed
    rows. Cancel deletes the reserved placeholder.
  • **Consent toggle inert (LD-17):** `promptOnConnect` was `askOnConnect &&
    !unattendedEnabled` in `ConnectPage.build()` — so any always-on host with an
    unattended password forced the prompt OFF regardless of the toggle (the
    `promptOnConnect=false` every log showed). Also `startHosting` only LOGGED the
    field, never read the setting. FIX: dropped the `&& !unattendedEnabled` clamp
    (`connect_page.dart`); `startHosting` now reads `askOnConnect` from prefs LIVE
    at start (`remote_service.dart`). Consent UI already existed + fully wired
    (`_showConsentDialog`), so it now fires. Win↔Win video/input/secure-desktop
    untouched; the only Go change is the additive `saved` ack.
- **2026-07-15 — File transfer: fixed the "stops after 4 files" leak + multi-file
  select (r65).** ROOT CAUSE (not a literal pool of 4): the single bidirectional
  `file` SCTP data channel's send buffer (~16 MB libwebrtc default) saturated
  because backpressure was broken — `_fileBuffered()` read only `_viewerPeer`, so
  when HOSTING it returned 0 and Host→Viewer had zero backpressure; and the send
  loop force-sent into a full buffer after a 32 s give-up. ~4 medium files ×
  ~4 MB ≈ 16 MB → "file 5 fails", and since it's ONE channel per peer, a full
  buffer stalls BOTH directions until reconnect. FIX: `_fileBuffered()` now
  reports the max buffered across whichever peers we send to (host or viewer);
  send loop drains to a 512 KB high-water and, if it can't drain in 30 s, ABORTS
  that transfer (never force-floods) so the channel stays healthy for the next
  file and the other direction; `bufferedAmountLowThreshold` armed so native emits
  drain events. Go receive side (`filerecv.go`) already released handles — clean.
  MULTI-FILE: `openFiles()` on both export and import; `sendFilesQueued()` sends
  each file sequentially through the fixed channel, fault-isolated (one failure
  logs and the queue continues); per-file progress via existing FileTransfer rows.


- **2026-07-15 — Mac→Mac: daemon follows console session (D-4) + file size cap
  (MM-2/3) (r59).**
  • **D-4 (viewer showed the PREVIOUS user after a switch):** ROOT CAUSE = macOS
    fast-user-switch keeps all sessions alive; each session's LaunchAgent worker
    captures its OWN framebuffer and attaches to the transport, which streamed the
    last-attached one (often the backgrounded old user). ZERO on-console detection
    existed in `agent/`. FIX (LD-14): new `console_darwin.go` (cgo
    `CGSessionCopyCurrentDictionary` + `kCGSessionOnConsoleKey`) + `console_other.go`
    stub (always true → Windows/Linux behavior byte-identical). Worker now
    `waitUntilOnConsole()` before dialing, and a 500ms watcher cancels `runCtx` the
    moment the session leaves the console, so it stops streaming and launchd
    respawns it to wait. Exactly one on-console producer.
  • **MM-2/3 (.dmg/.pkg/.exe fail both ways):** ROOT CAUSE = `maxFile` was a 200 MB
    in-memory cap — real installers exceed it and were silently rejected on send
    (`sendFile` returned null) and errored on receive. NOT a type/`public.file-url`
    bug: the native `ClipboardMonitor` read/writeObjects(NSURL) is type-agnostic.
    FIX = cap raised to 2 GB (matches the clipboard-file cap). Send already base64s
    per-slice, so only raw bytes sit in memory. Multi-GB streaming-to-disk deferred
    (shared web-safe path; not worth the Windows-regression risk now).
  • **.app bundles** still unsupported (directories are skipped by
    `_announceClipFiles`) — needs zip-on-send; separate additive piece.
  • **MM-1 privacy — FIXED in r60.** ROOT CAUSE (confirmed by user: daemon
    installed on both Macs, nothing blanked at all): with the daemon hosting, the
    viewer's `{k:cmd,c:privacy}` reached the Go worker, whose `handleCommand` was a
    no-op off Windows (`command_other.go`), so it was dropped — Flutter's working
    `PrivacyMode.swift` never runs because the app is no longer the host. Second
    wall: the daemon captures with **CGDisplayStream (full framebuffer)**, which
    IGNORES `sharingType=.none`, so a black overlay window would have blacked out
    the VIEWER too. FIX = blank via the display **transfer/gamma table**
    (`CGSetDisplayTransferByFormula` 0 on every active display): gamma is applied
    at SCANOUT, so the physical screen goes black while the FRAMEBUFFER — what
    CGDisplayStream captures — is untouched, so the viewer still sees the real
    desktop. No ScreenCaptureKit rewrite needed. Local input blocked by a
    `CGEventTap` on a dedicated pthread+CFRunLoop (the daemon has no GUI run loop);
    remote input passes because `input_darwin.go` now stamps every injected event
    with `kCGEventSourceUserData = 0x4E56494E4A` (same tag as the app's
    InputInjector). New `privacy_darwin.go` + `command_darwin.go`; `privacy_other.go`
    / `command_other.go` narrowed to `!windows && !darwin`, so Windows and Linux
    take byte-identical paths.
- **2026-07-15 — Viewer captures TRACKPAD two-finger scroll (r58).** A mouse WHEEL
  scrolled the host fine, but a trackpad two-finger scroll did nothing. Flutter
  delivers precision-trackpad scroll as PAN-ZOOM events
  (`PointerPanZoomUpdateEvent.panDelta`), NOT `PointerScrollEvent`, and the viewer's
  `Listener` only wired `onPointerSignal` → trackpad scroll was dropped before being
  sent. Fix (viewer-side only, `remote_view_widget.dart`): added
  `onPointerPanZoom{Start,Update,End}`; the update handler converts `panDelta` into
  the SAME `InputEvent.wheel` message the mouse wheel sends, through the existing
  pipeline → existing (UNCHANGED) host injection. Negated to match scrollDelta sign,
  scaled ×2 (`_kTrackpadScrollScale`, tunable). Purely additive — mouse-wheel path
  and host injection untouched; no platform branching. Temp `scroll` diag log
  confirms event type/direction on first HW test.
- **2026-07-15 — Windows-host scroll + Ctrl+Alt+Del (r57).** Scroll: the Go host's
  `whl` handler went through `sendMouseAbsolute` (OR-ed MOUSEEVENTF_ABSOLUTE + move
  to 0,0 onto the wheel → Windows dropped it); new `sendWheel()` sends a pure wheel
  event at the cursor (mouse + touchpad both scroll). Ctrl+Alt+Del: was a no-op in
  TransportMode (viewer's `sas` reached the user worker, which can't SAS); now the
  transport (SYSTEM, session 0) intercepts `{k:cmd,c:sas}` and calls SendSAS(FALSE)
  after setting SoftwareSASGeneration (`sas_windows.go`, mirrors the helper).
- **2026-07-15 — Cross-platform Mac↔Windows: clipboard/Lock/input/file-transfer
  fixes (r53), platform-guarded so Win↔Win is byte-for-byte unchanged.** Diagnosed
  each by cross-platform root cause (3 parallel code investigations) before coding.
  • **B1 (Win→Mac clipboard "works once then stops") + A2 (Mac→Win copy fails):**
    ROOT CAUSE = macOS change-detection was done by *reading + hashing content*
    every poll; after writing a received item the next read couldn't be told from a
    fresh user copy (cheap-hash collisions + write/read round-trips) → wedged. FIX =
    native `ClipboardMonitor.swift` using **NSPasteboard.changeCount** (records the
    count OUR writes cause → precise echo-suppression). Dart `clipboard_monitor.dart`
    + integration in remote_service (`_ensureClipboardSync` starts the native
    monitor on macOS instead of the Dart poll; receive-writes go through it).
  • **A2 text:** Mac emits LF, Windows apps want CRLF. FIX = convert LF→CRLF on the
    Mac VIEWER send side ONLY when `remoteHostOs=='windows'` (idempotent) — so the
    Go/Windows host receives CRLF exactly as from a Windows viewer; **zero Windows
    code touched**.
  • **B2 (Win→Mac file export = files vanish):** `Pasteboard.writeFiles` pasted as
    a MOVE. FIX = native NSPasteboard file-URL write (COPY semantics) in
    ClipboardMonitor; macOS receive-writes route to it.
  • **A1 (Mac→Win Lock):** the "Lock device" action already worked (sends
    `cmd/lock`); the failing path was the **Win+L shortcut** — Windows IGNORES an
    injected Win+L (protected hotkey), so it was a no-op from ANY viewer. FIX =
    `_Shortcut.command` routes Win+L through `sendHostCommand('lock')`. Improves
    Win→Win too (shortcut was already a no-op there — no working behavior changed).
  • **A3 (Mac→Win can't click after user-switch):** all 3 investigations found the
    input pipeline OS-agnostic (normalized 0..1 coords + HID; viewer routes to the
    LIVE peer after reconnect). Not pinned to Mac-specific code → added a throttled
    diagnostic (viewer logs "input dropped — no live peer" vs host's existing
    "SendInput inserted 0 events"). NEEDS the two-machine hardware test to localise.
  • **A4 (Mac↔Win file transfer):** path is cross-platform (`file_selector` +
    `FileStore` → ~/Downloads/NeevRemote, `file_selector_macos.framework` ships).
    De-blackholed the silent `catch(_)` in `_onFileRequest` + added transfer logs.
  ALL macOS clipboard/file code is behind `NativeClipboardMonitor.supported`
  (`TargetPlatform.macOS`); Windows/Linux take the identical branch as before.
  Builds + links locally (Xcode 26.6) + analyzes clean.
- **2026-07-15 — macOS switch-user/lock-screen daemon: FEASIBILITY PROVEN +
  full buildable scaffolding shipped (r49–r51).** The dev Mac now has the full
  toolchain (Xcode 26.6 + CocoaPods + Go 1.26.3 + brew ffmpeg/x264/libvpx), so
  macOS is now built + validated LOCALLY, not blind. Key proofs THIS session, all
  on real hardware: (1) the entire Go TransportMode agent COMPILES, LINKS, RUNS
  and REGISTERS with the relay on macOS (capture_darwin.go/input_darwin.go were
  already full impls, not stubs); (2) the transport+worker split + loopback IPC
  47930 works on macOS end-to-end (worker attaches to transport) — only blocker to
  live capture is Screen Recording + Accessibility TCC. Shipped:
  • **r49 Stage 1** — same-user lock/unlock + fast-user-switch video RECOVERY:
    native `SessionWatcher.swift` (screenIsLocked/Unlocked + NSWorkspace session/
    wake) → Dart `session_watcher.dart` → RemoteService re-acquires capture and
    hot-swaps the track on every viewer (fixes the "same user, video frozen after
    unlock" symptom). No elevated perms. Does NOT capture the login window itself.
  • **r50 Stage 2 scaffold** — `session/datadir.go` (cross-platform machine-wide
    dir: ProgramData / **/Library/Application Support/NeevRemote** / /var/lib so
    root transport + per-session workers share one machine.dat); launchd plists
    `packaging/mac/com.neev.transport.plist` (root LaunchDaemon --transport) +
    `com.neev.worker.plist` (LaunchAgent --capture-worker, **LimitLoadToSessionType
    [Aqua, LoginWindow]** — the LoginWindow instance is what captures the login/
    lock screen; a plain daemon canNOT — empty frames); `install-daemon.sh`; CI
    builds neev-agent (darwin/arm64) + build_macos.sh bundles it into
    Contents/Resources/daemon. macOS CI job GREEN.
  • **r51 handoff + install UI** — `HostMode` defers hosting to the daemon on
    macOS when its plist is installed (app stays viewer/control-only, matching
    Windows TransportMode); `mac_daemon.dart` installs/removes via osascript admin
    prompt; Settings → Security "Install lock-screen daemon" card.
  REMAINING (HARDWARE-ONLY, user must do): grant Screen Recording + Accessibility
  TCC to /Library/Application Support/NeevRemote/neev-agent (no prompt possible at
  login window), then validate login-window capture across an actual lock / user
  switch with a second device viewing. Distribution needs Developer-ID signing +
  notarization (CI is ad-hoc) and possibly the restricted persistent-content-
  capture entitlement for unattended TCC. See [[flutter-build-env]] for the local
  build/sign + Go-agent-on-Mac recipes.
- **2026-07-15 — macOS parity: native privacy (r46) + keyboard capture (r47) —
  pending Mac hardware validation.** Ported two Windows-only features to macOS:
  `PrivacyMode.swift` (black window on every screen, `sharingType=.none` so it's
  excluded from capture, + a CGEventTap blocking local input while letting
  remote-injected input through — injected events tagged via `eventSourceUserData`
  in `InputInjector.swift`); `KeyHook.swift` (session CGEventTap capturing all
  keys incl. reserved combos → HID usages → drained by Dart; keyCode→HID reverse
  map + flagsChanged modifier handling). Registered in `MainFlutterWindow.swift`;
  Dart `PrivacyMode.supported`/`KeyboardHook.supported` + the viewer Privacy
  button now include macOS. FILE CLIPBOARD (Ctrl+C/V files) on Mac needs NO new
  code — it already works via the cross-platform `pasteboard` package
  (`Pasteboard.files()`/`writeFiles()`); the SANDBOX was blocking it, so r45
  un-sandbox unblocks it. PARITY STATUS: input/screen/clipboard(text/image/file)/
  file-transfer/chat/privacy/keyboard-capture all now cross-platform (pending Mac
  test). ONLY remaining gap = **Mac switch-user/lock-screen capture** — the login
  window is a protected macOS context needing a privileged ROOT LaunchDaemon
  (the macOS equivalent of the Windows SYSTEM service/TransportMode); a dedicated
  project that CANNOT be done blind and needs Mac hardware per iteration. I can't
  run macOS here, so r45–r47 native features need the user's Mac to validate.
- **2026-07-15 — GOAL: full cross-platform parity (win↔win, win↔mac, mac↔win,
  mac↔mac) like AnyDesk. STEP 1: un-sandbox the macOS app (r45-mac-nosandbox).**
  Root cause of Mac host crashing / "cursor moves but can't click" / import-export
  broken = the macOS app was **App-Sandboxed** (`com.apple.security.app-sandbox`
  =true). A sandboxed app cannot inject CGEvents into other apps, cannot be
  granted Accessibility to control other apps, and can't access arbitrary files —
  fatal for a remote-desktop host. Fixed: both `Release.entitlements` +
  `DebugProfile.entitlements` → app-sandbox=false + hardened-runtime entitlements
  (allow-jit, disable-library-validation, apple-events, network). Bonus: Mac log
  now at real `~/.neev_remote/app.log` (was sandbox container). User must grant
  **Screen Recording + Accessibility** (TCC) after reinstall. STILL TODO for
  parity (all need Mac hardware to validate; I can't run macOS): Mac clipboard
  files, Mac switch-user/lock-screen (needs a privileged macOS helper like the
  Windows SYSTEM service — large), Mac stability confirm, privacy on Mac. Mac app
  is `com.neev.neevRemote`; input via `InputInjector.swift` (CGEvent, solid).
- **2026-07-14 — FIX: Mac "agent not found" — Mac registered a DASHED id
  (r44-idfix).** Relay logs (deploy-server-1) were definitive: Mac registered
  `id="532-034-441"` (host Admins-MacBook-Pro) while Windows registers plain
  `958897411`; the relay matches IDs EXACTLY, so a Windows peer typing `532034441`
  never found the Mac. Root cause: Flutter `_generateAgentId()` returned
  `%03d-%03d-%03d` (dashes baked in) and registered it. Fix (`remote_service.dart`,
  Flutter so it lands on Mac+Windows app): generate PLAIN 9 digits;
  `_persistentAgentId` normalizes any stored dashed id (strips + re-saves);
  `connectToHost` strips non-alnum from the target so typing dashes or plain both
  match. NOTE: also must publish the updated MAC installer to the portal (was
  stale from 07-09). Mac app.log lives at `~/.neev_remote/app.log`.
- **2026-07-14 — W2W CLIPBOARD/FILES largely WORKING; raise file-clip cap to 2GB
  + stream (r43-bigfiles).** User: Windows↔Windows "almost everything working" —
  clipboard file copy-paste works for pdf/text/exe/image; zip/dmg/mp4/mp3 failed.
  Root cause = the 64 MB size cap (NOT file type). Raised `_clipFileMaxBytes`
  (Dart) and `clipFileMaxBytes` (Go) to 2 GB; `serveBytes` now STREAMS the host
  file in 36 KB raw chunks (base64, mult-of-3 so concatenation stays valid) so
  large files don't load into memory. (Viewer→host still assembles in memory —
  large uploads may be heavy; follow-up.) OPEN cross-platform items (Mac):
  keyboard-capture + privacy are `supported => windows` (Windows-only by design,
  absent on Mac viewer); Mac↔Windows import/export + some clip file types + the
  ID "agent not found" need Mac-side logs/testing (dev Mac limits per notes).
- **2026-07-14 — File clipboard: Ctrl+C file → Ctrl+V on other machine
  (r42-fileclip) — pending hardware validation.** User wants Explorer-style file
  copy-paste (NOT the auto-listener idea, dropped). Implemented the HOST end in
  the worker (`clipfiles_windows.go`/`_other.go`), REUSING the existing clipf*
  protocol (`clipfann`/`clipfreq`/`clipfdat` on the file channel — the viewer
  already implements the other end incl. delayed-render) and the neev_helper
  `clipagent` (127.0.0.1:47922, `'R'`/`'W'` = CF_HDROP read/write). Host SOURCE:
  poll clipagent 'R' → clipfann → on clipfreq read bytes → clipfdat chunks
  (deliver-on-paste, reuses viewer delayed-render). Host DESTINATION: clipfann →
  eager clipfreq → assemble clipfdat → temp file → clipagent 'W' → host
  clipboard. Routed over the existing `ipc.KindFileData` path (worker↔transport↔
  viewer file channel); `fileReceiver.handle` tried first, else clipf*. No new
  clipboard system, no Flutter bridge. Manual file-transfer button untouched.
  Images: r41 BI_BITFIELDS fix still needs a user test (Ctrl+C image→Ctrl+V).
- **2026-07-14 — File import CONFIRMED working; image read fix (r41-imgfmt).**
  worker.log (host) proved file import works — `receiving file
  path=C:\Users\manickam.c\Downloads\… size=452` → `file transfer finished` (user
  thought "files not working" but the file landed in Downloads; may have meant
  export, fixed r40). Image showed ZERO worker activity → root cause found:
  `readClipboardImagePNG` only accepted `BI_RGB` and rejected `BI_BITFIELDS`, but
  most apps put 32bpp BI_BITFIELDS on the clipboard → host→viewer image silently
  bailed. Fix (`clipimg_windows.go`): accept BI_BITFIELDS (skip the 3 mask DWORDs,
  assume BGRA); poll (`clipboard.go`) no longer skips image when the clipboard
  sequence number is 0. Viewer→host still needs a retest (no `receiving clipboard
  image` seen — may be viewer-side Pasteboard).
- **2026-07-14 — Chat WORKS (r39 confirmed on hardware); r40 shrinks it + fixes
  the export picker's desktop.** r39 desktop-binding fix worked — bidirectional
  chat confirmed (host chat window shows viewer msgs + host replies reach viewer).
  r40: chat window shrunk 420×360 → 300×380 docked top-right (was covering the
  host work area); shared `bindInputDesktop()` (`deskbind_windows.go`/`_other.go`)
  now also applied to the file-EXPORT picker thread (`serveExport`) — the picker
  ran on an unbound goroutine so it likely failed the same way the chat window
  did. IMAGE + file-IMPORT still reported not working but NOT yet seen in a
  worker.log (need `receiving clipboard image` / `receiving file` lines to tell
  arrived-but-native-fails vs not-arriving).
- **2026-07-14 — FIX: worker GUI windows (chat/privacy) failed to create
  (r39-chatwin) — pending hardware validation.** r38 confirmed the routing fix
  (worker.log: `chat message received from viewer`), but `chat window create
  failed` — the service-spawned worker was denied window creation even though
  SendInput works (its thread lands on a non-interactive desktop for GUI). Fix:
  `OpenInputDesktop`+`SetThreadDesktop` bind the chat/privacy loop thread to the
  interactive input desktop before creating the window; also fixed a bad
  `hbrBackground` (was GetStockObject(NULL_BRUSH)+1 → now (HBRUSH)(COLOR_WINDOW+1))
  and added `GetLastError` logging on RegisterClass/CreateWindowEx so the exact
  failure is visible if it persists. IMAGE: not exercised in the r38 logs (only
  chat was) — needs a retest; the r38 routing fix should have unblocked it too.
- **2026-07-14 — FIX: clip/chat/file/cmd dropped to secure-bridge while
  elevated (r38-route-fix) — root cause of "chat + image not working."** Field
  logs (helper 6 / transport 2) showed the host constantly `foreground elevated
  -> YES` / `input desktop -> Winlogon`; transport routed ALL control-channel
  messages to the secure/elevated bridge in that state, but the bridge only
  injects mouse/keyboard — so chat/image/file/command messages were silently
  dropped whenever the host was elevated/secure (frequent on this machine; text
  clipboard "worked" only because it was tested while not elevated). Fix
  (`transport.go`): new `workerOnlyMessage` — only real input goes to the bridge;
  `{k:clip|chat|ft|cmd}` ALWAYS go to the worker (it handles them regardless of
  desktop). Added worker.log diagnostics: chat received/window-created, image
  send/receive, so the native paths are observable. LD-9/LD-3 note: bridge is
  input-only; worker owns clipboard/chat/files/commands even during secure/UAC.
- **2026-07-14 — TransportMode Phase B, batch 6: chat (r37-chat) — Phase B
  feature-complete, pending hardware validation.** The worker renders a native
  Win32 chat window on the host (`chatwin_windows.go`: log edit + input edit +
  Send button, custom wndproc for WM_COMMAND/WM_SIZE/WM_CLOSE, own OS thread +
  PeekMessage pump; `chatwin_other.go` stub). Viewer `{k:'chat'}` rides the
  control channel → worker `handleChat` (`chat.go`) → `chatShow`; host replies go
  worker→transport via new `ipc.KindChat` → `transport.go` relays to viewers on
  the control channel (SendControlText). RISK: native window + child controls +
  message routing untested-on-hardware. **Phase B parity now complete** (screen,
  input, lock/logoff/reboot, privacy button+execution, text+image clipboard, file
  import+export, chat). NOTE: r33–r37 native features are all built but NOT yet
  hardware-validated; portal 172.17.17.77 was DOWN at ship time so r34–r37 went
  to the GitHub ci-windows release only — push to portal when reachable.
- **2026-07-14 — TransportMode Phase B, batch 5: privacy-mode execution
  (r36-privacy) — pending hardware validation.** Ports privacy_mode.cpp to the
  worker: `{k:'cmd',c:'privacy',on:bool}` → `setPrivacy` (new
  `privacy_windows.go`, `privacy_other.go` stub). A full-virtual-screen black
  layered/click-through/no-activate window with
  `SetWindowDisplayAffinity(WDA_EXCLUDEFROMCAPTURE)` (viewer keeps seeing the
  real desktop, local user sees black) + `BlockInput` (blocks local physical
  input; remote SendInput still lands — the shipped Flutter behavior). Runs on
  its own OS thread with a PeekMessage pump; toggled via a channel. `command_
  windows.go` now parses `on` and routes privacy. RISK: Win32 window/message-loop
  + BlockInput-vs-SendInput are native/untested-here. STILL OPEN: **chat** — in
  the unattended seamless model the host has no operator UI, so bidirectional
  chat needs a native chat window in the worker OR routing to the host Flutter
  app (design choice pending); lowest value in this mode.
- **2026-07-14 — TransportMode Phase B, batch 4: file transfer EXPORT host→viewer
  (r35-fileexport) — pending hardware validation.** Completes file transfer both
  ways. On a viewer `{k:'ft',t:'request'}`, the worker pops a native Windows
  picker on the user's desktop (`filedlg_windows.go`, `GetOpenFileNameW` — it runs
  its own modal loop, called on a locked OS thread; `filedlg_other.go` stub) —
  the viewer, controlling that desktop, selects the file; the worker reads it and
  streams {offer/data/end} (36 KB→base64 chunks) back over `ipc.KindFileData`.
  `transport.go` relays worker→transport KindFileData onto each viewer's 'file'
  channel as TEXT via new `Peer.SendFileTransferText` (viewer ignores binary
  there); `CreateAgentOffer` now stores `FileTransferDC`. `fileReceiver` gained
  the conn + an id counter. Bundled with r34 (import + image clipboard) → r35.
  RISK: native picker struct layout / dialog focus on the remote desktop are
  untested-on-hardware. STILL OPEN: chat, privacy-mode execution, SAS.
- **2026-07-13 — TransportMode Phase B, batch 3: file transfer viewer→host
  (import) (r34-filexfer) — pending hardware validation.** Viewer→host file send
  now works in TransportMode: `transport.go` routes the 'file' data channel
  (previously dropped — OnData only handled control/cursor) to the worker via new
  `ipc.KindFileData`; new `filerecv.go` parses the {k:'ft',offer/data/end} stream
  (reliable+ordered channel, so chunks append in order) and writes to the
  logged-in user's Downloads (path-sanitized, unique-name). `sendInputToWorker`
  generalized to `sendToWorker(kind,raw)`. Host→viewer "export" ({t:request})
  needs a native file picker on the headless host — deferred (logged as
  unsupported). Bundled with batch 2 (image clipboard) into r34. STILL OPEN:
  file EXPORT (host→viewer), chat, privacy-mode execution, SAS.
- **2026-07-13 — TransportMode Phase B, batch 2: image clipboard both ways
  (r33-imgclip) — pending hardware validation.** Extends clipboard-over-transport
  from text to images. New `clipimg_windows.go` reads the host clipboard's CF_DIB
  → PNG and writes a viewer PNG back as a top-down 32bpp CF_DIB (hand-rolled
  syscall, no cgo/dep; `clipimg_other.go` stubs). `clipboard.go`: poll gated on
  `GetClipboardSequenceNumber` re-reads the bitmap only on change and pushes it
  via new `ipc.KindClipboardImage`; `handleInbound` reassembles the viewer's
  chunked `{"k":"clip","img":1,"i","n","d"}` (48 KB base64, in order) → decode →
  write; FNV hash + seq echo-guard both ways. `transport.go` `broadcastClipImage`
  chunks the worker's PNG to viewers in the exact Flutter format. Viewer side
  unchanged (its clip watcher `_ensureClipboardSync` already runs on connect and
  the transport relays over the control channel). r32 also shipped: reliable
  host-OS announce (retry until control DC open) so Privacy/Login buttons appear.
  STILL OPEN: file transfer (import/export), chat, privacy-mode execution, SAS.
- **2026-07-13 — TransportMode Phase B, batch 1: host-OS announce + session
  commands (r31-cmds) — pending hardware validation.** r30 confirmed the crop
  fix; user's r31 test surfaced the remaining TransportMode parity gaps (the
  transport carries video+input+text-clipboard, but the viewer's chat/cmd/ft/
  image-clip messages ride the control channel and were dropped by the worker).
  Batch 1: (a) transport announces `{"k":"os","v":"windows"}` on viewer connect
  (`transport.go` OnConnected) — the viewer gates the Windows-only Privacy/Login
  toolbar buttons on `remoteHostOs=='windows'`, so without this they were HIDDEN
  (user issue "privacy button missing"); (b) `handleCommand` (new
  `command_windows.go`, no-op `command_other.go`) runs lock/logoff/reboot in the
  worker's user session (LockWorkStation / ExitWindowsEx + SeShutdownPrivilege),
  wired into the worker's control-channel reader before input injection (user
  issue "lock not working"). STILL OPEN (later batches): chat relay, file
  transfer (import/export), image clipboard, privacy-mode execution, SAS.
- **2026-07-13 — TransportMode capture: DPI-aware + capture-size logging
  (r30-capture-dpi) — pending hardware validation.** With the viewer CONFIRMED on
  r29-view (app.log stamp now truthful) the remote screen STILL cropped → the crop
  is the TransportMode Go-worker capture, not the viewer render (Contain is
  correct). The Go host exe had no DPI manifest → DPI-UNAWARE: on a scaled display
  the GDI capture (`GetSystemMetrics(SM_CXSCREEN)`, logical) and `Bounds`
  (`GetDeviceCaps(DESKTOPHORZRES)`, physical) disagree — a classic lost-right/
  bottom-edge cause. Fix: `setProcessDpiAware()` (new `dpi_windows.go`,
  PER_MONITOR_AWARE_V2 via `SetProcessDpiAwarenessContext`; no-op stub
  `dpi_other.go`) called in `RunCaptureWorker` BEFORE `NewPlatformCapture`, so
  capture grabs the full PHYSICAL desktop across 125/150/175%. Added worker.log
  lines for `capture bounds` and `captured frame size` to confirm on hardware
  whether the frame equals the full screen. Viewer/transport/input/secure-desktop
  untouched. NEXT (still open): clipboard + file-transfer over the transport are
  Phase B (never carried) — user wants them; separate follow-up after the crop is
  confirmed fixed.
- **2026-07-13 — REGRESSION + FIX: viewer full-screen restored (r27-view broke it,
  reverted to r20 render) — VIEW-ONLY.** Report: host screen cut off (right/bottom
  pushed off-view) on `r27-view`, while `r20` showed the full host desktop edge to
  edge. Root cause = a change I made in `r27-view` (`831220a`): it REPLACED r20's
  proven one-liner — `RTCVideoView(objectFit: fillMode ? Cover : Contain)`, which
  lets the renderer scale the whole frame to fit (Contain) off the host's ACTUAL
  decoded resolution — with a hand-rolled geometry layer (`_videoRect` +
  `Positioned.fromRect` inside `ClipRect`/`Stack`, bypassing objectFit). The
  premise ("objectFit unreliable on Windows") was WRONG; r20 proves Contain works.
  The manual Positioned/Stack sizing didn't fill the viewer area the way objectFit
  does → the video overflowed and the ClipRect cropped the right/bottom. Fix:
  `git revert 831220a` — restores r20's `objectFit` render + the existing Fit/Fill
  toggle (`_fillModeProvider`, default Fit=Contain=full screen; Fill=Cover),
  keeping the r25 stuck-modifier + taskbar-overlap fixes intact. Build stamp
  bumped `r27-view`→`r28-viewfix` so the restored build is identifiable. LESSON:
  do NOT hand-roll video geometry — `objectFit: Contain` already scales correctly
  across resolutions and Windows DPI (125/150%) off the real frame size, no
  hardcoding. NO change to capture/transport/worker/secure-desktop/UAC/input/
  clipboard. Original 1:1 mode deliberately NOT added (a hand-rolled sizing mode
  is exactly what regressed; revisit separately only if needed).
- **2026-07-13 — Diag: transport→worker input path made observable (Go, `acae7aa`).**
  Sampled logging of input routing (worker vs secure/elevated bridge), dropped
  input when no worker attached, and `SendInput` non-landing + inject-thread
  desktop name → `transport.log`/`worker.log`. Diagnostic only, no behavior
  change. (Input later confirmed working by the user; retained for future debug.)
- **2026-07-09 — Fix: two host identities (viewer landed on user-app host with
  no input) → collapse to ONE service-owned host + clipboard over transport;
  implements LD-11/LD-12, pending hardware validation.** Logs showed a SYSTEM
  transport (machine id 769370465, full input/secure pipeline) AND a separate
  user-launched Flutter host (per-install id 318504232); when the transport
  briefly lost signaling (~27 min, infinite-backoff reconnect), the viewer landed
  on the user-app host → screen but NO SYSTEM input. Code root cause: only
  `_autoStartHost` was gated; three other `startHosting` sites (settings
  reconnect, Share button, fixed-password) registered a host regardless, and the
  id fell back to per-install `_persistentAgentId` when the helper wasn't reached.
  Fix (scope = option b, copy-paste preserved): (1) guard inside `startHosting`
  keyed off `HostMode.serviceOwnsHosting()` (new; reads `transportMode`) so the
  app NEVER registers a host in service mode — UI-only, shows the machine
  id+password via `_showServiceIdentity` (fetchMachineCreds); (2) `ServiceMain`
  mints `machine.dat` before launching the transport (stable id first-boot too);
  (3) **clipboard over the transport** so copy-paste doesn't regress:
  `agent/session/clipboard.go` — the worker (logged-in user) applies inbound
  viewer clipboard (control-channel {"k":"clip"}) via atotto/clipboard and polls
  host clipboard changes → `ipc.KindClipboard` → transport `broadcastClip` →
  viewers on the control channel as TEXT (`Peer.SendControlText`; the viewer
  ignores binary there). Text both ways; file clipboard still via helper
  clipagent; image clipboard over transport is a follow-up. Deployment-safe: no
  hardcoded sessions (dynamic `GetTargetSessionId`), account-type agnostic
  (`WTSQueryUserToken`), timing-safe (worker `DialRetry` + infinite signaling
  backoff), standard ProgramData path.
- **2026-07-09 — Fix: black screen after switch (worker died on dial-refused,
  never retried) — implements LD-10, pending hardware validation.** `transport.log`
  confirmed the transport (session-0, pid 20792) persisted and did NOT
  re-register (correct); `worker.log` showed the session-2 worker hit
  `FTL session process exited error="dial tcp 127.0.0.1:47930: ...refused"
  mode=capture-worker` and never logged "connected" — root cause = the worker
  dialed the transport ONCE (`ipc.Dial`) and `main.go` `log.Fatal`'d on refused,
  so it lost the startup race on a switch and died, leaving the transport with no
  frame producer (black; input still worked via the agent/secure-bridge pipe).
  Narrow fix, three parts, nothing else touched (transport lifecycle,
  registration, session detection, secure-desktop capture, copy-paste all
  unchanged): (1) `agent/ipc/ipc.go` `DialRetry` — the worker retries the dial
  ~300 ms up to 15 s instead of exiting; (2) `transport.go` single-producer guard
  — `handleWorker` distributes only from the current worker, so a brief old/new
  overlap can't interleave/corrupt the decoder; (3) `neev_helper.cpp` worker-swap
  — spawn the new worker first and defer terminating the old (`prevWorker`) to the
  next service loop, so the old keeps producing until the new attaches (no
  zero-producer window). Expected: after a switch the new worker connects to the
  already-running transport within ~1 s and frames resume on the SAME connection.
- **2026-07-09 — Seamless switch hardening: secure-desktop bridge + observable
  logs (implements LD-8/LD-9) — pending hardware validation.** Field logs from
  the latest test showed the app BOOTING repeatedly and RE-REGISTERING
  agentId=696561846 per switch, with `host.log` recreating the Flutter engine.
  Diagnosis: those are the DEFAULT Flutter ServiceHost path — the helper log has
  ZERO TransportMode markers (`launched transport in session 0` / `swapping
  capture worker` absent) and instead shows `relaunching host` every switch
  (changing PIDs). So the seamless backend was NOT active for that test
  (TransportMode off / wrong build). The re-register-on-switch model they hit is
  exactly what TransportMode replaces. Two honest gaps in Phase A fixed now:
  • **Observability** (Go): `setupFileLog` (`agent/session/hostlog.go`) tees
    zerolog to `C:\ProgramData\NeevRemote\transport.log` + `worker.log` (stderr
    is discarded under the service's CREATE_NO_WINDOW, which left TransportMode
    undiagnosable). All existing log lines now land in a file.
  • **Secure-desktop bridge** (Go, `agent/session/securebridge.go`): the
    transport connects to the helper's `127.0.0.1:47921` pipe; while the helper
    reports the secure desktop active ('A'/'F'/'G'), it decodes the helper's
    JPEG frames → re-encodes VP8 → feeds the SAME live track (worker frames
    dropped meanwhile; keyframe forced on switch), and translates viewer input
    to the helper's 'I' forwarded-input protocol (sub m/b/w/k). So a
    user-profile switch shows and ACCEPTS the login/UAC password with no
    disconnect, and elevated-window input routes to the helper too. The proven
    helper secure-desktop C++ is untouched (just another pipe client — no
    regression to UAC / secure-desktop capture). `transport.go`: source-switch
    gating in `handleWorker` + input routing in `OnData` (single owner). Shared
    `controlEvent`/`num` moved to securebridge.go (cross-platform).
- **2026-07-09 — Phase A: seamless user-profile switch (TransportMode) built
  end-to-end (native Go + C++ + installer) — pending hardware validation.**
  Delivers LD-7. User approved after diagnosis confirmed: (Q1) the helper service
  genuinely runs as LocalSystem/session 0 (`CreateServiceW(..,nullptr,..)`), so
  `WTSQueryUserToken` works; (Q2) the shipping path only RELAUNCHES the whole
  Flutter host into the new session (transport dies → disconnect), never swaps a
  worker behind a live connection. Fix = finish the opt-in Go transport backend:
  • **Input over the transport** (Go): `ipc.KindInput` carries the viewer's raw
    control JSON transport→worker; new `agent/session/inject_windows.go` is a
    faithful port of `input_injector.cpp` (HID→VK, extended keys, absolute-over-
    primary coords, last-position fallback, single serial goroutine for ordering)
    that SendInputs into the worker's session; `inject_other.go` = no-op stub.
    `transport.go` sets `peer.OnData` (control+cursor) → `sendInputToWorker`.
  • **WebRTC role fix** (Go): the transport is now the OFFERER (was answerer) via
    new `Peer.CreateAgentOffer` creating the exact channels the unchanged Flutter
    viewer binds (`control`/`cursor`/`file`) + trickle offer, and handles the
    viewer's ANSWER. Without this, viewer(answerer)+transport(answerer) deadlock.
    Transport auto-accepts on connect → NO consent dialog.
  • **Same machine creds** (Go): transport reads `machine.dat` (id+password) and
    registers under the machine id, so the viewer uses the SAME credentials as
    the normal host.
  • **Worker as the user** (C++): new `LaunchAsUserInSession` (WTSQueryUserToken →
    DuplicateTokenEx → CreateProcessAsUser on `winsta0\default`); `LaunchWorker
    InSession` now uses it (was SYSTEM-retarget), so capture+SendInput land on the
    logged-in user's desktop. Null at the logon screen → loop retries after login.
  • **No double host** (C++/Dart): `host_mode` now reports `transportMode`;
    `HostMode.shouldAutoHost` returns false when it's on, so a Flutter window
    never fights the transport for the machine-id.
  • **Bundle + ship** (CI/installer): `flutter.yml` Windows job builds
    `neev-host.exe` (Go, CGO+libvpx); `build_windows.ps1` bundles it into the
    installer; new opt-in installer task "Seamless user-switch" sets HKLM
    `TransportMode=1` (default OFF — Flutter host stays default), and writes
    `RelayURL` for the transport.
  NO regression to UAC / secure-desktop capture / clipboard: those stay in the
  unchanged `neev_helper` GDI path + Flutter+helper for the default mode; the
  seamless path is opt-in. Phase B (deferred): carry clipboard/files + commands
  (reboot/lock/SAS) over the transport for full parity before any default cutover.
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
  **M2 DONE 2026-07-08:** process split shipped (all builds green in CI).
  `agent/ipc` = framed loopback protocol; `agent/session` = `RunTransport`
  (persistent, drains worker stream; WebRTC lands in M3) + `RunCaptureWorker`
  (real DXGI capture + libvpx encode → IPC frames; exits on ErrAccessDenied so
  the service respawns it in the new session on a switch). `main.go` dispatches
  `--transport` / `--capture-worker`; default path unchanged.
  **M3 DONE 2026-07-08 (CI green):** `agent/session/transport.go` — transport
  registers (network.Client + FetchICEServers), and per viewer connect creates a
  network.NewPeer (RoleAgent) with its own VP8 rtp.Packetizer; worker frames are
  packetized onto every viewer track with a CONTINUOUS RTP seq/timestamp (so a
  worker swap = brief freeze, not disconnect); viewer PLI/FIR → KindKeyframeReq
  to the worker. **M4 NEXT:** neev_helper launches the transport ONCE in session
  0 (survives switches; networking-only, no desktop) + the capture worker per
  active session via CreateProcessAsUser (swap on switch), behind an opt-in
  `HKLM\SOFTWARE\NeevRemote\TransportMode` flag so default behavior is unchanged.
  Then prove one live frame surviving a switch on hardware.

**M4 DONE 2026-07-09 (both CI green):** `neev_helper` opt-in TransportMode
(HKLM `TransportMode`=1, default off) launches the Go transport once in session 0
+ a capture worker per active session (swap on switch); Flutter host skipped when
on. Transport writes id/password to `ProgramData\NeevRemote\transport.txt`.
Test kit published (NOT in the shipping installer — guardrail): portal
`…/public/installers/seamless-test/` = neev-host.exe + enable/disable-seamless.reg
+ README. **Awaiting user hardware validation of the zero-drop switch.**

## PROGRAM PLAN (user-approved 2026-07-08, in priority order)

1. **Finish transport (M3✓ → M4✓; hardware validation pending)** — zero-drop switch.
2. **Merge branch → main** — ✓ DONE 2026-07-09 (clean fast-forward; main = branch
   HEAD 0828788; all 118 commits / features consolidated).
3. **Flutter UI/UX polish pass** — connect screen, toolbar, settings, file/
   clipboard/chat, visual consistency.
4. **Phase 2 cutover** — Flutter becomes viewer/control-only over the Go backend;
   all features unified. Deliberate, user-approved switchover.
Guardrail: shipping Flutter host (r30) stays default + untouched through 1–3.
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
