import 'package:flutter/foundation.dart'
    show defaultTargetPlatform, kIsWeb, TargetPlatform;
import 'package:flutter/services.dart';

/// Host-side privacy mode: blanks the physical screen (still visible to the
/// remote viewer) and blocks local input. Windows + macOS (native); no-op else.
class PrivacyMode {
  static const MethodChannel _channel = MethodChannel('neev_remote/privacy');

  static bool get supported =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.macOS);

  /// Whether privacy is currently engaged on THIS machine.
  ///
  /// Tracked because privacy is session state that must be torn down when a
  /// session ends: without knowing it is on, the host cannot reliably clear it,
  /// and a disconnect while blanked locks the user out of their own screen.
  static bool _on = false;
  static bool get isOn => _on;

  static Future<void> set(bool on) async {
    if (!supported) return;
    try {
      await _channel.invokeMethod('setPrivacy', on);
      _on = on;
    } catch (_) {
      // Failing to ENABLE leaves it off, which is safe. Failing to DISABLE is
      // not: assume it may still be engaged so later teardown attempts retry.
      if (!on) _on = true;
    }
  }
}
