package capture

import (
	"errors"
	"image"
	"sync"
	"time"
)

// ErrNoNewFrame is returned when the display hasn't changed since the last capture.
var ErrNoNewFrame = errors.New("no new frame available")

// ErrAccessDenied is returned when desktop access is blocked (e.g. locked screen, service mode).
var ErrAccessDenied = errors.New("desktop access denied (locked or service mode)")

// DisplayInfo contains metadata about a physical monitor.
type DisplayInfo struct {
	ID       uint32 `json:"id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	IsPrimary bool  `json:"isPrimary"`
}

// CursorInfo holds the system cursor state for overlay rendering on the viewer.
type CursorInfo struct {
	X          int    `json:"x"`          // cursor X position in screen pixels
	Y          int    `json:"y"`          // cursor Y position in screen pixels
	Visible    bool   `json:"visible"`    // whether cursor is shown
	Width      int    `json:"width"`      // cursor bitmap width (0 if baked into frames)
	Height     int    `json:"height"`     // cursor bitmap height
	HotX       int    `json:"hotX"`       // hot-spot X offset within bitmap
	HotY       int    `json:"hotY"`       // hot-spot Y offset within bitmap
	CursorType int    `json:"cursorType"` // 0=arrow, 1=ibeam, 2=cross, 3=wait, 4=resize, 5=hand
	Mask       []byte `json:"-"`          // 32-bit BGRA cursor bitmap — nil on macOS (baked in frames)
}

// Capturer is the platform-agnostic interface for screen capture.
type Capturer interface {
	// CaptureFrame returns the current screen contents as an RGBA image.
	// Returns ErrNoNewFrame if nothing changed since last call.
	CaptureFrame() (*image.RGBA, error)
	// Bounds returns the current screen resolution.
	Bounds() (width, height int)
	// GetCursorInfo returns the current system cursor position and bitmap
	// for overlay rendering on the viewer side.
	GetCursorInfo() CursorInfo
	// Close releases OS resources.
	Close() error
}

// FrameCallback is called with each captured frame.
type FrameCallback func(frame *image.RGBA, capturedAt time.Time)

// Loop runs a capture loop at the target FPS, calling cb for each new frame.
// It blocks until ctx is cancelled or an unrecoverable error occurs.
func Loop(capturer Capturer, targetFPS int, cb FrameCallback, stop <-chan struct{}) error {
	if targetFPS <= 0 {
		targetFPS = 30
	}
	interval := time.Second / time.Duration(targetFPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track last frame for delta comparison.
	var lastHash uint64
	var mu sync.Mutex

	for {
		select {
		case <-stop:
			return nil
		case t := <-ticker.C:
			frame, err := capturer.CaptureFrame()
			if err != nil {
				if errors.Is(err, ErrNoNewFrame) {
					continue
				}
				return err
			}

			// Simple hash to skip identical frames (saves encoder CPU).
			h := quickHash(frame)
			mu.Lock()
			changed := h != lastHash
			lastHash = h
			mu.Unlock()

			if changed {
				cb(frame, t)
			}
		}
	}
}

// quickHash computes a fast non-cryptographic hash of image pixels
// by sampling every 64th pixel — sufficient for change detection.
func quickHash(img *image.RGBA) uint64 {
	var h uint64 = 14695981039346656037
	pix := img.Pix
	step := 64 * 4 // sample every 64th pixel (4 bytes per pixel)
	for i := 0; i < len(pix); i += step {
		h ^= uint64(pix[i])
		h *= 1099511628211
	}
	return h
}
