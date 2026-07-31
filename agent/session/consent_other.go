//go:build !windows && !darwin

package session

// Consent Accept/Deny prompts exist on Windows (consent_windows.go) and macOS
// (consent_darwin.go). On any other platform there is no host UI to show one,
// so deny rather than accept: "ask before allowing connections" with nobody
// able to answer means NOT allowed. This used to return true, which would have
// silently auto-accepted every connection the moment the flag was readable.
func showConsentDialog(viewerID string) (allow, control, remember bool) {
	return false, false, false
}
