//go:build windows

package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

// Windows clipboard image support for TransportMode: read the host clipboard's
// bitmap (CF_DIB) as PNG, and write a viewer-sent PNG back as CF_DIB. Hand-rolled
// on syscall (no cgo / new deps) to match the rest of the native worker code.

var (
	modUser32Clip                  = syscall.NewLazyDLL("user32.dll")
	procOpenClipboard              = modUser32Clip.NewProc("OpenClipboard")
	procCloseClipboard             = modUser32Clip.NewProc("CloseClipboard")
	procEmptyClipboard             = modUser32Clip.NewProc("EmptyClipboard")
	procGetClipboardData           = modUser32Clip.NewProc("GetClipboardData")
	procSetClipboardData           = modUser32Clip.NewProc("SetClipboardData")
	procIsClipboardFormatAvailable = modUser32Clip.NewProc("IsClipboardFormatAvailable")
	procGetClipboardSequenceNumber = modUser32Clip.NewProc("GetClipboardSequenceNumber")

	modKernel32Clip  = syscall.NewLazyDLL("kernel32.dll")
	procGlobalAlloc  = modKernel32Clip.NewProc("GlobalAlloc")
	procGlobalFree   = modKernel32Clip.NewProc("GlobalFree")
	procGlobalLock   = modKernel32Clip.NewProc("GlobalLock")
	procGlobalUnlock = modKernel32Clip.NewProc("GlobalUnlock")
	procGlobalSize   = modKernel32Clip.NewProc("GlobalSize")
)

const (
	cfDIB        = 8
	gmemMoveable = 0x0002
	biRGB        = 0
	biBitfields  = 3
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// clipboardSeq returns a counter that changes whenever the clipboard content
// changes — cheap way to skip re-reading the (large) image every poll.
func clipboardSeq() uint32 {
	r, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(r)
}

// readClipboardImagePNG returns the host clipboard image as PNG, or (nil,false)
// if there is no bitmap on the clipboard (or an unsupported variant).
func readClipboardImagePNG() ([]byte, bool) {
	if r, _, _ := procIsClipboardFormatAvailable.Call(cfDIB); r == 0 {
		return nil, false
	}
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return nil, false
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfDIB)
	if h == 0 {
		return nil, false
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil, false
	}
	defer procGlobalUnlock.Call(h)
	size, _, _ := procGlobalSize.Call(h)
	if size < 40 {
		return nil, false
	}
	raw := make([]byte, int(size))
	copy(raw, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size)))

	var hdr bitmapInfoHeader
	if err := binary.Read(bytes.NewReader(raw[:40]), binary.LittleEndian, &hdr); err != nil {
		return nil, false
	}
	// Accept 24/32bpp truecolor DIBs. BI_RGB (uncompressed) AND BI_BITFIELDS —
	// most apps (browsers, Snip & Sketch, Office) put 32bpp BI_BITFIELDS on the
	// clipboard with the standard BGRA masks, so rejecting it (as before) made
	// host→viewer image silently fail.
	if hdr.Size < 40 || hdr.Width <= 0 ||
		(hdr.BitCount != 24 && hdr.BitCount != 32) ||
		(hdr.Compression != biRGB && hdr.Compression != biBitfields) {
		return nil, false
	}
	width := int(hdr.Width)
	height := int(hdr.Height)
	topDown := false
	if height < 0 {
		height = -height
		topDown = true
	}
	bpp := int(hdr.BitCount) / 8
	stride := ((width*int(hdr.BitCount) + 31) / 32) * 4
	pixOff := int(hdr.Size) // truecolor DIBs have no palette
	if hdr.Compression == biBitfields {
		pixOff += 12 // three color-mask DWORDs follow the header (assume BGRA)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := y
		if !topDown {
			srcY = height - 1 - y // DIB rows are bottom-up unless height<0
		}
		rowStart := pixOff + srcY*stride
		if rowStart+width*bpp > len(raw) {
			break
		}
		for x := 0; x < width; x++ {
			s := rowStart + x*bpp
			b, g, r := raw[s], raw[s+1], raw[s+2]
			a := byte(255)
			if bpp == 4 {
				if raw[s+3] != 0 { // many 32bpp DIBs leave alpha 0 (opaque)
					a = raw[s+3]
				}
			}
			d := img.PixOffset(x, y)
			img.Pix[d], img.Pix[d+1], img.Pix[d+2], img.Pix[d+3] = r, g, b, a
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// writeClipboardImagePNG decodes a viewer-sent PNG and puts it on the host
// clipboard as a top-down 32bpp CF_DIB (widely pasteable).
func writeClipboardImagePNG(pngBytes []byte) error {
	src, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}
	b := src.Bounds()
	width, height := b.Dx(), b.Dy()
	if width <= 0 || height <= 0 {
		return fmt.Errorf("empty image")
	}
	stride := width * 4
	hdr := bitmapInfoHeader{
		Size: 40, Width: int32(width), Height: int32(-height), // negative = top-down
		Planes: 1, BitCount: 32, Compression: biRGB,
		SizeImage: uint32(stride * height),
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, &hdr)
	pix := make([]byte, stride*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			o := y*stride + x*4
			pix[o], pix[o+1], pix[o+2], pix[o+3] =
				byte(bl>>8), byte(g>>8), byte(r>>8), byte(a>>8) // BGRA
		}
	}
	buf.Write(pix)
	dib := buf.Bytes()

	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(dib)))
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc failed")
	}
	dst, _, _ := procGlobalLock.Call(hMem)
	if dst == 0 {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("GlobalLock failed")
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(dst)), len(dib)), dib)
	procGlobalUnlock.Call(hMem)

	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if r, _, _ := procSetClipboardData.Call(cfDIB, hMem); r == 0 {
		procGlobalFree.Call(hMem) // still ours on failure
		return fmt.Errorf("SetClipboardData failed")
	}
	// On success the system owns hMem — must NOT free it.
	return nil
}
