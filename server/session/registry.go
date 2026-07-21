package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sessionTTL       = 24 * time.Hour
	sessionRecordTTL = 30 * 24 * time.Hour
	sessionPrefix    = "session:"
	sessionIndex     = "session:index"
	agentPrefix      = "agent:"
	revokedCertIndex = "agent:client_cert:revoked"
	aliasPrefix      = "alias:" // alias:<lower-name> -> agent id (Phase 3)
)

// aliasPattern: 3-32 chars, letters/digits/hyphen, must start with a letter.
var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

// Status represents the current state of a session/agent registration.
type Status string

const (
	StatusWaiting    Status = "waiting"    // agent registered, no controller yet
	StatusConnecting Status = "connecting" // SDP exchange in progress
	StatusConnected  Status = "connected"  // active WebRTC session
	StatusOffline    Status = "offline"
)

// SessionStatus is the lifecycle state for a recorded remote session.
type SessionStatus string

const (
	SessionStatusConnecting SessionStatus = "connecting"
	SessionStatusActive     SessionStatus = "active"
	SessionStatusEnded      SessionStatus = "ended"
)

// AgentInfo is stored in Redis for every registered agent.
type AgentInfo struct {
	ID                    string    `json:"id"`
	Hostname              string    `json:"hostname"`
	OS                    string    `json:"os"`
	Version               string    `json:"version"`
	PasswordHash          string    `json:"password_hash"` // Argon2id hash from agent
	UnattendedHash        string    `json:"unattended_hash,omitempty"`
	ClientCertFingerprint string    `json:"client_cert_fingerprint,omitempty"`
	ClientCertRevoked     bool      `json:"client_cert_revoked,omitempty"`
	ClientCertRevokedAt   time.Time `json:"client_cert_revoked_at,omitempty"`
	OrgID                 string    `json:"org_id,omitempty"`
	DeviceGroup           string    `json:"device_group,omitempty"`
	Status                Status    `json:"status"`
	RegisteredAt          time.Time `json:"registered_at"`
	LastSeen              time.Time `json:"last_seen"`
	PublicAddr            string    `json:"public_addr"`
	SessionCount          int       `json:"session_count"`
	Alias                 string    `json:"alias,omitempty"` // human-readable name (Phase 3)
}

// SessionInfo is stored in Redis for every remote control session.
type SessionInfo struct {
	ID           string        `json:"id"`
	AgentID      string        `json:"agent_id"`
	ControllerID string        `json:"controller_id,omitempty"`
	TargetID     string        `json:"target_id,omitempty"`
	OrgID        string        `json:"org_id,omitempty"`
	DeviceGroup  string        `json:"device_group,omitempty"`
	Status       SessionStatus `json:"status"`
	StartedAt    time.Time     `json:"started_at"`
	EndedAt      time.Time     `json:"ended_at,omitempty"`
	LastSeen     time.Time     `json:"last_seen"`
	ControllerIP string        `json:"controller_ip,omitempty"`
	AgentIP      string        `json:"agent_ip,omitempty"`
}

// Registry manages agent registrations and session state via Redis.
type Registry struct {
	rdb *redis.Client
}

// NewRegistry creates a new Registry backed by the given Redis client.
func NewRegistry(rdb *redis.Client) *Registry {
	return &Registry{rdb: rdb}
}

// GenerateID creates a unique 9-digit agent ID formatted as "XXX-XXX-XXX".
func GenerateID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	n = n & 0x7FFFFFFF // ensure positive
	n = n % 1_000_000_000
	return fmt.Sprintf("%03d-%03d-%03d", n/1_000_000, (n/1000)%1000, n%1000), nil
}

// GenerateSessionID creates a unique session identifier.
func GenerateSessionID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sess-" + hex.EncodeToString(b), nil
}

// Register stores a new agent and returns its ID.
func (r *Registry) Register(ctx context.Context, info *AgentInfo) error {
	if info.RegisteredAt.IsZero() {
		info.RegisteredAt = time.Now()
	}
	info.LastSeen = time.Now()
	if info.Status == "" {
		info.Status = StatusWaiting
	}

	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	key := agentPrefix + info.ID
	return r.rdb.Set(ctx, key, data, sessionTTL).Err()
}

