//go:build cgo

package audio

import (
	"math"
	"testing"
)

// Opus replaces PCMU as the voice codec: 48 kHz instead of 8 kHz telephone
// audio, at half the bitrate. These prove the codec actually carries speech,
// not merely that it returns bytes without erroring.

func speechFrame(freq float64) []int16 {
	pcm := make([]int16, OpusFrameSamples)
	for i := range pcm {
		pcm[i] = int16(12000 * math.Sin(2*math.Pi*freq*float64(i)/OpusRate))
	}
	return pcm
}

func TestOpusRoundTripPreservesATone(t *testing.T) {
	enc, err := NewOpusEncoder()
	if err != nil {
		t.Fatal(err)
	}
	dec, err := NewOpusDecoder()
	if err != nil {
		t.Fatal(err)
	}

	in := speechFrame(440)
	// Opus needs a few frames to converge; feed the same tone repeatedly and
	// judge the last one, which is what a continuous stream looks like.
	var out []int16
	for i := 0; i < 12; i++ {
		pkt, err := enc.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(pkt) == 0 {
			t.Fatal("encoder produced an empty packet for a loud tone")
		}
		out, err = dec.Decode(pkt)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(out) != OpusFrameSamples {
		t.Fatalf("decoded %d samples, want %d", len(out), OpusFrameSamples)
	}
	var peak int16
	for _, v := range out {
		if v > peak {
			peak = v
		}
	}
	// A 440 Hz tone at 12000 must survive recognisably. Silence here would mean
	// the codec is wired up but carrying nothing — the failure that would sound
	// exactly like the muting bugs already hit.
	if peak < 6000 {
		t.Fatalf("a 440 Hz tone decoded to peak %d of ~12000 — voice is being lost", peak)
	}
}

func TestOpusIsCheaperThanPCMU(t *testing.T) {
	// The whole point of the change: better audio for FEWER bytes than the
	// 8 kHz codec it replaces (160 bytes per 20 ms frame).
	enc, err := NewOpusEncoder()
	if err != nil {
		t.Fatal(err)
	}
	in := speechFrame(300)
	total := 0
	const frames = 25
	for i := 0; i < frames; i++ {
		pkt, err := enc.Encode(in)
		if err != nil {
			t.Fatal(err)
		}
		total += len(pkt)
	}
	avg := total / frames
	if avg >= SamplesPerFrame {
		t.Errorf("Opus averaged %d bytes/frame, no better than PCMU's %d",
			avg, SamplesPerFrame)
	}
	t.Logf("Opus %d bytes per 20 ms frame vs PCMU %d", avg, SamplesPerFrame)
}

func TestOpusRejectsAWrongSizedFrame(t *testing.T) {
	// Opus only accepts specific frame sizes. A silent mismatch here would be
	// an encoder that never emits anything.
	enc, err := NewOpusEncoder()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Encode(make([]int16, OpusFrameSamples-1)); err == nil {
		t.Fatal("a wrong-sized frame was accepted")
	}
}

func TestOpusFrameMatchesOneDevicePeriod(t *testing.T) {
	// The device delivers exactly this many samples per callback, so no
	// buffering or resampling sits between capture and the encoder.
	if OpusFrameSamples != DeviceFrames {
		t.Fatalf("Opus frame %d != device period %d", OpusFrameSamples, DeviceFrames)
	}
	if OpusRate != DeviceRate {
		t.Fatalf("Opus rate %d != device rate %d", OpusRate, DeviceRate)
	}
}
