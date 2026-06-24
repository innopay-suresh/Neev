// Package backend is the Go backend for the Neev Remote desktop client.
// It is bound to the Wails runtime and exposes methods callable from JS.
package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/neev/remote-agent/agent/bootstrap"
	"github.com/neev/remote-agent/agent/core"
)

// ConnectionState mirrors the JS-side enum.
type ConnectionState string

const (
	StateIdle         ConnectionState = "idle"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateDisconnected ConnectionState = "disconnected"
	StateError        ConnectionState = "error"
)

// SessionInfo holds the active session metadata sent to the frontend.
type SessionInfo struct {
	AgentID     string          `json:"agent_id"`
	Hostname    string          `json:"hostname"`
	OS          string          `json:"os"`
	State       ConnectionState `json:"state"`
	Latency     int             `json:"latency_ms"`
	BitrateKbps int             `json:"bitrate_kbps"`
	FPS         int             `json:"fps"`
	StartedAt   string          `json:"started_at"`
}

// AppSettings stores user configurations like unattended access password.
type AppSettings struct {
	UnattendedPassword string `json:"unattended_password"`
	RelayURL           string `json:"relay_url"`
}

// RecentConnection is a saved recent session (persisted in prefs).
type RecentConnection struct {
	AgentID     string    `json:"agent_id"`
	Label       string    `json:"label"`
	LastUsed    string    `json:"last_used"`
}

// App is the main application backend — bound to JS via Wails.
type App struct {
	ctx         context.Context
	cancel      context.CancelFunc
	agentCtx    context.Context
	agentCancel context.CancelFunc
	mu          sync.Mutex

	// Active session
	session     *controllerSession
	sessionInfo SessionInfo

	// Persisted state
	recents  []RecentConnection
	settings AppSettings
	relayURL string

	// Wails event emitter (set after DomReady)
	emitFn func(eventName string, optionalData ...interface{})

	// Buffered logs for the in-app log viewer.
	logMu sync.Mutex
	logs  []LogEntry

	// Local host agent
	hostAgent   *core.AgentInstance
	cachedAgent *LocalAgentInfo
}

// NewApp creates the app backend.
func NewApp() *App {
	bootstrapCfg, err := bootstrap.Load()
	if err != nil {
		log.Warn().Err(err).Msg("failed to load bootstrap config; falling back to environment")
	}

	relayURL := bootstrapCfg.RelayURL
	if relayURL == "" {
		relayURL = envOr("REMOTE_AGENT_SERVER_URL", envOr("RELAY_URL", "ws://localhost:8080/ws"))
	}

	return &App{
		relayURL: relayURL,
	}
}

// Startup is called when the Wails app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.loadRecents()
	a.loadSettings()
	
	if a.settings.RelayURL != "" {
		a.relayURL = a.settings.RelayURL
	}
	
	log.Info().Str("relay", a.relayURL).Msg("Neev Remote client starting")

	// Start local host agent in the background so the UI is never blocked.
	// If the relay server is unreachable, the app still launches normally.
	go a.startHostAgent()
}

// startHostAgent connects to the relay and starts the local agent.
// Runs in a goroutine so it never blocks the Wails UI from appearing.
func (a *App) startHostAgent() {
	onChat := func(msg string) {
		if a.emitFn != nil {
			a.emitFn("host:chat_received", msg)
		}
	}

	a.mu.Lock()
	if a.agentCancel != nil {
		a.agentCancel()
	}
	a.agentCtx, a.agentCancel = context.WithCancel(a.ctx)
	agentCtx := a.agentCtx
	a.mu.Unlock()

	// Use a timeout context so we don't hang forever if the relay is down.
	startCtx, startCancel := context.WithTimeout(agentCtx, 15*time.Second)
	defer startCancel()

	// Always start an in-process agent so screen capture runs in the logged-in user's
	// session (not in a LocalSystem service that has no desktop access).
	// The background service (port 7891) is for unattended access only; it cannot
	// capture the screen when running as LocalSystem.
	instance, err := core.StartAgentWithContext(startCtx, agentCtx, a.relayURL, a.settings.UnattendedPassword, onChat)
	if err != nil {
		log.Warn().Err(err).Msg("Host agent failed to start (relay may be unreachable); app will work in controller-only mode")
		// Emit an event so the frontend can show a warning
		if a.emitFn != nil {
			a.emitFn("host:agent_error", map[string]interface{}{
				"error": err.Error(),
			})
		}
		return
	}
	a.hostAgent = instance
	log.Info().Str("AgentID", instance.GetID()).Msg("Host Agent Ready")
	// Emit the agent info to frontend once ready
	if a.emitFn != nil {
		a.emitFn("host:agent_ready", map[string]interface{}{
			"id":       instance.GetID(),
			"password": instance.Password,
		})
	}
}

