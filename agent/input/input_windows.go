//go:build windows
// +build windows

package input

import (
	"syscall"
	"unsafe"
)

var (
	moduser32            = syscall.NewLazyDLL("user32.dll")
	procSendInput        = moduser32.NewProc("SendInput")
	procGetSystemMetrics = moduser32.NewProc("GetSystemMetrics")
)

const (
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1

	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_ABSOLUTE   = 0x8000

	KEYEVENTF_KEYUP = 0x0002
)

// Win32 INPUT structure (pad out for 64-bit alignment if needed, but the simple way is generic byte array)
type mouseInput struct {
	Type        uint32
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
	// Padding to match largest union size
	padding1 uint32
	padding2 uint32
}

type keybdInput struct {
	Type        uint32
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
	padding1    uint32
	padding2    uint32
}

type windowsInjector struct {
	screenWidth  int32
	screenHeight int32
}

func newPlatformInjector() (Injector, error) {
	w, _, _ := procGetSystemMetrics.Call(uintptr(SM_CXSCREEN))
	h, _, _ := procGetSystemMetrics.Call(uintptr(SM_CYSCREEN))
	return &windowsInjector{
		screenWidth:  int32(w),
		screenHeight: int32(h),
	}, nil
}

func (w *windowsInjector) InjectEvent(e Event) error {
	switch e.Type {
	case EventMouseMove:
		x := int32((e.X * 65535.0) + 0.5)
		y := int32((e.Y * 65535.0) + 0.5)
		w.sendMouseInput(x, y, 0, MOUSEEVENTF_MOVE|MOUSEEVENTF_ABSOLUTE)
	case EventMouseDown, EventMouseUp:
		isDown := e.Type == EventMouseDown
		flags := uint32(0)
		switch e.Button {
		case ButtonLeft:
			if isDown {
				flags = MOUSEEVENTF_LEFTDOWN
			} else {
				flags = MOUSEEVENTF_LEFTUP
			}
		case ButtonRight:
			if isDown {
				flags = MOUSEEVENTF_RIGHTDOWN
			} else {
				flags = MOUSEEVENTF_RIGHTUP
			}
		case ButtonMiddle:
			if isDown {
				flags = MOUSEEVENTF_MIDDLEDOWN
			} else {
				flags = MOUSEEVENTF_MIDDLEUP
			}
		}
		w.sendMouseInput(0, 0, 0, flags)
	case EventMouseScroll:
		w.sendMouseInput(0, 0, uint32(int32(e.DeltaY*120)), MOUSEEVENTF_WHEEL)
	case EventKeyDown, EventKeyUp:
		isDown := e.Type == EventKeyDown
		flags := uint32(0)
		if !isDown {
			flags = KEYEVENTF_KEYUP
		}
		vk := mapJSCodeToVK(e.Code, e.KeyCode)
		w.sendKeyInput(uint16(vk), flags)
	}
	return nil
}

func (w *windowsInjector) sendMouseInput(dx, dy int32, data uint32, flags uint32) {
	// Win32 INPUT struct for mouse on 64-bit: 40 bytes total.
	// type(4) + pad(4) + dx(4) + dy(4) + mouseData(4) + dwFlags(4) + time(4) + implicit_pad(4) + dwExtraInfo(8) = 40
	// Do NOT add Pad1 here — that makes the struct 48 bytes and causes SendInput to silently fail.
	var input struct {
		Type        uint32
		Pad0        uint32
		Dx          int32
		Dy          int32
		MouseData   uint32
		DwFlags     uint32
		Time        uint32
		DwExtraInfo uintptr
	}
	input.Type = INPUT_MOUSE
	input.Dx = dx
	input.Dy = dy
	input.MouseData = data
	input.DwFlags = flags

	procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&input)),
		uintptr(unsafe.Sizeof(input)), // = 40 bytes, matching Win32 sizeof(INPUT)
	)
}

func (w *windowsInjector) sendKeyInput(vk uint16, flags uint32) {
	var input struct {
		Type        uint32
		Pad0        uint32
		WVk         uint16
		WScan       uint16
		DwFlags     uint32
		Time        uint32
		DwExtraInfo uintptr
		Pad1        uint64
	}
	input.Type = INPUT_KEYBOARD
	input.WVk = vk
	input.DwFlags = flags

	procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&input)),
		uintptr(unsafe.Sizeof(input)),
	)
}

func (w *windowsInjector) Close() error { return nil }

func mapJSCodeToVK(code string, fallback int) int {
	// A simple mapping for critical JS codes to VK codes.
	// For full fidelity, we map exactly as needed.
	m := map[string]int{
		"Backspace": 0x08, "Tab": 0x09, "Enter": 0x0D, "ShiftLeft": 0x10, "ShiftRight": 0xA1,
		"ControlLeft": 0x11, "ControlRight": 0xA3, "AltLeft": 0x12, "AltRight": 0xA5,
		"Escape": 0x1B, "Space": 0x20, "PageUp": 0x21, "PageDown": 0x22, "End": 0x23, "Home": 0x24,
		"ArrowLeft": 0x25, "ArrowUp": 0x26, "ArrowRight": 0x27, "ArrowDown": 0x28, "Delete": 0x2E,
		"MetaLeft": 0x5B, "MetaRight": 0x5C,
	}
	if v, ok := m[code]; ok {
		return v
	}
	return fallback
}
