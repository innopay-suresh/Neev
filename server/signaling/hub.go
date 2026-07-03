package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/auth"
	serverauth "github.com/neev/remote-agent/server/auth"
	"github.com/neev/remote-agent/server/config"
	"github.com/neev/remote-agent/server/session"
)

// MessageType identifies the kind of signaling message.
type MessageType string

const (
	// Agent → Server
	MsgRegister  MessageType = "register"  // agent registers, gets its ID
	MsgHeartbeat MessageType = "heartbeat" // keep-alive ping

	// Controller → Server
	MsgConnect  MessageType = "connect"  // controller requests to connect to agentID
	MsgDiscover MessageType = "discover" // client asks who else is on its network

	// Server → both peers (SDP / ICE exchange)
	MsgOffer     MessageType = "offer"     // SDP offer (controller → agent via server)
	MsgAnswer    MessageType = "answer"    // SDP answer (agent → controller via server)
	MsgCandidate MessageType = "candidate" // ICE candidate (either direction)

	// Server → client (control messages)
	MsgRegistered MessageType = "registered"  // response to register, carries assigned ID
	MsgClientCert MessageType = "client_cert" // server pushes a new client cert bundle
	MsgPeers      MessageType = "peers"       // response to discover: same-network hosts
	MsgError      MessageType = "error"
	MsgBye        MessageType = "bye" // session ended
)

// Message is the universal signaling envelope.
type Message struct {
	Type      MessageType     `json:"type"`
	SessionID string          `json:"session_id,omitempty"` // unique pairing ID
	From      string          `json:"from,omitempty"`       // sender agent ID
	To        string          `json:"to,omitempty"`         // target agent ID
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// RegisterPayload is sent by an agent on first connect.
type RegisterPayload struct {
	AgentID  string `json:"agent_id,omitempty"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Version  string `json:"version"`
	// Password is the hashed access password; stored with session info.
	PasswordHash   string `json:"password_hash,omitempty"`
	UnattendedHash string `json:"unattended_hash,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
	DeviceGroup    string `json:"device_group,omitempty"`
	EnrollmentCode string `json:"enrollment_code,omitempty"`
}

// RegisteredPayload is sent back to the agent with its assigned ID.
type RegisteredPayload struct {
	AgentID    string                              `json:"agent_id"`
	ClientCert *serverauth.ClientCertificateBundle `json:"client_cert,omitempty"`
}

// ConnectPayload is sent by a controller to initiate a session.
type ConnectPayload struct {
	TargetID     string `json:"target_id"`
	PasswordHash string `json:"password_hash"`
}

// client wraps a WebSocket connection with its agent ID and role.
type client struct {
	conn      *websocket.Conn
	agentID   string
	sessionID string
	role      string // "agent" or "controller"
	certFP    string
	hostname  string // for same-network discovery
	os        string
	orgID     string
	mu        sync.Mutex
}

func (c *client) send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(msg)
}

// Hub is the central signaling hub — maintains all connected clients and
// routes messages between controller↔agent pairs.
type Hub struct {
	mu       sync.RWMutex
	agents   map[string]*client // agentID → client
	registry *session.Registry
	cfg      *config.Config
	clientCA *serverauth.ClientCA

	// Brute-force protection: TargetID -> failed attempts
	failCount map[string]int
	failMutex sync.Mutex
}

// NewHub creates a new signaling hub.
func NewHub(registry *session.Registry, cfg *config.Config, clientCA *serverauth.ClientCA) *Hub {
	return &Hub{
		agents:    make(map[string]*client),
		registry:  registry,
		cfg:       cfg,
		clientCA:  clientCA,
		failCount: make(map[string]int),
	}
}

// ManagedDeviceTrust reports whether agent certificates are issued by this hub.
func (h *Hub) ManagedDeviceTrust() bool {
	return h != nil && h.clientCA != nil
}

// ClientCAPEM returns the managed agent CA certificate if configured.
func (h *Hub) ClientCAPEM() string {
	if h == nil || h.clientCA == nil {
		return ""
	}
	return h.clientCA.CAPEM()
}

// ClientCAFingerprint returns the managed agent CA certificate fingerprint if configured.
func (h *Hub) ClientCAFingerprint() string {
	if h == nil || h.clientCA == nil {
		return ""
	}
	return h.clientCA.Fingerprint()
}

