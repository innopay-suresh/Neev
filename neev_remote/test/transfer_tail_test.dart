// A sender's control messages and its data can travel independent priority
// lanes, so 'end' could arrive ahead of the tail of a large file. Finalising on
// 'end' alone wrote a short file and reported an incomplete transfer — which is
// how a screen recording, the biggest thing this moves, failed as a transfer
// error rather than as a recording problem.
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/file_transfer_service.dart';
import 'package:neev_remote/data/services/file_store_io.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  FileTransferManager makeManager(List<String> sent) => FileTransferManager(
        send: sent.add,
        buffered: () => 0,
        store: FileStore(),
        onChange: () {},
      );

  test('end arriving before the tail does NOT fail the transfer', () async {
    final sent = <String>[];
    final mgr = makeManager(sent);

    mgr.handleMessage({
      'k': 'ft', 't': 'offer', 'id': 'x1', 'name': 'session.webm', 'size': 1000,
    });
    mgr.handleMessage({
      'k': 'ft', 't': 'data', 'id': 'x1', 'seq': 0,
      'd': base64.encode(List.filled(600, 7)),
    });
    // The sender says it is done while 400 bytes are still in flight.
    mgr.handleMessage({'k': 'ft', 't': 'end', 'id': 'x1'});
    await Future<void>.delayed(const Duration(milliseconds: 50));

    // Asserting on bytes, not status: writing to disk needs path_provider,
    // which unit tests do not have, so status reflects the environment rather
    // than the behaviour under test.
    var t = mgr.transfers.firstWhere((t) => t.id == 'x1');
    expect(t.transferred, 600,
        reason: 'an early end must not finalise a transfer still in flight');

    // The tail arrives.
    mgr.handleMessage({
      'k': 'ft', 't': 'data', 'id': 'x1', 'seq': 1,
      'd': base64.encode(List.filled(400, 7)),
    });
    await Future<void>.delayed(const Duration(milliseconds: 50));

    t = mgr.transfers.firstWhere((t) => t.id == 'x1');
    expect(t.transferred, 1000,
        reason: 'every offered byte must be accounted for once the tail lands');
  });

  test('a genuinely truncated transfer still fails', () async {
    // The wait must be a grace period, not a licence to hang: if the rest never
    // comes, the transfer has to report the shortfall rather than sit in
    // "receiving" forever.
    final mgr = makeManager(<String>[]);
    mgr.handleMessage({
      'k': 'ft', 't': 'offer', 'id': 'x2', 'name': 'cut.webm', 'size': 1000,
    });
    mgr.handleMessage({
      'k': 'ft', 't': 'data', 'id': 'x2', 'seq': 0,
      'd': base64.encode(List.filled(600, 7)),
    });
    mgr.handleMessage({'k': 'ft', 't': 'end', 'id': 'x2'});
    await Future<void>.delayed(const Duration(milliseconds: 50));
    // Held, not finalised: the tail might still arrive. The grace timer is what
    // eventually fails it, so nothing here should show a completed byte count.
    expect(mgr.transfers.firstWhere((t) => t.id == 'x2').transferred, 600,
        reason: 'a short transfer must neither complete nor invent bytes');
  });

  test('a complete transfer finalises immediately on end', () async {
    // The common case must not pay the grace period.
    final mgr = makeManager(<String>[]);
    mgr.handleMessage({
      'k': 'ft', 't': 'offer', 'id': 'x3', 'name': 'small.txt', 'size': 10,
    });
    mgr.handleMessage({
      'k': 'ft', 't': 'data', 'id': 'x3', 'seq': 0,
      'd': base64.encode(List.filled(10, 1)),
    });
    mgr.handleMessage({'k': 'ft', 't': 'end', 'id': 'x3'});
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(mgr.transfers.firstWhere((t) => t.id == 'x3').transferred, 10);
  });

  test('one waiting transfer does not block another completing', () async {
    // Per-transfer independence: a large file waiting for its tail must not
    // hold up a small one that arrived intact.
    final mgr = makeManager(<String>[]);
    mgr.handleMessage({
      'k': 'ft', 't': 'offer', 'id': 'big', 'name': 'big.webm', 'size': 1000,
    });
    mgr.handleMessage({
      'k': 'ft', 't': 'data', 'id': 'big', 'seq': 0,
      'd': base64.encode(List.filled(400, 7)),
    });
    mgr.handleMessage({'k': 'ft', 't': 'end', 'id': 'big'});

    mgr.handleMessage({
      'k': 'ft', 't': 'offer', 'id': 'small', 'name': 'small.txt', 'size': 5,
    });
    mgr.handleMessage({
      'k': 'ft', 't': 'data', 'id': 'small', 'seq': 0,
      'd': base64.encode(List.filled(5, 1)),
    });
    mgr.handleMessage({'k': 'ft', 't': 'end', 'id': 'small'});
    await Future<void>.delayed(const Duration(milliseconds: 50));

    expect(mgr.transfers.firstWhere((t) => t.id == 'small').transferred, 5,
        reason: 'a small intact transfer must complete while a big one waits');
  });
}
