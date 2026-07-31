//go:build windows

package session

import (
	"syscall"
	"unsafe"
)

// A hand-drawn consent window, replacing the stock MessageBoxW.
//
// MessageBoxW cannot render the approved design: no brand mark, no accent
// colour, no device-id block, no copy button and no "Remember this decision"
// checkbox. Everything here is owner-drawn in WM_PAINT with GDI, and the three
// hit targets (Decline / Accept / checkbox / copy) are plain rectangles we
// hit-test ourselves, so there are no stock control themes to fight.
//
// Runs on the caller's thread, which showConsentDialog has already locked and
// bound to the interactive input desktop. Pumps its own modal message loop and
// returns once the user answers.

var (
	modUser32CW = syscall.NewLazyDLL("user32.dll")
	modGdi32CW  = syscall.NewLazyDLL("gdi32.dll")

	procRegisterClassCW    = modUser32CW.NewProc("RegisterClassW")
	procCreateWindowExCW   = modUser32CW.NewProc("CreateWindowExW")
	procDefWindowProcCW    = modUser32CW.NewProc("DefWindowProcW")
	procDestroyWindowCW    = modUser32CW.NewProc("DestroyWindow")
	procGetMessageCW       = modUser32CW.NewProc("GetMessageW")
	procTranslateMessageCW = modUser32CW.NewProc("TranslateMessage")
	procDispatchMessageCW  = modUser32CW.NewProc("DispatchMessageW")
	procPostQuitMessageCW  = modUser32CW.NewProc("PostQuitMessage")
	procBeginPaintCW       = modUser32CW.NewProc("BeginPaint")
	procEndPaintCW         = modUser32CW.NewProc("EndPaint")
	procFillRectCW         = modUser32CW.NewProc("FillRect")
	procInvalidateRectCW   = modUser32CW.NewProc("InvalidateRect")
	procGetClientRectCW    = modUser32CW.NewProc("GetClientRect")
	procDrawTextCW         = modUser32CW.NewProc("DrawTextW")
	procLoadCursorCW       = modUser32CW.NewProc("LoadCursorW")
	procSetCursorCW        = modUser32CW.NewProc("SetCursor")
	procSetForegroundWinCW = modUser32CW.NewProc("SetForegroundWindow")
	procGetSystemMetricsCW = modUser32CW.NewProc("GetSystemMetrics")
	procSetWindowPosCW     = modUser32CW.NewProc("SetWindowPos")
	procGetDCCW            = modUser32CW.NewProc("GetDC")
	procReleaseDCCW        = modUser32CW.NewProc("ReleaseDC")
	procOpenClipboardCW    = modUser32CW.NewProc("OpenClipboard")
	procCloseClipboardCW   = modUser32CW.NewProc("CloseClipboard")
	procEmptyClipboardCW   = modUser32CW.NewProc("EmptyClipboard")
	procSetClipboardDataCW = modUser32CW.NewProc("SetClipboardData")

	procCreateSolidBrushCW = modGdi32CW.NewProc("CreateSolidBrush")
	procCreatePenCW        = modGdi32CW.NewProc("CreatePen")
	procSelectObjectCW     = modGdi32CW.NewProc("SelectObject")
	procDeleteObjectCW     = modGdi32CW.NewProc("DeleteObject")
	procCreateFontCW       = modGdi32CW.NewProc("CreateFontW")
	procSetTextColorCW     = modGdi32CW.NewProc("SetTextColor")
	procSetBkModeCW        = modGdi32CW.NewProc("SetBkMode")
	procRoundRectCW        = modGdi32CW.NewProc("RoundRect")
	procEllipseCW          = modGdi32CW.NewProc("Ellipse")
	procRectangleCW        = modGdi32CW.NewProc("Rectangle")
	procMoveToExCW         = modGdi32CW.NewProc("MoveToEx")
	procLineToCW           = modGdi32CW.NewProc("LineTo")
	procGetDeviceCapsCW    = modGdi32CW.NewProc("GetDeviceCaps")
	procPolygonCW          = modGdi32CW.NewProc("Polygon")

	modKernel32CW      = syscall.NewLazyDLL("kernel32.dll")
	procGlobalAllocCW  = modKernel32CW.NewProc("GlobalAlloc")
	procGlobalLockCW   = modKernel32CW.NewProc("GlobalLock")
	procGlobalUnlockCW = modKernel32CW.NewProc("GlobalUnlock")
)

