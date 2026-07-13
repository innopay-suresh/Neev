//go:build windows

package session

import (
	"encoding/json"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// inputSink injects viewer input into the worker's (active user) session. The
// worker runs as the logged-in user via WTSQueryUserToken, so SendInput here
// lands on that user's desktop — which is how mouse/keyboard control follows a
// user switch without the WebRTC connection dropping.
//
// This is a faithful port of the shipping Flutter host injector
// (windows/runner/input_injector.cpp): same HID→VK map, extended-key handling,
// absolute-over-primary coordinates, last-position fallback, and — crucially —
// a single serial worker so events apply in receipt order (a button-up must
// never overtake its button-down, or the remote button sticks: the classic
// "click and everything freezes" bug).
type inputSink interface {
	Post(raw []byte)
	Close()
}

func newInputSink() inputSink { return newPlatformInputSink() }

var (
	moduser32         = syscall.NewLazyDLL("user32.dll")
	procSendInput     = moduser32.NewProc("SendInput")
	procMapVirtualKey = moduser32.NewProc("MapVirtualKeyW")

	modkernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadId = modkernel32.NewProc("GetCurrentThreadId")
	procGetThreadDesktop   = moduser32.NewProc("GetThreadDesktop")
	procGetUserObjectInfo  = moduser32.NewProc("GetUserObjectInformationW")
)

// uoiName selects the desktop's name in GetUserObjectInformationW.
const uoiName = 2

// injectSeq counts injection attempts so SendInput failures can be sample-logged
// (loud on the first few, then throttled) rather than flooding at move rates.
var injectSeq atomic.Uint64

// currentDesktopName returns the desktop the calling thread is bound to (e.g.
// "Default", "Winlogon"). SendInput only lands on the desktop that currently has
// the input focus, so if the worker's inject thread is not on "Default" while
// the user sits at a normal desktop, every event silently no-ops — exactly the
// "video works, clicks dead" failure the July-9 process split can introduce.
func currentDesktopName() string {
	tid, _, _ := procGetCurrentThreadId.Call()
	hdesk, _, _ := procGetThreadDesktop.Call(tid)
	if hdesk == 0 {
		return "?"
	}
	var buf [256]uint16
	var needed uint32
	r, _, _ := procGetUserObjectInfo.Call(hdesk, uoiName,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2),
		uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return "?"
	}
	return syscall.UTF16ToString(buf[:])
}

// sendInput wraps SendInput and makes a non-landing injection observable. On
// Windows SendInput returns the number of events inserted; 0 means the OS
// rejected it (wrong desktop / UIPI integrity / locked input) — the event
// vanished. That return was previously discarded, so a fully dead input path
// looked identical to a working one.
func sendInput(ptr unsafe.Pointer, size uintptr) {
	n, _, errno := procSendInput.Call(1, uintptr(ptr), size)
	seq := injectSeq.Add(1)
	if n == 0 {
		if seq <= 5 || seq%256 == 0 {
			log.Warn().Uint64("seq", seq).Str("desktop", currentDesktopName()).
				Str("err", errno.Error()).
				Msg("worker: SendInput inserted 0 events — input not landing on desktop")
		}
		return
	}
	if seq == 1 {
		log.Info().Str("desktop", currentDesktopName()).
			Msg("worker: first viewer input injected OK")
	}
}

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove       = 0x0001
	mouseeventfLeftDown   = 0x0002
	mouseeventfLeftUp     = 0x0004
	mouseeventfRightDown  = 0x0008
	mouseeventfRightUp    = 0x0010
	mouseeventfMiddleDown = 0x0020
	mouseeventfMiddleUp   = 0x0040
	mouseeventfWheel      = 0x0800
	mouseeventfHWheel     = 0x1000
	mouseeventfAbsolute   = 0x8000

	keyeventfExtended = 0x0001
	keyeventfKeyup    = 0x0002

	mapvkVkToVsc = 0

	vkReturn  = 0x0D
	vkEscape  = 0x1B
	vkBack    = 0x08
	vkTab     = 0x09
	vkSpace   = 0x20
	vkF1      = 0x70
	vkCapital = 0x14

	vkOemMinus  = 0xBD
	vkOemPlus   = 0xBB
	vkOem4      = 0xDB
	vkOem6      = 0xDD
	vkOem5      = 0xDC
	vkOem1      = 0xBA
	vkOem7      = 0xDE
	vkOem3      = 0xC0
	vkOemComma  = 0xBC
	vkOemPeriod = 0xBE
	vkOem2      = 0xBF

	vkInsert = 0x2D
	vkHome   = 0x24
	vkPrior  = 0x21
	vkDelete = 0x2E
	vkEnd    = 0x23
	vkNext   = 0x22
	vkRight  = 0x27
	vkLeft   = 0x25
	vkDown   = 0x28
	vkUp     = 0x26

	vkLControl = 0xA2
	vkLShift   = 0xA0
	vkLMenu    = 0xA4
	vkLWin     = 0x5B
	vkRControl = 0xA3
	vkRShift   = 0xA1
	vkRMenu    = 0xA5
	vkRWin     = 0x5C
)

