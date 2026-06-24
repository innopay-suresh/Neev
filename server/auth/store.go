package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	agentauth "github.com/neev/remote-agent/agent/auth"
	"github.com/neev/remote-agent/server/config"
)

const (
	userPrefix = "auth:user:"
	userIndex  = "auth:users"
)

const (
	RoleAdmin   = "admin"
	RoleSupport = "support"
	RoleViewer  = "viewer"
)

// User represents an authenticated dashboard account.
type User struct {
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
	TOTPSecret   string    `json:"totp_secret,omitempty"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Store persists dashboard users in Redis.
type Store struct {
	rdb *redis.Client
}

// NewStore creates a new auth store.
func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// EnsureBootstrapUser creates the initial dashboard account if needed.
func (s *Store) EnsureBootstrapUser(ctx context.Context, cfg config.AuthConfig) error {
	if !cfg.Enabled {
		return nil
	}
	email := normalizeEmail(cfg.BootstrapEmail)
	if email == "" {
		return fmt.Errorf("auth is enabled but bootstrap_email is not configured")
	}
	if _, err := s.GetUser(ctx, email); err == nil {
		return nil
	}
	hash := strings.TrimSpace(cfg.BootstrapPasswordHash)
	if hash == "" && cfg.BootstrapPassword != "" {
		derived, err := agentauth.HashPassword(cfg.BootstrapPassword)
		if err != nil {
			return err
		}
		hash = derived
	}
	if hash == "" {
		return fmt.Errorf("bootstrap password hash or password is required when auth is enabled")
	}
	role := strings.ToLower(strings.TrimSpace(cfg.BootstrapRole))
	if role == "" {
		role = RoleAdmin
	}
	return s.UpsertUser(ctx, &User{
		Email:        email,
		PasswordHash: hash,
		Role:         role,
	})
}

// UpsertUser stores or updates a dashboard user.
func (s *Store) UpsertUser(ctx context.Context, user *User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	user.Email = normalizeEmail(user.Email)
	if user.Email == "" {
		return fmt.Errorf("email is required")
	}
	if user.Role == "" {
		user.Role = RoleViewer
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, userPrefix+user.Email, data, 0).Err(); err != nil {
		return err
	}
	return s.rdb.SAdd(ctx, userIndex, user.Email).Err()
}

// CreateUser creates a new dashboard user.
func (s *Store) CreateUser(ctx context.Context, email, password, role string) (*User, error) {
	hash, err := agentauth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	user := &User{
		Email:        email,
		PasswordHash: hash,
		Role:         role,
	}
	if err := s.UpsertUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// DeleteUser removes a dashboard user.
func (s *Store) DeleteUser(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if err := s.rdb.Del(ctx, userPrefix+email).Err(); err != nil {
		return err
	}
	return s.rdb.SRem(ctx, userIndex, email).Err()
}

// SetPassword updates a user's password.
func (s *Store) SetPassword(ctx context.Context, email, password string) error {
	user, err := s.GetUser(ctx, email)
	if err != nil {
		return err
	}
	hash, err := agentauth.HashPassword(password)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	return s.UpsertUser(ctx, user)
}

// SetRole updates a user's role.
func (s *Store) SetRole(ctx context.Context, email, role string) error {
	user, err := s.GetUser(ctx, email)
	if err != nil {
		return err
	}
	user.Role = role
	return s.UpsertUser(ctx, user)
}

// SetTOTPSecret enables MFA for the given user.
func (s *Store) SetTOTPSecret(ctx context.Context, email, secret string) error {
	user, err := s.GetUser(ctx, email)
	if err != nil {
		return err
	}
	user.TOTPSecret = secret
	return s.UpsertUser(ctx, user)
}

// ClearTOTPSecret disables MFA for the user.
func (s *Store) ClearTOTPSecret(ctx context.Context, email string) error {
	user, err := s.GetUser(ctx, email)
	if err != nil {
		return err
	}
	user.TOTPSecret = ""
	return s.UpsertUser(ctx, user)
}

// GenerateTOTPSecret creates a random base32 secret.
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32NoPadEncode(buf), nil
}

// TOTPProvisioningURI returns an otpauth URI for QR enrollment.
func TOTPProvisioningURI(issuer, accountName, secret string) string {
	issuer = strings.TrimSpace(issuer)
	accountName = strings.TrimSpace(accountName)
	label := accountName
	if issuer != "" {
		label = issuer + ":" + accountName
	}
	params := urlValues{
		"secret":    secret,
		"issuer":    issuer,
		"algorithm": "SHA1",
		"digits":    "6",
		"period":    "30",
	}
	return "otpauth://totp/" + urlEscape(label) + "?" + params.Encode()
}

// SetPasswordAndRole updates multiple fields at once.
func (s *Store) SetPasswordAndRole(ctx context.Context, email, password, role string) error {
	user, err := s.GetUser(ctx, email)
	if err != nil {
		return err
	}
	hash, err := agentauth.HashPassword(password)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	user.Role = role
	return s.UpsertUser(ctx, user)
}

// GetUser fetches a dashboard user by email.
func (s *Store) GetUser(ctx context.Context, email string) (*User, error) {
	data, err := s.rdb.Get(ctx, userPrefix+normalizeEmail(email)).Bytes()
	if err != nil {
		return nil, err
	}
	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// ListUsers returns all dashboard users.
func (s *Store) ListUsers(ctx context.Context) ([]*User, error) {
	emails, err := s.rdb.SMembers(ctx, userIndex).Result()
	if err != nil {
		return nil, err
	}
	result := make([]*User, 0, len(emails))
	for _, email := range emails {
		user, err := s.GetUser(ctx, email)
		if err == nil {
			result = append(result, user)
		}
	}
	return result, nil
}

// Authenticate validates credentials and returns the matched user.
func (s *Store) Authenticate(ctx context.Context, email, password, otpCode string) (*User, error) {
	user, err := s.GetUser(ctx, email)
	if err != nil {
		return nil, err
	}
	ok, err := agentauth.VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		return nil, fmt.Errorf("invalid credentials")
	}
	if strings.TrimSpace(user.TOTPSecret) != "" {
		if strings.TrimSpace(otpCode) == "" {
			return nil, ErrMFARequired
		}
		if !VerifyTOTP(user.TOTPSecret, otpCode, time.Now()) {
			return nil, fmt.Errorf("invalid mfa code")
		}
	}
	return user, nil
}

var ErrMFARequired = fmt.Errorf("mfa required")

// --- minimal local helpers to avoid extra dependencies ---

type urlValues map[string]string

func (v urlValues) Encode() string {
	keys := make([]string, 0, len(v))
	for key := range v {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, key := range keys {
		if v[key] == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(urlEscape(key))
		b.WriteByte('=')
		b.WriteString(urlEscape(v[key]))
		if i == len(keys)-1 {
			_ = i
		}
	}
	return b.String()
}

func urlEscape(value string) string {
	replacer := strings.NewReplacer(" ", "%20", ":", "%3A", "/", "%2F", "?", "%3F", "&", "%26", "=", "%3D")
	return replacer.Replace(value)
}
