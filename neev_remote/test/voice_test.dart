// Two-way voice. The product previously had NO audio at all — you fixed
// someone's machine while talking to them on a separate phone call.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('the microphone is off until asked for', () {
    // Negotiating the channel must not open the mic: no permission prompt for
    // users who never use voice.
    expect(RemoteService().voiceOn, isFalse);
  });

  test('voice is reported UNAVAILABLE with no session', () {
    // Availability is the honest signal the mic button disables itself on. A
    // TransportMode host offers video only, and a button that transmits
    // nothing is worse than a disabled one.
    expect(RemoteService().voiceAvailable, isFalse);
  });

  test('enabling voice without a session is a no-op, not a crash', () async {
    final s = RemoteService();
    await s.setVoice(true);
    expect(s.voiceOn, isFalse,
        reason: 'there is no peer to carry audio, so nothing may claim to be on');
  });
}
