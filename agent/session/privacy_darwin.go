//go:build darwin

package session

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <pthread.h>

// Must match the tag stamped on injected events in agent/input/input_darwin.go.
#define NEEV_INJECTED_TAG 0x4E56494E4ALL

static CFMachPortRef  gTap = NULL;
static CFRunLoopRef   gTapLoop = NULL;
static pthread_t      gTapThread;
static int            gTapRunning = 0;

// Blank the PHYSICAL display via the gamma/transfer table. This is the key trick:
// the transfer table is applied at scanout, so the local screen goes black while
// the FRAMEBUFFER is untouched — and CGDisplayStream captures the framebuffer, so
// the remote viewer still sees the real desktop. (An overlay window would be IN
// the framebuffer and would black out the viewer too.)
static void neev_gamma_black(void) {
    CGDirectDisplayID ids[16];
    uint32_t n = 0;
    if (CGGetActiveDisplayList(16, ids, &n) != kCGErrorSuccess) return;
    for (uint32_t i = 0; i < n; i++) {
        // min=max=0 over the whole ramp => zero output on every channel.
        CGSetDisplayTransferByFormula(ids[i], 0.0, 0.0, 1.0,
                                              0.0, 0.0, 1.0,
                                              0.0, 0.0, 1.0);
    }
}

static void neev_gamma_restore(void) {
    CGDisplayRestoreColorSyncSettings();
}

// Block the LOCAL user's physical input while privacy is on; let our injected
// (tagged) remote input through so the viewer keeps control.
static CGEventRef neev_tap_cb(CGEventTapProxy proxy, CGEventType type,
                              CGEventRef event, void *refcon) {
    (void)proxy; (void)refcon;
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (gTap) CGEventTapEnable(gTap, true);
        return event;
    }
    if (CGEventGetIntegerValueField(event, kCGEventSourceUserData) == NEEV_INJECTED_TAG) {
        return event; // ours — allow
    }
    return NULL; // local physical input — swallow
}

static void* neev_tap_main(void *arg) {
    (void)arg;
    CGEventMask mask =
        CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp) |
        CGEventMaskBit(kCGEventFlagsChanged) |
        CGEventMaskBit(kCGEventLeftMouseDown) | CGEventMaskBit(kCGEventLeftMouseUp) |
        CGEventMaskBit(kCGEventRightMouseDown) | CGEventMaskBit(kCGEventRightMouseUp) |
        CGEventMaskBit(kCGEventOtherMouseDown) | CGEventMaskBit(kCGEventOtherMouseUp) |
        CGEventMaskBit(kCGEventMouseMoved) |
        CGEventMaskBit(kCGEventLeftMouseDragged) | CGEventMaskBit(kCGEventRightMouseDragged) |
        CGEventMaskBit(kCGEventScrollWheel);
    gTap = CGEventTapCreate(kCGHIDEventTap, kCGHeadInsertEventTap,
                            kCGEventTapOptionDefault, mask, neev_tap_cb, NULL);
    if (!gTap) return NULL;
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, gTap, 0);
    gTapLoop = CFRunLoopGetCurrent();
    CFRunLoopAddSource(gTapLoop, src, kCFRunLoopCommonModes);
    CGEventTapEnable(gTap, true);
    CFRunLoopRun(); // until neev_input_unblock stops it
    if (src) CFRelease(src);
    if (gTap) { CFRelease(gTap); gTap = NULL; }
    gTapLoop = NULL;
    return NULL;
}

static void neev_input_block(void) {
    if (gTapRunning) return;
    gTapRunning = 1;
    pthread_create(&gTapThread, NULL, neev_tap_main, NULL);
}

static void neev_input_unblock(void) {
    if (!gTapRunning) return;
    if (gTap) CGEventTapEnable(gTap, false);
    if (gTapLoop) CFRunLoopStop(gTapLoop);
    gTapRunning = 0;
}

static void neev_privacy_on(void)  { neev_gamma_black(); neev_input_block(); }
static void neev_privacy_off(void) { neev_gamma_restore(); neev_input_unblock(); }
*/
import "C"

import "github.com/rs/zerolog/log"

// setPrivacy blanks the host Mac's PHYSICAL screen and blocks its local input,
// while the remote viewer keeps seeing and controlling the real desktop.
//
// Blanking uses the display TRANSFER (gamma) table, not an overlay window: gamma
// is applied at scanout so the framebuffer — which is what CGDisplayStream
// captures — stays intact. An overlay would sit IN the framebuffer and black out
// the viewer too (and this daemon has no GUI run loop to host one anyway).
// Requires Accessibility for the input tap (already granted for remote control).
func setPrivacy(on bool) {
	if on {
		C.neev_privacy_on()
		log.Info().Msg("worker: privacy ON (display blanked via gamma, local input blocked)")
	} else {
		C.neev_privacy_off()
		log.Info().Msg("worker: privacy OFF (display + local input restored)")
	}
}
