import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:path_provider/path_provider.dart';

/// Stores one PNG thumbnail per device (the last captured remote frame), keyed by
/// a normalised device id, under <appSupport>/NeevRemote/thumbs/. Real data — an
/// actual frame from a real session — so it satisfies the Data Honesty Rule.
class ThumbStore {
  String? _dir;

  Future<void> init() async {
    try {
      final base = await getApplicationSupportDirectory();
      final sep = Platform.pathSeparator;
      final d = Directory('${base.path}${sep}NeevRemote${sep}thumbs');
      if (!await d.exists()) await d.create(recursive: true);
      _dir = d.path;
    } catch (_) {}
  }

  bool get ready => _dir != null;

  /// Absolute path a device's thumbnail would live at (may not exist yet).
  String? pathFor(String normId) =>
      _dir == null ? null : '$_dir${Platform.pathSeparator}$normId.png';

  Future<void> save(String normId, Uint8List png) async {
    final p = pathFor(normId);
    if (p == null) return;
    try {
      await File(p).writeAsBytes(png, flush: true);
    } catch (_) {}
  }
}

/// A device card's thumbnail image, or [fallback] if the file is missing/unreadable.
Widget thumbImage(String path, {required Widget fallback}) {
  return Image.file(
    File(path),
    fit: BoxFit.cover,
    gaplessPlayback: true,
    errorBuilder: (_, __, ___) => fallback,
  );
}
