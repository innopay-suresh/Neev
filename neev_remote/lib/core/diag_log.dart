import 'dart:io';

import 'package:flutter/foundation.dart';

import 'constants/app_constants.dart';

/// Human-readable build stamp so a field log unambiguously identifies which
/// build produced it (the pubspec version is a static 1.0.0 and can't).
/// Defaults to the SAME visible [AppConstants.buildTag] shown in the UI top bar
/// — the build never passed a BUILD_STAMP dart-define, so the old hardcoded
/// default silently lied in every field log. Now the log and the on-screen
/// stamp always agree.
const String kBuildStamp = String.fromEnvironment(
  'BUILD_STAMP',
  defaultValue: AppConstants.buildTag,
);

/// Persistent, best-effort file logger for the parts of the app the native
/// helper log can't see: signaling, WebRTC connection state, host registration,
/// consent, and viewer auto-reconnect.
///
/// Why this exists: in release builds `debugPrint` goes nowhere, so field
/// failures in the Dart transport (e.g. "session doesn't resume after a user
/// switch") were undiagnosable. This writes next to the native logs
/// (`C:\ProgramData\NeevRemote\app.log` on Windows — user-independent, and the
/// service-host runs as SYSTEM which can write there) so host + viewer machines
/// each produce one shareable log.
class DiagLog {
  static IOSink? _sink;
  static String? _path;
  static bool _tried = false;

  static String get path => _path ?? '(uninitialized)';

  /// Resolve the log path and write a startup banner. Safe to call more than
  /// once; only the first call opens the file.
  static void init() {
    if (_tried) return;
    _tried = true;
    if (kIsWeb) return;
    try {
      final dir = _logDir();
      Directory(dir).createSync(recursive: true);
      final file = File('$dir${Platform.pathSeparator}app.log');
      // Roll the file if it grows past ~2 MB so it can't fill the disk.
      try {
        if (file.existsSync() && file.lengthSync() > 2 * 1024 * 1024) {
          file.writeAsStringSync('');
        }
      } catch (_) {}
      _sink = file.openWrite(mode: FileMode.append);
      _path = file.path;
      log('boot', 'app start — build=$kBuildStamp os=${Platform.operatingSystem} '
          'pid=$pid');
    } catch (_) {
      _sink = null; // logging is best-effort; never break the app
    }
  }

  static String _logDir() {
    if (Platform.isWindows) {
      final pd = Platform.environment['ProgramData'];
      if (pd != null && pd.isNotEmpty) return '$pd\\NeevRemote';
      return 'C:\\ProgramData\\NeevRemote';
    }
    // macOS/Linux: a stable, writable spot that survives restarts.
    final home = Platform.environment['HOME'] ?? '.';
    return '$home/.neev_remote';
  }

  /// Append one timestamped line. `tag` groups related events (e.g. 'host',
  /// 'viewer', 'reconnect'). Never throws.
  static void log(String tag, String message) {
    if (kIsWeb) return;
    if (!_tried) init();
    final line = '[${DateTime.now().toIso8601String()}] $tag: $message';
    try {
      _sink?.writeln(line);
    } catch (_) {}
    if (kDebugMode) debugPrint('[diag] $line');
  }
}
