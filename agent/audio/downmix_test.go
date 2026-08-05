//go:build cgo

package audio

import (
	"testing"
	"unsafe"
)

// The audio callbacks used to assume the device gave exactly the channel count
// they asked for. When miniaudio opens a device as stereo instead, reading
// `frames` samples takes one channel's worth of an interleaved buffer and
// mangles everything after it — heard as a continuous static/shutter noise that
// has nothing to do with the microphone and does not stop when it is muted.

func bytesOf(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&samples[0])), len(samples)*2)
}

func TestDownmixMonoPassesThroughUnchanged(t *testing.T) {
	in := []int16{100, -200, 300, -400}
	got := downmix(bytesOf(in), len(in))
	if len(got) != len(in) {
		t.Fatalf("got %d samples, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("sample %d changed: %d -> %d", i, in[i], got[i])
		}
	}
}

func TestDownmixStereoAveragesBothChannels(t *testing.T) {
	// Interleaved L,R for two frames. Taking every sample as if it were mono
	// would return {1000, 2000} — the first frame's two channels — and run the
	// stream at double speed.
	in := []int16{1000, 2000, 3000, 4000}
	got := downmix(bytesOf(in), 2)
	if len(got) != 2 {
		t.Fatalf("stereo downmix produced %d frames, want 2", len(got))
	}
	if got[0] != 1500 || got[1] != 3500 {
		t.Errorf("got %v, want [1500 3500] (per-frame channel average)", got)
	}
}

func TestDownmixHandlesAnEmptyBuffer(t *testing.T) {
	if got := downmix(nil, 0); got != nil {
		t.Errorf("empty buffer produced %v", got)
	}
}

func TestDownmixDoesNotOverrunOnOddCounts(t *testing.T) {
	// A device reporting more frames than the buffer holds must not read past
	// the end — that is where garbage audio comes from.
	in := []int16{10, 20}
	got := downmix(bytesOf(in), 2)
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2", len(got))
	}
}
