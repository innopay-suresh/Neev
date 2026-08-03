//go:build windows

package session

import (
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	modUser32Desk            = syscall.NewLazyDLL("user32.dll")
	procOpenInputDesktop     = modUser32Desk.NewProc("OpenInputDesktop")
	procSetThreadDesktop     = modUser32Desk.NewProc("SetThreadDesktop")
	procGetThreadDesktopDB   = modUser32Desk.NewProc("GetThreadDesktop")
	procCloseDesktopDB       = modUser32Desk.NewProc("CloseDesktop")
	modKernel32Desk          = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadIDDB = modKernel32Desk.NewProc("GetCurrentThreadId")
)

// bindInputDesktop binds the CURRENT OS thread to the interactive input desktop
// so it may create windows / show common dialogs. A service-spawned worker is
// otherwise denied GUI (even though SendInput works). Call after
// runtime.LockOSThread and before creating any window/dialog on the thread.
// Best-effort: fails at the secure desktop (nothing to show there).
//
// Returns whether the thread is actually bound. SetThreadDesktop fails with
// "The requested resource is in use." when the calling thread still owns a
// window or hook on its current desktop — a transient condition that clears
// once that window is gone, which is why it is retried rather than accepted on
// the first attempt. Callers that MUST have a desktop (a file picker whose
// result is sent to the viewer) have to check this and abort; callers showing
// optional GUI can ignore it and continue as before.
func bindInputDesktop() bool {
	var lastErr error
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(150 * time.Millisecond)
		}
		hdesk, _, err := procOpenInputDesktop.Call(0, 0, 0x10000000 /*GENERIC_ALL*/)
		if hdesk == 0 {
			lastErr = err
			continue
		}
		if r, _, err := procSetThreadDesktop.Call(hdesk); r == 0 {
			lastErr = err
			procCloseDesktopDB.Call(hdesk) // don't leak the handle across retries
			continue
		}
		if i > 0 {
			log.Info().Int("attempt", i+1).Msg("worker: bound to the input desktop after a retry")
		}
		return true
	}
	e := "unknown"
	if lastErr != nil {
		e = lastErr.Error()
	}
	log.Warn().Str("err", e).Msg("worker: SetThreadDesktop failed after 3 attempts — no GUI on this thread")
	return false
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
