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

// Host-side chat window for TransportMode. In the seamless model the app no
// longer hosts, so the worker renders a small native window on the host desktop:
// a read-only log + an input box + Send. Incoming viewer messages ({k:'chat'})
// are shown; the host's replies are streamed back over ipc.KindChat → transport
// → viewers. The viewer, controlling the host, can also see/use it on-screen.

var (
	modUser32Chat        = syscall.NewLazyDLL("user32.dll")
	procCreateWindowExWC = modUser32Chat.NewProc("CreateWindowExW")
	procDefWindowProcWC  = modUser32Chat.NewProc("DefWindowProcW")
	procRegisterClassWC  = modUser32Chat.NewProc("RegisterClassW")
	procShowWindowC      = modUser32Chat.NewProc("ShowWindow")
	procSetForegroundWC  = modUser32Chat.NewProc("SetForegroundWindow")
	procBringToTopC      = modUser32Chat.NewProc("BringWindowToTop")
	procMoveWindowC      = modUser32Chat.NewProc("MoveWindow")
	procSendMessageWC    = modUser32Chat.NewProc("SendMessageW")
	procGetWindowTextWC  = modUser32Chat.NewProc("GetWindowTextW")
	procSetWindowTextWC  = modUser32Chat.NewProc("SetWindowTextW")
	procGetClientRectC   = modUser32Chat.NewProc("GetClientRect")
	procPeekMessageWC    = modUser32Chat.NewProc("PeekMessageW")
	procTranslateMessageC = modUser32Chat.NewProc("TranslateMessage")
	procDispatchMessageWC = modUser32Chat.NewProc("DispatchMessageW")
	procLoadCursorWC     = modUser32Chat.NewProc("LoadCursorW")
	procGetStockObjectC  = syscall.NewLazyDLL("gdi32.dll").NewProc("GetStockObject")
	procGetModuleHandleWC = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsVScroll      = 0x00200000
	wsBorder       = 0x00800000
	wsOverlapped   = 0x00CF0000 // WS_OVERLAPPEDWINDOW
	esMultiline    = 0x0004
	esReadonly     = 0x0800
	esAutoVScroll  = 0x0040
	esAutoHScroll  = 0x0080
	wmCommand      = 0x0111
	wmSize         = 0x0005
	wmClose        = 0x0010
	wmSetFont      = 0x0030
	emSetSel       = 0x00B1
	emReplaceSel   = 0x00C2
	swHide         = 0
	swShow         = 5
	colorWindow    = 5
	idcArrow       = 32512
	defaultGUIFont = 17
	sendBtnID      = 101
)

var (
	chatIncoming = make(chan string, 64)
	chatStart    sync.Once
	chatSend     func(string) // set once; streams a host reply to the viewer

	chatParent uintptr
	chatLog    uintptr
	chatInput  uintptr
	chatBtn    uintptr
)

// chatEnsure starts the window (once) and wires the reply sender.
func chatEnsure(send func(string)) {
	chatStart.Do(func() {
		chatSend = send
		go chatLoop()
	})
}

// chatShow queues an incoming viewer message for display.
func chatShow(text string) {
	chatEnsureShown()
	select {
	case chatIncoming <- text:
	default:
	}
}

// chatEnsureShown pops the window to the foreground when a message arrives.
// Force it visible AND foreground/top — the plain ShowWindow could leave it
// behind other windows or minimised, which reads as "message received but no
// popup". SW_RESTORE (9) un-minimises; SetForegroundWindow + BringWindowToTop
// raise it.
func chatEnsureShown() {
	if chatParent != 0 {
		procShowWindowC.Call(chatParent, 9 /*SW_RESTORE*/)
		procBringToTopC.Call(chatParent)
		procSetForegroundWC.Call(chatParent)
	}
}

func chatWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmCommand:
		if (wparam&0xFFFF) == sendBtnID && chatInput != 0 {
			chatSendFromInput()
			return 0
		}
	case wmSize:
		chatLayout(hwnd)
		return 0
	case wmClose:
		procShowWindowC.Call(hwnd, swHide) // hide, don't destroy
		return 0
	}
	r, _, _ := procDefWindowProcWC.Call(hwnd, msg, wparam, lparam)
	return r
}

