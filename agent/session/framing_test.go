package session

import (
	"testing"

	"github.com/neev/remote-agent/agent/audio"
)

// Opus encodes an EXACT frame size and nothing else. Capture callbacks deliver
// whatever the device felt like giving — miniaudio's period size is a hint, not
// a promise, and ScreenCaptureKit varies freely — so encoding each callback
// directly failed on any buffer that was not exactly one frame. It failed
// QUIETLY: one warning, then nothing, which is how macOS system sound could be
// silent while every log line looked healthy.

func drainFrames(t *testing.T, chunks []int) int {
	t.Helper()
	hostAudioMu.Lock()
	encPending = nil
	hostAudioMu.Unlock()

	got := 0
	for _, n := range chunks {
		encodeAndSend(make([]int16, n), func([]byte) { got++ })
	}
	return got
}

func TestOddSizedCapturesStillProduceFrames(t *testing.T) {
	// 1024 is a very common device period and is NOT a multiple of 960.
	// Four of them is 4096 samples = 4 whole frames with 256 left over.
	if got := drainFrames(t, []int{1024, 1024, 1024, 1024}); got != 4 {
		t.Fatalf("1024-sample captures produced %d frames, want 4", got)
	}
}

func TestSmallCapturesAccumulateInsteadOfBeingDropped(t *testing.T) {
	// Buffers smaller than a frame must be held, not discarded — otherwise a
	// device with a short period transmits nothing at all.
	half := audio.OpusFrameSamples / 2
	if got := drainFrames(t, []int{half}); got != 0 {
		t.Fatalf("half a frame emitted %d packets, want 0 (it should be held)", got)
	}
	if got := drainFrames(t, []int{half, half}); got != 1 {
		t.Fatalf("two halves produced %d frames, want 1", got)
	}
}

func TestExactFramesPassStraightThrough(t *testing.T) {
	if got := drainFrames(t, []int{audio.OpusFrameSamples}); got != 1 {
		t.Fatalf("an exact frame produced %d packets, want 1", got)
	}
}

func TestBacklogIsBoundedSoAudioStaysCurrent(t *testing.T) {
	// A stalled sink must not let the buffer grow forever; dropping the oldest
	// keeps conversation current rather than increasingly late.
	hostAudioMu.Lock()
	encPending = nil
	hostAudioMu.Unlock()
	for i := 0; i < 200; i++ {
		encodeAndSend(make([]int16, 500), func([]byte) {})
	}
	hostAudioMu.Lock()
	left := len(encPending)
	hostAudioMu.Unlock()
	if left >= audio.OpusFrameSamples {
		t.Fatalf("%d samples left pending — a whole frame or more was not drained", left)
	}
}