// DomReady is called when the frontend DOM is ready.
func (a *App) DomReady(ctx context.Context) {
	a.emitFn = func(name string, data ...interface{}) {
		wailsrt.EventsEmit(ctx, name, data...)
	}
}

// Shutdown is called when the app is closing.
func (a *App) Shutdown(ctx context.Context) {
	a.cancel()
	a.mu.Lock()
	sess := a.session
	a.mu.Unlock()
	if sess != nil {
		sess.disconnect()
	}
}

// ─── JS-callable methods ────────────────────────────────────────────────────

// GetVersion returns the agent version string.
func (a *App) GetVersion() string {
	return fmt.Sprintf("1.0.0 (%s/%s)", runtime.GOOS, runtime.GOARCH)
}

// LocalAgentInfo contains the local host agent credentials.
type LocalAgentInfo struct {
	ID       string `json:"id"`
	Password string `json:"password"`
}

// GetLocalAgent returns the local host agent credentials to display in the UI.
func (a *App) GetLocalAgent() LocalAgentInfo {
	a.mu.Lock()
	cached := a.cachedAgent
	a.mu.Unlock()

	if cached != nil {
		return *cached
	}

	if a.hostAgent == nil {
		return LocalAgentInfo{}
	}
	return LocalAgentInfo{
		ID:       a.hostAgent.GetID(),
		Password: a.hostAgent.Password,
	}
}

// GetRelayURL returns the currently configured relay URL.
func (a *App) GetRelayURL() string {
	return a.relayURL
}

// SetRelayURL updates the relay server URL.
func (a *App) SetRelayURL(url string) {
	a.relayURL = url
	a.mu.Lock()
	a.settings.RelayURL = url
	a.mu.Unlock()
	a.saveSettings()
	
	// Restart the local host agent with the new URL
	go a.startHostAgent()
}

// GetSettings returns the current app settings.
func (a *App) GetSettings() AppSettings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

// SaveSettings updates and saves the app settings to disk.
func (a *App) SaveSettings(s AppSettings) {
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	a.saveSettings()
	// Update host agent password dynamically if it's running
	if a.hostAgent != nil {
		if err := a.hostAgent.SetUnattendedPassword(s.UnattendedPassword); err != nil {
			log.Error().Err(err).Msg("Failed to update host agent password")
		}
	}
}

// GetRecentConnections returns the list of recent connections.
func (a *App) GetRecentConnections() []RecentConnection {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recents
}

// GetLocalIPs returns all non-loopback IPv4 addresses of the machine.
func (a *App) GetLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return ips
}

// Connect initiates a connection to the given agentID with password.
// Returns an error string (empty = success).
func (a *App) Connect(agentID, password string) string {
	a.mu.Lock()
	if a.session != nil {
		a.session.disconnect()
		a.session = nil
	}
	a.mu.Unlock()

	a.emitState(StateConnecting, agentID)

	sess, err := newControllerSession(a.ctx, a.relayURL, agentID, password)
	if err != nil {
		a.emitState(StateError, agentID)
		return err.Error()
	}

	a.mu.Lock()
	a.session = sess
	a.mu.Unlock()

	// Save to recents.
	a.addRecent(agentID)

	// Forward session events to the frontend.
	go a.monitorSession(sess, agentID)

	return ""
}

