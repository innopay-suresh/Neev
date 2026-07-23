// Per-device session thumbnails (last captured remote frame). Conditional so the
// web build (no dart:io) gets no-op stubs and device cards fall back to a glyph.
export 'thumb_store_web.dart' if (dart.library.io) 'thumb_store_io.dart';
