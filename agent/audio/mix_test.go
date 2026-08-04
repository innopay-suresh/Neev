package audio

import "testing"

// Mixing is where host voice and host system sound become one track. Getting it
// wrong is audible immediately, so the failure modes are pinned here.

func TestMixSumsInPCMNotMuLaw(t *testing.T) {
	// mu-law is logarithmic: adding the BYTES produces noise, not a mixture.
	// Two copies of the same tone must come back roughly twice as loud.
	one := EncodeFrame([]int16{4000, 4000, 4000})
	mixed := DecodeFrame(Mix(one, one))
	for _, s := range mixed {
		if s < 7000 || s > 9000 {
			t.Fatalf("mixing a signal with itself gave %d, want ~8000", s)
		}
	}
}

func TestMixClipsInsteadOfWrapping(t *testing.T) {
	// Loud voice over loud system sound must distort, not invert. A wrap flips
	// the sign and is heard as a crack, which is far worse than clipping.
	loud := EncodeFrame([]int16{30000, 30000})
	for _, s := range DecodeFrame(Mix(loud, loud)) {
		if s < 25000 {
			t.Fatalf("loud mix collapsed or inverted: %d", s)
		}
	}
}

func TestMixWithSilenceIsUnchanged(t *testing.T) {
	// System sound off must not colour the voice going out.
	voice := EncodeFrame([]int16{1000, -2000, 3000})
	silence := EncodeFrame([]int16{0, 0, 0})
	got := DecodeFrame(Mix(voice, silence))
	want := DecodeFrame(voice)
	for i := range want {
		if diff := int(got[i]) - int(want[i]); diff > 200 || diff < -200 {
			t.Fatalf("silence altered the voice at %d: %d vs %d", i, got[i], want[i])
		}
	}
}

func TestMixHandlesUnevenLengths(t *testing.T) {
	// The two devices have independent clocks, so a short frame must not panic
	// or truncate the longer source.
	if n := len(Mix(make([]byte, 160), make([]byte, 80))); n != 160 {
		t.Fatalf("mixed length %d, want 160", n)
	}
	if n := len(Mix(make([]byte, 80), make([]byte, 160))); n != 160 {
		t.Fatalf("mixed length %d, want 160", n)
	}
}
