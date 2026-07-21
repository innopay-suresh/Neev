//go:build !windows

package session

// Consent Accept/Deny dialog is Windows-only in TransportMode for now. Off
// Windows the flag file (consent.txt) is never written, so consentRequired() is
// false and this is never called; return true so an accidental call can't block.
func showConsentDialog(viewerID string) bool { return true }
