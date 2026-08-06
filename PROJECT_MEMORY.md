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

- **LD-AUDIO-5 — echo must be suppressed on the HOST; nothing else can.**
  The viewer's libwebrtc AEC only cancels echo created on the VIEWER's machine.
  When the host plays the viewer's voice, its microphone (acoustic) and its
  system-sound loopback (digital, guaranteed) both re-capture it and send it
  back, so the viewer hears itself. Only the host knows when it is playing
  far-end audio. r144: loopback is DROPPED outright while far-end audio plays
  (it is literally our own output), the microphone is DUCKED to ~-16 dB rather
  than muted (so the host can still interrupt and the call stays two-way), with
  a 250 ms hangover because speakers and rooms do not stop instantly.
  This is suppression, not cancellation — proper AEC needs an adaptive filter,
  pion has no audio processing module, and hand-rolled DSP would be worse.

- **LD-AUDIO-4 — voice codec is OPUS at 48 kHz, via layeh.com/gopus.**
  Replaces PCMU 8 kHz: ~79 bytes per 20 ms frame vs 160, at six times the audio
  bandwidth. gopus VENDORS the Opus C sources and compiles them on amd64
  (Windows + Linux), so there is NO system libopus, no MSYS2 package and no DLL
  to ship — deliberately chosen over hraban/opus, which needs libopus AND
  opusfile via pkg-config on every runner. Adding a native dep to the Windows
  toolchain is exactly what has broken this build before.
  Devices already open at 48 kHz (r140) and Opus is natively 48 kHz, so the
  resampling PCMU forced on every frame is gone. Opus is STATEFUL — one encoder
  and one decoder per worker, reset on teardown; a codec per frame would cost
  more and sound worse. RTP: payload type 111, 48 kHz clock, and the timestamp
  advances by one FRAME of samples per packet — NOT by byte count, which means
  nothing for a compressed codec and would drift the jitter buffer.
  The macOS NeevVoice helper still sends 8 kHz mu-law and is bridged up to PCM
  on receipt, so already-installed bundles keep working.

- **LD-AUDIO-3 — voice IS a WebRTC track on the existing peer connection.**
  One `RTCPeerConnection` carries VP8 video and PCMU audio in one SDP. The raw
  PCM only crosses the worker↔transport IPC, mirroring video exactly
  (`KindVideoFrame` / `KindAudioFrame`+`KindAudioPlay`, same droppable lane),
  because the session-0 transport has no audio device — the same constraint
  that puts capture and input in the worker. It follows a user switch via
  `sendToWorker` targeting the current worker, like input does.
  Checked against RustDesk (closest comparator: Rust core, Flutter UI, same
  session-0 problem): same shape — capture in the session process, resample in
  code, encode, relay. Differences: they use OPUS where this uses PCMU 8 kHz,
  and they gate silence (r142 adds the gate). Notably RustDesk has NO AEC/NS/AGC
  either; on this product the viewer gets it from libwebrtc's audio engine and
  the host has none, since pion has no audio processing module.

- **LD-AUDIO-1 — never decode an RTP packet without checking its PAYLOAD TYPE.**
  A voice track carries more than the negotiated codec: libwebrtc also sends CN
  (comfort noise, PT 13) and telephone-event on the SAME SSRC, and switches to
  CN whenever the microphone is muted or silent. `pumpViewerVoice` forwarded
  every payload to the speakers as mu-law, so CN bytes were decoded as audio —
  noise BY CONSTRUCTION, audible with the mic off, on both machines, unrelated
  to speech. Filter on `track.PayloadType()`; drop padding too.
- **LD-AUDIO-2 — miniaudio already converts between client and hardware format.**
  `channels/format/rate` in the "device opened" log are the CLIENT side;
  `hw_*` are the internal device side, bridged by miniaudio's `ma_data_converter`.
  A mismatch between them is normal and must NOT be "fixed" with a second
  conversion layer — that double-converts. The one value that is NOT converted
  is the rate you request: asking for 8 kHz makes miniaudio run the HARDWARE at
  8 kHz (see r140). Open devices at a rate hardware supports and convert in code.


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
- **LD-23 — The machine password MUST be STABLE across restarts/reinstalls;
  never re-minted per boot. And every connect-failure reason MUST surface a
  distinct, actionable viewer message — never a silent "Connecting…".** ROOT of
  the 2026-07-23 "won't connect one direction" hunt: `machine.dat` carries only
  the id (the installer never sets a password), so `transport.go` setupSignaling
  fell through to `GenerateRandomPassword()` on EVERY boot and never wrote it
  back → the host advertised a fresh password each start → a viewer's saved
  password went stale → the relay rejected it (`hub.go` logs `invalid password
  attempt`, sends `errMsg("invalid password")`). FIX: `persistMachineCreds()`
  writes the resolved id+password back to `machine.dat` on first register, ONLY
  when it had no password (never clobbers a user-set one). Do NOT reintroduce a
  per-boot random password without persisting it. Viewer side: `_friendlyConnectError()`
  maps raw relay errors (password / too-many / offline) to clear text; the raw
  reason still drives retry/stop. **META-LESSON (this class has now cost multiple
  cycles): ONE symptom, MANY causes.** A hung viewer looked identical for stale
  password, consent timeout, consent-toggle-write-failure, and transient Wi-Fi —
  and today it was misattributed first to the r77 IPC change (reverted as r78,
  then restored — r77 was innocent) and then to a consent-popup Accept-wiring
  regression that DID NOT EXIST (the AnyDesk two-Accept popup was never built —
  the only consent UIs are the single-Accept `AlertDialog` in `connect_page.dart`,
  wired to `acceptConnection()`, and the Win32 `MessageBox`; both verified wired).
  BEFORE assuming a code regression on a "won't connect" report: (1) read the
  RELAY logs first (`docker logs deploy-server-1` — `invalid password` / `forwarded`
  / `registered` are decisive), (2) separate config/env (password, consent flag,
  network) from code, (3) confirm the suspected UI element actually EXISTS and its
  button actually calls the backend before "re-wiring" it. AND the standing rule
  the user set: **every UI rebuild of an action button (Accept/Dismiss, Import/
  Export, toggles) must be verified end-to-end against its backend call
  immediately** — this UI-rebuilt-but-backend-disconnected class has recurred, so
  test the full connection flow after ANY change to the consent popup or connect
  buttons.

