package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// Remembered consent decisions, keyed by the viewer's device id.
//
// "Remember this decision" on the consent prompt writes here: a remembered
// Accept auto-accepts that device on every later connection, a remembered
// Decline auto-declines it, and neither shows the prompt again. The password
// check is unaffected — this only skips the human Accept/Deny step.
//
// Stored in the USER's own data dir, not the machine-wide one: the capture
// worker that reads and writes this runs as the logged-in user on both Windows
// (per-session) and macOS (Aqua LaunchAgent), while dataDir() is created
// root/SYSTEM-owned by the daemon. Writing there fails with "permission denied"
// and the remembered decision is silently lost — verified on macOS, where
// /Library/Application Support/NeevRemote is root-owned. It also scopes the
// decision to the user who actually made it, which is the correct blast radius
// for a security choice.

// consentDecision is what the host user chose for a device: whether to admit it
// at all, and at what access level. Stored as an object so a remembered
// "view only" grant stays view-only on every later connection — remembering the
// admission but silently upgrading it to full control would be a security bug.
type consentDecision struct {
	Allow   bool `json:"allow"`
	Control bool `json:"control"`
}

var (
	consentMu    sync.Mutex
	consentCache map[string]consentDecision
)

func consentStorePath() string {
	return filepath.Join(userDataDir(), "consent-decisions.json")
}

// normConsentID strips the internal "ctrl-" prefix and keeps only digits, so a
// decision remembered for a viewer matches on the id the user actually sees.
func normConsentID(id string) string {
	id = strings.TrimPrefix(id, "ctrl-")
	var b strings.Builder
	for _, r := range id {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return id
	}
	return b.String()
}

func loadConsentDecisions() map[string]consentDecision {
	consentMu.Lock()
	defer consentMu.Unlock()
	if consentCache != nil {
		return consentCache
	}
	consentCache = map[string]consentDecision{}
	data, err := os.ReadFile(consentStorePath())
	if err != nil {
		return consentCache // no file yet — nothing remembered
	}
	// Decode leniently, because an older build wrote a DIFFERENT shape here.
	//
	// Before access levels existed this file was {"id": true} — a bare bool. The
	// struct decode fails on that whole-file, so every remembered decision was
	// thrown away on upgrade: a host that chose "remember Accept" got prompted
	// again on every single connection, with only a warning in a log to say why.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		// Genuinely corrupt: start clean rather than wedge every future
		// connection behind a parse error.
		log.Warn().Err(err).Msg("worker: consent decisions file unreadable — ignoring it")
		return consentCache
	}
	migrated := 0
	for id, entry := range raw {
		var d consentDecision
		if err := json.Unmarshal(entry, &d); err == nil {
			consentCache[id] = d
			continue
		}
		var legacy bool
		if err := json.Unmarshal(entry, &legacy); err == nil {
			// A remembered decision from before access levels. Grant control:
			// that is what it meant at the time it was made, and quietly
			// downgrading someone's remembered Accept to view-only would look
			// like the product forgetting a setting.
			consentCache[id] = consentDecision{Allow: legacy, Control: true}
			migrated++
			continue
		}
		log.Warn().Str("device", id).Msg("worker: skipping an unreadable remembered decision")
	}
	if migrated > 0 {
		log.Info().Int("devices", migrated).
			Msg("worker: migrated remembered consent decisions from the older format")
	}
	return consentCache
}

// rememberedConsent reports a previously remembered decision for a device.
// ok is false when the user has never chosen "Remember this decision" for it.
func rememberedConsent(viewerID string) (d consentDecision, ok bool) {
	m := loadConsentDecisions()
	consentMu.Lock()
	defer consentMu.Unlock()
	d, ok = m[normConsentID(viewerID)]
	return
}

// saveConsentDecision persists the user's choice for this device, including the
// access level granted.
func saveConsentDecision(viewerID string, allow, control bool) {
	m := loadConsentDecisions()
	consentMu.Lock()
	m[normConsentID(viewerID)] = consentDecision{Allow: allow, Control: control}
	data, err := json.MarshalIndent(m, "", "  ")
	consentMu.Unlock()
	if err != nil {
		return
	}
	path := consentStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Warn().Err(err).Msg("worker: could not create the consent store directory")
		return
	}
	// Write via a temp file + rename so a crash mid-write can't leave a
	// truncated file that silently loses every remembered decision.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Warn().Err(err).Msg("worker: could not save the consent decision")
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		log.Warn().Err(err).Msg("worker: could not commit the consent decision")
		return
	}
	log.Info().Str("device", normConsentID(viewerID)).Bool("allow", allow).
		Bool("control", control).
		Msg("worker: remembered consent decision for this device")
}

// ForgetConsentDecisions clears every remembered decision. Exposed so a future
// Settings action ("forget remembered devices") has something to call — a
// remembered Decline is otherwise impossible to undo from the UI.
func ForgetConsentDecisions() {
	consentMu.Lock()
	consentCache = map[string]consentDecision{}
	consentMu.Unlock()
	_ = os.Remove(consentStorePath())
}
