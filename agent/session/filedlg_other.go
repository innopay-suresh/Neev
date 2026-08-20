//go:build !windows && !darwin

package session

// showOpenFileDialog is a no-op off Windows (TransportMode is Windows-only).
func showOpenFileDialog() (string, bool) { return "", false }
