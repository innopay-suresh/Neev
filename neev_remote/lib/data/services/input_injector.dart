import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import 'input_event.dart';

/// Host-side bridge that injects received [InputEvent]s into the local OS via a
/// native platform channel.
///
/// Native handlers live in:
///   * windows/runner/input_injector.cpp   (SendInput)
///   * macos/Runner/InputInjector.swift     (CGEvent)
///   * linux/input_injector.cc              (XTest)
///
/// On web or any platform without a handler, injection is a safe no-op.
class InputInjector {
  static const MethodChannel _channel = MethodChannel('neev_remote/input');

  bool get isSupported =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.macOS ||
          defaultTargetPlatform == TargetPlatform.linux);

  /// Injects a single decoded event. Errors are swallowed — a failed inject
  /// must never tear down the session.
  Future<void> inject(InputEvent event) async {
    if (!isSupported) return;
    try {
      await _channel.invokeMethod('inject', event.data);
    } catch (_) {
      // Permission not granted yet, or platform handler missing.
    }
  }

  /// Whether the host's foreground window runs at a higher integrity level than
  /// us (i.e. an elevated / run-as-admin app). When true, UIPI blocks our own
  /// `inject` from reaching it and input must be routed through the SYSTEM
  /// agent instead. Windows-only; false (and cheap) elsewhere.
  Future<bool> isForegroundElevated() async {
    if (!isSupported) return false;
    try {
      final r = await _channel.invokeMethod('isForegroundElevated');
      return r == true;
    } catch (_) {
      return false;
    }
  }
}