- **LD-24 — A zero-byte or short transfer is NEVER reported as success, at any
  layer.** A file that is open and being written (a live log) or locked by the
  app that owns it can read as 0 bytes on Windows. Every layer used to accept
  that: the viewer logged `file: send "worker.log" (0 B)`, the host's truncation
  guard was `rf.size > 0 && rf.written != rf.size` so a transfer DECLARING 0
  bytes skipped verification entirely and logged `file transfer finished`, and
  `serveExport` shipped whatever `os.ReadFile` returned and logged `sent file to
  viewer bytes=0`. The user saw a completed transfer and an empty file. RULE:
  reads are verified against the on-disk size (`readFileVerified` in Go,
  `_readVerified` in Dart — retry 3×/150 ms for a transient lock, then a real
  error), the receiver compares written-vs-declared UNCONDITIONALLY, and a file
  that cannot be read fails VISIBLY (`{t:failed}` / `failLocal`) instead of
  moving zero bytes successfully. Applies to import, export and clipboard files.
  Never re-introduce a size>0 escape into an integrity check.

- **LD-25 — GUI that the remote user must actually SEE aborts when it cannot
  attach to the input desktop.** `bindInputDesktop()` logged
  `SetThreadDesktop failed err="The requested resource is in use."` and returned
  normally, so `serveExport` opened a file picker on a thread with no desktop —
  invisible or absent — and the export proceeded anyway. That error is transient
  (the thread still owns a window/hook on its current desktop), so the bind
  retries 3×/150 ms and RETURNS whether it succeeded. Callers whose result the
  remote user depends on (the export picker) must abort and report a failure the
  viewer can retry; callers showing optional GUI (chat window, privacy overlay)
  may ignore it. Note: this was NOT the cause of the 0-byte export in the
  2026-07-30 log — that file was already 0 bytes on disk (see LD-24).

- **LD-26 — Per-session child processes are supervised by what they SERVE, not
  by whether the process object still exists.** The clipboard agent was judged
  solely by `WaitForSingleObject(clip, 0)`, so one that was alive but not
  listening on 47922 was neither "dead" nor "moved" and was never relaunched —
  the file clipboard stayed broken for the rest of the session while every op
  got "the target machine actively refused it". Reachable in practice: with
  `SO_REUSEADDR`, Windows lets a bind take a port another socket already holds,
  so a leftover agent from a previous user session can leave the new one running
  without a live listener. RULE: probe the actual service (non-blocking connect,
  500 ms select, every ~15 s, act after 2 consecutive failures) and restart on
  failure. Any future per-session helper that owns a port gets the same
  treatment. The service process needs its own `WSAStartup` for this.

- **LD-27 — Clipboard files are delayed-render: an announce WITHOUT a pull is
  correct, but a pull that fails must fail LOUDLY.** `clipfann` means the host
  clipboard now holds files; bytes move only when the other side actually pastes
  (`clipfreq`). Copying eight things and pasting one legitimately produces eight
  announces and one pull, so un-pulled announce tokens are NOT evidence of
  failure and must never be given acks, retries or expiry — that machinery would
  fire constantly on correct behaviour. What DOES need to be explicit is a
  failed pull: on Windows the paste is a blocked delayed-render call, so any
  abandoned receive (an out-of-order chunk, an unreadable source) must complete
  as a failure or Explorer hangs until the session ends.

---

## Working Features (confirmed)

- Large file transfer (viewer→host, tens of MB) over TransportMode — receiver-ack
  flow control (r82), hardware-confirmed 2026-07-24. Sends multiple files back-to-
  back incl. a 24MB one under live mouse movement with no ack-timeout, no peer drop,
  no input death. Do NOT pace on `bufferedAmount` (reads 0 on flutter_webrtc Windows)
  — the RECEIVER acks bytes/1MB, the sender windows 2MB ahead (see LD-15 / r82).
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

- **r140 — the static was the DEVICE RATE.** Found from r139's diagnostics:
  `audio: device opened ... hw_rate=8000` on speakers, microphone AND loopback,
  on both machines. Setting `cfg.SampleRate = 8000` makes miniaudio run the
  HARDWARE at 8 kHz; Windows endpoints run at their mix-format rate and almost
  none support 8 kHz natively, so every device was being driven at a rate it
  could not do — continuous static regardless of speech or mute state. Devices
  now open at `DeviceRate` (48 kHz, universally supported) and the 8 kHz wire
  conversion happens in `audio.Downsample` / `audio.Upsample`. Downsampling
  AVERAGES (plain decimation aliases into a metallic warble); upsampling
  INTERPOLATES (sample-and-hold is broadband noise on top of the voice).
  Earlier attempts r138 (stereo buffer shape, idle release) and the r139
  overrun guard were real bugs but not this one — guessing failed twice, the
  log line settled it immediately.

- **r137 — the ACTUAL cause of silent viewer→host voice.** `pc.OnTrack` in
  `agent/network/peer.go` was registered only `if role == RoleController`. The
  host is RoleAgent, so pion never delivered an incoming track to it and
  `transport.go`'s `peer.OnTrack` assignment was DEAD CODE. No SDP change could
  have fixed it. Proven by host logs: `audio track added ... codec=audio/PCMU`
  present, `viewer opened a voice track` absent. Regression test connects two
  real pion peers and asserts a RoleAgent peer receives an audio track.
  r134/r135 (direction + mid matching) were still necessary but were fixes at
  the wrong layer — the lesson is that "host→viewer works, reverse silent"
  points at the RECEIVING side, and I spent two releases on the sender.

- **r134 — viewer→host voice was NEVER working (fixed).** Host→viewer worked,
  so voice looked fine. The viewer adopted the offered audio transceiver and
  took its sender, but never set its DIRECTION — with no mic attached at answer
  time the answer declared `recvonly`, and `replaceTrack` swaps a track, it
  does NOT change a negotiated direction. So enabling the viewer mic opened the
  device and transmitted into a channel the SDP said was one-way. Fix:
  `setDirection(SendRecv)` on the adopted transceiver BEFORE `createAnswer`.
  Lesson: a one-way voice bug is indistinguishable from a dead microphone from
  the outside. Diagnostics added on all three hops — viewer logs the NEGOTIATED
  direction when the mic turns on, transport logs the viewer track opening and
  its first arriving packet, worker logs first playback to the speakers.

- **r133 — the VIEWER can start/stop recording.** Record button in the viewer
  toolbar sends `{k:'cmd',c:'record',on:...}`; the host captures (no re-encode)
  and streams the file back on stop. Handled BEFORE the per-platform
  `handleCommand` so lock/logoff/reboot/privacy keep their existing shape —
  tests assert the record handler consumes ONLY its own message and never mouse
  or keyboard traffic.
  No host approval prompt: the viewer already sees every pixel live, so a
  recording adds no new visibility. It is not silent either — the host's session
  bar turns red "Recording" whoever started it (`setSessionBarRecording`, pushed
  to the macOS helper over the control socket), and the host can stop it.

