package audio

import "testing"

// The codec is the one piece of the voice path with no library behind it, so
// it is worth proving rather than assuming.

func TestMuLawRoundTripStaysClose(t *testing.T) {
	// mu-law is lossy by design — it trades precision for range. What must
	// hold is that the error stays proportional to the signal, so quiet speech
	// is not swamped by quantisation noise.
	for _, s := range []int16{0, 1, -1, 100, -100, 1000, -1000, 8000, -8000, 30000, -30000} {
		got := DecodeMuLaw(EncodeMuLaw(s))
		diff := int(got) - int(s)
		if diff < 0 {
			diff = -diff
		}
		tolerance := int(s)/16 + 200
		if tolerance < 0 {
			tolerance = -tolerance
		}
		if diff > tolerance {
			t.Errorf("sample %d round-tripped to %d (error %d > tolerance %d)",
				s, got, diff, tolerance)
		}
	}
}

func TestMuLawClipsInsteadOfWrapping(t *testing.T) {
	// The failure this guards: without clipping, a loud sample overflows and
	// comes back with the OPPOSITE sign — which is heard as a harsh crackle,
	// not as distortion. Loud input must stay loud and stay the right way up.
	loud := DecodeMuLaw(EncodeMuLaw(32767))
	if loud < 30000 {
		t.Errorf("loud positive sample collapsed to %d", loud)
	}
	quiet := DecodeMuLaw(EncodeMuLaw(-32768))
	if quiet > -30000 {
		t.Errorf("loud negative sample collapsed to %d", quiet)
	}
}

func TestSilenceStaysSilent(t *testing.T) {
	// A codec that emits a DC offset for silence produces a constant hiss on
	// an open mic that nobody is speaking into.
	for _, s := range []int16{0, 1, -1} {
		if got := DecodeMuLaw(EncodeMuLaw(s)); got > 200 || got < -200 {
			t.Errorf("near-silence %d decoded to %d", s, got)
		}
	}
}

func TestFrameSizeMatchesPacketisation(t *testing.T) {
	// 20 ms at 8 kHz is 160 samples, and PCMU is one byte per sample — so an
	// RTP payload is 160 bytes. If this drifts, timestamps desynchronise from
	// real time and audio slowly runs ahead of or behind the speaker.
	if SamplesPerFrame != 160 {
		t.Fatalf("SamplesPerFrame = %d, want 160", SamplesPerFrame)
	}
	if n := len(EncodeFrame(make([]int16, SamplesPerFrame))); n != 160 {
		t.Errorf("encoded frame = %d bytes, want 160", n)
	}
}
