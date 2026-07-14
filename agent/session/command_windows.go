//go:build windows

package session

import (
	"encoding/json"
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"
)

// Session commands ({"k":"cmd","c":...}) that the viewer sends over the control
// channel. On the standard Flutter host these run in-app; in TransportMode the
// app is UI-only, so the capture worker (running as the logged-in user) executes
// them. lock/logoff/reboot are supported here; sas/privacy are handled elsewhere
// (or are follow-ups) and are consumed so they never reach the input injector.
var (
	modUser32Cmd              = syscall.NewLazyDLL("user32.dll")
	procLockWorkStation       = modUser32Cmd.NewProc("LockWorkStation")
	procExitWindowsEx         = modUser32Cmd.NewProc("ExitWindowsEx")
	modAdvapi32Cmd            = syscall.NewLazyDLL("advapi32.dll")
	procOpenProcessToken      = modAdvapi32Cmd.NewProc("OpenProcessToken")
	procLookupPrivilegeValueW = modAdvapi32Cmd.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges = modAdvapi32Cmd.NewProc("AdjustTokenPrivileges")
	modKernel32Cmd            = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProcessCmd  = modKernel32Cmd.NewProc("GetCurrentProcess")
)

const (
	ewxLogoff             = 0x00000000
	ewxReboot             = 0x00000002
	ewxForceIfHung        = 0x00000010
	tokenAdjustPrivileges = 0x0020
	tokenQuery            = 0x0008
	sePrivilegeEnabled    = 0x00000002
)

type luid struct {
	LowPart  uint32
	HighPart int32
}
type luidAndAttributes struct {
	Luid       luid
	Attributes uint32
}
type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luidAndAttributes
}

// enableShutdownPrivilege grants this process SeShutdownPrivilege, required by
// ExitWindowsEx for reboot/shutdown (not for logoff). Best-effort.
func enableShutdownPrivilege() {
	proc, _, _ := procGetCurrentProcessCmd.Call()
	var tok syscall.Handle
	r, _, _ := procOpenProcessToken.Call(proc,
		tokenAdjustPrivileges|tokenQuery, uintptr(unsafe.Pointer(&tok)))
	if r == 0 {
		return
	}
	defer syscall.CloseHandle(tok)
	name, err := syscall.UTF16PtrFromString("SeShutdownPrivilege")
	if err != nil {
		return
	}
	var lu luid
	r, _, _ = procLookupPrivilegeValueW.Call(0,
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&lu)))
	if r == 0 {
		return
	}
	tp := tokenPrivileges{PrivilegeCount: 1}
	tp.Privileges[0] = luidAndAttributes{Luid: lu, Attributes: sePrivilegeEnabled}
	procAdjustTokenPrivileges.Call(uintptr(tok), 0,
		uintptr(unsafe.Pointer(&tp)), 0, 0, 0)
}

// handleCommand runs a viewer session command in the worker's (user) session.
// Returns true if the payload was a {"k":"cmd"} message (so the caller does NOT
// re-interpret it as input), whether or not the specific command is supported.
func handleCommand(payload []byte) bool {
	var m struct {
		K  string `json:"k"`
		C  string `json:"c"`
		On bool   `json:"on"`
	}
	if err := json.Unmarshal(payload, &m); err != nil || m.K != "cmd" {
		return false
	}
	switch m.C {
	case "lock":
		procLockWorkStation.Call()
	case "logoff":
		procExitWindowsEx.Call(ewxLogoff|ewxForceIfHung, 0)
	case "reboot":
		enableShutdownPrivilege()
		procExitWindowsEx.Call(ewxReboot|ewxForceIfHung, 0)
	case "privacy":
		setPrivacy(m.On) // blank the local screen (excluded from capture) + block local input
	default:
		// sas / privacy / anything new: consume it (don't inject as input) but
		// note it's not yet carried over the transport.
		log.Info().Str("cmd", m.C).
			Msg("worker: session command not yet supported over transport")
		return true
	}
	log.Info().Str("cmd", m.C).Msg("worker: executed session command")
	return true
}