- **r132 — recordings reach the VIEWER.** Recording still happens host-side
  (VP8 mux, no re-encode), but when the host stops it the file is streamed to
  the connected viewer over the existing file-transfer channel and lands in
  their Downloads. No consent gate: the viewer WATCHED that session live, so the
  file contains nothing new to them — and the host keeps their copy regardless.
  Viewer-side recording remains impossible (flutter_webrtc `startRecordToFile`
  is unimplemented on Windows/macOS; `captureFrame` exists but polling PNGs is
  not a recorder).
  Two transfer bugs found while building it:
  • `ExportFileStreaming` added — the old export read the WHOLE file into memory,
    which at ~11 MB/min of recording is an OOM risk on the host.
  • RECEIVER never checked that the bytes received matched the offered size: it
    wrote whatever arrived, marked it done, acked `saved`, and set
    `transferred = offered size` regardless. Since `end` (hi lane) can overtake
    data (bulk lane), a truncated recording looked identical to a good one. Now
    a length mismatch is an error. This was the receiving half of the sender-side
    short-read bug fixed earlier.

- **r129 features (pending hardware verify).** Remote sound + session recording.
  • Remote sound = WASAPI loopback via miniaudio on Windows. **CORRECTION
    (r130): the earlier claim that macOS cannot capture system audio without a
    virtual device (BlackHole) was WRONG.** ScreenCaptureKit gained audio
    capture in macOS 13 (`SCStreamConfiguration.capturesAudio`), so no driver is
    needed. r130 implements it in NeevVoice.app — Swift, because SCStream is an
    ObjC/Swift framework and TCC attributes capture to the asking bundle — and
    ships 8 kHz mu-law frames to the worker as base64 lines over the existing
    control socket. Needs SCREEN RECORDING permission (macOS captures system
    audio under that grant even for audio-only), and NeevVoice.app needs its own
    grant separate from neev-agent; the helper shows an alert saying so. macOS
    12 and older keep the disabled menu item.
    The Swift mu-law encoder was verified byte-identical to agent/audio's Go one
    across all 65536 sample values — a mismatch would decode as noise, not fail. Mic + system sound share
    ONE PCMU track and are mixed in the PCM domain (`audio.Mix`) — adding mu-law
    bytes would be noise, since mu-law is logarithmic. Quality is 8 kHz mono
    telephone-grade: fine for an error chime or speech, poor for music.
  • Recording = VP8 → WebM MUX in the worker (`agent/record`), reusing frames
    already encoded, so no second encoder competes with the live stream.
    Segment and clusters use UNKNOWN size so a recording cut short by a crash
    still plays — verified with ffprobe in the test suite. Saved to
    ~/Documents/Neev Recordings/. A resolution change closes the file and opens
    a new one, because a WebM track declares one size.
  • BOTH are host-only controls, by design: a viewer able to start recording or
    open the host's mic remotely would make this a surveillance tool.
  • NOT possible viewer-side: flutter_webrtc's MediaRecorder (`startRecordToFile`)
    is unimplemented on Windows and macOS — Dart-side only. Viewer-side recording
    needs native plugin work in libwebrtc.

- **KP-WHENOPEN — FIXED r128 (pending hardware verify).** Interactive Access
  "Only while the app is open" behaved exactly like "always": the headless
  transport could not observe the app window, so `interactiveAllowed()` admitted
  everything while the UI promised "Requests are ignored when the app is
  closed". The app now writes a HEARTBEAT (`app-open.txt`, refreshed every 5s)
  and the transport treats anything older than 15s as closed. A heartbeat, not a
  create/delete flag: a crashed or force-quit app never deletes anything, and a
  stale flag would hold the door open forever. Refusal is the correct failure
  direction — the host chose to be reachable only while at the app.
  Also: refused viewers now receive a bye with a stable reason token
  (`interactive_disabled`, `consent_denied`) instead of sitting on a spinner
  until timeout and blaming the network. The viewer stops auto-reconnecting on a
  refusal — re-dialling a decision means repeated consent popups at someone who
  already said no.

- **KP-VOICE — PARTLY CLOSED in r126 (pending hardware verify).** The Go
  transport now carries a PCMU audio track, capture/playback live in the worker,
  and the host has a "Mic on/off" button on the Windows session bar. Still open:
  CLOSED in r127: macOS hosts now get NeevVoice.app, a menu-bar item with
  "Remote session active", a microphone toggle and End session, started by the
  worker for the life of a session. It is an .app bundle rather than a bare tool
  because macOS shows the mic prompt using the CALLING process's Info.plist — a
  bare executable gets an unexplained prompt, which users decline. Ad-hoc signed
  with a pinned identifier (com.neev.remote.voice) so TCC grants survive updates.
  Still unverified on hardware: the TCC prompt itself, and audio over a real
  session. Original entry follows.

- **r140 — the static was the DEVICE RATE.** Found from r139's diagnostics:
  `audio: device opened ... hw_rate=8000` on speakers, microphone AND loopback,
  on both machines. Setting `cfg.SampleRate = 8000` makes miniaudio run the
  HARDWARE at 8 kHz; Windows endpoints run at their mix-format rate and almost
  none support 8 kHz natively, so every device was being driven at a rate it
  could not do — continuous static regardless of speech or mute state. Devices
  now open at `DeviceRate` (48 kHz, universally supported) and the 8 kHz wire
  conversion happens in `audio.Downsample` / `audio.Upsample`. Downsampling
  AVERAGES (plain decimation aliases into a metallic warble); upsampling
  INTERPOLATES (sample-and-hold is broadband noise on top of the voice).
  Earlier attempts r138 (stereo buffer shape, idle release) and the r139
  overrun guard were real bugs but not this one — guessing failed twice, the
  log line settled it immediately.

- **r137 — the ACTUAL cause of silent viewer→host voice.** `pc.OnTrack` in
  `agent/network/peer.go` was registered only `if role == RoleController`. The
  host is RoleAgent, so pion never delivered an incoming track to it and
  `transport.go`'s `peer.OnTrack` assignment was DEAD CODE. No SDP change could
  have fixed it. Proven by host logs: `audio track added ... codec=audio/PCMU`
  present, `viewer opened a voice track` absent. Regression test connects two
  real pion peers and asserts a RoleAgent peer receives an audio track.
  r134/r135 (direction + mid matching) were still necessary but were fixes at
  the wrong layer — the lesson is that "host→viewer works, reverse silent"
  points at the RECEIVING side, and I spent two releases on the sender.

- **r134 — viewer→host voice was NEVER working (fixed).** Host→viewer worked,
  so voice looked fine. The viewer adopted the offered audio transceiver and
  took its sender, but never set its DIRECTION — with no mic attached at answer
  time the answer declared `recvonly`, and `replaceTrack` swaps a track, it
  does NOT change a negotiated direction. So enabling the viewer mic opened the
  device and transmitted into a channel the SDP said was one-way. Fix:
  `setDirection(SendRecv)` on the adopted transceiver BEFORE `createAnswer`.
  Lesson: a one-way voice bug is indistinguishable from a dead microphone from
  the outside. Diagnostics added on all three hops — viewer logs the NEGOTIATED
  direction when the mic turns on, transport logs the viewer track opening and
  its first arriving packet, worker logs first playback to the speakers.