// hidToVk maps a USB HID usage code to a Windows virtual-key code. Exact port of
// HidToVk() in input_injector.cpp — the viewer sends HID usages so the mapping
// stays keyboard-layout independent.
func hidToVk(usage int) uint16 {
	switch {
	case usage >= 0x04 && usage <= 0x1D:
		return uint16('A' + (usage - 0x04))
	case usage >= 0x1E && usage <= 0x26:
		return uint16('1' + (usage - 0x1E))
	case usage == 0x27:
		return uint16('0')
	case usage >= 0x3A && usage <= 0x45:
		return uint16(vkF1 + (usage - 0x3A))
	}
	switch usage {
	case 0x28:
		return vkReturn
	case 0x29:
		return vkEscape
	case 0x2A:
		return vkBack
	case 0x2B:
		return vkTab
	case 0x2C:
		return vkSpace
	case 0x2D:
		return vkOemMinus
	case 0x2E:
		return vkOemPlus
	case 0x2F:
		return vkOem4
	case 0x30:
		return vkOem6
	case 0x31:
		return vkOem5
	case 0x33:
		return vkOem1
	case 0x34:
		return vkOem7
	case 0x35:
		return vkOem3
	case 0x36:
		return vkOemComma
	case 0x37:
		return vkOemPeriod
	case 0x38:
		return vkOem2
	case 0x39:
		return vkCapital
	case 0x49:
		return vkInsert
	case 0x4A:
		return vkHome
	case 0x4B:
		return vkPrior
	case 0x4C:
		return vkDelete
	case 0x4D:
		return vkEnd
	case 0x4E:
		return vkNext
	case 0x4F:
		return vkRight
	case 0x50:
		return vkLeft
	case 0x51:
		return vkDown
	case 0x52:
		return vkUp
	case 0xE0:
		return vkLControl
	case 0xE1:
		return vkLShift
	case 0xE2:
		return vkLMenu
	case 0xE3:
		return vkLWin
	case 0xE4:
		return vkRControl
	case 0xE5:
		return vkRShift
	case 0xE6:
		return vkRMenu
	case 0xE7:
		return vkRWin
	}
	return 0
}

func isExtendedVk(vk uint16) bool {
	switch vk {
	case vkRight, vkLeft, vkUp, vkDown,
		vkHome, vkEnd, vkPrior, vkNext,
		vkInsert, vkDelete,
		vkRControl, vkRMenu, vkLWin, vkRWin:
		return true
	}
	return false
}

type winInputSink struct {
	ch     chan []byte
	done   chan struct{}
	lastNx float64
	lastNy float64
}

