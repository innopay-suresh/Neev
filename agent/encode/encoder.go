//go:build cgo
// +build cgo

package encode

/*
#cgo darwin pkg-config: vpx
#cgo windows CFLAGS: -I${SRCDIR}/windows_lib/include
#cgo windows LDFLAGS: -static ${SRCDIR}/windows_lib/lib/libvpx.a
#cgo linux   LDFLAGS: -lvpx
#include "vpx_encode.h"
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

// Encoder wraps the C VP8 encoder context.
// It is goroutine-safe via an internal mutex.
type Encoder struct {
	mu      sync.Mutex
	enc     *C.VpxEncoder
	width   int
	height  int
	fps     int
	bitrate int // kbps
}

// EncodedFrame is a compressed VP8 packet ready for RTP packetization.
type EncodedFrame struct {
	Data       []byte
	IsKeyframe bool
}

// NewEncoder creates a VP8 encoder for the given resolution, FPS, and bitrate.
// bitrate is in kbps (e.g. 1500 for 1.5 Mbps).
func NewEncoder(width, height, fps, bitrateKbps int) (*Encoder, error) {
	enc := C.vpx_encoder_create(
		C.int(width), C.int(height),
		C.int(fps), C.int(bitrateKbps),
	)
	if enc == nil {
		return nil, fmt.Errorf("vpx_encoder_create failed (libvpx not found?)")
	}
	return &Encoder{
		enc:     enc,
		width:   width,
		height:  height,
		fps:     fps,
		bitrate: bitrateKbps,
	}, nil
}

// Encode encodes a single RGBA frame.
// Returns nil if the encoder is still buffering (no output packet yet).
// forceKeyframe forces an IDR frame — use on reconnect.
func (e *Encoder) Encode(frame *image.RGBA, forceKeyframe bool) (*EncodedFrame, error) {
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

	var result C.EncodeResult
	kf := C.int(0)
	if forceKeyframe {
		kf = 1
	}

	rc := C.vpx_encode_frame(e.enc, cPix, kf, &result)
	switch rc {
	case -1:
		return nil, fmt.Errorf("vpx_encode_frame error: %s", C.GoString(&result.error_msg[0]))
	case 1:
		return nil, nil // encoder buffering, no packet yet
	}

	// Copy the packet data to a Go slice before freeing C memory.
	size := int(result.size)
	data := make([]byte, size)
	C.memcpy(unsafe.Pointer(&data[0]), unsafe.Pointer(result.data), C.size_t(size))
	C.vpx_free_packet(result.data)

	return &EncodedFrame{
		Data:       data,
		IsKeyframe: result.is_keyframe != 0,
	}, nil
}

// SetBitrate updates the target bitrate on the fly (adaptive bitrate control).
func (e *Encoder) SetBitrate(kbps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bitrate = kbps
	C.vpx_encoder_set_bitrate(e.enc, C.int(kbps))
}

// Bitrate returns the current target bitrate in kbps.
func (e *Encoder) Bitrate() int {
	e.mu.Lock()
	defer e.mu.Unlock()
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

// Close tears down the encoder and frees C memory.
func (e *Encoder) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.enc != nil {
		C.vpx_encoder_destroy(e.enc)
		e.enc = nil
	}
}
