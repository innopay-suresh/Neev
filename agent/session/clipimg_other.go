//go:build !windows

package session

// Image clipboard is Windows-only in TransportMode (the worker is Windows).
func clipboardSeq() uint32                       { return 0 }
func readClipboardImagePNG() ([]byte, bool)      { return nil, false }
func writeClipboardImagePNG(pngBytes []byte) error { return nil }
