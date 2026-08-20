package session

import (
	"os"
	"path/filepath"
	"runtime"
)

// dataDir returns the machine-wide directory where the transport/worker keep
// their credentials and logs. It must be identical for the root transport
// (LaunchDaemon / SYSTEM service) and any per-session worker so a viewer's
// machine.dat and the written transport creds line up across processes.
//
//	Windows: %ProgramData%\NeevRemote      (e.g. C:\ProgramData\NeevRemote)
//	macOS:   /Library/Application Support/NeevRemote
//	Linux:   /var/lib/NeevRemote
//
// The directory is created 0755 so a per-session worker (running as the logged-in
// user) can still READ creds a root transport wrote. On macOS/Linux, falling back
// to a per-user temp dir when the system dir isn't writable keeps local dev runs
// (no daemon, no root) working — matching the previous os.TempDir() behaviour.
func dataDir() string {
	// Test/dev override. Not a supported deployment knob: the transport runs as
	// a service and does not inherit a user's environment, so this cannot be
	// used to redirect a real host's state.
	if d := os.Getenv("NEEV_DATA_DIR"); d != "" {
		return d
	}
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("ProgramData")
	case "darwin":
		base = "/Library/Application Support"
	default: // linux and friends
		base = "/var/lib"
	}
	if base != "" {
		dir := filepath.Join(base, "NeevRemote")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		}
	}
	// Unprivileged fallback (local dev, no daemon): a temp dir we can always write.
	dir := filepath.Join(os.TempDir(), "NeevRemote")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// userDataDir returns a directory the CURRENT USER can always write, for state
// that belongs to that user rather than the machine.
//
// dataDir() is deliberately machine-wide and is created root/SYSTEM-owned by
// the daemon, so a capture worker — which runs as the logged-in user on both
// Windows (per-session) and macOS (Aqua LaunchAgent) — cannot write into it.
// Writing user state there fails with "permission denied" and is silently lost;
// hostlog.go already hit this and had to fall back to a per-user path.
//
//	Windows: %LOCALAPPDATA%\NeevRemote
//	macOS:   ~/Library/Application Support/NeevRemote
//	Linux:   $XDG_DATA_HOME/NeevRemote or ~/.local/share/NeevRemote
func userDataDir() string {
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("APPDATA")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "Library", "Application Support")
		}
	default:
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			if home, err := os.UserHomeDir(); err == nil {
				base = filepath.Join(home, ".local", "share")
			}
		}
	}
	if base != "" {
		dir := filepath.Join(base, "NeevRemote")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		}
	}
	dir := filepath.Join(os.TempDir(), "NeevRemote")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}
