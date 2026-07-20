import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:path_provider/path_provider.dart';

import '../../core/constants/app_constants.dart';
import '../../core/diag_log.dart';

/// Roadmap Phase 2 — session audit trail.
///
/// Append-only JSONL at a fixed location, one record per session, written on
/// session END (start details are carried in the open record until then).
///
/// Tamper-resistance: every line carries `prev` (the previous line's hash) and
/// `hash` = sha256(prev + canonical-payload). Editing or deleting any line
/// breaks the chain from that point on, and [verify] reports exactly where. It
/// does not make the file unwritable — that needs an OS-protected location or a
/// remote sink — but it makes silent edits detectable, which is the compliance
/// requirement.
///
/// Schema is deliberately stable so Phase 4 (central console) can ship these
/// records without a rewrite: field names and types here are the wire format.
class AuditLog {
  AuditLog._();
  static final AuditLog instance = AuditLog._();

  static const int schemaVersion = 1;
  static const String _fileName = 'sessions.jsonl';

  File? _file;
  String _lastHash = '';
  bool _ready = false;

  /// <appSupport>/NeevRemote/audit/sessions.jsonl
  Future<File> _open() async {
    if (_file != null) return _file!;
    Directory base;
    try {
      base = await getApplicationSupportDirectory();
    } catch (_) {
      base = Directory.systemTemp;
    }
    final dir = Directory('${base.path}${Platform.pathSeparator}audit');
    if (!await dir.exists()) await dir.create(recursive: true);
    final f = File('${dir.path}${Platform.pathSeparator}$_fileName');
    if (!await f.exists()) await f.create(recursive: true);
    _file = f;
    if (!_ready) {
      _lastHash = await _tailHash(f);
      _ready = true;
    }
    return f;
  }

  Future<String> _tailHash(File f) async {
    try {
      final lines = await f.readAsLines();
      for (var i = lines.length - 1; i >= 0; i--) {
        final l = lines[i].trim();
        if (l.isEmpty) continue;
        final m = jsonDecode(l) as Map<String, dynamic>;
        return (m['hash'] as String?) ?? '';
      }
    } catch (_) {}
    return '';
  }

  /// Canonical payload → stable hash input (keys sorted, no hash fields).
  String _canonical(Map<String, dynamic> rec) {
    final keys = rec.keys.where((k) => k != 'hash' && k != 'prev').toList()
      ..sort();
    return keys.map((k) => '$k=${rec[k]}').join('&');
  }

  /// Records one completed session. Returns the written record.
  ///
  /// [role] 'host' (someone connected TO this machine) or 'viewer' (this
  /// machine connected out). [consent] one of accepted / declined / unattended.
  /// [endReason] e.g. user_ended, peer_ended, network_lost, declined.
  Future<Map<String, dynamic>> record({
    required String role,
    required String peerId,
    required String deviceId,
    required DateTime startedAt,
    required DateTime endedAt,
    required String consent,
    required String endReason,
  }) async {
    final f = await _open();
    final rec = <String, dynamic>{
      'v': schemaVersion,
      'ts_start': startedAt.toUtc().toIso8601String(),
      'ts_end': endedAt.toUtc().toIso8601String(),
      'duration_s': endedAt.difference(startedAt).inSeconds,
      'role': role,
      'peer_id': peerId,
      'device_id': deviceId,
      'consent': consent,
      'end_reason': endReason,
      'platform': Platform.operatingSystem,
      'build': AppConstants.buildTag,
    };
    rec['prev'] = _lastHash;
    rec['hash'] =
        sha256.convert(utf8.encode('$_lastHash|${_canonical(rec)}')).toString();
    _lastHash = rec['hash'] as String;
    try {
      await f.writeAsString('${jsonEncode(rec)}\n',
          mode: FileMode.append, flush: true);
    } catch (e) {
      DiagLog.log('audit', 'write failed: $e');
    }
    return rec;
  }

  /// All records, newest first.
  Future<List<Map<String, dynamic>>> read({int limit = 500}) async {
    final f = await _open();
    try {
      final lines = await f.readAsLines();
      final out = <Map<String, dynamic>>[];
      for (final l in lines.reversed) {
        if (l.trim().isEmpty) continue;
        try {
          out.add(jsonDecode(l) as Map<String, dynamic>);
        } catch (_) {}
        if (out.length >= limit) break;
      }
      return out;
    } catch (_) {
      return const [];
    }
  }

  /// Walks the hash chain. Returns null when intact, else a description of the
  /// first broken line (1-indexed).
  Future<String?> verify() async {
    final f = await _open();
    try {
      final lines = await f.readAsLines();
      var prev = '';
      var n = 0;
      for (final l in lines) {
        if (l.trim().isEmpty) continue;
        n++;
        final rec = jsonDecode(l) as Map<String, dynamic>;
        if ((rec['prev'] ?? '') != prev) {
          return 'Chain broken at record $n (previous-hash mismatch).';
        }
        final expect =
            sha256.convert(utf8.encode('$prev|${_canonical(rec)}')).toString();
        if (rec['hash'] != expect) {
          return 'Record $n has been modified (hash mismatch).';
        }
        prev = rec['hash'] as String;
      }
      return null;
    } catch (e) {
      return 'Could not read the audit file: $e';
    }
  }

  Future<String> filePath() async => (await _open()).path;

  /// Sessions started today (real value for the dashboard chip).
  Future<int> countToday() async {
    final now = DateTime.now();
    final recs = await read(limit: 2000);
    var n = 0;
    for (final r in recs) {
      final t = DateTime.tryParse('${r['ts_start']}')?.toLocal();
      if (t != null &&
          t.year == now.year &&
          t.month == now.month &&
          t.day == now.day) {
        n++;
      }
    }
    return n;
  }
}
