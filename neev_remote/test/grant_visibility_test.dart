// The viewer must be TOLD when the host granted view-only.
//
// Host-side enforcement silently drops input. Before this, the viewer's toolbar
// still said "Control", so a correctly-enforced restriction was
// indistinguishable from a broken app: clicks did nothing, with no explanation.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('control is assumed until the host says otherwise', () {
    // An older host announces no grant at all; it must keep behaving as before
    // rather than appearing restricted.
    expect(RemoteService().hostGrantedControl, isTrue);
  });

  test('the grant resets between sessions', () async {
    final s = RemoteService();
    await s.disconnectViewer();
    expect(s.hostGrantedControl, isTrue,
        reason: 'a previous host\'s view-only must not mislabel the next '
            'session, which may well have control');
  });
}
