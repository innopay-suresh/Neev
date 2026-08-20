//go:build darwin

package session

import (
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// Host file picker for export (host -> viewer).
//
// This did not exist. filedlg_other.go (//go:build !windows) returns
// ("", false), so macOS inherited a stub meant for unsupported platforms: the
// picker never opened and the worker logged "export picker closed/cancelled"
// in the same second the request arrived. Import worked, because that
// direction needs no dialog — which is exactly how it presented, as "import
// works, export doesn't".
//
// Same shape as the consent prompt: the worker is an Aqua LaunchAgent, so it
// may show UI, but it has no NSApplication and no main-thread run loop and
// cannot drive AppKit directly. osascript IS a real app with a run loop, and
// activateIgnoringOtherApps brings the panel in front of whatever the host is
// looking at — without it the dialog opens behind the current window and looks
// like nothing happened.
const exportPickerJS = `
function run() {
  ObjC.import('AppKit');
  var nsapp = $.NSApplication.sharedApplication;
  nsapp.setActivationPolicy(1); // accessory — no Dock icon for a transient panel
  nsapp.activateIgnoringOtherApps(true);
  var app = Application.currentApplication();
  app.includeStandardAdditions = true;
  // Cancelling throws; the caller treats a non-zero exit as "cancelled".
  var f = app.chooseFile({ withPrompt: 'Choose a file to send to the viewer' });
  return f.toString();
}
`

// showOpenFileDialog asks the host to pick one file. Returns ("", false) when
// the host cancels, which the caller reports as a cancellation rather than a
// failure.
func showOpenFileDialog() (string, bool) {
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", exportPickerJS).Output()
	if err != nil {
		// Cancel exits non-zero, so this is the normal path for "no thanks" as
		// well as for a genuine failure. Logged at debug volume either way; the
		// caller already reports the outcome to the viewer.
		log.Debug().Err(err).Msg("worker: export picker returned no file (cancelled or failed)")
		return "", false
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false
	}
	return path, true
}
