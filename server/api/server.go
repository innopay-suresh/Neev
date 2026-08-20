package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/websocket/v2"
	"github.com/rs/zerolog/log"

	serverauth "github.com/neev/remote-agent/server/auth"
	"github.com/neev/remote-agent/server/config"
	"github.com/neev/remote-agent/server/session"
	"github.com/neev/remote-agent/server/signaling"
)

// Server is the HTTP + WebSocket server.
type Server struct {
	app            *fiber.App
	cfg            *config.Config
	registry       *session.Registry
	hub            *signaling.Hub
	authStore      *serverauth.Store
	allowedOrigins []string
	startTime      time.Time
}

// New creates and configures the Fiber application.
func New(cfg *config.Config, registry *session.Registry, hub *signaling.Hub, authStore *serverauth.Store) *Server {
	app := fiber.New(fiber.Config{
		AppName:      "Remote Agent Signaling Server",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	// Middleware
	allowedOriginList := configureSecurity(app, cfg)
	app.Use(compress.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} ${locals:requestid} - ${method} ${path} (${latency})\n",
	}))
	app.Use(limiter.New(limiter.Config{
		Max:        200,
		Expiration: 1 * time.Minute,
	}))

	s := &Server{app: app, cfg: cfg, registry: registry, hub: hub, authStore: authStore, allowedOrigins: allowedOriginList, startTime: time.Now()}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Health check
	s.app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"version": "1.0.0",
			"time":    time.Now().UTC(),
		})
	})

	s.app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/downloads", fiber.StatusFound)
	})

	// WebSocket upgrade — agents and controllers connect here
	s.app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	s.app.Use("/ws", controllerWSGuard(), wsOriginGuard(s.allowedOrigins))
	s.app.Get("/ws", websocket.New(s.hub.HandleWS, websocket.Config{
		HandshakeTimeout: 10 * time.Second,
	}))
	s.app.Use("/agent/ws", agentWSGuard())
	s.app.Get("/agent/ws", websocket.New(s.hub.HandleWS, websocket.Config{
		HandshakeTimeout: 10 * time.Second,
	}))

	// REST API v1
	v1 := s.app.Group("/api/v1")
	v1.Post("/auth/login", s.login)
	v1.Get("/auth/me", s.requireAuth(), s.me)
	v1.Post("/auth/mfa/setup", s.requireAuth(), s.setupMFA)
	v1.Post("/auth/mfa/confirm", s.requireAuth(), s.confirmMFA)
	v1.Delete("/auth/mfa", s.requireAuth(), s.disableMFA)
	public := v1.Group("/public")
	public.Get("/installers", s.listPublicInstallers)
	public.Get("/installers/:filename", s.downloadInstaller)
	public.Get("/flutter-installers", s.listFlutterInstallers)
	public.Get("/flutter-installers/:filename", s.downloadFlutterInstaller)
	v1.Get("/agent/:id", s.getAgent)
	v1.Get("/session/ice-servers", s.getICEServers)
	admin := v1.Group("/admin", s.requireRole(serverauth.RoleAdmin))
	admin.Get("/enrollment", s.getEnrollment)
	admin.Get("/bootstrap", s.getBootstrap)
	admin.Get("/trust-bundle", s.getTrustBundle)
	admin.Get("/users", s.listUsers)
	admin.Post("/users", s.createUser)
	admin.Patch("/users/:email", s.updateUser)
	admin.Delete("/users/:email", s.deleteUser)
	admin.Post("/agents/:id/certificate/reissue", s.reissueAgentCertificate)
	admin.Delete("/agents/:id/certificate", s.revokeAgentCertificate)

	// Dashboard API
	dash := v1.Group("/dashboard", s.requireRole(serverauth.RoleSupport))
	dash.Get("/agents", s.listAgents)
	dash.Get("/sessions", s.listSessions)
	dash.Get("/stats", s.getStats)
	dash.Get("/analytics", s.getAnalytics)
	dash.Get("/audit", s.requireRole(serverauth.RoleAdmin), s.listAuditEvents)

	// Serve static web app (if built and placed in ./public)
	if _, err := os.Stat("./public"); err == nil {
		s.app.Static("/", "./public")
		// Catch-all for SPA routing
		s.app.Get("/*", func(c *fiber.Ctx) error {
			return c.SendFile("./public/index.html")
		})
	}
}

