package session

import (
	"runtime"
	"testing"
)

// The viewer's vocabulary is not Go's. It compares the announced value against
// "macos"; runtime.GOOS says "darwin". Announcing GOOS directly would leave a
// Mac host unrecognised and Ctrl->Command translation still disabled — the
// exact bug, with a different wrong string.
func TestViewerOSNameMatchesTheViewersVocabulary(t *testing.T) {
	got := viewerOSName()
	switch runtime.GOOS {
	case "darwin":
		if got != "macos" {
			t.Fatalf("darwin must announce as %q (what the viewer checks), got %q", "macos", got)
		}
	case "windows", "linux":
		if got != runtime.GOOS {
			t.Fatalf("got %q, want %q", got, runtime.GOOS)
		}
	}
	if got == "darwin" {
		t.Fatal("announced \"darwin\": the viewer only recognises \"macos\"")
	}
}
