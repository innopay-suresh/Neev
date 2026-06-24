//go:build windows && cgo
// +build windows,cgo

package capture

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR} -ld3d11 -ldxgi -luuid -lgdi32
#include "capture_windows.h"
*/
import "C"
import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

var (
	gdi32            = syscall.NewLazyDLL("gdi32.dll")
	user32           = syscall.NewLazyDLL("user32.dll")
	getDC            = user32.NewProc("GetDC")
	releaseDC        = user32.NewProc("ReleaseDC")
	getDeviceCaps    = gdi32.NewProc("GetDeviceCaps")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
)

// WindowsCapture implements Capturer using DXGI Desktop Duplication.
// The D3D11 device and duplication object are created once on first call
// and reused across frames for maximum performance.
type WindowsCapture struct{}

// NewPlatformCapture returns a Windows DXGI capturer.
func NewPlatformCapture(displayID uint32) (Capturer, error) {
	return &WindowsCapture{}, nil
}

// CaptureFrame captures a desktop frame and cursor info together.
func (w *WindowsCapture) CaptureFrame() (*image.RGBA, error) {
	result := C.capture_frame_win()
	switch result.status {
	case C.STATUS_NO_NEW_FRAME:
		return nil, ErrNoNewFrame
	case C.STATUS_ACCESS_DENIED:
		return nil, ErrAccessDenied
	case C.STATUS_ERROR:
		return nil, fmt.Errorf("dxgi capture error (HRESULT: 0x%08X)", uint32(result.hr))
	}
	defer C.free_frame_win(result.data)

	width := int(result.width)
	height := int(result.height)
	stride := int(result.stride)

	raw := C.GoBytes(unsafe.Pointer(result.data), C.int(stride*height))
	// Ensure even dimensions for VP8
	if width%2 != 0 {
		width--
	}
	if height%2 != 0 {
		height--
	}

	rgba := image.NewRGBA(image.Rect(0, 0, width, height))

	// DXGI returns BGRA with a potentially padded stride — convert to packed RGBA.
	for y := 0; y < height; y++ {
		srcRow := raw[y*stride : y*stride+width*4]
		dstOff := rgba.PixOffset(0, y)
		for x := 0; x < width; x++ {
			s := x * 4
			d := dstOff + x*4
			rgba.Pix[d+0] = srcRow[s+2] // R ← B
			rgba.Pix[d+1] = srcRow[s+1] // G ← G
			rgba.Pix[d+2] = srcRow[s+0] // B ← R
			rgba.Pix[d+3] = 255
		}
	}
	return rgba, nil
}

// GetCursorInfo returns the current system cursor position and bitmap.
func (w *WindowsCapture) GetCursorInfo() CursorInfo {
	var ci C.CursorInfo
	C.get_cursor_info(&ci)

	if ci.visible == 0 {
		return CursorInfo{Visible: false}
	}

	info := CursorInfo{
		X:          int(ci.x),
		Y:          int(ci.y),
		Visible:    ci.visible != 0,
		Width:      int(ci.width),
		Height:     int(ci.height),
		HotX:       int(ci.hotX),
		HotY:       int(ci.hotY),
		CursorType: int(ci.cursorType),
	}

	if ci.mask != nil && ci.width > 0 && ci.height > 0 {
		size := int(ci.width) * int(ci.height) * 4
		info.Mask = C.GoBytes(unsafe.Pointer(ci.mask), C.int(size))
		C.free_cursor_mask(ci.mask)
	}

	return info
}

func (w *WindowsCapture) Bounds() (width, height int) {
	hdc, _, _ := getDC.Call(0)
	if hdc != 0 {
		defer releaseDC.Call(0, hdc)
		wPx, _, _ := getDeviceCaps.Call(hdc, 118) // DESKTOPHORZRES
		hPx, _, _ := getDeviceCaps.Call(hdc, 117) // DESKTOPVERTRES
		if wPx > 0 && hPx > 0 {
			return int(wPx), int(hPx)
		}
	}
	wPx, _, _ := getSystemMetrics.Call(0) // SM_CXSCREEN
	hPx, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN
	return int(wPx), int(hPx)
}

func (c *WindowsCapture) Close() error {
	return nil
}

// ListDisplays returns all active displays on Windows via EnumDisplayMonitors.
func ListDisplays() []DisplayInfo {
	list := C.get_active_displays_win()
	defer C.free_display_list_win(list)
	if list.count == 0 {
		return []DisplayInfo{{ID: 0, Width: 1920, Height: 1080, IsPrimary: true}}
	}
	displays := make([]DisplayInfo, int(list.count))
	cDisplays := (*[1 << 30]C.WinDisplayInfo)(unsafe.Pointer(list.displays))[:list.count:list.count]
	for i := 0; i < int(list.count); i++ {
		displays[i] = DisplayInfo{
			ID:        uint32(i),
			Width:     int(cDisplays[i].width),
			Height:    int(cDisplays[i].height),
			IsPrimary: cDisplays[i].isPrimary != 0,
		}
	}
	return displays
}