// getAgent returns public info about a registered agent.
func (s *Server) getAgent(c *fiber.Ctx) error {
	id := c.Params("id")
	info, err := s.registry.Get(c.Context(), id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "agent not found"})
	}
	return c.JSON(toAgentResponse(info))
}

// getICEServers returns the STUN/TURN server list for WebRTC clients.
func (s *Server) getICEServers(c *fiber.Ctx) error {
	servers := []fiber.Map{
		{"urls": s.cfg.Network.STUNServers},
	}
	if s.cfg.Network.TURNServer != "" {
		// Advertise the relay over BOTH UDP and TCP. Many networks (and the
		// Win<->Win case here) block UDP while allowing TCP, so the TCP variant
		// is what actually makes the relay usable when the direct path fails.
		urls := []string{
			s.cfg.Network.TURNServer,
			s.cfg.Network.TURNServer + "?transport=tcp",
		}
		servers = append(servers, fiber.Map{
			"urls":       urls,
			"username":   s.cfg.TURN.AuthUser,
			"credential": s.cfg.TURN.AuthPass,
		})
	}
	return c.JSON(fiber.Map{"ice_servers": servers})
}

// getEnrollment returns the current fleet enrollment settings for the admin dashboard.
func (s *Server) getEnrollment(c *fiber.Ctx) error {
	relayURL := s.effectiveRelayURL(c)
	return c.JSON(fiber.Map{
		"relay_url":            relayURL,
		"agent_relay_url":      agentRelayURL(relayURL),
		"enrollment_code":      s.cfg.Network.EnrollmentCode,
		"default_org_id":       s.cfg.Network.DefaultOrgID,
		"default_device_group": s.cfg.Network.DefaultDeviceGroup,
		"stun_servers":         s.cfg.Network.STUNServers,
		"turn_server":          s.cfg.Network.TURNServer,
		"managed_device_trust": strings.TrimSpace(s.cfg.Server.TLSClientCAKey) != "",
		"mtls_enabled":         strings.TrimSpace(s.cfg.Server.TLSClientCA) != "",
		"install_hint":         "Install the agent, point it at the relay URL, and provide the enrollment code during first launch.",
	})
}

// getBootstrap returns generated install snippets for Linux, macOS, and Windows.
func (s *Server) getBootstrap(c *fiber.Ctx) error {
	relayURL := s.effectiveRelayURL(c)
	envFile := renderBootstrapEnv(s, relayURL)
	return c.JSON(fiber.Map{
		"relay_url":            relayURL,
		"agent_relay_url":      agentRelayURL(relayURL),
		"enrollment_code":      s.cfg.Network.EnrollmentCode,
		"default_org_id":       s.cfg.Network.DefaultOrgID,
		"default_device_group": s.cfg.Network.DefaultDeviceGroup,
		"managed_device_trust": strings.TrimSpace(s.cfg.Server.TLSClientCAKey) != "",
		"linux_config_path":    "/etc/remote-agent/agent.env",
		"mac_config_path":      "/Library/Application Support/RemoteAgent/agent.env",
		"windows_config_path":  `%ProgramData%\RemoteAgent\agent.env`,
		"env_file":             envFile,
		"linux_install":        buildLinuxInstallCommand(envFile),
		"mac_install":          buildMacInstallCommand(envFile),
		"windows_install":      buildWindowsInstallCommand(envFile),
	})
}

// getTrustBundle returns the managed agent CA bundle for admin inspection.
func (s *Server) getTrustBundle(c *fiber.Ctx) error {
	if s.hub == nil || !s.hub.ManagedDeviceTrust() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "managed device trust is not configured"})
	}
	return c.JSON(fiber.Map{
		"managed_device_trust":  true,
		"client_ca_pem":         s.hub.ClientCAPEM(),
		"client_ca_fingerprint": s.hub.ClientCAFingerprint(),
	})
}

func (s *Server) agentRelayURL() string {
	return agentRelayURL(s.cfg.Network.RelayURL)
}

