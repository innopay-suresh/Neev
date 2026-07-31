//go:build darwin

package session

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// macOS consent prompt.
//
// The capture worker on macOS is a LaunchAgent with LimitLoadToSessionType=Aqua,
// so it already runs as the logged-in user inside their GUI session and may show
// UI. What it cannot do is drive AppKit directly: the agent has no
// NSApplication and no main-thread run loop (privacy_darwin.go runs its
// CFRunLoop on a private pthread), and this call arrives on an arbitrary
// goroutine — so [NSAlert runModal] in-process would be unsafe.
//
// Instead the alert is hosted by osascript, which IS a proper app with a run
// loop. JavaScript-for-Automation reaches AppKit through the ObjC bridge, which
// is what allows a real NSAlert with an accessory checkbox — "Remember this
// decision" — rather than the button-only dialog `display dialog` is limited to.
// That keeps the macOS prompt semantically identical to the Windows card,
// including the ability to remember a DECLINE.
//
// The device id is passed as an argument, never interpolated into the script,
// so nothing from the wire can be executed as code.
const consentAlertJS = `
function run(argv) {
  ObjC.import('AppKit');
  var app = $.NSApplication.sharedApplication;
  app.setActivationPolicy(1); // accessory — no Dock icon for a transient prompt
  var alert = $.NSAlert.alloc.init;
  alert.messageText = 'Connection Request';
  alert.informativeText =
    'A remote device is requesting to connect and control this computer.\n\n' +
    'Device ID:  ' + argv[0] + '\n\n' +
    'Only allow if you recognise this request. If you don\'t recognise this ' +
    'device, do not allow the connection.';
  alert.alertStyle = 2; // critical — this is a security decision
  alert.addButtonWithTitle('Accept');
  alert.addButtonWithTitle('Decline');
  var cb = $.NSButton.alloc.initWithFrame($.NSMakeRect(0, 0, 280, 20));
  cb.setButtonType(3); // NSButtonTypeSwitch
  cb.title = 'Remember this decision';
  alert.setAccessoryView(cb);
  app.activateIgnoringOtherApps(true);
  var button = alert.runModal;
  // 1000 = first button (Accept). Anything else is a refusal.
  return JSON.stringify({accept: button === 1000, remember: cb.state === 1});
}
`

// showConsentDialog shows the Accept/Decline prompt to the logged-in macOS user
// and reports their answer plus whether to remember it.
//
// Any failure returns false (deny). That is deliberate and matches the toggle's
// intent: at the login window there is no user who can consent, and "ask before
// allowing" with nobody there to accept means not allowed.
func showConsentDialog(viewerID string) (allow bool, remember bool) {
	// Bound the wait so a prompt nobody answers can't pin the transport's
	// consent waiter forever; the transport applies its own 30s timeout too.
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", consentAlertJS,
		prettyConsentID(viewerID))
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		log.Warn().Msg("worker: consent prompt timed out with no answer — denying")
		return false, false
	}
	if err != nil {
		log.Warn().Err(err).Msg("worker: could not show the consent prompt — denying")
		return false, false
	}
	var res struct {
		Accept   bool `json:"accept"`
		Remember bool `json:"remember"`
	}
	if e := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &res); e != nil {
		log.Warn().Err(e).Str("out", strings.TrimSpace(string(out))).
			Msg("worker: unreadable consent answer — denying")
		return false, false
	}
	return res.Accept, res.Remember
}

// prettyConsentID strips the internal "ctrl-" prefix and groups a 9-digit id as
// "XXX XXX XXX" so the prompt shows the id the user shares, not a raw token.
// (The Windows build has its own copy in consent_windows.go.)
func prettyConsentID(id string) string {
	id = strings.TrimPrefix(id, "ctrl-")
	digits := make([]rune, 0, len(id))
	for _, r := range id {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 9 {
		return string(digits[0:3]) + " " + string(digits[3:6]) + " " + string(digits[6:9])
	}
	return id
}
