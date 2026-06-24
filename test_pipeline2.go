package main

import (
	"fmt"
	"image"
	"github.com/neev/remote-agent/agent/encode"
)

func main() {
	fps := 30
	bitrate := 3000

	enc, err := encode.NewEncoder(1440, 900, fps, bitrate)
	if err != nil {
		fmt.Printf("NewEncoder failed: %v\n", err)
		return
	}

	frame := image.NewRGBA(image.Rect(0, 0, 2880, 1798))
	
	fw, fh := frame.Bounds().Dx(), frame.Bounds().Dy()
	if fw != enc.Width() || fh != enc.Height() {
		fmt.Printf("Recreating encoder: old %dx%d, new %dx%d\n", enc.Width(), enc.Height(), fw, fh)
		newEnc, _ := encode.NewEncoder(fw, fh, fps, enc.Bitrate())
		enc.Close()
		enc = newEnc
	}

	_, err = enc.Encode(frame, true)
	if err != nil {
		fmt.Printf("Encode failed: %v\n", err)
		return
	}

	fmt.Println("Encode succeeded!")
}
