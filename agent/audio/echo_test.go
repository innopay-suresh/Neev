//go:build cgo

package audio

import (
	"testing"
	"time"
)

// Echo suppression on the host.
//
// The viewer's libwebrtc AEC only cancels echo created on the VIEWER's machine.
// When the host plays the viewer's voice through its speakers, its microphone
// and its system-sound loopback both pick that up and send it back — the viewer
// hears itself. Only the host can break that, and only it knows when it is
// playing far-end audio.

func TestNotPlayingMeansNoSuppression(t *testing.T) {
	d := &Device{}
	if d.playingFarEnd() {
		t.Fatal("a device that has never played anything reported far-end audio")
	}
}

func TestSuppressionEngagesWhilePlaying(t *testing.T) {
	d := &Device{playing: true, lastPlay: time.Now()}
	if !d.playingFarEnd() {
		t.Fatal("far-end audio playing right now was not detected")
	}
}

func TestSuppressionReleasesAfterTheHangover(t *testing.T) {
	// It must let go, or the host's microphone stays ducked for the rest of the
	// call after a single incoming word.
	d := &Device{playing: true, lastPlay: time.Now().Add(-echoHangover - time.Millisecond)}
	if d.playingFarEnd() {
		t.Fatal("suppression stayed engaged past the hangover window")
	}
}

func TestHangoverOutlastsSpeakerDecay(t *testing.T) {
	// Speakers and the room do not stop the instant the last sample is written.
	// Too short and the tail leaks back as echo; too long and the host cannot
	// talk.
	if echoHangover < 100*time.Millisecond {
		t.Errorf("hangover %v is shorter than typical speaker/room decay", echoHangover)
	}
	if echoHangover > time.Second {
		t.Errorf("hangover %v would keep the host ducked between words", echoHangover)
	}
}

func TestDuckingLeavesRoomToInterrupt(t *testing.T) {
	// Ducking, not muting: a raised voice must still get through, or the call
	// becomes walkie-talkie. Muting outright would be the easy fix and the
	// wrong one.
	if echoSuppressGain <= 0 {
		t.Fatal("the microphone is muted outright — the host could never interrupt")
	}
	if echoSuppressGain > 0.35 {
		t.Errorf("gain %v barely attenuates — the echo would still be audible",
			echoSuppressGain)
	}
	// A loud interruption should survive at an audible level.
	loud := int16(float64(20000) * echoSuppressGain)
	if loud < 1500 {
		t.Errorf("a shout attenuates to %d — inaudible, so no interrupting", loud)
	}
}
