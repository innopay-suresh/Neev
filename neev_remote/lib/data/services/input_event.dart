import 'dart:convert';

/// Remote input events sent from the viewer to the host over the WebRTC
/// control data channel.
///
/// Design notes:
///  * Pointer coordinates are **normalized** (0.0–1.0) relative to the remote
///    frame, so the host maps them to its own screen resolution — no need for
///    the viewer to know the host's pixel dimensions.
///  * Keys are identified by their **USB HID usage** code, which is platform
///    independent. Each native host maps HID usage → its own virtual key
///    (Windows VK / macOS CGKeyCode / Linux keysym).
class InputEvent {
  final Map<String, dynamic> data;
  const InputEvent(this.data);

  /// Pointer move to a normalized position.
  factory InputEvent.move(double x, double y) =>
      InputEvent({'k': 'mv', 'x': _clamp01(x), 'y': _clamp01(y)});

  /// Mouse button: [button] 0=left, 1=right, 2=middle; [down] = press/release.
  factory InputEvent.button(int button, bool down) =>
      InputEvent({'k': 'btn', 'b': button, 'd': down});

  /// Scroll wheel deltas (logical pixels).
  factory InputEvent.wheel(double dx, double dy) =>
      InputEvent({'k': 'whl', 'dx': dx, 'dy': dy});

  /// Keyboard event by USB HID usage code; [down] = press/release.
  factory InputEvent.key(int usbHidUsage, bool down) =>
      InputEvent({'k': 'key', 'u': usbHidUsage, 'd': down});

  String get kind => data['k'] as String;

  String encode() => jsonEncode(data);

  static InputEvent? decode(String raw) {
    try {
      final m = jsonDecode(raw);
      if (m is Map<String, dynamic> && m['k'] is String) return InputEvent(m);
    } catch (_) {}
    return null;
  }

  static double _clamp01(double v) => v < 0 ? 0 : (v > 1 ? 1 : v);
}
