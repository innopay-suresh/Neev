//go:build windows

package session

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	modUser32Consent = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = modUser32Consent.NewProc("MessageBoxW")
)

const (
	mbYesNo         = 0x00000004
	mbIconQuestion  = 0x00000020
	mbSystemModal   = 0x00001000
	mbTopMost       = 0x00040000
	mbSetForeground = 0x00010000
	idYes           = 6
)

// showConsentDialog shows a modal Accept/Deny box to the logged-in user on the
// interactive desktop and returns true only if they click Yes (Accept). Blocks
// until answered (the transport applies its own timeout → deny). Bind the thread
// to the input desktop first (same reason the chat window / file picker do), or a
// worker thread lands on a non-interactive desktop and the box never appears.
func showConsentDialog(viewerID string) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	bindInputDesktop()
	text, _ := syscall.UTF16PtrFromString(
		"A remote user (" + viewerID + ") wants to connect to this computer.\n\n" +
			"Allow this connection?")
	caption, _ := syscall.UTF16PtrFromString("Neev Remote — Allow connection?")
	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(caption)),
		uintptr(mbYesNo|mbIconQuestion|mbSystemModal|mbTopMost|mbSetForeground),
	)
	return ret == idYes
}
