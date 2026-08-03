//go:build windows

package session

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// A small always-on-top bar shown on the HOST while a viewer is connected:
// "Remote session active" plus a Disconnect button.
//
// Why this exists: only the VIEWER could hang up. The person whose screen was
// being watched had no way to end it — the wrong way round for a remote-access
// tool, and worse in TransportMode where the host is a SYSTEM service with no
// Flutter UI at all. The bar is the host's hang-up control, and it doubles as
// the honest indicator that a session is live.
//
// Deliberately small, topmost and non-activating, so it never steals focus from
// whatever the user is doing while someone watches.

var (
	modUser32SB = syscall.NewLazyDLL("user32.dll")

	procRegisterClassSB    = modUser32SB.NewProc("RegisterClassW")
	procCreateWindowExSB   = modUser32SB.NewProc("CreateWindowExW")
	procDefWindowProcSB    = modUser32SB.NewProc("DefWindowProcW")
	procShowWindowSB       = modUser32SB.NewProc("ShowWindow")
	procDestroyWindowSB    = modUser32SB.NewProc("DestroyWindow")
	procGetMessageSB       = modUser32SB.NewProc("GetMessageW")
	procTranslateMessageSB = modUser32SB.NewProc("TranslateMessage")
	procDispatchMessageSB  = modUser32SB.NewProc("DispatchMessageW")
	procBeginPaintSB       = modUser32SB.NewProc("BeginPaint")
	procEndPaintSB         = modUser32SB.NewProc("EndPaint")
	procFillRectSB         = modUser32SB.NewProc("FillRect")
	procGetClientRectSB    = modUser32SB.NewProc("GetClientRect")
	procInvalidateRectSB   = modUser32SB.NewProc("InvalidateRect")
	procDrawTextSB         = modUser32SB.NewProc("DrawTextW")
	procGetSystemMetricsSB = modUser32SB.NewProc("GetSystemMetrics")
	procLoadCursorSB       = modUser32SB.NewProc("LoadCursorW")
	procSetWindowPosSB     = modUser32SB.NewProc("SetWindowPos")
	procGetDCSB            = modUser32SB.NewProc("GetDC")
	procReleaseDCSB        = modUser32SB.NewProc("ReleaseDC")

	modGdi32SB             = syscall.NewLazyDLL("gdi32.dll")
	procCreateSolidBrushSB = modGdi32SB.NewProc("CreateSolidBrush")
	procCreatePenSB        = modGdi32SB.NewProc("CreatePen")
	procSelectObjectSB     = modGdi32SB.NewProc("SelectObject")
	procDeleteObjectSB     = modGdi32SB.NewProc("DeleteObject")
	procCreateFontSB       = modGdi32SB.NewProc("CreateFontW")
	procSetTextColorSB     = modGdi32SB.NewProc("SetTextColor")
	procSetBkModeSB        = modGdi32SB.NewProc("SetBkMode")
	procRoundRectSB        = modGdi32SB.NewProc("RoundRect")
	procEllipseSB          = modGdi32SB.NewProc("Ellipse")
	procGetDeviceCapsSB    = modGdi32SB.NewProc("GetDeviceCaps")
	procCreateCompatDCSB   = modGdi32SB.NewProc("CreateCompatibleDC")
	procCreateCompatBmpSB  = modGdi32SB.NewProc("CreateCompatibleBitmap")
	procBitBltSB           = modGdi32SB.NewProc("BitBlt")
	procDeleteDCSB         = modGdi32SB.NewProc("DeleteDC")
)

const (
	sbWSPopup        = 0x80000000
	sbWSExTopmost    = 0x00000008
	sbWSExToolWindow = 0x00000080
	sbWSExNoActivate = 0x08000000
	sbSWShow         = 5
	sbSWHide         = 0
)

type sessionBar struct {
	hwnd     uintptr
	scale    float64
	rcHangUp cwRect
	hot      bool
	onHangUp func()
	mu       sync.Mutex
	started  bool
	visible  bool
}

var theBar = &sessionBar{scale: 1}

func (b *sessionBar) px(v int) int32 { return int32(float64(v) * b.scale) }

// showHostSessionBar displays the host's session indicator, starting its window on
// first use. onHangUp runs when the host clicks Disconnect.
func showHostSessionBar(onHangUp func()) {
	b := theBar
	b.mu.Lock()
	b.onHangUp = onHangUp
	already := b.started
	b.started = true
	b.mu.Unlock()
	if !already {
		go b.loop()
		return
	}
	if b.hwnd != 0 {
		procShowWindowSB.Call(b.hwnd, sbSWShow)
		b.visible = true
	}
}

