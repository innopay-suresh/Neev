//go:build darwin

package session

import "testing"

// The viewer's button numbering and the injector's are DIFFERENT orders. Wiring
// them straight through turns every right-click into a middle-click, which is
// the kind of fault that survives a demo and shows up in real use.
func TestWireButtonMapping(t *testing.T) {
	right, middle, left, odd := 1, 2, 0, 7
	cases := []struct {
		name string
		in   *int
		want MouseButtonAlias
	}{
		{"right", &right, 2},
		{"middle", &middle, 1},
		{"left", &left, 0},
		{"absent means left", nil, 0},
		{"unknown falls back to left", &odd, 0},
	}
	for _, c := range cases {
		if got := MouseButtonAlias(wireButton(c.in)); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// MouseButtonAlias keeps the test readable without importing agent/input.
type MouseButtonAlias int

// Spot-check the HID → macOS keycode table. macOS keycodes are POSITIONAL, so
// letters cannot be derived arithmetically the way the Windows VK range allows
// and a transposed entry types the wrong character silently.
func TestHIDToCGKey(t *testing.T) {
	cases := map[int]int{
		0x04: 0,   // A
		0x1D: 6,   // Z
		0x16: 1,   // S
		0x1E: 18,  // 1
		0x27: 29,  // 0
		0x28: 36,  // Return
		0x2A: 51,  // Backspace
		0x2C: 49,  // Space
		0x3A: 122, // F1
		0x45: 111, // F12
		0x4F: 124, // Right arrow
		0xE3: 55,  // Left Command
	}
	for usage, want := range cases {
		if got := hidToCGKey(usage); got != want {
			t.Errorf("hidToCGKey(0x%02X) = %d, want %d", usage, got, want)
		}
	}
	if got := hidToCGKey(0xFF); got != -1 {
		t.Errorf("unmapped usage should return -1, got %d", got)
	}
}

// Every letter and digit must map to a DISTINCT keycode: a duplicate silently
// types the wrong character and would not show up in a spot check.
func TestHIDLetterDigitKeysAreDistinct(t *testing.T) {
	seen := map[int]int{}
	for usage := 0x04; usage <= 0x27; usage++ {
		code := hidToCGKey(usage)
		if code < 0 {
			t.Errorf("usage 0x%02X unmapped", usage)
			continue
		}
		if prev, dup := seen[code]; dup {
			t.Errorf("keycode %d used by both 0x%02X and 0x%02X", code, prev, usage)
		}
		seen[code] = usage
	}
}
