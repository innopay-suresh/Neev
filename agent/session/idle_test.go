package session

import "testing"

// The worker encoded 30fps with nobody connected — 90% of a CPU on an idle
// laptop, for frames the transport discarded. These pin the gate, including the
// case that must NOT idle.
func TestShouldCapture(t *testing.T) {
	t.Cleanup(func() { sessionStateKnown.Store(false); viewersConnected.Store(0) })

	// Before the transport has said anything, capture: an older transport that
	// never sends session state must not be idled into a blank session.
	sessionStateKnown.Store(false)
	viewersConnected.Store(0)
	if !shouldCapture() {
		t.Error("must capture while the viewer count is unknown")
	}

	setViewerCount(0)
	if shouldCapture() {
		t.Error("must idle when no viewer is connected")
	}

	setViewerCount(1)
	if !shouldCapture() {
		t.Error("must capture with a viewer connected")
	}

	setViewerCount(0)
	if shouldCapture() {
		t.Error("must idle again after the last viewer leaves")
	}
}
