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

  /// True when the transport daemon is installed (its plist exists).
  static bool get isInstalled {
    if (!supported) return false;
    try {
      return File(_transportPlist).existsSync();
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
