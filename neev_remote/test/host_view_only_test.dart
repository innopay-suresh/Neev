// The HOST decides the access level. Viewer-side view-only is a courtesy the
// viewer grants itself; host-side view-only is the one that actually holds.
//
// Reported symptom this locks down: enabling "view only" on the HOST did
// nothing (the viewer still had full control), while enabling it on the VIEWER
// worked — exactly backwards for a permission.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('a host defaults to granting control', () {
    expect(RemoteService().hostGrantsControl, isTrue,
        reason: 'never silently drop input on a host that never opted in');
  });

  test('host view-only is independent of the viewer-side flag', () {
    final s = RemoteService();
    s.hostGrantsControl = false; // host says: watch only

    // The viewer-side gate is a separate concern and must not be entangled:
    // a host granting view-only says nothing about what this app sends when it
    // is acting as a VIEWER of some other machine.
    expect(s.inputBlocked, isFalse);

    s.viewOnlySetting = true;
    expect(s.inputBlocked, isTrue);
    expect(s.hostGrantsControl, isFalse);
  });
}
