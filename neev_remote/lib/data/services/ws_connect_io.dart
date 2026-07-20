import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../../core/diag_log.dart';

/// Opens the signaling socket.
///
/// For `wss://` the relay normally runs on a PRIVATE address (e.g.
/// 172.17.17.77), so a public CA cannot issue for it — Let's Encrypt needs a
/// public domain and a reachable ACME challenge. The realistic trust model is
/// therefore a self-signed certificate pinned by SHA-256 fingerprint.
///
/// Behaviour:
///  * `ws://`  — unchanged (LAN / dev). Nothing here applies.
///  * `wss://` with a known pin — the certificate MUST match, else the
///    connection is refused. This is what stops an on-path attacker swapping
///    the DTLS fingerprints carried in the SDP.
///  * `wss://` with no pin yet — trust-on-first-use: accept once, hand the
///    fingerprint back so it can be stored, and log it loudly.
///
/// TOFU is a deliberate tradeoff, not an oversight: it protects every
/// connection after the first, which is the best a private-IP deployment can do
/// without shipping an internal CA. Pre-seeding [pinSha256] from the installer
/// removes the first-use gap entirely.
Future<WebSocketChannel> connectSignaling(
  String url, {
  String? pinSha256,
  void Function(String sha256)? onPinLearned,
}) async {
  final uri = Uri.parse(url);
  if (uri.scheme != 'wss') {
    // Plaintext / LAN path — untouched.
    return WebSocketChannel.connect(uri);
  }

  final expected = pinSha256?.trim().toLowerCase().replaceAll(':', '');
  final client = HttpClient()
    ..badCertificateCallback = (X509Certificate cert, String host, int port) {
      final actual = sha256.convert(cert.der).toString().toLowerCase();
      if (expected == null || expected.isEmpty) {
        DiagLog.log('tls',
            'TOFU: pinning relay cert for $host:$port sha256=$actual');
        onPinLearned?.call(actual);
        return true;
      }
      final ok = actual == expected;
      DiagLog.log(
          'tls',
          ok
              ? 'relay cert pin OK ($host:$port)'
              : 'relay cert pin MISMATCH for $host:$port — refusing. '
                  'expected=$expected actual=$actual');
      return ok;
    };

  return IOWebSocketChannel.connect(uri, customClient: client);
}

/// Formats a fingerprint for display: aa:bb:cc…
String prettyFingerprint(String sha256Hex) {
  final s = sha256Hex.replaceAll(':', '').toLowerCase();
  final out = <String>[];
  for (var i = 0; i + 1 < s.length; i += 2) {
    out.add(s.substring(i, i + 2));
  }
  return out.join(':');
}

/// Kept so callers can json-encode a pin record without importing dart:convert.
String encodePin(Map<String, String> pins) => jsonEncode(pins);
