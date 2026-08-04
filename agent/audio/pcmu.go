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
)

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