// ReissueClientCertificate rotates the active device certificate for an agent.
func (h *Hub) ReissueClientCertificate(ctx context.Context, agentID string) (*serverauth.ClientCertificateBundle, error) {
	if h == nil || h.clientCA == nil {
		return nil, fmt.Errorf("managed device trust is not configured")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	info, err := h.registry.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	bundle, err := h.clientCA.IssueCertificate(agentID, info.OrgID, info.DeviceGroup)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(info.ClientCertFingerprint) != "" && info.ClientCertFingerprint != bundle.Fingerprint {
		_ = h.registry.MarkClientCertRevokedFingerprint(ctx, info.ClientCertFingerprint)
	}
	if err := h.registry.SetClientCertFingerprint(ctx, agentID, bundle.Fingerprint); err != nil {
		return nil, err
	}
	_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
		Type:    "agent.cert.reissue",
		Actor:   authActorFromAgent(info),
		Target:  agentID,
		Outcome: "success",
		Details: map[string]any{
			"client_cert":  bundle.Fingerprint,
			"org_id":       info.OrgID,
			"device_group": info.DeviceGroup,
		},
	})
	h.pushClientCertBundle(agentID, bundle)
	return bundle, nil
}

// RevokeClientCertificate revokes the currently active device certificate for an agent.
func (h *Hub) RevokeClientCertificate(ctx context.Context, agentID string) (*session.AgentInfo, error) {
	if h == nil || h.clientCA == nil {
		return nil, fmt.Errorf("managed device trust is not configured")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent id is required")
	}
	info, err := h.registry.RevokeClientCert(ctx, agentID)
	if err != nil {
		return nil, err
	}
	_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
		Type:    "agent.cert.revoke",
		Actor:   authActorFromAgent(info),
		Target:  agentID,
		Outcome: "success",
		Details: map[string]any{
			"client_cert":  info.ClientCertFingerprint,
			"org_id":       info.OrgID,
			"device_group": info.DeviceGroup,
		},
	})
	h.disconnectAgent(agentID, "certificate revoked")
	return info, nil
}

func (h *Hub) pushClientCertBundle(agentID string, bundle *serverauth.ClientCertificateBundle) {
	if h == nil || bundle == nil {
		return
	}
	h.mu.RLock()
	cli, ok := h.agents[agentID]
	h.mu.RUnlock()
	if !ok || cli == nil {
		return
	}
	payload, _ := json.Marshal(RegisteredPayload{AgentID: agentID, ClientCert: bundle})
	_ = cli.send(Message{
		Type:    MsgClientCert,
		Payload: payload,
	})
}

func (h *Hub) disconnectAgent(agentID, reason string) {
	if h == nil {
		return
	}
	h.mu.RLock()
	cli, ok := h.agents[agentID]
	h.mu.RUnlock()
	if !ok || cli == nil || cli.conn == nil {
		return
	}
	_ = cli.send(Message{Type: MsgBye, Error: reason})
	_ = cli.conn.Close()
}

func authActorFromAgent(info *session.AgentInfo) string {
	if info == nil {
		return "system"
	}
	if strings.TrimSpace(info.Hostname) != "" {
		return info.Hostname
	}
	return info.ID
}

// HandleWS is the Fiber WebSocket handler — one goroutine per connection.
func (h *Hub) HandleWS(c *websocket.Conn) {
	ctx := context.Background()
	cli := &client{conn: c}
	if role, ok := c.Locals("client_role").(string); ok {
		cli.role = role
	}
	if certFP, ok := c.Locals("client_cert_fingerprint").(string); ok {
		cli.certFP = certFP
	}
	defer h.disconnect(ctx, cli)

	for {
		var msg Message
		if err := c.ReadJSON(&msg); err != nil {
			log.Debug().Err(err).Msg("ws read error")
			return
		}

		switch msg.Type {
		case MsgRegister:
			h.handleRegister(ctx, cli, msg)
		case MsgHeartbeat:
			h.handleHeartbeat(ctx, cli)
		case MsgConnect:
			h.handleConnect(ctx, cli, msg)
		case MsgDiscover:
			h.handleDiscover(cli)
		case MsgOffer, MsgAnswer, MsgCandidate:
			h.handleRelay(ctx, cli, msg)
		case MsgBye:
			return
		}
	}
}

