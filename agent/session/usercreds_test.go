package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The app reads this file to render the share card, so its shape and its
// permissions both matter: the wrong shape shows a blank id on a machine that
// is hosting fine, and the wrong mode hands this machine's remote-access
// password to every other local account.

func TestWriteUserCredsRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	if err := writeUserCreds([]byte(`{"id":"399-559-302","password":"MhUynvg8"}`)); err != nil {
		t.Fatalf("writeUserCreds: %v", err)
	}

	path, err := userCredsPath()
	if err != nil {
		t.Fatalf("userCredsPath: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got struct {
		ID       string `json:"id"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "399-559-302" || got.Password != "MhUynvg8" {
		t.Fatalf("round trip lost data: %+v", got)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows modes are advisory; the guarantee there is the profile ACL.
	if perm := st.Mode().Perm(); perm != 0o600 && os.Getenv("OS") != "Windows_NT" {
		t.Fatalf("creds file is %v, want 0600 — the password is in it", perm)
	}
}

// An empty id must not overwrite a good file. Rendering an empty share card is
// the exact failure this whole path exists to fix, so a malformed announce has
// to leave the last known-good credentials in place.
func TestWriteUserCredsKeepsGoodFileOnEmptyID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	if err := writeUserCreds([]byte(`{"id":"399-559-302","password":"pw"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := writeUserCreds([]byte(`{"id":"","password":""}`)); err != nil {
		t.Fatalf("empty announce should be ignored, not fail: %v", err)
	}

	path, _ := userCredsPath()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &got)
	if got.ID != "399-559-302" {
		t.Fatalf("empty announce clobbered the id: %q", got.ID)
	}
}

func TestWriteUserCredsRejectsGarbage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := writeUserCreds([]byte("not json")); err == nil {
		t.Fatal("expected an error for a non-JSON payload")
	}
}
