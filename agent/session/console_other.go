//go:build !darwin

package session

// isOnConsole is macOS-only session-follow logic. Everywhere else the worker is
// already spawned into the active session (Windows: WTSQueryUserToken), so it's
// always "on console" — no behavior change.
func isOnConsole() bool { return true }
