package network

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog/log"
)

// ICEServer matches the shape returned by /api/v1/session/ice-servers.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// PeerRole identifies whether this peer is the host (agent) or the controller.
type PeerRole string

const (
	RoleAgent      PeerRole = "agent"
	RoleController PeerRole = "controller"
)

// DataChannelLabel names for different data streams.
const (
	LabelControl      = "control"       // mouse/keyboard events
	LabelMeta         = "meta"          // session metadata
	LabelClipboard    = "clipboard"     // text clipboard sync
	LabelChat         = "chat"          // IT support chat
	LabelFileTransfer = "file_transfer" // bidirectional file chunks
)

// VideoTrackID is the standard track ID used for the desktop stream.
const VideoTrackID = "desktop-video"

// ConnectionMode indicates which ICE candidate type succeeded first.
// This is used by the UI to show connection quality.
type ConnectionMode string

const (
	// ConnectionModeDirect means host candidates were used (same LAN)
	ConnectionModeDirect ConnectionMode = "direct"
	// ConnectionModeSTUN means srflx candidates were used (simple NAT)
	ConnectionModeSTUN ConnectionMode = "stun"
	// ConnectionModeRelay means relay (TURN) candidates were used (symmetric NAT)
	ConnectionModeRelay ConnectionMode = "relay"
)

// ICE gathering phases for phased candidate gathering.
type ICEGatheringPhase int

const (
	ICEGatheringPhaseHost  ICEGatheringPhase = iota // 0-3s: host candidates only
	ICEGatheringPhaseSTUN                           // 3-8s: add srflx (STUN) candidates
	ICEGatheringPhaseRelay                          // 8s+: add relay (TURN) candidates
)

