package input

import (
	"encoding/json"
)

// EventType identifies the kind of input event.
type EventType string

const (
	EventMouseMove     EventType = "mouse_move"
	EventMouseDown     EventType = "mouse_down"
	EventMouseUp       EventType = "mouse_up"
	EventMouseScroll   EventType = "mouse_scroll"
	EventKeyDown       EventType = "key_down"
	EventKeyUp         EventType = "key_up"
	EventKeyChar       EventType = "key_char"
	EventSwitchDisplay EventType = "switch_display"
	EventGetClipboard  EventType = "get_clipboard"
)

// MouseButton identifies which mouse button was pressed.
type MouseButton int

const (
	ButtonLeft   MouseButton = 0
	ButtonMiddle MouseButton = 1
	ButtonRight  MouseButton = 2
)

// Event is a serializable input event sent from controller → agent.
type Event struct {
	Type EventType `json:"type"`

	// Mouse fields
	X      float64     `json:"x,omitempty"` // normalized 0.0–1.0
	Y      float64     `json:"y,omitempty"` // normalized 0.0–1.0
	Button MouseButton `json:"button,omitempty"`
	DeltaX float64     `json:"dx,omitempty"` // scroll delta
	DeltaY float64     `json:"dy,omitempty"`

	// Keyboard fields
	KeyCode   int    `json:"key_code,omitempty"`  // OS virtual key code
	Code      string `json:"code,omitempty"`      // JS physical key code (e.g. KeyA)
	Char      string `json:"char,omitempty"`      // unicode character
	Modifiers int    `json:"modifiers,omitempty"` // bitmask: shift|ctrl|alt|meta

	// Display fields
	DisplayID uint32 `json:"display_id,omitempty"`
}

// Injector injects input events into the OS.
type Injector interface {
	InjectEvent(e Event) error
	Close() error
}

// Modifier bit flags.
const (
	ModShift = 1 << 0
	ModCtrl  = 1 << 1
	ModAlt   = 1 << 2
	ModMeta  = 1 << 3
)

// NewInjector returns the platform-specific injector.
// The actual implementation is in platform-specific files:
//   - input_windows.go (build tag: windows)
//   - input_darwin.go  (build tag: darwin)
//   - input_linux.go   (build tag: linux)
func NewInjector() (Injector, error) {
	return newPlatformInjector()
}

// Decode decodes an Event from JSON bytes (received over WebRTC DataChannel).
func Decode(data []byte) (Event, error) {
	var e Event
	err := json.Unmarshal(data, &e)
	return e, err
}

// Encode serializes an Event to JSON bytes.
func Encode(e Event) ([]byte, error) {
	return json.Marshal(e)
}

// ScreenSize is the remote screen dimensions, used to denormalize coordinates.
type ScreenSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Denormalize converts normalized (0–1) coordinates to absolute screen pixels.
func (s ScreenSize) Denormalize(nx, ny float64) (int, int) {
	return int(nx * float64(s.Width)), int(ny * float64(s.Height))
}
