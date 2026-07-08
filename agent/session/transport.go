package session

import (
	"context"
	"net"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/ipc"
)

// RunTransport runs the persistent transport process. It listens for the
// per-session capture worker on loopback and (eventually) owns the WebRTC peer
// connection + signaling, forwarding worker frames to the viewer and keyframe
// requests back to the worker. It stays alive across worker restarts (i.e.
// across user switches), which is the whole point of the split.
//
// Phase 0, milestone 2: accept the worker and drain its stream so the channel +
// worker-swap lifecycle can be validated. WebRTC wiring lands in milestone 3.
func RunTransport(ctx context.Context, port int) error {
	if port == 0 {
		port = ipc.DefaultPort
	}
	ln, err := ipc.Listen(port)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Info().Int("port", port).Msg("transport listening for capture worker")

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				log.Warn().Err(err).Msg("transport: accept failed")
				continue
			}
		}
		// One worker at a time (the active session's). A new worker (after a
		// switch) simply connects and replaces the old stream; the WebRTC peer
		// this transport owns is never torn down.
		go handleWorker(ctx, conn)
	}
}

func handleWorker(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	log.Info().Str("remote", conn.RemoteAddr().String()).Msg("capture worker attached")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		kind, payload, err := ipc.ReadMessage(conn)
		if err != nil {
			log.Info().Err(err).Msg("capture worker detached")
			return
		}
		switch kind {
		case ipc.KindVideoInfo:
			if w, h, ok := ipc.DecodeVideoInfo(payload); ok {
				log.Info().Int("w", w).Int("h", h).Msg("transport: video info")
			}
		case ipc.KindVideoFrame:
			// TODO(M3): write the VP8 sample to the WebRTC track.
			if kf, vp8, ok := ipc.DecodeVideoFrame(payload); ok {
				log.Debug().Bool("keyframe", kf).Int("bytes", len(vp8)).
					Msg("transport: frame")
			}
		case ipc.KindPing:
		}
	}
}
