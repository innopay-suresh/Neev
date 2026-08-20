//go:build darwin

package session

import (
	"encoding/json"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/input"
)

// Viewer input injection for a macOS TransportMode host.
//
// This did not exist. inject_other.go (!windows) supplied a no-op sink, so the
// worker received every mouse and key event and threw it away: the transport
// logged "input -> capture worker n=512 worker=true" while nothing moved on the
// host, and clicks had never worked on a daemon-hosted Mac. The CGEvent
// injector in agent/input was already written and correct — it was simply never
// wired to the worker, only to the older all-in-one pipeline.
//
// The wire format is the same controlEvent the Windows sink parses, so both
// platforms are driven by one viewer protocol; only the translation differs.

// darwinInputSink serializes injection onto one goroutine, mirroring the
// Windows sink: input arrives at mouse-move rates and must never block the IPC
// reader, or a burst of moves stalls capture and clipboard behind it.
type darwinInputSink struct {
	inj    input.Injector
	ch     chan []byte
	done   chan struct{}
	lastNx float64
	lastNy float64
}

func newInputSink() inputSink {
	inj, err := input.NewInjector()
	if err != nil {
		// Never fail the worker for this: video, clipboard, file transfer and
		// voice all still work, and a host that shows a screen but ignores
		// clicks is far more useful than one that does not start.
		log.Error().Err(err).Msg("worker: input injector unavailable — viewer clicks and keys will do nothing")
		return noopInputSink{}
	}
	s := &darwinInputSink{
		inj:  inj,
		ch:   make(chan []byte, 512),
		done: make(chan struct{}),
	}
	go s.run()
	log.Info().Msg("worker: input injector started")
	return s
}

func (s *darwinInputSink) Post(raw []byte) {
	// Copy: the caller reuses the IPC buffer once this returns.
	buf := make([]byte, len(raw))
	copy(buf, raw)
	select {
	case s.ch <- buf:
	default:
		// Full queue means injection is stalling. Drop the OLDEST event rather
		// than the newest: the newest is where the pointer actually is.
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- buf:
		default:
		}
	}
}

func (s *darwinInputSink) Close() { close(s.done) }

func (s *darwinInputSink) run() {
	for {
		select {
		case <-s.done:
			return
		case raw := <-s.ch:
			s.handle(raw)
		}
	}
}

// lastNx/lastNy remember the pointer so a click carrying (0,0) — which happens
// when the preceding move was throttled or dropped — lands where the pointer
// is instead of the top-left corner. The Windows sink does the same.
func (s *darwinInputSink) handle(raw []byte) {
	var e controlEvent
	if err := json.Unmarshal(raw, &e); err != nil {
		return
	}
	switch e.K {
	case "mv":
		nx, ny := num(e.X), num(e.Y)
		s.lastNx, s.lastNy = nx, ny
		_ = s.inj.InjectEvent(input.Event{
			Type: input.EventMouseMove, X: nx, Y: ny,
		})
	case "btn":
		nx, ny := num(e.X), num(e.Y)
		if nx == 0 && ny == 0 {
			nx, ny = s.lastNx, s.lastNy
		}
		typ := input.EventMouseUp
		if e.D != nil && *e.D {
			typ = input.EventMouseDown
		}
		_ = s.inj.InjectEvent(input.Event{
			Type: typ, X: nx, Y: ny, Button: wireButton(e.B),
		})
	case "whl":
		// Negated to match the Windows sink, which sends -dy: the viewer's dy is
		// positive when scrolling DOWN (the browser wheel convention), while both
		// platforms' scroll APIs take positive as scrolling up.
		_ = s.inj.InjectEvent(input.Event{
			Type:   input.EventMouseScroll,
			DeltaX: num(e.DX),
			DeltaY: -num(e.DY),
		})
	case "key":
		if e.U == nil {
			return
		}
		code := hidToCGKey(*e.U)
		if code < 0 {
			return // unmapped key: better ignored than injected as the wrong one
		}
		typ := input.EventKeyUp
		if e.D != nil && *e.D {
			typ = input.EventKeyDown
		}
		_ = s.inj.InjectEvent(input.Event{Type: typ, KeyCode: code})
	}
}

