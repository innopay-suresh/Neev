//go:build windows

package session

import (
	"syscall"
	"unsafe"

	"github.com/rs/zerolog/log"
)

var (
	modadvapi32      = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateExW = modadvapi32.NewProc("RegCreateKeyExW")
	procRegSetValExW = modadvapi32.NewProc("RegSetValueExW")
	procRegCloseKey  = modadvapi32.NewProc("RegCloseKey")
)

const (
	hkeyLocalMachine = 0x80000002
	keySetValue      = 0x0002
	regDWORDType     = 4
)

// triggerSAS generates a real Ctrl+Alt+Del (Secure Attention Sequence). Only a
// SYSTEM process (this transport runs in session 0 as SYSTEM) can do it — a
// normal app's synthetic Ctrl+Alt+Del is ignored by Windows. Mirrors the SYSTEM
// helper's TriggerSAS(): enable the "services may generate SAS" policy, then call
// SendSAS(FALSE). Used so Ctrl+Alt+Del works in TransportMode, where the viewer's
// command reaches the Go transport rather than the Flutter host's UAC bridge.
func triggerSAS() {
	// HKLM\...\Policies\System\SoftwareSASGeneration = 1 (services may send SAS).
	path, _ := syscall.UTF16PtrFromString(
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`)
	var hkey syscall.Handle
	if r, _, _ := procRegCreateExW.Call(
		uintptr(hkeyLocalMachine), uintptr(unsafe.Pointer(path)), 0, 0, 0,
		uintptr(keySetValue), 0, uintptr(unsafe.Pointer(&hkey)), 0); r == 0 {
		name, _ := syscall.UTF16PtrFromString("SoftwareSASGeneration")
		val := uint32(1)
		procRegSetValExW.Call(uintptr(hkey), uintptr(unsafe.Pointer(name)), 0,
			uintptr(regDWORDType), uintptr(unsafe.Pointer(&val)), 4)
		procRegCloseKey.Call(uintptr(hkey))
	}
	// SendSAS(FALSE): called from a service (SYSTEM, session 0).
	proc := syscall.NewLazyDLL("sas.dll").NewProc("SendSAS")
	if err := proc.Find(); err != nil {
		log.Warn().Err(err).Msg("transport: SendSAS/sas.dll not available")
		return
	}
	proc.Call(0) // FALSE
	log.Info().Msg("transport: SAS (Ctrl+Alt+Del) triggered")
}
