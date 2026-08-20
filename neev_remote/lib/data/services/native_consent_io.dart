import 'dart:convert';
import 'dart:io';

/// The JXA the DAEMON already uses for this prompt, minus its access-level
/// accessory view (the app applies its own defaults).
///
/// The critical line is activateIgnoringOtherApps: a plain `display dialog`
/// from a backgrounded app opens BEHIND everything, which is exactly how the
/// consent prompt stayed invisible and made "ask before allowing" look broken.
/// NSAlert plus an explicit activation is what the daemon has always done, and
/// it works — so this reuses it rather than inventing a weaker second prompt.
const _consentAlertJS = r'''
function run(argv) {
  ObjC.import('AppKit');
  var app = $.NSApplication.sharedApplication;
  app.setActivationPolicy(1); // accessory — no Dock icon for a transient prompt
  var alert = $.NSAlert.alloc.init;
  alert.messageText = 'Connection Request';
  alert.informativeText =
    'A remote device is requesting to connect and control this computer.\n\n' +
    'Device ID:  ' + argv[0] + '\n\n' +
    'Only allow if you recognise this request.';
  alert.alertStyle = 2; // critical — this is a security decision
  alert.addButtonWithTitle('Allow');
  alert.addButtonWithTitle('Decline');
  app.activateIgnoringOtherApps(true);
  var button = alert.runModal;
  return JSON.stringify({ accept: button === 1000 });
}
''';

/// Shows a native, always-on-top Accept/Decline prompt for an incoming
/// connection.
///
/// Returns true for allow, false for decline, and NULL when this platform has
/// no native prompt — the caller then relies on the in-app dialog.
///
/// This exists because of how a host is actually used: the machine being
/// controlled has this app backgrounded or minimised, and the app cannot raise
/// its own window (window_manager is not built in). The in-window dialog
/// therefore rendered where nobody could see it, the request timed out, and
/// connecting to a Mac looked broken unless the user turned "ask before
/// allowing" off — trading the security prompt away to get a working product.
Future<bool?> showNativeConsent(String deviceId) async {
  if (!Platform.isMacOS) return null;
  final pretty = deviceId.replaceFirst('ctrl-', '');
  try {
    final res = await Process.run(
      'osascript',
      ['-l', 'JavaScript', '-e', _consentAlertJS, pretty],
    );
    final out = (res.stdout as String? ?? '').trim();
    if (out.isEmpty) return null; // prompt never ran — leave the in-app dialog
    final m = jsonDecode(out);
    return m is Map && m['accept'] == true;
  } catch (_) {
    // A broken prompt must not decide anything; the in-app dialog is still up.
    return null;
  }
}