func agentRelayURL(relayURL string) string {
	relayURL = strings.TrimSpace(relayURL)
	if relayURL == "" {
		return ""
	}
	if strings.Contains(relayURL, "/agent/ws") {
		return relayURL
	}
	if strings.HasSuffix(relayURL, "/ws") {
		return strings.TrimSuffix(relayURL, "/ws") + "/agent/ws"
	}
	return relayURL
}

func (s *Server) effectiveRelayURL(c *fiber.Ctx) string {
	relayURL := strings.TrimSpace(s.cfg.Network.RelayURL)
	if relayURL != "" && !isPlaceholderRelayURL(relayURL) {
		return relayURL
	}
	if c == nil {
		return relayURL
	}
	baseURL := strings.TrimRight(c.BaseURL(), "/")
	if baseURL == "" {
		return relayURL
	}
	if strings.HasPrefix(baseURL, "https://") {
		return "wss://" + strings.TrimPrefix(baseURL, "https://") + "/ws"
	}
	if strings.HasPrefix(baseURL, "http://") {
		return "ws://" + strings.TrimPrefix(baseURL, "http://") + "/ws"
	}
	return relayURL
}

func isPlaceholderRelayURL(raw string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	return normalized == "" ||
		normalized == "ws://localhost:8080/ws" ||
		normalized == "ws://127.0.0.1:8080/ws" ||
		normalized == "wss://localhost:8080/ws" ||
		normalized == "wss://127.0.0.1:8080/ws" ||
		strings.Contains(normalized, "0.0.0.0")
}

func renderBootstrapEnv(s *Server, relayURL string) string {
	lines := []string{
		"RELAY_URL=" + relayURL,
	}
	if s.cfg.Network.EnrollmentCode != "" {
		lines = append(lines, "ENROLLMENT_CODE="+s.cfg.Network.EnrollmentCode)
	}
	if s.cfg.Network.DefaultOrgID != "" {
		lines = append(lines, "ORG_ID="+s.cfg.Network.DefaultOrgID)
	}
	if s.cfg.Network.DefaultDeviceGroup != "" {
		lines = append(lines, "DEVICE_GROUP="+s.cfg.Network.DefaultDeviceGroup)
	}
	if s.cfg.Network.TURNServer != "" {
		lines = append(lines, "TURN_URL="+s.cfg.Network.TURNServer)
	}
	if s.cfg.TURN.AuthUser != "" {
		lines = append(lines, "TURN_USER="+s.cfg.TURN.AuthUser)
	}
	if s.cfg.TURN.AuthPass != "" {
		lines = append(lines, "TURN_PASS="+s.cfg.TURN.AuthPass)
	}
	lines = append(lines, "NO_BROWSER=1")
	return strings.Join(lines, "\n") + "\n"
}

func buildLinuxInstallCommand(envFile string) string {
	return "sudo install -d -m 755 /etc/remote-agent && printf '%s' " + shellQuote(envFile) + " | sudo tee /etc/remote-agent/agent.env >/dev/null && sudo systemctl daemon-reload && sudo systemctl enable --now remote-agent"
}

func buildMacInstallCommand(envFile string) string {
	return "sudo mkdir -p \"/Library/Application Support/RemoteAgent\" && printf '%s' " + shellQuote(envFile) + " | sudo tee \"/Library/Application Support/RemoteAgent/agent.env\" >/dev/null && sudo launchctl bootstrap system /Library/LaunchDaemons/com.neev.remoteagent.plist"
}

func buildWindowsInstallCommand(envFile string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(envFile))
	return "powershell -NoProfile -ExecutionPolicy Bypass -Command \"$dir = Join-Path $env:ProgramData 'RemoteAgent'; New-Item -ItemType Directory -Force -Path $dir | Out-Null; $bytes = [Convert]::FromBase64String('" + encoded + "'); [IO.File]::WriteAllText((Join-Path $dir 'agent.env'), [Text.Encoding]::UTF8.GetString($bytes))\""
}

func shellQuote(value string) string {
	escaped := strings.ReplaceAll(value, "'", `'"'"'`)
	return "'" + escaped + "'"
}

