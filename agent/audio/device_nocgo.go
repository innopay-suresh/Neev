//go:build !cgo

package audio

import "errors"

// Audio device I/O goes through miniaudio, which is C. A build with cgo
// disabled therefore cannot open a microphone or speakers.
//
// This stub exists so that such a build still COMPILES. Without it the whole
// agent fails to build the moment CGO_ENABLED=0 — cross-compiling, a container
// image, a quick vet on another platform — and voice, a secondary feature,
// would be taking down the remote desktop itself. Voice degrades to
// unavailable; everything else is unaffected.
type Device struct{}

var errNoCGO = errors.New("audio: built without cgo — voice unavailable")

func NewDevice() (*Device, error)                  { return nil, errNoCGO }
func (d *Device) StartCapture(func([]int16)) error { return errNoCGO }
func (d *Device) StopCapture()                     {}
func (d *Device) Play([]int16)                     {}
func (d *Device) Close()                           {}

// System-sound capture is WASAPI-only and needs cgo either way.
func LoopbackSupported() bool                       { return false }
func (d *Device) StartLoopback(func([]int16)) error { return errNoCGO }
func (d *Device) StopLoopback()                     {}

// Opus needs cgo too, so a no-cgo build has no codec. Declared here for the
// same reason as the device stubs: a secondary feature must never stop the
// remote desktop itself from building.
type OpusEncoder struct{}
type OpusDecoder struct{}

func NewOpusEncoder() (*OpusEncoder, error)           { return nil, errNoCGO }
func (e *OpusEncoder) Encode([]int16) ([]byte, error) { return nil, errNoCGO }
func NewOpusDecoder() (*OpusDecoder, error)           { return nil, errNoCGO }
func (d *OpusDecoder) Decode([]byte) ([]int16, error) { return nil, errNoCGO }
