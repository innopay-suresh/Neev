//go:build !cgo
// +build !cgo

package encode

import (
	"image"
)

// Encoder is a mock VP8 encoder used when CGO is disabled.
type Encoder struct {
	width   int
	height  int
	bitrate int
}

// EncodedFrame is a mock frame payload.
type EncodedFrame struct {
	Data       []byte
	IsKeyframe bool
}

// NewEncoder creates a mock encoder instance.
func NewEncoder(width, height, fps, bitrateKbps int) (*Encoder, error) {
	return &Encoder{width: width, height: height, bitrate: bitrateKbps}, nil
}

// Encode returns dummy bytes to satisfy the RTP pipeline without compression logic.
func (e *Encoder) Encode(frame *image.RGBA, forceKeyframe bool) (*EncodedFrame, error) {
	return &EncodedFrame{
		Data:       make([]byte, 12),
		IsKeyframe: true,
	}, nil
}

// SetBitrate updates the mock bitrate.
func (e *Encoder) SetBitrate(kbps int) {
	e.bitrate = kbps
}

// Bitrate returns the current mock bitrate.
func (e *Encoder) Bitrate() int {
	return e.bitrate
}

// Width returns the encoder's width.
func (e *Encoder) Width() int {
	return e.width
}

// Height returns the encoder's height.
func (e *Encoder) Height() int {
	return e.height
}

// Close is a no-op.
func (e *Encoder) Close() {}
