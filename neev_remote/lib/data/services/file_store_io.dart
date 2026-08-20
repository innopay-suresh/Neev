import 'dart:io';
import 'dart:typed_data';

import 'package:path_provider/path_provider.dart';

/// Writes received files into a "NeevRemote" folder under the user's Downloads
/// (Documents as fallback). Desktop only; the web build uses the no-op stub.
class FileStore {
  bool get supported => true;

  /// Resolves (creating if needed) the NeevRemote destination directory.
  Future<Directory> _destDir() async {
    Directory? base;
    try {
      base = await getDownloadsDirectory();
    } catch (_) {}
    // When the host runs as SYSTEM (service / unattended mode),
    // getDownloadsDirectory resolves into the SYSTEM profile
    // (…\system32\config\systemprofile\Downloads) where no user can find the
    // file. Redirect to the all-users Public Downloads so received files are
    // always visible regardless of which account (or SYSTEM) is hosting.
    if (Platform.isWindows) {
      final p = base?.path.toLowerCase() ?? '';
      if (base == null ||
          p.contains('systemprofile') ||
          p.contains('system32')) {
        final pub = Platform.environment['PUBLIC'];
        if (pub != null && pub.isNotEmpty) {
          base = Directory('$pub${Platform.pathSeparator}Downloads');
        }
      }
    }
    base ??= await getApplicationDocumentsDirectory();
    final dir = Directory('${base.path}${Platform.pathSeparator}NeevRemote');
    if (!await dir.exists()) await dir.create(recursive: true);
    return dir;
  }

  String _sanitize(String name) {
    final safe = name.replaceAll(RegExp(r'[\\/:*?"<>|\x00-\x1f]'), '_').trim();
    return safe.isEmpty ? 'file' : safe;
  }

  /// Atomically reserves a UNIQUE destination path for [name] and returns it,
  /// creating an empty placeholder file so no concurrent transfer can pick the
  /// same name. `create(exclusive: true)` fails if the path already exists, so
  /// the dedup is race-free — unlike a check-then-write, two rapid transfers of
  /// the same-named file can never both win the same slot and clobber. Call
  /// [writeReserved] with the returned path to fill it.
  Future<String> reserveUnique(String name) async {
    final dir = await _destDir();
    final sep = Platform.pathSeparator;
    final cleaned = _sanitize(name);
    final dot = cleaned.lastIndexOf('.');
    final stem = dot > 0 ? cleaned.substring(0, dot) : cleaned;
    final ext = dot > 0 ? cleaned.substring(dot) : '';
    for (var i = 0; i < 10000; i++) {
      final path = i == 0
          ? '${dir.path}$sep$cleaned'
          : '${dir.path}$sep$stem ($i)$ext';
      try {
        await File(path).create(exclusive: true);
        return path;
      } catch (_) {
        // Catch ANY error, not just FileSystemException: on some platforms an
        // existing-path create surfaces a different subtype, and if that
        // propagated it would reject the reservation future and (pre-hardening)
        // strand every later transfer. Advance to the next candidate name.
        continue;
      }
    }
    // Last resort (10k collisions or a persistently-odd create error): a
    // timestamp-suffixed name that effectively cannot collide, so the file still
    // lands as its own distinct file rather than failing the transfer.
    final unique = '$stem-${DateTime.now().microsecondsSinceEpoch}$ext';
    final path = '${dir.path}$sep$unique';
    try {
      await File(path).create(exclusive: true);
    } catch (_) {}
    return path;
  }

  /// Writes [bytes] to a path previously returned by [reserveUnique].
  Future<void> writeReserved(String path, Uint8List bytes) async {
    await File(path).writeAsBytes(bytes, flush: true);
  }

  /// Best-effort delete (used to clean up a reserved placeholder on cancel).
  Future<void> deleteQuietly(String path) async {
    try {
      final f = File(path);
      if (await f.exists()) await f.delete();
    } catch (_) {}
  }

  /// Saves [bytes] as [name] (sanitised, never overwriting) and returns the
  /// absolute path written. Used for one-shot saves (e.g. clipboard staging);
  /// the streamed transfer path uses [reserveUnique] + [writeReserved].
  Future<String> saveToDownloads(String name, Uint8List bytes) async {
    final path = await reserveUnique(name);
    await writeReserved(path, bytes);
    return path;
  }

  /// Writes [bytes] to a temp file (used for clipboard-file paste: the receiver
  /// stages the file, then puts its path on the clipboard so Ctrl+V pastes it).
  Future<String?> saveToTemp(String name, Uint8List bytes) async {
    try {
      final dir = Directory(
          '${Directory.systemTemp.path}${Platform.pathSeparator}NeevRemote');
      if (!await dir.exists()) await dir.create(recursive: true);
      final safe =
          name.replaceAll(RegExp(r'[\\/:*?"<>|\x00-\x1f]'), '_').trim();
      final cleaned = safe.isEmpty ? 'file' : safe;
      final path = '${dir.path}${Platform.pathSeparator}$cleaned';
      await File(path).writeAsBytes(bytes, flush: true);
      return path;
    } catch (_) {
      return null;
    }
  }
}