// hideHostSessionBar removes the indicator when no viewer is connected.
func hideHostSessionBar() {
	b := theBar
	if b.hwnd != 0 {
		procShowWindowSB.Call(b.hwnd, sbSWHide)
		b.visible = false
	}
}

func (b *sessionBar) loop() {
	runtime.LockOSThread()
	// A service-spawned worker is denied GUI unless bound to the input desktop.
	bindInputDesktop()

	if dc, _, _ := procGetDCSB.Call(0); dc != 0 {
		if dpi, _, _ := procGetDeviceCapsSB.Call(dc, cwLogPixelsX); dpi > 0 {
			b.scale = float64(dpi) / 96.0
		}
		procReleaseDCSB.Call(0, dc)
	}

	className, _ := syscall.UTF16PtrFromString("NeevSessionBar")
	cursor, _, _ := procLoadCursorSB.Call(0, cwIDCArrow)
	bg, _, _ := procCreateSolidBrushSB.Call(cwColCard)
	wc := cwWndClass{
		WndProc:    syscall.NewCallback(sessionBarProc),
		Cursor:     cursor,
		Background: bg,
		ClassName:  className,
	}
	procRegisterClassSB.Call(uintptr(unsafe.Pointer(&wc)))

	w := b.px(212)
	h := b.px(38)
	sw, _, _ := procGetSystemMetricsSB.Call(cwSMCXScreen)
	sh, _, _ := procGetSystemMetricsSB.Call(cwSMCYScreen)
	// BOTTOM-centre, lifted clear of the taskbar. Top-centre put it straight on
	// top of the app's own search bar — an always-on-top indicator must not
	// cover the UI it is reporting on. Bottom-centre is out of the way of both
	// the header and the taskbar, and it is where screen-sharing indicators are
	// conventionally expected.
	x := (int32(sw) - w) / 2
	y := int32(sh) - h - b.px(64)

	title, _ := syscall.UTF16PtrFromString("Neev Remote")
	hwnd, _, _ := procCreateWindowExSB.Call(
		sbWSExTopmost|sbWSExToolWindow|sbWSExNoActivate,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		sbWSPopup,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return
	}
	b.hwnd = hwnd
	procShowWindowSB.Call(hwnd, sbSWShow)
	b.visible = true

	var msg cwMsg
	for {
		r, _, _ := procGetMessageSB.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessageSB.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageSB.Call(uintptr(unsafe.Pointer(&msg)))
	}
	procDestroyWindowSB.Call(hwnd)
}

func sessionBarProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	b := theBar
	switch msg {
	case cwWMPaint:
		b.paint(hwnd)
		return 0
	case cwWMEraseBkgnd:
		return 1 // painted in full via the back buffer; no system erase flash
	case cwWMMouseMove:
		x, y := int32(lparam&0xFFFF), int32((lparam>>16)&0xFFFF)
		if h := ptInRect(b.rcHangUp, x, y); h != b.hot {
			b.hot = h
			procInvalidateRectSB.Call(hwnd, 0, 0)
		}
		return 0
	case cwWMLButtonDown:
		x, y := int32(lparam&0xFFFF), int32((lparam>>16)&0xFFFF)
		if ptInRect(b.rcHangUp, x, y) {
			b.mu.Lock()
			cb := b.onHangUp
			b.mu.Unlock()
			if cb != nil {
				go cb()
			}
			procShowWindowSB.Call(hwnd, sbSWHide)
			b.visible = false
		}
		return 0
	case cwWMClose:
		// Hiding the bar must NOT end the session silently; the user closes it
		// to get it out of the way, and the session is still live.
		procShowWindowSB.Call(hwnd, sbSWHide)
		b.visible = false
		return 0
	}
	r, _, _ := procDefWindowProcSB.Call(hwnd, msg, wparam, lparam)
	return r
}

