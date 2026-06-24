//go:build darwin && cgo
// +build darwin,cgo

package capture

/*
#cgo CFLAGS: -x objective-c -I${SRCDIR} -mmacosx-version-min=14.0 -Wno-availability -Wno-deprecated-declarations -Wno-unguarded-availability
#cgo LDFLAGS: -framework Foundation -framework CoreGraphics -framework CoreFoundation -framework CoreMedia -framework IOSurface -mmacosx-version-min=14.0
#include "capture_darwin.h"
*/
import "C"
import (
	"fmt"
	"image"
	"sync"
	"unsafe"
)

// DarwinCapture implements Capturer for macOS using CGDisplayStream + IOSurface.
type DarwinCapture struct {
	state  *C.MacCaptureState
	closed bool
	mu     sync.RWMutex
}

// NewPlatformCapture returns a macOS capturer targeting the given display.
func NewPlatformCapture(displayID uint32) (Capturer, error) {
	if C.request_screen_capture_access_mac() == 0 {
		return nil, fmt.Errorf("screen recording permission not granted; open System Settings → Privacy & Security → Screen Recording and allow RemoteAgent")
	}
	state := C.init_stream_mac(C.uint32_t(displayID))
	if state == nil {
		return nil, fmt.Errorf("failed to initialize macOS capture stream; screen recording permission may still be pending")
	}
	return &DarwinCapture{state: state, closed: false}, nil
}

// RequestScreenCapturePermission prompts macOS to grant screen recording access.
func RequestScreenCapturePermission() error {
	if C.request_screen_capture_access_mac() == 1 {
		return nil
	}
	return fmt.Errorf("screen recording permission not granted")
}

func (d *DarwinCapture) Bounds() (width, height int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var w, h C.int
	C.get_bounds_mac(d.state, &w, &h)
	return int(w), int(h)
}

func (d *DarwinCapture) CaptureFrame() (*image.RGBA, error) {
	d.mu.RLock()
	if d.closed || d.state == nil {
		d.mu.RUnlock()
		return nil, fmt.Errorf("capture stream closed")
	}
	defer d.mu.RUnlock()

	result := C.capture_frame_mac(d.state)
	switch result.status {
	case C.STATUS_NO_NEW_FRAME:
		return nil, ErrNoNewFrame
	case C.STATUS_ERROR:
		return nil, fmt.Errorf("CGDisplayStream capture error — Screen Recording permission may be missing. Grant it in System Settings → Privacy & Security → Screen Recording")
	}
	defer C.free_frame_mac(result.data)

	width := int(result.width)
	height := int(result.height)
	bpr := int(result.bytes_per_row)

	raw := C.GoBytes(unsafe.Pointer(result.data), C.int(bpr*height))
	// Ensure even dimensions for VP8
	if width%2 != 0 {
		width--
	}
	if height%2 != 0 {
		height--
	}

	rgba := image.NewRGBA(image.Rect(0, 0, width, height))

	// CoreGraphics returns BGRA — convert to RGBA.
	for y := 0; y < height; y++ {
		srcRow := raw[y*bpr : y*bpr+width*4]
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

// Close stops the capture stream and releases resources.
func (d *DarwinCapture) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed || d.state == nil {
		return nil // Already closed or invalid state
	}
	d.closed = true
	C.stop_stream_mac(d.state)
	d.state = nil
	return nil
}

// ListDisplays returns a list of active displays on macOS.
func ListDisplays() []DisplayInfo {
	list := C.get_active_displays_mac()
	defer C.free_display_list_mac(list)

	var displays []DisplayInfo
	count := int(list.count)
	if count == 0 {
		return displays
	}

	// Unsafe pointer conversion to access the C array
	cDisplays := (*[1 << 30]C.MacDisplayInfo)(unsafe.Pointer(list.displays))[:count:count]
	for i := 0; i < count; i++ {
		displays = append(displays, DisplayInfo{
			ID:        uint32(cDisplays[i].id),
			Width:     int(cDisplays[i].width),
			Height:    int(cDisplays[i].height),
			IsPrimary: cDisplays[i].isPrimary != 0,
		})
	}
	return displays
}

// GetCursorInfo returns the current system cursor position on macOS.
// Note: On macOS, the cursor is baked into captured frames via
// kCGDisplayStreamShowCursor, so a separate mask is not needed.
// This returns cursor position for overlay alignment purposes.
func (d *DarwinCapture) GetCursorInfo() CursorInfo {
	var ci C.MacCursorInfo
	C.get_cursor_info_mac(&ci)

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
		CursorType: 0, // macOS cursor is baked into frames; type always 0 (arrow)
	}
	return info
}
