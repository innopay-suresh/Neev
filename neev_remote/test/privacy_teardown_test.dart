// Privacy mode is SESSION state, not machine state.
//
// The bug: turning privacy ON and then disconnecting left the host's screen
// blanked and its local input blocked. The host user was locked out of their
// own machine and could only recover by having someone reconnect and turn it
// off — the worst possible failure mode for a remote-access tool.
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/privacy_mode.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const channel = MethodChannel('neev_remote/privacy');
  final calls = <bool>[];

  setUp(() {
    calls.clear();
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
      if (call.method == 'setPrivacy') calls.add(call.arguments as bool);
      return null;
    });
  });

  test('privacy state is tracked so it can be torn down', () async {
    await PrivacyMode.set(true);
    if (!PrivacyMode.supported) return; // no native channel on this platform
    expect(PrivacyMode.isOn, isTrue,
        reason: 'the host must know privacy is engaged to clear it later');

    await PrivacyMode.set(false);
    expect(PrivacyMode.isOn, isFalse);
    expect(calls, [true, false]);
  });

  test('a failed disable keeps it marked ON so teardown retries', () async {
    if (!PrivacyMode.supported) return;
    await PrivacyMode.set(true);

    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
      throw PlatformException(code: 'boom');
    });
    await PrivacyMode.set(false);
    expect(PrivacyMode.isOn, isTrue,
        reason: 'a failed disable must not be recorded as "screen restored" — '
            'that would strand the user on a black screen');
  });
}
