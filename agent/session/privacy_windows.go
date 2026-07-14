//go:build windows

package session

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// Host privacy mode for TransportMode, ported from the Flutter host's
// privacy_mode.cpp: a full-virtual-screen black window that is EXCLUDED from
// screen capture (WDA_EXCLUDEFROMCAPTURE) — so the local user sees black while
// the viewer keeps seeing the real desktop — plus BlockInput to stop local
// mouse/keyboard. Runs on its own OS thread with a message pump.

var (
	modUser32Priv                  = syscall.NewLazyDLL("user32.dll")
	procRegisterClassW             = modUser32Priv.NewProc("RegisterClassW")
	procCreateWindowExW            = modUser32Priv.NewProc("CreateWindowExW")
	procDefWindowProcW             = modUser32Priv.NewProc("DefWindowProcW")
	procShowWindow                 = modUser32Priv.NewProc("ShowWindow")
	procDestroyWindow              = modUser32Priv.NewProc("DestroyWindow")
	procPeekMessageW               = modUser32Priv.NewProc("PeekMessageW")
	procTranslateMessage           = modUser32Priv.NewProc("TranslateMessage")
	procDispatchMessageW           = modUser32Priv.NewProc("DispatchMessageW")
	procSetLayeredWindowAttributes = modUser32Priv.NewProc("SetLayeredWindowAttributes")
	procSetWindowDisplayAffinity   = modUser32Priv.NewProc("SetWindowDisplayAffinity")
	procGetSystemMetricsPriv       = modUser32Priv.NewProc("GetSystemMetrics")
	procBlockInput                 = modUser32Priv.NewProc("BlockInput")

	procGetStockObject       = syscall.NewLazyDLL("gdi32.dll").NewProc("GetStockObject")
	procGetModuleHandleWPriv = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	wsExTopmost    = 0x00000008
	wsExLayered    = 0x00080000
	wsExTransparent = 0x00000020
	wsExToolwindow = 0x00000080
	wsExNoactivate = 0x08000000
	wsPopup        = 0x80000000
	lwaAlpha       = 0x00000002
	swShowNoActivate = 4
	blackBrush     = 4
	wdaExcludeFromCapture = 0x00000011
	smXVirtual     = 76
	smYVirtual     = 77
	smCXVirtual    = 78
	smCYVirtual    = 79
	pmRemove       = 0x0001
)

type wndClassW struct {
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
}

type msgStruct struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

var (
	privacyCmd     = make(chan bool, 8)
	privacyStarted sync.Once
	privacyClassOK bool
)

// setPrivacy toggles host privacy mode (idempotent; safe to call repeatedly).
func setPrivacy(on bool) {
	privacyStarted.Do(func() { go privacyLoop() })
	select {
	case privacyCmd <- on:
	default:
	}
}

func privacyWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func privacyLoop() {
	runtime.LockOSThread()
	// Bind to the interactive input desktop so the overlay window can be created
	// (see chatLoop) — a service-spawned worker may otherwise be denied GUI.
	bindInputDesktop()
	className, _ := syscall.UTF16PtrFromString("NeevPrivacyBlank")
	hInst, _, _ := procGetModuleHandleWPriv.Call(0)
	if !privacyClassOK {
		brush, _, _ := procGetStockObject.Call(blackBrush)
		wc := wndClassW{
			lpfnWndProc:   syscall.NewCallback(privacyWndProc),
			hInstance:     hInst,
			hbrBackground: brush,
			lpszClassName: className,
		}
		procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))
		privacyClassOK = true
	}

	var hwnd uintptr
	blocked := false
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case on := <-privacyCmd:
			if on {
				if hwnd == 0 {
					hwnd = createBlankWindow(className, hInst)
				}
				if !blocked {
					r, _, _ := procBlockInput.Call(1)
					blocked = r != 0
				}
				log.Info().Bool("blocked", blocked).Uint64("hwnd", uint64(hwnd)).Msg("worker: privacy ON")
			} else {
				if blocked {
					procBlockInput.Call(0)
					blocked = false
				}
				if hwnd != 0 {
					procDestroyWindow.Call(hwnd)
					hwnd = 0
				}
				log.Info().Msg("worker: privacy OFF")
			}
		case <-ticker.C:
			if hwnd != 0 {
				pumpMessages()
			}
		}
	}
}

func createBlankWindow(className *uint16, hInst uintptr) uintptr {
	x, _, _ := procGetSystemMetricsPriv.Call(smXVirtual)
	y, _, _ := procGetSystemMetricsPriv.Call(smYVirtual)
	w, _, _ := procGetSystemMetricsPriv.Call(smCXVirtual)
	h, _, _ := procGetSystemMetricsPriv.Call(smCYVirtual)
	empty, _ := syscall.UTF16PtrFromString("")
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExTopmost|wsExLayered|wsExTransparent|wsExToolwindow|wsExNoactivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(empty)),
		wsPopup, x, y, w, h, 0, 0, hInst, 0)
	if hwnd == 0 {
		return 0
	}
	procSetLayeredWindowAttributes.Call(hwnd, 0, 255, lwaAlpha)
	procSetWindowDisplayAffinity.Call(hwnd, wdaExcludeFromCapture)
	procShowWindow.Call(hwnd, swShowNoActivate)
	return hwnd
}

func pumpMessages() {
	var m msgStruct
	for {
		r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if r == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