func chatLoop() {
	runtime.LockOSThread()
	// Bind to the interactive input desktop so window creation is permitted — a
	// service-spawned worker can be denied GUI even though SendInput works.
	bindInputDesktop()
	className, _ := syscall.UTF16PtrFromString("NeevChatWindow")
	editClass, _ := syscall.UTF16PtrFromString("EDIT")
	btnClass, _ := syscall.UTF16PtrFromString("BUTTON")
	title, _ := syscall.UTF16PtrFromString("Neev Remote — Chat")
	sendLabel, _ := syscall.UTF16PtrFromString("Send")
	hInst, _, _ := procGetModuleHandleWC.Call(0)
	cursor, _, _ := procLoadCursorWC.Call(0, idcArrow)
	font, _, _ := procGetStockObjectC.Call(defaultGUIFont)

	wc := wndClassW{
		lpfnWndProc:   syscall.NewCallback(chatWndProc),
		hInstance:     hInst,
		hCursor:       cursor,
		hbrBackground: uintptr(colorWindow + 1), // (HBRUSH)(COLOR_WINDOW+1)
		lpszClassName: className,
	}
	atom, _, regErr := procRegisterClassWC.Call(uintptr(unsafe.Pointer(&wc)))

	// Compact chat window, docked to the top-right of the primary screen so it
	// doesn't cover the host's work area (it's resizable/movable if needed).
	const winW, winH = 300, 380
	scrW, _, _ := procGetSystemMetricsPriv.Call(0) // SM_CXSCREEN
	x := int(scrW) - winW - 24
	if x < 0 {
		x = 24
	}
	parent, _, createErr := procCreateWindowExWC.Call(0,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		wsOverlapped, uintptr(x), 48, winW, winH, 0, 0, hInst, 0)
	if parent == 0 {
		log.Warn().Uint64("atom", uint64(atom)).
			Str("regErr", regErr.Error()).Str("createErr", createErr.Error()).
			Msg("worker: chat window create failed")
		return
	}
	chatParent = parent
	log.Info().Uint64("hwnd", uint64(parent)).Msg("worker: chat window created")
	chatLog, _, _ = procCreateWindowExWC.Call(0,
		uintptr(unsafe.Pointer(editClass)), 0,
		wsChild|wsVisible|wsVScroll|wsBorder|esMultiline|esReadonly|esAutoVScroll,
		0, 0, 0, 0, parent, 0, hInst, 0)
	chatInput, _, _ = procCreateWindowExWC.Call(0,
		uintptr(unsafe.Pointer(editClass)), 0,
		wsChild|wsVisible|wsBorder|esAutoHScroll,
		0, 0, 0, 0, parent, 0, hInst, 0)
	chatBtn, _, _ = procCreateWindowExWC.Call(0,
		uintptr(unsafe.Pointer(btnClass)), uintptr(unsafe.Pointer(sendLabel)),
		wsChild|wsVisible, 0, 0, 0, 0, parent, sendBtnID, hInst, 0)
	for _, h := range []uintptr{chatLog, chatInput, chatBtn} {
		procSendMessageWC.Call(h, wmSetFont, font, 1)
	}
	chatLayout(parent)
	procShowWindowC.Call(parent, swShow)

	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	var m msgStruct
	for {
		select {
		case text := <-chatIncoming:
			chatAppend("Viewer: " + text)
			procShowWindowC.Call(chatParent, swShow)
		case <-ticker.C:
			for {
				r, _, _ := procPeekMessageWC.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
				if r == 0 {
					break
				}
				procTranslateMessageC.Call(uintptr(unsafe.Pointer(&m)))
				procDispatchMessageWC.Call(uintptr(unsafe.Pointer(&m)))
			}
		}
	}
}

// chatLayout positions the log (fills top), input (bottom-left) and Send button.
func chatLayout(hwnd uintptr) {
	var rc struct{ left, top, right, bottom int32 }
	procGetClientRectC.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := int(rc.right - rc.left)
	h := int(rc.bottom - rc.top)
	if w <= 0 || h <= 0 {
		return
	}
	const pad, inputH, btnW = 8, 26, 72
	logH := h - inputH - pad*3
	procMoveWindowC.Call(chatLog, pad, pad, uintptr(w-pad*2), uintptr(logH), 1)
	inputY := h - inputH - pad
	procMoveWindowC.Call(chatInput, pad, uintptr(inputY), uintptr(w-btnW-pad*3), inputH, 1)
	procMoveWindowC.Call(chatBtn, uintptr(w-btnW-pad), uintptr(inputY), btnW, inputH, 1)
}

// chatSendFromInput reads the input box, echoes it, clears it, and streams the
// reply to the viewer.
func chatSendFromInput() {
	buf := make([]uint16, 2048)
	n, _, _ := procGetWindowTextWC.Call(chatInput, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return
	}
	text := syscall.UTF16ToString(buf[:n])
	if text == "" {
		return
	}
	empty, _ := syscall.UTF16PtrFromString("")
	procSetWindowTextWC.Call(chatInput, uintptr(unsafe.Pointer(empty)))
	chatAppend("You: " + text)
	if chatSend != nil {
		chatSend(text)
	}
}

// chatAppend appends a line to the read-only log (caret to end, then insert).
func chatAppend(line string) {
	if chatLog == 0 {
		return
	}
	s, err := syscall.UTF16PtrFromString(line + "\r\n")
	if err != nil {
		return
	}
	procSendMessageWC.Call(chatLog, emSetSel, ^uintptr(0), ^uintptr(0)) // (-1,-1) = end
	procSendMessageWC.Call(chatLog, emReplaceSel, 0, uintptr(unsafe.Pointer(s)))
}
