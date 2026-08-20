// A short transfer must never be saved as a complete file.
//
// 'end' and the data chunks ride different priority lanes, so 'end' can arrive
// before the tail of a large file. The receiver used to write whatever had
// turned up, mark it done, and ack 'saved' — so a truncated download was
// indistinguishable from a good one. Session recordings are the biggest thing
// this app moves; a short one simply refuses to play, with nothing saying why.
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/file_transfer_service.dart';
import 'package:neev_remote/data/services/file_store_io.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  /// Feeds an offer, some chunks, then 'end', and reports the resulting state.
  Future<FileTransfer> receive({
    required int offeredSize,
    required List<int> bytesActuallySent,
  }) async {
    final sent = <String>[];
    final mgr = FileTransferManager(
      send: sent.add,
      buffered: () => 0,
      store: FileStore(),
      onChange: () {},
    );

    mgr.handleMessage({
      'k': 'ft', 't': 'offer', 'id': 'x1',
      'name': 'session.webm', 'size': offeredSize,
    });
    if (bytesActuallySent.isNotEmpty) {
      mgr.handleMessage({
        'k': 'ft', 't': 'data', 'id': 'x1', 'seq': 0,
        'd': base64.encode(bytesActuallySent),
      });
    }
    mgr.handleMessage({'k': 'ft', 't': 'end', 'id': 'x1'});
    // _finishIncoming is async.
    await Future<void>.delayed(const Duration(milliseconds: 50));
    return mgr.transfers.firstWhere((t) => t.id == 'x1');
  }

  // NOTE ON TIMING: a short transfer is no longer failed the instant 'end'
  // arrives. 'end' can overtake the tail of a large file when control messages
  // and data travel different priority lanes, so the receiver now waits briefly
  // for the outstanding bytes and only then reports the shortfall. What must
  // never change is the invariant these guard: a short file is never saved or
  // reported as complete.

  test('a truncated transfer is never reported as done', () async {
    final t = await receive(offeredSize: 1000, bytesActuallySent: List.filled(600, 7));
    expect(t.status, isNot(FileStatus.done),
        reason: 'receiving 600 of 1000 bytes must not be reported as complete');
    expect(t.transferred, lessThan(1000),
        reason: 'progress must never claim bytes that did not arrive');
  });

  test('a transfer that never sends its bytes is never reported as done', () async {
    final t = await receive(offeredSize: 1000, bytesActuallySent: const []);
    expect(t.status, isNot(FileStatus.done),
        reason: 'a 0-byte arrival against a 1000-byte offer is not a success');
    expect(t.transferred, 0);
  });

  test('progress reports what arrived, never the promised size', () async {
    final t = await receive(offeredSize: 1000, bytesActuallySent: List.filled(600, 7));
    expect(t.transferred, isNot(1000),
        reason: 'a short file must not display a full progress bar');
  });
}
