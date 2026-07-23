//go:build windows

package session

import (
	"runtime"
	"strings"
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
	// Restore + close the desktop before unlocking, so this pooled thread isn't
	// returned to the Go runtime bound to the input desktop — otherwise a later
	// clipboard access that lands on this reused thread runs under the wrong
	// desktop (the reported "clipboard stopped working" while consent is on).
	restore := bindInputDesktopSaved()
	defer restore()
	text, _ := syscall.UTF16PtrFromString(
		"A remote device is requesting to connect and control this computer.\n\n"+
			"Device ID:  "+prettyConsentID(viewerID)+"\n\n"+
			"Only allow if you recognise this request.")
	caption, _ := syscall.UTF16PtrFromString("Neev Remote  —  Allow connection?")
	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(caption)),
		uintptr(mbYesNo|mbIconQuestion|mbSystemModal|mbTopMost|mbSetForeground),
	)
	return ret == idYes
}

// prettyConsentID strips the internal "ctrl-" prefix and groups a 9-digit id as
// "XXX XXX XXX" so the prompt reads like the ID the user shares, not a raw token.
func prettyConsentID(id string) string {
	id = strings.TrimPrefix(id, "ctrl-")
	digits := make([]rune, 0, len(id))
	for _, r := range id {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 9 {
		return string(digits[0:3]) + " " + string(digits[3:6]) + " " + string(digits[6:9])
	}
	return id
}
