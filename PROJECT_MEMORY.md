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

---

## Working Features (confirmed)

- Normal remote control host↔viewer (~20 fps), Windows/macOS/Linux.
- Clicks/drags correct (no click-becomes-drag; no dead clicks; no stuck-Alt →
  double-click opens files, not Properties).
- Discovery shows real machine names (LAN UDP + relay-assisted).
- File **copy** no longer becomes **move** (Preferred DropEffect = Copy).
- Clipboard text/image sync; clipboard sync on/off toggle.
- SYSTEM helper: secure-desktop capture + send (helper log verified 2026-07-08).
- TransportMode capture shows the FULL host screen on scaled displays — DPI-aware
  worker (`setProcessDpiAware`, r30). **Hardware-confirmed 2026-07-13** (user: "screen
  layout fixed"; also text copy/paste, UAC dialog, switch-user all working on r30).
- Viewer shows the FULL host screen by default (r20 behavior: `objectFit: Contain`
  off the host's actual frame size — correct across resolutions + Windows DPI),
  with an optional Fit/Fill toggle in the session toolbar (Fill = `Cover`/crop).
  Restored 2026-07-13 (`r28-viewfix`) after the `r27-view` hand-rolled-geometry
  regression; do NOT reintroduce manual video sizing.

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
