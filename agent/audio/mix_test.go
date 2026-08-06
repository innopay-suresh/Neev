package audio

import "testing"

// Mixing is where host voice and host system sound become one stream. It works
// on PCM now — Opus decodes to and encodes from PCM, so there is no longer a
// lossy codec sitting between the two sources being combined.

func TestMixSumsBothSources(t *testing.T) {
	got := Mix([]int16{4000, 4000, 4000}, []int16{4000, 4000, 4000})
	for _, s := range got {
		if s != 8000 {
			t.Fatalf("mixing 4000+4000 gave %d, want 8000", s)
		}
	}
}

func TestMixClampsInsteadOfWrapping(t *testing.T) {
	// Loud voice over loud system sound must distort, not invert. An overflow
	// flips the sign and is heard as a crack rather than as loudness.
	for _, s := range Mix([]int16{30000, 30000}, []int16{30000, 30000}) {
		if s != 32767 {
			t.Fatalf("loud mix gave %d, want a clamp at 32767", s)
		}
	}
	for _, s := range Mix([]int16{-30000}, []int16{-30000}) {
		if s != -32768 {
			t.Fatalf("loud negative mix gave %d, want a clamp at -32768", s)
		}
	}
}

func TestMixWithSilenceIsUnchanged(t *testing.T) {
	// System sound off must not colour the voice going out.
	voice := []int16{1000, -2000, 3000}
	got := Mix(voice, []int16{0, 0, 0})
	for i := range voice {
		if got[i] != voice[i] {
			t.Fatalf("silence altered the voice at %d: %d vs %d", i, got[i], voice[i])
		}
	}
}

func TestMixHandlesUnevenLengths(t *testing.T) {
	// The two devices have independent clocks, so a short frame must not panic
	// or truncate the longer source.
	if n := len(Mix(make([]int16, 960), make([]int16, 480))); n != 960 {
		t.Fatalf("mixed length %d, want 960", n)
	}
	if n := len(Mix(make([]int16, 480), make([]int16, 960))); n != 960 {
		t.Fatalf("mixed length %d, want 960", n)
	}
}
