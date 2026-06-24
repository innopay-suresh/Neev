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

	frame := image.NewRGBA(image.Rect(0, 0, 2880, 1800))
	_, err = enc.Encode(frame, true)
	if err != nil {
		fmt.Printf("Encode failed: %v\n", err)
		return
	}

	fmt.Println("Encode succeeded!")
}
