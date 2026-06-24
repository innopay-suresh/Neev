package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	serverauth "github.com/neev/remote-agent/server/auth"
	"github.com/neev/remote-agent/server/session"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OTPCode  string `json:"otp_code,omitempty"`
}

type authUserResponse struct {
	Email      string    `json:"email"`
	Role       string    `json:"role"`
	MFAEnabled bool      `json:"mfa_enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func toAuthUserResponse(user *serverauth.User) *authUserResponse {
	if user == nil {
		return nil
	}
	return &authUserResponse{
		Email:      user.Email,
		Role:       user.Role,
		MFAEnabled: strings.TrimSpace(user.TOTPSecret) != "",
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
	}
}

func (s *Server) authEnabled() bool {
	return s.authStore != nil && s.cfg != nil && s.cfg.Auth.Enabled && s.cfg.JWT.Secret != ""
}

func (s *Server) tokenExpiry() time.Duration {
	hours := s.cfg.JWT.ExpiryHours
	if hours <= 0 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

func bearerToken(c *fiber.Ctx) string {
	header := c.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func (s *Server) currentUser(c *fiber.Ctx) (*serverauth.User, *serverauth.Claims, error) {
	raw := bearerToken(c)
	if raw == "" {
		return nil, nil, errors.New("missing bearer token")
	}
	claims, err := serverauth.ParseToken(s.cfg.JWT.Secret, raw)
	if err != nil {
		return nil, nil, err
	}
	if s.authStore == nil {
		return nil, nil, errors.New("auth store unavailable")
	}
	user, err := s.authStore.GetUser(c.Context(), claims.Sub)
	if err != nil {
		return nil, nil, err
	}
	if !serverauth.RoleAllows(user.Role, claims.Role) {
		return nil, nil, errors.New("role mismatch")
	}
	return user, claims, nil
}

func (s *Server) requireAuth() fiber.Handler {
	return s.requireRole(serverauth.RoleViewer)
}

func (s *Server) requireRole(required string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !s.authEnabled() {
			return c.Next()
		}
		user, claims, err := s.currentUser(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		if !serverauth.RoleAllows(user.Role, required) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		c.Locals("auth_user", user)
		c.Locals("auth_claims", claims)
		return c.Next()
	}
}

func (s *Server) login(c *fiber.Ctx) error {
	if !s.authEnabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "authentication is disabled"})
	}
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}

	user, err := s.authStore.Authenticate(c.Context(), req.Email, req.Password, req.OTPCode)
	if err != nil {
		if errors.Is(err, serverauth.ErrMFARequired) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":        "mfa required",
				"mfa_required": true,
			})
		}
		_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
			Type:    "auth.login",
			Actor:   req.Email,
			Outcome: "denied",
			IP:      c.IP(),
			Details: map[string]any{"reason": "invalid_credentials"},
		})
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}

	token, err := serverauth.GenerateToken(s.cfg.JWT.Secret, user, s.tokenExpiry())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "token generation failed"})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "auth.login",
		Actor:   user.Email,
		Outcome: "success",
		IP:      c.IP(),
		Details: map[string]any{"role": user.Role},
	})

	return c.JSON(fiber.Map{
		"token":      token,
		"token_type": "Bearer",
		"expires_in": int64(s.tokenExpiry() / time.Second),
		"user":       toAuthUserResponse(user),
	})
}

func (s *Server) me(c *fiber.Ctx) error {
	if !s.authEnabled() {
		return c.JSON(fiber.Map{"enabled": false, "user": nil})
	}
	user, _, err := s.currentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	return c.JSON(fiber.Map{"enabled": true, "user": toAuthUserResponse(user)})
}

func (s *Server) listUsers(c *fiber.Ctx) error {
	if s.authStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "auth store unavailable"})
	}
	users, err := s.authStore.ListUsers(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	result := make([]*authUserResponse, 0, len(users))
	for _, user := range users {
		result = append(result, toAuthUserResponse(user))
	}
	return c.JSON(fiber.Map{"users": result, "total": len(result)})
}
