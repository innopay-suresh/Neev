package session

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// "Only while the app is open" used to behave exactly like "always": the
// transport could not observe the app, so it admitted everything while the UI
// promised requests would be ignored with the app closed. These pin the fixed
// behaviour.

func writeBeacon(t *testing.T, age time.Duration) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NEEV_DATA_DIR", dir)
	stamp := time.Now().Add(-age).Unix()
	if err := os.WriteFile(filepath.Join(dir, "app-open.txt"),
		[]byte(strconv.FormatInt(stamp, 10)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAppIsOpenWithFreshBeacon(t *testing.T) {
	writeBeacon(t, 2*time.Second)
	if !appIsOpen() {
		t.Fatal("a beacon written 2s ago should mean the app is open")
	}
}

func TestAppIsClosedWhenBeaconGoesStale(t *testing.T) {
	// The app crashed or was force-quit, so nothing deleted the file. Expiry is
	// what closes the door; a create/delete flag would leave it open forever.
	writeBeacon(t, 60*time.Second)
	if appIsOpen() {
		t.Fatal("a stale beacon must NOT keep 'only while the app is open' open")
	}
}

func TestAppIsClosedWithNoBeaconAtAll(t *testing.T) {
	t.Setenv("NEEV_DATA_DIR", t.TempDir())
	if appIsOpen() {
		t.Fatal("no beacon must mean the app is not running")
	}
}
