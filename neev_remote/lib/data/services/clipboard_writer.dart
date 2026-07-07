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
}
