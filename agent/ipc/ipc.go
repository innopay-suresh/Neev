// Package ipc defines the local (loopback TCP) protocol between the persistent
// transport process and a per-session capture worker.
//
// Architecture (Phase 0 of the SYSTEM-service transport):
//   - The TRANSPORT runs persistently (session 0, under the SYSTEM service). It
//     owns the WebRTC peer connection + signaling and never dies across a user
//     switch.
//   - A per-session CAPTURE WORKER is spawned by the service into the active
//     desktop session (CreateProcessAsUser). It captures + VP8-encodes frames
//     and streams them to the transport over this IPC. On a user switch the
//     service swaps the worker; the transport's connection is untouched, so the
//     viewer never disconnects.
//
// Wire format (both directions): [uint32 LE length][uint8 kind][payload].
// length = 1 (kind) + len(payload).
package ipc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// DefaultPort is the loopback port the transport listens on for its worker.
const DefaultPort = 47930

// Message kinds.
const (
	// Worker -> transport: an encoded VP8 frame.
	// payload = [uint8 keyframe(0/1)][VP8 bitstream bytes].
	KindVideoFrame byte = 0x01

	// Transport -> worker: request a keyframe on the next frame (from a viewer
	// PLI/FIR). payload = empty.
	KindKeyframeReq byte = 0x02

	// Worker -> transport: capture metadata (width,height). payload =
	// [uint32 LE width][uint32 LE height]. Sent when the resolution changes.
	KindVideoInfo byte = 0x03

	// Either direction: liveness ping. payload = empty.
	KindPing byte = 0x04

	// Transport -> worker: a viewer input event to inject into the worker's
	// session. payload = the raw viewer control-channel JSON, e.g.
	// {"k":"mv","x":..,"y":..} / {"k":"btn",..} / {"k":"whl",..} /
	// {"k":"key","u":hidUsage,"d":bool}. The worker parses + injects it in the
	// active session, so mouse/keyboard control survives a user switch (the
	// transport keeps the WebRTC connection; only the worker is swapped).
	KindInput byte = 0x05

	// Worker -> transport: the host's clipboard text changed; the transport
	// relays it to viewers on the control channel as {"k":"clip","t":...}. Carries
	// host→viewer copy-paste in TransportMode (where the app no longer hosts).
	// payload = UTF-8 clipboard text. Viewer→host clipboard rides KindInput (the
	// viewer's {"k":"clip",...} arrives on the control channel like other input).
	KindClipboard byte = 0x06

	// Worker -> transport: the host's clipboard IMAGE changed. payload = the raw
	// PNG bytes. The transport base64-chunks it into {"k":"clip","img":1,...}
	// control-channel messages for viewers (matching the Flutter host format).
	// Viewer→host image chunks ride KindInput like other control messages and are
	// reassembled by the worker.
	KindClipboardImage byte = 0x07

	// Transport -> worker: a viewer file-transfer message from the dedicated
	// 'file' data channel ({"k":"ft","t":"offer|data|end|cancel",...}). The
	// worker writes received files to the logged-in user's Downloads folder.
	KindFileData byte = 0x08

	// Worker -> transport: a host chat reply typed into the worker's chat window.
	// payload = {"k":"chat","t":...} JSON; the transport relays it to viewers on
	// the control channel.
	KindChat byte = 0x09

	// Transport -> worker: ask the logged-in user to approve an incoming viewer
	// (the "Ask before allowing connections" gate in TransportMode, where the
	// headless session-0 transport can't draw UI). payload = viewer id (string).
	KindConsentRequest byte = 0x0A

	// Worker -> transport: the user's Accept/Deny answer. payload =
	// {"id":<viewer id>,"allow":bool,"control":bool} JSON.
	KindConsentReply byte = 0x0B

	// Worker -> transport: the HOST user wants to end the session now. Until
	// this existed, only the viewer could hang up: the person whose screen was
	// being watched had no way to stop it, which is the wrong way round for a
	// remote-access tool. payload = viewer id to drop, or empty for ALL viewers.
	KindEndSession byte = 0x0C

	// Transport -> worker: a viewer connected or the last one left, so the
	// worker can show/hide the host's session indicator. payload = viewer count
	// as decimal text ("0" = nobody connected).
	KindSessionState byte = 0x0D

	// Transport -> worker: withdraw a consent prompt that is still on screen.
	// The viewer cancelled, disconnected, or the request timed out, so the
	// question is moot — leaving the dialog up asks the host to decide something
	// that no longer exists, and it sat there until dismissed by hand.
	// payload = viewer id.
	KindConsentCancel byte = 0x0E

	// Worker -> transport: one 20 ms frame of the host's microphone, already
	// mu-law encoded (see agent/audio). payload = 160 bytes of PCMU.
	//
	// Capture lives in the WORKER, not here, and that placement is the whole
	// reason voice can work at all under TransportMode: the worker is launched
	// into the interactive session via CreateProcessAsUser, so it has an audio
	// session and can see the user's microphone. The transport runs as SYSTEM
	// in session 0, which has no audio endpoint of its own.
	KindAudioFrame byte = 0x0F

	// Transport -> worker: one 20 ms frame of the VIEWER's voice to play out on
	// the host's speakers. payload = 160 bytes of PCMU.
	KindAudioPlay byte = 0x10

	// Transport -> worker: turn host microphone capture on or off.
	// payload = "1" to capture, "0" to stop.
	//
	// Capture must be OFF until asked. A remote-support tool that opens the
	// microphone on connect is indistinguishable from a bug that listens to the
	// room, and the OS mic indicator lighting up unbidden is how that accusation
	// starts. The device is opened on "1" and CLOSED on "0" — not merely muted.
	KindAudioCapture byte = 0x11

	// Transport -> worker: the id + password this machine is registered under,
	// so the app can SHOW them. payload = {"id":string,"password":string} JSON.
	//
	// The app cannot read them from disk. The transport's own transport.txt is
	// root-owned 0600 (it holds the password), and the app runs as the
	// logged-in user, so on macOS the share card rendered an empty id and the
	// host had nothing to hand out — the daemon was hosting correctly and
	// unreachable in practice. Windows never hit this because its app asks the
	// SYSTEM helper over localhost TCP; this gives macOS the same answer over
	// the channel that already exists.
	//
	// It goes to the WORKER rather than to a shared file because the worker
	// runs as the logged-in user: it can write the credentials somewhere only
	// that user can read. Widening transport.txt instead would hand this
	// machine's remote-access password to every other local account.
	KindHostCreds byte = 0x12
)

