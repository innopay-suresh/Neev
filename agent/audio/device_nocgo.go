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

func NewDevice() (*Device, error)                 { return nil, errNoCGO }
func (d *Device) StartCapture(func([]byte)) error { return errNoCGO }
func (d *Device) StopCapture()                    {}
func (d *Device) Play([]byte)                     {}
func (d *Device) Close()                          {}

// System-sound capture is WASAPI-only and needs cgo either way.
func LoopbackSupported() bool                      { return false }
func (d *Device) StartLoopback(func([]byte)) error { return errNoCGO }
func (d *Device) StopLoopback()                    {}
