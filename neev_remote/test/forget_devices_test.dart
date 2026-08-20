// The "Forget remembered devices" row is the ONLY way to undo a remembered
// Decline, which otherwise refuses a device with no prompt left to change.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/consent_store.dart';
import 'package:shared_preferences/shared_preferences.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() => SharedPreferences.setMockInitialValues({}));

  test('a remembered Decline round-trips, then forgetAll clears it', () async {
    await ConsentStore.remember('ctrl-926941775', false);

    // Same device, different wire form — must still match.
    expect((await ConsentStore.decisionFor('926 941 775'))!.allow, isFalse,
        reason: 'a remembered Decline must be found again');
    expect(await ConsentStore.count(), 1);

    await ConsentStore.forgetAll();
    expect(await ConsentStore.decisionFor('926941775'), isNull,
        reason: 'after forgetting, the device must prompt again');
    expect(await ConsentStore.count(), 0);
  });

  test('an unknown device has no remembered decision', () async {
    expect(await ConsentStore.decisionFor('111222333'), isNull);
  });

  test('a remembered VIEW-ONLY grant stays view-only', () async {
    await ConsentStore.remember('ctrl-444555666', true, control: false);
    final d = await ConsentStore.decisionFor('444 555 666');
    expect(d!.allow, isTrue, reason: 'the device is still admitted');
    expect(d.control, isFalse,
        reason: 'remembering the admission must not upgrade the access level');
  });
}
