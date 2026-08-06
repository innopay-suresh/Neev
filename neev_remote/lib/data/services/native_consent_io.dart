import 'dart:io';

/// Shows a native, always-on-top Accept/Deny prompt for an incoming connection.
///
/// Returns true for allow, false for deny, and NULL when this platform has no
/// native prompt — the caller then relies on the in-app dialog.
///
/// This exists because of how a host is actually used. A machine being
/// controlled normally has the app in the background or minimised, and the app
/// cannot raise its own window (window_manager is not built in). On macOS the
/// in-window dialog therefore rendered where nobody could see it: the request
/// timed out, and connecting to a Mac looked broken unless the user turned
/// "ask before allowing" off — trading the security prompt away to make the
/// product work at all.
///
/// A native alert floats above every other window and needs no window of ours.
Future<bool?> showNativeConsent(String deviceId) async {
  if (!Platform.isMacOS) return null;
  final pretty = deviceId.replaceFirst('ctrl-', '');
  // Quote defensively: the id reaches us over the network, and an unescaped
  // quote would end the AppleScript string and change what is being asked.
  final safe = pretty.replaceAll('\\', '').replaceAll('"', '');
  try {
    final res = await Process.run('osascript', [
      '-e',
      'display dialog "Device $safe wants to control this Mac." '
          'with title "Neev Remote" '
          'buttons {"Deny", "Allow"} default button "Deny" '
          'with icon caution giving up after 45',
    ]);
    final out = (res.stdout as String? ?? '');
    // Deny, dismissed, and timed out all read as refusal: "ask before allowing"
    // with no answer means not allowed.
    return out.contains('button returned:Allow');
  } catch (_) {
    // A broken prompt must not decide anything; the in-app dialog is still up.
    return null;
  }
}
