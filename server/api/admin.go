package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	serverauth "github.com/neev/remote-agent/server/auth"
	"github.com/neev/remote-agent/server/session"
)

type userUpsertRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type mfaSetupRequest struct {
	CurrentPassword string `json:"current_password"`
}

type mfaConfirmRequest struct {
	Secret  string `json:"secret"`
	OTPCode string `json:"otp_code"`
}

func (s *Server) createUser(c *fiber.Ctx) error {
	if s.authStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "auth store unavailable"})
	}
	var req userUpsertRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}
	if req.Role == "" {
		req.Role = serverauth.RoleViewer
	}
	user, err := s.authStore.CreateUser(c.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "auth.user.create",
		Actor:   authActor(c),
		Target:  user.Email,
		Outcome: "success",
		IP:      c.IP(),
		Details: map[string]any{"role": user.Role},
	})
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"user": toAuthUserResponse(user)})
}

func (s *Server) updateUser(c *fiber.Ctx) error {
	if s.authStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "auth store unavailable"})
	}
	email := strings.TrimSpace(c.Params("email"))
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	var req userUpsertRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if strings.TrimSpace(req.Password) != "" {
		if err := s.authStore.SetPassword(c.Context(), email, req.Password); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if strings.TrimSpace(req.Role) != "" {
		if err := s.authStore.SetRole(c.Context(), email, strings.ToLower(strings.TrimSpace(req.Role))); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}
	user, err := s.authStore.GetUser(c.Context(), email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "auth.user.update",
		Actor:   authActor(c),
		Target:  user.Email,
		Outcome: "success",
		IP:      c.IP(),
		Details: map[string]any{"role": user.Role},
	})
	return c.JSON(fiber.Map{"user": toAuthUserResponse(user)})
}

func (s *Server) deleteUser(c *fiber.Ctx) error {
	if s.authStore == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "auth store unavailable"})
	}
	currentUser, _, err := s.currentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	email := strings.TrimSpace(c.Params("email"))
	if email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	if strings.EqualFold(email, currentUser.Email) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot delete your own account"})
	}
	if err := s.authStore.DeleteUser(c.Context(), email); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "auth.user.delete",
		Actor:   authActor(c),
		Target:  email,
		Outcome: "success",
		IP:      c.IP(),
	})
	return c.JSON(fiber.Map{"deleted": true})
}

func (s *Server) setupMFA(c *fiber.Ctx) error {
	if !s.authEnabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "authentication is disabled"})
	}
	user, _, err := s.currentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req mfaSetupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if strings.TrimSpace(req.CurrentPassword) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "current password is required"})
	}
	if _, err := s.authStore.Authenticate(c.Context(), user.Email, req.CurrentPassword, ""); err != nil && !errors.Is(err, serverauth.ErrMFARequired) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	secret, err := serverauth.GenerateTOTPSecret()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate secret"})
	}
	uri := serverauth.TOTPProvisioningURI("RemoteAgent", user.Email, secret)
	return c.JSON(fiber.Map{
		"secret":       secret,
		"otpauth_uri":  uri,
		"issuer":       "RemoteAgent",
		"account_name": user.Email,
	})
}

func (s *Server) confirmMFA(c *fiber.Ctx) error {
	if !s.authEnabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "authentication is disabled"})
	}
	user, _, err := s.currentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req mfaConfirmRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	if strings.TrimSpace(req.Secret) == "" || strings.TrimSpace(req.OTPCode) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "secret and otp_code are required"})
	}
	if !serverauth.VerifyTOTP(req.Secret, req.OTPCode, time.Now()) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid otp code"})
	}
	if err := s.authStore.SetTOTPSecret(c.Context(), user.Email, req.Secret); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	updated, err := s.authStore.GetUser(c.Context(), user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "auth.mfa.enable",
		Actor:   user.Email,
		Outcome: "success",
		IP:      c.IP(),
	})
	return c.JSON(fiber.Map{"user": toAuthUserResponse(updated)})
}

func (s *Server) disableMFA(c *fiber.Ctx) error {
	if !s.authEnabled() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "authentication is disabled"})
	}
	user, _, err := s.currentUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if err := s.authStore.ClearTOTPSecret(c.Context(), user.Email); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	updated, err := s.authStore.GetUser(c.Context(), user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "auth.mfa.disable",
		Actor:   user.Email,
		Outcome: "success",
		IP:      c.IP(),
	})
	return c.JSON(fiber.Map{"user": toAuthUserResponse(updated)})
}

func (s *Server) reissueAgentCertificate(c *fiber.Ctx) error {
	if s.hub == nil || !s.hub.ManagedDeviceTrust() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "managed device trust is not configured"})
	}
	agentID := strings.TrimSpace(c.Params("id"))
	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent id is required"})
	}
	bundle, err := s.hub.ReissueClientCertificate(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	agent, err := s.registry.Get(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "agent.cert.reissue.request",
		Actor:   authActor(c),
		Target:  agentID,
		Outcome: "success",
		IP:      c.IP(),
	})
	return c.JSON(fiber.Map{
		"action":      "reissue",
		"agent":       toAgentResponse(agent),
		"client_cert": bundle,
	})
}

func (s *Server) revokeAgentCertificate(c *fiber.Ctx) error {
	if s.hub == nil || !s.hub.ManagedDeviceTrust() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "managed device trust is not configured"})
	}
	agentID := strings.TrimSpace(c.Params("id"))
	if agentID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "agent id is required"})
	}
	agent, err := s.hub.RevokeClientCertificate(c.Context(), agentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	_ = s.registry.AddAuditEvent(c.Context(), &session.AuditEvent{
		Type:    "agent.cert.revoke.request",
		Actor:   authActor(c),
		Target:  agentID,
		Outcome: "success",
		IP:      c.IP(),
	})
	return c.JSON(fiber.Map{
		"action": "revoke",
		"agent":  toAgentResponse(agent),
	})
}

func authActor(c *fiber.Ctx) string {
	if value, ok := c.Locals("auth_user").(*serverauth.User); ok && value != nil {
		return value.Email
	}
	return c.IP()
}
