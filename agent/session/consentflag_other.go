//go:build !darwin

package session

import "path/filepath"

// consentFlagPaths lists where the "Ask before allowing connections" flag may
// live. On Windows the Flutter app and the service share
// %ProgramData%\NeevRemote, so the single machine-wide file is enough.
func consentFlagPaths() []string {
	return []string{filepath.Join(dataDir(), "consent.txt")}
}