// Listen starts the HTTP server. If TLSCert and TLSKey are provided in config, it starts with TLS.
func (s *Server) Listen(addr string) error {
	if s.cfg.Server.TLSCert != "" && s.cfg.Server.TLSKey != "" {
		if strings.TrimSpace(s.cfg.Server.TLSClientCA) != "" {
			return s.listenTLSWithClientCA(addr)
		}
		return s.app.ListenTLS(addr, s.cfg.Server.TLSCert, s.cfg.Server.TLSKey)
	}
	return s.app.Listen(addr)
}

// ListenDual serves plaintext ws:// on plainAddr AND, when a TLS port + cert are
// configured, wss:// on the TLS port from the SAME Fiber app in a goroutine. This
// avoids a flag-day: existing ws:// installs keep working on plainAddr while new
// builds move to wss, and plainAddr is retired only after every client migrates.
// Falls back to plain-only when TLS isn't configured.
func (s *Server) ListenDual(plainAddr string) error {
	if s.cfg.Server.TLSPort > 0 && s.cfg.Server.TLSCert != "" && s.cfg.Server.TLSKey != "" {
		tlsAddr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.TLSPort)
		go func() {
			cert, err := tls.LoadX509KeyPair(s.cfg.Server.TLSCert, s.cfg.Server.TLSKey)
			if err != nil {
				log.Error().Err(err).Msg("tls: cannot load server certificate; wss disabled")
				return
			}
			cfg := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
			if strings.TrimSpace(s.cfg.Server.TLSClientCA) != "" {
				if caData, e := os.ReadFile(s.cfg.Server.TLSClientCA); e == nil {
					pool := x509.NewCertPool()
					if pool.AppendCertsFromPEM(caData) {
						cfg.ClientCAs = pool
						cfg.ClientAuth = tls.VerifyClientCertIfGiven
					}
				}
			}
			ln, err := net.Listen("tcp", tlsAddr)
			if err != nil {
				log.Error().Err(err).Str("addr", tlsAddr).Msg("tls: cannot bind wss port")
				return
			}
			log.Info().Str("addr", tlsAddr).Msg("wss (TLS) listener up")
			if err := s.app.Listener(tls.NewListener(ln, cfg)); err != nil {
				log.Error().Err(err).Msg("tls listener stopped")
			}
		}()
	}
	log.Info().Str("addr", plainAddr).Msg("ws (plaintext) listener up")
	return s.app.Listen(plainAddr)
}

func (s *Server) listenTLSWithClientCA(addr string) error {
	cert, err := tls.LoadX509KeyPair(s.cfg.Server.TLSCert, s.cfg.Server.TLSKey)
	if err != nil {
		return fmt.Errorf("tls: cannot load server certificate: %w", err)
	}
	caData, err := os.ReadFile(s.cfg.Server.TLSClientCA)
	if err != nil {
		return fmt.Errorf("tls: cannot read client CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return fmt.Errorf("tls: invalid client CA bundle")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	tlsListener := tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	})
	return s.app.Listener(tlsListener)
}

// ── Dashboard handlers ────────────────────────────────────────────────────

// listAgents returns all registered agents with live status.
func (s *Server) listAgents(c *fiber.Ctx) error {
	agents, err := s.registry.List(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	result := make([]fiber.Map, 0, len(agents))
	for _, a := range agents {
		result = append(result, toAgentResponse(a))
	}
	return c.JSON(fiber.Map{"agents": result, "total": len(result)})
}

func toAgentResponse(info *session.AgentInfo) fiber.Map {
	if info == nil {
		return fiber.Map{}
	}
	response := fiber.Map{
		"id":                      info.ID,
		"hostname":                info.Hostname,
		"os":                      info.OS,
		"version":                 info.Version,
		"org_id":                  info.OrgID,
		"device_group":            info.DeviceGroup,
		"client_cert_fingerprint": info.ClientCertFingerprint,
		"client_cert_revoked":     info.ClientCertRevoked,
		"status":                  info.Status,
		"last_seen":               info.LastSeen,
		"sessions":                info.SessionCount,
	}
	if !info.ClientCertRevokedAt.IsZero() {
		response["client_cert_revoked_at"] = info.ClientCertRevokedAt
	}
	return response
}

// listSessions returns recent sessions (last 100).
func (s *Server) listSessions(c *fiber.Ctx) error {
	sessions, err := s.registry.ListSessions(c.Context(), 100)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"sessions": sessions, "total": len(sessions)})
}

// listAuditEvents returns recent security and lifecycle events.
func (s *Server) listAuditEvents(c *fiber.Ctx) error {
	events, err := s.registry.ListAuditEvents(c.Context(), 100)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"events": events, "total": len(events)})
}

