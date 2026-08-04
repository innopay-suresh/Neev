// Viewer-initiated recording. The button must never claim a state the session
// is not actually in — the failure this project has hit repeatedly.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('recording is off until asked', () {
    expect(RemoteService().recording, isFalse);
  });

  test('with no session, Record does NOT claim to be recording', () {
    // There is no host to capture anything, so flipping the flag would light a
    // button for a recording that does not exist and promise a file that will
    // never arrive.
    final s = RemoteService();
    s.setRecording(true);
    expect(s.recording, isFalse);
  });
}
