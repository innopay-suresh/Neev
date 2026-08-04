package session

import (
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/audio"
)

// Host-side voice. Lives in the WORKER process, which runs in the interactive
// session and so can reach the microphone and speakers; the SYSTEM transport
// cannot.
//
// The device is created on first use and torn down when voice stops, so a host
// that never enables audio never opens an audio device — and never lights the
// OS microphone indicator.

var (
	hostAudioMu sync.Mutex
	hostAudio   *audio.Device
	micOn       bool
)

// ensureDeviceLocked returns the shared device, creating it on first use.
// Caller holds hostAudioMu.
func ensureDeviceLocked() *audio.Device {
	if hostAudio != nil {
		return hostAudio
	}
	d, err := audio.NewDevice()
	if err != nil {
		// A machine with no sound card, or no audio service in this session,
		// must not take the session down with it. Voice degrades; the remote
		// desktop keeps working.
		log.Warn().Err(err).Msg("worker: audio unavailable — voice disabled for this session")
		return nil
	}
	hostAudio = d
	return d
}

// startHostMic opens the microphone and streams mu-law frames to fn.
func startHostMic(fn func([]byte)) {
	hostAudioMu.Lock()
	defer hostAudioMu.Unlock()
	if micOn {
		return
	}
	d := ensureDeviceLocked()
	if d == nil {
		return
	}
	if err := d.StartCapture(fn); err != nil {
		log.Warn().Err(err).Msg("worker: microphone could not be opened")
		return
	}
	micOn = true
}

// stopHostMic closes the microphone and hands it back to the OS.
func stopHostMic() {
	hostAudioMu.Lock()
	defer hostAudioMu.Unlock()
	if !micOn || hostAudio == nil {
		return
	}
	hostAudio.StopCapture()
	micOn = false
}

// playViewerVoice queues one mu-law frame from the viewer for the speakers.
func playViewerVoice(mu []byte) {
	hostAudioMu.Lock()
	d := ensureDeviceLocked()
	hostAudioMu.Unlock()
	if d == nil || len(mu) == 0 {
		return
	}
	d.Play(mu)
}

// closeHostAudio releases every audio device. Called when the worker shuts down
// so a microphone can never outlive the session that opened it.
func closeHostAudio() {
	hostAudioMu.Lock()
	d := hostAudio
	hostAudio = nil
	micOn = false
	hostAudioMu.Unlock()
	if d != nil {
		d.Close()
	}
}
