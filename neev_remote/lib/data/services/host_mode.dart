import 'dart:io' show File;

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

  /// macOS: the root transport daemon (com.neev.transport) owns hosting only when
  /// it is genuinely HOSTING — installed AND actively producing video. Merely
  /// installing it is NOT enough: without Screen Recording permission its capture
  /// worker crash-loops and produces nothing, so deferring to it would strand the
  /// app "Offline" with no video anywhere (the regression this guard prevents).
  ///
  /// The daemon writes a `transport.ready` file every ~2s WHILE producing
  /// frames; we treat the daemon as owning hosting only if that heartbeat is
  /// fresh. The app is un-sandboxed so it can stat /Library directly (no native
  /// code). When the daemon is truly hosting, the Flutter app stays viewer/
  /// control-only like Windows TransportMode.
  static const String _macTransportPlist =
      '/Library/LaunchDaemons/com.neev.transport.plist';
  static const String _macReadyFile =
      '/Library/Application Support/NeevRemote/transport.ready';
  static const Duration _macReadyMaxAge = Duration(seconds: 15);

  static bool _macDaemonHosting() {
    if (defaultTargetPlatform != TargetPlatform.macOS) return false;
    try {
      if (!File(_macTransportPlist).existsSync()) return false;
      final ready = File(_macReadyFile);
      if (!ready.existsSync()) return false;
      final age = DateTime.now().difference(ready.lastModifiedSync());
      return age <= _macReadyMaxAge; // fresh ⇒ worker is really producing video
    } catch (_) {
      return false;
    }
  }

  /// True if this instance should host. Non-Windows always hosts (unchanged),
  /// EXCEPT macOS when the transport daemon is actively hosting (producing video).
  static Future<bool> shouldAutoHost() async {
    if (kIsWeb) return true;
    if (defaultTargetPlatform == TargetPlatform.macOS) {
      return !_macDaemonHosting();
    }
    if (defaultTargetPlatform != TargetPlatform.windows) return true;
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

  /// True when the SYSTEM service transport owns hosting for this machine
  /// (TransportMode). In that mode the Flutter app must NEVER register as a
  /// second connectable host by ANY path — the service transport is the single
  /// host identity. Guards every startHosting entry point, not just auto-host.
  static Future<bool> serviceOwnsHosting() async {
    if (kIsWeb) return false;
    if (defaultTargetPlatform == TargetPlatform.macOS) {
      return _macDaemonHosting();
    }
    if (defaultTargetPlatform != TargetPlatform.windows) return false;
    try {
      final m = await _channel.invokeMethod<Map>('query');
      return m != null && m['transportMode'] == true;
    } catch (_) {
      return false; // channel absent → legacy Flutter-host mode
    }
  }
}
