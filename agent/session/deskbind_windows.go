//go:build windows

package session

import (
	"syscall"

	"github.com/rs/zerolog/log"
)

var (
	modUser32Desk        = syscall.NewLazyDLL("user32.dll")
	procOpenInputDesktop = modUser32Desk.NewProc("OpenInputDesktop")
	procSetThreadDesktop = modUser32Desk.NewProc("SetThreadDesktop")
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
