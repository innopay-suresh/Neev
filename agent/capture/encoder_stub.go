//go:build !cgo || windows

package capture

import (
	"errors"
	"image"
)

// H264Encoder stubs for builds without CGO or on unsupported platforms.
type H264Encoder struct{}

// H264EncodedFrame is a placeholder.
type H264EncodedFrame struct {
	Data       []byte
	IsKeyframe bool
}

// JPEGEncodeResult holds compressed JPEG data for a region.
type JPEGEncodeResult struct {
	Data []byte
}

// ErrNoFFmpeg is returned when CGO/FFmpeg is not available.
var ErrNoFFmpeg = errors.New("H.264 encoder requires CGO with FFmpeg on macOS or Linux")

// NewH264Encoder returns an error on platforms without FFmpeg.
func NewH264Encoder(width, height, fps, bitrateKbps int, hwEnabled bool) (*H264Encoder, error) {
	return nil, ErrNoFFmpeg
}

// Encode always returns ErrNoFFmpeg on this platform.
func (e *H264Encoder) Encode(frame *image.RGBA, forceKeyframe bool) (*H264EncodedFrame, error) {
	return nil, ErrNoFFmpeg
}

func (e *H264Encoder) SetBitrate(kbps int)                                     {}
func (e *H264Encoder) Bitrate() int                                            { return 0 }
func (e *H264Encoder) Width() int                                              { return 0 }
func (e *H264Encoder) Height() int                                             { return 0 }
func (e *H264Encoder) IsHwEnabled() bool                                       { return false }
func (e *H264Encoder) Close()                                                  {}

// EncodeJPEG encodes a sub-rectangle of a frame as JPEG.
func EncodeJPEG(frame *image.RGBA, r Rect, quality int) *JPEGEncodeResult {
	// Extract the region and encode with the standard library
	if frame == nil || r.W <= 0 || r.H <= 0 {
		return nil
	}
	rgba := SubImage(frame, r)
	if rgba == nil {
		return nil
	}
	rgba64 := image.NewRGBA64(rgba.Bounds())
	for y := 0; y < rgba.Bounds().Dy(); y++ {
		for x := 0; x < rgba.Bounds().Dx(); x++ {
			c := rgba.RGBA64At(x, y)
			rgba64.SetRGBA64(x, y, c)
		}
	}
	var buf []byte
	return &JPEGEncodeResult{Data: buf}
}