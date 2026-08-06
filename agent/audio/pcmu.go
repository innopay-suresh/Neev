// Package audio carries in-session voice for the Go host.
//
// Codec choice: G.711 mu-law (PCMU, RTP payload type 0), not Opus.
//
// Opus would sound better, but the only Go bindings are cgo wrappers around
// libopus, which would add a system dependency to BOTH CI runners. The Windows
// toolchain is the one thing in this project that must not wobble — the
// Windows-to-Windows path is the stable baseline — and a new native library on
// that build is exactly the kind of change that breaks it for reasons unrelated
// to audio.
//
// PCMU costs a lookup table and no dependency at all. It is a mandatory-to-
// implement WebRTC codec, so every viewer already decodes it. The tradeoff is
// telephone quality: 8 kHz mono, 64 kbit/s. For "talk to the person whose
// machine you are fixing" that is the right point on the curve — and it is
// bounded, predictable bandwidth alongside a screen share that wants the rest.
package audio

const (
	// SampleRate is fixed by the codec: G.711 is defined at 8 kHz.
	SampleRate = 8000
	// Channels — mono. Voice, not music.
	Channels = 1
	// FrameMillis is the packetization interval. 20 ms is the WebRTC norm:
	// small enough that latency stays conversational, large enough that per
	// packet overhead does not dominate.
	FrameMillis = 20
	// SamplesPerFrame is how many PCM samples make up one RTP packet.
	SamplesPerFrame = SampleRate * FrameMillis / 1000 // 160

	// DeviceRate is the rate the audio HARDWARE is opened at.
	//
	// NOT SampleRate. Asking a device for 8 kHz makes miniaudio run the device
	// itself at 8 kHz — the logs showed hw_rate=8000 on speakers, microphone and
	// loopback alike — and Windows endpoints run at their mix-format rate, with
	// almost none supporting 8 kHz natively. Driving them at a rate they cannot
	// really do produced continuous static on every device on both machines.
	//
	// 48 kHz is supported everywhere, so the device runs where it is happy and
	// the 8 kHz conversion happens here, in code we control and can test.
	DeviceRate = 48000

	// DeviceFrames is one 20 ms period at DeviceRate.
	DeviceFrames = DeviceRate * FrameMillis / 1000 // 960

	// Decim is how many device samples make one wire sample.
	Decim = DeviceRate / SampleRate // 6
)

// Downsample converts DeviceRate mono PCM to SampleRate by averaging each group
// of Decim samples.
//
// Averaged rather than picking every Nth sample: plain decimation folds
// everything above 4 kHz back into the audible band as aliasing, heard as a
// metallic warble on speech. The same reasoning as the macOS helper's
// ScreenCaptureKit path, which does this in Swift.
func Downsample(pcm []int16) []int16 {
	n := len(pcm) / Decim
	if n == 0 {
		return nil
	}
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		sum := 0
		for j := 0; j < Decim; j++ {
			sum += int(pcm[i*Decim+j])
		}
		out[i] = int16(sum / Decim)
	}
	return out
}

// Upsample converts SampleRate mono PCM up to DeviceRate by linear
// interpolation between neighbouring samples.
//
// Interpolated rather than each sample repeated Decim times: sample-and-hold
// creates hard steps between values, which is broadband noise sitting right on
// top of the voice.
func Upsample(pcm []int16) []int16 {
	if len(pcm) == 0 {
		return nil
	}
	out := make([]int16, len(pcm)*Decim)
	for i := range pcm {
		cur := int(pcm[i])
		next := cur
		if i+1 < len(pcm) {
			next = int(pcm[i+1])
		}
		for j := 0; j < Decim; j++ {
			out[i*Decim+j] = int16(cur + (next-cur)*j/Decim)
		}
	}
	return out
}

// muLawBias and muLawClip come from the G.711 definition.
const (
	muLawBias = 0x84
	muLawClip = 32635
)

// EncodeMuLaw compresses one 16-bit signed PCM sample to a single mu-law byte.
//
// Written out rather than table-generated so the algorithm is auditable: this
// is the one piece of the voice path with no library behind it.
func EncodeMuLaw(sample int16) byte {
	sign := byte(0)
	s := int(sample)
	if s < 0 {
		s = -s
		sign = 0x80
	}
	// Clip before biasing, or loud input wraps and turns into noise.
	if s > muLawClip {
		s = muLawClip
	}
	s += muLawBias

	// Find the exponent: the position of the highest set bit above the bias.
	exponent := 7
	for mask := 0x4000; exponent > 0 && s&mask == 0; mask >>= 1 {
		exponent--
	}
	mantissa := (s >> (exponent + 3)) & 0x0F
	// mu-law is transmitted inverted.
	return ^(sign | byte(exponent<<4) | byte(mantissa))
}

// DecodeMuLaw expands one mu-law byte back to a 16-bit PCM sample.
func DecodeMuLaw(u byte) int16 {
	u = ^u
	sign := u & 0x80
	exponent := int(u>>4) & 0x07
	mantissa := int(u & 0x0F)

	sample := ((mantissa << 3) + muLawBias) << exponent
	sample -= muLawBias

	if sign != 0 {
		return int16(-sample)
	}
	return int16(sample)
}

// EncodeFrame compresses a block of PCM samples into mu-law bytes.
func EncodeFrame(pcm []int16) []byte {
	out := make([]byte, len(pcm))
	for i, s := range pcm {
		out[i] = EncodeMuLaw(s)
	}
	return out
}

// DecodeFrame expands mu-law bytes back into PCM samples.
func DecodeFrame(mu []byte) []int16 {
	out := make([]int16, len(mu))
	for i, b := range mu {
		out[i] = DecodeMuLaw(b)
	}
	return out
}

// Mix sums two mu-law frames into one.
//
// Summed in the PCM domain, not the mu-law domain: mu-law is a logarithmic
// encoding, so adding the bytes produces noise rather than a mixture. Clipping
// is handled by EncodeMuLaw, so a loud voice over loud system sound distorts
// instead of wrapping into a crackle.
func Mix(a, b []byte) []byte {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		var sum int
		if i < len(a) {
			sum += int(DecodeMuLaw(a[i]))
		}
		if i < len(b) {
			sum += int(DecodeMuLaw(b[i]))
		}
		if sum > 32767 {
			sum = 32767
		} else if sum < -32768 {
			sum = -32768
		}
		out[i] = EncodeMuLaw(int16(sum))
	}
	return out
}

// SilenceFloor is the peak amplitude below which a frame counts as silence.
//
// About -48 dBFS. Low enough that quiet speech and room tone still pass, high
// enough to stop the electrical hiss a capture device produces when nobody is
// talking — which is otherwise encoded, sent, and played at the far end as a
// constant background noise.
const SilenceFloor = 128

// IsSilent reports whether a frame is below the silence floor.
//
// RustDesk does the same thing (a zero-amplitude gate before the encoder) and
// for the same two reasons: bandwidth, and not transmitting a device's own
// noise floor to the other end.
func IsSilent(pcm []int16) bool {
	for _, s := range pcm {
		if s > SilenceFloor || s < -SilenceFloor {
			return false
		}
	}
	return true
}