func (h *Hub) handleRegister(ctx context.Context, cli *client, msg Message) {
	var payload RegisterPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		_ = cli.send(errMsg("invalid register payload"))
		return
	}

	isAgentRegistration := payload.PasswordHash != "" || payload.UnattendedHash != ""
	if cli.role == "agent" && !isAgentRegistration {
		_ = cli.send(errMsg("agent certificate connection requires agent registration payload"))
		return
	}
	isUnattended := payload.UnattendedHash != ""
	if isUnattended && h.cfg != nil && h.cfg.Network.EnrollmentCode != "" && payload.EnrollmentCode != h.cfg.Network.EnrollmentCode {
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "agent.register",
			Actor:   cli.conn.RemoteAddr().String(),
			Outcome: "denied",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{
				"reason":       "invalid_enrollment",
				"hostname":     payload.Hostname,
				"os":           payload.OS,
				"org_id":       payload.OrgID,
				"device_group": payload.DeviceGroup,
			},
		})
		_ = cli.send(errMsg("enrollment required"))
		return
	}

	// Determine the agent ID.
	var agentID string
	if cli.agentID != "" {
		agentID = cli.agentID
	} else if id := strings.TrimSpace(payload.AgentID); id != "" {
		// Honour a client-supplied persistent ID so an install keeps the same
		// ID across restarts/reconnects (new or already known). A later
		// h.agents[id] = cli simply replaces any stale prior connection; that
		// connection's own disconnect is guarded against removing the newer
		// entry, so re-registration after a dropped session "just works"
		// without the user having to refresh their ID.
		agentID = id
	} else {
		for i := 0; i < 5; i++ {
			id, err := session.GenerateID()
			if err != nil {
				continue
			}
			if _, err := h.registry.Get(ctx, id); err != nil {
				agentID = id
				break
			}
		}
	}
	if agentID == "" {
		_ = cli.send(errMsg("failed to generate unique ID"))
		return
	}

	var orgID string
	var deviceGroup string
	var info *session.AgentInfo
	var certBundle *serverauth.ClientCertificateBundle

	if isAgentRegistration {
		orgID = payload.OrgID
		if orgID == "" && h.cfg != nil {
			orgID = h.cfg.Network.DefaultOrgID
		}
		deviceGroup = payload.DeviceGroup
		if deviceGroup == "" && h.cfg != nil {
			deviceGroup = h.cfg.Network.DefaultDeviceGroup
		}

		// Register agent in Redis.
		info = &session.AgentInfo{
			ID:                    agentID,
			Hostname:              payload.Hostname,
			OS:                    payload.OS,
			Version:               payload.Version,
			PasswordHash:          payload.PasswordHash,
			UnattendedHash:        payload.UnattendedHash,
			OrgID:                 orgID,
			DeviceGroup:           deviceGroup,
			ClientCertFingerprint: cli.certFP,
			Status:                session.StatusWaiting,
			PublicAddr:            cli.conn.RemoteAddr().String(),
		}
		if cli.role == "agent" && h.clientCA != nil {
			if cli.certFP != "" {
				revoked, err := h.registry.IsClientCertRevoked(ctx, cli.certFP)
				if err == nil && revoked {
					_ = cli.send(errMsg("client certificate revoked"))
					return
				}
			}
			existing, err := h.registry.Get(ctx, agentID)
			if err == nil && existing.ClientCertFingerprint != "" && cli.certFP != "" && existing.ClientCertFingerprint != cli.certFP {
				_ = cli.send(errMsg("client certificate mismatch"))
				return
			}
			if err == nil && existing.ClientCertRevoked {
				_ = cli.send(errMsg("client certificate revoked"))
				return
			}
			if cli.certFP == "" {
				bundle, err := h.clientCA.IssueCertificate(agentID, orgID, deviceGroup)
				if err != nil {
					log.Error().Err(err).Msg("issue client certificate")
				} else {
					certBundle = bundle
					info.ClientCertFingerprint = bundle.Fingerprint
				}
			} else {
				info.ClientCertFingerprint = cli.certFP
			}
		}
		if err := h.registry.Register(ctx, info); err != nil {
			log.Error().Err(err).Msg("registry register")
			_ = cli.send(errMsg("registration failed"))
			return
		}
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "agent.register",
			Actor:   agentID,
			Outcome: "success",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{
				"hostname":     payload.Hostname,
				"os":           payload.OS,
				"version":      payload.Version,
				"org_id":       orgID,
				"device_group": deviceGroup,
				"client_cert":  cli.certFP,
			},
		})
	}

	cli.agentID = agentID
	cli.hostname = payload.Hostname
	cli.os = payload.OS
	cli.orgID = orgID
	if isAgentRegistration {
		cli.role = "agent"
	} else {
		cli.role = "controller"
	}

	h.mu.Lock()
	h.agents[agentID] = cli
	h.mu.Unlock()

	payload2, _ := json.Marshal(RegisteredPayload{AgentID: agentID, ClientCert: certBundle})
	_ = cli.send(Message{
		Type:    MsgRegistered,
		Payload: payload2,
	})
	log.Info().Str("id", agentID).Str("host", payload.Hostname).Msg("agent registered")
}

