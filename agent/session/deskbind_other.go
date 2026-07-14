//go:build !windows

package session

// bindInputDesktop is a no-op off Windows.
func bindInputDesktop() {}
