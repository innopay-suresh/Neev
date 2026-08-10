//go:build darwin && cgo

package capture

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

static int neevScreenCaptureGranted(void) {
    return CGPreflightScreenCaptureAccess() ? 1 : 0;
}
*/
import "C"

// ScreenCaptureGranted reports whether this process has Screen Recording.
//
// CGPreflightScreenCaptureAccess answers WITHOUT prompting and without touching
// the capture stack, so it is safe to call on every retry.
//
// The distinction it provides is the important one: macOS decides a process's
// TCC answer ONCE, at first use, and keeps it for that process's lifetime. A
// grant added afterwards does not reach the running process. Observed in the
// field as 312 consecutive capture failures over 26 minutes with the grant
// already in place — capture only started after the worker was restarted by
// hand. So "granted, yet capture fails" is not a permission problem to retry;
// it is a stale process that has to be replaced.
func ScreenCaptureGranted() bool {
	return C.neevScreenCaptureGranted() == 1
}