- **r133 — the VIEWER can start/stop recording.** Record button in the viewer
  toolbar sends `{k:'cmd',c:'record',on:...}`; the host captures (no re-encode)
  and streams the file back on stop. Handled BEFORE the per-platform
  `handleCommand` so lock/logoff/reboot/privacy keep their existing shape —
  tests assert the record handler consumes ONLY its own message and never mouse
  or keyboard traffic.
  No host approval prompt: the viewer already sees every pixel live, so a
  recording adds no new visibility. It is not silent either — the host's session
  bar turns red "Recording" whoever started it (`setSessionBarRecording`, pushed
  to the macOS helper over the control socket), and the host can stop it.

- **r132 — recordings reach the VIEWER.** Recording still happens host-side
  (VP8 mux, no re-encode), but when the host stops it the file is streamed to
  the connected viewer over the existing file-transfer channel and lands in
  their Downloads. No consent gate: the viewer WATCHED that session live, so the
  file contains nothing new to them — and the host keeps their copy regardless.
  Viewer-side recording remains impossible (flutter_webrtc `startRecordToFile`
  is unimplemented on Windows/macOS; `captureFrame` exists but polling PNGs is
  not a recorder).
  Two transfer bugs found while building it:
  • `ExportFileStreaming` added — the old export read the WHOLE file into memory,
    which at ~11 MB/min of recording is an OOM risk on the host.
  • RECEIVER never checked that the bytes received matched the offered size: it
    wrote whatever arrived, marked it done, acked `saved`, and set
    `transferred = offered size` regardless. Since `end` (hi lane) can overtake
    data (bulk lane), a truncated recording looked identical to a good one. Now
    a length mismatch is an error. This was the receiving half of the sender-side
    short-read bug fixed earlier.

- **r129 features (pending hardware verify).** Remote sound + session recording.
  • Remote sound = WASAPI loopback via miniaudio on Windows. **CORRECTION
    (r130): the earlier claim that macOS cannot capture system audio without a
    virtual device (BlackHole) was WRONG.** ScreenCaptureKit gained audio
    capture in macOS 13 (`SCStreamConfiguration.capturesAudio`), so no driver is
    needed. r130 implements it in NeevVoice.app — Swift, because SCStream is an
    ObjC/Swift framework and TCC attributes capture to the asking bundle — and
    ships 8 kHz mu-law frames to the worker as base64 lines over the existing
    control socket. Needs SCREEN RECORDING permission (macOS captures system
    audio under that grant even for audio-only), and NeevVoice.app needs its own
    grant separate from neev-agent; the helper shows an alert saying so. macOS
    12 and older keep the disabled menu item.
    The Swift mu-law encoder was verified byte-identical to agent/audio's Go one
    across all 65536 sample values — a mismatch would decode as noise, not fail. Mic + system sound share
    ONE PCMU track and are mixed in the PCM domain (`audio.Mix`) — adding mu-law
    bytes would be noise, since mu-law is logarithmic. Quality is 8 kHz mono
    telephone-grade: fine for an error chime or speech, poor for music.
  • Recording = VP8 → WebM MUX in the worker (`agent/record`), reusing frames
    already encoded, so no second encoder competes with the live stream.
    Segment and clusters use UNKNOWN size so a recording cut short by a crash
    still plays — verified with ffprobe in the test suite. Saved to
    ~/Documents/Neev Recordings/. A resolution change closes the file and opens
    a new one, because a WebM track declares one size.
  • BOTH are host-only controls, by design: a viewer able to start recording or
    open the host's mic remotely would make this a surveillance tool.
  • NOT possible viewer-side: flutter_webrtc's MediaRecorder (`startRecordToFile`)
    is unimplemented on Windows and macOS — Dart-side only. Viewer-side recording
    needs native plugin work in libwebrtc.

- **KP-WHENOPEN — FIXED r128 (pending hardware verify).** Interactive Access
  "Only while the app is open" behaved exactly like "always": the headless
  transport could not observe the app window, so `interactiveAllowed()` admitted
  everything while the UI promised "Requests are ignored when the app is
  closed". The app now writes a HEARTBEAT (`app-open.txt`, refreshed every 5s)
  and the transport treats anything older than 15s as closed. A heartbeat, not a
  create/delete flag: a crashed or force-quit app never deletes anything, and a
  stale flag would hold the door open forever. Refusal is the correct failure
  direction — the host chose to be reachable only while at the app.
  Also: refused viewers now receive a bye with a stable reason token
  (`interactive_disabled`, `consent_denied`) instead of sitting on a spinner
  until timeout and blaming the network. The viewer stops auto-reconnecting on a
  refusal — re-dialling a decision means repeated consent popups at someone who
  already said no.

- **KP-VOICE (as filed at r125) — in-session voice does not work against a
  TransportMode host.** The Flutter host/viewer path carries two-way audio, but a
  TransportMode host is the Go transport, which builds a video track only
  (`agent/network/peer.go` adds one `TrackLocalStaticRTP`, VP8) — so those
  sessions have no `m=audio` section at all. Since TransportMode is the default
  install, most users get no voice. This is SURFACED, not hidden:
  `RemoteService.voiceAvailable` is false and the mic button disables with an
  explanation. Plan for closing it (design settled, not built):
  - Capture/playback must live in the **capture worker**, not the transport. The
    worker runs in the user session via `CreateProcessAsUser` and therefore has
    an audio session; the SYSTEM transport does not.
  - Use **G.711 PCMU, not Opus**. PCMU is ~40 lines of pure Go (mu-law table),
    is a standard WebRTC codec, and needs no libopus — which would mean a new
    system dependency on both CI runners. Telephone-grade 8 kHz mono is right
    for support voice, and keeping the Windows toolchain unchanged protects the
    Windows-to-Windows baseline (LD rule).
  - Device I/O via header-only miniaudio (cgo, compiles inline, no external
    lib). cgo is already in the build for libvpx, so this adds no new class of
    dependency.
  - New IPC kinds for audio in both directions, mirroring `KindVideoFrame`;
    transport packetizes to an RTP audio track and depacketizes `OnTrack` back
    down to the worker.
  - The Flutter viewer needs NO change: it adopts whatever audio transceiver the
    offer contains and does not munge the `m=audio` section.

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

