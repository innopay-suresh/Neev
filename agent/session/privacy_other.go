//go:build !windows

package session

// setPrivacy is a no-op off Windows (TransportMode is Windows-only).
func setPrivacy(on bool) {}