const (
	cwWSPopup       = 0x80000000
	cwWSVisible     = 0x10000000
	cwWSExTopmost   = 0x00000008
	cwWSExToolWin   = 0x00000080
	cwWMDestroy     = 0x0002
	cwWMPaint       = 0x000F
	cwWMClose       = 0x0010
	cwWMSetCursor   = 0x0020
	cwWMMouseMove   = 0x0200
	cwWMLButtonDown = 0x0201
	cwWMLButtonUp   = 0x0202
	cwWMKeyDown     = 0x0100
	cwVKEscape      = 0x1B
	cwVKReturn      = 0x0D
	cwIDCArrow      = 32512
	cwIDCHand       = 32649
	cwTransparent   = 1
	cwDTLeft        = 0x0000
	cwDTCenter      = 0x0001
	cwDTVCenter     = 0x0004
	cwDTSingleLine  = 0x0020
	cwDTWordBreak   = 0x0010
	cwSMCXScreen    = 0
	cwSMCYScreen    = 1
	cwLogPixelsX    = 88
	cwCFUnicodeText = 13
	cwGMemMoveable  = 0x0002
)

type cwRect struct{ Left, Top, Right, Bottom int32 }
type cwPoint struct{ X, Y int32 }

type cwPaintStruct struct {
	Hdc         uintptr
	Erase       int32
	RcPaint     cwRect
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type cwMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      cwPoint
}

type cwWndClass struct {
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
}

// rgb packs a colour the way GDI wants it (0x00BBGGRR).
func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

// Palette — the approved design tokens (DESIGN.md), not the mockup's raw pixels.
var (
	cwColCard      = rgb(0xFF, 0xFF, 0xFF)
	cwColInk       = rgb(0x17, 0x17, 0x14)
	cwColInkSoft   = rgb(0x77, 0x72, 0x66)
	cwColAccent    = rgb(0xF0, 0x5A, 0x28)
	cwColAccentDim = rgb(0xC9, 0x44, 0x18)
	cwColTint      = rgb(0xFC, 0xE5, 0xD9)
	cwColBorder    = rgb(0xDE, 0xD6, 0xC8)
	cwColBorderStr = rgb(0xD0, 0xC6, 0xAC)
)

// consentResult carries the user's answer out of the window procedure.
type consentResult struct {
	allow    bool
	remember bool
	answered bool
}

type consentWin struct {
	hwnd     uintptr
	deviceID string
	scale    float64
	res      consentResult
	// Hit rectangles, recomputed on every paint so they always match what is
	// actually drawn (no second source of truth to drift).
	rcAccept, rcDecline, rcRemember, rcCopy, rcClose cwRect
	hotAccept, hotDecline                            bool
	copied                                           bool
}

func (c *consentWin) px(v int) int32 { return int32(float64(v) * c.scale) }

