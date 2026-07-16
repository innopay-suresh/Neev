//go:build darwin

package session

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>

static int neev_is_on_console() {
    CFDictionaryRef d = CGSessionCopyCurrentDictionary();
    if (d == NULL) return 0; // no window session (e.g. session 0) = not on console
    int r = 0;
    CFBooleanRef b = (CFBooleanRef)CFDictionaryGetValue(d, kCGSessionOnConsoleKey);
    if (b != NULL && CFBooleanGetValue(b)) r = 1;
    CFRelease(d);
    return r;
}
*/
import "C"

// isOnConsole reports whether THIS process's login session currently owns the
// physical display. On fast-user-switch macOS keeps every user's session alive,
// so multiple per-session capture workers exist at once; only the on-console one
// may stream/inject, else the viewer would see a backgrounded user's screen after
// a switch (the D-4 divergence). Uses CGSessionCopyCurrentDictionary +
// kCGSessionOnConsoleKey.
func isOnConsole() bool { return C.neev_is_on_console() == 1 }
