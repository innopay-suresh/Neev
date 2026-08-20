package session

import "testing"

// Voice off means silent in BOTH directions on that side.
//
// WebRTC does not link the two: a local sender's mute state has no effect on a
// remote track it is receiving, so playback carried on regardless of the toggle
// and the host heard the viewer while its own UI said off. The gate has to be
// explicit, and it has to sit before the output device is opened — otherwise
// muting still holds the speakers open for audio it will never play.

func TestPlaybackIsSilentWhenVoiceIsOff(t *testing.T) {
	hostAudioMu.Lock()
	micOn = false
	hostAudioMu.Unlock()

	// A frame arriving with voice off must be discarded outright. If the gate
	// were missing or placed after the device open, this would open the
	// speakers as a side effect.
	playViewerVoice([]byte{1, 2, 3, 4})

	hostAudioMu.Lock()
	opened := hostAudio != nil
	hostAudioMu.Unlock()
	if opened {
		t.Fatal("incoming audio opened the speakers while voice was off")
	}
}

func TestEmptyPacketIsIgnoredWhateverTheToggle(t *testing.T) {
	for _, on := range []bool{true, false} {
		hostAudioMu.Lock()
		micOn = on
		hostAudioMu.Unlock()
		playViewerVoice(nil)
		playViewerVoice([]byte{})
	}
	hostAudioMu.Lock()
	micOn = false
	hostAudioMu.Unlock()
}
