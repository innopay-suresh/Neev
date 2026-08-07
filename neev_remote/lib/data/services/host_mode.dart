import 'dart:convert' show jsonDecode;
import 'dart:io' show File, Platform;

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// The machine identity a daemon-hosted Mac is reachable under.
class DaemonCreds {
  const DaemonCreds(this.id, this.password);
  final String id;
  final String password;
}

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

  /// Heartbeat the transport refreshes while REGISTERED, whether or not anyone
  /// is connected.
  static const String _macAliveFile =
      '/Library/Application Support/NeevRemote/transport.alive';

  /// Generous against the transport's ~10s heartbeat, so a slow write does not
  /// briefly hand hosting back and register a second identity for this machine.
  static const Duration _macAliveMaxAge = Duration(seconds: 45);

  /// The id + password the macOS daemon registered this machine under.
  ///
  /// The app cannot read the daemon's own transport.txt — it is root-owned 0600
  /// because it carries the password, and the app runs as the logged-in user.
  /// The result was a share card showing an empty id on a machine that was
  /// registered and hosting perfectly well, so the host had nothing to give a
  /// viewer. Windows never had this problem: its app asks the SYSTEM helper
  /// directly.
  ///
  /// Two sources, in order:
  ///  - host-creds.json, written by the WORKER (which runs as this user) from
  ///    the credentials the transport announces over IPC. Has the password.
  ///  - transport.id, written world-readable by the transport. Id only — enough
  ///    to name the machine before a worker exists, and the id is not a secret.
  static DaemonCreds? macDaemonCreds() {
    if (kIsWeb || !Platform.isMacOS) return null;
    try {
      final f = File('${Platform.environment['HOME'] ?? ''}'
          '/Library/Application Support/NeevRemote/host-creds.json');
      if (f.existsSync()) {
        final m = jsonDecode(f.readAsStringSync()) as Map<String, dynamic>;
        final id = (m['id'] as String?) ?? '';
        if (id.isNotEmpty) {
          return DaemonCreds(id, (m['password'] as String?) ?? '');
        }
      }
    } catch (_) {
      // Fall through to the id-only file rather than showing nothing.
    }
    try {
      final f = File('/Library/Application Support/NeevRemote/transport.id');
      if (f.existsSync()) {
        final id = f.readAsStringSync().trim();
        if (id.isNotEmpty) return DaemonCreds(id, '');
      }
    } catch (_) {}
    return null;
  }

  static bool _macDaemonHosting() {
    if (defaultTargetPlatform != TargetPlatform.macOS) return false;
    try {
      if (!File(_macTransportPlist).existsSync()) return false;
      // ALIVE, not READY.
      //
      // transport.ready is only written while frames are FLOWING, which cannot
      // happen until a viewer connects. Deciding on it meant that with no
      // session the daemon always looked idle, so the app registered its own id
      // as a second host for this machine — and a viewer reaching that id got an
      // app-hosted session with no recording and no system sound. The question
      // "does the service own hosting?" has to be answerable before any session
      // exists, so it is answered by the transport's heartbeat instead.
      final alive = File(_macAliveFile);
      if (alive.existsSync()) {
        final age = DateTime.now().difference(alive.lastModifiedSync());
        if (age <= _macAliveMaxAge) return true;
      }
      // Fall back to the old signal, so a transport too old to write a
      // heartbeat is still detected while it is actively streaming.
      final ready = File(_macReadyFile);
      if (!ready.existsSync()) return false;
      final age = DateTime.now().difference(ready.lastModifiedSync());
      return age <= _macReadyMaxAge;
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
