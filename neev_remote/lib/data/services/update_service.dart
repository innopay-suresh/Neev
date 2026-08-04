import 'dart:convert';

import 'package:http/http.dart' as http;

import '../../core/constants/app_constants.dart';
import '../../core/diag_log.dart';

/// Checks whether a newer build has been published.
///
/// This exists because "which build am I actually running?" cost this project
/// real time: stale installers, browser-cached .exe downloads, and bug reports
/// against code that had already been fixed. The app now tells the user when it
/// is behind, and shows its own build stamp so the question is answerable.
///
/// Deliberately does NOT download or run anything. Silently fetching and
/// executing an installer is a serious security surface — it needs signature
/// verification at minimum — so this reports and hands over a URL. The user
/// stays in control of what runs on their machine.
class UpdateInfo {
  const UpdateInfo({
    required this.available,
    required this.latestBuild,
    required this.currentBuild,
    this.downloadUrl,
  });

  final bool available;
  final String latestBuild;
  final String currentBuild;
  final String? downloadUrl;
}

class UpdateService {
  /// Where the published build stamp lives. It sits in the downloads directory,
  /// which the portal already serves by filename, so no server change was
  /// needed to publish it.
  static const _versionFile = 'version.json';

  /// Extracts the release number from a build tag like
  /// "build 2026-07-31 · r123-honest-progress" -> 123.
  ///
  /// Comparing numbers rather than strings matters: a plain string difference
  /// would also fire when the running build is NEWER than the portal (a dev or
  /// pre-release machine), nagging the user to "update" to something older.
  static int? releaseNumber(String buildTag) {
    final m = RegExp(r'\br(\d+)\b').firstMatch(buildTag);
    if (m == null) return null;
    return int.tryParse(m.group(1)!);
  }

  /// Turns the relay's websocket URL into the portal's http(s) base.
  static String? _httpBase(String relayUrl) {
    if (relayUrl.trim().isEmpty) return null;
    var u = relayUrl.trim();
    if (u.startsWith('wss://')) {
      u = 'https://${u.substring(6)}';
    } else if (u.startsWith('ws://')) {
      u = 'http://${u.substring(5)}';
    }
    // Strip any signaling path so we are left with scheme://host[:port].
    final parsed = Uri.tryParse(u);
    if (parsed == null || parsed.host.isEmpty) return null;
    return Uri(scheme: parsed.scheme, host: parsed.host, port: parsed.hasPort ? parsed.port : null)
        .toString();
  }

  /// Ask the portal what the latest published build is.
  ///
  /// Returns null when the answer is unknown — no network, no version file, an
  /// unparsable stamp. Unknown must never be reported as "up to date": that is
  /// exactly the false reassurance this is meant to remove.
  static Future<UpdateInfo?> check(String relayUrl) async {
    final base = _httpBase(relayUrl);
    if (base == null) return null;
    final url = '$base/api/v1/public/installers/$_versionFile';
    try {
      final res = await http
          .get(Uri.parse(url))
          .timeout(const Duration(seconds: 8));
      if (res.statusCode != 200) {
        DiagLog.log('update', 'version check http=${res.statusCode}');
        return null;
      }
      final m = jsonDecode(res.body);
      if (m is! Map) return null;
      final latest = (m['build'] as String?)?.trim() ?? '';
      if (latest.isEmpty) return null;

      final current = AppConstants.buildTag;
      final latestNum = releaseNumber(latest);
      final currentNum = releaseNumber(current);
      // Only offer an update when the portal is genuinely AHEAD. If either
      // stamp can't be parsed, say nothing rather than guess.
      final available =
          latestNum != null && currentNum != null && latestNum > currentNum;

      DiagLog.log('update',
          'current=$current latest=$latest available=$available');
      return UpdateInfo(
        available: available,
        latestBuild: latest,
        currentBuild: current,
        downloadUrl: (m['url'] as String?) ?? '$base/downloads/',
      );
    } catch (e) {
      DiagLog.log('update', 'version check failed: $e');
      return null;
    }
  }
}
