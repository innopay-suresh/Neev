package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// MessageType mirrors the server signaling types.
type MessageType string

const (
	MsgRegister   MessageType = "register"
	MsgRegistered MessageType = "registered"
	MsgClientCert MessageType = "client_cert"
	MsgHeartbeat  MessageType = "heartbeat"
	MsgConnect    MessageType = "connect"
	MsgOffer      MessageType = "offer"
	MsgAnswer     MessageType = "answer"
	MsgCandidate  MessageType = "candidate"
	MsgError      MessageType = "error"
	MsgBye        MessageType = "bye"
)

type Message struct {
	Type      MessageType     `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	From      string          `json:"from,omitempty"`
	To        string          `json:"to,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// RegisterPayload is sent on first connection.
type RegisterPayload struct {
	AgentID        string `json:"agent_id,omitempty"`
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	Version        string `json:"version"`
	PasswordHash   string `json:"password_hash,omitempty"`
	UnattendedHash string `json:"unattended_hash,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
	DeviceGroup    string `json:"device_group,omitempty"`
	EnrollmentCode string `json:"enrollment_code,omitempty"`
}

// RegisteredPayload is received with the assigned ID.
type RegisteredPayload struct {
	AgentID    string            `json:"agent_id"`
	ClientCert *ClientCertBundle `json:"client_cert,omitempty"`
}

// ClientCertBundle carries the mTLS certificate material for an agent.
type ClientCertBundle struct {
	CertPEM     string `json:"cert_pem"`
	KeyPEM      string `json:"key_pem"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// MessageHandler is called for each incoming signaling message.
type MessageHandler func(msg Message)

// Client manages the WebSocket connection to the signaling server.
type Client struct {
	relayURL       string
	conn           *websocket.Conn
	AgentID        string
	mu             sync.Mutex
	handlers       map[MessageType][]MessageHandler
	handlerMu      sync.RWMutex
	reconnectCh    chan struct{}
	passwordHash   string
	unattendedHash string
	version        string
	orgID          string
	deviceGroup    string
	enrollmentCode string
	certFile       string
	keyFile        string
	caFile         string
}

// NewClient creates a new signaling client.
func NewClient(relayURL, passwordHash, unattendedHash, version, orgID, deviceGroup, enrollmentCode string) *Client {
	return &Client{
		relayURL:       relayURL,
		passwordHash:   passwordHash,
		unattendedHash: unattendedHash,
		version:        version,
		orgID:          orgID,
		deviceGroup:    deviceGroup,
		enrollmentCode: enrollmentCode,
		certFile:       os.Getenv("AGENT_CERT_FILE"),
		keyFile:        os.Getenv("AGENT_KEY_FILE"),
		caFile:         os.Getenv("AGENT_CA_FILE"),
		handlers:       make(map[MessageType][]MessageHandler),
		reconnectCh:    make(chan struct{}, 1),
	}
}

// UpdateUnattendedHash updates the unattended password hash and sends a re-registration to the signaling server.
func (c *Client) UpdateUnattendedHash(hash string) {
	c.mu.Lock()
	c.unattendedHash = hash
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		if err := c.register(); err != nil {
			log.Error().Err(err).Msg("failed to re-register after unattended password update")
		}
	}
}

// On registers a handler for a specific message type.
func (c *Client) On(msgType MessageType, handler MessageHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.handlers[msgType] = append(c.handlers[msgType], handler)
}

// Send sends a message to the signaling server.
func (c *Client) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteJSON(msg)
}

// Connect establishes the WebSocket connection and starts message loops.
// It reconnects automatically with exponential backoff on disconnection.
func (c *Client) Connect(ctx context.Context) error {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := c.dial(ctx); err != nil {
			log.Warn().Err(err).Dur("retry_in", backoff).Msg("signaling connect failed, retrying")
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		backoff = 1 * time.Second
		if err := c.register(); err != nil {
			log.Error().Err(err).Msg("registration failed")
			continue
		}

		// Start heartbeat.
		stopHB := make(chan struct{})
		go c.heartbeat(ctx, stopHB)

		// Read loop (blocks until disconnect).
		c.readLoop()
		close(stopHB)

		log.Warn().Msg("signaling connection lost, reconnecting…")
	}
}

