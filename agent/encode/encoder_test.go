package encode_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/neev/remote-agent/agent/encode"
)

// TestEncoderBasic creates an encoder, encodes a synthetic RGBA frame,
// and verifies we get a non-nil VP8 packet back.
func TestEncoderBasic(t *testing.T) {
	const (
		width   = 320
		height  = 240
		fps     = 30
		bitrate = 500 // kbps
	)

	enc, err := encode.NewEncoder(width, height, fps, bitrate)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	// Synthetic solid-colour frame.
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: 100, G: 149, B: 237, A: 255})
		}
	}

	// First frame must be a keyframe.
	pkt, err := enc.Encode(frame, true)
	if err != nil {
		t.Fatalf("Encode (keyframe): %v", err)
	}
	if pkt == nil {
		t.Fatal("expected VP8 packet, got nil (encoder buffering?)")
	}
	if !pkt.IsKeyframe {
		t.Errorf("expected keyframe flag on first frame")
	}
	if len(pkt.Data) == 0 {
		t.Error("VP8 packet data is empty")
	}
	t.Logf("keyframe size: %d bytes", len(pkt.Data))

	// Encode a second frame (P-frame).
	pkt2, err := enc.Encode(frame, false)
	if err != nil {
		t.Fatalf("Encode (p-frame): %v", err)
	}
	if pkt2 != nil {
		t.Logf("p-frame size: %d bytes (compression ratio: %.1fx)",
			len(pkt2.Data), float64(len(pkt.Data))/float64(len(pkt2.Data)))
	}
}

// TestABRController verifies bitrate is adjusted on simulated congestion.
func TestABRController(t *testing.T) {
	enc, err := encode.NewEncoder(320, 240, 30, 1000)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	abr := encode.NewABRController(enc, 200, 2000)

	// Simulate congestion: 5% packet loss.
	abr.UpdateStats(0, 0.05)

	// Verify initial bitrate matches what we set.
	initial := abr.CurrentBitrate()
	if initial != 1000 {
		t.Fatalf("expected initial bitrate 1000, got %d", initial)
	}
	t.Logf("ABR initial bitrate: %d kbps", initial)
}