// getStats returns aggregate server statistics.
func (s *Server) getStats(c *fiber.Ctx) error {
	agents, _ := s.registry.List(c.Context())
	online := 0
	for _, a := range agents {
		if a.Status != session.StatusOffline {
			online++
		}
	}
	sessions, _ := s.registry.ListSessions(c.Context(), 1000)
	activeSessions := 0
	for _, sess := range sessions {
		if sess.Status == session.SessionStatusActive {
			activeSessions++
		}
	}
	return c.JSON(fiber.Map{
		"agents_total":    len(agents),
		"agents_online":   online,
		"sessions_active": activeSessions,
		"sessions_total":  len(sessions),
		"server_time":     time.Now().UTC(),
		"uptime_seconds":  int(time.Since(s.startTime).Seconds()),
	})
}

// getAnalytics aggregates REAL history (from the audit log + agents) for the
// Analytics module: a 30-day session trend, top devices, connection outcomes
// and device health. Per-session duration/bytes aren't reported by agents, so
// only tracked metrics are returned.
func (s *Server) getAnalytics(c *fiber.Ctx) error {
	ctx := c.Context()

	agents, _ := s.registry.List(ctx)
	online := 0
	hostByID := make(map[string]string, len(agents))
	for _, a := range agents {
		if a.Status != session.StatusOffline {
			online++
		}
		name := a.Hostname
		if name == "" {
			name = a.ID
		}
		hostByID[a.ID] = name
	}

	events, _ := s.registry.ListAuditEvents(ctx, 5000)

	const days = 30
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -(days - 1))
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	perDay := make([]int, days)
	deviceCounts := map[string]int{}
	outcomes := map[string]int{}
	sessionsToday := 0
	for _, e := range events {
		if e.Type != "session.connect" {
			continue
		}
		outcomes[e.Outcome]++
		if e.Target != "" {
			deviceCounts[e.Target]++
		}
		if !e.CreatedAt.Before(todayStart) {
			sessionsToday++
		}
		idx := int(e.CreatedAt.UTC().Sub(start).Hours() / 24)
		if idx >= 0 && idx < days {
			perDay[idx]++
		}
	}

	trend := make([]fiber.Map, days)
	for i := 0; i < days; i++ {
		trend[i] = fiber.Map{
			"label":    start.AddDate(0, 0, i).Format("Jan 2"),
			"sessions": perDay[i],
		}
	}

	type dc struct {
		id string
		n  int
	}
	top := make([]dc, 0, len(deviceCounts))
	for id, n := range deviceCounts {
		top = append(top, dc{id, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	topDevices := make([]fiber.Map, 0, 8)
	for i := 0; i < len(top) && i < 8; i++ {
		name := hostByID[top[i].id]
		if name == "" {
			name = top[i].id
		}
		topDevices = append(topDevices, fiber.Map{"name": name, "sessions": top[i].n})
	}

	sessions, _ := s.registry.ListSessions(ctx, 1000)

	return c.JSON(fiber.Map{
		"trend":       trend,
		"top_devices": topDevices,
		"outcomes": []fiber.Map{
			{"name": "Accepted", "value": outcomes["accepted"]},
			{"name": "Denied", "value": outcomes["denied"]},
			{"name": "Rate limited", "value": outcomes["rate_limited"]},
		},
		"health": []fiber.Map{
			{"name": "Online", "value": online},
			{"name": "Offline", "value": len(agents) - online},
		},
		"summary": fiber.Map{
			"active_devices": online,
			"total_devices":  len(agents),
			"sessions_today": sessionsToday,
			"sessions_total": len(sessions),
		},
	})
}
