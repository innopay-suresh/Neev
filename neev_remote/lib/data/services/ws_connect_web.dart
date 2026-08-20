import 'package:web_socket_channel/web_socket_channel.dart';

/// Web build: the browser performs TLS validation and does not expose the peer
/// certificate, so pinning is not possible (and not ours to do). Parameters are
/// accepted so callers stay identical across platforms.
Future<WebSocketChannel> connectSignaling(
  String url, {
  String? pinSha256,
  void Function(String sha256)? onPinLearned,
}) async {
  return WebSocketChannel.connect(Uri.parse(url));
}

String prettyFingerprint(String sha256Hex) => sha256Hex;

String encodePin(Map<String, String> pins) => pins.toString();
