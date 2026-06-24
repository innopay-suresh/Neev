//go:build !cgo
// +build !cgo

package capture

import (
	"image"
	"image/color"
)

// StubCapture is a mock capturer used when cross-compilation has CGO disabled.
type StubCapture struct{}

// NewPlatformCapture returns a mock screen capturer.
func NewPlatformCapture(displayID uint32) (Capturer, error) {
	return &StubCapture{}, nil
}

// CaptureFrame returns a solid blue image to emulate screen capture.
func (s *StubCapture) CaptureFrame() (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	// Draw a solid background
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 120, 215, 255}) // Windows Blue
		}
	}
	return img, nil
}

// Bounds returns the default stub display size.
func (s *StubCapture) Bounds() (width, height int) {
	return 1280, 720
}

// GetCursorInfo returns a default cursor at center of screen.
func (s *StubCapture) GetCursorInfo() CursorInfo {
	return CursorInfo{X: 640, Y: 360, Visible: true, Width: 32, Height: 32, HotX: 0, HotY: 0}
}

// Close is a no-op.
func (s *StubCapture) Close() error {
	return nil
}

// ListDisplays returns a mock single monitor.
func ListDisplays() []DisplayInfo {
	return []DisplayInfo{{ID: 0, Width: 1280, Height: 720}}
}
