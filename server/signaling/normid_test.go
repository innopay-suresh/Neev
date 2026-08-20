package signaling

import "testing"

// The relay mints ids as NNN-NNN-NNN and the desktop app strips every
// non-alphanumeric character before dialing, so the two forms MUST land on the
// same registry key. When they did not, a daemon-hosted Mac showed as Online in
// the device list and answered "agent not found or offline" to every connect.
func TestNormAgentIDMatchesDialedForm(t *testing.T) {
	cases := []struct{ registered, dialed string }{
		{"106-198-026", "106198026"}, // relay-minted id vs what the app sends
		{"399-559-302", "399559302"},
		{"846405601", "846405601"}, // app-hosted id: already bare, unchanged
		{"595 632 641", "595632641"}, // as displayed on the share card
	}
	for _, c := range cases {
		if got, want := normAgentID(c.registered), normAgentID(c.dialed); got != want {
			t.Errorf("registered %q -> %q, dialed %q -> %q: keys must match",
				c.registered, got, c.dialed, want)
		}
	}
}

// Normalizing must not merge two genuinely different machines.
func TestNormAgentIDKeepsDistinctIDsDistinct(t *testing.T) {
	if normAgentID("106-198-026") == normAgentID("399-559-302") {
		t.Fatal("different ids collapsed to the same key")
	}
}

// Bare ids are the overwhelming majority in the field (every app-hosted host),
// so this must be exactly the identity function for them — a change there would
// regress Windows-to-Windows, which never used a dashed id.
func TestNormAgentIDIsIdentityForBareDigits(t *testing.T) {
	for _, id := range []string{"846405601", "595632641", "0", ""} {
		if got := normAgentID(id); got != id {
			t.Errorf("normAgentID(%q) = %q, want it unchanged", id, got)
		}
	}
}
