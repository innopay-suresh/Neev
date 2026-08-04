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
	procGetTextExtent32SB  = modGdi32SB.NewProc("GetTextExtentPoint32W")
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
	rcTalk   cwRect
	hotTalk  bool
	// micOn mirrors whether the host's microphone is currently open. The button
	// is the ONLY way it turns on: a viewer must never be able to open the
	// host's microphone remotely, or the tool becomes a listening device.
	micOn    bool
	hot      bool
	onHangUp func()
	onTalk   func(on bool)
	mu       sync.Mutex
	started  bool
	visible  bool
}

// The one label the bar shows; the window is sized from it.
const barLabel = "Remote session active"

var theBar = &sessionBar{scale: 1}

// measure returns the rendered width of s in the bar's font. Uses a scratch
// screen DC so the window can be sized BEFORE it exists.
func (b *sessionBar) measure(s string) int32 {
	dc, _, _ := procGetDCSB.Call(0)
	if dc == 0 {
		return b.px(150) // fall back to something generous rather than clipping
	}
	defer procReleaseDCSB.Call(0, dc)
	f := b.font(11, true)
	defer procDeleteObjectSB.Call(f)
	old, _, _ := procSelectObjectSB.Call(dc, f)
	u16, _ := syscall.UTF16FromString(s)
	var sz struct{ CX, CY int32 }
	procGetTextExtent32SB.Call(dc, uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(u16)-1), uintptr(unsafe.Pointer(&sz)))
	procSelectObjectSB.Call(dc, old)
	return sz.CX
}

func (b *sessionBar) px(v int) int32 { return int32(float64(v) * b.scale) }

// showHostSessionBar displays the host's session indicator, starting its window on
// first use. onHangUp runs when the host clicks Disconnect.
func showHostSessionBar(onHangUp func()) {
	showHostSessionBarWithVoice(onHangUp, nil)
}

// showHostSessionBarWithVoice adds a host-controlled microphone toggle.
func showHostSessionBarWithVoice(onHangUp func(), onTalk func(bool)) {
	b := theBar
	b.mu.Lock()
	b.onHangUp = onHangUp
	b.onTalk = onTalk
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

	// Width MEASURED from the label, not hardcoded. A fixed 212px was narrower
	// than "Remote session active" renders at some DPI/font combinations, so the
	// text was clipped and the leading "R" disappeared.
	h := b.px(38)
	w := b.px(30) + b.measure(barLabel) + b.px(12) + b.px(64) + b.px(6) + b.px(76) + b.px(10)
	sw, _, _ := procGetSystemMetricsSB.Call(cwSMCXScreen)
	// TOP-centre, as requested. Note this can sit over a maximised app's own
	// header; it is deliberately short and only visible while a session is live.
	x := (int32(sw) - w) / 2
	y := b.px(8)

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
		h := ptInRect(b.rcHangUp, x, y)
		ht := ptInRect(b.rcTalk, x, y)
		if h != b.hot || ht != b.hotTalk {
			b.hot, b.hotTalk = h, ht
			procInvalidateRectSB.Call(hwnd, 0, 0)
		}
		return 0
	case cwWMLButtonDown:
		x, y := int32(lparam&0xFFFF), int32((lparam>>16)&0xFFFF)
		if ptInRect(b.rcTalk, x, y) {
			b.mu.Lock()
			b.micOn = !b.micOn
			on := b.micOn
			cb := b.onTalk
			b.mu.Unlock()
			if cb != nil {
				go cb(on)
			}
			procInvalidateRectSB.Call(hwnd, 0, 0)
			return 0
		}
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
	b.text(hdc, barLabel,
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

	// Talk button — the host's own microphone control, and the only way it can
	// be opened. Label states the CURRENT state ("Mic on") rather than the
	// action, so a host glancing at the bar can tell at once whether they are
	// being heard.
	tw := b.px(64)
	b.rcTalk = cwRect{b.rcHangUp.Left - tw - b.px(6), (rc.Bottom - bh) / 2,
		b.rcHangUp.Left - b.px(6), (rc.Bottom-bh)/2 + bh}
	b.mu.Lock()
	micOn := b.micOn
	b.mu.Unlock()
	tfill := cwColTint
	if micOn {
		tfill = cwColAccent
	}
	if b.hotTalk {
		tfill = cwColAccentDim
	}
	tb, _, _ := procCreateSolidBrushSB.Call(tfill)
	tp, _, _ := procCreatePenSB.Call(5, 0, 0)
	ob4, _, _ := procSelectObjectSB.Call(hdc, tb)
	op4, _, _ := procSelectObjectSB.Call(hdc, tp)
	procRoundRectSB.Call(hdc, uintptr(b.rcTalk.Left), uintptr(b.rcTalk.Top),
		uintptr(b.rcTalk.Right), uintptr(b.rcTalk.Bottom),
		uintptr(b.px(7)), uintptr(b.px(7)))
	procSelectObjectSB.Call(hdc, ob4)
	procSelectObjectSB.Call(hdc, op4)
	procDeleteObjectSB.Call(tb)
	procDeleteObjectSB.Call(tp)
	tcol := cwColInk
	if micOn || b.hotTalk {
		tcol = cwColCard
	}
	label := "Mic off"
	if micOn {
		label = "Mic on"
	}
	b.text(hdc, label, b.rcTalk, font, tcol)
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
