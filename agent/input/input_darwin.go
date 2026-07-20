//go:build darwin
// +build darwin

package input

import (
	"log"
	"sync"
	"sync/atomic"
)

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <CoreFoundation/CoreFoundation.h>

static int checkAccessibility(int promptUser) {
    const void *keys[] = { kAXTrustedCheckOptionPrompt };
    const void *values[] = { promptUser ? kCFBooleanTrue : kCFBooleanFalse };
    CFDictionaryRef options = CFDictionaryCreate(kCFAllocatorDefault, keys, values, 1, &kCFCopyStringDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    Boolean trusted = AXIsProcessTrustedWithOptions(options);
    CFRelease(options);
    return trusted ? 1 : 0;
}

static double getScreenWidth() {
    return CGRectGetWidth(CGDisplayBounds(CGMainDisplayID()));
}

static double getScreenHeight() {
    return CGRectGetHeight(CGDisplayBounds(CGMainDisplayID()));
}

// Stamp every event WE inject so privacy mode's input tap can tell remote input
// (let through) from the local user's physical input (blocked). Must match
// NEEV_INJECTED_TAG in privacy_darwin.go and InputInjector.injectedTag in the app.
#define NEEV_INJECTED_TAG 0x4E56494E4ALL

static void neev_tag_injected(CGEventRef e) {
    CGEventSetIntegerValueField(e, kCGEventSourceUserData, NEEV_INJECTED_TAG);
}

static void injectMouseMove(double x, double y) {
    CGPoint pt = CGPointMake(x, y);
    CGEventRef event = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, pt, kCGMouseButtonLeft);
    if (event) {
        neev_tag_injected(event);
        CGEventPost(kCGHIDEventTap, event);
        CFRelease(event);
    }
}

static void injectMouseButton(int button, int isDown, double x, double y) {
    CGEventType eventType;
    if (button == 0) {
        eventType = isDown ? kCGEventLeftMouseDown : kCGEventLeftMouseUp;
    } else if (button == 2) {
        eventType = isDown ? kCGEventRightMouseDown : kCGEventRightMouseUp;
    } else {
        eventType = isDown ? kCGEventOtherMouseDown : kCGEventOtherMouseUp;
    }

    CGPoint pt = CGPointMake(x, y);
    CGEventRef event = CGEventCreateMouseEvent(NULL, eventType, pt, (CGMouseButton)button);
    if (event) {
        neev_tag_injected(event);
        CGEventPost(kCGHIDEventTap, event);
        CFRelease(event);
    }
}

static void injectScroll(int dx, int dy) {
    CGEventRef event = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, dy, dx);
    if (event) {
        neev_tag_injected(event);
        CGEventPost(kCGHIDEventTap, event);
        CFRelease(event);
    }
}

static void injectKey(int keyCode, int isDown) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, isDown ? true : false);
    if (event) {
        neev_tag_injected(event);
        CGEventPost(kCGHIDEventTap, event);
        CFRelease(event);
    }
}
*/
import "C"

var (
	initOnce      sync.Once
	hasPermission int32
)

// darwinInjector implements Injector using macOS Core Graphics CGEvent API.
type darwinInjector struct {
	screenWidth  float64
	screenHeight float64
}

func newPlatformInjector() (Injector, error) {
	initOnce.Do(func() {
		log.Println("[input] macOS input injector initialized - testing Accessibility permission...")
		ret := C.checkAccessibility(1)
		if ret == 1 {
			atomic.StoreInt32(&hasPermission, 1)
			log.Println("[input] ✅ Accessibility permission available")
		} else {
			atomic.StoreInt32(&hasPermission, 0)
			log.Println("[input] ⚠️  Accessibility permission denied - input injection disabled")
		}
	})

	w := float64(C.getScreenWidth())
	h := float64(C.getScreenHeight())
	log.Printf("[input] Configured for screen size: %.0fx%.0f\n", w, h)
	return &darwinInjector{screenWidth: w, screenHeight: h}, nil
}

