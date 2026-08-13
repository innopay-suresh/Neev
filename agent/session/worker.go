// Package session implements the two-process split for the SYSTEM-service
// transport model: a persistent transport (owns the WebRTC connection) and a
// per-session capture worker (owns the desktop capture), linked over local IPC
// (see package ipc).
//
// Phase 0, milestone 2: process split + IPC skeleton. Capture and WebRTC wiring
// are added in later milestones; for now the worker connects and streams a
// heartbeat so the transport↔worker channel and lifecycle can be validated end
// to end (including worker swap on session change).
package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/capture"
	"github.com/neev/remote-agent/agent/encode"
	"github.com/neev/remote-agent/agent/ipc"
)

// Capture/encode defaults for the PoC. Bitrate/FPS adaptation (already present
// in the all-in-one pipeline) is folded in during parity (Phase 1).
const (
	workerFPS     = 30
	workerBitrate = 3000 // kbps
)

// isFileTransferMsg cheaply peeks a KindFileData payload's kind so the reader can
// route it to the file-transfer lane ({k:"ft"}) vs the clipboard-file lane
// ({k:"clipf*"}). Full parsing happens in the lane's handler.
func isFileTransferMsg(payload []byte) bool {
	var m struct {
		K string `json:"k"`
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return false
	}
	return m.K == "ft"
}

