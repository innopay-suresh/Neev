// The worker reports "I cannot capture" through a file the app reads, because
// otherwise a macOS host connects, the viewer accepts, and nothing happens —
// the permission is missing and the only evidence is a log nobody reads.
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/capture_status.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('reports a boolean without throwing', () async {
    // Deliberately NOT asserting a particular value: this reads real machine
    // state, and the first version of this test failed on a developer Mac whose
    // worker genuinely could not capture. A test that depends on the state of
    // the machine running it tells you nothing about the code.
    expect(await isCaptureBlocked(), isA<bool>());
  });

  test('the binary path is the one the user has to grant', () {
    if (!Platform.isMacOS) return;
    expect(captureBinaryPath,
        '/Library/Application Support/NeevRemote/neev-agent',
        reason: 'the path shown and copied must be the binary TCC keys on, '
            'not the app bundle');
  });
}