// Heartbeat refreshes the agent TTL and last-seen timestamp.
func (r *Registry) Heartbeat(ctx context.Context, agentID string) error {
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return err
	}
	info.LastSeen = time.Now()
	return r.Register(ctx, info)
}

// Get retrieves agent info by ID.
func (r *Registry) Get(ctx context.Context, agentID string) (*AgentInfo, error) {
	data, err := r.rdb.Get(ctx, agentPrefix+agentID).Bytes()
	if err != nil {
		return nil, err
	}
	var info AgentInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// SetStatus updates just the status field of an agent.
func (r *Registry) SetStatus(ctx context.Context, agentID string, status Status) error {
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return err
	}
	info.Status = status
	return r.Register(ctx, info)
}

// IncrementSessionCount increments the total session count for an agent.
func (r *Registry) IncrementSessionCount(ctx context.Context, agentID string) error {
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return err
	}
	info.SessionCount++
	return r.Register(ctx, info)
}

// SetClientCertFingerprint updates the active client certificate fingerprint for an agent.
func (r *Registry) SetClientCertFingerprint(ctx context.Context, agentID, fingerprint string) error {
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return err
	}
	info.ClientCertFingerprint = fingerprint
	info.ClientCertRevoked = false
	info.ClientCertRevokedAt = time.Time{}
	return r.Register(ctx, info)
}

// RevokeClientCert marks the active agent certificate as revoked.
func (r *Registry) RevokeClientCert(ctx context.Context, agentID string) (*AgentInfo, error) {
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(info.ClientCertFingerprint) != "" {
		if err := r.MarkClientCertRevokedFingerprint(ctx, info.ClientCertFingerprint); err != nil {
			return nil, err
		}
	}
	info.ClientCertRevoked = true
	info.ClientCertRevokedAt = time.Now().UTC()
	if err := r.Register(ctx, info); err != nil {
		return nil, err
	}
	return info, nil
}

// MarkClientCertRevokedFingerprint persists a revoked client certificate fingerprint.
func (r *Registry) MarkClientCertRevokedFingerprint(ctx context.Context, fingerprint string) error {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return nil
	}
	return r.rdb.SAdd(ctx, revokedCertIndex, fingerprint).Err()
}

// IsClientCertRevoked checks whether a client certificate fingerprint has been revoked.
func (r *Registry) IsClientCertRevoked(ctx context.Context, fingerprint string) (bool, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false, nil
	}
	exists, err := r.rdb.SIsMember(ctx, revokedCertIndex, fingerprint).Result()
	if err != nil {
		return false, err
	}
	return exists, nil
}

// Unregister removes an agent from the registry.
func (r *Registry) Unregister(ctx context.Context, agentID string) error {
	return r.rdb.Del(ctx, agentPrefix+agentID).Err()
}

// ---- Custom alias / namespace (roadmap Phase 3) --------------------------

// ErrAliasTaken / ErrAliasInvalid surface a precise reason to the client.
var (
	ErrAliasInvalid = errors.New("alias must be 3-32 chars: a letter, then letters, digits or hyphens")
	ErrAliasTaken   = errors.New("alias is already in use")
)

// NormalizeAlias lower-cases and trims; returns ErrAliasInvalid if malformed.
func NormalizeAlias(alias string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(alias))
	if !aliasPattern.MatchString(a) {
		return "", ErrAliasInvalid
	}
	return a, nil
}

// SetAlias binds [alias] to [agentID], enforcing global uniqueness. Frees the
// agent's previous alias. The alias key mirrors the agent TTL so a long-gone
// agent doesn't hold a name forever; Heartbeat/Register refresh both.
func (r *Registry) SetAlias(ctx context.Context, agentID, alias string) error {
	a, err := NormalizeAlias(alias)
	if err != nil {
		return err
	}
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return err
	}
	// Claim atomically: SET NX. If it exists and points elsewhere, it's taken.
	key := aliasPrefix + a
	ok, err := r.rdb.SetNX(ctx, key, agentID, sessionTTL).Result()
	if err != nil {
		return err
	}
	if !ok {
		owner, _ := r.rdb.Get(ctx, key).Result()
		if owner != agentID {
			return ErrAliasTaken
		}
		// Already ours — just refresh the TTL.
		_ = r.rdb.Expire(ctx, key, sessionTTL).Err()
	}
	// Release the old alias if it changed.
	if info.Alias != "" && info.Alias != a {
		_ = r.rdb.Del(ctx, aliasPrefix+info.Alias).Err()
	}
	info.Alias = a
	return r.Register(ctx, info)
}

