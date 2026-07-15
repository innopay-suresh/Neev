import 'package:flutter/foundation.dart'
    show defaultTargetPlatform, kIsWeb, TargetPlatform, Uint8List;
import 'package:flutter/services.dart';

/// macOS-only native clipboard bridge (see ClipboardMonitor.swift). Uses
/// NSPasteboard.changeCount for reliable change-detection + echo-suppression,
/// replacing the fragile Dart content-poll/hash that wedged after the first sync.
///
/// On Windows/Linux [supported] is false and this is never used — those
/// platforms keep their existing Dart poller + native ClipboardWriter untouched.
class NativeClipboardMonitor {
  NativeClipboardMonitor({
    required this.onText,
    required this.onImage,
    required this.onFiles,
  });

  /// Fired when the LOCAL user copies (not our own writes).
  final void Function(String text) onText;
  final void Function(Uint8List pngBytes) onImage;
  final void Function(List<String> paths) onFiles;

  static const MethodChannel _ch = MethodChannel('neev_remote/clipboard');
  bool _started = false;

  static bool get supported =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.macOS;

  void start() {
    if (!supported || _started) return;
    _started = true;
    _ch.setMethodCallHandler((call) async {
      if (call.method != 'changed') return null;
      final a = call.arguments;
      if (a is! Map) return null;
      switch (a['type']) {
        case 'text':
          final t = a['text'] as String?;
          if (t != null && t.isNotEmpty) onText(t);
          break;
        case 'image':
          final d = a['data'];
          if (d is Uint8List && d.isNotEmpty) onImage(d);
          break;
        case 'files':
          final p = (a['paths'] as List?)?.cast<String>() ?? const [];
          if (p.isNotEmpty) onFiles(p);
          break;
      }
      return null;
    });
    _ch.invokeMethod('start');
  }

  // Writes go through the native side so it records the changeCount they produce
  // (echo-suppression) and so files paste with COPY semantics.
  Future<void> writeText(String s) async {
    if (!supported) return;
    try {
      await _ch.invokeMethod('writeText', s);
    } catch (_) {}
  }

  Future<void> writeImage(Uint8List bytes) async {
    if (!supported) return;
    try {
      await _ch.invokeMethod('writeImage', bytes);
    } catch (_) {}
  }

  Future<void> writeFiles(List<String> paths) async {
    if (!supported) return;
    try {
      await _ch.invokeMethod('writeFiles', paths);
    } catch (_) {}
  }

  void stop() {
    if (!_started) return;
    _started = false;
    try {
      _ch.invokeMethod('stop');
    } catch (_) {}
  }
}
