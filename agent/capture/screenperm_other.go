//go:build !darwin || !cgo

package capture

// ScreenCaptureGranted is macOS-only. Everywhere else there is no such
// permission, so capture failures are never a TCC problem and the caller must
// keep retrying rather than restarting the process.
func ScreenCaptureGranted() bool { return false }
