// Turning the microphone off must hand the device back to the OS.
//
// MediaStream.dispose() only tears down the stream object — it does NOT stop
// the tracks. The capture device stayed open and the OS went on showing the app
// as listening after the user switched the microphone off. Silent, and the
// worst bug a voice feature can have: the app says off, the mic indicator says
// otherwise.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('voice starts off', () {
    expect(RemoteService().voiceOn, isFalse);
  });

  test('turning voice off with no session is safe and stays off', () async {
    final s = RemoteService();
    await s.setVoice(false);
    expect(s.voiceOn, isFalse);
  });

  test('disconnecting leaves the microphone off', () async {
    // Teardown must release the device too — a session ending is exactly when
    // an app is least likely to be watched and most likely to be blamed for
    // holding the mic.
    final s = RemoteService();
    await s.disconnectViewer();
    expect(s.voiceOn, isFalse);
  });
}
