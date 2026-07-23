//go:build windows

package session

import (
	"syscall"

	"github.com/rs/zerolog/log"
)

var (
	modUser32Desk         = syscall.NewLazyDLL("user32.dll")
	procOpenInputDesktop  = modUser32Desk.NewProc("OpenInputDesktop")
	procSetThreadDesktop  = modUser32Desk.NewProc("SetThreadDesktop")
	procGetThreadDesktopDB   = modUser32Desk.NewProc("GetThreadDesktop")
	procCloseDesktopDB       = modUser32Desk.NewProc("CloseDesktop")
	modKernel32Desk          = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadIDDB = modKernel32Desk.NewProc("GetCurrentThreadId")
)

// bindInputDesktop binds the CURRENT OS thread to the interactive input desktop
// so it may create windows / show common dialogs. A service-spawned worker is
// otherwise denied GUI (even though SendInput works). Call after
// runtime.LockOSThread and before creating any window/dialog on the thread.
// Best-effort: fails silently at the secure desktop (nothing to show there).
func bindInputDesktop() {
	hdesk, _, _ := procOpenInputDesktop.Call(0, 0, 0x10000000 /*GENERIC_ALL*/)
	if hdesk == 0 {
		return
	}
	if r, _, err := procSetThreadDesktop.Call(hdesk); r == 0 {
		log.Warn().Str("err", err.Error()).Msg("worker: SetThreadDesktop failed")
	}
}

// bindInputDesktopSaved binds the interactive input desktop for TRANSIENT GUI on
// a pooled OS thread and returns a restore func. Unlike bindInputDesktop, it
// restores the thread's previous desktop and closes the opened handle, so the
// thread is NOT returned to the Go pool bound to a leaked input-desktop HDESK
// (that pollution can make a later clipboard/OpenClipboard call on the reused
// thread run under the wrong desktop). Call after runtime.LockOSThread; defer the
// returned func before runtime.UnlockOSThread.
func bindInputDesktopSaved() func() {
	tid, _, _ := procGetCurrentThreadIDDB.Call()
	prev, _, _ := procGetThreadDesktopDB.Call(tid)
	hdesk, _, _ := procOpenInputDesktop.Call(0, 0, 0x10000000 /*GENERIC_ALL*/)
	if hdesk == 0 {
		return func() {}
	}
	procSetThreadDesktop.Call(hdesk)
	return func() {
		if prev != 0 {
			procSetThreadDesktop.Call(prev)
		}
		procCloseDesktopDB.Call(hdesk)
	}
}
