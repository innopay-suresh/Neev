package session

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/draw"
	_ "image/jpeg" // register JPEG decoder (helper sends JPEG secure-desktop frames)
	_ "image/png"  // and PNG, defensively
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/encode"
)

// helperPipePort is the SYSTEM helper's secure-desktop pipe (127.0.0.1 only).
// Wire format (both directions): [uint32 LE len][uint8 type][payload], len =
// 1 + payloadLen. Agent->us: 'A' int32 w,h (secure active) / 'F' JPEG frame /
// 'G' (secure gone) / 'e' uint8 (foreground elevated). Us->agent: 'I' forwarded
// input (sub 'm'/'b'/'w'/'k') — the same protocol the Flutter host uses.
const helperPipePort = 47921

// controlEvent is the viewer's control/cursor-channel JSON
// ({"k":"mv"|"btn"|"whl"|"key", ...}). Shared by the Windows injector and the
// secure-desktop bridge (which translates it to the helper's 'I' protocol).
type controlEvent struct {
	K  string   `json:"k"`
	X  *float64 `json:"x"`
	Y  *float64 `json:"y"`
	B  *int     `json:"b"`
	D  *bool    `json:"d"`
	DX *float64 `json:"dx"`
	DY *float64 `json:"dy"`
	U  *int     `json:"u"`
}

func num(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// secureBridge connects to the SYSTEM helper's secure-desktop pipe. When the
// helper reports the secure desktop is active (UAC / lock / a different user's
// login screen), it re-encodes the helper's JPEG frames to VP8 for the live
// transport track and forwards viewer input to the helper so the screen stays
// interactive. This reuses the proven helper secure-desktop code unchanged (no
// regression); the bridge is just another pipe client (the helper already
// broadcasts to all clients).
type secureBridge struct {
	onFrame func(vp8 []byte, keyframe bool)

	secure   atomic.Bool // secure desktop currently shown
	elevated atomic.Bool // foreground window elevated (route input via helper)
	forceKey atomic.Bool // force a keyframe on the next secure frame (source switch)

	writeMu sync.Mutex
	conn    net.Conn // current pipe connection (nil if down)

	enc        *encode.Encoder
	encW, encH int
}

// newSecureBridge starts the pipe client. onFrame receives each VP8-encoded
// secure-desktop frame (only produced while the secure desktop is active).
func newSecureBridge(onFrame func(vp8 []byte, keyframe bool)) *secureBridge {
	b := &secureBridge{onFrame: onFrame}
	go b.runLoop()
	return b
}

func (b *secureBridge) SecureActive() bool   { return b.secure.Load() }
func (b *secureBridge) ElevatedActive() bool { return b.elevated.Load() }

// requestKeyframe forces the next secure frame to be a keyframe — used when the
// frame source switches to the bridge so the viewer's decoder recovers cleanly.
func (b *secureBridge) requestKeyframe() { b.forceKey.Store(true) }

// runLoop keeps a connection to the helper alive, reconnecting so the bridge is
// ready the instant a secure desktop appears.
func (b *secureBridge) runLoop() {
	for {
		conn, err := net.DialTimeout("tcp",
			net.JoinHostPort("127.0.0.1", itoa(helperPipePort)), 2*time.Second)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		b.writeMu.Lock()
		b.conn = conn
		b.writeMu.Unlock()
		log.Info().Msg("secure-bridge: connected to helper pipe")
		b.readLoop(conn)
		// Connection dropped: reset state so we don't leave frames pinned.
		b.writeMu.Lock()
		if b.conn == conn {
			b.conn = nil
		}
		b.writeMu.Unlock()
		b.secure.Store(false)
		b.elevated.Store(false)
		conn.Close()
		log.Info().Msg("secure-bridge: helper pipe dropped; reconnecting")
		time.Sleep(1 * time.Second)
	}
}

func (b *secureBridge) readLoop(conn net.Conn) {
	var hdr [5]byte
	for {
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return
		}
		n := binary.LittleEndian.Uint32(hdr[0:4])
		if n == 0 || n > 64*1024*1024 {
			return
		}
		typ := hdr[4]
		payload := make([]byte, n-1)
		if len(payload) > 0 {
			if _, err := io.ReadFull(conn, payload); err != nil {
				return
			}
		}
		switch typ {
		case 'A': // secure desktop active (+int32 w,h — we size the encoder off
			// the decoded frame, so the dims here are informational).
			if !b.secure.Swap(true) {
				b.forceKey.Store(true)
				log.Info().Msg("secure-bridge: secure desktop ACTIVE")
			}
		case 'F': // JPEG frame of the secure desktop.
			if b.secure.Load() {
				b.handleFrame(payload)
			}
		case 'G': // secure desktop gone → revert to the worker's frames.
			if b.secure.Swap(false) {
				log.Info().Msg("secure-bridge: secure desktop GONE")
			}
		case 'e': // foreground elevated (1) / not (0).
			b.elevated.Store(len(payload) >= 1 && payload[0] != 0)
		}
	}
}

