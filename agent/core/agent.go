package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/neev/remote-agent/agent/auth"
	"github.com/neev/remote-agent/agent/capture"
	"github.com/neev/remote-agent/agent/input"
	"github.com/neev/remote-agent/agent/network"
	"github.com/neev/remote-agent/agent/stream"
	"github.com/neev/remote-agent/agent/wol"
	log "github.com/rs/zerolog/log"
)

// envOr returns the value of env[key] or def if env[key] is empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseTurnURLs parses comma-separated TURN URLs.
func parseTurnURLs(turnURL string) []string {
	var servers []string
	for _, rawURL := range strings.Split(turnURL, ",") {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		u, err := url.Parse(rawURL)
		if err != nil {
			log.Warn().Str("url", rawURL).Err(err).Msg("invalid TURN URL, skipping")
			continue
		}
		switch u.Scheme {
		case "turn", "turns", "stun":
		default:
			if !strings.Contains(rawURL, "://") {
				servers = append(servers, "turn:"+rawURL)
				continue
			}
			log.Warn().Str("scheme", u.Scheme).Str("url", rawURL).Msg("unsupported TURN URL scheme, skipping")
			continue
		}
		servers = append(servers, rawURL)
	}
	return servers
}

const agentVersion = "1.0.0"

type AgentInstance struct {
	client          *network.Client
	Password        string
	cancel          context.CancelFunc
	sessionMu       sync.Mutex
	sessions        map[string]*activeSession
	macAddress      string
	registered      bool
	registeredCh    chan struct{}
}

type activeSession struct {
	peer   *network.Peer
	cancel context.CancelFunc
	done   chan struct{}
}

func (a *AgentInstance) GetID() string {
	if a.client != nil {
		return a.client.AgentID
	}
	return ""
}

// WaitForRegistered blocks until the agent has registered with the signaling
// server and has a valid AgentID. Returns immediately if already registered.
func (a *AgentInstance) WaitForRegistered() {
	<-a.registeredCh
}

func StartAgent(ctx context.Context, relayURL string, unattendedPassword string, onChat func(string)) (*AgentInstance, error) {
	return StartAgentWithContext(ctx, ctx, relayURL, unattendedPassword, onChat)
}

