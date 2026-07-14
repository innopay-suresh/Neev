//go:build !windows

package session

// Chat window is Windows-only in TransportMode (the worker is Windows).
func chatEnsure(send func(string)) {}
func chatShow(text string)         {}