func (p ICEGatheringPhase) String() string {
	switch p {
	case ICEGatheringPhaseHost:
		return "host"
	case ICEGatheringPhaseSTUN:
		return "stun"
	case ICEGatheringPhaseRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// Peer wraps a pion WebRTC PeerConnection and manages ICE/SDP signaling.
type Peer struct {
	mu                 sync.Mutex
	pc                 *webrtc.PeerConnection
	role               PeerRole
	sigClient          *Client
	peerID             string
	connMode           ConnectionMode
	icePhase           ICEGatheringPhase
	iceGatheringDone   bool
	firstCandidateSet  bool
	fallbackICEservers []webrtc.ICEServer // TURN servers for fallback
	iceTimeoutTimer    *time.Timer
	VideoTrack         *webrtc.TrackLocalStaticRTP // nil for controller role
	ControlDC          *webrtc.DataChannel         // the control data channel
	ClipboardDC        *webrtc.DataChannel         // clipboard data channel
	ChatDC             *webrtc.DataChannel         // chat data channel
	FileTransferDC     *webrtc.DataChannel         // file transfer data channel
	OnTrack            func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)
	OnData             func(label string, data []byte, isString bool)
	OnConnected        func()
	OnDisconnected     func(reason string)           // called when connection drops unexpectedly
	OnReconnected      func()                        // called after successful reconnect
	OnFallbackAttempt  func(phase ICEGatheringPhase) // called when falling back to next ICE phase
	pendingCandidates  []webrtc.ICECandidateInit
}

// NewPeer creates a WebRTC PeerConnection configured with the given ICE servers.
func NewPeer(iceServers []ICEServer, role PeerRole, sigClient *Client, peerID string) (*Peer, error) {
	var pionServers []webrtc.ICEServer
	for _, s := range iceServers {
		log.Info().
			Strs("urls", s.URLs).
			Str("username", s.Username).
			Str("credential", "****").
			Msg("ICE server configured for PeerConnection")
		pionServers = append(pionServers, webrtc.ICEServer{
			URLs:           s.URLs,
			Username:       s.Username,
			Credential:     s.Credential,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	log.Info().Int("total_ice_servers", len(pionServers)).Msg("creating PeerConnection with ICE servers")

	cfg := webrtc.Configuration{
		ICEServers:         pionServers,
		ICETransportPolicy: webrtc.ICETransportPolicyAll, // gather all candidate types
	}
	pc, err := webrtc.NewPeerConnection(cfg)
	if err != nil {
		return nil, fmt.Errorf("new peer connection: %w", err)
	}

	p := &Peer{pc: pc, role: role, sigClient: sigClient, peerID: peerID}

	// For the agent role, add a local video track.
	// Use VP8 as the default codec since it's available on all platforms
	// (Windows: libvpx, macOS/Linux: libvpx via encode package).
	// H.264 is macOS-only in this build.
	if role == RoleAgent {
		// Prefer VP8 — available on all platforms via libvpx.
		// H.264 support is platform-dependent and requires FFmpeg on macOS.
		videoCodec := webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		}
		track, err := webrtc.NewTrackLocalStaticRTP(
			videoCodec,
			VideoTrackID,
			"remote-agent-stream",
		)
		if err != nil {
			pc.Close()
			return nil, fmt.Errorf("create video track: %w", err)
		}
		if _, err := pc.AddTrack(track); err != nil {
			pc.Close()
			return nil, fmt.Errorf("add video track: %w", err)
		}
		p.VideoTrack = track
		log.Info().Str("codec", videoCodec.MimeType).Msg("video track added to peer connection")
	}

	// ICE candidate handling — both forwards to peer and detects connection mode.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			// Gathering complete
			p.mu.Lock()
			p.iceGatheringDone = true
			p.mu.Unlock()
			log.Debug().Msg("ICE candidate gathering complete")
			return
		}

		// Detect and track connection mode based on first successful candidate pair.
		// The candidate type tells us what path will be used:
		// - host: direct LAN connection (lowest latency)
		// - srflx: STUN-based (simple NAT traversal)
		// - relay: TURN-based (symmetric NAT, highest latency but most reliable)
		p.mu.Lock()
		if !p.firstCandidateSet {
			p.firstCandidateSet = true
			switch c.Typ {
			case webrtc.ICECandidateTypeHost:
				p.connMode = ConnectionModeDirect
				log.Info().Str("type", c.Typ.String()).Str("address", c.Address).Msg("ICE: using direct (host) candidate - same network")
			case webrtc.ICECandidateTypeSrflx:
				p.connMode = ConnectionModeSTUN
				log.Info().Str("type", c.Typ.String()).Str("address", c.Address).Msg("ICE: using STUN candidate - simple NAT")
			case webrtc.ICECandidateTypeRelay:
				p.connMode = ConnectionModeRelay
				log.Info().Str("type", c.Typ.String()).Str("address", c.Address).Msg("ICE: using relay candidate - symmetric NAT/traversal required")
			default:
				log.Info().Str("type", c.Typ.String()).Str("address", c.Address).Msg("ICE: candidate gathered")
			}
		}
		p.mu.Unlock()

		// Log all candidates with their type for debugging.
		log.Info().
			Str("protocol", c.Protocol.String()).
			Str("address", c.Address).
			Int("port", int(c.Port)).
			Str("type", c.Typ.String()).
			Str("mode", string(p.GetConnectionMode())).
			Msg("ICE candidate gathered")

		// Forward to remote peer.
		payload, _ := json.Marshal(c.ToJSON())
		_ = sigClient.Send(Message{Type: MsgCandidate, To: peerID, Payload: payload})
	})

	// ICE connection state for debugging + fallback trigger.
	p.iceTimeoutTimer = time.AfterFunc(10*time.Second, func() {
		p.mu.Lock()
		if p.connMode == "" {
			// No successful connection yet — trigger fallback
			log.Warn().Str("phase", p.icePhase.String()).Msg("ICE timeout, attempting relay fallback")
			p.mu.Unlock()
			if p.OnFallbackAttempt != nil {
				p.OnFallbackAttempt(ICEGatheringPhaseRelay)
			}
		}
	})
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Info().Str("state", s.String()).Str("mode", string(p.GetConnectionMode())).Msg("ICE connection state changed")
		// Cancel the timeout on any state change after connecting
		switch s {
		case webrtc.ICEConnectionStateConnected, webrtc.ICEConnectionStateCompleted:
			if p.iceTimeoutTimer != nil {
				p.iceTimeoutTimer.Stop()
			}
		case webrtc.ICEConnectionStateFailed:
			log.Warn().Str("phase", p.icePhase.String()).Msg("ICE connection failed, attempting relay fallback")
			if p.fallbackICEservers != nil && p.OnFallbackAttempt != nil {
				p.OnFallbackAttempt(ICEGatheringPhaseRelay)
			}
		}
	})

	// Connection state.
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Info().Str("state", s.String()).Msg("WebRTC connection state changed")
		switch s {
		case webrtc.PeerConnectionStateConnected:
			log.Info().Msg("✅ WebRTC P2P connected")
			if p.OnConnected != nil {
				p.OnConnected()
			}
		case webrtc.PeerConnectionStateFailed:
			log.Error().Msg("❌ WebRTC connection FAILED - ICE/dtls failed to establish")
			if p.OnDisconnected != nil {
				p.OnDisconnected("ICE/DTLS failed")
			}
		case webrtc.PeerConnectionStateDisconnected:
			log.Warn().Msg("⚠️ WebRTC connection disconnected, attempting reconnect")
			go p.attemptReconnect(30 * time.Second)
		case webrtc.PeerConnectionStateClosed:
			log.Info().Msg("WebRTC connection closed")
		case webrtc.PeerConnectionStateConnecting:
			log.Info().Msg("🔄 WebRTC connecting...")
		}
	})

	// Remote track (controller receives video).
	if role == RoleController {
		pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			log.Info().Str("kind", track.Kind().String()).Msg("remote track received")
			if p.OnTrack != nil {
				p.OnTrack(track, receiver)
			}
		})
	}

	// DataChannel (agent receives control events).
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		p.mu.Lock()
		switch dc.Label() {
		case LabelControl:
			p.ControlDC = dc
		case LabelClipboard:
			p.ClipboardDC = dc
		case LabelChat:
			p.ChatDC = dc
		case LabelFileTransfer:
			p.FileTransferDC = dc
		}
		p.mu.Unlock()

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if p.OnData != nil {
				p.OnData(dc.Label(), msg.Data, msg.IsString)
			}
		})
		log.Info().Str("label", dc.Label()).Msg("data channel opened")
	})

	return p, nil
}