// RunCaptureWorker connects to the transport and (eventually) streams captured,
// VP8-encoded frames. Runs until ctx is cancelled or the transport goes away.
// The service spawns one of these into the active session and replaces it on a
// user switch; the transport connection is unaffected.
func RunCaptureWorker(ctx context.Context, port int) error {
	setupFileLog("worker.log")
	// Failsafe both ways. On the way OUT, a worker that exits (user switch,
	// service restart, crash-and-relaunch) must never leave the screen blanked:
	// on macOS the gamma table persists after the process dies, so a crash with
	// privacy ON would black the display until someone reset it. On the way IN,
	// clear whatever a PREVIOUS worker may have left behind, for the same reason.
	setPrivacy(false)
	defer clearPrivacy()
	// The guarantee: however a session ends — clean bye, viewer crash, network
	// drop, lost IPC — the screen cannot stay blanked longer than the lease.
	go runPrivacyWatchdog(ctx)
	// Make the capture process DPI-aware BEFORE creating any capture DC, so on a
	// scaled display (125/150/175%) the capture grabs the FULL physical desktop
	// instead of losing the right/bottom edges to a logical/physical mismatch.
	setProcessDpiAware()
	if port == 0 {
		port = ipc.DefaultPort
	}
	// macOS session-follow: only the session on the physical console may stream.
	// Block here until we're on-console (no-op off macOS) so a worker spawned into
	// a backgrounded user's session idles instead of streaming the wrong desktop.
	if err := waitUntilOnConsole(ctx); err != nil {
		return err
	}
	// Retry the dial: the persistent transport (session 0) may not be accepting
	// at the instant the service spawns us on a user switch. Without retrying, a
	// single connection-refused would fatally exit the worker, leaving the
	// transport with no frame producer (frozen/black screen). Wait for it.
	conn, err := ipc.DialRetry(ctx, port, 15*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Serialize every write to this connection: the reader goroutine, the capture
	// loop, clipboard, chat, and file-export all write concurrently, and an
	// interleaved partial message corrupts the stream (a large file transfer
	// racing with input is what wedged input + capture in the field). One reader
	// per direction, so reads stay lock-free.
	ic := ipc.NewConn(conn)
	defer ic.Close() // stop the writer goroutine + unblock any enqueue on teardown
	log.Info().Int("port", port).Msg("capture worker connected to transport")

	// Stop streaming the instant this session leaves the physical console (a
	// fast-user-switch). Cancelling runCtx unwinds the capture loop + helpers; the
	// worker exits and launchd (KeepAlive) respawns it, which then blocks in
	// waitUntilOnConsole until this user is back on console. No-op off macOS.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				if !isOnConsole() {
					log.Info().Msg("worker: session left the console; yielding for respawn")
					runCancel()
					return
				}
			}
		}
	}()

	// A keyframe request from the transport (viewer PLI) sets this; the capture
	// loop clears it after forcing a keyframe.
	var wantKeyframe atomic.Bool
	wantKeyframe.Store(true) // first frame is always a keyframe

	// Injects viewer input into THIS session. On Windows this is real SendInput
	// (the worker runs as the logged-in user, so control follows the switch);
	// elsewhere it's a no-op.
	injector := newInputSink()
	defer injector.Close()
	// A microphone must never outlive the session that opened it. If the worker
	// dies for any reason — transport gone, session switch, crash on the way out
	// — the device is handed back and the OS mic indicator goes dark.
	defer closeHostAudio()
	// Close the recording file on the way out, whatever the reason. An
	// unfinished WebM still plays, but leaving the handle open risks the last
	// frames never reaching disk.
	defer stopRecording()

	// Text clipboard both ways (viewer↔host) so copy-paste keeps working in
	// TransportMode where the app no longer hosts. Runs as the logged-in user.
	clip := newClipSync(ic)
	go clip.poll(runCtx)

	// File transfer both ways: viewer→host lands in Downloads; host→viewer pops a
	// picker and streams back over the same conn.
	files := newFileReceiver(ic)
	defer files.closeAll()

	// File CLIPBOARD (Ctrl+C a file → Ctrl+V on the other machine), reusing the
	// clipf* protocol + the neev_helper clipagent. Polls the host clipboard for
	// file copies; handles viewer clipf* over the same file channel.
	cfiles := newClipFiles(ic)
	clipFilesStop := make(chan struct{})
	go cfiles.poll(clipFilesStop)
	defer close(clipFilesStop)

	// Host chat window: shows viewer messages; host replies stream back to viewers
	// over the transport. Started lazily on the first message either way.
	chatEnsure(func(reply string) {
		msg, err := json.Marshal(map[string]string{"k": "chat", "t": reply})
		if err == nil {
			_ = ic.WriteMessage(ipc.KindChat, msg)
		}
	})

	// KindFileData carries TWO unrelated workloads: explicit file transfers
	// ({k:ft}) and clipboard-file ops ({k:clipf*}). Give each its OWN lane so
	// neither can head-of-line-block the other — r69 funnelled both onto one
	// goroutine, so a synchronous clipboard serve stalled file-transfer acks and a
	// file transfer stalled clipboard pulls (the reported bug). Both lanes are also
	// off the reader goroutine, so neither ever delays input injection (r68).
	// Ordered within a lane (single drainer), so one transfer's offer/data/end and
	// one clipboard token's chunks stay in sequence.
	fileCh := make(chan []byte, 512)
	clipCh := make(chan []byte, 256)
	go func() {
		for payload := range fileCh {
			files.handle(payload)
		}
	}()
	go func() {
		for payload := range clipCh {
			cfiles.handle(payload)
		}
	}()

	// Reader: transport -> worker messages (keyframe requests, input, clipboard).
	// Ends when the transport goes away, which also unblocks the capture loop via
	// ctx.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(fileCh) // stop + drain the lane goroutines when the reader ends
		defer close(clipCh)
		for {
			kind, payload, err := ic.ReadMessage()
			if err != nil {
				return
			}
			switch kind {
			case ipc.KindKeyframeReq:
				wantKeyframe.Store(true)
			case ipc.KindInput:
				// The control channel multiplexes several message types. Apply
				// clipboard updates and session commands (lock/logoff/reboot) here
				// in the user session; anything else is real mouse/keyboard input.
				if clip.handleInbound(payload) {
					// consumed as a clipboard update
				} else if handleRecordCmd(payload, files) {
					// consumed as a viewer record start/stop
				} else if handleCommand(payload) {
					// consumed as a session command
				} else if handleChat(payload) {
					// consumed as a chat message
				} else {
					injector.Post(payload)
				}
			case ipc.KindFileData:
				// Route to the file-transfer lane or the clipboard lane by kind so
				// the two never block each other; either way it's off the reader
				// goroutine, so input never waits behind file/clipboard I/O.
				ch := clipCh
				if isFileTransferMsg(payload) {
					ch = fileCh
				}
				select {
				case ch <- payload:
				case <-runCtx.Done():
					return
				}
			case ipc.KindSessionState:
				// Also drives whether we capture at all. With nobody connected the
				// frames are encoded and then discarded by the transport, which
				// cost 90% of a CPU on an idle laptop for as long as the service
				// was running — i.e. all day, since it starts at login.
				if n, err := strconv.Atoi(strings.TrimSpace(string(payload))); err == nil {
					setViewerCount(n)
				}
				// Show the host's "Remote session active / Disconnect" bar while at
				// least one viewer is connected. The host had no way to end a session
				// it did not start; this is that control.
				if strings.TrimSpace(string(payload)) == "0" {
					hideHostSessionBar()
					// Privacy mode is SESSION state, not machine state. It used to survive
					// the session that switched it on: disconnecting while privacy was ON
					// left the host screen blanked and local input blocked, locking the
					// user out of their own machine until someone reconnected to turn it
					// off.
					clearPrivacy()
				} else {
					// Register where host audio goes for this session. On macOS
					// the system-sound frames arrive from the helper app over the
					// control socket, with no closure to carry the destination.
					setAudioSink(func(frame []byte) {
						_ = ic.WriteDroppable(ipc.KindAudioFrame, frame)
					})
					showHostSessionBarWithVoice(func() {
						// Empty payload = drop every viewer.
						_ = ic.WriteMessage(ipc.KindEndSession, nil)
						log.Info().Msg("worker: host ended the session from the session bar")
					}, func(kind string, on bool) {
						if kind == "record" {
							if on {
								w, h := capturerBounds()
								if path, ok := startRecording(w, h); ok {
									log.Info().Str("path", path).Msg("worker: host started recording")
								}
							} else if path := stopRecording(); path != "" {
								// Off the toggle goroutine: a long recording
								// takes real time to stream and must not stall
								// the session bar.
								go deliverRecording(files, path)
							}
							return
						}
						if kind == "sound" {
							if on {
								if !startHostSound(func(frame []byte) {
									_ = ic.WriteDroppable(ipc.KindAudioFrame, frame)
								}) {
									log.Info().Msg("worker: system sound sharing refused (unsupported)")
								}
							} else {
								stopHostSound()
							}
							return
						}
						// The host's OWN microphone control. It is driven from this
						// process and never from an IPC message, so there is no path
						// by which a viewer can open the host's microphone remotely.
						if on {
							startHostMic(func(frame []byte) {
								_ = ic.WriteDroppable(ipc.KindAudioFrame, frame)
							})
						} else {
							stopHostMic()
						}
						log.Info().Bool("on", on).Msg("worker: host microphone toggled from session bar")
					})
				}
			case ipc.KindAudioCapture:
				// Host microphone on/off. The device is opened here, in the
				// worker, because the worker runs in the interactive session and
				// therefore HAS an audio session; the transport is SYSTEM in
				// session 0 and has no audio endpoint at all.
				if strings.TrimSpace(string(payload)) == "1" {
					startHostMic(func(frame []byte) {
						// Droppable: voice is realtime, so if the pipe is congested
						// the right move is to lose a frame, not to queue audio that
						// will arrive too late to be worth hearing.
						_ = ic.WriteDroppable(ipc.KindAudioFrame, frame)
					})
				} else {
					stopHostMic()
				}
			case ipc.KindAudioPlay:
				// Viewer's voice → host speakers.
				playViewerVoice(payload)
			case ipc.KindHostCreds:
				// The app runs as this user and cannot read the transport's own
				// 0600 creds file, so the share card had no id to show. Land
				// them in this user's own directory, readable only by them.
				if err := writeUserCreds(payload); err != nil {
					log.Warn().Err(err).Msg("worker: could not publish host creds for the app")
				}
			case ipc.KindConsentCancel:
				// The viewer stopped asking (cancelled, disconnected, timed out).
				// Withdraw the prompt instead of leaving the host staring at a
				// question about a request that no longer exists.
				id := string(payload)
				cancelConsentPrompt(id)
				log.Info().Str("from", id).Msg("worker: consent prompt withdrawn — viewer is gone")
			case ipc.KindConsentRequest:
				// Ask the logged-in user to Accept/Deny this viewer. The modal
				// blocks, so run it off the reader goroutine and reply when answered.
				vid := string(payload)
				go func(id string) {
					// "Remember this decision" from an earlier prompt: apply it and
					// don't ask again. Both directions -- a remembered Decline
					// auto-declines just as a remembered Accept auto-accepts.
					if remembered, ok := rememberedConsent(id); ok {
						// A remembered decision also remembers the ACCESS LEVEL granted.
						reply, _ := json.Marshal(map[string]interface{}{
							"id": id, "allow": remembered.Allow, "control": remembered.Control})
						_ = ic.WriteMessage(ipc.KindConsentReply, reply)
						log.Info().Str("from", id).Bool("allow", remembered.Allow).
							Bool("control", remembered.Control).
							Msg("worker: consent auto-answered from a remembered decision")
						return
					}
					allow, control, remember := showConsentDialog(id)
					reply, _ := json.Marshal(map[string]interface{}{
						"id": id, "allow": allow, "control": control})
					_ = ic.WriteMessage(ipc.KindConsentReply, reply)
					if remember {
						saveConsentDecision(id, allow, control)
					}
					log.Info().Str("from", id).Bool("allow", allow).Bool("control", control).
						Bool("remember", remember).Msg("worker: consent answered")
				}(vid)
			}
		}
	}()

	// Retry rather than die.
	//
	// Returning here made main log.Fatal, launchd's KeepAlive restart the
	// worker, and the whole thing loop invisibly — `launchctl list` showed a
	// live PID with a non-zero last exit and nothing else explained why a
	// session stalled. On macOS the usual cause is Screen Recording not being
	// in effect, and that is a state which can be FIXED while the process runs,
	// so exiting is exactly wrong: it throws away the session and tells nobody.
	var capturer capture.Capturer
	for attempt := 1; ; attempt++ {
		c, err := capture.NewPlatformCapture(0)
		if err == nil {
			capturer = c
			clearCaptureBlocked()
			if attempt > 1 {
				log.Info().Int("attempt", attempt).Msg("worker: screen capture started")
			}
			break
		}
		// Tell the app, so it can show the user what to fix rather than leaving
		// them with a session that connects and then does nothing.
		markCaptureBlocked()

		// Granted, yet still failing: the PROCESS is stale, so replace it.
		//
		// macOS fixes a process's TCC answer at first use and keeps it for that
		// process's lifetime, so a grant added afterwards never reaches the
		// running worker. In the field this showed up as 312 consecutive
		// failures over 26 minutes with Screen Recording already allowed —
		// retrying could not have worked, and capture started only when the
		// worker was restarted BY HAND. That manual step is the difference
		// between the macOS host and the Windows one, and it is not something a
		// user should ever have to know about.
		//
		// launchd has KeepAlive on this job, so exiting brings back a fresh
		// worker that picks the grant up. Guarded to a granted-but-failing
		// state, and never before a couple of attempts, so a genuinely
		// ungranted machine keeps retrying and logging instead of respawning in
		// a loop.
		if attempt >= 3 && capture.ScreenCaptureGranted() {
			log.Warn().Int("attempt", attempt).
				Msg("worker: Screen Recording IS granted but this process was denied at " +
					"launch — exiting so launchd restarts it with the grant applied")
			os.Exit(0)
		}
		// Loud on the first failure and then occasionally: a permission problem
		// must be visible in the log without drowning it.
		if attempt == 1 || attempt%12 == 0 {
			log.Error().Err(err).Int("attempt", attempt).
				Msg("worker: CANNOT CAPTURE THE SCREEN — on macOS grant Screen Recording to " +
					"/Library/Application Support/NeevRemote/neev-agent (re-add it after an " +
					"update: the grant is tied to the binary, which the update replaced)")
		}
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	defer capturer.Close()

	w, h := capturer.Bounds()
	setCaptureSize(w, h)
	log.Info().Int("bounds_w", w).Int("bounds_h", h).
		Msg("worker: capture bounds (should equal the host's full physical screen)")
	enc, err := encode.NewEncoder(w, h, workerFPS, workerBitrate)
	if err != nil {
		return err
	}
	defer enc.Close()
	if err := ic.WriteMessage(ipc.KindVideoInfo, ipc.EncodeVideoInfo(w, h)); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second / workerFPS)
	defer ticker.Stop()
	framesSinceKey := 0
	for {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-readerDone:
			return nil // transport disconnected
		case <-ticker.C:
		}

		// Nobody is watching: skip the grab and the encode entirely.
		//
		// The tick still runs (cheap) so the loop stays responsive the instant a
		// viewer arrives, and the next frame is forced to be a keyframe so that
		// viewer gets a decodable picture immediately rather than waiting for
		// the ~2s keepalive.
		if !shouldCapture() {
			wantKeyframe.Store(true)
			framesSinceKey = 0
			continue
		}

		frame, err := capturer.CaptureFrame()
		if err != nil {
			if errors.Is(err, capture.ErrNoNewFrame) {
				// Nothing changed. Force a keepalive keyframe ~every 2s so a late
				// viewer can still decode a static screen.
				if framesSinceKey < workerFPS*2 {
					continue
				}
				wantKeyframe.Store(true)
				continue // no frame buffer to re-encode here; wait for next real frame
			}
			if errors.Is(err, capture.ErrAccessDenied) {
				// Desktop went away (lock / session switch). The service will
				// respawn us in the new session; exit cleanly.
				log.Info().Msg("worker: desktop access denied; exiting for respawn")
				return nil
			}
			continue
		}

		// Resolution change (e.g. DPI/monitor) → rebuild encoder + tell transport.
		if fw, fh := frame.Bounds().Dx(), frame.Bounds().Dy(); fw != enc.Width() || fh != enc.Height() {
			// If this captured frame is SMALLER than the reported bounds, the
			// source is cropped (edges lost) — log it loudly so a "screen cut off"
			// report is pinpointed to capture vs. render.
			log.Info().Int("frame_w", fw).Int("frame_h", fh).
				Int("prev_enc_w", enc.Width()).Int("prev_enc_h", enc.Height()).
				Msg("worker: captured frame size (encoder resized to match — this is what the viewer receives)")
			enc.Close()
			enc, err = encode.NewEncoder(fw, fh, workerFPS, enc.Bitrate())
			if err != nil {
				return err
			}
			_ = ic.WriteMessage(ipc.KindVideoInfo, ipc.EncodeVideoInfo(fw, fh))
			setCaptureSize(fw, fh)
			// A WebM track declares ONE size. Rather than keep appending frames
			// of a different resolution to a file that says otherwise — which
			// players render stretched or refuse — close the recording and start
			// a fresh one at the new size.
			if recordingActive() {
				old := stopRecording()
				if path, ok := startRecording(fw, fh); ok {
					log.Info().Str("previous", old).Str("now", path).
						Msg("worker: screen resolution changed — recording continued in a new file")
				}
			}
			wantKeyframe.Store(true)
		}

		forceKey := wantKeyframe.Swap(false)
		out, err := enc.Encode(frame, forceKey)
		if err != nil || out == nil || len(out.Data) == 0 {
			continue
		}
		if out.IsKeyframe {
			framesSinceKey = 0
		} else {
			framesSinceKey++
		}
		// Record from the frame we already encoded — a mux, not a second
		// encode, so recording does not compete with the live stream.
		writeRecordingFrame(out.Data, out.IsKeyframe)

		if err := ic.WriteDroppable(ipc.KindVideoFrame,
			ipc.EncodeVideoFrame(out.IsKeyframe, out.Data)); err != nil {
			log.Info().Err(err).Msg("worker: transport disconnected")
			return err
		}
	}
}

// waitUntilOnConsole blocks until this session owns the physical console (macOS
// fast-user-switch). No-op off macOS, where isOnConsole is always true.
func waitUntilOnConsole(ctx context.Context) error {
	if isOnConsole() {
		return nil
	}
	log.Info().Msg("worker: session not on console yet; waiting")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			if isOnConsole() {
				return nil
			}
		}
	}
}
