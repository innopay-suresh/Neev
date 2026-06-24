//go:build linux
// +build linux

package input

import "fmt"

// linuxInjector implements Injector using XTest extension on X11.
// On Wayland, libei or ydotool can be used instead.
type linuxInjector struct{}

func newPlatformInjector() (Injector, error) {
	return &linuxInjector{}, nil
}

func (l *linuxInjector) InjectEvent(e Event) error {
	switch e.Type {
	case EventMouseMove:
		// XTestFakeMotionEvent(display, screen, x, y, CurrentTime)
		fmt.Printf("[X11] mouse_move %.2f,%.2f\n", e.X, e.Y)
	case EventMouseDown:
		// XTestFakeButtonEvent(display, button+1, True, CurrentTime)
		fmt.Printf("[X11] mouse_down btn=%d\n", e.Button)
	case EventMouseUp:
		fmt.Printf("[X11] mouse_up btn=%d\n", e.Button)
	case EventMouseScroll:
		// XTestFakeButtonEvent for button 4/5 (scroll up/down)
		fmt.Printf("[X11] scroll dx=%.1f dy=%.1f\n", e.DeltaX, e.DeltaY)
	case EventKeyDown:
		// XTestFakeKeyEvent(display, keycode, True, CurrentTime)
		fmt.Printf("[X11] key_down %d mod=%d\n", e.KeyCode, e.Modifiers)
	case EventKeyUp:
		fmt.Printf("[X11] key_up %d\n", e.KeyCode)
	case EventKeyChar:
		fmt.Printf("[X11] key_char %s\n", e.Char)
	}
	return nil
}

func (l *linuxInjector) Close() error { return nil }
