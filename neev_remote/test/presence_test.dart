// Presence for SAVED devices.
//
// The bug this addresses: relay discovery only reports machines sharing the
// requester's public IP, so an address-book entry on another network was always
// drawn "Offline" — even while it was reachable and actively being connected
// to. Presence asks the relay about specific ids the user already holds.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('no devices are claimed online before the relay answers', () {
    expect(RemoteService().presentIds, isEmpty,
        reason: 'never show a device as online on optimism alone');
  });

  test('tracking ids does not itself mark anything online', () {
    final s = RemoteService();
    s.trackPresenceFor(['036829329', '287290215']);
    expect(s.presentIds, isEmpty,
        reason: 'asking about a device says nothing about whether it answered');
  });

  test('an empty watch list is a no-op', () {
    final s = RemoteService();
    s.trackPresenceFor([]);
    expect(s.presentIds, isEmpty);
  });
}
