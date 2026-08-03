// Unattended access, modelled on AnyDesk: whether a connection is prompted
// depends on HOW it authenticated, not on one global switch.
//
// The bug this replaces: "Ask before allowing connections" = off disabled the
// prompt for EVERYONE, including anyone holding the ordinary session password.
// That is strictly weaker than unattended access is supposed to be.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/presentation/providers/app_providers.dart';

void main() {
  test('defaults preserve existing behaviour', () {
    const s = AppSettings(relayUrl: 'wss://x');
    expect(s.interactiveAccess, 'always',
        reason: 'a host that never touched the setting keeps prompting');
    expect(s.unattendedAllowControl, isTrue);
    expect(s.unattendedAllowClipboard, isTrue);
    expect(s.unattendedAllowFiles, isTrue);
  });

  test('unattended permissions are independent of interactive ones', () {
    const s = AppSettings(relayUrl: 'wss://x');
    // An unmanned machine can be handed strictly less than someone at the
    // keyboard — the point of having two profiles.
    final narrowed = s.copyWith(
      unattendedAllowControl: false,
      unattendedAllowFiles: false,
    );
    expect(narrowed.unattendedAllowControl, isFalse);
    expect(narrowed.unattendedAllowFiles, isFalse);
    expect(narrowed.defaultAllowControl, isTrue,
        reason: 'interactive sessions are unaffected');
    expect(narrowed.defaultAllowFiles, isTrue);
  });

  test('interactive access can be disabled entirely', () {
    const s = AppSettings(relayUrl: 'wss://x');
    final locked = s.copyWith(interactiveAccess: 'never');
    expect(locked.interactiveAccess, 'never',
        reason: 'unattended password becomes the only way in');
  });
}