- **2026-08-03 — Unattended access is no longer optional; seamless transport has
  a crash-loop fallback (r120).** The installer offered "Seamless user-switch
  (experimental)" UNCHECKED, plus a second unattended-access checkbox. A user who
  skipped them silently got a lesser product — that path carries unattended
  access, the host consent prompt, host-side view-only enforcement and the
  session bar — and could not fix it afterwards: the flags live in
  `HKLM\SOFTWARE\NeevRemote` and nothing in the app can write them, so the only
  remedy was reinstalling. Both checkboxes are removed and both flags are now
  written unconditionally; only the desktop-shortcut choice remains.
  **This was only safe once the service could give up.** The helper previously
  relaunched the transport forever with no alternative, so a `neev-host.exe`
  that could never start left the machine unreachable with a log full of
  relaunch lines. It now counts a transport exiting within 15s of launch (or
  failing to launch at all) as a fast failure, and after three marks seamless
  broken and runs the Flutter host instead — regardless of the ServiceHost flag,
  because unreachable is the worst outcome. Logged loudly, never silent.
  **LD-34 (new): a capability the product depends on must not be an installer
  checkbox that is OFF by default, especially when nothing in the app can turn
  it on afterwards — getting it wrong once meant reinstalling. Default it on and
  make the failure path recoverable, rather than relying on the user to choose
  correctly.**
  The fallback is deliberately TEMPORARY: `transport.Run()` exits immediately if
  `ipc.Listen(47930)` fails, which is exactly what an upgrade does while the old
  transport still holds the port, so a permanent fallback would have downgraded
  healthy machines until reboot. Threshold 5 fast failures, retried after a
  5-minute cool-off. A relay outage cannot trigger it at all — the signaling
  client reconnects with backoff and its error is logged, not returned.

