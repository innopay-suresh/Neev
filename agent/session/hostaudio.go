package session

import (
	"sync"
	"sync/atomic"

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
	soundOn     bool

	// sink is where outgoing audio goes, set when either source starts. Held so
	// the second source to start can reuse it without the caller passing it
	// twice.
	sink func([]byte)

	// pendingSound holds the most recent system-sound frame waiting to be mixed
	// with the microphone. Exactly one frame, deliberately: mic and loopback run
	// on independent device clocks, and queueing would let system sound drift
	// steadily behind the voice describing it. Dropping to the newest keeps them
	// aligned.
	pendingSound []byte
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
//
// When system sound is also on, the microphone drives the clock: each mic frame
// is mixed with the latest system-sound frame and sent as one. Both sources
// share a single audio track, so mixing here is what lets the host be heard
// over the sound of their own machine.
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
	sink = fn
	if err := d.StartCapture(func(frame []byte) {
		hostAudioMu.Lock()
		other := pendingSound
		pendingSound = nil
		out := sink
		hostAudioMu.Unlock()
		if out == nil {
			return
		}
		if other != nil {
			frame = audio.Mix(frame, other)
		}
		out(frame)
	}); err != nil {
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
	pendingSound = nil
	if !soundOn {
		sink = nil
	}
}

// startHostSound shares what the host machine is PLAYING with the viewer, so a
// technician can hear an error chime or the audio of whatever is on screen.
//
// Windows only — see audio.LoopbackSupported. Returns false when unavailable so
// the caller can say why instead of leaving a control that does nothing.
func startHostSound(fn func([]byte)) bool {
	hostAudioMu.Lock()
	defer hostAudioMu.Unlock()
	if soundOn {
		return true
	}
	if !audio.LoopbackSupported() {
		log.Info().Msg("worker: system sound sharing unavailable on this OS")
		return false
	}
	d := ensureDeviceLocked()
	if d == nil {
		return false
	}
	if sink == nil {
		sink = fn
	}
	if err := d.StartLoopback(func(frame []byte) {
		hostAudioMu.Lock()
		// With the microphone on, the mic callback does the sending so the two
		// arrive as one mixed frame. Park this one for it to collect.
		if micOn {
			pendingSound = frame
			hostAudioMu.Unlock()
			return
		}
		out := sink
		hostAudioMu.Unlock()
		if out != nil {
			out(frame)
		}
	}); err != nil {
		log.Warn().Err(err).Msg("worker: system sound could not be captured")
		return false
	}
	soundOn = true
	return true
}

// stopHostSound stops sharing the host's system sound.
func stopHostSound() {
	hostAudioMu.Lock()
	d := hostAudio
	soundOn = false
	pendingSound = nil
	hostAudioMu.Unlock()
	if d != nil {
		d.StopLoopback()
	}
}

// setAudioSink registers where outgoing host audio should go for this session.
//
// Needed on macOS, where system sound is captured by the helper app rather than
// by this process: the frames arrive over the control socket with no closure to
// carry the destination, so the destination is registered once per session.
func setAudioSink(fn func([]byte)) {
	hostAudioMu.Lock()
	sink = fn
	hostAudioMu.Unlock()
}

// feedHostSound accepts one mu-law frame of the host's system sound captured
// outside this process (macOS helper) and puts it on the same path as Windows
// loopback — including mixing with the microphone.
func feedHostSound(frame []byte) {
	if len(frame) == 0 {
		return
	}
	hostAudioMu.Lock()
	// Mic on: let the microphone callback collect and mix this, so host voice
	// and host sound leave as one frame rather than fighting for the track.
	if micOn {
		pendingSound = frame
		hostAudioMu.Unlock()
		return
	}
	out := sink
	hostAudioMu.Unlock()
	if out != nil {
		out(frame)
	}
}

// playViewerVoice queues one mu-law frame from the viewer for the speakers.
func playViewerVoice(mu []byte) {
	if len(mu) == 0 {
		return
	}
	hostAudioMu.Lock()
	d := ensureDeviceLocked()
	hostAudioMu.Unlock()
	if d == nil {
		return
	}
	// One line the first time audio actually reaches the speakers. Without it,
	// "the host cannot hear the viewer" could be the channel, the transport, or
	// the audio device, and there was nothing to tell them apart.
	if !playedViewerVoice.Swap(true) {
		log.Info().Msg("worker: playing viewer voice on the host speakers")
	}
	d.Play(mu)
}

// playedViewerVoice makes the log above fire once per worker, not per frame.
var playedViewerVoice atomic.Bool

// closeHostAudio releases every audio device. Called when the worker shuts down
// so a microphone can never outlive the session that opened it.
func closeHostAudio() {
	hostAudioMu.Lock()
	d := hostAudio
	hostAudio = nil
	micOn = false
	soundOn = false
	sink = nil
	pendingSound = nil
	hostAudioMu.Unlock()
	if d != nil {
		d.Close()
	}
}