// CreateOffer creates an SDP offer (controller side).
func (p *Peer) CreateOffer(ctx context.Context) error {
	// Open control data channel.
	dc, err := p.pc.CreateDataChannel(LabelControl, nil)
	if err != nil {
		return fmt.Errorf("create data channel: %w", err)
	}
	p.mu.Lock()
	p.ControlDC = dc
	p.mu.Unlock()
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if p.OnData != nil {
			p.OnData(dc.Label(), msg.Data, msg.IsString)
		}
	})

	// Open clipboard data channel.
	clipDC, err := p.pc.CreateDataChannel(LabelClipboard, nil)
	if err == nil {
		p.mu.Lock()
		p.ClipboardDC = clipDC
		p.mu.Unlock()
		clipDC.OnMessage(func(msg webrtc.DataChannelMessage) {
			if p.OnData != nil {
				p.OnData(clipDC.Label(), msg.Data, msg.IsString)
			}
		})
	}

	// Open chat data channel.
	chatDC, err := p.pc.CreateDataChannel(LabelChat, nil)
	if err == nil {
		p.mu.Lock()
		p.ChatDC = chatDC
		p.mu.Unlock()
		chatDC.OnMessage(func(msg webrtc.DataChannelMessage) {
			if p.OnData != nil {
				p.OnData(chatDC.Label(), msg.Data, msg.IsString)
			}
		})
	}

	// Open file transfer data channel.
	fileDC, err := p.pc.CreateDataChannel(LabelFileTransfer, nil)
	if err == nil {
		p.mu.Lock()
		p.FileTransferDC = fileDC
		p.mu.Unlock()
		fileDC.OnMessage(func(msg webrtc.DataChannelMessage) {
			if p.OnData != nil {
				p.OnData(fileDC.Label(), msg.Data, msg.IsString)
			}
		})
	}

	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}

	// Wait for ICE gathering to complete before sending offer.
	gatherComplete := webrtc.GatheringCompletePromise(p.pc)
	if err := p.pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}
	<-gatherComplete

	// Send the fully gathered offer
	payload, err := json.Marshal(p.pc.LocalDescription())
	if err != nil {
		return fmt.Errorf("marshal offer: %w", err)
	}
	return p.sigClient.Send(Message{Type: MsgOffer, To: p.peerID, Payload: payload})
}