// handleFrame decodes one helper JPEG frame and re-encodes it to VP8.
func (b *secureBridge) handleFrame(jpegBytes []byte) {
	img, _, err := image.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		bnd := img.Bounds()
		rgba = image.NewRGBA(bnd)
		draw.Draw(rgba, bnd, img, bnd.Min, draw.Src)
	}
	w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	if b.enc == nil || w != b.encW || h != b.encH {
		if b.enc != nil {
			b.enc.Close()
		}
		enc, err := encode.NewEncoder(w, h, workerFPS, workerBitrate)
		if err != nil {
			log.Error().Err(err).Msg("secure-bridge: encoder create failed")
			return
		}
		b.enc, b.encW, b.encH = enc, w, h
		b.forceKey.Store(true)
	}
	forceKey := b.forceKey.Swap(false)
	out, err := b.enc.Encode(rgba, forceKey)
	if err != nil || out == nil || len(out.Data) == 0 {
		return
	}
	if b.onFrame != nil {
		b.onFrame(out.Data, out.IsKeyframe)
	}
}

// SendInput translates one viewer control-channel JSON event into the helper's
// 'I' forwarded-input protocol and sends it, so clicks/keys reach the secure or
// elevated desktop (which only the SYSTEM helper can inject into).
func (b *secureBridge) SendInput(raw []byte) {
	var e controlEvent
	if err := json.Unmarshal(raw, &e); err != nil {
		return
	}
	var sub byte
	var buf bytes.Buffer
	switch e.K {
	case "mv":
		sub = 'm'
		writeF32(&buf, num(e.X))
		writeF32(&buf, num(e.Y))
	case "btn":
		sub = 'b'
		btn := 0
		if e.B != nil {
			btn = *e.B
		}
		down := byte(0)
		if e.D != nil && *e.D {
			down = 1
		}
		hasPos := byte(0)
		if e.X != nil && e.Y != nil {
			hasPos = 1
		}
		buf.WriteByte(byte(btn))
		buf.WriteByte(down)
		buf.WriteByte(hasPos)
		writeF32(&buf, num(e.X))
		writeF32(&buf, num(e.Y))
	case "whl":
		sub = 'w'
		writeF32(&buf, num(e.DX))
		writeF32(&buf, num(e.DY))
	case "key":
		sub = 'k'
		usage := 0
		if e.U != nil {
			usage = *e.U
		}
		down := byte(0)
		if e.D != nil && *e.D {
			down = 1
		}
		var u [2]byte
		binary.LittleEndian.PutUint16(u[:], uint16(usage))
		buf.Write(u[:])
		buf.WriteByte(down)
	default:
		return // quality/cmd/etc. — not input
	}
	payload := append([]byte{sub}, buf.Bytes()...)
	b.writeMsg('I', payload)
}

// writeMsg frames and sends one pipe message to the helper.
func (b *secureBridge) writeMsg(typ byte, payload []byte) {
	b.writeMu.Lock()
	conn := b.conn
	b.writeMu.Unlock()
	if conn == nil {
		return
	}
	frame := make([]byte, 5+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], uint32(1+len(payload)))
	frame[4] = typ
	copy(frame[5:], payload)
	b.writeMu.Lock()
	_, _ = conn.Write(frame)
	b.writeMu.Unlock()
}

func writeF32(buf *bytes.Buffer, v float64) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], math.Float32bits(float32(v)))
	buf.Write(b[:])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
