import 'dart:io';

/// Where the capture worker reports that it cannot record the screen.
///
/// Same directory the app writes consent.txt to — the worker runs as the
/// logged-in user and cannot write the machine-wide (root-owned) one, so this
/// per-user path is the place both sides can reach.
String? _markerPath() {
  final home = Platform.environment['HOME'];
  if (home == null || home.isEmpty) return null;
  if (Platform.isMacOS) {
    return '$home/Library/Application Support/NeevRemote/capture-blocked';
  }
  return null; // only macOS needs this today
}

/// Whether screen capture is currently blocked on this machine.
///
/// Judged by FRESHNESS, not mere existence: a marker left behind by a crashed
/// worker would otherwise accuse the user of a permission problem forever. The
/// worker rewrites it every retry (~5s), so anything older than a minute is
/// stale.
Future<bool> isCaptureBlocked() async {
  final path = _markerPath();
  if (path == null) return false;
  try {
    final f = File(path);
    if (!await f.exists()) return false;
    final txt = (await f.readAsString()).trim();
    final secs = int.tryParse(txt);
    if (secs == null) return false;
    final age = DateTime.now().difference(
        DateTime.fromMillisecondsSinceEpoch(secs * 1000));
    return age.inSeconds < 60;
  } catch (_) {
    return false;
  }
}

/// The binary the user has to grant, so it can be shown and copied.
const String captureBinaryPath =
    '/Library/Application Support/NeevRemote/neev-agent';

/// Opens the exact Privacy pane. Without this the instruction is a path the
/// user has to find in a Settings app that hides that folder by default.
Future<void> openScreenRecordingSettings() async {
  if (!Platform.isMacOS) return;
  try {
    await Process.run('open', [
      'x-apple.systempreferences:com.apple.preference.security'
          '?Privacy_ScreenCapture',
    ]);
  } catch (_) {}
}
