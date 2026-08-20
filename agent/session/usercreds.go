package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Publishing the host's id + password to the app that has to display them.
//
// The transport keeps them in transport.txt, root-owned 0600 because the file
// carries the password. The Flutter app runs as the logged-in user, so it
// cannot read that file: on macOS the share card rendered an empty id while the
// daemon was registered and hosting normally, and the host had nothing to hand
// out. Relaxing transport.txt would have been a two-line fix and the wrong one —
// it would expose this machine's remote-access password to every other local
// account.
//
// The worker is the right place to land them because it already runs AS the
// logged-in user (CreateProcessAsUser on Windows, an Aqua LaunchAgent on
// macOS). A file it writes in that user's own directory is readable by exactly
// the person sitting at the keyboard, which is who the credentials are for.

// userCredsPath is the per-user location the app reads. It must be inside the
// user's own profile — a machine-wide directory would put the password back
// where every account can see it.
func userCredsPath() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Library", "Application Support", "NeevRemote")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		dir = filepath.Join(appData, "NeevRemote")
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config", "NeevRemote")
	}
	return filepath.Join(dir, "host-creds.json"), nil
}

// writeUserCreds stores the transport's announced credentials for the app.
//
// The payload is validated rather than written through: an empty id would
// replace a good file with one that makes the share card look broken, and the
// card showing nothing is the exact failure this exists to fix.
func writeUserCreds(payload []byte) error {
	var creds struct {
		ID       string `json:"id"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(payload, &creds); err != nil {
		return err
	}
	if creds.ID == "" {
		return nil
	}
	path, err := userCredsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	// 0600: the password is in here. On Windows the mode is advisory, but the
	// path is inside the user's own roaming profile, which is protected by its
	// ACL.
	return os.WriteFile(path, body, 0o600)
}
