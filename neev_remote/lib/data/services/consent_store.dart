import 'dart:convert';
import 'dart:io';

import 'package:shared_preferences/shared_preferences.dart';

/// Remembered Accept/Decline decisions, keyed by the viewer's device id.
///
/// "Remember this decision" on the consent prompt writes here: a remembered
/// Accept auto-accepts that device on every later connection, a remembered
/// Decline auto-declines it, and neither shows the prompt again. The password
/// check is unaffected — this only skips the human Accept/Deny step.
///
/// The Go worker keeps its own copy of this list (agent/session/consentstore.go)
/// because the SYSTEM-service host never runs Flutter. The two stores are
/// deliberately separate: they belong to different host modes and a decision
/// made in one mode should not silently apply in the other.
class ConsentStore {
  static const _key = 'rememberedConsent';

  static Map<String, bool>? _cache;

  /// Strips the internal "ctrl-" prefix and keeps digits only, so a decision
  /// matches on the id the user actually sees on the prompt.
  static String normalizeId(String id) {
    final digits = id.replaceAll(RegExp(r'[^0-9]'), '');
    return digits.isEmpty ? id : digits;
  }

  static Future<Map<String, bool>> _load() async {
    if (_cache != null) return _cache!;
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_key);
    if (raw == null || raw.isEmpty) return _cache = {};
    try {
      final decoded = jsonDecode(raw);
      _cache = {
        for (final e in (decoded as Map).entries)
          e.key.toString(): e.value == true,
      };
    } catch (_) {
      // Corrupt entry must not wedge every future connection behind a parse
      // error; start clean and let the next decision rewrite it.
      _cache = {};
    }
    return _cache!;
  }

  /// The remembered decision for [deviceId], or null if there isn't one.
  static Future<bool?> decisionFor(String deviceId) async {
    final m = await _load();
    return m[normalizeId(deviceId)];
  }

  /// Persist the user's choice for [deviceId].
  static Future<void> remember(String deviceId, bool allow) async {
    final m = await _load();
    m[normalizeId(deviceId)] = allow;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key, jsonEncode(m));
  }

  /// Clear every remembered decision, in BOTH stores. A remembered Decline is
  /// otherwise impossible to undo from the UI — the device would just keep
  /// being refused with no prompt and no explanation.
  ///
  /// Also deletes the Go worker's `consent-decisions.json`. That file lives in
  /// the same user's data dir (agent/session/consentstore.go → userDataDir),
  /// so this app can reach it; clearing only the Flutter side would leave a
  /// TransportMode host still auto-answering from the worker's copy.
  static Future<void> forgetAll() async {
    _cache = {};
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_key);
    try {
      final f = File(_workerStorePath());
      if (await f.exists()) await f.delete();
    } catch (_) {
      // Best effort: the Flutter store is cleared either way.
    }
  }

  /// Mirrors userDataDir() in agent/session/datadir.go.
  static String _workerStorePath() {
    final env = Platform.environment;
    if (Platform.isWindows) {
      final base = env['LOCALAPPDATA'] ?? env['APPDATA'] ?? '';
      return '$base\\NeevRemote\\consent-decisions.json';
    }
    final home = env['HOME'] ?? '';
    if (Platform.isMacOS) {
      return '$home/Library/Application Support/NeevRemote/'
          'consent-decisions.json';
    }
    final xdg = env['XDG_DATA_HOME'];
    final base = (xdg == null || xdg.isEmpty) ? '$home/.local/share' : xdg;
    return '$base/NeevRemote/consent-decisions.json';
  }

  /// How many devices this app has a remembered decision for. Counts the
  /// Flutter store only — the worker's file is not read here, so treat a zero
  /// count as "none from this app", not "none anywhere".
  static Future<int> count() async => (await _load()).length;
}
