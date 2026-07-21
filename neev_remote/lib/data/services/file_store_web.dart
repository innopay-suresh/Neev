import 'dart:typed_data';

/// Web/no-op stub of [FileStore]. Saving received files to disk is a desktop
/// feature; on web it does nothing (returns an empty path).
class FileStore {
  bool get supported => false;

  Future<String> saveToDownloads(String name, Uint8List bytes) async => '';

  Future<String> reserveUnique(String name) async => '';

  Future<void> writeReserved(String path, Uint8List bytes) async {}

  Future<void> deleteQuietly(String path) async {}

  /// Writes to a temp file and returns its path (null on web).
  Future<String?> saveToTemp(String name, Uint8List bytes) async => null;
}