- **2026-08-03 — Consent card flicker fixed at the root; stale prompt withdrawn;
  new logo applied (r114–r115).**
  **(a) FLICKER.** Removing the map animation was necessary but NOT sufficient —
  the animation was only one trigger. The card was painted DIRECTLY to the
  window, so every repaint rebuilt it in place (background, map polygons, text)
  and the intermediate states were visible. Hovering Accept/Decline invalidates
  the whole card, so the flash happened exactly while the user was deciding.
  Fixed by drawing into an off-screen bitmap blitted in one BitBlt, and claiming
  WM_ERASEBKGND so the system never blanks the window first. Session bar gets the
  same treatment. **LD-33 (new): any owner-drawn Win32 surface here must be
  double buffered and must claim WM_ERASEBKGND. Hover states alone repaint often
  enough that direct drawing is visibly broken.**
  **(b) STALE CONSENT PROMPT.** If the viewer cancelled while the host's prompt
  was open, it stayed up until dismissed by hand — nothing told the host the
  question was moot; the transport just waited out its 30s timeout and the worker
  was never notified. Added `KindConsentCancel`; withdrawn on viewer bye, on
  askConsent giving up (covers cancel/timeout/shutdown), via WM_CLOSE on Windows
  and by killing osascript on macOS. **The Flutter host had the same bug on its
  own path** (bye removed the peer but left `_pendingConsent` set and the dialog
  on screen) — fixed there too.
  **(c) BRANDING.** New logo applied to macOS/Windows app icons, Flutter web
  favicon + PWA icons, the deployed website and the Wails client. Two latent
  problems found: `web/index.html` referenced `/favicon.png` **which never
  existed** (broken favicon all along), and `.gitignore`'s blanket `public/` rule
  would have silently swallowed the new one — narrow exceptions added. Browser
  tab identity corrected from `neev_remote` to "Neev Remote".
  **OPEN: the logo is green while DESIGN.md specifies an orange accent
  (#F05A28) used throughout the UI. Shipped as-is on the user's instruction; the
  icon and the app currently read as different brands. Needs a decision: retune
  the design system to green, or recolour the mark.**

- **2026-08-01 — Privacy becomes a LEASE; End button moved to the page that
  actually renders; laptop artwork corrected (r112).** All three reported as
  "still broken" after r111.
  **(a) THE END BUTTON WAS MY REGRESSION.** It was added to
  `CommandActivityPanel`, which **nothing instantiates** — dead code from the
  redesign, so it could never appear. Moved to a live-session banner at the top
  of `HomeCommandCenter` (the rendered page), showing who is watching and
  whether they hold control or view-only. `RemoteService` now notifies on viewer
  CONNECT as well as removal, or the banner would not appear until something
  else rebuilt the page. **Lesson: verify a widget is actually instantiated
  before adding a control to it — the analyzer reports these as
  `unused_element`, and several other redesign leftovers are still listed.**
  **(b) PRIVACY IS NO LONGER A LATCH.** The r111 fix cleared privacy when the
  transport reported zero viewers. That wiring is correct but depends on
  NOTICING a disconnect, and every disconnect signal can fail: a crashed or
  network-dropped viewer sends no bye, an unclean teardown never reaches
  dropPeer, a lost IPC message takes the session-state notice with it. Now the
  viewer re-asserts privacy every 4s and the host restores itself 12s after
  those stop — for any reason. **LD-32 (new): any state that can lock a user out
  of their own machine must EXPIRE on its own, never depend on observing a
  disconnect. Session-end teardown stays as the fast path, never as the
  guarantee.**
  **(c) ARTWORK.** The accent laptop read as a leaning slab: bezel and display
  were both accent-filled so no edge defined the screen plane, over a
  tint-on-white deck that was nearly invisible. Both machines now share one
  construction and differ only in display fill; bezel inset 0.055 -> 0.13.

- **2026-07-31 — Access model reworked end to end: host authority, AnyDesk-style
  unattended access, host-side disconnect, privacy lock-out fix (r109–r111).
  Pending hardware validation.**
  **(a) VIEW-ONLY WAS BACKWARDS.** Reported: enabling view-only on the HOST did
  nothing while enabling it on the VIEWER worked. View-only was enforced only in
  RemoteViewWidget (viewer side), so it was an honour system; `permControl`
  existed on the host, was assigned in three places and READ IN NONE. The host is
  now authoritative and drops control attempts where input is consumed
  (`transport.go` for SYSTEM/root, `remote_service.dart` for the Flutter host).
  Blocked: mouse/keys/wheel + lock/logoff/reboot/privacy/SAS. Still allowed:
  clipboard, chat, file transfer, monitor switch — a watcher stays useful.
  Two later holes closed in the same class: the UNATTENDED path and the
  REMEMBERED-decision path both took control from `defaultPermControl` and
  ignored the host's view-only setting entirely.
  **(b) UNATTENDED ACCESS NOW MATCHES ANYDESK.** The relay already verified each
  connection against the session password OR the unattended password, then threw
  that distinction away — so `askOnConnect=false` admitted ANYONE holding the
  session password unprompted. The relay now stamps `auth: unattended|session`;
  unattended logins skip the prompt (that password IS the authorisation), session
  logins are still prompted. Interactive Access is three-state
  (always / when-open / never); "never" leaves the unattended password as the only
  door. Separate permission profiles per mode, with clipboard and files now
  enforced (previously only control was). An older relay omits the field and falls
  back to "session" — the side that prompts.
  **(c) THE HOST COULD NEVER HANG UP.** Only the viewer had a disconnect control.
  Added `KindEndSession` + `KindSessionState`, a native always-on-top
  "Remote session active / Disconnect" bar on Windows (topmost, non-activating;
  closing it only hides it — dismissing an indicator must not disconnect anyone),
  and `endHostSession()` + a button on the Flutter host. **Gap: no native
  indicator on the macOS daemon host** (its worker has no window infrastructure).
  **(d) PRIVACY MODE LOCKED USERS OUT OF THEIR OWN MACHINE.** Reported: privacy ON
  + disconnect left the host blanked with local input blocked, recoverable only by
  reconnecting to turn it off. Privacy was only ever toggled by an explicit viewer
  command and nothing cleared it when a session ended. Now torn down on every path
  (zero-viewer signal, worker start AND defer-on-exit, every Flutter peer-removal
  route). **The start-side clear matters most on macOS: the gamma table PERSISTS
  after the process dies, so a crash with privacy on blacked the display until
  someone reset it manually.** `PrivacyMode` was fire-and-forget and now tracks
  state; a failed DISABLE stays marked ON so teardown retries.
  **(e) CONSENT CARD.** Redrawn to the approved landscape design, then corrected:
  800x560 -> 700x452, and the "laptops" that rendered as stray lines were a
  PROJECTION bug — an isometric rhombus spans both axes but scale came from `du`
  alone, so a 132px-wide laptop was drawn 230px wide with ~98px clipped off-panel.
  **LD-29 (new): the HOST is the sole authority on access level. Viewer-side
  view-only is a courtesy; enforcement belongs where input is consumed. Any new
  path that admits a viewer MUST record an explicit grant — and unknown viewers
  default to ALLOWED, never denied, so an unset flag can never silently kill
  working features (the footgun that got the original host gate deleted).**
  **LD-30 (new): whether a connection is prompted depends on HOW it
  authenticated, never on a global switch. The unattended password is the
  authorisation; the session password is a request.**
  **LD-31 (new): privacy mode is SESSION state, not machine state. It must be
  cleared on every path a session can end, including worker start (a previous
  crash) — on macOS the gamma blanking survives process death.**

- **2026-07-31 — CRITICAL: the r106 native consent prompt worked only ONCE per
  worker, then denied every connection (r108).** Reported from the field as
  "connects only 1 time; closing and reopening the app doesn't help" — the
  viewer sat on "Negotiating display quality" forever. Restarting the viewer
  could never help: the wedge was in the HOST worker process. Two independent
  cross-prompt state leaks, both mine, both shipped in r106/r107:
  (1) The window class was registered PER PROMPT with a fresh
  `syscall.NewCallback` closed over that `consentWin`. The second
  `RegisterClassW` fails with ERROR_CLASS_ALREADY_EXISTS (the code comment
  asserting this "is fine" was wrong), so the class kept the FIRST callback and
  every later window was driven by the first prompt's state — Accept set
  `answered` on a struct nobody was watching, the loop never exited,
  `showConsentDialog` never returned, and the transport denied on its 30s
  timeout. Also leaked one callback per prompt against a small process-wide cap.
  (2) `answer()` and WM_DESTROY called `PostQuitMessage`; WM_QUIT lands on the
  THREAD queue, and `showConsentDialog` locks/unlocks its OS thread, so a pooled
  thread could carry a stale WM_QUIT into the next prompt, whose `GetMessage`
  returns 0 immediately and denies. Fixes: register the class exactly once
  (`sync.Once`) behind one stable window procedure that resolves the prompt by
  hwnd, and never post WM_QUIT (every answer arrives from a dispatched message,
  so the loop exits on its own). Only reachable with "Ask before allowing
  connections" ON.
  **LD-28 (new): a process-wide Win32 window class is registered ONCE, behind a
  stable window procedure that resolves per-window state by hwnd. Never close a
  wndproc over one dialog's state, and never PostQuitMessage from a dialog that
  runs on a pooled/locked OS thread.**
  Two reporting lessons recorded: view-only does NOT touch the WebRTC/peer setup
  and cannot stop the remote stream, so that correlation was incidental; and the
  connect screen's four stage checkmarks are a COSMETIC 420ms timer that holds
  on the last stage until the stream arrives — they indicate nothing about real
  progress and are a Data Honesty violation still to be fixed.

- **2026-07-31 — macOS consent prompt implemented; a Mac daemon host no longer
  auto-accepts every connection (r107). Pending hardware validation.** Found
  while answering "will the popup work on Mac too?" — it would not have, and
  worse, NO prompt appeared at all. macOS ships the same two-process Go
  architecture (`com.neev.transport.plist` root LaunchDaemon +
  `com.neev.worker.plist` LaunchAgent), but two things were Windows-only:
  (1) `showConsentDialog` was a `!windows` stub returning **true**, and
  (2) `writeConsentFlag` began `if (!Platform.isWindows) return;`, so
  `consent.txt` was never written, `consentRequired()` returned false, and the
  transport auto-accepted — the "Ask before allowing connections" toggle had NO
  effect on a Mac daemon host.
  Fixes: `consent_darwin.go` shows a real NSAlert **with a "Remember this
  decision" checkbox**, hosted by `osascript -l JavaScript` through the ObjC
  bridge. In-process AppKit is not an option — the agent has no NSApplication
  and no main-thread run loop (`privacy_darwin.go` runs its CFRunLoop on a
  private pthread) and the call arrives on an arbitrary goroutine. The device id
  is passed as argv, never interpolated, so nothing from the wire is executable.
  The flag now travels via the console user's
  `~/Library/Application Support/NeevRemote/consent.txt`, read by the root
  transport through `/dev/console` ownership (`consentflag_darwin.go`); the
  machine-wide path is still checked FIRST so MDM can force it on. The
  non-Windows/non-darwin stub now returns **false** (deny) — returning true
  would auto-accept everything the moment the flag became readable.
  **Also fixes a real bug in the r106 Windows work:** the consent store used
  `dataDir()`, which the daemon creates root/SYSTEM-owned, but the capture
  worker runs as the logged-in USER on both platforms — so saving a remembered
  decision failed with "permission denied" and was silently lost. Proven on
  macOS in a test. Added `userDataDir()` (LOCALAPPDATA / ~/Library/Application
  Support / XDG) and moved the store there, which also scopes a security
  decision to the user who made it. Same class as the r104 hostlog fallback.
  Verified by running the real JXA script on macOS: AppKit construction and
  argv both work and it returns exactly the JSON the Go side parses. 4 Go tests.

- **2026-07-31 — Consent prompt redesigned on BOTH hosts + "Remember this
  decision" wired; view-only actually enforced (r106). Pending hardware
  validation.** Two separate pieces of work.
  (a) VIEW-ONLY was enforced only in `RemoteViewWidget`, which simply doesn't
  wire its pointer/Focus listeners when viewOnly is set. That covers the mouse,
  but the OS keyboard hook (`KeyboardHook.supported` is true on Windows AND
  macOS — why all three pairs broke) and `sendKeyCombo()` from the shortcuts
  menu call `sendViewerInput` directly and never saw the flag; the host does not
  gate input at all, so everything sent was injected. The gate now lives in
  `sendViewerInput`, the documented single funnel. The hook is DISARMED under
  view-only rather than ignored (it swallows keys locally), and held keys are
  released when view-only turns on mid-session. NOT a redesign regression: the
  mode selector was correctly wired the whole time. 5 tests in
  `test/view_only_test.dart`.
  (b) CONSENT PROMPT: the box users actually see on a TransportMode host was a
  stock `MessageBoxW` — its text is verbatim the copy in the approved mockup,
  which is how that was identified. `MessageBoxW` can't render the design, so
  `consentwin_windows.go` now hand-draws the card (GDI owner-draw, DPI-scaled,
  self-hit-tested buttons), with the MessageBox kept as a fallback so a
  windowing failure degrades to a plain prompt instead of denying a legitimate
  connection. `consent_dialog.dart` mirrors it for macOS/attended hosts. Closing
  or dismissing the prompt is always a refusal, never an accept.
  "Remember this decision" is fully wired in both directions — a remembered
  Decline auto-declines — with separate stores per host mode
  (`consent-decisions.json`, SharedPreferences), written atomically via
  temp+rename. `ForgetConsentDecisions()` / `ConsentStore.forgetAll()` exist so
  a remembered Decline is undoable; **neither is surfaced in Settings yet — that
  is the known gap.** A render test caught a 178px footer overflow before it
  ever shipped.

- **2026-07-30 — Four evidence-based fixes from worker.log: silent zero-byte
  transfers, the ignored desktop bind, unsupervised clipagent, and hung
  clipboard pulls (r105). Implements LD-24..LD-27. Pending hardware
  validation.** Three of the four reported root causes were confirmed in code;
  the fourth was investigated and REJECTED with its evidence (see LD-27).
  (1) `filerecv.go` end-guard `rf.size > 0 && rf.written != rf.size` skipped
  verification whenever a transfer declared 0 bytes → `file transfer finished
  size=0` and an empty file in Downloads. Now compared unconditionally. The 0
  bytes originated on the SENDER, where an in-use file (`worker.log`, a
  spreadsheet open in Excel) read as empty and was sent as a valid transfer;
  both sides now verify reads against the on-disk size with a short retry, and
  report a visible failure instead. `serveExport` aborts rather than logging
  `sent file to viewer bytes=0`.
  (2) `bindInputDesktop()` warned `SetThreadDesktop failed err="The requested
  resource is in use."` and returned normally; `serveExport` then opened a
  picker on a desktop-less thread. Now retries 3×/150 ms, returns success, and
  the export aborts with `{t:failed}` when it can't attach. NOTE: this did not
  cause the 0-byte Airtel export — that file was already 0 bytes on disk from
  the import at 16:27:56, so the correlation in the log is coincidental. Two
  real bugs, not cause and effect.
  (3) `clipagent dial FAILED ... actively refused` with no recovery: the helper
  judged the clipboard agent by process liveness only, so an agent that was
  alive but not listening was never relaunched. Added a loopback health probe
  (+ `WSAStartup` in the service process, which had none, and a log line for a
  failed clipagent launch — previously silent).
  (4) "Announce tokens never retrieved" is NOT a bug — it is the documented
  deliver-on-paste design (LD-4/LD-27), and the log contains a healthy pair
  (18:23:33 announce h2 → 18:23:42 served). No ack/retry/expiry added. The real
  gap was a FAILED pull: an out-of-order chunk removed the receive record and
  returned, leaving the blocked delayed-render paste hung in Explorer until the
  session ended; and the source side swallowed read errors in `catch (_) {}`.
  Both now complete as explicit failures and are logged.
  Large-file transfer is deliberately untouched — chunking, the 1 MB `{t:prog}`
  acks and the 2 MB send window (r82, hardware-confirmed) are unchanged, and a
  large file stats and reads equal so it takes exactly the same path.

- **2026-07-24 — THE real large-upload fix: receiver-driven ack flow control (r82).
  ✅ HARDWARE-CONFIRMED by user ("now everything is working after this test").
  Supersedes the r77/r81 keyframe theory — that was the WRONG layer.** Decisive
  logs: viewer app.log `sent end` for a 24MB file just 1.3s after `send` (a full
  dump, no pacing), then host worker.log `receiving file progress written=8404992`
  then NOTHING, then viewer `ack TIMEOUT` + peer dies + input dies. ROOT CAUSE: the
  viewer never paces large uploads. `FileTransferManager.sendFile` gated on
  `bufferedAmount` (`while buffered() > 512KB`), but flutter_webrtc's `bufferedAmount`
  reads ~0 on Windows (async-cached value the tight send loop starves), so the loop
  NEVER blocked → the whole file dumped into the ~16MB SCTP send buffer at once →
  overflow → the 'file' data channel + peer connection tore down → the host froze
  ~8MB in and EVERY later transfer failed too (small files fit the buffer, so they
  "worked" — the misleading clue). r80 (clipagent) and r81 (keyframe) both fixed the
  wrong layer; `8404992` was just the every-8MB progress-LOG mark, not a hard buffer.
  FIX (bufferedAmount-independent, drain-rate-adaptive): the RECEIVER acks bytes-
  written every 1MB (`{k:ft,t:prog,id,recv}` — Go `filerecv.go` + Dart host
  `handleMessage 'data'`); the SENDER never runs more than a 2MB window ahead of the
  last ack (`file_transfer_service.dart`), so it paces to the real drain rate on any
  link and can never flood SCTP. Graceful fallback: if the receiver never acks (old
  build) the sender time-paces (~9MB/s) instead of blocking. Refines LD-15 (pacing is
  now receiver-ack-driven, NOT bufferedAmount, which is unreliable). Kept r80+r81
  (independent, correct). Go builds (win) + vet + Dart analyze clean.
- **2026-07-24 — RESTORED the r77 large-upload fix that a botched revert silently
  dropped (r81). THE actual cause of "file/clipboard broken after switch".** The
  viewer app.log (app 10.log) was decisive and reframed the whole hunt: small
  files SAVE instantly (19KB docx, 298KB pdf → `recv saved`), but LARGE uploads
  (24MB exe, 13MB zip) → `sent end` then NO ack → the WHOLE viewer peer dies
  (`no live viewer peer` floods from the instant the big files land) → `ft: ack
  TIMEOUT` at 30s. Input dies with it. This is NOT a clipboard/switch bug — it is
  the exact r77 symptom (large upload → host receive-lane starves → worker wedges
  ~8MB → no ack → peer drops → everything dead). ROOT CAUSE of the regression: the
  r77→r78 revert dance. r77 (b814309, keyframe throttle + IPC writeLoop fairness)
  was reverted by b76b335 (the code revert). The intended "un-revert" then ran
  `git revert HEAD` against the WRONG commit — 68a8ae7, the r78 build-TAG bump
  ("1 file, 1 insertion") — NOT b76b335. So r77's CODE was never restored; r79 and
  r80 shipped without it (verified: ipc.go writeLoop had no `hiStreak` fairness,
  transport.go had no `lastKeyframe`). FIX: `git revert b76b335` (code files only;
  kept current docs/tag) → restored the keyframe throttle (`requestKeyframe`
  ≤1/200ms via `lastKeyframe atomic.Int64`) + writeLoop bulk-fairness (a bulk slot
  after 8 consecutive hi, so a keyframe flood can't starve the file lane). r80's
  clipagent recv-timeout + dial logging STAY (independent, real hardening — the
  dial-fail log confirmed the clipagent was healthy, exonerating it). LESSON
  (reinforces LD-23 meta): after ANY `git revert` of a revert, VERIFY the code
  actually came back (grep for the restored symbols), never trust the commit
  subject — a revert can silently target the tag-bump commit instead of the code.
  Re-test: viewer→host send several files back-to-back INCLUDING a >15MB one while
  moving the mouse; all must save, none time out, peer must not drop. Go builds
  (win+darwin) + vet clean.
- **2026-07-23 — Clipagent wedge-hardening + clipboard-file diagnostics (r80).
  Part 1 of the file/clipboard session-switch investigation — HARDENING + DIAG
  only; the targeted fix + full isolation refactor are GATED on a switch-repro
  log (below).** A deep trace (Go worker/transport/ipc + C++ helper + Dart viewer)
  of the reported "after a user-profile switch, Import/Export AND clipboard break
  globally and don't recover" found the Go per-session layer is CLEAN and
  self-healing: the worker is a SEPARATE PROCESS per session (CreateProcessAsUser),
  so every Go package singleton (incl. chatStart/chatSend) resets fresh each swap;
  the transport repoints `t.worker` under lock on every attach and holds no
  per-worker clip/file state (transport.go:518-536); ipc.Conn is per-connection.
  So the code as-written SHOULD recover on switch-back — which contradicts the
  symptom (clipboard+file break, but video+input keep working). Rather than guess
  a fix blind (the r77/r78 mistake), shipped only what is safe + non-speculative:
  • **Clipagent recv-timeout** (neev_helper.cpp RunClipAgent): the clipboard-file
    agent on 127.0.0.1:47922 is single-threaded with a blocking `recv()` and NO
    read timeout — a half-open/stalled client (e.g. a worker killed mid-op during
    the deferred prevWorker+new-worker swap overlap, both polling 47922) blocks
    the one thread FOREVER, so no future worker is served: a genuine global,
    non-recovering wedge. Added `SO_RCVTIMEO` 5s on accepted sockets (payloads are
    tiny CF_HDROP paths, so a healthy client never hits it). This is the only
    single-slot server in the clipboard-file path (contrast the 47921 UAC server,
    already multi-client). Real robustness bug regardless of whether it is THE bug.
  • **Clipagent dial-failure logging** (clipfiles_windows.go): the polled read-dial
    (`clipAgentReadFiles`, every 700ms) and the write-dial (`clipAgentWriteFiles`)
    swallowed connect errors — the one diagnostic blind spot. Now logged (read is
    throttled to once-until-recovery). So the switch-repro log will show if the
    new-session worker can't reach its clipagent.
  • **REGRESSION_CHECKLIST.md** added (repo root): the mandatory file/clipboard +
    session-switch matrix to run after ANY UI/IPC/session/helper change.
  NEXT (gated): user runs the switch-repro (import/export/text-copy/file-copy each
  tested, original→profile2→profile3→back, both worker.logs + transport.log +
  helper log) → pins the exact op that fails → THEN the targeted fix + the Part 2
  isolation (separate KindClipFile IPC lane, one explicit per-session lifecycle
  contract, Dart file-channel dispatch split) lands as a reviewed refactor with a
  new Locked Decision. Win↔Win capture/input/secure-desktop unchanged (LD-13). Go
  builds (windows) + vet clean; C++ compiled by CI's build_windows.ps1 (cl).
- **2026-07-23 — Stable host password + clear viewer connect errors (r79).
  Implements LD-23.** A long "won't connect one direction / stuck on Connecting"
  investigation resolved to a NON-code + code mix. RELAY logs (`deploy-server-1`)
  were decisive: `invalid password attempt ×5` for one target, `connect request
  forwarded` for the other. ROOT CAUSE: the TransportMode host re-minted a random
  password every boot (`transport.go` fell through to `GenerateRandomPassword()`
  and never persisted it — `machine.dat` had only the id), so a viewer's saved
  password silently went stale and the relay rejected it. FIX: `persistMachineCreds()`
  freezes id+password into `machine.dat` on first register (only when it had no
  password — never clobbers a user-set one); `_friendlyConnectError()` turns raw
  relay errors into actionable viewer text (wrong password / too many / offline)
  so a failure is never a blank "Connecting…". Also folds back the restored r77
  (keyframe throttle + IPC lane fairness) that the r78 revert had removed — r77
  was proven innocent of the connect issue by the relay logs. Detours corrected
  this session (recorded so they aren't repeated): (a) r77→r78 REVERT then
  RESTORE — r77 never broke Win↔Win; the first failure was a consent 30 s timeout
  (`consent.txt=1`, nobody clicked the Win32 box); (b) a hypothesised AnyDesk
  consent-popup Accept-wiring regression was DISPROVEN — that rich popup was never
  built; the only consent UIs (single-Accept `AlertDialog` → `acceptConnection()`,
  Win32 `MessageBox`) are both correctly wired. No change to consent dialog,
  Win↔Win capture/input/secure-desktop, clipboard, chat, or file transfer (LD-13).
  Go builds (darwin + windows) + vet + Dart analyze clean.
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
- A control that cannot do anything must be DISABLED and say why, never left
  looking live. This project has been bitten twice (End button on a widget
  nothing instantiated; mic button with no audio channel behind it).
