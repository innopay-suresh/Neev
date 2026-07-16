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

// RunCaptureWorker connects to the transport and (eventually) streams captured,
// VP8-encoded frames. Runs until ctx is cancelled or the transport goes away.
// The service spawns one of these into the active session and replaces it on a
// user switch; the transport connection is unaffected.
func RunCaptureWorker(ctx context.Context, port int) error {
	setupFileLog("worker.log")
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

	// Text clipboard both ways (viewer↔host) so copy-paste keeps working in
	// TransportMode where the app no longer hosts. Runs as the logged-in user.
	clip := newClipSync(conn)
	go clip.poll(runCtx)

	// File transfer both ways: viewer→host lands in Downloads; host→viewer pops a
	// picker and streams back over the same conn.
	files := newFileReceiver(conn)
	defer files.closeAll()

	// File CLIPBOARD (Ctrl+C a file → Ctrl+V on the other machine), reusing the
	// clipf* protocol + the neev_helper clipagent. Polls the host clipboard for
	// file copies; handles viewer clipf* over the same file channel.
	cfiles := newClipFiles(conn)
	clipFilesStop := make(chan struct{})
	go cfiles.poll(clipFilesStop)
	defer close(clipFilesStop)

	// Host chat window: shows viewer messages; host replies stream back to viewers
	// over the transport. Started lazily on the first message either way.
	chatEnsure(func(reply string) {
		msg, err := json.Marshal(map[string]string{"k": "chat", "t": reply})
		if err == nil {
			_ = ipc.WriteMessage(conn, ipc.KindChat, msg)
		}
	})

	// Reader: transport -> worker messages (keyframe requests, input, clipboard).
	// Ends when the transport goes away, which also unblocks the capture loop via
	// ctx.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			kind, payload, err := ipc.ReadMessage(conn)
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
				} else if handleCommand(payload) {
					// consumed as a session command
				} else if handleChat(payload) {
					// consumed as a chat message
				} else {
					injector.Post(payload)
				}
			case ipc.KindFileData:
				// The file channel carries both explicit transfers ({k:ft}) and
				// file-clipboard control ({k:clipf*}). Try the transfer receiver
				// first; anything else is a clipboard-file message.
				if !files.handle(payload) {
					cfiles.handle(payload)
				}
			}
		}
	}()

	capturer, err := capture.NewPlatformCapture(0)
	if err != nil {
		return err
	}
	defer capturer.Close()

	w, h := capturer.Bounds()
	log.Info().Int("bounds_w", w).Int("bounds_h", h).
		Msg("worker: capture bounds (should equal the host's full physical screen)")
	enc, err := encode.NewEncoder(w, h, workerFPS, workerBitrate)
	if err != nil {
		return err
	}
	defer enc.Close()
	if err := ipc.WriteMessage(conn, ipc.KindVideoInfo, ipc.EncodeVideoInfo(w, h)); err != nil {
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
			_ = ipc.WriteMessage(conn, ipc.KindVideoInfo, ipc.EncodeVideoInfo(fw, fh))
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
		if err := ipc.WriteMessage(conn, ipc.KindVideoFrame,
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
