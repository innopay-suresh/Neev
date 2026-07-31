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
  alert.addButtonWithTitle('Allow');
  alert.addButtonWithTitle('Decline');

  // Accessory: the ACCESS LEVEL the host is granting, plus the remember box.
  // The host decides here — a viewer cannot escalate itself to control.
  var box = $.NSView.alloc.initWithFrame($.NSMakeRect(0, 0, 300, 66));
  var viewOnly = $.NSButton.alloc.initWithFrame($.NSMakeRect(0, 44, 300, 20));
  viewOnly.setButtonType(4); // NSButtonTypeRadio
  viewOnly.title = 'View only — they can see the screen';
  var fullCtl = $.NSButton.alloc.initWithFrame($.NSMakeRect(0, 24, 300, 20));
  fullCtl.setButtonType(4);
  fullCtl.title = 'Full control — they can control this Mac';
  // argv[1] is "1" when the host's own View-only setting is on: preselect it so
  // the prompt defaults to the host's stated wish.
  if (argv[1] === '1') { viewOnly.setState(1); } else { fullCtl.setState(1); }
  var cb = $.NSButton.alloc.initWithFrame($.NSMakeRect(0, 0, 300, 20));
  cb.setButtonType(3); // NSButtonTypeSwitch
  cb.title = 'Remember this decision';
  box.addSubview(viewOnly);
  box.addSubview(fullCtl);
  box.addSubview(cb);
  alert.setAccessoryView(box);
  app.activateIgnoringOtherApps(true);
  var button = alert.runModal;
  // 1000 = first button (Allow). Anything else is a refusal.
  // NSButton.state comes back through the ObjC bridge as a wrapper object, NOT
  // a JS number: 'state === 1' is ALWAYS false. Coerce with Number() or every
  // checkbox silently reads as unticked (which is exactly what happened to
  // "Remember this decision" before this was caught).
  return JSON.stringify({
    accept: button === 1000,
    control: Number(fullCtl.state) === 1,
    remember: Number(cb.state) === 1
  });
}
`

// showConsentDialog shows the Accept/Decline prompt to the logged-in macOS user
// and reports their answer plus whether to remember it.
//
// Any failure returns false (deny). That is deliberate and matches the toggle's
// intent: at the login window there is no user who can consent, and "ask before
// allowing" with nobody there to accept means not allowed.
func showConsentDialog(viewerID string) (allow bool, control bool, remember bool) {
	// Bound the wait so a prompt nobody answers can't pin the transport's
	// consent waiter forever; the transport applies its own 30s timeout too.
	viewOnlyDefault := "0"
	if hostViewOnlyDefault() {
		viewOnlyDefault = "1"
	}
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", consentAlertJS,
		prettyConsentID(viewerID), viewOnlyDefault)
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
		return false, false, false
	}
	if err != nil {
		log.Warn().Err(err).Msg("worker: could not show the consent prompt — denying")
		return false, false, false
	}
	var res struct {
		Accept   bool `json:"accept"`
		Control  bool `json:"control"`
		Remember bool `json:"remember"`
	}
	if e := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &res); e != nil {
		log.Warn().Err(e).Str("out", strings.TrimSpace(string(out))).
			Msg("worker: unreadable consent answer — denying")
		return false, false, false
	}
	return res.Accept, res.Control, res.Remember
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
