import 'dart:typed_data';

import 'package:flutter/material.dart';

/// Web stub — no filesystem; device cards always use the glyph fallback.
class ThumbStore {
  Future<void> init() async {}
  bool get ready => false;
  String? pathFor(String normId) => null;
  Future<void> save(String normId, Uint8List png) async {}
}

Widget thumbImage(String path, {required Widget fallback}) => fallback;
