import 'package:flutter/foundation.dart'
    show defaultTargetPlatform, kIsWeb, TargetPlatform;
import 'package:flutter/services.dart';

/// Listens for macOS session transitions (screen lock/unlock, fast-user-switch,
/// wake) pushed from the native [SessionWatcher] over `neev_remote/session`.
///
/// The host uses [onResume] to re-acquire its screen-capture stream: on macOS a
/// getDisplayMedia stream freezes when the Mac locks or the session is switched
/// away and never restarts itself, leaving the remote viewer stuck on the last
/// frame. Re-capturing on resume makes the video recover. No-op off macOS (the
/// native channel only exists there).
class SessionWatcher {
  SessionWatcher({this.onResume, this.onSuspend});

  /// Called when the session becomes usable again (unlock / switch back / wake).
  final void Function(String reason)? onResume;

  /// Called when the session is locked or switched away.
  final void Function(String reason)? onSuspend;

  static const MethodChannel _channel = MethodChannel('neev_remote/session');
  bool _started = false;

  static bool get supported =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.macOS;

  void start() {
    if (!supported || _started) return;
    _started = true;
    _channel.setMethodCallHandler((call) async {
      if (call.method != 'sessionEvent') return null;
      final args = call.arguments;
      if (args is! Map) return null;
      final event = args['event'] as String?;
      final reason = (args['reason'] as String?) ?? '';
      if (event == 'resume') {
        onResume?.call(reason);
      } else if (event == 'suspend') {
        onSuspend?.call(reason);
      }
      return null;
    });
  }

  void dispose() {
    if (_started) {
      _channel.setMethodCallHandler(null);
      _started = false;
    }
  }

  /// macOS: bring the app frontmost so a file picker opened for a remote import
  /// request is visible to the controlling viewer. No-op elsewhere.
  static Future<void> activateApp() async {
    if (!supported) return;
    try {
      await _channel.invokeMethod('activateApp');
    } catch (_) {}
  }
}
