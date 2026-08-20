// Connect progress must reflect reality.
//
// The old screen advanced four ticks on a 420ms timer regardless of what was
// happening — it showed "Verifying identity" complete for a password that had
// already been rejected, and a stall told you nothing about where it stalled.
// That is a Data Honesty Rule violation (DESIGN.md), and it actively misled us
// while debugging this product.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('a fresh service claims no progress', () {
    expect(RemoteService().viewerPhase, 0,
        reason: 'nothing has happened yet, so nothing may be shown as done');
  });

  test('progress does not advance on its own over time', () async {
    final s = RemoteService();
    await Future<void>.delayed(const Duration(milliseconds: 900));
    expect(s.viewerPhase, 0,
        reason: 'the old screen would have marked two stages complete by now '
            'without a single byte having been sent');
  });
}
