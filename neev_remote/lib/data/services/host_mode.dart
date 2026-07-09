import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Decides whether THIS app instance should auto-start hosting.
///
/// With ServiceHost mode on, the SYSTEM service already runs a host that follows
/// the active session. A second, manually-opened host would compete for the same
/// machine-id and, on a user switch, get stranded in the old session (the
/// "app closed, doesn't return" symptom). So a manually-opened window becomes
/// viewer/control-only in that case; only the service-launched instance hosts.
class HostMode {
  static const MethodChannel _channel = MethodChannel('neev_remote/hostmode');

  /// True if this instance should host. Non-Windows always hosts (unchanged).
  static Future<bool> shouldAutoHost() async {
    if (kIsWeb || defaultTargetPlatform != TargetPlatform.windows) return true;
    try {
      final m = await _channel.invokeMethod<Map>('query');
      if (m == null) return true;
      final serviceInstance = m['serviceInstance'] == true;
      final serviceHostMode = m['serviceHostMode'] == true;
      final transportMode = m['transportMode'] == true;
      // Seamless mode: the Go transport (session 0) owns the machine-id, so a
      // Flutter window must never host — it would double-register and fight the
      // transport. Stay viewer/control-only regardless of instance.
      if (transportMode) return false;
      // Host only if we ARE the service instance, or the service isn't hosting.
      return serviceInstance || !serviceHostMode;
    } catch (_) {
      return true; // channel absent → default to hosting
    }
  }
}