func (h *Hub) handleHeartbeat(ctx context.Context, cli *client) {
	if cli.agentID != "" && cli.role == "agent" {
		_ = h.registry.Heartbeat(ctx, cli.agentID)
	}
}

func (h *Hub) handleConnect(ctx context.Context, cli *client, msg Message) {
	var payload ConnectPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		_ = cli.send(errMsg("invalid connect payload"))
		return
	}

	h.failMutex.Lock()
	fails := h.failCount[payload.TargetID]
	h.failMutex.Unlock()

	if fails >= 5 {
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "session.connect",
			Actor:   cli.agentID,
			Target:  payload.TargetID,
			Outcome: "rate_limited",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{"reason": "too_many_failed_attempts"},
		})
		_ = cli.send(errMsg("too many failed attempts. Try again later."))
		// Exponential backoff or simple lockout could be added here
		time.Sleep(2 * time.Second)
		return
	}

	// Fetch agent info from registry to verify password
	info, err := h.registry.Get(ctx, payload.TargetID)
	if err != nil {
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "session.connect",
			Actor:   cli.agentID,
			Target:  payload.TargetID,
			Outcome: "denied",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{"reason": "agent_not_found"},
		})
		_ = cli.send(errMsg("agent not found or offline"))
		return
	}

	// Verify Argon2id password hash against either the session hash or the unattended hash
	ok, err := auth.VerifyPassword(payload.PasswordHash, info.PasswordHash)
	if (err != nil || !ok) && info.UnattendedHash != "" {
		ok, err = auth.VerifyPassword(payload.PasswordHash, info.UnattendedHash)
	}
	if err != nil || !ok {
		h.failMutex.Lock()
		h.failCount[payload.TargetID]++
		h.failMutex.Unlock()
		log.Warn().Str("target", payload.TargetID).Str("ip", cli.conn.RemoteAddr().String()).Msg("invalid password attempt")
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "session.connect",
			Actor:   cli.agentID,
			Target:  payload.TargetID,
			Outcome: "denied",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{"reason": "invalid_password"},
		})
		_ = cli.send(errMsg("invalid password"))
		return
	}

	// Password OK, reset fail count
	h.failMutex.Lock()
	delete(h.failCount, payload.TargetID)
	h.failMutex.Unlock()

	h.mu.RLock()
	target, targetOk := h.agents[payload.TargetID]
	h.mu.RUnlock()

	if !targetOk {
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "session.connect",
			Actor:   cli.agentID,
			Target:  payload.TargetID,
			Outcome: "denied",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{"reason": "agent_disconnected"},
		})
		_ = cli.send(errMsg("agent disconnected"))
		return
	}

	// Assign a controller ID so we can route return messages.
	controllerID := "ctrl-" + payload.TargetID

	sessionInfo := &session.SessionInfo{
		AgentID:      payload.TargetID,
		ControllerID: controllerID,
		TargetID:     payload.TargetID,
		OrgID:        info.OrgID,
		DeviceGroup:  info.DeviceGroup,
		Status:       session.SessionStatusConnecting,
		ControllerIP: cli.conn.RemoteAddr().String(),
		AgentIP:      target.conn.RemoteAddr().String(),
	}
	createdSession, err := h.registry.StartSession(ctx, sessionInfo)
	if err != nil {
		log.Error().Err(err).Str("target", payload.TargetID).Msg("failed to create session record")
		_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
			Type:    "session.connect",
			Actor:   controllerID,
			Target:  payload.TargetID,
			Outcome: "error",
			IP:      cli.conn.RemoteAddr().String(),
			Details: map[string]any{"reason": "session_creation_failed"},
		})
		_ = cli.send(errMsg("session creation failed"))
		return
	}
	cli.agentID = controllerID
	cli.role = "controller"
	cli.sessionID = createdSession.ID
	target.sessionID = createdSession.ID

	h.mu.Lock()
	h.agents[controllerID] = cli
	h.mu.Unlock()

	// Forward connect request to the target agent — it will show a consent prompt or just accept.
	connectFwd, _ := json.Marshal(payload)
	_ = target.send(Message{
		Type:    MsgConnect,
		From:    controllerID,
		Payload: connectFwd,
	})

	// Confirm to the controller that the connection is accepted.
	_ = cli.send(Message{
		Type: MsgConnect,
	})

	_ = h.registry.SetSessionStatus(ctx, createdSession.ID, session.SessionStatusActive)
	_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
		Type:      "session.connect",
		Actor:     controllerID,
		Target:    payload.TargetID,
		SessionID: createdSession.ID,
		Outcome:   "accepted",
		IP:        cli.conn.RemoteAddr().String(),
		Details: map[string]any{
			"controller_ip": cli.conn.RemoteAddr().String(),
			"agent_ip":      target.conn.RemoteAddr().String(),
		},
	})

	log.Info().Str("controller", controllerID).Str("target", payload.TargetID).Str("session", createdSession.ID).Msg("connect request forwarded")
}

