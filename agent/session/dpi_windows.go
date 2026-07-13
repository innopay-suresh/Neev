//go:build windows

package session

import "syscall"

// setProcessDpiAware marks the capture-worker process PER_MONITOR_AWARE_V2 so
// GDI/GetSystemMetrics and the capture DCs all report PHYSICAL pixels on scaled
// displays (125/150/175%). Without it the worker is DPI-UNAWARE: GetSystemMetrics
// (used by the GDI capture) returns LOGICAL/downscaled dimensions while
// GetDeviceCaps(DESKTOPHORZRES) (used by Bounds) reports PHYSICAL — that
// logical/physical split is a classic cause of a captured frame that loses the
// right/bottom edges of a high-DPI desktop. Must run before any capture DC is
// created (i.e. before NewPlatformCapture). Best-effort: pre-1703 Windows without
// the API simply stays unaware (unchanged behaviour).
func setProcessDpiAware() {
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")
	if proc.Find() != nil {
		return
	}
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 == (HANDLE)-4
	const perMonitorAwareV2 = ^uintptr(3)
	proc.Call(perMonitorAwareV2)
}