// showConsentWindow displays the consent card and blocks until answered.
// Returns allow + whether to remember the decision.
func showConsentWindow(viewerID string) (allow bool, remember bool) {
	c := &consentWin{deviceID: prettyConsentID(viewerID), scale: 1}

	// DPI: the worker is not per-monitor DPI aware, so use the desktop DC's
	// logical pixel count. On a 150% display this keeps the card from rendering
	// at a third of its intended size.
	if dc, _, _ := procGetDCCW.Call(0); dc != 0 {
		if dpi, _, _ := procGetDeviceCapsCW.Call(dc, cwLogPixelsX); dpi > 0 {
			c.scale = float64(dpi) / 96.0
		}
		procReleaseDCCW.Call(0, dc)
	}

	className, _ := syscall.UTF16PtrFromString("NeevConsentWindow")
	wndProc := syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
		return c.proc(hwnd, msg, wparam, lparam)
	})
	cursor, _, _ := procLoadCursorCW.Call(0, cwIDCArrow)
	bg, _, _ := procCreateSolidBrushCW.Call(cwColCard)
	wc := cwWndClass{
		WndProc:    wndProc,
		Cursor:     cursor,
		Background: bg,
		ClassName:  className,
	}
	// A repeat connection re-registers the same class; that returns 0 with
	// ERROR_CLASS_ALREADY_EXISTS, which is fine — CreateWindowEx still works.
	procRegisterClassCW.Call(uintptr(unsafe.Pointer(&wc)))

	w := c.px(440)
	h := c.px(384)
	sw, _, _ := procGetSystemMetricsCW.Call(cwSMCXScreen)
	sh, _, _ := procGetSystemMetricsCW.Call(cwSMCYScreen)
	x := (int32(sw) - w) / 2
	y := (int32(sh) - h) / 2

	title, _ := syscall.UTF16PtrFromString("Neev Remote")
	hwnd, _, _ := procCreateWindowExCW.Call(
		cwWSExTopmost|cwWSExToolWin,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		cwWSPopup|cwWSVisible,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		// Could not create the window: fall back to the stock box rather than
		// silently auto-denying a legitimate connection.
		return showConsentMessageBox(viewerID), false
	}
	c.hwnd = hwnd
	procSetForegroundWinCW.Call(hwnd)

	var msg cwMsg
	for {
		r, _, _ := procGetMessageCW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessageCW.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageCW.Call(uintptr(unsafe.Pointer(&msg)))
		if c.res.answered {
			break
		}
	}
	procDestroyWindowCW.Call(hwnd)
	return c.res.allow, c.res.remember
}

func ptInRect(r cwRect, x, y int32) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

func (c *consentWin) answer(allow bool) {
	c.res.allow = allow
	c.res.answered = true
	procPostQuitMessageCW.Call(0)
}

func (c *consentWin) proc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case cwWMPaint:
		c.paint(hwnd)
		return 0
	case cwWMMouseMove:
		x, y := int32(lparam&0xFFFF), int32((lparam>>16)&0xFFFF)
		ha := ptInRect(c.rcAccept, x, y)
		hd := ptInRect(c.rcDecline, x, y)
		if ha != c.hotAccept || hd != c.hotDecline {
			c.hotAccept, c.hotDecline = ha, hd
			procInvalidateRectCW.Call(hwnd, 0, 0)
		}
		return 0
	case cwWMSetCursor:
		return 1 // we set it in WM_MOUSEMOVE territory; keep the arrow
	case cwWMLButtonDown:
		x, y := int32(lparam&0xFFFF), int32((lparam>>16)&0xFFFF)
		switch {
		case ptInRect(c.rcAccept, x, y):
			c.answer(true)
		case ptInRect(c.rcDecline, x, y), ptInRect(c.rcClose, x, y):
			c.answer(false)
		case ptInRect(c.rcRemember, x, y):
			c.res.remember = !c.res.remember
			procInvalidateRectCW.Call(hwnd, 0, 0)
		case ptInRect(c.rcCopy, x, y):
			c.copyDeviceID()
			c.copied = true
			procInvalidateRectCW.Call(hwnd, 0, 0)
		}
		return 0
	case cwWMKeyDown:
		// Esc declines, Enter accepts. Deliberately NOT defaulting Enter to
		// accept-on-open would be safer, but this window is only shown after an
		// explicit incoming request and Esc is the first-listed action.
		if wparam == cwVKEscape {
			c.answer(false)
		} else if wparam == cwVKReturn {
			c.answer(true)
		}
		return 0
	case cwWMClose:
		// Closing the prompt is a refusal, never an accept.
		c.answer(false)
		return 0
	case cwWMDestroy:
		procPostQuitMessageCW.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcCW.Call(hwnd, msg, wparam, lparam)
	return r
}

