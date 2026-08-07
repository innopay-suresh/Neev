// The worker reports "I cannot capture" through a file the app reads, because
// otherwise a macOS host connects, the viewer accepts, and nothing happens —
// the permission is missing and the only evidence is a log nobody reads.
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/capture_status.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('no marker means capture is fine', () async {
    // The common case must never raise a warning at the user.
    expect(await isCaptureBlocked(), isFalse);
  });

  test('the binary path is the one the user has to grant', () {
    if (!Platform.isMacOS) return;
    expect(captureBinaryPath,
        '/Library/Application Support/NeevRemote/neev-agent',
        reason: 'the path shown and copied must be the binary TCC keys on, '
            'not the app bundle');
  });
}
