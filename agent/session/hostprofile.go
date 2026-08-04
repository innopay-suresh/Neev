package session

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Host-side access policy, modelled on how AnyDesk actually works.
//
// The decisive idea: whether a connection is prompted depends on HOW the
// controller authenticated, not on one global "ask / don't ask" switch.
//
//   * The unattended password IS the authorisation. It exists precisely for
//     machines with nobody sitting at them, so it must not be prompted.
//   * The session password is an interactive request. A person at the host
//     should still get to accept or refuse it.
//
// Collapsing both into a single boolean is why turning off "Ask before allowing
// connections" used to let ANYONE with the ordinary session password in
// unprompted — strictly weaker than intended.

// Interactive-access states, stored in interactive.txt.
const (
	InteractiveAlways   = "always"    // always prompt an interactive request
	InteractiveWhenOpen = "when-open" // only when the app window is open
	InteractiveNever    = "never"     // unattended password is the ONLY way in
)

// connectAuthMode reports how the controller authenticated, as stamped by the
// relay ("unattended" or "session"). Anything unrecognised — including an older
// relay that doesn't send the field — is treated as "session", the safer of the
// two: it prompts rather than silently admitting.
func connectAuthMode(payload []byte) string {
	var m struct {
		Auth string `json:"auth"`
	}
	if err := json.Unmarshal(payload, &m); err != nil {
		return "session"
	}
	if m.Auth == "unattended" {
		return "unattended"
	}
	return "session"
}

// interactiveAccessMode reads the host's Interactive Access setting.
// Absent/unreadable means "always", preserving existing behaviour.
func interactiveAccessMode() string {
	for _, p := range hostFlagPaths("interactive.txt") {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(data)) {
		case InteractiveNever:
			return InteractiveNever
		case InteractiveWhenOpen:
			return InteractiveWhenOpen
		}
		return InteractiveAlways
	}
	return InteractiveAlways
}

// appHeartbeatTTL is how stale app-open.txt may be before the app counts as
// closed. The app refreshes it every few seconds; this leaves room for a missed
// tick under load without holding the door open long after the app quits.
const appHeartbeatTTL = 15 * time.Second

// appIsOpen reports whether the user's Neev Remote app is running right now.
//
// The transport is a headless service with no view of the user's desktop, so
// the app tells it: while running, the app rewrites app-open.txt with the
// current unix time. A heartbeat rather than a create/delete flag because a
// crashed or force-quit app never gets to delete anything, and a stale flag
// would leave "only while the app is open" permanently open — exactly the
// failure this is meant to remove.
func appIsOpen() bool {
	for _, p := range hostFlagPaths("app-open.txt") {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		secs, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			continue
		}
		if time.Since(time.Unix(secs, 0)) <= appHeartbeatTTL {
			return true
		}
	}
	return false
}

// interactiveAllowed reports whether a session-password (interactive) login may
// be admitted at all.
//
// "when-open" now means what the setting says. It previously behaved exactly
// like "always" — the transport could not see the app window, so it admitted
// everything and the UI's promise that "requests are ignored when the app is
// closed" was simply untrue. The app's heartbeat makes the signal observable.
//
// Refusing is the correct failure direction here: the host explicitly chose to
// be reachable only while sitting in front of the app, so when the signal is
// missing the answer is no.
func (t *Transport) interactiveAllowed() bool {
	switch interactiveAccessMode() {
	case InteractiveNever:
		return false
	case InteractiveWhenOpen:
		open := appIsOpen()
		if !open {
			log.Info().Msg("transport: interactive login refused — " +
				"host set access to 'only while the app is open' and the app is not running")
		}
		return open
	default:
		return true
	}
}

// AccessProfile is the permission set granted to a session. Separate profiles
// for unattended and interactive sessions let an unmanned machine be handed a
// narrower grant than someone sitting in front of it.
type AccessProfile struct {
	Control   bool `json:"control"`
	Clipboard bool `json:"clipboard"`
	Files     bool `json:"files"`
}

// hostProfile returns the profile for this connection mode. Written by the app
// as unattended-profile.json / interactive-profile.json; a missing file means
// full access, which is the historical behaviour.
//
// The host's global "View only mode" still overrides the profile: it is the
// machine owner's standing instruction and must not be undone by a profile that
// happens to say control=true.
func hostProfile(unattended bool) AccessProfile {
	name := "interactive-profile.json"
	if unattended {
		name = "unattended-profile.json"
	}
	p := AccessProfile{Control: true, Clipboard: true, Files: true}
	for _, path := range hostFlagPaths(name) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var got AccessProfile
		if json.Unmarshal(data, &got) == nil {
			p = got
		}
		break
	}
	if hostViewOnlyDefault() {
		p.Control = false
	}
	return p
}
