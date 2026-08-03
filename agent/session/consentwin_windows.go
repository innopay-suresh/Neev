//go:build windows

package session

import (
	"sync"
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
	procPostMessageCW      = modUser32CW.NewProc("PostMessageW")

	procCreateSolidBrushCW     = modGdi32CW.NewProc("CreateSolidBrush")
	procCreatePenCW            = modGdi32CW.NewProc("CreatePen")
	procSelectObjectCW         = modGdi32CW.NewProc("SelectObject")
	procDeleteObjectCW         = modGdi32CW.NewProc("DeleteObject")
	procCreateFontCW           = modGdi32CW.NewProc("CreateFontW")
	procSetTextColorCW         = modGdi32CW.NewProc("SetTextColor")
	procSetBkModeCW            = modGdi32CW.NewProc("SetBkMode")
	procRoundRectCW            = modGdi32CW.NewProc("RoundRect")
	procEllipseCW              = modGdi32CW.NewProc("Ellipse")
	procRectangleCW            = modGdi32CW.NewProc("Rectangle")
	procMoveToExCW             = modGdi32CW.NewProc("MoveToEx")
	procLineToCW               = modGdi32CW.NewProc("LineTo")
	procGetDeviceCapsCW        = modGdi32CW.NewProc("GetDeviceCaps")
	procPolygonCW              = modGdi32CW.NewProc("Polygon")
	procGetStockObjectCW       = modGdi32CW.NewProc("GetStockObject")
	procGetTextExtentPoint32CW = modGdi32CW.NewProc("GetTextExtentPoint32W")
	procCreateCompatDCCW       = modGdi32CW.NewProc("CreateCompatibleDC")
	procCreateCompatBmpCW      = modGdi32CW.NewProc("CreateCompatibleBitmap")
	procBitBltCW               = modGdi32CW.NewProc("BitBlt")
	procDeleteDCCW             = modGdi32CW.NewProc("DeleteDC")

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
	cwWMEraseBkgnd  = 0x0014
	cwSrcCopy       = 0x00CC0020
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
	cwColDanger    = rgb(0xD8, 0x49, 0x3F) // Decline mark (DESIGN.md error hue)
	// Wordmark green, sampled from the logo itself so the name and the icon
	// agree. "Neev" is green, "Remote" is the product accent.
	cwColBrandGreen = rgb(0x2E, 0x54, 0x11)
)

const consentClassName = "NeevConsentWindow"

// The window class is process-wide and can only be registered once, so its
// window procedure must be a single stable callback that routes each message to
// the right prompt. Prompts are modal and serial in practice, but keying by
// hwnd keeps that from being a correctness assumption.
var (
	consentClassOnce sync.Once
	consentWinsMu    sync.Mutex
	consentWins      = map[uintptr]*consentWin{}
	// viewer id -> live prompt window, so a request that goes away can take its
	// dialog with it.
	consentByViewer = map[string]uintptr{}
)

// cancelConsentPrompt withdraws the prompt for a viewer that is no longer
// asking. Posting WM_CLOSE runs the normal refusal path on the window's own
// thread — safe from any goroutine, and it cannot race the user answering.
func cancelConsentPrompt(viewerID string) {
	consentWinsMu.Lock()
	hwnd := consentByViewer[viewerID]
	consentWinsMu.Unlock()
	if hwnd != 0 {
		procPostMessageCW.Call(hwnd, cwWMClose, 0, 0)
	}
}

// consentDispatch is the one registered window procedure. Messages that arrive
// before the hwnd is registered (WM_NCCREATE/WM_CREATE, sent inside
// CreateWindowEx) simply fall through to the default handler.
func consentDispatch(hwnd, msg, wparam, lparam uintptr) uintptr {
	consentWinsMu.Lock()
	c := consentWins[hwnd]
	consentWinsMu.Unlock()
	if c == nil {
		r, _, _ := procDefWindowProcCW.Call(hwnd, msg, wparam, lparam)
		return r
	}
	return c.proc(hwnd, msg, wparam, lparam)
}

// consentResult carries the user's answer out of the window procedure.
type consentResult struct {
	allow    bool
	control  bool // host granted CONTROL rather than view-only
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
	rcFullCtl, rcViewOnly                            cwRect
	hotAccept, hotDecline                            bool
	copied                                           bool
}