func StartAgentWithContext(startupCtx, parentCtx context.Context, relayURL string, unattendedPassword string, onChat func(string)) (*AgentInstance, error) {
	agentCtx, cancel := context.WithCancel(parentCtx)

	password, err := auth.GenerateRandomPassword()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to generate password: %w", err)
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to hash session password: %w", err)
	}
	var unattendedHash string
	if unattendedPassword != "" {
		unattendedHash, err = auth.HashPassword(unattendedPassword)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to hash unattended password: %w", err)
		}
	}

	log.Info().Str("platform", runtime.GOOS+"/"+runtime.GOARCH).Msg("Host Agent starting")

	wol.LogMACOnStartup()
	var macAddress string
	if mac, err := wol.GetPrimaryMAC(); err == nil {
		macAddress = mac.String()
	}

	injector, err := input.NewInjector()
	if err != nil {
		log.Warn().Err(err).Msg("input injection unavailable (view-only mode)")
	}

	iceServers, err := network.FetchICEServers(agentCtx, relayURL)
	if err != nil || len(iceServers) == 0 {
		log.Warn().Err(err).Msg("failed to fetch ICE servers from relay; falling back to local defaults")
		iceServers = []network.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		}
	}
	if turnURL := os.Getenv("TURN_URL"); turnURL != "" {
		for _, urlStr := range parseTurnURLs(turnURL) {
			iceServers = append(iceServers, network.ICEServer{
				URLs:       []string{urlStr},
				Username:   envOr("TURN_USER", "agent"),
				Credential: envOr("TURN_PASS", "changeme"),
			})
		}
	}

	sigClient := network.NewClient(
		relayURL,
		passwordHash,
		unattendedHash,
		agentVersion,
		envOr("ORG_ID", ""),
		envOr("DEVICE_GROUP", ""),
		envOr("ENROLLMENT_CODE", ""),
	)

	agent := &AgentInstance{
		client:       sigClient,
		Password:     password,
		cancel:       cancel,
		sessions:     make(map[string]*activeSession),
		macAddress:   macAddress,
		registeredCh: make(chan struct{}),
	}

	// Connect to signaling server in background — registration is async.
	// AgentID is assigned by the server and available after MsgRegistered arrives.
	go func() {
		if err := sigClient.Connect(agentCtx); err != nil && agentCtx.Err() == nil {
			log.Error().Err(err).Msg("signaling client failed")
		}
	}()

	// Log the agent ID once the server assigns it.
	sigClient.On(network.MsgRegistered, func(_ network.Message) {
		if !agent.registered {
			agent.registered = true
			log.Info().Str("id", sigClient.AgentID).Msg("agent registered")
			close(agent.registeredCh)
		}
	})

	sigClient.On(network.MsgOffer, func(m network.Message) {
		agent.sessionMu.Lock()
		s, ok := agent.sessions[m.From]
		agent.sessionMu.Unlock()
		if !ok {
			return
		}
		if err := s.peer.HandleOffer(m.Payload); err != nil {
			log.Error().Err(err).Msg("handle offer")
		}
	})
	sigClient.On(network.MsgCandidate, func(m network.Message) {
		agent.sessionMu.Lock()
		s, ok := agent.sessions[m.From]
		agent.sessionMu.Unlock()
		if !ok {
			return
		}
		if err := s.peer.HandleCandidate(m.Payload); err != nil {
			log.Warn().Err(err).Msg("add candidate")
		}
	})
	sigClient.On(network.MsgBye, func(m network.Message) {
		agent.sessionMu.Lock()
		s, ok := agent.sessions[m.From]
		if ok {
			delete(agent.sessions, m.From)
		}
		agent.sessionMu.Unlock()
		if !ok {
			return
		}
		log.Info().Str("controller", m.From).Msg("controller disconnected")
		if s.cancel != nil {
			s.cancel()
		}
		s.peer.Close()
	})

	sigClient.On(network.MsgConnect, func(msg network.Message) {
		log.Info().Str("from", msg.From).Msg("incoming connect request")

		agent.sessionMu.Lock()
		if old, ok := agent.sessions[msg.From]; ok {
			if old.cancel != nil {
				old.cancel()
			}
			old.peer.Close()
		}

		peer, err := network.NewPeer(iceServers, network.RoleAgent, sigClient, msg.From)
		if err != nil {
			agent.sessionMu.Unlock()
			log.Error().Err(err).Msg("create peer failed")
			return
		}
		agent.sessionMu.Unlock()

		log.Info().Msg("WebRTC peer created")

		// Build video pipeline.
		caps := capture.ListDisplays()
		var displayID uint32 = 0
		if len(caps) > 0 {
			displayID = caps[0].ID
		}

		var capturer capture.Capturer
		var capErr error
		if len(caps) > 0 {
			capturer, capErr = capture.NewPlatformCapture(displayID)
			if capErr != nil {
				log.Error().Err(capErr).Msg("create capturer failed")
				// Do not return here. Let WebRTC connect so we can send the error to the viewer.
			}
		}

		var pipeline *stream.Pipeline
		var pipelineErr error
		if capturer != nil {
			pipeline, pipelineErr = stream.NewPipeline(peer.VideoTrack, peer.PeerConnection(), 30, displayID)
			if pipelineErr != nil {
				log.Error().Err(pipelineErr).Msg("create pipeline failed")
			}
		}

		if pipeline != nil {
			pipeline.SendDirtyRects = func(data []byte) {
				peer.SendDirtyRects(data)
			}
			// On macOS, cursor is baked into frames via CGDisplayStreamShowCursor.
			// Only send cursor overlay info on Windows/Linux.
			if runtime.GOOS != "darwin" {
				pipeline.SendCursorInfo = func(ci capture.CursorInfo) {
					data, _ := json.Marshal(map[string]interface{}{
						"type":       "cursor_info",
						"x":          ci.X,
						"y":          ci.Y,
						"visible":    ci.Visible,
						"width":      ci.Width,
						"height":     ci.Height,
						"hotX":       ci.HotX,
						"hotY":       ci.HotY,
						"cursorType": ci.CursorType,
					})
					peer.SendControl(data)
				}
			}
			pipeline.OnQualityChange = func(qs stream.QualityState) {
				log.Debug().
					Float64("loss", qs.LossRate*100).
					Float64("rtt_ms", qs.RTTMs).
					Int("bw_kbps", qs.BitrateKbps).
					Int("fps", qs.FPS).
					Msg("quality")
				if data, err := json.Marshal(map[string]interface{}{
					"type": "quality",
					"rtt":  qs.RTTMs,
					"loss": qs.LossRate * 100,
					"bw":   qs.BitrateKbps,
					"fps":  qs.FPS,
				}); err == nil {
					peer.SendControl(data)
				}
			}
		}

		peer.OnConnected = func() {
			log.Info().Str("controller", msg.From).Msg("WebRTC P2P connected")
			// Start video pipeline AFTER connection is established
			if pipeline != nil {
				if err := pipeline.Start(agentCtx); err != nil {
					log.Error().Err(err).Msg("pipeline start failed")
				}
			}
			if agent.macAddress != "" {
				go func() {
					time.Sleep(600 * time.Millisecond)
					wolMsg := map[string]interface{}{
						"type": "wol_mac",
						"mac":  agent.macAddress,
					}
					if data, err := json.Marshal(wolMsg); err == nil {
						peer.SendControl(data)
						log.Info().Str("mac", agent.macAddress).Msg("WoL: sent MAC to controller")
					}
				}()
			}
			if capErr != nil || pipelineErr != nil {
				go func() {
					time.Sleep(500 * time.Millisecond)
					errMsg := ""
					if capErr != nil {
						errMsg += capErr.Error()
					}
					if pipelineErr != nil {
						errMsg += " | pipeline error: " + pipelineErr.Error()
					}
					errData := map[string]interface{}{
						"type": "agent_error",
						"message": errMsg,
					}
					if data, err := json.Marshal(errData); err == nil {
						peer.SendControl(data)
					}
				}()
			}
		}
		peer.OnDisconnected = func(reason string) {
			log.Warn().Str("reason", reason).Msg("peer disconnected")
			agent.sessionMu.Lock()
			delete(agent.sessions, msg.From)
			agent.sessionMu.Unlock()
		}
		peer.OnReconnected = func() {
			log.Info().Msg("peer reconnected")
		}
		peer.OnFallbackAttempt = func(phase network.ICEGatheringPhase) {
			log.Warn().Str("phase", phase.String()).Msg("ICE gathering failed, trying fallback")
		}

		// Route all data channel messages by label.
		peer.OnData = func(label string, data []byte, isString bool) {
			switch label {
			case "control":
				if len(data) > 0 && injector != nil {
					var ev input.Event
					if err := json.Unmarshal(data, &ev); err == nil {
						injector.InjectEvent(ev)
					}
				}
			case "clipboard":
				if len(data) > 0 {
					if err := clipboard.WriteAll(string(data)); err != nil {
						log.Warn().Err(err).Msg("set clipboard failed")
					}
				}
			case "wol":
				if agent.macAddress != "" {
					log.Info().Str("mac", agent.macAddress).Msg("WoL: magic packet requested")
					if err := wol.SendMagicPacketToMACString(agent.macAddress); err != nil {
						log.Error().Err(err).Msg("WoL: failed to send magic packet")
					}
				}
			}
		}

		agent.sessionMu.Lock()
		s := &activeSession{peer: peer, done: make(chan struct{})}
		agent.sessions[msg.From] = s
		agent.sessionMu.Unlock()

		_, sessionCancel := context.WithCancel(agentCtx)
		s.cancel = sessionCancel
	})

	// Clipboard sync — watch local clipboard and push to all controllers.
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		var lastClip string
		for {
			select {
			case <-agentCtx.Done():
				return
			case <-ticker.C:
				cur, err := clipboard.ReadAll()
				if err != nil || cur == lastClip {
					continue
				}
				lastClip = cur
				agent.sessionMu.Lock()
				for _, s := range agent.sessions {
					s.peer.SendClipboard([]byte(cur))
				}
				agent.sessionMu.Unlock()
			}
		}
	}()

	// HTTP API server for browser-based viewer.
	mux := http.NewServeMux()
	mux.HandleFunc("/clipboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			agent.sessionMu.Lock()
			for _, s := range agent.sessions {
				s.peer.SendClipboard(body)
			}
			agent.sessionMu.Unlock()
			return
		}
		text, _ := clipboard.ReadAll()
		w.Write([]byte(text))
	})
	mux.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
		agent.sessionMu.Lock()
		sessions := make([]map[string]string, 0, len(agent.sessions))
		for from, s := range agent.sessions {
			sessions = append(sessions, map[string]string{"id": from, "state": s.peer.PeerConnection().ConnectionState().String()})
		}
		agent.sessionMu.Unlock()
		json.NewEncoder(w).Encode(sessions)
	})
	agentAPI := http.Server{Handler: mux}
	ln, err := net.Listen("tcp", ":"+envOr("API_PORT", "7891"))
	if err == nil {
		go agentAPI.Serve(ln)
	}

	return agent, nil
}

func (a *AgentInstance) Stop() {
	a.cancel()
	a.sessionMu.Lock()
	for _, s := range a.sessions {
		if s.cancel != nil {
			s.cancel()
		}
		s.peer.Close()
	}
	a.sessions = make(map[string]*activeSession)
	a.sessionMu.Unlock()
}
func (a *AgentInstance) SetUnattendedPassword(password string) error {
	return nil
}

func (a *AgentInstance) SendChat(message string) error {
	return nil
}


