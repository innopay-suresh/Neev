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
	pendingSound []int16

	// One encoder and one decoder per worker. Opus is stateful — it predicts
	// from previous frames — so a fresh codec per frame would both cost more and
	// sound worse.
	opusEnc *audio.OpusEncoder
	opusDec *audio.OpusDecoder
)

// encodeAndSend compresses one device-rate PCM frame and hands it to the sink.
func encodeAndSend(pcm []int16, out func([]byte)) {
	if out == nil || len(pcm) == 0 {
		return
	}
	hostAudioMu.Lock()
	if opusEnc == nil {
		e, err := audio.NewOpusEncoder()
		if err != nil {
			hostAudioMu.Unlock()
			log.Warn().Err(err).Msg("worker: no Opus encoder — voice unavailable")
			return
		}
		opusEnc = e
	}
	enc := opusEnc
	hostAudioMu.Unlock()

	pkt, err := enc.Encode(pcm)
	if err != nil {
		// A wrong-sized frame is a bug, not a transient: log once rather than
		// on every 20 ms.
		if !encWarned.Swap(true) {
			log.Warn().Err(err).Int("samples", len(pcm)).Msg("worker: Opus encode failed")
		}
		return
	}
	out(pkt)
}

var encWarned atomic.Bool

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
	if err := d.StartCapture(func(frame []int16) {
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
		encodeAndSend(frame, out)
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
	if hostAudio == nil {
		return
	}
	// Stop unconditionally rather than only when micOn is set. If that flag
	// ever disagrees with the device — a failed start, a worker that inherited
	// state, a toggle handled twice — the guard would skip the stop and leave a
	// live microphone behind a UI that says off. Stopping an already-stopped
	// device is harmless; the reverse is not.
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
	if err := d.StartLoopback(func(frame []int16) {
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
		encodeAndSend(frame, out)
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
	if !micOn {
		sink = nil
	}
	hostAudioMu.Unlock()
	// Same reasoning as stopHostMic: stop the device regardless of the flag.
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

// feedHostSound accepts one device-rate PCM frame of the host's system sound
// captured outside this process (macOS helper) and puts it on the same path as
// Windows loopback — including mixing with the microphone.
func feedHostSound(frame []int16) {
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
	encodeAndSend(frame, out)
}

// playViewerVoice decodes one Opus packet from the viewer and queues it.
func playViewerVoice(pkt []byte) {
	if len(pkt) == 0 {
		return
	}
	hostAudioMu.Lock()
	d := ensureDeviceLocked()
	if d != nil && opusDec == nil {
		dec, err := audio.NewOpusDecoder()
		if err != nil {
			hostAudioMu.Unlock()
			log.Warn().Err(err).Msg("worker: no Opus decoder — cannot play viewer voice")
			return
		}
		opusDec = dec
	}
	dec := opusDec
	hostAudioMu.Unlock()
	if d == nil || dec == nil {
		return
	}
	mu, err := dec.Decode(pkt)
	if err != nil || len(mu) == 0 {
		if err != nil && !decWarned.Swap(true) {
			log.Warn().Err(err).Msg("worker: Opus decode failed")
		}
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

var decWarned atomic.Bool

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
	// Opus is stateful; a new session must start from a clean codec.
	opusEnc = nil
	opusDec = nil
	hostAudioMu.Unlock()
	if d != nil {
		d.Close()
	}
}
