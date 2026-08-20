// Regression tests for the view-only input gate.
//
// The bug: "View Only" was enforced ONLY in RemoteViewWidget, which simply
// doesn't wire its pointer/Focus listeners when viewOnly is set. That covers
// the mouse, but two producers never go through the widget and so bypassed it
// completely — the OS-level keyboard hook (Windows AND macOS) and
// sendKeyCombo() from the shortcuts menu. The host does not gate input, so
// anything sent was executed: typing and shortcuts still controlled the remote
// machine while the UI showed "View Only", on every platform pair.
//
// The gate now lives in RemoteService, which is what both bypassing producers
// call into.

import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/remote_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('view-only gate', () {
    test('is off by default (full control still works)', () {
      final s = RemoteService();
      expect(s.inputBlocked, isFalse);
    });

    test('the persisted View Only setting blocks input', () {
      final s = RemoteService();
      s.viewOnlySetting = true;
      expect(s.inputBlocked, isTrue,
          reason: 'the mode selector writes this setting; the keyboard hook '
              'and shortcut buttons must see it');
    });

    test('the in-session toggle blocks input', () {
      final s = RemoteService();
      s.setViewOnly(true);
      expect(s.inputBlocked, isTrue);
    });

    test('either source alone is enough to block', () {
      final s = RemoteService();
      s.viewOnlySetting = true;
      s.setViewOnly(true);
      expect(s.inputBlocked, isTrue);

      // Clearing only ONE of the two must NOT re-enable control.
      s.setViewOnly(false);
      expect(s.inputBlocked, isTrue,
          reason: 'the persisted setting is still on');

      s.viewOnlySetting = false;
      expect(s.inputBlocked, isFalse, reason: 'both cleared → control returns');
    });

    test('keyboard capture cannot be armed while view-only', () {
      final s = RemoteService();
      s.viewOnlySetting = true;
      s.setKeyboardCapture(true);
      // The user preference is remembered...
      expect(s.keyboardCapture, isTrue);
      // ...but input stays blocked, so nothing reaches the host. Arming the
      // hook under view-only would swallow the user's own keystrokes locally
      // while sending nothing.
      expect(s.inputBlocked, isTrue);
    });
  });
}
