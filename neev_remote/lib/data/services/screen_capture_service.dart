import 'dart:async';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/services.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

/// Thrown when macOS has not granted Screen Recording, so capture cannot start.
///
/// This exists because the alternative is not an error — it is a process
/// abort. See [ScreenCaptureService.getSources].
class ScreenPermissionDenied implements Exception {
  const ScreenPermissionDenied();

  @override
  String toString() =>
      'Screen Recording permission is required to share this screen. '
      'Grant it in System Settings → Privacy & Security → Screen Recording, '
      'then reopen Neev Remote.';
}

/// A capturable screen/window source on the host.
class CaptureSource {
  final String id;
  final String name;
  final SourceType type;

  const CaptureSource({
    required this.id,
    required this.name,
    required this.type,
  });
}

/// Real desktop screen capture backed by `flutter_webrtc`'s `desktopCapturer`
/// + `getDisplayMedia`. Works on Windows, macOS and Linux desktop builds.
///
/// macOS requires the user to grant Screen Recording permission the first
/// time capture starts (system TCC prompt).
class ScreenCaptureService {
  bool _isCapturing = false;
  MediaStream? _stream;

  bool get isCapturing => _isCapturing;
  MediaStream? get stream => _stream;

  static const MethodChannel _permission =
      MethodChannel('neev_remote/screenpermission');

  /// True when macOS has granted Screen Recording. Always true off macOS.
  ///
  /// Checked with CGPreflightScreenCaptureAccess, which never prompts and never
  /// touches the capture stack.
  static Future<bool> hasPermission() async {
    if (kIsWeb || !Platform.isMacOS) return true;
    try {
      return await _permission.invokeMethod<bool>('check') ?? false;
    } catch (_) {
      // No channel (older build / test harness) — don't block hosting on a
      // check that isn't there.
      return true;
    }
  }

  /// Shows the macOS Screen Recording prompt. Returns true if already granted.
  ///
  /// A FIRST grant does not apply to the running process — macOS only honours
  /// it for a newly launched one — so callers must still treat capture as
  /// unavailable until the app restarts.
  static Future<bool> requestPermission() async {
    if (kIsWeb || !Platform.isMacOS) return true;
    try {
      return await _permission.invokeMethod<bool>('request') ?? false;
    } catch (_) {
      return false;
    }
  }

  /// Enumerates available screens (and windows) on the host.
  ///
  /// Throws [ScreenPermissionDenied] rather than calling into flutter_webrtc
  /// without the grant. That call does not fail politely: on a Mac without
  /// Screen Recording, ObjCDesktopMediaList::UpdateSourceList calls abort() and
  /// takes the whole app down with SIGABRT. What that looked like in the field
  /// was not a permission problem at all — the app died the instant hosting
  /// started, relaunched, re-registered with the relay, and every viewer that
  /// dialed it hung at "Requesting the device" because the host aborted before
  /// it could show the consent prompt.
  Future<List<CaptureSource>> getSources({bool includeWindows = false}) async {
    if (!await hasPermission()) {
      // Ask once; the answer only takes effect for a new process, so this call
      // still throws and the caller reports why.
      await requestPermission();
      throw const ScreenPermissionDenied();
    }
    final types = <SourceType>[
      SourceType.Screen,
      if (includeWindows) SourceType.Window,
    ];
    final sources = await desktopCapturer.getSources(types: types);
    return sources
        .map((s) => CaptureSource(id: s.id, name: s.name, type: s.type))
        .toList();
  }

  /// Captures a specific screen by [sourceId]. When [sourceId] is null the
  /// primary screen is captured automatically (no picker dialog) — the
  /// behaviour an unattended host needs.
  Future<MediaStream?> startCapture({
    String? sourceId,
    int fps = 30,
    int? maxWidth,
    int? maxHeight,
  }) async {
    if (_isCapturing) {
      await stopCapture();
    }

    // Guard here too, not only in getSources: a caller passing an explicit
    // sourceId (a saved monitor choice, a viewer's monitor switch) would skip
    // enumeration entirely and reach getDisplayMedia — which aborts just the
    // same on a Mac without the grant.
    if (!await hasPermission()) {
      await requestPermission();
      throw const ScreenPermissionDenied();
    }

    var id = sourceId;
    if (id == null) {
      final screens = await getSources();
      if (screens.isEmpty) {
        _isCapturing = false;
        return null;
      }
      id = screens.first.id;
    }

    final mandatory = <String, dynamic>{
      'frameRate': fps.toDouble(),
      if (maxWidth != null) 'maxWidth': maxWidth,
      if (maxHeight != null) 'maxHeight': maxHeight,
    };

    try {
      _stream = await navigator.mediaDevices.getDisplayMedia(<String, dynamic>{
        'audio': false,
        'video': {
          'deviceId': {'exact': id},
          'mandatory': mandatory,
        },
      });
      _isCapturing = _stream!.getVideoTracks().isNotEmpty;
      return _isCapturing ? _stream : null;
    } catch (e) {
      _isCapturing = false;
      _stream = null;
      rethrow;
    }
  }

  MediaStreamTrack? get videoTrack {
    final tracks = _stream?.getVideoTracks();
    return (tracks != null && tracks.isNotEmpty) ? tracks.first : null;
  }

  Future<void> stopCapture() async {
    final stream = _stream;
    _stream = null;
    _isCapturing = false;
    if (stream != null) {
      for (final track in stream.getTracks()) {
        await track.stop();
      }
      await stream.dispose();
    }
  }

  Future<void> dispose() => stopCapture();
}
