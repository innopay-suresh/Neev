import 'dart:convert';

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

  /// Clear every remembered decision. A remembered Decline is otherwise
  /// impossible to undo from the UI, so this needs to stay reachable.
  static Future<void> forgetAll() async {
    _cache = {};
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_key);
  }

  /// How many devices currently have a remembered decision (for Settings).
  static Future<int> count() async => (await _load()).length;
}
