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
	"time"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/ipc"
)

// RunCaptureWorker connects to the transport and (eventually) streams captured,
// VP8-encoded frames. Runs until ctx is cancelled or the transport goes away.
// The service spawns one of these into the active session and replaces it on a
// user switch; the transport connection is unaffected.
func RunCaptureWorker(ctx context.Context, port int) error {
	if port == 0 {
		port = ipc.DefaultPort
	}
	conn, err := ipc.Dial(port)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Info().Int("port", port).Msg("capture worker connected to transport")

	// Reader: handle transport -> worker messages (keyframe requests, pings).
	go func() {
		for {
			kind, _, err := ipc.ReadMessage(conn)
			if err != nil {
				return
			}
			switch kind {
			case ipc.KindKeyframeReq:
				// TODO(M2.2): force a keyframe on the next captured frame.
				log.Debug().Msg("worker: keyframe requested")
			case ipc.KindPing:
			}
		}
	}()

	// TODO(M2.2): replace this heartbeat with the real capture+encode loop that
	// sends KindVideoInfo once and KindVideoFrame per encoded frame.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ipc.WriteMessage(conn, ipc.KindPing, nil); err != nil {
				log.Info().Err(err).Msg("worker: transport disconnected")
				return err
			}
		}
	}
}