func (d *darwinInjector) InjectEvent(e Event) error {
	if atomic.LoadInt32(&hasPermission) == 0 {
		// Do not dynamically check; if the OS doesn't recognize the signature,
		// calling checkAccessibility repeatedly can spam the user with prompts.
		return nil
	}

	switch e.Type {
	case EventMouseMove:
		x, y := d.denormalize(e.X, e.Y)
		C.injectMouseMove(C.double(x), C.double(y))
	case EventMouseDown, EventMouseUp:
		x, y := d.denormalize(e.X, e.Y)
		isDown := e.Type == EventMouseDown
		C.injectMouseButton(C.int(e.Button), C.int(boolToInt(isDown)), C.double(x), C.double(y))
	case EventMouseScroll:
		C.injectScroll(C.int(e.DeltaX), C.int(e.DeltaY))
	case EventKeyDown, EventKeyUp:
		isDown := e.Type == EventKeyDown
		cgCode := mapJSCodeToCG(e.Code, e.KeyCode)
		C.injectKey(C.int(cgCode), C.int(boolToInt(isDown)))
	case EventKeyChar:
		return nil
	}
	return nil
}

func (d *darwinInjector) denormalize(nx, ny float64) (float64, float64) {
	x := nx * d.screenWidth
	y := ny * d.screenHeight
	return x, y
}

func (d *darwinInjector) Close() error { return nil }

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func mapJSCodeToCG(code string, fallback int) int {
	// Standard macOS CGKeyCodes mapped from Javascript KeyboardEvent.code
	m := map[string]int{
		"KeyA": 0, "KeyS": 1, "KeyD": 2, "KeyF": 3, "KeyH": 4, "KeyG": 5, "KeyZ": 6, "KeyX": 7,
		"KeyC": 8, "KeyV": 9, "KeyB": 11, "KeyQ": 12, "KeyW": 13, "KeyE": 14, "KeyR": 15, "KeyY": 16,
		"KeyT": 17, "Digit1": 18, "Digit2": 19, "Digit3": 20, "Digit4": 21, "Digit6": 22, "Digit5": 23,
		"Equal": 24, "Digit9": 25, "Digit7": 26, "Minus": 27, "Digit8": 28, "Digit0": 29, "BracketRight": 30,
		"KeyO": 31, "KeyU": 32, "BracketLeft": 33, "KeyI": 34, "KeyP": 35, "Enter": 36, "KeyL": 37,
		"KeyJ": 38, "Quote": 39, "KeyK": 40, "Semicolon": 41, "Backslash": 42, "Comma": 43, "Slash": 44,
		"KeyN": 45, "KeyM": 46, "Period": 47, "Tab": 48, "Space": 49, "Backquote": 50, "Backspace": 51,
		"Escape": 53, "MetaLeft": 54, "MetaRight": 54, "ShiftLeft": 56, "CapsLock": 57, "AltLeft": 58,
		"ControlLeft": 59, "ShiftRight": 60, "AltRight": 61, "ControlRight": 62, "F17": 64, "NumpadDecimal": 65,
		"NumpadMultiply": 67, "NumpadAdd": 69, "NumpadClear": 71, "NumpadDivide": 75, "NumpadEnter": 76,
		"NumpadSubtract": 78, "F18": 79, "F19": 80, "NumpadEqual": 81, "Numpad0": 82, "Numpad1": 83,
		"Numpad2": 84, "Numpad3": 85, "Numpad4": 86, "Numpad5": 87, "Numpad6": 88, "Numpad7": 89,
		"F20": 90, "Numpad8": 91, "Numpad9": 92, "F5": 96, "F6": 97, "F7": 98, "F3": 99, "F8": 100,
		"F9": 101, "F11": 103, "F13": 105, "F16": 106, "F14": 107, "F10": 109, "F12": 111, "F15": 113,
		"Help": 114, "Home": 115, "PageUp": 116, "Delete": 117, "F4": 118, "End": 119, "F2": 120,
		"PageDown": 121, "F1": 122, "ArrowLeft": 123, "ArrowRight": 124, "ArrowDown": 125, "ArrowUp": 126,
	}
	if val, ok := m[code]; ok {
		return val
	}
	// Fallback to basic mapping if code is empty (though it will likely be wrong for letters)
	return fallback
}