func newPlatformInputSink() inputSink {
	s := &winInputSink{
		ch:   make(chan []byte, 512),
		done: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *winInputSink) Post(raw []byte) {
	// Copy: the caller reuses/frees the IPC buffer after this returns.
	buf := make([]byte, len(raw))
	copy(buf, raw)
	select {
	case s.ch <- buf:
	default:
		// Queue full (a stall injecting): drop the oldest move-class event so
		// input stays responsive rather than backing up unboundedly.
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

func (s *winInputSink) Close() { close(s.done) }

func (s *winInputSink) run() {
	log.Info().Str("desktop", currentDesktopName()).
		Msg("worker: input injector started")
	for {
		select {
		case <-s.done:
			return
		case raw := <-s.ch:
			s.handle(raw)
		}
	}
}

func (s *winInputSink) handle(raw []byte) {
	var e controlEvent
	if err := json.Unmarshal(raw, &e); err != nil {
		return
	}
	switch e.K {
	case "mv":
		nx, ny := num(e.X), num(e.Y)
		s.lastNx, s.lastNy = nx, ny
		sendMouseAbsolute(nx, ny, mouseeventfMove, 0)
	case "btn":
		button := 0
		if e.B != nil {
			button = *e.B
		}
		down := e.D != nil && *e.D
		var btnFlag uint32
		switch button {
		case 1:
			if down {
				btnFlag = mouseeventfRightDown
			} else {
				btnFlag = mouseeventfRightUp
			}
		case 2:
			if down {
				btnFlag = mouseeventfMiddleDown
			} else {
				btnFlag = mouseeventfMiddleUp
			}
		default:
			if down {
				btnFlag = mouseeventfLeftDown
			} else {
				btnFlag = mouseeventfLeftUp
			}
		}
		nx, ny := num(e.X), num(e.Y)
		// Fall back to the last known position if the button carries (0,0), so a
		// throttled/dropped preceding move can't make the click land at (0,0).
		if nx == 0 && ny == 0 {
			nx, ny = s.lastNx, s.lastNy
		}
		sendMouseAbsolute(nx, ny, mouseeventfMove|btnFlag, 0)
		s.lastNx, s.lastNy = nx, ny
	case "whl":
		if dy := num(e.DY); dy != 0 {
			sendMouseAbsolute(0, 0, mouseeventfWheel, uint32(int32(-dy)))
		}
		if dx := num(e.DX); dx != 0 {
			sendMouseAbsolute(0, 0, mouseeventfHWheel, uint32(int32(dx)))
		}
	case "key":
		usage := 0
		if e.U != nil {
			usage = *e.U
		}
		down := e.D != nil && *e.D
		vk := hidToVk(usage)
		if vk == 0 {
			return
		}
		sendKey(vk, down)
	}
}

// sendMouseAbsolute mirrors SendMouseAbsolute() in input_injector.cpp: absolute
// coordinates over the PRIMARY monitor (no VIRTUALDESK) to match the worker's
// primary-monitor capture.
func sendMouseAbsolute(nx, ny float64, flags, mouseData uint32) {
	var in struct {
		Type        uint32
		Pad0        uint32
		Dx          int32
		Dy          int32
		MouseData   uint32
		DwFlags     uint32
		Time        uint32
		DwExtraInfo uintptr
	}
	in.Type = inputMouse
	in.Dx = int32(nx * 65535.0)
	in.Dy = int32(ny * 65535.0)
	in.MouseData = mouseData
	in.DwFlags = flags | mouseeventfAbsolute
	sendInput(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func sendKey(vk uint16, down bool) {
	var in struct {
		Type        uint32
		Pad0        uint32
		WVk         uint16
		WScan       uint16
		DwFlags     uint32
		Time        uint32
		DwExtraInfo uintptr
		Pad1        uint64
	}
	scan, _, _ := procMapVirtualKey.Call(uintptr(vk), mapvkVkToVsc)
	in.Type = inputKeyboard
	in.WVk = vk
	in.WScan = uint16(scan)
	if !down {
		in.DwFlags |= keyeventfKeyup
	}
	if isExtendedVk(vk) {
		in.DwFlags |= keyeventfExtended
	}
	sendInput(unsafe.Pointer(&in), unsafe.Sizeof(in))
}
