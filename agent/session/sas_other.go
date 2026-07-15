//go:build !windows

package session

// triggerSAS is a no-op off Windows (Ctrl+Alt+Del / SendSAS is Windows-only).
func triggerSAS() {}