// copyDeviceID puts the device id on the clipboard (the ⧉ button in the card).
func (c *consentWin) copyDeviceID() {
	u16, err := syscall.UTF16FromString(c.deviceID)
	if err != nil {
		return
	}
	size := uintptr(len(u16) * 2)
	h, _, _ := procGlobalAllocCW.Call(cwGMemMoveable, size)
	if h == 0 {
		return
	}
	p, _, _ := procGlobalLockCW.Call(h)
	if p == 0 {
		return
	}
	// go vet flags the uintptr→Pointer conversion here (as it does for the same
	// idiom in clipimg_windows.go). It is sound: GlobalAlloc memory lives
	// outside the Go heap, so the GC never moves it, and the block stays pinned
	// between GlobalLock and GlobalUnlock.
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u16))
	copy(dst, u16)
	procGlobalUnlockCW.Call(h)
	if r, _, _ := procOpenClipboardCW.Call(c.hwnd); r == 0 {
		return
	}
	procEmptyClipboardCW.Call()
	procSetClipboardDataCW.Call(cwCFUnicodeText, h)
	procCloseClipboardCW.Call()
}

// ---- drawing helpers ------------------------------------------------------

func (c *consentWin) font(size int, bold bool, mono bool) uintptr {
	weight := uintptr(400)
	if bold {
		weight = 700
	}
	name := "Segoe UI"
	if mono {
		// The bundled JetBrains Mono is a Flutter asset the worker can't load,
		// so use the OS monospace for the device id. It still reads as tabular.
		name = "Consolas"
	}
	face, _ := syscall.UTF16PtrFromString(name)
	f, _, _ := procCreateFontCW.Call(
		uintptr(-c.px(size)), 0, 0, 0, weight,
		0, 0, 0, 0, 0, 0, 5 /*CLEARTYPE_QUALITY*/, 0,
		uintptr(unsafe.Pointer(face)),
	)
	return f
}

func (c *consentWin) text(hdc uintptr, s string, r cwRect, f uintptr, col uintptr, flags uintptr) {
	old, _, _ := procSelectObjectCW.Call(hdc, f)
	procSetTextColorCW.Call(hdc, col)
	procSetBkModeCW.Call(hdc, cwTransparent)
	u16, _ := syscall.UTF16PtrFromString(s)
	rr := r
	procDrawTextCW.Call(hdc, uintptr(unsafe.Pointer(u16)), ^uintptr(0),
		uintptr(unsafe.Pointer(&rr)), flags)
	procSelectObjectCW.Call(hdc, old)
}

// roundRect draws a filled rounded rectangle, optionally with a border.
func (c *consentWin) roundRect(hdc uintptr, r cwRect, radius int32, fill uintptr, border uintptr, hasBorder bool) {
	brush, _, _ := procCreateSolidBrushCW.Call(fill)
	var pen uintptr
	if hasBorder {
		pen, _, _ = procCreatePenCW.Call(0 /*PS_SOLID*/, uintptr(c.px(1)), border)
	} else {
		pen, _, _ = procCreatePenCW.Call(5 /*PS_NULL*/, 0, 0)
	}
	ob, _, _ := procSelectObjectCW.Call(hdc, brush)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procRoundRectCW.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
		uintptr(radius), uintptr(radius))
	procSelectObjectCW.Call(hdc, ob)
	procSelectObjectCW.Call(hdc, op)
	procDeleteObjectCW.Call(brush)
	procDeleteObjectCW.Call(pen)
}

func (c *consentWin) line(hdc uintptr, x1, y1, x2, y2 int32, col uintptr) {
	pen, _, _ := procCreatePenCW.Call(0, uintptr(c.px(1)), col)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procMoveToExCW.Call(hdc, uintptr(x1), uintptr(y1), 0)
	procLineToCW.Call(hdc, uintptr(x2), uintptr(y2))
	procSelectObjectCW.Call(hdc, op)
	procDeleteObjectCW.Call(pen)
}

