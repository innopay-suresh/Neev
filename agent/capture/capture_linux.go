//go:build linux && cgo
// +build linux,cgo

package capture

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -lX11 -lXext
#include "capture_linux.h"
*/
import "C"
import (
	"fmt"
	"image"
	"unsafe"
)

// LinuxCapture implements Capturer for Linux via X11 XShm.
type LinuxCapture struct {
	display unsafe.Pointer
}

// NewPlatformCapture opens the X11 display connection.
// Requires DISPLAY environment variable to be set (e.g. ":0").
func NewPlatformCapture(displayID uint32) (Capturer, error) {
	dpy := C.open_display_x11()
	if dpy == nil {
		return nil, fmt.Errorf("cannot open X11 display — is $DISPLAY set? (e.g. DISPLAY=:0)")
	}
	return &LinuxCapture{display: unsafe.Pointer(dpy)}, nil
}

func (l *LinuxCapture) Bounds() (width, height int) {
	var w, h C.int
	C.get_bounds_x11((*C.Display)(l.display), &w, &h)
	return int(w), int(h)
}

func (l *LinuxCapture) CaptureFrame() (*image.RGBA, error) {
	dpy := (*C.Display)(l.display)
	result := C.capture_frame_x11(dpy)
	switch result.status {
	case C.STATUS_NO_NEW_FRAME:
		return nil, ErrNoNewFrame
	case C.STATUS_ERROR:
		return nil, fmt.Errorf("X11 capture error")
	}
	defer C.free_linux_frame(result.data)

	width := int(result.width)
	height := int(result.height)
	bpl := int(result.bytes_per_line)

	raw := C.GoBytes(unsafe.Pointer(result.data), C.int(bpl*height))
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))

	// X11 XShm returns BGRX (4 bytes: B, G, R, unused) — convert to RGBA.
	for y := 0; y < height; y++ {
		srcRow := raw[y*bpl : y*bpl+width*4]
		dstOff := rgba.PixOffset(0, y)
		for x := 0; x < width; x++ {
			s := x * 4
			d := dstOff + x*4
			rgba.Pix[d+0] = srcRow[s+2] // R
			rgba.Pix[d+1] = srcRow[s+1] // G
			rgba.Pix[d+2] = srcRow[s+0] // B
			rgba.Pix[d+3] = 255
		}
	}
	return rgba, nil
}

func (l *LinuxCapture) Close() error {
	C.close_display_x11((*C.Display)(l.display))
	return nil
}

// GetCursorInfo returns the current X11 cursor position.
func (l *LinuxCapture) GetCursorInfo() CursorInfo {
	var ci C.XCursorInfo
	C.get_xcursor_info((*C.Display)(l.display), &ci)
	if ci.visible == 0 {
		return CursorInfo{Visible: false}
	}
	return CursorInfo{
		X:          int(ci.x),
		Y:          int(ci.y),
		Visible:    ci.visible != 0,
		Width:      int(ci.width),
		Height:     int(ci.height),
		HotX:       int(ci.hotX),
		HotY:       int(ci.hotY),
		CursorType: 0, // X11 cursor type detection requires XFixes extension
	}
}

// ListDisplays returns a single display entry for Linux.
// Full multi-monitor via Xinerama/XRR can be added later.
func ListDisplays() []DisplayInfo {
	return []DisplayInfo{{ID: 0, Width: 1920, Height: 1080, IsPrimary: true}}
}