// CreateAgentOffer makes the AGENT the offerer, matching the Flutter host so an
// unchanged Flutter viewer (which always answers the host's offer) can connect
// to the SYSTEM-service transport. It opens the exact data channels the viewer
// binds — "control" (reliable, ordered), "cursor" (unreliable, unordered), and
// "file" (reliable, ordered) — and sends the offer immediately, letting ICE
// candidates trickle via OnICECandidate, exactly like the Flutter host. The
// video track was already added in NewPeer for RoleAgent.
func (p *Peer) CreateAgentOffer(ctx context.Context) error {
	// control: reliable, ordered (buttons, keys, wheel, commands, quality).
	ctrl, err := p.pc.CreateDataChannel("control", nil)
	if err != nil {
		return fmt.Errorf("create control channel: %w", err)
	}
	p.mu.Lock()
	p.ControlDC = ctrl
	p.mu.Unlock()
	ctrl.OnMessage(func(m webrtc.DataChannelMessage) {
		if p.OnData != nil {
			p.OnData("control", m.Data, m.IsString)
		}
	})

	// cursor: unreliable, unordered — high-rate mouse moves where only the
	// latest position matters (a lost/late move must not stall input).
	ordered := false
	var noRetransmit uint16 = 0
	if cur, err := p.pc.CreateDataChannel("cursor",
		&webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &noRetransmit}); err == nil {
		cur.OnMessage(func(m webrtc.DataChannelMessage) {
			if p.OnData != nil {
				p.OnData("cursor", m.Data, m.IsString)
			}
		})
	}

	// file: reliable, ordered — viewer→host import rides this; host→viewer export
	// (file-picker result) is sent back on it via SendFileTransferText.
	if fileDC, err := p.pc.CreateDataChannel("file", nil); err == nil {
		p.mu.Lock()
		p.FileTransferDC = fileDC
		p.mu.Unlock()
		fileDC.OnMessage(func(m webrtc.DataChannelMessage) {
			if p.OnData != nil {
				p.OnData("file", m.Data, m.IsString)
			}
		})
	}

	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}
	if err := p.pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}
	payload, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("marshal offer: %w", err)
	}
	return p.sigClient.Send(Message{Type: MsgOffer, To: p.peerID, Payload: payload})
}

// HandleOffer processes an incoming SDP offer (agent side).
func (p *Peer) HandleOffer(offerJSON json.RawMessage) error {
	var offer webrtc.SessionDescription
	if err := json.Unmarshal(offerJSON, &offer); err != nil {
		return err
	}
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	// Drain pending candidates
	p.mu.Lock()
	candidates := p.pendingCandidates
	p.pendingCandidates = nil
	p.mu.Unlock()
	for _, ci := range candidates {
		if err := p.pc.AddICECandidate(ci); err != nil {
			log.Warn().Err(err).Msg("add pending candidate")
		}
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}

	// Wait for ICE gathering to complete before sending answer.
	// This ensures all host candidates are included in the SDP,
	// preventing ICE connection failures when peer is on same LAN.
	gatherComplete := webrtc.GatheringCompletePromise(p.pc)
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}
	<-gatherComplete

	// Send the fully gathered answer
	payload, err := json.Marshal(p.pc.LocalDescription())
	if err != nil {
		return fmt.Errorf("marshal answer: %w", err)
	}
	return p.sigClient.Send(Message{Type: MsgAnswer, To: p.peerID, Payload: payload})
}

// HandleAnswer processes an incoming SDP answer (controller side).
func (p *Peer) HandleAnswer(answerJSON json.RawMessage) error {
	var answer webrtc.SessionDescription
	if err := json.Unmarshal(answerJSON, &answer); err != nil {
		return err
	}
	return p.pc.SetRemoteDescription(answer)
}

// HandleCandidate adds a remote ICE candidate.
func (p *Peer) HandleCandidate(candidateJSON json.RawMessage) error {
	var ci webrtc.ICECandidateInit
	if err := json.Unmarshal(candidateJSON, &ci); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pc.RemoteDescription() == nil {
		p.pendingCandidates = append(p.pendingCandidates, ci)
		log.Debug().Msg("queued early remote candidate")
		return nil
	}
	return p.pc.AddICECandidate(ci)
}

// SendDirtyRects sends dirty rectangle data over the control DataChannel.
// Dirty rects use the same channel as control messages since they're already JSON.
func (p *Peer) SendDirtyRects(data []byte) error {
	return p.SendControl(data)
}

// SendControl sends a raw bytes message on the control DataChannel.
func (p *Peer) SendControl(data []byte) error {
	p.mu.Lock()
	dc := p.ControlDC
	p.mu.Unlock()
	if dc == nil {
		return fmt.Errorf("control channel not open")
	}
	return dc.Send(data)
}

// SendControlText sends a TEXT message on the control DataChannel. The Flutter
// viewer only accepts non-binary messages there (clipboard/OS handshake ride the
// control channel as text), so host→viewer clipboard must go through this.
func (p *Peer) SendControlText(s string) error {
	p.mu.Lock()
	dc := p.ControlDC
	p.mu.Unlock()
	if dc == nil {
		return fmt.Errorf("control channel not open")
	}
	return dc.SendText(s)
}

// SendClipboard sends a text message on the clipboard DataChannel.
func (p *Peer) SendClipboard(data []byte) error {
	p.mu.Lock()
	dc := p.ClipboardDC
	p.mu.Unlock()
	if dc == nil {
		return fmt.Errorf("clipboard channel not open")
	}
	return dc.SendText(string(data))
}

