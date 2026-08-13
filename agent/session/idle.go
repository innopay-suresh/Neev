package session

import "sync/atomic"

// Capturing only while somebody is watching.
//
// The worker captured and VP8-encoded at 30fps for as long as it ran, whether
// or not a viewer was connected, and the transport dropped every one of those
// frames on the floor because it had no peers. Measured on an idle MacBook with
// nobody connected: the capture worker sat at 90% CPU indefinitely, which is a
// hot, flat-battery laptop for a session that never happened. This is a
// background service that starts at login, so "idle" is its normal state and
// the cost was being paid all day.
//
// Both platforms are affected — the loop has no OS-specific part — but it shows
// up worst on laptops, which is why it surfaced on macOS first.
var (
	// viewersConnected is the viewer count the transport announces via
	// KindSessionState.
	viewersConnected atomic.Int32
	// sessionStateKnown records whether that announcement has EVER arrived.
	//
	// Without it, an older transport that never sends KindSessionState would
	// leave the count at zero forever and the worker would idle through a live
	// session — trading a battery bug for a black screen. Capture therefore
	// runs unless we positively know there is no viewer.
	sessionStateKnown atomic.Bool
)

// setViewerCount records the transport's latest session state.
func setViewerCount(n int) {
	viewersConnected.Store(int32(n))
	sessionStateKnown.Store(true)
}

// shouldCapture reports whether the capture loop should be doing work.
func shouldCapture() bool {
	if !sessionStateKnown.Load() {
		return true // unknown: capture, never risk a blank session
	}
	return viewersConnected.Load() > 0
}
