import 'dart:typed_data';

/// Web/no-op stub of [UacBridge]. UAC remote-control is a Windows-host-only
/// native feature; on web (and as a safe default) it does nothing.
class UacBridge {
  void Function(int w, int h)? onActive;
  void Function(Uint8List png)? onFrame;
  void Function()? onGone;

  bool get isSupported => false;
  bool get isConnected => false;

  void start() {}
  void sendClick(int button, double x, double y) {}
  void sendKey(int vk) {}
  void dispose() {}
}