// maxPayload caps a single message so a corrupt stream can't allocate wildly.
// A 4K keyframe is comfortably under this.
const maxPayload = 32 << 20 // 32 MiB

// ErrConnClosed is returned by the write methods after the Conn is closed.
var ErrConnClosed = fmt.Errorf("ipc: connection closed")

// Conn wraps the single transport↔worker net.Conn. All writes go through a
// SINGLE dedicated writer goroutine draining THREE priority queues, so:
//   - no producer ever holds a lock across a blocking socket write (the r69
//     mutex did — a large file's blocked write starved input and deadlocked the
//     bidirectional pipe);
//   - one message's [header][payload] is written by exactly one goroutine, so
//     the frame stream can never interleave/corrupt (the reason r69 serialized);
//   - INPUT/control always beat bulk file data to the wire (hi > bulk), so a
//     large transfer can't head-of-line-block clicks;
//   - VIDEO is droppable (drop-oldest) so a slow peer can never stall capture;
//   - BULK file data is a bounded queue, so a producer blocks on enqueue when
//     the peer is behind — that is real backpressure to the file sender and it
//     never touches the hi lane.
//
// One reader per direction, so ReadMessage stays lock-free.
type Conn struct {
	net.Conn
	hi        chan []byte // reliable, high priority: input/control/acks/clip/chat/keyframe/video-info
	bulk      chan []byte // reliable, bulk: file-transfer + clipboard-file bytes (backpressured)
	vid       chan []byte // droppable: video frames (drop-oldest when the peer is behind)
	done      chan struct{}
	closeOnce sync.Once
}

// NewConn wraps c and starts its writer goroutine.
func NewConn(c net.Conn) *Conn {
	cn := &Conn{
		Conn: c,
		hi:   make(chan []byte, 1024),
		bulk: make(chan []byte, 256),
		vid:  make(chan []byte, 8),
		done: make(chan struct{}),
	}
	go cn.writeLoop()
	return cn
}

func (c *Conn) writeLoop() {
	hiStreak := 0
	for {
		var b []byte
		fromHi := false
		// FAIRNESS: after a run of hi-lane messages, force a bulk slot. hi (input +
		// keyframe requests) can otherwise flood — e.g. a PLI storm or rapid mouse
		// moves during a big transfer — and STARVE the bulk file lane, wedging the
		// upload (the reported "large file stalls at ~8MB"). This still gives hi
		// ~8:1 priority (input stays snappy) while guaranteeing file data always
		// makes progress.
		if hiStreak >= 8 {
			select {
			case b = <-c.bulk:
				hiStreak = 0
			case <-c.done:
				return
			default:
			}
		}
		// Prefer the hi lane: drain it first (non-blocking) so input/acks are never
		// stuck behind bulk file data.
		if b == nil {
			select {
			case b = <-c.hi:
				fromHi = true
			case <-c.done:
				return
			default:
				select {
				case b = <-c.hi:
					fromHi = true
				case b = <-c.bulk:
				case b = <-c.vid:
				case <-c.done:
					return
				}
			}
		}
		if fromHi {
			hiStreak++
		} else {
			hiStreak = 0
		}
		if _, err := c.Conn.Write(b); err != nil {
			// CRITICAL: on a write error the writer is done, but producers are
			// blocked on `case c.hi <- b` / `c.bulk <- b`. Close `done` so every
			// pending and future Write* returns ErrConnClosed instead of hanging
			// forever (a single transient error otherwise permanently wedges
			// clipboard, chat, and file acks until the process restarts). The dead
			// conn then surfaces to the reader, which drives reconnect/teardown.
			c.closeOnce.Do(func() { close(c.done) })
			return
		}
	}
}

