package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/network"
)

// SessionStats holds live performance metrics for the session.
type SessionStats struct {
	LatencyMs   int
	BitrateKbps int
	FPS         int
}

// controllerSession manages one active WebRTC connection to a remote agent.
// It handles signaling, WebRTC negotiation, and the control data channel.
type controllerSession struct {
	mu        sync.Mutex
	sigClient *network.Client
	peer      *network.Peer
	connected chan struct{}
	done      chan struct{}
	once      sync.Once

	// Stats
	frameCount  atomic.Int64
	lastFPSCalc time.Time
	fps         atomic.Int32
	latencyMs   atomic.Int32
}

// newControllerSession dials the signaling server, connects to agentID,
// and begins the WebRTC offer/answer exchange.
func newControllerSession(ctx context.Context, relayURL, agentID, password string) (*controllerSession, error) {
	s := &controllerSession{
		connected: make(chan struct{}),
		done:      make(chan struct{}),
	}

	// Connect to signaling server.
	sigClient := network.NewClient(relayURL, "", "", "1.0.0-controller", "", "", "")
	s.sigClient = sigClient

	// Start the signaling connection (runs until ctx cancelled or disconnect).
	connectCtx, connectCancel := context.WithCancel(ctx)
	go func() {
		if err := sigClient.Connect(connectCtx); err != nil {
			log.Debug().Err(err).Msg("controller signaling ended")
		}
	}()

	// Wait for the signaling to register us.
	timeout := time.After(10 * time.Second)
	for sigClient.AgentID == "" {
		select {
		case <-timeout:
			connectCancel()
			return nil, fmt.Errorf("signaling server connection timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}

	controllerID := sigClient.AgentID
	log.Info().Str("controller_id", controllerID).Str("target", agentID).Msg("sending connect request")

	iceServers, err := network.FetchICEServers(connectCtx, relayURL)
	if err != nil || len(iceServers) == 0 {
		log.Warn().Err(err).Msg("failed to fetch ICE servers from relay; falling back to STUN defaults")
		iceServers = []network.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		}
	}

	// Create WebRTC peer (controller role).
	peer, err := network.NewPeer(iceServers, network.RoleController, sigClient, agentID)
	if err != nil {
		connectCancel()
		return nil, fmt.Errorf("create peer: %w", err)
	}
	s.peer = peer

	// On WebRTC connected.
	peer.OnConnected = func() {
		log.Info().Msg("✅ WebRTC connected to agent")
		s.once.Do(func() { close(s.connected) })
		// Start FPS counter.
		go s.countFPS(ctx)
	}

	// Register answer/candidate handlers.
	sigClient.On(network.MsgAnswer, func(msg network.Message) {
		if msg.From != agentID {
			return
		}
		if err := peer.HandleAnswer(msg.Payload); err != nil {
			log.Error().Err(err).Msg("handle answer")
		}
	})
	sigClient.On(network.MsgCandidate, func(msg network.Message) {
		if msg.From != agentID {
			return
		}
		if err := peer.HandleCandidate(msg.Payload); err != nil {
			log.Warn().Err(err).Msg("add candidate")
		}
	})
	sigClient.On(network.MsgBye, func(msg network.Message) {
		log.Info().Msg("agent disconnected")
		s.closeOnce(connectCancel)
	})
	sigClient.On(network.MsgError, func(msg network.Message) {
		log.Error().Str("err", msg.Error).Msg("signaling error")
		s.closeOnce(connectCancel)
	})

	// Send connect request to signaling server → forwarded to agent.
	payload, _ := json.Marshal(map[string]string{
		"target_id":     agentID,
		"password_hash": password,
	})
	if err := sigClient.Send(network.Message{
		Type:    network.MsgConnect,
		Payload: payload,
	}); err != nil {
		connectCancel()
		return nil, fmt.Errorf("send connect: %w", err)
	}

	// Begin WebRTC offer (controller initiates).
	if err := peer.CreateOffer(ctx); err != nil {
		connectCancel()
		return nil, fmt.Errorf("create offer: %w", err)
	}

	return s, nil
}

// sendInput serializes and sends an input event over the control DataChannel.
func (s *controllerSession) sendInput(data []byte) error {
	return s.peer.SendControl(data)
}

// getStats returns a snapshot of live session metrics.
func (s *controllerSession) getStats() SessionStats {
	return SessionStats{
		LatencyMs:   int(s.latencyMs.Load()),
		BitrateKbps: 0, // TODO: read from WebRTC stats API
		FPS:         int(s.fps.Load()),
	}
}

// disconnect tears down the session.
func (s *controllerSession) disconnect() {
	s.closeOnce(func() {})
}

func (s *controllerSession) closeOnce(cancelFn func()) {
	s.once.Do(func() {
		// If connected was never closed (e.g. disconnect before connect):
		select {
		case <-s.connected:
		default:
			close(s.connected)
		}
	})
	select {
	case <-s.done:
	default:
		close(s.done)
		cancelFn()
		if s.peer != nil {
			_ = s.peer.Close()
		}
	}
}

// countFPS increments frameCount each time a video frame arrives and
// computes fps every second. The peer's OnTrack handler calls frameCount++.
func (s *controllerSession) countFPS(ctx context.Context) {
	// Wire frame counter via OnTrack.
	s.peer.OnTrack = func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// placeholder — in real impl read RTP packets and count
		_ = track
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	prev := s.frameCount.Load()
	for {
		select {
		case <-ticker.C:
			cur := s.frameCount.Load()
			s.fps.Store(int32(cur - prev))
			prev = cur
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
	}
}
