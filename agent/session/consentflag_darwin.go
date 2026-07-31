//go:build darwin

package session

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// consentFlagPaths lists every location the "Ask before allowing connections"
// flag may live, most authoritative first.
//
// On macOS the transport runs as root out of /Library/Application Support/
// NeevRemote, which the daemon creates root-owned 0755 — so the Flutter app,
// running as the user, CANNOT write consent.txt there. That is why the flag was
// Windows-only and why a Mac daemon host silently auto-accepted every
// connection regardless of the setting.
//
// The app therefore writes the flag into its own
// ~/Library/Application Support/NeevRemote/consent.txt, and the root transport
// reads the CONSOLE user's copy (root can read any user's file). The
// system-wide path is still checked first so an admin or MDM can force the
// setting on for every account.
func consentFlagPaths() []string { return hostFlagPaths("consent.txt") }

// hostFlagPaths lists every location a host setting flag may live.
func hostFlagPaths(name string) []string {
	paths := []string{filepath.Join(dataDir(), name)}
	if home := consoleUserHome(); home != "" {
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "NeevRemote", name))
	}
	return paths
}

// consoleUserHome returns the home directory of the user currently logged in at
// the physical display, or "" when nobody is (the login window). /dev/console is
// owned by that user, which is the standard way to identify them without
// linking SystemConfiguration.
func consoleUserHome() string {
	fi, err := os.Stat("/dev/console")
	if err != nil {
		return ""
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	// root owning /dev/console means the login window — no logged-in user.
	if st.Uid == 0 {
		return ""
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(st.Uid), 10))
	if err != nil {
		return ""
	}
	return u.HomeDir
}