// SendChat sends a text message on the chat DataChannel.
func (p *Peer) SendChat(data []byte) error {
	p.mu.Lock()
	dc := p.ChatDC
	p.mu.Unlock()
	if dc == nil {
		return fmt.Errorf("chat channel not open")
	}
	return dc.SendText(string(data))
}

// SendFileTransfer sends a raw bytes message on the file transfer DataChannel.
func (p *Peer) SendFileTransfer(data []byte) error {
	p.mu.Lock()
	dc := p.FileTransferDC
	p.mu.Unlock()
	if dc == nil {
		return fmt.Errorf("file transfer channel not open")
	}
	return dc.Send(data)
}

// SendFileTransferText sends a TEXT message on the file transfer DataChannel.
// The Flutter viewer only routes TEXT messages on 'file' into its file-transfer
// handler (binary is ignored), so host→viewer export uses this.
func (p *Peer) SendFileTransferText(s string) error {
	p.mu.Lock()
	dc := p.FileTransferDC
	p.mu.Unlock()
	if dc == nil {
		return fmt.Errorf("file transfer channel not open")
	}
	return dc.SendText(s)
}

// Close tears down the peer connection.
func (p *Peer) Close() error {
	return p.pc.Close()
}

// PeerConnection returns the underlying WebRTC PeerConnection for stats access.
func (p *Peer) PeerConnection() *webrtc.PeerConnection {
	return p.pc
}

// GetConnectionMode returns the ICE candidate type that was selected for the connection.
func (p *Peer) GetConnectionMode() ConnectionMode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connMode
}

// SetFallbackServers sets TURN relay servers for fallback when direct connection fails.
func (p *Peer) SetFallbackServers(servers []webrtc.ICEServer) {
	p.mu.Lock()
	p.fallbackICEservers = servers
	p.mu.Unlock()
}

// GetFallbackServers returns the configured TURN fallback servers.
func (p *Peer) GetFallbackServers() []webrtc.ICEServer {
	p.mu.Lock()
	servers := p.fallbackICEservers
	p.mu.Unlock()
	return servers
}

// attemptReconnect tries to reconnect within the given timeout.
// It attempts ICE restart and re-signaling.
func (p *Peer) attemptReconnect(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	attempts := 0
	for time.Now().Before(deadline) {
		<-ticker.C
		attempts++
		log.Info().Int("attempt", attempts).Msg("reconnect attempt")

		// Check if already connected
		if p.pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
			log.Info().Msg("reconnect succeeded")
			if p.OnReconnected != nil {
				p.OnReconnected()
			}
			return
		}

		// Try ICE restart
		if err := p.TryNextCandidatePair(); err != nil {
			log.Warn().Err(err).Int("attempt", attempts).Msg("reconnect ICE restart failed")
			continue
		}
		log.Info().Int("attempt", attempts).Msg("ICE restart signal sent, waiting for reconnect")
	}

	log.Error().Int("attempts", attempts).Msg("reconnect timeout - gave up")
	if p.OnDisconnected != nil {
		p.OnDisconnected(fmt.Sprintf("reconnect timeout after %ds", int(timeout.Seconds())))
	}
}

// Ping sends a keepalive ping over the control DataChannel.
// Returns an error if the control channel is not open.
func (p *Peer) Ping() error {
	p.mu.Lock()
	dc := p.ControlDC
	p.mu.Unlock()
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("control channel not open")
	}
	// Send a tiny keepalive (0-byte message signals liveliness)
	return dc.Send([]byte{})
}

// TryNextCandidatePair triggers ICE restart to use a different candidate pair.
// This forces re-gathering of candidates and can help when a better path is available.
func (p *Peer) TryNextCandidatePair() error {
	p.mu.Lock()
	p.icePhase = ICEGatheringPhaseRelay // force relay phase
	p.mu.Unlock()
	if p.fallbackICEservers != nil {
		// Restart ICE with TURN servers only
		p.icePhase = ICEGatheringPhaseRelay
		options := &webrtc.OfferOptions{ICERestart: true}
		offer, err := p.pc.CreateOffer(options)
		if err != nil {
			return fmt.Errorf("create ICE restart offer: %w", err)
		}
		if err := p.pc.SetLocalDescription(offer); err != nil {
			return fmt.Errorf("set local description (ICE restart): %w", err)
		}
	} else {
		p.icePhase = ICEGatheringPhaseRelay
	}
	return nil
}
