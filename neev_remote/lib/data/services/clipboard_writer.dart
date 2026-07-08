import 'dart:typed_data';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Writes a file list to the OS clipboard with a COPY drop-effect.
///
/// The `pasteboard` package puts CF_HDROP on the Windows clipboard WITHOUT a
/// "Preferred DropEffect", and Windows then treats the paste as a MOVE — so a
/// mirrored clipboard file vanished from its folder after the user pasted it.
/// This calls a tiny native handler in the Windows runner that sets the effect
/// to COPY. No-op (returns false) off Windows / on web, so callers fall back to
/// the normal clipboard path.
class ClipboardWriter {
  static const MethodChannel _channel = MethodChannel('neev_remote/clipboard');

  static bool get isSupported =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.windows;

  /// Puts [paths] on the clipboard as a COPY. Returns true only if the native
  /// handler confirmed success.
  static Future<bool> writeFilesCopy(List<String> paths) async {
    if (!isSupported || paths.isEmpty) return false;
    try {
      final ok = await _channel.invokeMethod<bool>('writeFilesCopy', paths);
      return ok ?? false;
    } catch (_) {
      return false;
    }
  }

  /// Places a DELAYED-RENDER virtual-file group on the clipboard: the shell sees
  /// [files] (each `{'name': String, 'size': int}`) immediately, but their bytes
  /// are pulled only when the user pastes — at which point the native side asks
  /// Dart (via [pollFileRequests]) to fetch them. [token] identifies this set.
  /// Returns true on success (attended Windows only).
  static Future<bool> announceRemoteFiles(
      String token, List<Map<String, Object>> files) async {
    if (!isSupported || files.isEmpty) return false;
    try {
      final ok = await _channel.invokeMethod<bool>(
          'announceRemoteFiles', {'token': token, 'files': files});
      return ok ?? false;
    } catch (_) {
      return false;
    }
  }

  /// Returns byte-fetch requests the shell raised on paste, as a list of
  /// `{'token': String, 'index': int}`. Each is handed out once.
  static Future<List<Map<String, Object?>>> pollFileRequests() async {
    if (!isSupported) return const [];
    try {
      final res = await _channel.invokeMethod<List<Object?>>('pollFileRequests');
      if (res == null) return const [];
      return res
          .whereType<Map>()
          .map((m) => {'token': m['token'], 'index': m['index']})
          .toList();
    } catch (_) {
      return const [];
    }
  }

  /// Delivers fetched bytes back to a blocked paste. [ok]=false fails the paste
  /// cleanly (e.g. transfer error / timeout).
  static Future<void> deliverRemoteFileBytes(
      String token, int index, bool ok, Uint8List bytes) async {
    if (!isSupported) return;
    try {
      await _channel.invokeMethod('deliverRemoteFileBytes', {
        'token': token,
        'index': index,
        'ok': ok,
        'bytes': bytes,
      });
    } catch (_) {}
  }
}
