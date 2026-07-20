//go:build !windows && !darwin

package session

// handleCommand is a no-op off Windows (TransportMode is Windows-only).
func handleCommand(payload []byte) bool { return false }
