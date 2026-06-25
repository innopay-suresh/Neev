package api

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/neev/remote-agent/server/config"
)

func configureSecurity(app *fiber.App, cfg *config.Config) []string {
	allowedOrigins := computeAllowedOrigins(cfg)

	var corsOrigins []string
	for _, o := range allowedOrigins {
		if strings.HasPrefix(o, "http://") || strings.HasPrefix(o, "https://") {
			corsOrigins = append(corsOrigins, o)
		}
	}

	app.Use(requestid.New())
	app.Use(recover.New())
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Join(corsOrigins, ","),
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-Request-Id",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowCredentials: false,
	}))

	return allowedOrigins
}

func computeAllowedOrigins(cfg *config.Config) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, 6)

	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	for _, origin := range cfg.Server.AllowedOrigins {
		add(origin)
	}
	for _, origin := range devOrigins() {
		add(origin)
	}
	if relayOrigin := originFromRelayURL(cfg.Network.RelayURL); relayOrigin != "" {
		add(relayOrigin)
	}

	return result
}

func devOrigins() []string {
	return []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"wails://wails",
		"http://wails.localhost",
	}
}

func originFromRelayURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := "http"
	switch parsed.Scheme {
	case "wss":
		scheme = "https"
	case "https":
		scheme = "https"
	case "ws":
		scheme = "http"
	case "http":
		scheme = "http"
	}
	return scheme + "://" + parsed.Host
}

func originAllowed(origin string, allowed []string) bool {
	// Empty or "null" (Flutter desktop / native apps don't send a real origin)
	if origin == "" || origin == "null" {
		return true
	}
	for _, candidate := range allowed {
		if origin == candidate {
			return true
		}
	}
	return false
}

func wsOriginGuard(allowed []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if originAllowed(c.Get("Origin"), allowed) {
			return c.Next()
		}
		return fiber.NewError(fiber.StatusForbidden, "origin not allowed")
	}
}

func controllerWSGuard() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("client_role", "controller")
		return c.Next()
	}
}

func agentWSGuard() fiber.Handler {
	return func(c *fiber.Ctx) error {
		state := clientTLSState(c)
		if state == nil || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
			return fiber.NewError(fiber.StatusForbidden, "client certificate required")
		}
		sum := sha256.Sum256(state.PeerCertificates[0].Raw)
		c.Locals("client_role", "agent")
		c.Locals("client_cert_fingerprint", hex.EncodeToString(sum[:]))
		return c.Next()
	}
}

func hasVerifiedClientCert(c *fiber.Ctx) bool {
	state := clientTLSState(c)
	return state != nil && len(state.VerifiedChains) > 0 && len(state.PeerCertificates) > 0
}

func clientTLSState(c *fiber.Ctx) *tls.ConnectionState {
	return c.Context().TLSConnectionState()
}
