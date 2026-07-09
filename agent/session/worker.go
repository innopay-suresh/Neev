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
	if port == 0 {
		port = ipc.DefaultPort
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

	// A keyframe request from the transport (viewer PLI) sets this; the capture
	// loop clears it after forcing a keyframe.
	var wantKeyframe atomic.Bool
	wantKeyframe.Store(true) // first frame is always a keyframe

	// Injects viewer input into THIS session. On Windows this is real SendInput
	// (the worker runs as the logged-in user, so control follows the switch);
	// elsewhere it's a no-op.
	injector := newInputSink()
	defer injector.Close()

	// Reader: transport -> worker messages (keyframe requests, input). Ends when
	// the transport goes away, which also unblocks the capture loop via ctx.
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
				injector.Post(payload)
			}
		}
	}()

	capturer, err := capture.NewPlatformCapture(0)
	if err != nil {
		return err
	}
	defer capturer.Close()

	w, h := capturer.Bounds()
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
		case <-ctx.Done():
			return ctx.Err()
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
