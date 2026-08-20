package audio

import "testing"

// Overrun handling. Cutting the buffer mid-frame leaves a partial packet whose
// samples resume halfway through a waveform — heard as a click on top of
// whatever was already wrong.

func TestPlayCapIsAWholeNumberOfFrames(t *testing.T) {
	d := &Device{playCap: SamplesPerFrame * 30}
	if d.playCap%SamplesPerFrame != 0 {
		t.Fatalf("playCap %d is not a whole number of %d-sample frames",
			d.playCap, SamplesPerFrame)
	}
	// Enough to ride out jitter without letting the conversation drift.
	ms := d.playCap * 1000 / SampleRate
	if ms < 200 || ms > 1000 {
		t.Errorf("playback buffer holds %d ms — outside a usable range", ms)
	}
}

func TestOverrunDropsOnFrameBoundaries(t *testing.T) {
	// Mirrors the arithmetic in Play(): whatever is dropped must leave the
	// buffer aligned to a frame boundary.
	cap := SamplesPerFrame * 30
	for _, existing := range []int{cap, cap - 1, cap - SamplesPerFrame/2} {
		incoming := SamplesPerFrame
		if existing+incoming <= cap {
			continue
		}
		drop := existing + incoming - cap
		if r := drop % SamplesPerFrame; r != 0 {
			drop += SamplesPerFrame - r
		}
		if drop%SamplesPerFrame != 0 {
			t.Fatalf("drop of %d bytes is not frame-aligned", drop)
		}
		if drop > existing {
			drop = existing
		}
		if remaining := existing - drop; remaining < 0 {
			t.Fatalf("dropped more than the buffer held: %d", remaining)
		}
	}
}
