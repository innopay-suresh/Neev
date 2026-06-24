//go:build !darwin || !cgo
// +build !darwin !cgo

package capture

// RequestScreenCapturePermission is a no-op on non-macOS platforms.
func RequestScreenCapturePermission() error {
	return nil
}
