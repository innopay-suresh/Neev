package session

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Privacy mode as a DEAD-MAN'S SWITCH.
//
// Blanking the host's screen is the one feature that can lock a user out of
// their own machine, so it must not depend on noticing a disconnect. Every
// disconnect signal can fail: a viewer that crashes or loses the network sends
// no bye, an unclean teardown never reaches dropPeer, and an IPC hiccup drops
// the session-state message. Any of those left the screen black with local
// input blocked and no way back in — reported from the field twice.
//
// So privacy is not a latch that someone must remember to release. It EXPIRES.
// The viewer re-asserts it every few seconds while it wants the screen blanked;
// the moment those stop arriving — for any reason at all — the host restores
// itself. The teardown on session end is still there as the fast path, but
// nothing depends on it any more.
const (
	// How long a privacy assertion stays valid without being renewed. Long
	// enough to ride out a slow frame or a brief stall, short enough that a
	// dead viewer never leaves the screen dark for long.
	privacyTTL = 12 * time.Second
	// How often the watchdog checks. Worst-case blank time after a viewer
	// vanishes is privacyTTL + privacyCheck.
	privacyCheck = 2 * time.Second
)

var (
	privacyMu       sync.Mutex
	privacyIsOn     bool
	privacyDeadline time.Time
)

// assertPrivacy applies a privacy command from the viewer and renews its lease.
// Called for every {"k":"cmd","c":"privacy"} message, including the keepalives
// the viewer repeats while privacy is on.
func assertPrivacy(on bool) {
	privacyMu.Lock()
	changed := on != privacyIsOn
	privacyIsOn = on
	if on {
		privacyDeadline = time.Now().Add(privacyTTL)
	}
	privacyMu.Unlock()

	if changed {
		setPrivacy(on)
		return
	}
	// Unchanged state: this was a renewal, so don't re-drive the OS each time.
	if !on {
		// Re-assert OFF even when we believe it is already off: cheap, and it
		// covers the case where a previous restore failed silently.
		setPrivacy(false)
	}
}

// clearPrivacy forces privacy off and drops the lease. Used on session end and
// on worker shutdown.
func clearPrivacy() {
	privacyMu.Lock()
	was := privacyIsOn
	privacyIsOn = false
	privacyDeadline = time.Time{}
	privacyMu.Unlock()
	if was {
		log.Info().Msg("worker: clearing privacy (session ended)")
	}
	setPrivacy(false)
}

// runPrivacyWatchdog restores the screen if the viewer stops re-asserting
// privacy. This is the guarantee: whatever else fails, the host cannot stay
// blanked for longer than the lease.
func runPrivacyWatchdog(ctx context.Context) {
	t := time.NewTicker(privacyCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// Shutting down must never leave the machine dark.
			clearPrivacy()
			return
		case <-t.C:
			privacyMu.Lock()
			expired := privacyIsOn && time.Now().After(privacyDeadline)
			if expired {
				privacyIsOn = false
			}
			privacyMu.Unlock()
			if expired {
				log.Warn().Dur("ttl", privacyTTL).
					Msg("worker: privacy lease expired — restoring the screen (viewer stopped responding)")
				setPrivacy(false)
			}
		}
	}
}
