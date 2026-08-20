package audio

import (
	"math"
	"testing"
)

// The devices are opened at DeviceRate and converted here, because asking the
// hardware for 8 kHz made miniaudio run the device itself at 8 kHz — a rate
// almost no Windows endpoint supports — and the result was continuous static on
// every device on both machines.

func TestRatesAreConsistent(t *testing.T) {
	if DeviceRate%SampleRate != 0 {
		t.Fatalf("DeviceRate %d is not an integer multiple of %d", DeviceRate, SampleRate)
	}
	if Decim != DeviceRate/SampleRate {
		t.Fatalf("Decim %d does not match the rate ratio", Decim)
	}
	if DeviceFrames != SamplesPerFrame*Decim {
		t.Fatalf("a device period (%d) is not one wire frame (%d) upsampled",
			DeviceFrames, SamplesPerFrame*Decim)
	}
}

func TestDownsampleProducesOneWireFrame(t *testing.T) {
	got := Downsample(make([]int16, DeviceFrames))
	if len(got) != SamplesPerFrame {
		t.Fatalf("a 20 ms device period downsampled to %d samples, want %d",
			len(got), SamplesPerFrame)
	}
}

func TestDownsamplePreservesASteadyTone(t *testing.T) {
	// A constant signal must come back as the same constant. An averaging bug
	// shows up here as a level change, which is audible as distortion.
	in := make([]int16, DeviceFrames)
	for i := range in {
		in[i] = 8000
	}
	for _, v := range Downsample(in) {
		if v < 7900 || v > 8100 {
			t.Fatalf("steady 8000 became %d after downsampling", v)
		}
	}
}

func TestDownsampleAveragesRatherThanPickingEveryNth(t *testing.T) {
	// Alternating +/- is what aliasing feeds on. Averaging cancels it to ~0;
	// picking every 6th sample would return a full-scale value and be heard as
	// a metallic warble.
	in := make([]int16, DeviceFrames)
	for i := range in {
		if i%2 == 0 {
			in[i] = 10000
		} else {
			in[i] = -10000
		}
	}
	for _, v := range Downsample(in) {
		if v > 500 || v < -500 {
			t.Fatalf("alternating signal survived downsampling as %d — "+
				"that is decimation, not averaging", v)
		}
	}
}

func TestUpsampleRestoresTheDeviceRate(t *testing.T) {
	got := Upsample(make([]int16, SamplesPerFrame))
	if len(got) != DeviceFrames {
		t.Fatalf("one wire frame upsampled to %d samples, want %d",
			len(got), DeviceFrames)
	}
}

func TestUpsampleInterpolatesInsteadOfHolding(t *testing.T) {
	// Sample-and-hold makes hard steps between values — broadband noise sitting
	// right on the voice. Between 0 and 6000 the intermediate samples must
	// climb, not jump.
	up := Upsample([]int16{0, 6000})
	rising := 0
	for i := 1; i < Decim; i++ {
		if up[i] > up[i-1] {
			rising++
		}
	}
	if rising < Decim-2 {
		t.Fatalf("upsampling held its value instead of interpolating: %v", up[:Decim])
	}
}

func TestRoundTripKeepsASineRecognisable(t *testing.T) {
	// 300 Hz sits inside the 4 kHz telephone band, so a full
	// downsample→upsample round trip must preserve its shape.
	in := make([]int16, DeviceFrames)
	for i := range in {
		in[i] = int16(10000 * math.Sin(2*math.Pi*300*float64(i)/DeviceRate))
	}
	out := Upsample(Downsample(in))
	if len(out) != len(in) {
		t.Fatalf("round trip changed length: %d -> %d", len(in), len(out))
	}
	var peak int16
	for _, v := range out {
		if v > peak {
			peak = v
		}
	}
	if peak < 7000 {
		t.Fatalf("a 300 Hz tone lost most of its amplitude (peak %d of ~10000)", peak)
	}
}