// wireButton maps the viewer's button number onto the injector's.
//
// The two orders are NOT the same and must not be passed through: the wire uses
// 1=right, 2=middle (matching the Windows sink), while input.MouseButton is
// 0=left, 1=middle, 2=right. Passing the raw value turns every right-click into
// a middle-click.
func wireButton(b *int) input.MouseButton {
	if b == nil {
		return input.ButtonLeft
	}
	switch *b {
	case 1:
		return input.ButtonRight
	case 2:
		return input.ButtonMiddle
	default:
		return input.ButtonLeft
	}
}

// hidToCGKey maps a USB HID usage code to a macOS virtual keycode, the darwin
// counterpart of hidToVk. Returns -1 when unmapped.
//
// The layout is ANSI; macOS keycodes are positional, not alphabetical, so this
// has to be a table rather than arithmetic the way the Windows VK range allows.
func hidToCGKey(usage int) int {
	// Letters: HID 0x04..0x1D is A..Z.
	letters := [26]int{
		0,  // A
		11, // B
		8,  // C
		2,  // D
		14, // E
		3,  // F
		5,  // G
		4,  // H
		34, // I
		38, // J
		40, // K
		37, // L
		46, // M
		45, // N
		31, // O
		35, // P
		12, // Q
		15, // R
		1,  // S
		17, // T
		32, // U
		9,  // V
		13, // W
		7,  // X
		16, // Y
		6,  // Z
	}
	if usage >= 0x04 && usage <= 0x1D {
		return letters[usage-0x04]
	}
	// Digits: HID 0x1E..0x26 is 1..9, 0x27 is 0.
	digits := [9]int{18, 19, 20, 21, 23, 22, 26, 28, 25} // 1..9
	if usage >= 0x1E && usage <= 0x26 {
		return digits[usage-0x1E]
	}
	if usage == 0x27 {
		return 29 // 0
	}
	// Function keys: HID 0x3A..0x45 is F1..F12.
	fkeys := [12]int{122, 120, 99, 118, 96, 97, 98, 100, 101, 109, 103, 111}
	if usage >= 0x3A && usage <= 0x45 {
		return fkeys[usage-0x3A]
	}
	switch usage {
	case 0x28:
		return 36 // Return
	case 0x29:
		return 53 // Escape
	case 0x2A:
		return 51 // Delete (backspace)
	case 0x2B:
		return 48 // Tab
	case 0x2C:
		return 49 // Space
	case 0x2D:
		return 27 // Minus
	case 0x2E:
		return 24 // Equal
	case 0x2F:
		return 33 // LeftBracket
	case 0x30:
		return 30 // RightBracket
	case 0x31:
		return 42 // Backslash
	case 0x33:
		return 41 // Semicolon
	case 0x34:
		return 39 // Quote
	case 0x35:
		return 50 // Grave
	case 0x36:
		return 43 // Comma
	case 0x37:
		return 47 // Period
	case 0x38:
		return 44 // Slash
	case 0x39:
		return 57 // CapsLock
	case 0x49:
		return 114 // Help/Insert
	case 0x4A:
		return 115 // Home
	case 0x4B:
		return 116 // PageUp
	case 0x4C:
		return 117 // ForwardDelete
	case 0x4D:
		return 119 // End
	case 0x4E:
		return 121 // PageDown
	case 0x4F:
		return 124 // Right
	case 0x50:
		return 123 // Left
	case 0x51:
		return 125 // Down
	case 0x52:
		return 126 // Up
	case 0xE0:
		return 59 // Left Control
	case 0xE1:
		return 56 // Left Shift
	case 0xE2:
		return 58 // Left Option
	case 0xE3:
		return 55 // Left Command
	case 0xE4:
		return 62 // Right Control
	case 0xE5:
		return 60 // Right Shift
	case 0xE6:
		return 61 // Right Option
	case 0xE7:
		return 54 // Right Command
	}
	return -1
}