func frame(kind byte, payload []byte) ([]byte, error) {
	if len(payload) > maxPayload {
		return nil, fmt.Errorf("ipc: payload too large (%d)", len(payload))
	}
	b := make([]byte, 5+len(payload))
	binary.LittleEndian.PutUint32(b[0:4], uint32(1+len(payload)))
	b[4] = kind
	copy(b[5:], payload)
	return b, nil
}

// WriteMessage enqueues a reliable, HIGH-priority message (input, control,
// acks, clipboard control, chat, keyframe req, video info). Blocks only if the
// hi queue is full (rare — it's low volume), never across the socket write.
func (c *Conn) WriteMessage(kind byte, payload []byte) error {
	b, err := frame(kind, payload)
	if err != nil {
		return err
	}
	select {
	case c.hi <- b:
		return nil
	case <-c.done:
		return ErrConnClosed
	}
}

// WriteBulk enqueues reliable BULK data (file-transfer / clipboard-file bytes).
// Blocks on enqueue when the peer is behind — that is the backpressure that
// paces the sender; it never blocks the hi lane and never holds a lock across
// the socket write, so a huge file can't deadlock input/capture.
func (c *Conn) WriteBulk(kind byte, payload []byte) error {
	b, err := frame(kind, payload)
	if err != nil {
		return err
	}
	select {
	case c.bulk <- b:
		return nil
	case <-c.done:
		return ErrConnClosed
	}
}

// WriteDroppable enqueues a droppable message (video). Never blocks: if the peer
// is behind, the oldest queued frame is dropped so capture is never stalled (a
// keyframe request recovers the decoder).
func (c *Conn) WriteDroppable(kind byte, payload []byte) error {
	b, err := frame(kind, payload)
	if err != nil {
		return err
	}
	for {
		select {
		case c.vid <- b:
			return nil
		case <-c.done:
			return ErrConnClosed
		default:
			// Queue full — drop the oldest frame, then retry.
			select {
			case <-c.vid:
			default:
			}
		}
	}
}

// Close stops the writer goroutine and closes the underlying connection.
func (c *Conn) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	return c.Conn.Close()
}

// ReadMessage reads one framed message (single reader per direction; no lock).
func (c *Conn) ReadMessage() (byte, []byte, error) {
	return ReadMessage(c.Conn)
}

// WriteMessage frames and writes one message to w.
func WriteMessage(w io.Writer, kind byte, payload []byte) error {
	n := 1 + len(payload)
	if n-1 > maxPayload {
		return fmt.Errorf("ipc: payload too large (%d)", len(payload))
	}
	hdr := make([]byte, 5)
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(n))
	hdr[4] = kind
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// ReadMessage reads one framed message from r. Returns the kind and payload.
func ReadMessage(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[0:4])
	if n == 0 || uint64(n)-1 > maxPayload {
		return 0, nil, fmt.Errorf("ipc: bad frame length %d", n)
	}
	kind := hdr[4]
	payload := make([]byte, n-1)
	if len(payload) > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}
	return kind, payload, nil
}

// EncodeVideoFrame builds the KindVideoFrame payload.
func EncodeVideoFrame(keyframe bool, vp8 []byte) []byte {
	out := make([]byte, 1+len(vp8))
	if keyframe {
		out[0] = 1
	}
	copy(out[1:], vp8)
	return out
}

// DecodeVideoFrame parses a KindVideoFrame payload.
func DecodeVideoFrame(payload []byte) (keyframe bool, vp8 []byte, ok bool) {
	if len(payload) < 1 {
		return false, nil, false
	}
	return payload[0] != 0, payload[1:], true
}

// EncodeVideoInfo builds the KindVideoInfo payload.
func EncodeVideoInfo(width, height int) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint32(out[0:4], uint32(width))
	binary.LittleEndian.PutUint32(out[4:8], uint32(height))
	return out
}

// DecodeVideoInfo parses a KindVideoInfo payload.
func DecodeVideoInfo(payload []byte) (width, height int, ok bool) {
	if len(payload) < 8 {
		return 0, 0, false
	}
	return int(binary.LittleEndian.Uint32(payload[0:4])),
		int(binary.LittleEndian.Uint32(payload[4:8])), true
}

// Listen starts a loopback TCP listener for the transport side.
func Listen(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// Dial connects a worker to the transport.
func Dial(port int) (net.Conn, error) {
	return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// DialRetry connects a worker to the transport, retrying on failure until it
// succeeds, ctx is cancelled, or the timeout elapses. The transport (session 0)
// may not be accepting at the instant the service spawns a new worker on a user
// switch; without retrying, a single connection-refused would fatally kill the
// worker and leave the transport with no frame producer (black screen). Retrying
// makes the worker simply wait for the transport to come up.
func DialRetry(ctx context.Context, port int, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		conn, err := Dial(port)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}
