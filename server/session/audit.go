package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	auditTTL    = 90 * 24 * time.Hour
	auditPrefix = "audit:"
	auditIndex  = "audit:index"
)

// AuditEvent records a notable action in the control plane.
type AuditEvent struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Actor     string         `json:"actor,omitempty"`
	Target    string         `json:"target,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Outcome   string         `json:"outcome,omitempty"`
	IP        string         `json:"ip,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

func generateAuditID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "audit-" + hex.EncodeToString(b), nil
}

// AddAuditEvent stores a new audit event with retention.
func (r *Registry) AddAuditEvent(ctx context.Context, event *AuditEvent) error {
	if event == nil {
		return nil
	}
	if event.ID == "" {
		id, err := generateAuditID()
		if err != nil {
			return err
		}
		event.ID = id
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := r.rdb.Set(ctx, auditPrefix+event.ID, data, auditTTL).Err(); err != nil {
		return err
	}
	return r.rdb.ZAdd(ctx, auditIndex, redis.Z{
		Score:  float64(event.CreatedAt.UnixNano()),
		Member: event.ID,
	}).Err()
}

// ListAuditEvents returns the newest audit events.
func (r *Registry) ListAuditEvents(ctx context.Context, n int) ([]*AuditEvent, error) {
	if n <= 0 {
		n = 100
	}
	ids, err := r.rdb.ZRevRange(ctx, auditIndex, 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}
	result := make([]*AuditEvent, 0, len(ids))
	for _, id := range ids {
		data, err := r.rdb.Get(ctx, auditPrefix+id).Bytes()
		if err != nil {
			continue
		}
		var event AuditEvent
		if err := json.Unmarshal(data, &event); err == nil {
			result = append(result, &event)
		}
	}
	return result, nil
}
