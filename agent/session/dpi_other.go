//go:build !windows

package session

// setProcessDpiAware is a no-op off Windows (DPI awareness is a Windows concept).
func setProcessDpiAware() {}
