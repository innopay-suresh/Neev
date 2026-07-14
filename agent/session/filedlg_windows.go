//go:build windows

package session

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Native "open file" dialog for host→viewer export: when the viewer asks the
// host for a file, the worker (running on the user's desktop) pops the standard
// Windows picker. The viewer, controlling that desktop, navigates + selects it;
// the chosen path comes back and the worker streams the file to the viewer.

var (
	modComdlg32          = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileNameW = modComdlg32.NewProc("GetOpenFileNameW")
)

// openFileNameW mirrors OPENFILENAMEW on x64 — field order + Go's natural
// alignment match the C struct (uint32→pointer transitions pad identically).
type openFileNameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

const (
	ofnExplorer     = 0x00080000
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir  = 0x00000008
)

// showOpenFileDialog blocks (on a locked OS thread — GetOpenFileNameW runs its
// own modal message loop) until the user picks a file or cancels.
func showOpenFileDialog() (string, bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	buf := make([]uint16, 4096)
	title, _ := syscall.UTF16PtrFromString("Neev Remote — choose a file to send to the viewer")

	var ofn openFileNameW
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.lpstrFile = &buf[0]
	ofn.nMaxFile = uint32(len(buf))
	ofn.lpstrTitle = title
	ofn.flags = ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir

	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false // cancelled or error
	}
	return syscall.UTF16ToString(buf), true
}
