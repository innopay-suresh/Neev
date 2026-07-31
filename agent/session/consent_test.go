package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrettyConsentID(t *testing.T) {
	cases := map[string]string{
		"ctrl-926941775": "926 941 775", // the wire form the transport passes
		"926941775":      "926 941 775",
		"926-941-775":    "926 941 775",
		"short":          "short", // not 9 digits — passed through untouched
	}
	for in, want := range cases {
		if got := prettyConsentID(in); got != want {
			t.Errorf("prettyConsentID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormConsentIDMatchesAcrossForms(t *testing.T) {
	// A decision remembered from the prompt must match the same device when it
	// reconnects, whatever form the id arrives in.
	forms := []string{"ctrl-926941775", "926941775", "926 941 775", "926-941-775"}
	want := normConsentID(forms[0])
	for _, f := range forms {
		if got := normConsentID(f); got != want {
			t.Errorf("normConsentID(%q) = %q, want %q", f, got, want)
		}
	}
}

func TestConsentDecisionRoundTrip(t *testing.T) {
	// Isolate the store from the real per-user file. The store lives in
	// userDataDir(), which is HOME-derived on macOS/Linux and LOCALAPPDATA on
	// Windows, so point all of those at a temp dir.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	consentMu.Lock()
	consentCache = nil
	consentMu.Unlock()
	t.Cleanup(func() {
		consentMu.Lock()
		consentCache = nil
		consentMu.Unlock()
	})

	if _, ok := rememberedConsent("ctrl-111222333"); ok {
		t.Fatal("nothing should be remembered before a decision is saved")
	}

	// A remembered DECLINE must persist, not just an accept.
	saveConsentDecision("ctrl-111222333", false)
	consentMu.Lock()
	consentCache = nil // force a re-read from disk
	consentMu.Unlock()

	allow, ok := rememberedConsent("111222333") // different form, same device
	if !ok {
		t.Fatal("the decision should have been remembered")
	}
	if allow {
		t.Error("a remembered Decline must stay a decline")
	}
}

func TestConsentFlagPathsIncludesDataDir(t *testing.T) {
	paths := consentFlagPaths()
	if len(paths) == 0 {
		t.Fatal("expected at least the machine-wide flag path")
	}
	want := filepath.Join(dataDir(), "consent.txt")
	if paths[0] != want {
		t.Errorf("first path = %q, want the machine-wide %q", paths[0], want)
	}
	// The system-wide file must be checked FIRST so an admin/MDM can force the
	// setting for every account.
	if _, err := os.Stat(filepath.Dir(want)); err != nil && !os.IsNotExist(err) {
		t.Errorf("unexpected error stating the data dir: %v", err)
	}
}