// ClearAlias removes the agent's alias.
func (r *Registry) ClearAlias(ctx context.Context, agentID string) error {
	info, err := r.Get(ctx, agentID)
	if err != nil {
		return err
	}
	if info.Alias != "" {
		_ = r.rdb.Del(ctx, aliasPrefix+info.Alias).Err()
	}
	info.Alias = ""
	return r.Register(ctx, info)
}

// ResolveAlias returns the agent ID an alias points to, or "" if none. Refreshes
// the alias TTL on a hit so an actively-dialed name doesn't expire under load.
func (r *Registry) ResolveAlias(ctx context.Context, alias string) (string, error) {
	a, err := NormalizeAlias(alias)
	if err != nil {
		return "", err
	}
	id, err := r.rdb.Get(ctx, aliasPrefix+a).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_ = r.rdb.Expire(ctx, aliasPrefix+a, sessionTTL).Err()
	return id, nil
}

// List returns all registered agents.
func (r *Registry) List(ctx context.Context) ([]*AgentInfo, error) {
	keys, err := r.rdb.Keys(ctx, agentPrefix+"*").Result()
	if err != nil {
		return nil, err
	}
	result := make([]*AgentInfo, 0, len(keys))
	for _, k := range keys {
		data, err := r.rdb.Get(ctx, k).Bytes()
		if err != nil {
			continue
		}
		var info AgentInfo
		if err := json.Unmarshal(data, &info); err == nil {
			result = append(result, &info)
		}
	}
	return result, nil
}

// ListSessions returns the most recent n session records.
func (r *Registry) saveSession(ctx context.Context, info *SessionInfo) error {
	if info.StartedAt.IsZero() {
		info.StartedAt = time.Now()
	}
	info.LastSeen = time.Now()
	if info.Status == "" {
		info.Status = SessionStatusConnecting
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := r.rdb.Set(ctx, sessionPrefix+info.ID, data, sessionRecordTTL).Err(); err != nil {
		return err
	}
	return r.rdb.ZAdd(ctx, sessionIndex, redis.Z{
		Score:  float64(info.StartedAt.UnixNano()),
		Member: info.ID,
	}).Err()
}

// StartSession stores a new session record.
func (r *Registry) StartSession(ctx context.Context, info *SessionInfo) (*SessionInfo, error) {
	if info == nil {
		info = &SessionInfo{}
	}
	if info.ID == "" {
		id, err := GenerateSessionID()
		if err != nil {
			return nil, err
		}
		info.ID = id
	}
	if err := r.saveSession(ctx, info); err != nil {
		return nil, err
	}
	if info.AgentID != "" {
		_ = r.IncrementSessionCount(ctx, info.AgentID)
	}
	return info, nil
}

// GetSession retrieves a session record by ID.
func (r *Registry) GetSession(ctx context.Context, sessionID string) (*SessionInfo, error) {
	data, err := r.rdb.Get(ctx, sessionPrefix+sessionID).Bytes()
	if err != nil {
		return nil, err
	}
	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// SetSessionStatus updates the status of an existing session.
func (r *Registry) SetSessionStatus(ctx context.Context, sessionID string, status SessionStatus) error {
	info, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	info.Status = status
	if status == SessionStatusEnded && info.EndedAt.IsZero() {
		info.EndedAt = time.Now()
	}
	return r.saveSession(ctx, info)
}

// EndSession marks a session as ended.
func (r *Registry) EndSession(ctx context.Context, sessionID string) error {
	return r.SetSessionStatus(ctx, sessionID, SessionStatusEnded)
}

// ListSessions returns the most recent n session records.
func (r *Registry) ListSessions(ctx context.Context, n int) ([]*SessionInfo, error) {
	if n <= 0 {
		n = 100
	}
	keys, err := r.rdb.ZRevRange(ctx, sessionIndex, 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}
	result := make([]*SessionInfo, 0, len(keys))
	for _, id := range keys {
		data, err := r.rdb.Get(ctx, sessionPrefix+id).Bytes()
		if err != nil {
			continue
		}
		var info SessionInfo
		if err := json.Unmarshal(data, &info); err == nil {
			result = append(result, &info)
		}
	}
	return result, nil
}
