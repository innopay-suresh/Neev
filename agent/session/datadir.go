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
//   Windows: %ProgramData%\NeevRemote      (e.g. C:\ProgramData\NeevRemote)
//   macOS:   /Library/Application Support/NeevRemote
//   Linux:   /var/lib/NeevRemote
//
// The directory is created 0755 so a per-session worker (running as the logged-in
// user) can still READ creds a root transport wrote. On macOS/Linux, falling back
// to a per-user temp dir when the system dir isn't writable keeps local dev runs
// (no daemon, no root) working — matching the previous os.TempDir() behaviour.
func dataDir() string {
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
