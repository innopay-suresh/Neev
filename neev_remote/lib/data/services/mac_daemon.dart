import 'dart:io';

import 'package:flutter/foundation.dart';

/// Installs / removes the macOS switch-user + lock-screen daemon set
/// (com.neev.transport root LaunchDaemon + com.neev.worker LaunchAgent) that the
/// build bundles into the app at Contents/Resources/daemon.
///
/// This is the macOS analog of the Windows "install service" flow: once the
/// daemon is running it owns hosting for the machine (see [HostMode]), survives
/// fast-user-switch, and — with Screen Recording TCC granted — lets a viewer see
/// the login/lock window. Installing requires admin rights, so we run the bundled
/// install script through `osascript … with administrator privileges`, which
/// shows the standard macOS auth prompt (no password ever passes through Dart).
class MacDaemon {
  static bool get supported =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.macOS;

  static const String _transportPlist =
      '/Library/LaunchDaemons/com.neev.transport.plist';
  static const String _workerPlist =
      '/Library/LaunchAgents/com.neev.worker.plist';

  /// Where install-daemon.sh puts the agent the LaunchDaemon executes.
  static const String _installedAgent =
      '/Library/Application Support/NeevRemote/neev-agent';

  /// True when the transport daemon is installed AND can actually run.
  ///
  /// This used to test only that the plist existed. A machine was found with
  /// both plists in place and the agent binary gone from
  /// /Library/Application Support/NeevRemote — launchd stuck on
  /// `last exit code = 78: EX_CONFIG`, `state = spawn scheduled`, retrying a
  /// program that wasn't there. The plist check called that "installed", so the
  /// app skipped the repair, hosted the session itself, and left the user with
  /// Record and Sound greyed out and no idea why.
  ///
  /// This is the failure LD-MAC-HOST-1 already warns about: never answer "is
  /// the service there?" with a signal that can be true while it isn't. A
  /// LaunchDaemon is only installed if the thing it launches exists.
  static bool get isInstalled {
    if (!supported) return false;
    try {
      return File(_transportPlist).existsSync() &&
          File(_installedAgent).existsSync();
    } catch (_) {
      return false;
    }
  }

  /// True when the daemon is half-installed: plists present, agent missing.
  /// Worth distinguishing because it must override a previous "don't ask me
  /// again" — the user declining a FIRST install is not consent to stay broken.
  static bool get isBroken {
    if (!supported) return false;
    try {
      return File(_transportPlist).existsSync() &&
          !File(_installedAgent).existsSync();
    } catch (_) {
      return false;
    }
  }

  /// Path to the bundled daemon payload (…/Neev Remote.app/Contents/Resources/
  /// daemon) derived from the running executable, or null if not bundled (e.g. a
  /// dev build that didn't include the Go agent).
  static String? _payloadDir() {
    try {
      // resolvedExecutable = …/Contents/MacOS/neev_remote
      final macosDir = File(Platform.resolvedExecutable).parent; // Contents/MacOS
      final contents = macosDir.parent; // Contents
      final dir = Directory('${contents.path}/Resources/daemon');
      final agent = File('${dir.path}/neev-agent');
      return agent.existsSync() ? dir.path : null;
    } catch (_) {
      return null;
    }
  }

  /// Whether this build actually shipped the daemon payload.
  static bool get canInstall => supported && _payloadDir() != null;

  /// Installs + loads the daemon set. Returns null on success, else an error
  /// string. Shows the macOS admin auth prompt. [relayUrl] overrides the baked
  /// default in install-daemon.sh.
  static Future<String?> install({String? relayUrl}) async {
    if (!supported) return 'not macOS';
    final dir = _payloadDir();
    if (dir == null) return 'daemon payload missing from app bundle';
    final script = '$dir/install-daemon.sh';
    final agent = '$dir/neev-agent';
    // Build the privileged shell command. Quote the paths (the app lives under
    // "…/Neev Remote.app" — a space-bearing path).
    final relayArg = (relayUrl != null && relayUrl.isNotEmpty) ? ' "$relayUrl"' : '';
    final shell = 'bash "$script" "$agent"$relayArg';
    return _runPrivileged(shell);
  }

  /// Marker recording that the user dismissed the first-launch install prompt,
  /// so [ensureInstalled] asks once per app version instead of every launch.
  static File? _declinedMarker() {
    final home = Platform.environment['HOME'];
    if (home == null || home.isEmpty) return null;
    return File('$home/Library/Application Support/NeevRemote/'
        'daemon-install-declined');
  }

  /// Installs the daemon at app launch if it isn't there yet.
  ///
  /// The .pkg does this in its postinstall with no prompt at all, but the .dmg
  /// cannot: dragging an app to /Applications runs no scripts, so a .dmg user
  /// used to end up with a viewer-only install and no TransportMode — no
  /// hosting across a user switch, no login-window access, and none of the
  /// host-side session controls. Writing /Library/LaunchDaemons needs root, so
  /// this shows the standard macOS auth prompt once; after that the daemon is
  /// installed for good and no later launch asks again.
  ///
  /// Also acts as the safety net for a .pkg whose postinstall didn't run.
  /// Returns null when nothing was needed or the install succeeded.
  static Future<String?> ensureInstalled({String? relayUrl}) async {
    if (!canInstall || isInstalled) return null;
    final marker = _declinedMarker();
    final stamp = '${Platform.resolvedExecutable}\n$_installPromptVersion';
    try {
      // A BROKEN install is repaired regardless of the marker: declining a
      // first-time install is not a decision to leave a stuck daemon in place.
      if (!isBroken &&
          marker != null &&
          marker.existsSync() &&
          marker.readAsStringSync() == stamp) {
        return 'declined';
      }
    } catch (_) {
      // An unreadable marker must not block the install.
    }
    final err = await install(relayUrl: relayUrl);
    if (err == 'cancelled') {
      try {
        marker?.parent.createSync(recursive: true);
        marker?.writeAsStringSync(stamp);
      } catch (_) {
        // Worst case we ask again next launch — better than not installing.
      }
    }
    return err;
  }

  /// Bump to re-ask users who previously cancelled (e.g. when the install
  /// itself gains a fix that makes it worth prompting for again).
  static const int _installPromptVersion = 1;

  /// Stops + removes the daemon set (admin prompt).
  static Future<String?> uninstall() async {
    if (!supported) return 'not macOS';
    const shell = 'launchctl bootout system "$_transportPlist" 2>/dev/null; '
        'rm -f "$_transportPlist" "$_workerPlist"; '
        'rm -rf "/Library/Application Support/NeevRemote"';
    return _runPrivileged(shell);
  }

  static Future<String?> _runPrivileged(String shell) async {
    // Escape for the AppleScript string literal: backslashes then double-quotes.
    final esc = shell.replaceAll(r'\', r'\\').replaceAll('"', r'\"');
    final osa = 'do shell script "$esc" with administrator privileges';
    try {
      final res = await Process.run('osascript', ['-e', osa]);
      if (res.exitCode == 0) return null;
      final err = (res.stderr as String).trim();
      // User cancelled the auth prompt → -128; report cleanly.
      if (err.contains('-128')) return 'cancelled';
      return err.isEmpty ? 'exit ${res.exitCode}' : err;
    } catch (e) {
      return '$e';
    }
  }
}
