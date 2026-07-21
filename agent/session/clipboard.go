package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
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
	conn *ipc.Conn
	mu   sync.Mutex
	last string // last text seen/set, to break the copy echo loop

	// Inbound image reassembly (viewer→host, chunked base64 PNG).
	imgParts []string
	imgNext  int
	imgTotal int
	// Echo guard for images: skip re-sending an image we just applied, and skip
	// re-reading the (large) clipboard image unless the clipboard changed.
	lastSeq     uint32
	lastImgHash uint64
}

// hashBytes is a cheap FNV-1a change-detector for clipboard images.
func hashBytes(b []byte) uint64 {
	var h uint64 = 1469598103934665603
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

func newClipSync(conn *ipc.Conn) *clipSync {
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
		I   int    `json:"i"`
		N   int    `json:"n"`
		D   string `json:"d"`
	}
	if err := json.Unmarshal(payload, &m); err != nil || m.K != "clip" {
		return false
	}
	if m.Img != 0 {
		c.recvImageChunk(m.I, m.N, m.D)
		return true
	}
	c.mu.Lock()
	c.last = m.T
	c.mu.Unlock()
	if err := clipboard.WriteAll(m.T); err != nil {
		log.Warn().Err(err).Msg("worker: set clipboard failed")
	}
	return true
}

// recvImageChunk accumulates the viewer's chunked image ({"k":"clip","img":1,
// "i","n","d"}) and, on the last in-order chunk, decodes the PNG and writes it
// to the host clipboard. Out-of-order/stale chunks reset the buffer.
func (c *clipSync) recvImageChunk(i, n int, d string) {
	c.mu.Lock()
	if i == 0 {
		c.imgParts = make([]string, n)
		c.imgTotal = n
		c.imgNext = 0
		log.Info().Int("chunks", n).Msg("worker: receiving clipboard image from viewer")
	}
	if c.imgTotal == 0 || n != c.imgTotal || i != c.imgNext || i >= len(c.imgParts) {
		c.imgParts, c.imgTotal, c.imgNext = nil, 0, 0
		c.mu.Unlock()
		return
	}
	c.imgParts[i] = d
	c.imgNext++
	done := c.imgNext == c.imgTotal
	var b64 string
	if done {
		b64 = strings.Join(c.imgParts, "")
		c.imgParts, c.imgTotal, c.imgNext = nil, 0, 0
	}
	c.mu.Unlock()
	if !done {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		log.Warn().Err(err).Msg("worker: decode clipboard image failed")
		return
	}
	c.mu.Lock()
	c.lastImgHash = hashBytes(raw) // don't echo it back to the viewer
	c.mu.Unlock()
	if err := writeClipboardImagePNG(raw); err != nil {
		log.Warn().Err(err).Msg("worker: set clipboard image failed")
		return
	}
	log.Info().Int("bytes", len(raw)).Msg("worker: applied viewer clipboard image")
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
		// Image clipboard (host→viewer): re-read the bitmap when the clipboard
		// changed (sequence number) and send when the image content changed
		// (hash, echo-guarded). If the sequence number is unavailable (0), fall
		// back to always checking so image sync still works.
		seq := clipboardSeq()
		c.mu.Lock()
		seqChanged := seq == 0 || seq != c.lastSeq
		c.lastSeq = seq
		c.mu.Unlock()
		if seqChanged {
			if img, ok := readClipboardImagePNG(); ok {
				h := hashBytes(img)
				c.mu.Lock()
				imgChanged := h != c.lastImgHash
				if imgChanged {
					c.lastImgHash = h
				}
				c.mu.Unlock()
				if imgChanged {
					if err := c.conn.WriteMessage(ipc.KindClipboardImage, img); err != nil {
						return
					}
					log.Info().Int("bytes", len(img)).Msg("worker: sent host clipboard image to viewer")
					continue // don't also emit text this tick
				}
			}
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
		if err := c.conn.WriteMessage(ipc.KindClipboard, []byte(cur)); err != nil {
			return // transport gone; worker will exit/respawn
		}
	}
}