func (b *sessionBar) paint(hwnd uintptr) {
	var ps cwPaintStruct
	screen, _, _ := procBeginPaintSB.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaintSB.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc cwRect
	procGetClientRectSB.Call(hwnd, uintptr(unsafe.Pointer(&rc)))

	// Double buffered for the same reason as the consent card: this repaints on
	// every hover of the Disconnect button, and it sits on top of whatever the
	// user is doing, so an in-place redraw is very visible.
	hdc := screen
	memDC, _, _ := procCreateCompatDCSB.Call(screen)
	var memBmp, oldBmp uintptr
	if memDC != 0 {
		memBmp, _, _ = procCreateCompatBmpSB.Call(screen,
			uintptr(rc.Right), uintptr(rc.Bottom))
		if memBmp != 0 {
			oldBmp, _, _ = procSelectObjectSB.Call(memDC, memBmp)
			hdc = memDC
		}
	}
	defer func() {
		if hdc == memDC {
			procBitBltSB.Call(screen, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom),
				memDC, 0, 0, cwSrcCopy)
		}
		if oldBmp != 0 {
			procSelectObjectSB.Call(memDC, oldBmp)
		}
		if memBmp != 0 {
			procDeleteObjectSB.Call(memBmp)
		}
		if memDC != 0 {
			procDeleteDCSB.Call(memDC)
		}
	}()

	bg, _, _ := procCreateSolidBrushSB.Call(cwColCard)
	procFillRectSB.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
	procDeleteObjectSB.Call(bg)

	// Card body + border.
	brush, _, _ := procCreateSolidBrushSB.Call(cwColCard)
	pen, _, _ := procCreatePenSB.Call(0, uintptr(b.px(1)), cwColBorderStr)
	ob, _, _ := procSelectObjectSB.Call(hdc, brush)
	op, _, _ := procSelectObjectSB.Call(hdc, pen)
	procRoundRectSB.Call(hdc, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom),
		uintptr(b.px(10)), uintptr(b.px(10)))
	procSelectObjectSB.Call(hdc, ob)
	procSelectObjectSB.Call(hdc, op)
	procDeleteObjectSB.Call(brush)
	procDeleteObjectSB.Call(pen)

	// Live dot.
	dot := b.px(8)
	dy := (rc.Bottom - dot) / 2
	dbrush, _, _ := procCreateSolidBrushSB.Call(cwColAccent)
	dpen, _, _ := procCreatePenSB.Call(5, 0, 0)
	ob2, _, _ := procSelectObjectSB.Call(hdc, dbrush)
	op2, _, _ := procSelectObjectSB.Call(hdc, dpen)
	procEllipseSB.Call(hdc, uintptr(b.px(12)), uintptr(dy),
		uintptr(b.px(12)+dot), uintptr(dy+dot))
	procSelectObjectSB.Call(hdc, ob2)
	procSelectObjectSB.Call(hdc, op2)
	procDeleteObjectSB.Call(dbrush)
	procDeleteObjectSB.Call(dpen)

	font := b.font(11, true)
	defer procDeleteObjectSB.Call(font)
	b.text(hdc, "Remote session active",
		cwRect{b.px(26), 0, rc.Right - b.px(90), rc.Bottom},
		font, cwColInk)

	// Disconnect button.
	bw, bh := b.px(76), b.px(24)
	b.rcHangUp = cwRect{rc.Right - bw - b.px(8), (rc.Bottom - bh) / 2,
		rc.Right - b.px(8), (rc.Bottom-bh)/2 + bh}
	fill := cwColDanger
	if b.hot {
		fill = cwColAccentDim
	}
	fb, _, _ := procCreateSolidBrushSB.Call(fill)
	fp, _, _ := procCreatePenSB.Call(5, 0, 0)
	ob3, _, _ := procSelectObjectSB.Call(hdc, fb)
	op3, _, _ := procSelectObjectSB.Call(hdc, fp)
	procRoundRectSB.Call(hdc, uintptr(b.rcHangUp.Left), uintptr(b.rcHangUp.Top),
		uintptr(b.rcHangUp.Right), uintptr(b.rcHangUp.Bottom),
		uintptr(b.px(7)), uintptr(b.px(7)))
	procSelectObjectSB.Call(hdc, ob3)
	procSelectObjectSB.Call(hdc, op3)
	procDeleteObjectSB.Call(fb)
	procDeleteObjectSB.Call(fp)
	b.text(hdc, "Disconnect", b.rcHangUp, font, cwColCard)
}

func (b *sessionBar) font(size int, bold bool) uintptr {
	weight := uintptr(400)
	if bold {
		weight = 700
	}
	face, _ := syscall.UTF16PtrFromString("Segoe UI")
	f, _, _ := procCreateFontSB.Call(uintptr(-b.px(size)), 0, 0, 0, weight,
		0, 0, 0, 0, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	return f
}

func (b *sessionBar) text(hdc uintptr, s string, r cwRect, f, col uintptr) {
	old, _, _ := procSelectObjectSB.Call(hdc, f)
	procSetTextColorSB.Call(hdc, col)
	procSetBkModeSB.Call(hdc, cwTransparent)
	u16, _ := syscall.UTF16PtrFromString(s)
	rr := r
	flags := uintptr(cwDTVCenter | cwDTSingleLine)
	if r.Left > 0 && r.Right-r.Left < 200 {
		flags |= cwDTCenter
	}
	procDrawTextSB.Call(hdc, uintptr(unsafe.Pointer(u16)), ^uintptr(0),
		uintptr(unsafe.Pointer(&rr)), flags)
	procSelectObjectSB.Call(hdc, old)
}
