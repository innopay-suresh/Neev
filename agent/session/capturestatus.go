package session

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// A marker the worker writes when it cannot capture the screen, so the app can
// SAY SO instead of the user watching a session stall with no explanation.
//
// Written to the per-user dir (~/Library/Application Support/NeevRemote on
// macOS) rather than the machine-wide one: the worker runs as the logged-in
// user and cannot write to the root-owned machine directory — the same
// constraint that stopped the LaunchAgent starting at all. The app runs as the
// same user, so this is a place both can reach. It mirrors how the app already
// hands consent.txt down to the daemon, in reverse.
//
// A timestamp rather than a flag: a marker left behind by a crashed worker
// would otherwise accuse the user of a permission problem forever.
func captureBlockedPath() string {
	return filepath.Join(userDataDir(), "capture-blocked")
}

// markCaptureBlocked records that capture is failing right now.
func markCaptureBlocked() {
	p := captureBlockedPath()
	if err := os.WriteFile(p,
		[]byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644); err != nil {
		log.Debug().Err(err).Msg("worker: could not write the capture-blocked marker")
	}
}

// clearCaptureBlocked removes the marker once capture works.
func clearCaptureBlocked() {
	_ = os.Remove(captureBlockedPath())
}