// DiscoverPeer is one machine on the same network (shared public IP).
type DiscoverPeer struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
}

// handleDiscover replies with the other registered hosts that share this
// client's public IP (and org) — LAN-mate discovery that works even when the
// network blocks UDP broadcast. No presence data leaves the requester's own
// public-IP group.
func (h *Hub) handleDiscover(cli *client) {
	myIP := hostOnly(cli.conn.RemoteAddr().String())
	if myIP == "" {
		return
	}
	peers := make([]DiscoverPeer, 0, 4)
	h.mu.RLock()
	for id, other := range h.agents {
		if other == cli || other.conn == nil {
			continue
		}
		// Only registered hosts (agents) are discoverable, never controllers.
		if other.role != "agent" || strings.HasPrefix(id, "ctrl-") {
			continue
		}
		if hostOnly(other.conn.RemoteAddr().String()) != myIP {
			continue
		}
		if cli.orgID != other.orgID {
			continue
		}
		peers = append(peers, DiscoverPeer{ID: id, Hostname: other.hostname, OS: other.os})
	}
	h.mu.RUnlock()
	payload, _ := json.Marshal(map[string]any{"peers": peers})
	_ = cli.send(Message{Type: MsgPeers, Payload: payload})
}

// hostOnly strips the port from an "ip:port" (v4 or v6) address.
func hostOnly(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func (h *Hub) handleRelay(ctx context.Context, cli *client, msg Message) {
	if msg.To == "" {
		return
	}
	h.mu.RLock()
	dest, ok := h.agents[msg.To]
	h.mu.RUnlock()
	if !ok {
		_ = cli.send(errMsg("destination not connected"))
		return
	}
	msg.From = cli.agentID
	_ = dest.send(msg)
}

func (h *Hub) disconnect(ctx context.Context, cli *client) {
	if cli.agentID == "" {
		return
	}
	shouldClear := false
	h.mu.Lock()
	if current, ok := h.agents[cli.agentID]; ok && current == cli {
		delete(h.agents, cli.agentID)
		shouldClear = true
	}
	h.mu.Unlock()
	if shouldClear && cli.role == "agent" {
		_ = h.registry.SetStatus(ctx, cli.agentID, session.StatusOffline)
	}
	if cli.sessionID != "" {
		_ = h.registry.EndSession(ctx, cli.sessionID)
	}
	_ = h.registry.AddAuditEvent(ctx, &session.AuditEvent{
		Type:      "client.disconnect",
		Actor:     cli.agentID,
		SessionID: cli.sessionID,
		Outcome:   "ended",
		IP:        cli.conn.RemoteAddr().String(),
		Details: map[string]any{
			"role": cli.role,
		},
	})
	log.Info().Str("id", cli.agentID).Str("role", cli.role).Msg("client disconnected")

	// Notify paired peer.
	peerID := peerOf(cli.agentID)
	h.mu.RLock()
	peer, ok := h.agents[peerID]
	h.mu.RUnlock()
	if ok {
		_ = peer.send(Message{Type: MsgBye, From: cli.agentID})
	}
}

func peerOf(id string) string {
	// Convention: controller ID is "ctrl-<agentID>"; agent ID is the raw 9-digit ID.
	if len(id) > 5 && id[:5] == "ctrl-" {
		return id[5:]
	}
	return "ctrl-" + id
}

func errMsg(text string) Message {
	return Message{Type: MsgError, Error: text}
}

// Ping sends periodic heartbeats to detect stale connections.
func (h *Hub) RunPinger(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		h.mu.RLock()
		for _, cli := range h.agents {
			go func(c *client) {
				_ = c.send(Message{Type: MsgHeartbeat})
			}(cli)
		}
		h.mu.RUnlock()
	}
}
