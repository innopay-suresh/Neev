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
)

// maxPayload caps a single message so a corrupt stream can't allocate wildly.
// A 4K keyframe is comfortably under this.
const maxPayload = 32 << 20 // 32 MiB

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
