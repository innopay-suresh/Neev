//go:build darwin

package session

import "encoding/json"

// handleCommand consumes viewer session commands on a macOS host. Only "privacy"
// is implemented here — the daemon owns hosting on macOS, so the Flutter app's
// PrivacyMode never runs and the command was previously dropped entirely (host
// screen simply never blanked). Every other command returns false so it keeps the
// exact behavior it had before (falls through to chat / input handling).
func handleCommand(payload []byte) bool {
	var m struct {
		K  string `json:"k"`
		C  string `json:"c"`
		On bool   `json:"on"`
	}
	if err := json.Unmarshal(payload, &m); err != nil || m.K != "cmd" {
		return false
	}
	if m.C != "privacy" {
		return false // unchanged: lock/logoff/reboot/sas not handled on macOS
	}
	setPrivacy(m.On)
	return true
}