func (c *consentWin) px(v int) int32 { return int32(float64(v) * c.scale) }

// showConsentWindow displays the consent card and blocks until answered.
// Returns allow + whether to remember the decision.
func showConsentWindow(viewerID string) (allow bool, control bool, remember bool) {
	c := &consentWin{deviceID: prettyConsentID(viewerID), scale: 1}
	rawViewerID := viewerID
	// Default the access level to the host's own View-only setting, so the
	// prompt opens on what this machine's owner already asked for.
	c.res.control = !hostViewOnlyDefault()

	// DPI: the worker is not per-monitor DPI aware, so use the desktop DC's
	// logical pixel count. On a 150% display this keeps the card from rendering
	// at a third of its intended size.
	if dc, _, _ := procGetDCCW.Call(0); dc != 0 {
		if dpi, _, _ := procGetDeviceCapsCW.Call(dc, cwLogPixelsX); dpi > 0 {
			c.scale = float64(dpi) / 96.0
		}
		procReleaseDCCW.Call(0, dc)
	}

	// Register the class exactly ONCE, with a dispatcher that finds the window's
	// state by hwnd.
	//
	// This used to build a fresh syscall.NewCallback per prompt, closed over THIS
	// consentWin, and re-register the class each time. The second prompt's
	// RegisterClassW fails with ERROR_CLASS_ALREADY_EXISTS, so the class kept the
	// FIRST callback — every later window was driven by the first prompt's state.
	// Accept then set `answered` on a struct nobody was watching, this function's
	// message loop never exited, showConsentDialog never returned, and the
	// transport denied the connection on its 30s timeout. Symptom: the host
	// accepted exactly ONE connection per worker lifetime and refused every one
	// after it, which no amount of restarting the VIEWER could clear.
	// (It also leaked a callback per prompt against a small process-wide cap.)
	consentClassOnce.Do(func() {
		className, _ := syscall.UTF16PtrFromString(consentClassName)
		cursor, _, _ := procLoadCursorCW.Call(0, cwIDCArrow)
		bg, _, _ := procCreateSolidBrushCW.Call(cwColCard)
		wc := cwWndClass{
			WndProc: syscall.NewCallback(consentDispatch),
			Cursor:  cursor, Background: bg, ClassName: className,
		}
		procRegisterClassCW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	className, _ := syscall.UTF16PtrFromString(consentClassName)

	// Landscape card: information rail + artwork/access panel side by side.
	// Sized down from 800x560 — the first cut was larger than a consent prompt
	// needs to be and dominated the screen it interrupts.
	w := c.px(700)
	h := c.px(452)
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
		// The stock fallback cannot offer an access selector, so fall back to the
		// host's configured default rather than silently granting control.
		return showConsentMessageBox(viewerID), !hostViewOnlyDefault(), false
	}
	c.hwnd = hwnd
	// Route this window's messages to THIS prompt, and stop routing them when it
	// closes so a stale entry can never drive a later window.
	consentWinsMu.Lock()
	consentWins[hwnd] = c
	consentByViewer[rawViewerID] = hwnd
	consentWinsMu.Unlock()
	defer func() {
		consentWinsMu.Lock()
		delete(consentWins, hwnd)
		delete(consentByViewer, rawViewerID)
		consentWinsMu.Unlock()
	}()
	// The window is created before this point, so the first paint happens now.
	procInvalidateRectCW.Call(hwnd, 0, 1)
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
	return c.res.allow, c.res.control, c.res.remember
}

func ptInRect(r cwRect, x, y int32) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// answer records the choice. It deliberately does NOT PostQuitMessage: WM_QUIT
// goes on the THREAD's queue, and showConsentDialog locks/unlocks its OS thread
// so the Go runtime can hand that same thread to a later prompt. A leftover
// WM_QUIT would make the next prompt's GetMessage return 0 immediately, exiting
// its loop unanswered and denying the connection. Every answer arrives from a
// dispatched message, so the loop notices `answered` right after
// DispatchMessage returns and exits on its own.
func (c *consentWin) answer(allow bool) {
	c.res.allow = allow
	c.res.answered = true
}

func (c *consentWin) proc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case cwWMPaint:
		c.paint(hwnd)
		return 0
	case cwWMEraseBkgnd:
		// Claim the erase. Letting the system blank the window first and then
		// repainting over it is exactly the flash a user sees; paint() covers
		// every pixel via the back buffer.
		return 1
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
		case ptInRect(c.rcFullCtl, x, y):
			c.res.control = true
			procInvalidateRectCW.Call(hwnd, 0, 0)
		case ptInRect(c.rcViewOnly, x, y):
			c.res.control = false
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
		// Same reason as answer(): posting WM_QUIT here would outlive this prompt
		// on a pooled thread. If the window is going away unanswered, treat it as
		// a refusal so the loop exits.
		c.res.answered = true
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

// textWidth measures a run in the given font, so two-tone text can be laid out
// without guessing where the first half ends.
func (c *consentWin) textWidth(hdc uintptr, s string, f uintptr) int32 {
	old, _, _ := procSelectObjectCW.Call(hdc, f)
	u16, _ := syscall.UTF16FromString(s)
	var sz struct{ CX, CY int32 }
	procGetTextExtentPoint32CW.Call(hdc, uintptr(unsafe.Pointer(&u16[0])),
		uintptr(len(u16)-1), uintptr(unsafe.Pointer(&sz)))
	procSelectObjectCW.Call(hdc, old)
	return sz.CX
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
	screen, _, _ := procBeginPaintCW.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaintCW.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc cwRect
	procGetClientRectCW.Call(hwnd, uintptr(unsafe.Pointer(&rc)))

	// DOUBLE BUFFER. Everything below draws into an off-screen bitmap which is
	// blitted to the screen in one operation. Drawing straight to the window
	// meant each repaint rebuilt the card in place — background, map polygons,
	// text — and the intermediate states were visible as flicker. That happens
	// on every hover of the Accept/Decline buttons, so it was unavoidable while
	// deciding. If the buffer can't be created, fall back to direct drawing:
	// a flickering prompt still beats no prompt.
	hdc := screen
	memDC, _, _ := procCreateCompatDCCW.Call(screen)
	var memBmp, oldBmp uintptr
	if memDC != 0 {
		memBmp, _, _ = procCreateCompatBmpCW.Call(screen,
			uintptr(rc.Right), uintptr(rc.Bottom))
		if memBmp != 0 {
			oldBmp, _, _ = procSelectObjectCW.Call(memDC, memBmp)
			hdc = memDC
		}
	}
	defer func() {
		if hdc == memDC {
			procBitBltCW.Call(screen, 0, 0, uintptr(rc.Right), uintptr(rc.Bottom),
				memDC, 0, 0, cwSrcCopy)
		}
		if oldBmp != 0 {
			procSelectObjectCW.Call(memDC, oldBmp)
		}
		if memBmp != 0 {
			procDeleteObjectCW.Call(memBmp)
		}
		if memDC != 0 {
			procDeleteDCCW.Call(memDC)
		}
	}()

	bg, _, _ := procCreateSolidBrushCW.Call(cwColCard)
	procFillRectCW.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
	procDeleteObjectCW.Call(bg)
	c.roundRect(hdc, rc, c.px(2), cwColCard, cwColBorderStr, true)

	fTitle := c.font(16, true, false)
	fH1 := c.font(20, true, false)
	fBody := c.font(12, false, false)
	fLabel := c.font(11, true, false)
	fID := c.font(24, true, true)
	fBtn := c.font(13, true, false)
	fSmall := c.font(11, false, false)
	defer func() {
		for _, f := range []uintptr{fTitle, fH1, fBody, fLabel, fID, fBtn, fSmall} {
			procDeleteObjectCW.Call(f)
		}
	}()

	pad := c.px(18)
	// Two columns: a tinted information rail on the left, the access panel and
	// artwork on the right — the approved landscape layout.
	railW := c.px(228)
	footerH := c.px(66)

	// ---- header -------------------------------------------------------
	top := c.px(13)
	markSize := c.px(22)
	mark := cwRect{pad, top, pad + markSize, top + markSize}
	c.roundRect(hdc, mark, c.px(6), cwColAccent, 0, false)
	c.text(hdc, "N", mark, c.font(13, true, false), cwColCard,
		cwDTCenter|cwDTVCenter|cwDTSingleLine)
	// Two-tone wordmark. Drawn as two runs because GDI has no rich text: measure
	// "Neev" so "Remote" starts exactly where it ends, with no gap or overlap.
	wmX := pad + markSize + c.px(9)
	wmRect := cwRect{wmX, top, rc.Right - pad, top + markSize}
	c.text(hdc, "Neev", wmRect, fTitle, cwColBrandGreen,
		cwDTLeft|cwDTVCenter|cwDTSingleLine)
	neevW := c.textWidth(hdc, "Neev", fTitle)
	c.text(hdc, "Remote",
		cwRect{wmX + neevW, top, rc.Right - pad, top + markSize},
		fTitle, cwColAccent, cwDTLeft|cwDTVCenter|cwDTSingleLine)

	closeSz := c.px(24)
	c.rcClose = cwRect{rc.Right - pad - closeSz, top, rc.Right - pad, top + closeSz}
	cx, cy, k := (c.rcClose.Left+c.rcClose.Right)/2, (c.rcClose.Top+c.rcClose.Bottom)/2, c.px(5)
	c.line(hdc, cx-k, cy-k, cx+k, cy+k, cwColInkSoft)
	c.line(hdc, cx+k, cy-k, cx-k, cy+k, cwColInkSoft)

	railTop := top + markSize + c.px(11)
	railBottom := rc.Bottom - footerH

	// ---- left rail ----------------------------------------------------
	rail := cwRect{pad, railTop, pad + railW, railBottom}
	c.roundRect(hdc, rail, c.px(12), cwColTint, 0, false)

	rx := rail.Left + c.px(18)
	rw := rail.Right - c.px(18)

	// Avatar medallion.
	avSz := c.px(58)
	c.avatarGlyph(hdc, rx, rail.Top+c.px(14), avSz)

	ty := rail.Top + c.px(14) + avSz + c.px(12)
	c.text(hdc, "Incoming Connection", cwRect{rx, ty, rw, ty + c.px(26)},
		fH1, cwColInk, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	c.text(hdc, "A remote device is requesting to connect to your computer.",
		cwRect{rx, ty + c.px(28), rw, ty + c.px(76)},
		fBody, cwColInkSoft, cwDTLeft|cwDTWordBreak)

	// Hairline between the description and the identity block.
	c.line(hdc, rx, ty+c.px(78), rw, ty+c.px(78), cwColBorderStr)

	// Device id.
	idy := ty + c.px(88)
	c.text(hdc, "Remote Device ID", cwRect{rx, idy, rw, idy + c.px(16)},
		fLabel, cwColInkSoft, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	idRow := cwRect{rx, idy + c.px(18), rw, idy + c.px(50)}
	c.text(hdc, c.deviceID, idRow, fID, cwColAccent, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	copySz := c.px(24)
	c.rcCopy = cwRect{rw - copySz, idRow.Top + c.px(4), rw, idRow.Top + c.px(4) + copySz}
	c.copyGlyph(hdc, c.rcCopy)
	if c.copied {
		c.text(hdc, "Copied", cwRect{rx, idRow.Bottom, rw, idRow.Bottom + c.px(14)},
			fSmall, cwColInkSoft, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	}

	// Trust note, boxed.
	nbTop := idRow.Bottom + c.px(12)
	nb := cwRect{rx - c.px(6), nbTop, rw + c.px(6), nbTop + c.px(92)}
	c.roundRect(hdc, nb, c.px(10), cwColCard, cwColBorder, true)
	c.shieldGlyph(hdc, nb.Left+c.px(12), nb.Top+c.px(12), c.px(18))
	nl := nb.Left + c.px(38)
	c.text(hdc, "Allow only if you trust this device.",
		cwRect{nl, nb.Top + c.px(10), nb.Right - c.px(10), nb.Top + c.px(44)},
		fLabel, cwColInk, cwDTLeft|cwDTWordBreak)
	c.text(hdc, "If you don't recognise this device, deny the request to keep your computer safe.",
		cwRect{nl, nb.Top + c.px(46), nb.Right - c.px(10), nb.Bottom - c.px(8)},
		fSmall, cwColInkSoft, cwDTLeft|cwDTWordBreak)

	// ---- right panel: artwork + access level ---------------------------
	px0 := rail.Right + c.px(16)
	panel := cwRect{px0, railTop, rc.Right - pad, railBottom}
	c.connectionArt(hdc, panel)

	// Access level. This is the HOST's decision and the whole point of the
	// prompt: a viewer cannot escalate itself later.
	title := "Full Control Access"
	sub := "The remote user will be able to see your screen and control your computer."
	if !c.res.control {
		title = "View Only Access"
		sub = "The remote user will be able to see your screen but NOT control it."
	}
	ay := panel.Bottom - c.px(104)
	c.text(hdc, title, cwRect{panel.Left, ay, panel.Right, ay + c.px(26)},
		fH1, cwColInk, cwDTCenter|cwDTVCenter|cwDTSingleLine)
	c.text(hdc, sub, cwRect{panel.Left + c.px(16), ay + c.px(28), panel.Right - c.px(16), ay + c.px(70)},
		fSmall, cwColInkSoft, cwDTCenter|cwDTWordBreak)

	// Segmented selector.
	segW := c.px(132)
	segH := c.px(30)
	segY := panel.Bottom - c.px(36)
	midX := (panel.Left + panel.Right) / 2
	c.rcFullCtl = cwRect{midX - segW - c.px(4), segY, midX - c.px(4), segY + segH}
	c.rcViewOnly = cwRect{midX + c.px(4), segY, midX + c.px(4) + segW, segY + segH}
	c.segment(hdc, c.rcFullCtl, "Full control", c.res.control, fSmall)
	c.segment(hdc, c.rcViewOnly, "View only", !c.res.control, fSmall)

	// ---- footer --------------------------------------------------------
	c.line(hdc, pad, rc.Bottom-footerH+c.px(6), rc.Right-pad, rc.Bottom-footerH+c.px(6), cwColBorder)

	boxSz := c.px(16)
	fy := rc.Bottom - footerH + c.px(26)
	box := cwRect{pad, fy, pad + boxSz, fy + boxSz}
	fill := cwColCard
	if c.res.remember {
		fill = cwColAccent
	}
	c.roundRect(hdc, box, c.px(4), fill, cwColBorderStr, true)
	if c.res.remember {
		c.line(hdc, box.Left+c.px(4), (box.Top+box.Bottom)/2, box.Left+c.px(7), box.Bottom-c.px(4), cwColCard)
		c.line(hdc, box.Left+c.px(7), box.Bottom-c.px(4), box.Right-c.px(3), box.Top+c.px(4), cwColCard)
	}
	lbl := cwRect{box.Right + c.px(9), box.Top - c.px(3), pad + c.px(260), box.Bottom + c.px(2)}
	c.text(hdc, "Remember my decision for this device", lbl, fSmall, cwColInk,
		cwDTLeft|cwDTVCenter|cwDTSingleLine)
	c.text(hdc, "You can change this later in settings.",
		cwRect{box.Right + c.px(9), box.Bottom, pad + c.px(300), box.Bottom + c.px(16)},
		fSmall, cwColInkSoft, cwDTLeft|cwDTVCenter|cwDTSingleLine)
	c.rcRemember = cwRect{box.Left, box.Top - c.px(4), lbl.Right, box.Bottom + c.px(4)}

	btnH := c.px(40)
	btnY := rc.Bottom - footerH + c.px(18)
	accW := c.px(132)
	decW := c.px(124)
	c.rcAccept = cwRect{rc.Right - pad - accW, btnY, rc.Right - pad, btnY + btnH}
	c.rcDecline = cwRect{c.rcAccept.Left - c.px(12) - decW, btnY, c.rcAccept.Left - c.px(12), btnY + btnH}

	accFill := cwColAccent
	if c.hotAccept {
		accFill = cwColAccentDim
	}
	c.roundRect(hdc, c.rcAccept, c.px(9), accFill, 0, false)
	// Shield + label, centred as a pair.
	ic := c.px(15)
	ay2 := (c.rcAccept.Top+c.rcAccept.Bottom)/2 - ic/2
	c.shieldGlyphIn(hdc, c.rcAccept.Left+c.px(34), ay2, ic, cwColCard)
	c.text(hdc, "Allow",
		cwRect{c.rcAccept.Left + c.px(24), c.rcAccept.Top, c.rcAccept.Right, c.rcAccept.Bottom},
		fBtn, cwColCard, cwDTCenter|cwDTVCenter|cwDTSingleLine)

	decFill := cwColCard
	if c.hotDecline {
		decFill = cwColTint
	}
	c.roundRect(hdc, c.rcDecline, c.px(9), decFill, cwColBorderStr, true)
	dc2 := c.px(11)
	dy2 := (c.rcDecline.Top+c.rcDecline.Bottom)/2 - dc2/2
	c.crossGlyph(hdc, c.rcDecline.Left+c.px(30), dy2, dc2, cwColDanger)
	c.text(hdc, "Decline",
		cwRect{c.rcDecline.Left + c.px(26), c.rcDecline.Top, c.rcDecline.Right, c.rcDecline.Bottom},
		fBtn, cwColDanger, cwDTCenter|cwDTVCenter|cwDTSingleLine)
}

// segment draws one option of the access-level selector.
// segment draws one option of the access-level selector. The selected state is
// deliberately strong (filled + accent border + accent ink): a barely-visible
// selection on a security prompt means the user cannot tell what they are
// granting.
func (c *consentWin) segment(hdc uintptr, r cwRect, label string, on bool, f uintptr) {
	if on {
		c.roundRect(hdc, r, c.px(8), cwColAccent, cwColAccent, true)
		c.text(hdc, label, r, f, cwColCard, cwDTCenter|cwDTVCenter|cwDTSingleLine)
		return
	}
	c.roundRect(hdc, r, c.px(8), cwColCard, cwColBorderStr, true)
	c.text(hdc, label, r, f, cwColInkSoft, cwDTCenter|cwDTVCenter|cwDTSingleLine)
}

// avatarGlyph draws the person mark inside a ringed medallion.
func (c *consentWin) avatarGlyph(hdc uintptr, x, y, size int32) {
	// Outer dotted orbit, then the ring.
	c.ellipse(hdc, x, y, size, cwColCard, cwColBorder, true)
	inset := c.px(8)
	c.ellipse(hdc, x+inset, y+inset, size-2*inset, cwColTint, cwColAccent, true)

	cx := x + size/2
	hr := c.px(10)
	hy := y + size/2 - c.px(5)
	c.ellipse(hdc, cx-hr, hy-hr, hr*2, cwColAccent, 0, false)
	// Shoulders: a rounded cap under the head.
	c.roundRect(hdc, cwRect{cx - c.px(17), hy + c.px(7), cx + c.px(17), y + size - inset - c.px(3)},
		c.px(11), cwColAccent, 0, false)
}

// ellipse fills a circle of [size] at x,y, optionally outlined.
func (c *consentWin) ellipse(hdc uintptr, x, y, size int32, fill, border uintptr, outlined bool) {
	brush, _, _ := procCreateSolidBrushCW.Call(fill)
	var pen uintptr
	if outlined {
		pen, _, _ = procCreatePenCW.Call(0, uintptr(c.px(1)), border)
	} else {
		pen, _, _ = procCreatePenCW.Call(5 /*PS_NULL*/, 0, 0)
	}
	ob, _, _ := procSelectObjectCW.Call(hdc, brush)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procEllipseCW.Call(hdc, uintptr(x), uintptr(y), uintptr(x+size), uintptr(y+size))
	procSelectObjectCW.Call(hdc, ob)
	procSelectObjectCW.Call(hdc, op)
	procDeleteObjectCW.Call(brush)
	procDeleteObjectCW.Call(pen)
}

// poly fills a polygon.
func (c *consentWin) poly(hdc uintptr, pts []cwPoint, fill, border uintptr, outlined bool) {
	brush, _, _ := procCreateSolidBrushCW.Call(fill)
	var pen uintptr
	if outlined {
		pen, _, _ = procCreatePenCW.Call(0, uintptr(c.px(1)), border)
	} else {
		pen, _, _ = procCreatePenCW.Call(5, 0, 0)
	}
	ob, _, _ := procSelectObjectCW.Call(hdc, brush)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procPolygonCW.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	procSelectObjectCW.Call(hdc, ob)
	procSelectObjectCW.Call(hdc, op)
	procDeleteObjectCW.Call(brush)
	procDeleteObjectCW.Call(pen)
}

// iso projects floor coordinates (u,v) to screen space. A true isometric
// projection, so the two machines read as objects in a space rather than as
// flat rectangles — the thing the first version got wrong.
func iso(ox, oy int32, u, v, k float64) cwPoint {
	return cwPoint{
		X: ox + int32((u-v)*0.866*k),
		Y: oy + int32((u+v)*0.5*k),
	}
}

// connectionArt draws a world map with the connection arcing across it.
//
// Replaces the isometric laptops: the map is the product's existing visual
// identity (the same coastline data the Flutter home page uses, ported in
// worldmap_windows.go), so the prompt and the app show one world instead of two
// unrelated illustrations. It also says the right thing — a remote device,
// somewhere else, reaching this machine.
func (c *consentWin) connectionArt(hdc uintptr, r cwRect) {
	// Fit an equirectangular world into the rect, preserving the 2:1 aspect so
	// the continents aren't stretched.
	w := float64(r.Right - r.Left)
	h := float64(r.Bottom - r.Top)
	mw := w
	mh := mw / 2
	if mh > h*0.72 {
		mh = h * 0.72
		mw = mh * 2
	}
	ox := float64(r.Left) + (w-mw)/2
	oy := float64(r.Top) + float64(c.px(6))

	project := func(g geoPt) cwPoint {
		return cwPoint{
			X: int32(ox + (g.Lon+180.0)/360.0*mw),
			Y: int32(oy + (90.0-g.Lat)/180.0*mh),
		}
	}

	// Land masses, filled in the tint so the map reads as a soft backdrop rather
	// than competing with the text beneath it.
	for _, ring := range mapLand {
		if len(ring) < 3 {
			continue
		}
		pts := make([]cwPoint, 0, len(ring))
		for _, g := range ring {
			pts = append(pts, project(g))
		}
		c.poly(hdc, pts, cwColTint, cwColTint, true)
	}
	// Internal borders, a shade darker.
	for _, line := range mapBorders {
		for i := 0; i+1 < len(line); i++ {
			a, b := project(line[i]), project(line[i+1])
			c.line(hdc, a.X, a.Y, b.X, b.Y, cwColBorder)
		}
	}

	// The connection: remote device -> this machine, as a great-circle-ish arc.
	from := project(geoPt{Lon: -95, Lat: 40}) // somewhere else
	to := project(geoPt{Lon: 78, Lat: 22})    // this machine
	c.connectionArc(hdc, from, to)

	// Endpoints: origin ring, destination pin.
	c.ellipse(hdc, from.X-c.px(6), from.Y-c.px(6), c.px(12), cwColCard, cwColAccent, true)
	c.ellipse(hdc, from.X-c.px(3), from.Y-c.px(3), c.px(6), cwColAccent, 0, false)
	c.ellipse(hdc, to.X-c.px(10), to.Y-c.px(10), c.px(20), cwColTint, cwColAccent, true)
	c.ellipse(hdc, to.X-c.px(5), to.Y-c.px(5), c.px(10), cwColAccent, 0, false)
}

// arcPoint samples the connection arc at t in [0,1]. A quadratic bend lifted
// perpendicular to the chord, which reads as a hop between two places.
func arcPoint(a, b cwPoint, t float64) cwPoint {
	mx := float64(a.X+b.X) / 2
	my := float64(a.Y+b.Y) / 2
	dx := float64(b.X - a.X)
	dy := float64(b.Y - a.Y)
	// Control point pushed above the midpoint, scaled to the span.
	cx := mx - dy*0.16
	cy := my + dx*0.16 - float64(absInt32(b.X-a.X))*0.18
	u := 1 - t
	return cwPoint{
		X: int32(u*u*float64(a.X) + 2*u*t*cx + t*t*float64(b.X)),
		Y: int32(u*u*float64(a.Y) + 2*u*t*cy + t*t*float64(b.Y)),
	}
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// connectionArc draws the arc and the nodes sitting along it.
//
// Deliberately STATIC. An earlier version animated the nodes on a ~30fps
// WM_TIMER, but each tick invalidated the whole card and GDI repainted it
// without double buffering, so the entire prompt flickered. A consent dialog is
// something you read and answer, not something that should be moving while you
// decide.
func (c *consentWin) connectionArc(hdc uintptr, a, b cwPoint) {
	const steps = 48
	prev := a
	for i := 1; i <= steps; i++ {
		p := arcPoint(a, b, float64(i)/steps)
		c.line(hdc, prev.X, prev.Y, p.X, p.Y, cwColAccent)
		prev = p
	}
	// Nodes marking the route, evenly spaced along the arc.
	for i := 1; i <= 3; i++ {
		t := float64(i) / 4.0
		p := arcPoint(a, b, t)
		rr := c.px(4)
		c.ellipse(hdc, p.X-rr, p.Y-rr, rr*2, cwColAccent, 0, false)
		// Soft halo so the packet reads against the map.
		hr := c.px(7)
		c.ellipseOutline(hdc, p.X-hr, p.Y-hr, hr*2, cwColAccent)
	}
}

// ellipseOutline strokes a circle without filling it.
func (c *consentWin) ellipseOutline(hdc uintptr, x, y, size int32, col uintptr) {
	pen, _, _ := procCreatePenCW.Call(0, uintptr(c.px(1)), col)
	hollow, _, _ := procGetStockObjectCW.Call(5 /*NULL_BRUSH*/)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	ob, _, _ := procSelectObjectCW.Call(hdc, hollow)
	procEllipseCW.Call(hdc, uintptr(x), uintptr(y), uintptr(x+size), uintptr(y+size))
	procSelectObjectCW.Call(hdc, op)
	procSelectObjectCW.Call(hdc, ob)
	procDeleteObjectCW.Call(pen)
}

// shieldGlyph draws a filled shield — the security mark next to the trust note.
func (c *consentWin) shieldGlyph(hdc uintptr, x, y, size int32) {
	w := size
	h := size * 6 / 5
	pts := []cwPoint{
		{X: x + w/2, Y: y},
		{X: x + w, Y: y + h/5},
		{X: x + w, Y: y + h/2},
		{X: x + w/2, Y: y + h},
		{X: x, Y: y + h/2},
		{X: x, Y: y + h/5},
	}
	c.poly(hdc, pts, cwColAccent, cwColAccent, true)
	// Check mark knocked out in the card colour.
	c.line(hdc, x+w*30/100, y+h*48/100, x+w*45/100, y+h*63/100, cwColCard)
	c.line(hdc, x+w*45/100, y+h*63/100, x+w*72/100, y+h*33/100, cwColCard)
}

// shieldGlyphIn draws the shield in an arbitrary colour (white on the filled
// Allow button, where the accent-on-accent version would be invisible).
func (c *consentWin) shieldGlyphIn(hdc uintptr, x, y, size int32, col uintptr) {
	w := size
	h := size * 6 / 5
	pts := []cwPoint{
		{X: x + w/2, Y: y},
		{X: x + w, Y: y + h/5},
		{X: x + w, Y: y + h/2},
		{X: x + w/2, Y: y + h},
		{X: x, Y: y + h/2},
		{X: x, Y: y + h/5},
	}
	c.poly(hdc, pts, col, col, true)
}

// crossGlyph draws the ✕ on the Decline button.
func (c *consentWin) crossGlyph(hdc uintptr, x, y, size int32, col uintptr) {
	pen, _, _ := procCreatePenCW.Call(0, uintptr(c.px(2)), col)
	op, _, _ := procSelectObjectCW.Call(hdc, pen)
	procMoveToExCW.Call(hdc, uintptr(x), uintptr(y), 0)
	procLineToCW.Call(hdc, uintptr(x+size), uintptr(y+size))
	procMoveToExCW.Call(hdc, uintptr(x+size), uintptr(y), 0)
	procLineToCW.Call(hdc, uintptr(x), uintptr(y+size))
	procSelectObjectCW.Call(hdc, op)
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
