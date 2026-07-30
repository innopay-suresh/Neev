//go:build !windows

package session

// bindInputDesktop is a no-op off Windows: there is no desktop object to bind,
// so report success and let callers proceed.
func bindInputDesktop() bool { return true }