func (c *Client) dial(ctx context.Context) error {
	u, err := url.Parse(c.relayURL)
	if err != nil {
		return err
	}

	if c.usingMTLS() {
		u.Path = agentWSPath(u.Path)
	}

	dialer := *websocket.DefaultDialer
	if tlsConfig, err := c.tlsConfig(); err != nil {
		return err
	} else if tlsConfig != nil {
		dialer.TLSClientConfig = tlsConfig
	}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *Client) usingMTLS() bool {
	if strings.TrimSpace(c.certFile) == "" || strings.TrimSpace(c.keyFile) == "" {
		return false
	}
	if _, err := os.Stat(c.certFile); err != nil {
		return false
	}
	if _, err := os.Stat(c.keyFile); err != nil {
		return false
	}
	return true
}

func (c *Client) tlsConfig() (*tls.Config, error) {
	if !c.usingMTLS() {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if strings.TrimSpace(c.caFile) != "" {
		caData, err := os.ReadFile(c.caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("failed to load agent CA file")
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

func agentWSPath(current string) string {
	trimmed := strings.TrimRight(current, "/")
	if trimmed == "" || trimmed == "/" {
		return "/agent/ws"
	}
	if strings.HasSuffix(trimmed, "/ws") {
		return strings.TrimSuffix(trimmed, "/ws") + "/agent/ws"
	}
	if strings.HasSuffix(trimmed, "/agent/ws") {
		return trimmed
	}
	return path.Join(trimmed, "agent/ws")
}

func (c *Client) register() error {
	hostname, _ := os.Hostname()
	payload, _ := json.Marshal(RegisterPayload{
		AgentID:        c.AgentID,
		Hostname:       hostname,
		OS:             runtime.GOOS,
		Version:        c.version,
		PasswordHash:   c.passwordHash,
		UnattendedHash: c.unattendedHash,
		OrgID:          c.orgID,
		DeviceGroup:    c.deviceGroup,
		EnrollmentCode: c.enrollmentCode,
	})
	return c.Send(Message{Type: MsgRegister, Payload: payload})
}

func (c *Client) readLoop() {
	for {
		var msg Message
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if err := conn.ReadJSON(&msg); err != nil {
			log.Debug().Err(err).Msg("ws read error")
			return
		}

		if msg.Type == MsgError {
			log.Error().Str("error", msg.Error).Msg("signaling server returned error")
		}

		// Handle registration response inline.
		if msg.Type == MsgRegistered {
			c.handleClientCertPayload(msg.Payload, "registered")
		}
		if msg.Type == MsgClientCert {
			c.handleClientCertPayload(msg.Payload, "rotation")
		}

		c.dispatch(msg)
	}
}

func (c *Client) handleClientCertPayload(payload []byte, source string) {
	var p RegisteredPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}
	if strings.TrimSpace(p.AgentID) != "" {
		c.AgentID = p.AgentID
	}
	log.Info().Str("id", c.AgentID).Str("source", source).Msg("✅ registered with signaling server")
	if p.ClientCert != nil && strings.TrimSpace(p.ClientCert.CertPEM) != "" && strings.TrimSpace(p.ClientCert.KeyPEM) != "" {
		if err := c.persistClientCertBundle(p.ClientCert); err != nil {
			log.Error().Err(err).Msg("failed to persist issued client certificate")
		} else {
			log.Info().Str("id", c.AgentID).Str("source", source).Msg("persisted issued client certificate; reconnecting with mTLS")
			_ = c.closeConn()
		}
	}
}

func (c *Client) dispatch(msg Message) {
	c.handlerMu.RLock()
	handlers := c.handlers[msg.Type]
	c.handlerMu.RUnlock()
	for _, h := range handlers {
		go h(msg)
	}
}

func (c *Client) heartbeat(ctx context.Context, stop chan struct{}) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = c.Send(Message{Type: MsgHeartbeat})
		case <-stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) persistClientCertBundle(bundle *ClientCertBundle) error {
	if bundle == nil {
		return nil
	}
	if strings.TrimSpace(c.certFile) == "" || strings.TrimSpace(c.keyFile) == "" {
		return fmt.Errorf("agent certificate paths are not configured")
	}
	if err := writeSecureFile(c.certFile, []byte(bundle.CertPEM)); err != nil {
		return err
	}
	if err := writeSecureFile(c.keyFile, []byte(bundle.KeyPEM)); err != nil {
		return err
	}
	return nil
}

func writeSecureFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c *Client) closeConn() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
