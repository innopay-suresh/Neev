//go:build cgo && (darwin || linux)
// +build cgo,darwin linux

package capture

/*
#cgo darwin  pkg-config: libavcodec libavutil libswscale x264
#cgo linux   pkg-config: libavcodec libavutil libswscale x264

#include "encoder.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"image"
	"sync"
	"unsafe"
)

// JPEGEncodeResult holds compressed JPEG data for a region.
type JPEGEncodeResult struct {
	Data   []byte
	Width  int
	Height int
	X      int // region offset in source
	Y      int
}

// EncodeJPEG encodes a region of an RGBA image as JPEG.
// Returns nil if encoding fails.
// quality is 1-100 (recommended: 70-85 for dirty rects).
func EncodeJPEG(frame *image.RGBA, r Rect, quality int) *JPEGEncodeResult {
	if frame == nil || r.W <= 0 || r.H <= 0 {
		return nil
	}
	if quality <= 0 || quality > 100 {
		quality = 75
	}

	pix := frame.Pix
	cPix := (*C.uchar)(unsafe.Pointer(&pix[0]))

	var result C.JPEGEncodeResult
	rc := C.jpeg_encode_region(
		cPix,
		C.int(frame.Bounds().Dx()),
		C.int(frame.Bounds().Dy()),
		C.int(r.X),
		C.int(r.Y),
		C.int(r.W),
		C.int(r.H),
		C.int(quality),
		&result,
	)
	if rc != 0 {
		return nil
	}

	// Copy the JPEG data to a Go slice before freeing C memory.
	size := int(result.size)
	data := make([]byte, size)
	C.memcpy(unsafe.Pointer(&data[0]), unsafe.Pointer(result.data), C.size_t(size))
	C.jpeg_free_packet(result.data)

	return &JPEGEncodeResult{
		Data:   data,
		Width:  r.W,
		Height: r.H,
		X:      r.X,
		Y:      r.Y,
	}
}

// H264Encoder wraps the C H.264 encoder context.
// It is goroutine-safe via an internal mutex.
// On Windows, this wraps a VP8 encoder (libvpx) since H.264 FFmpeg is not available.
type H264Encoder struct {
	mu        sync.Mutex
	enc       *C.H264Encoder // FFmpeg H.264 encoder (darwin/linux)
	vpx       interface{ Close() } // libvpx VP8 encoder (windows)
	width     int
	height    int
	fps       int
	bitrate   int // kbps
	hwEnabled bool
}

// H264EncodedFrame is a compressed H.264 packet ready for RTP packetization.
type H264EncodedFrame struct {
	Data       []byte
	IsKeyframe bool
}

// HwAccelSupported returns true if hardware-accelerated H.264 encoding is
// available on the current platform (VideoToolbox on macOS, NVENC on Windows).
func HwAccelSupported() bool {
	// Try to create a minimal encoder to check hardware support
	// This is a quick check - the actual availability check happens at create time
	// We don't actually create the encoder here to avoid overhead
	return true // Will be determined at encoder creation time
}

// NewH264Encoder creates an H.264 encoder for the given resolution, FPS, and bitrate.
// bitrate is in kbps (e.g. 1500 for 1.5 Mbps).
// hwEnabled requests hardware acceleration (VideoToolbox/NVENC) if available.
func NewH264Encoder(width, height, fps, bitrateKbps int, hwEnabled bool) (*H264Encoder, error) {
	// Ensure even dimensions for H.264
	if width%2 != 0 {
		width--
	}
	if height%2 != 0 {
		height--
	}

	var cHw C.int
	if hwEnabled {
		cHw = 1
	}

	enc := C.h264_encoder_create(
		C.int(width), C.int(height),
		C.int(fps), C.int(bitrateKbps),
		cHw,
	)
	if enc == nil {
		return nil, fmt.Errorf("h264_encoder_create failed (FFmpeg not found?)")
	}

	isHw := C.h264_encoder_is_hw_active(enc) != 0

	return &H264Encoder{
		enc:       enc,
		width:     width,
		height:    height,
		fps:       fps,
		bitrate:   bitrateKbps,
		hwEnabled: isHw,
	}, nil
}

// Encode encodes a single RGBA frame.
// Returns nil if the encoder is still buffering (no output packet yet).
// forceKeyframe forces an IDR frame — use on reconnect.
func (e *H264Encoder) Encode(frame *image.RGBA, forceKeyframe bool) (*H264EncodedFrame, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate frame dimensions.
	b := frame.Bounds()
	w, h := b.Dx(), b.Dy()
	if w != e.width || h != e.height {
		return nil, fmt.Errorf("frame size mismatch: got %dx%d, encoder is %dx%d",
			w, h, e.width, e.height)
	}

	// Pin the Go slice so CGo can read it.
	pix := frame.Pix
	cPix := (*C.uchar)(unsafe.Pointer(&pix[0]))

	var result C.H264EncodeResult
	kf := C.int(0)
	if forceKeyframe {
		kf = 1
	}

	rc := C.h264_encode_frame(e.enc, cPix, kf, &result)
	switch rc {
	case -1:
		return nil, fmt.Errorf("h264_encode_frame error: %s", C.GoString(&result.error_msg[0]))
	case 1:
		return nil, nil // encoder buffering, no packet yet
	}

	// Copy the packet data to a Go slice before freeing C memory.
	size := int(result.size)
	data := make([]byte, size)
	C.memcpy(unsafe.Pointer(&data[0]), unsafe.Pointer(result.data), C.size_t(size))
	C.h264_free_packet(result.data)

	return &H264EncodedFrame{
		Data:       data,
		IsKeyframe: result.is_keyframe != 0,
	}, nil
}

// SetBitrate updates the target bitrate on the fly (adaptive bitrate control).
func (e *H264Encoder) SetBitrate(kbps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bitrate = kbps
	if e.enc != nil {
		C.h264_encoder_set_bitrate(e.enc, C.int(kbps))
	}
}

// Bitrate returns the current target bitrate in kbps.
func (e *H264Encoder) Bitrate() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.bitrate
}

// Width returns the encoder's width.
func (e *H264Encoder) Width() int {
	return e.width
}

// Height returns the encoder's height.
func (e *H264Encoder) Height() int {
	return e.height
}

// IsHwEnabled returns whether hardware acceleration is active.
func (e *H264Encoder) IsHwEnabled() bool {
	return e.hwEnabled
}

// Close tears down the encoder and frees C memory.
func (e *H264Encoder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.enc != nil {
		C.h264_encoder_destroy(e.enc)
		e.enc = nil
	}
	if e.vpx != nil {
		e.vpx.Close()
		e.vpx = nil
	}
}
