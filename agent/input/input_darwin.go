//go:build darwin
// +build darwin

package input

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
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
	// lastPermCheck is the last time Accessibility was re-read, and lastDropLog
	// the last time a dropped-input warning was written.
	lastPermCheck atomic.Int64
	lastDropLog   atomic.Int64
)

// permRecheckEvery bounds how often AXIsProcessTrustedWithOptions is called on
// the input path. The check is cheap, but input arrives at mouse-move rates.
const permRecheckEvery = 2 * time.Second

// accessibilityGranted re-reads the CURRENT Accessibility state, never
// prompting.
//
// This used to be read ONCE at startup and cached forever. A worker that
// started before the grant applied therefore dropped every mouse and keyboard
// event for the rest of its life, silently — InjectEvent returned nil, so
// nothing upstream ever saw a failure. What the user experienced was video
// arriving normally with clicks doing nothing, and no log line anywhere saying
// why. The original caching existed to avoid prompt spam, which is preserved
// here by passing promptUser=0: this asks, it never prompts.
func accessibilityGranted() bool {
	now := time.Now().UnixNano()
	if atomic.LoadInt32(&hasPermission) == 1 {
		return true // a granted state never revokes itself mid-session
	}
	last := lastPermCheck.Load()
	if now-last < int64(permRecheckEvery) {
		return false
	}
	if !lastPermCheck.CompareAndSwap(last, now) {
		return false // another goroutine is checking
	}
	if C.checkAccessibility(0) == 1 {
		atomic.StoreInt32(&hasPermission, 1)
		log.Info().Msg("input: Accessibility granted — viewer clicks and keys now reach this Mac")
		return true
	}
	return false
}

// darwinInjector implements Injector using macOS Core Graphics CGEvent API.
type darwinInjector struct {
	screenWidth  float64
	screenHeight float64
}

func newPlatformInjector() (Injector, error) {
	initOnce.Do(func() {
		// zerolog, not the stdlib logger: setupFileLog only redirects zerolog,
		// so stdlib output went to a stderr that a LaunchAgent discards. The one
		// message that says whether input can work at all was written to
		// nowhere, which is why "clicks do nothing" could not be diagnosed from
		// the logs.
		if C.checkAccessibility(1) == 1 {
			atomic.StoreInt32(&hasPermission, 1)
			log.Info().Msg("input: Accessibility granted — viewer input will reach this Mac")
		} else {
			atomic.StoreInt32(&hasPermission, 0)
			log.Error().Msg("input: NO ACCESSIBILITY PERMISSION — viewer clicks and keys will " +
				"do nothing. Grant Accessibility to /Library/Application Support/NeevRemote/" +
				"neev-agent in System Settings → Privacy & Security. Re-checked automatically; " +
				"no restart needed once granted")
		}
	})

	w := float64(C.getScreenWidth())
	h := float64(C.getScreenHeight())
	log.Info().Float64("width", w).Float64("height", h).Msg("input: injector configured for screen size")
	return &darwinInjector{screenWidth: w, screenHeight: h}, nil
}

func (d *darwinInjector) InjectEvent(e Event) error {
	if !accessibilityGranted() {
		// Say so, at most once a minute. Silently discarding input is what made
		// this cost a day: every layer above reported success.
		now := time.Now().UnixNano()
		if last := lastDropLog.Load(); now-last > int64(time.Minute) &&
			lastDropLog.CompareAndSwap(last, now) {
			log.Error().Msg("input: DROPPING viewer input — no Accessibility permission for " +
				"/Library/Application Support/NeevRemote/neev-agent")
		}
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