// Disconnect tears down the active session.
func (a *App) Disconnect() {
	a.mu.Lock()
	sess := a.session
	a.session = nil
	a.mu.Unlock()

	if sess != nil {
		sess.disconnect()
	}
	a.emitState(StateDisconnected, "")
}

// GetSessionInfo returns live metrics for the active session.
func (a *App) GetSessionInfo() SessionInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionInfo
}

// SendInputEvent forwards a serialized input.Event JSON to the remote host.
// Called from JS on every mouse move / key press.
func (a *App) SendInputEvent(eventJSON string) string {
	a.mu.Lock()
	sess := a.session
	a.mu.Unlock()
	if sess == nil {
		return "not connected"
	}
	if err := sess.sendInput([]byte(eventJSON)); err != nil {
		return err.Error()
	}
	return ""
}

// SendHostChat sends a chat message from the Host UI to all connected controllers.
func (a *App) SendHostChat(msg string) {
	if a.hostAgent != nil {
		a.hostAgent.SendChat(msg)
	}
}

// ─── Internal helpers ────────────────────────────────────────────────────────

func (a *App) emitState(state ConnectionState, agentID string) {
	if a.emitFn == nil {
		return
	}
	a.mu.Lock()
	a.sessionInfo.State = state
	a.sessionInfo.AgentID = agentID
	a.mu.Unlock()
	a.emitFn("session:state", map[string]interface{}{
		"state":    state,
		"agent_id": agentID,
	})
}

func (a *App) monitorSession(sess *controllerSession, agentID string) {
	// Wait for WebRTC to connect.
	select {
	case <-sess.connected:
		a.mu.Lock()
		a.sessionInfo.State = StateConnected
		a.sessionInfo.AgentID = agentID
		a.sessionInfo.StartedAt = time.Now().Format(time.RFC3339)
		a.mu.Unlock()
		a.emitState(StateConnected, agentID)
	case <-time.After(30 * time.Second):
		a.emitState(StateError, agentID)
		return
	case <-a.ctx.Done():
		return
	}

	// Periodic stats update.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			stats := sess.getStats()
			a.mu.Lock()
			a.sessionInfo.Latency = stats.LatencyMs
			a.sessionInfo.BitrateKbps = stats.BitrateKbps
			a.sessionInfo.FPS = stats.FPS
			info := a.sessionInfo
			a.mu.Unlock()
			if a.emitFn != nil {
				a.emitFn("session:stats", info)
			}
		case <-sess.done:
			a.emitState(StateDisconnected, agentID)
			return
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) addRecent(agentID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Update existing or prepend.
	found := false
	for i, r := range a.recents {
		if r.AgentID == agentID {
			a.recents[i].LastUsed = time.Now().Format(time.RFC3339)
			found = true
			break
		}
	}
	if !found {
		a.recents = append([]RecentConnection{{
			AgentID:  agentID,
			Label:    agentID,
			LastUsed: time.Now().Format(time.RFC3339),
		}}, a.recents...)
	}
	// Cap at 10 recents.
	if len(a.recents) > 10 {
		a.recents = a.recents[:10]
	}
	a.saveRecents()
}

func recentsPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/remote-agent/recents.json"
}

func (a *App) loadRecents() {
	data, err := os.ReadFile(recentsPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.recents)
}

func (a *App) saveRecents() {
	data, _ := json.Marshal(a.recents)
	_ = os.MkdirAll(recentsPath()[:len(recentsPath())-len("/recents.json")], 0755)
	_ = os.WriteFile(recentsPath(), data, 0644)
}

func settingsPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/remote-agent/settings.json"
}

func (a *App) loadSettings() {
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.settings)
}

func (a *App) saveSettings() {
	data, _ := json.Marshal(a.settings)
	_ = os.MkdirAll(settingsPath()[:len(settingsPath())-len("/settings.json")], 0755)
	_ = os.WriteFile(settingsPath(), data, 0644)
}

func envOr(k, v string) string {
	if e := os.Getenv(k); e != "" {
		return e
	}
	return v
}