// ---- painting -------------------------------------------------------------

func (c *consentWin) paint(hwnd uintptr) {
	var ps cwPaintStruct
	hdc, _, _ := procBeginPaintCW.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaintCW.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc cwRect
	procGetClientRectCW.Call(hwnd, uintptr(unsafe.Pointer(&rc)))

	// Card background + a hairline frame (the window is a borderless popup).
	bg, _, _ := procCreateSolidBrushCW.Call(cwColCard)
	procFillRectCW.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
	procDeleteObjectCW.Call(bg)
	c.roundRect(hdc, rc, c.px(2), cwColCard, cwColBorderStr, true)

	pad := c.px(26)
	fTitle := c.font(21, true, false)
	fH1 := c.font(19, true, false)
	fBody := c.font(12, false, false)
	fLabel := c.font(11, true, false)
	fID := c.font(20, true, true)
	fBtn := c.font(13, true, false)
	fSmall := c.font(11, false, false)
	defer func() {
		for _, f := range []uintptr{fTitle, fH1, fBody, fLabel, fID, fBtn, fSmall} {
			procDeleteObjectCW.Call(f)
		}
	}()

	// ---- header: brand mark + wordmark + close ----
	top := c.px(18)
	markSize := c.px(22)
	mark := cwRect{pad, top, pad + markSize, top + markSize}
	c.roundRect(hdc, mark, c.px(6), cwColAccent, 0, false)
	c.text(hdc, "N", mark, c.font(13, true, false), cwColCard, cwDTCenter|cwDTVCenter|cwDTSingleLine)
	c.text(hdc, "Neev Remote",
		cwRect{pad + markSize + c.px(9), top - c.px(1), rc.Right - pad, top + markSize},
		fTitle, cwColInk, cwDTLeft|cwDTVCenter|cwDTSingleLine)

	closeSz := c.px(24)
	c.rcClose = cwRect{rc.Right - pad - closeSz, top, rc.Right - pad, top + closeSz}
	cx, cy, k := (c.rcClose.Left+c.rcClose.Right)/2, (c.rcClose.Top+c.rcClose.Bottom)/2, c.px(5)
	c.line(hdc, cx-k, cy-k, cx+k, cy+k, cwColInkSoft)
	c.line(hdc, cx+k, cy-k, cx-k, cy+k, cwColInkSoft)

	// ---- icon medallion + heading ----
	medY := top + c.px(42)
	medSz := c.px(84)
	c.medallion(hdc, pad, medY, medSz)

	textLeft := pad + medSz + c.px(22)
	c.text(hdc, "Connection Request",
		cwRect{textLeft, medY - c.px(2), rc.Right - pad, medY + c.px(28)},
		fH1, cwColInk, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	c.text(hdc, "A remote device is requesting to connect and control this computer.",
		cwRect{textLeft, medY + c.px(28), rc.Right - pad, medY + c.px(84)},
		fBody, cwColInkSoft, cwDTLeft|cwDTWordBreak)

	// ---- device id ----
	idY := medY + medSz + c.px(14)
	c.text(hdc, "Device ID", cwRect{textLeft, idY, rc.Right - pad, idY + c.px(16)},
		fLabel, cwColInk, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	idRow := cwRect{textLeft, idY + c.px(17), rc.Right - pad, idY + c.px(45)}
	c.text(hdc, c.deviceID, idRow, fID, cwColAccent, cwDTLeft|cwDTVCenter|cwDTSingleLine)

	// Copy button, parked to the right of the id.
	copySz := c.px(22)
	copyX := textLeft + c.px(148)
	c.rcCopy = cwRect{copyX, idRow.Top + c.px(3), copyX + copySz, idRow.Top + c.px(3) + copySz}
	c.copyGlyph(hdc, c.rcCopy)
	if c.copied {
		c.text(hdc, "Copied",
			cwRect{c.rcCopy.Right + c.px(6), c.rcCopy.Top, rc.Right - pad, c.rcCopy.Bottom},
			fSmall, cwColInkSoft, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	}

	// ---- divider + security note ----
	secY := idRow.Bottom + c.px(14)
	c.line(hdc, pad, secY, rc.Right-pad, secY, cwColBorder)

	noteY := secY + c.px(16)
	c.shieldGlyph(hdc, pad+c.px(4), noteY+c.px(2), c.px(20))
	noteLeft := pad + c.px(36)
	c.text(hdc, "Only allow if you recognise this request.",
		cwRect{noteLeft, noteY, rc.Right - pad, noteY + c.px(18)},
		fLabel, cwColInk, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	c.text(hdc, "If you don't recognise this device, do not allow the connection.",
		cwRect{noteLeft, noteY + c.px(19), rc.Right - pad, noteY + c.px(56)},
		fSmall, cwColInkSoft, cwDTLeft|cwDTWordBreak)

	// ---- footer: remember + actions ----
	footY := rc.Bottom - c.px(64)
	c.line(hdc, pad, footY-c.px(12), rc.Right-pad, footY-c.px(12), cwColBorder)

	boxSz := c.px(16)
	box := cwRect{pad, footY + c.px(10), pad + boxSz, footY + c.px(10) + boxSz}
	fill := cwColCard
	if c.res.remember {
		fill = cwColAccent
	}
	c.roundRect(hdc, box, c.px(4), fill, cwColBorderStr, true)
	if c.res.remember {
		// Check mark.
		c.line(hdc, box.Left+c.px(4), (box.Top+box.Bottom)/2, box.Left+c.px(7), box.Bottom-c.px(4), cwColCard)
		c.line(hdc, box.Left+c.px(7), box.Bottom-c.px(4), box.Right-c.px(3), box.Top+c.px(4), cwColCard)
	}
	lblRect := cwRect{box.Right + c.px(9), box.Top - c.px(2), pad + c.px(190), box.Bottom + c.px(2)}
	c.text(hdc, "Remember this decision", lblRect, fSmall, cwColInkSoft,
		cwDTLeft|cwDTVCenter|cwDTSingleLine)
	// The whole row toggles, not just the 16px box.
	c.rcRemember = cwRect{box.Left, box.Top - c.px(4), lblRect.Right, box.Bottom + c.px(4)}

	btnH := c.px(38)
	btnY := footY + c.px(1)
	accW := c.px(104)
	decW := c.px(96)
	c.rcAccept = cwRect{rc.Right - pad - accW, btnY, rc.Right - pad, btnY + btnH}
	c.rcDecline = cwRect{c.rcAccept.Left - c.px(10) - decW, btnY, c.rcAccept.Left - c.px(10), btnY + btnH}

	accFill := cwColAccent
	if c.hotAccept {
		accFill = cwColAccentDim
	}
	c.roundRect(hdc, c.rcAccept, c.px(9), accFill, 0, false)
	c.text(hdc, "Accept", c.rcAccept, fBtn, cwColCard, cwDTCenter|cwDTVCenter|cwDTSingleLine)

	decFill := cwColCard
	if c.hotDecline {
		decFill = cwColTint
	}
	c.roundRect(hdc, c.rcDecline, c.px(9), decFill, cwColBorderStr, true)
	c.text(hdc, "Decline", c.rcDecline, fBtn, cwColInk, cwDTCenter|cwDTVCenter|cwDTSingleLine)
}

// medallion draws the tinted circle with a monitor-and-person glyph.
func (c *consentWin) medallion(hdc uintptr, x, y, size int32) {
	brush, _, _ := procCreateSolidBrushCW.Call(cwColTint)
	pen, _, _ := procCreatePenCW.Call(5 /*PS_NULL*/, 0, 0)
	ob, _, _ := procSelectObjectCW.Call(hdc, brush)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procEllipseCW.Call(hdc, uintptr(x), uintptr(y), uintptr(x+size), uintptr(y+size))
	procSelectObjectCW.Call(hdc, ob)
	procSelectObjectCW.Call(hdc, op)
	procDeleteObjectCW.Call(brush)
	procDeleteObjectCW.Call(pen)

	// Rounded monitor body.
	mw, mh := c.px(40), c.px(30)
	mx, my := x+(size-mw)/2, y+(size-mh)/2-c.px(2)
	c.roundRect(hdc, cwRect{mx, my, mx + mw, my + mh}, c.px(6), cwColAccent, 0, false)
	// Stand.
	sw2, sh2 := c.px(16), c.px(4)
	c.roundRect(hdc, cwRect{mx + (mw-sw2)/2, my + mh, mx + (mw-sw2)/2 + sw2, my + mh + sh2},
		c.px(2), cwColAccent, 0, false)
	// Person: head + shoulders, knocked out of the monitor in the tint colour.
	hr := c.px(5)
	hx, hy := mx+mw/2, my+c.px(11)
	brush2, _, _ := procCreateSolidBrushCW.Call(cwColTint)
	pen2, _, _ := procCreatePenCW.Call(5, 0, 0)
	ob2, _, _ := procSelectObjectCW.Call(hdc, brush2)
	op2, _, _ := procSelectObjectCW.Call(hdc, pen2)
	procEllipseCW.Call(hdc, uintptr(hx-hr), uintptr(hy-hr), uintptr(hx+hr), uintptr(hy+hr))
	procSelectObjectCW.Call(hdc, ob2)
	procSelectObjectCW.Call(hdc, op2)
	procDeleteObjectCW.Call(brush2)
	procDeleteObjectCW.Call(pen2)
	c.roundRect(hdc, cwRect{hx - c.px(9), hy + c.px(4), hx + c.px(9), my + mh - c.px(4)},
		c.px(5), cwColTint, 0, false)
}

// shieldGlyph draws the small security shield next to the warning line.
func (c *consentWin) shieldGlyph(hdc uintptr, x, y, size int32) {
	pts := []cwPoint{
		{x + size/2, y},
		{x + size, y + size/4},
		{x + size, y + size/2},
		{x + size/2, y + size},
		{x, y + size/2},
		{x, y + size/4},
	}
	brush, _, _ := procCreateSolidBrushCW.Call(cwColTint)
	pen, _, _ := procCreatePenCW.Call(0, uintptr(c.px(1)), cwColAccent)
	ob, _, _ := procSelectObjectCW.Call(hdc, brush)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procPolygonCW.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	procSelectObjectCW.Call(hdc, ob)
	procSelectObjectCW.Call(hdc, op)
	procDeleteObjectCW.Call(brush)
	procDeleteObjectCW.Call(pen)
}

// copyGlyph draws the two-rectangle "copy" affordance beside the device id.
func (c *consentWin) copyGlyph(hdc uintptr, r cwRect) {
	off := c.px(4)
	back := cwRect{r.Left + off, r.Top, r.Right, r.Bottom - off}
	front := cwRect{r.Left, r.Top + off, r.Right - off, r.Bottom}
	pen, _, _ := procCreatePenCW.Call(0, uintptr(c.px(1)), cwColInkSoft)
	nullBrush, _, _ := procCreateSolidBrushCW.Call(cwColCard)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	ob, _, _ := procSelectObjectCW.Call(hdc, nullBrush)
	procRectangleCW.Call(hdc, uintptr(back.Left), uintptr(back.Top), uintptr(back.Right), uintptr(back.Bottom))
	procRectangleCW.Call(hdc, uintptr(front.Left), uintptr(front.Top), uintptr(front.Right), uintptr(front.Bottom))
	procSelectObjectCW.Call(hdc, op)
	procSelectObjectCW.Call(hdc, ob)
	procDeleteObjectCW.Call(pen)
	procDeleteObjectCW.Call(nullBrush)
}
