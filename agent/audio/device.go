//go:build cgo

package audio

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/gen2brain/malgo"
	"github.com/rs/zerolog/log"
)

// Device owns the host's microphone and speakers for one session.
//
// Both halves are opened lazily and CLOSED — not merely paused — when voice is
// switched off. A remote-support tool holding the microphone open after the
// user muted is indistinguishable from one that listens to the room, and on
// both Windows and macOS the OS indicator stays lit while the device is held.
// Releasing it is what makes the indicator honest.
type Device struct {
	mu sync.Mutex

	ctx      *malgo.AllocatedContext
	capture  *malgo.Device
	play     *malgo.Device
	loopback *malgo.Device

	// onCaptured receives each mu-law encoded frame from the microphone.
	onCaptured func([]byte)

	// playBuf holds mu-law audio waiting to go to the speakers. Bounded: if the
	// network delivers faster than the sound card drains (or the device stalls),
	// old audio is dropped rather than queued. In a conversation, late audio is
	// worthless — better a short gap than a growing delay that never recovers.
	playBuf  []byte
	playCap  int
	playing  bool
	overruns int
}

// NewDevice prepares an audio context without opening any device.
func NewDevice() (*Device, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio context: %w", err)
	}
	return &Device{
		ctx: ctx,
		// ~400 ms of mu-law at 8 kHz. Enough to ride out normal jitter, small
		// enough that a stall is heard as a glitch and not as the whole
		// conversation sliding out of sync.
		playCap: SampleRate * 2 / 5,
	}, nil
}

// StartCapture opens the microphone and calls fn with each 20 ms mu-law frame.
func (d *Device) StartCapture(fn func([]byte)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.capture != nil {
		return nil // already capturing; toggling on twice is not an error
	}
	d.onCaptured = fn

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = Channels
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInFrames = SamplesPerFrame

	dev, err := malgo.InitDevice(d.ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(_, in []byte, frames uint32) {
			d.mu.Lock()
			cb := d.onCaptured
			d.mu.Unlock()
			if cb == nil || frames == 0 || len(in) < int(frames)*2 {
				return
			}
			pcm := unsafe.Slice((*int16)(unsafe.Pointer(&in[0])), frames)
			cb(EncodeFrame(pcm))
		},
	})
	if err != nil {
		return fmt.Errorf("open microphone: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start microphone: %w", err)
	}
	d.capture = dev
	log.Info().Msg("audio: microphone opened")
	return nil
}

// LoopbackSupported reports whether this machine can capture what it is
// playing.
//
// Loopback is a WASAPI feature, so Windows only. macOS has no system-audio
// capture without installing a virtual audio device (BlackHole and friends),
// and shipping an audio driver is a different product decision — so this
// returns false there rather than pretending and failing at open time.
func LoopbackSupported() bool { return runtime.GOOS == "windows" }

// StartLoopback captures what the host is PLAYING and calls fn with each 20 ms
// mu-law frame, so the viewer hears the host's system sound.
func (d *Device) StartLoopback(fn func([]byte)) error {
	if !LoopbackSupported() {
		return fmt.Errorf("audio: system sound capture needs Windows (WASAPI loopback)")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.loopback != nil {
		return nil
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Loopback)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = Channels
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInFrames = SamplesPerFrame

	dev, err := malgo.InitDevice(d.ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(_, in []byte, frames uint32) {
			if frames == 0 || len(in) < int(frames)*2 {
				return
			}
			pcm := unsafe.Slice((*int16)(unsafe.Pointer(&in[0])), frames)
			fn(EncodeFrame(pcm))
		},
	})
	if err != nil {
		return fmt.Errorf("open system sound: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start system sound: %w", err)
	}
	d.loopback = dev
	log.Info().Msg("audio: system sound capture opened")
	return nil
}

// StopLoopback closes system-sound capture.
func (d *Device) StopLoopback() {
	d.mu.Lock()
	dev := d.loopback
	d.loopback = nil
	d.mu.Unlock()
	if dev == nil {
		return
	}
	_ = dev.Stop()
	dev.Uninit()
	log.Info().Msg("audio: system sound capture closed")
}

// StopCapture closes the microphone and releases it back to the OS.
func (d *Device) StopCapture() {
	d.mu.Lock()
	dev := d.capture
	d.capture = nil
	d.onCaptured = nil
	d.mu.Unlock()
	if dev == nil {
		return
	}
	_ = dev.Stop()
	dev.Uninit()
	log.Info().Msg("audio: microphone closed")
}

// Play queues one mu-law frame for the speakers, opening the output device on
// first use.
func (d *Device) Play(mu []byte) {
	d.mu.Lock()
	if !d.playing {
		d.mu.Unlock()
		if err := d.startPlayback(); err != nil {
			log.Warn().Err(err).Msg("audio: cannot open speakers")
			return
		}
		d.mu.Lock()
	}
	// Drop the OLDEST audio on overrun. Keeping the newest is what preserves
	// conversational timing; keeping the oldest would play an ever-later echo.
	if len(d.playBuf)+len(mu) > d.playCap {
		drop := len(d.playBuf) + len(mu) - d.playCap
		if drop > len(d.playBuf) {
			drop = len(d.playBuf)
		}
		d.playBuf = d.playBuf[drop:]
		d.overruns++
		if d.overruns == 1 || d.overruns%100 == 0 {
			log.Warn().Int("overruns", d.overruns).
				Msg("audio: playback buffer overrun — dropping oldest voice")
		}
	}
	d.playBuf = append(d.playBuf, mu...)
	d.mu.Unlock()
}

func (d *Device) startPlayback() error {
	d.mu.Lock()
	if d.playing {
		d.mu.Unlock()
		return nil
	}
	d.playing = true
	d.mu.Unlock()

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = Channels
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInFrames = SamplesPerFrame

	dev, err := malgo.InitDevice(d.ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(out, _ []byte, frames uint32) {
			want := int(frames)
			pcm := unsafe.Slice((*int16)(unsafe.Pointer(&out[0])), want)

			d.mu.Lock()
			n := want
			if len(d.playBuf) < n {
				n = len(d.playBuf)
			}
			chunk := make([]byte, n)
			copy(chunk, d.playBuf[:n])
			d.playBuf = d.playBuf[n:]
			d.mu.Unlock()

			for i := 0; i < n; i++ {
				pcm[i] = DecodeMuLaw(chunk[i])
			}
			// Underrun: write SILENCE rather than leaving the buffer as it was.
			// Stale samples would be replayed as a buzz, which is far more
			// noticeable than a gap.
			for i := n; i < want; i++ {
				pcm[i] = 0
			}
		},
	})
	if err != nil {
		d.mu.Lock()
		d.playing = false
		d.mu.Unlock()
		return fmt.Errorf("open speakers: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		d.mu.Lock()
		d.playing = false
		d.mu.Unlock()
		return fmt.Errorf("start speakers: %w", err)
	}
	d.mu.Lock()
	d.play = dev
	d.mu.Unlock()
	log.Info().Msg("audio: speakers opened")
	return nil
}

// Close releases every device and the context. Safe to call more than once.
func (d *Device) Close() {
	d.StopCapture()
	d.StopLoopback()

	d.mu.Lock()
	dev := d.play
	d.play = nil
	d.playing = false
	d.playBuf = nil
	ctx := d.ctx
	d.ctx = nil
	d.mu.Unlock()

	if dev != nil {
		_ = dev.Stop()
		dev.Uninit()
	}
	if ctx != nil {
		_ = ctx.Uninit()
		ctx.Free()
	}
}
