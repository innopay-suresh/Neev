//go:build cgo

package audio

import (
	"fmt"

	"layeh.com/gopus"
)

// Opus, replacing G.711 mu-law as the voice codec.
//
// PCMU is 8 kHz telephone audio: intelligible, obviously a phone call. Opus at
// 48 kHz is what makes a call sound like Teams rather than a landline, at LOWER
// bitrate than PCMU's fixed 64 kbit/s. It is also what every WebRTC stack
// prefers, so the viewer's libwebrtc handles it natively with no munging.
//
// It fits the device layer exactly: audio hardware already runs at 48 kHz (see
// DeviceRate), and Opus is natively 48 kHz, so the resampling that PCMU forced
// on every frame disappears — one less place for the arithmetic to be wrong.
//
// layeh.com/gopus rather than a libopus binding: on amd64 (Windows and Linux)
// it COMPILES the bundled Opus sources, so there is no system library to
// install on either CI runner and no DLL to ship. Adding a native dependency to
// the Windows toolchain is exactly the kind of change that has broken this
// build before, and this avoids it entirely.

// OpusEncoder compresses 48 kHz mono PCM for the wire.
type OpusEncoder struct {
	enc *gopus.Encoder
}

// NewOpusEncoder builds an encoder tuned for voice.
func NewOpusEncoder() (*OpusEncoder, error) {
	// VoIP application mode: Opus biases towards speech intelligibility and
	// enables its own noise handling, rather than trying to be faithful to
	// music.
	enc, err := gopus.NewEncoder(OpusRate, Channels, gopus.Voip)
	if err != nil {
		return nil, fmt.Errorf("opus encoder: %w", err)
	}
	enc.SetBitrate(opusBitrate)
	return &OpusEncoder{enc: enc}, nil
}

// Encode compresses exactly one frame.
func (e *OpusEncoder) Encode(pcm []int16) ([]byte, error) {
	if len(pcm) != OpusFrameSamples {
		return nil, fmt.Errorf("opus: got %d samples, need exactly %d",
			len(pcm), OpusFrameSamples)
	}
	out, err := e.enc.Encode(pcm, OpusFrameSamples, opusMaxPacket)
	if err != nil {
		return nil, fmt.Errorf("opus encode: %w", err)
	}
	return out, nil
}

// OpusDecoder expands wire frames back to 48 kHz mono PCM.
type OpusDecoder struct {
	dec *gopus.Decoder
}

// NewOpusDecoder builds a decoder matching NewOpusEncoder.
func NewOpusDecoder() (*OpusDecoder, error) {
	dec, err := gopus.NewDecoder(OpusRate, Channels)
	if err != nil {
		return nil, fmt.Errorf("opus decoder: %w", err)
	}
	return &OpusDecoder{dec: dec}, nil
}

// Decode expands one packet.
func (d *OpusDecoder) Decode(pkt []byte) ([]int16, error) {
	if len(pkt) == 0 {
		return nil, nil
	}
	// fec=false: this stream has no forward error correction, and asking for it
	// on a packet that carries none returns garbage rather than an error.
	pcm, err := d.dec.Decode(pkt, OpusFrameSamples, false)
	if err != nil {
		return nil, fmt.Errorf("opus decode: %w", err)
	}
	return pcm, nil
}
