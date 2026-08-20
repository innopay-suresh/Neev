package audio

import "testing"

// Noise gate. A capture device produces a hiss of its own with nobody
// speaking; encoding and sending it makes that hiss the far end's constant
// background. RustDesk gates the same way, for the same reasons.

func TestSilenceIsGated(t *testing.T) {
	quiet := make([]int16, SamplesPerFrame) // digital silence
	if !IsSilent(quiet) {
		t.Fatal("digital silence was not treated as silence")
	}
}

func TestLowLevelHissIsGated(t *testing.T) {
	// The electrical noise floor of a real capture device: small, constant,
	// and exactly what should never be transmitted.
	hiss := make([]int16, SamplesPerFrame)
	for i := range hiss {
		if i%2 == 0 {
			hiss[i] = 40
		} else {
			hiss[i] = -35
		}
	}
	if !IsSilent(hiss) {
		t.Fatal("device hiss was treated as speech and would be transmitted")
	}
}

func TestQuietSpeechIsNOTGated(t *testing.T) {
	// The gate must not swallow someone talking softly or from across a room.
	// Cutting real speech is a far worse failure than passing a little hiss.
	speech := make([]int16, SamplesPerFrame)
	for i := range speech {
		speech[i] = 600 // well above the floor, still quiet
	}
	if IsSilent(speech) {
		t.Fatal("quiet speech was gated as silence — the gate is too aggressive")
	}
}

func TestASingleLoudSampleOpensTheGate(t *testing.T) {
	// Speech onset: the frame where a word starts is mostly silence with a
	// transient. Gating it would clip the beginning of every sentence.
	onset := make([]int16, SamplesPerFrame)
	onset[SamplesPerFrame-1] = 5000
	if IsSilent(onset) {
		t.Fatal("a speech onset frame was gated — word beginnings would be lost")
	}
}

func TestGateHangIsLongEnoughForWordEndings(t *testing.T) {
	// Closing the instant a frame goes quiet clips word tails and breaths.
	ms := gateHangFrames * FrameMillis
	if ms < 300 {
		t.Errorf("gate closes after %d ms — too fast, speech tails get clipped", ms)
	}
	if ms > 2000 {
		t.Errorf("gate stays open %d ms — long enough to defeat the point", ms)
	}
}
