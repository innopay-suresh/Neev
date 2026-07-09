package session

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/ipc"
)

// clipSync carries TEXT clipboard both ways in TransportMode, where the Flutter
// app no longer hosts (so its data-channel clipboard path is gone). It runs in
// the capture worker — which is the logged-in user, so it CAN read/write that
// user's clipboard. Viewer→host arrives inline on the control channel
// ({"k":"clip","t":...}); host→viewer is polled and pushed to the transport,
// which relays it to viewers. File clipboard is unaffected (helper clipagent);
// image clipboard is not carried here yet.
type clipSync struct {
	conn net.Conn
	mu   sync.Mutex
	last string // last text seen/set, to break the copy echo loop
}

func newClipSync(conn net.Conn) *clipSync {
	c := &clipSync{conn: conn}
	// Prime with the current clipboard so we don't immediately re-broadcast it.
	if s, err := clipboard.ReadAll(); err == nil {
		c.last = s
	}
	return c
}

// handleInbound applies a viewer clipboard message to the host clipboard.
// Returns true if the payload WAS a clipboard message (so the caller does not
// also treat it as input). Non-clipboard payloads return false.
func (c *clipSync) handleInbound(payload []byte) bool {
	var m struct {
		K   string `json:"k"`
		T   string `json:"t"`
		Img int    `json:"img"`
	}
	if err := json.Unmarshal(payload, &m); err != nil || m.K != "clip" {
		return false
	}
	if m.Img != 0 {
		return true // image clipboard not carried over the transport yet — drop
	}
	c.mu.Lock()
	c.last = m.T
	c.mu.Unlock()
	if err := clipboard.WriteAll(m.T); err != nil {
		log.Warn().Err(err).Msg("worker: set clipboard failed")
	}
	return true
}

// poll watches the host clipboard and pushes text changes to the transport
// until ctx is cancelled or the transport connection drops. Echo-guarded via
// last so a value we just applied from the viewer isn't sent straight back.
func (c *clipSync) poll(ctx context.Context) {
	t := time.NewTicker(600 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur, err := clipboard.ReadAll()
		if err != nil || cur == "" {
			continue
		}
		c.mu.Lock()
		changed := cur != c.last
		if changed {
			c.last = cur
		}
		c.mu.Unlock()
		if !changed {
			continue
		}
		if err := ipc.WriteMessage(c.conn, ipc.KindClipboard, []byte(cur)); err != nil {
			return // transport gone; worker will exit/respawn
		}
	}
}
