package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// An older build wrote {"id": true} here. The struct decode failed on the whole
// file, so upgrading silently threw away every remembered decision and the host
// was prompted again on every connection — with only a log line to say why.

func withStore(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NEEV_DATA_DIR", dir)
	// userDataDir() is what the store actually uses; point HOME at the temp dir
	// so both resolve inside the test.
	t.Setenv("HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("APPDATA", dir)
	consentMu.Lock()
	consentCache = nil
	consentMu.Unlock()
	path := consentStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyBoolDecisionsSurviveUpgrade(t *testing.T) {
	withStore(t, `{"123456789": true, "987654321": false}`)

	allow, ok := rememberedConsent("ctrl-123456789")
	if !ok {
		t.Fatal("a remembered Accept from the older format was lost on upgrade")
	}
	if !allow.Allow {
		t.Error("remembered Accept decoded as a decline")
	}
	if !allow.Control {
		t.Error("a pre-access-level Accept meant full control; downgrading it " +
			"silently looks like the product forgetting a setting")
	}

	deny, ok := rememberedConsent("ctrl-987654321")
	if !ok {
		t.Fatal("a remembered Decline from the older format was lost")
	}
	if deny.Allow {
		t.Error("remembered Decline decoded as an accept — worse than losing it")
	}
}

func TestCurrentFormatStillLoads(t *testing.T) {
	withStore(t, `{"111222333": {"allow": true, "control": false}}`)
	d, ok := rememberedConsent("111222333")
	if !ok || !d.Allow {
		t.Fatal("current-format decision did not load")
	}
	if d.Control {
		t.Error("a remembered view-only grant must not become full control")
	}
}

func TestMixedFormatsBothLoad(t *testing.T) {
	// Exactly what a part-migrated file looks like after one new decision is
	// saved alongside older entries.
	withStore(t, `{"111": true, "222": {"allow": true, "control": false}}`)
	if d, ok := rememberedConsent("111"); !ok || !d.Allow || !d.Control {
		t.Error("legacy entry lost when a new-format entry was present")
	}
	if d, ok := rememberedConsent("222"); !ok || !d.Allow || d.Control {
		t.Error("new-format entry lost when a legacy entry was present")
	}
}

func TestGenuinelyCorruptFileDoesNotWedgeConnections(t *testing.T) {
	withStore(t, `this is not json at all`)
	if _, ok := rememberedConsent("123"); ok {
		t.Fatal("a corrupt file must yield no decisions, not invented ones")
	}
	// And must still be writable afterwards, so the next choice repairs it.
	saveConsentDecision("123", true, true)
	if d, ok := rememberedConsent("123"); !ok || !d.Allow {
		t.Fatal("could not record a new decision after a corrupt file")
	}
	var m map[string]consentDecision
	data, _ := os.ReadFile(consentStorePath())
	if json.Unmarshal(data, &m) != nil {
		t.Fatal("the rewritten store is not valid JSON")
	}
}
