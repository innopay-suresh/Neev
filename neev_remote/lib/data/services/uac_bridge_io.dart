import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/foundation.dart';

/// Talks to the privileged Windows helper agent (`neev_helper.exe`) over
/// localhost TCP (127.0.0.1:47921). Windows host only.
///
/// The agent streams the UAC secure desktop (which the in-app screen capture
/// can't see) and injects clicks/keys that the in-app injector can't reach.
/// It also injects ordinary input when the host's foreground window is elevated
/// (High integrity): our own in-app injector runs at Medium integrity and UIPI
/// blocks it from reaching elevated windows, but the SYSTEM agent can.
///
/// Wire protocol — `[uint32 LE len][uint8 type][payload]` (len = 1 + payload):
///   agent -> us:  'A' int32 w,h (UAC active)   'F' jpeg bytes  'G' (UAC gone)
///   us -> agent:  'C' uint8 btn, f32 x, f32 y (click)   'K' uint16 vk (key)
///                 'I' uint8 sub, ... (forwarded normal-desktop input; sub is
///                     'm' move f32 x,y | 'b' button u8 btn,down,hasPos,f32 x,y
///                     | 'w' wheel f32 dx,dy | 'k' key u16 hidUsage,u8 down)
class UacBridge {
  static const int _port = 47921;
  static const int _kActive = 0x41; // 'A'
  static const int _kFrame = 0x46; // 'F'
  static const int _kGone = 0x47; // 'G'
  static const int _kClick = 0x43; // 'C'
  static const int _kKey = 0x4B; // 'K'
  static const int _kInput = 0x49; // 'I'

  Socket? _sock;
  Uint8List _pending = Uint8List(0);
  Timer? _retry;
  bool _stopped = false;
  bool _started = false;

  /// UAC prompt appeared on the host; [w]x[h] is the captured desktop size.
  void Function(int w, int h)? onActive;

  /// A new PNG frame of the secure desktop.
  void Function(Uint8List png)? onFrame;

  /// The UAC prompt closed.
  void Function()? onGone;

  bool get isSupported => !kIsWeb && Platform.isWindows;
  bool get isConnected => _sock != null;

  /// Begins connecting (and auto-reconnecting) to the agent. No-op off Windows.
  /// Idempotent — safe to call on every host start.
  void start() {
    if (!isSupported || _stopped || _started) return;
    _started = true;
    _connect();
  }

  Future<void> _connect() async {
    if (_stopped) return;
    try {
      final s = await Socket.connect('127.0.0.1', _port,
          timeout: const Duration(seconds: 3));
      _sock = s;
      _pending = Uint8List(0);
      s.listen(_onData,
          onError: (_) => _reconnect(), onDone: _reconnect, cancelOnError: true);
      if (kDebugMode) debugPrint('[uac] connected to agent');
    } catch (_) {
      _scheduleRetry();
    }
  }

  void _scheduleRetry() {
    _retry?.cancel();
    _retry = Timer(const Duration(seconds: 3), () {
      if (!_stopped) _connect();
    });
  }

  void _reconnect() {
    _sock?.destroy();
    _sock = null;
    _pending = Uint8List(0);
    if (!_stopped) _scheduleRetry();
  }

  void _onData(Uint8List chunk) {
    _pending = Uint8List.fromList(<int>[..._pending, ...chunk]);
    int off = 0;
    while (_pending.length - off >= 4) {
      final len = ByteData.sublistView(_pending, off, off + 4)
          .getUint32(0, Endian.little);
      // Cap generously: a secure-desktop frame of a standard-user credential
      // prompt (undimmed wallpaper) is far bigger than the old 1 MB limit, which
      // silently dropped those frames so the prompt never reached the viewer.
      if (len == 0 || len > (16 << 20)) {
        _pending = Uint8List(0); // corrupt stream; resync by dropping
        return;
      }
      if (_pending.length - off - 4 < len) break; // wait for more bytes
      final type = _pending[off + 4];
      final payload =
          Uint8List.sublistView(_pending, off + 5, off + 4 + len);
      _handle(type, payload);
      off += 4 + len;
    }
    _pending = off >= _pending.length
        ? Uint8List(0)
        : Uint8List.fromList(Uint8List.sublistView(_pending, off));
  }

  void _handle(int type, Uint8List payload) {
    switch (type) {
      case _kActive:
        if (payload.length >= 8) {
          final bd = ByteData.sublistView(payload, 0, 8);
          onActive?.call(
              bd.getInt32(0, Endian.little), bd.getInt32(4, Endian.little));
        }
        break;
      case _kFrame:
        onFrame?.call(Uint8List.fromList(payload));
        break;
      case _kGone:
        onGone?.call();
        break;
    }
  }

  void _send(int type, Uint8List payload) {
    final s = _sock;
    if (s == null) return;
    final len = 1 + payload.length;
    final out = BytesBuilder();
    out.add((ByteData(4)..setUint32(0, len, Endian.little)).buffer.asUint8List());
    out.addByte(type);
    out.add(payload);
    try {
      s.add(out.toBytes());
    } catch (_) {
      _reconnect();
    }
  }

  /// Inject a click at normalized [x],[y] (0..1 over the captured desktop).
  void sendClick(int button, double x, double y) {
    final p = ByteData(9)
      ..setUint8(0, button)
      ..setFloat32(1, x, Endian.little)
      ..setFloat32(5, y, Endian.little);
    _send(_kClick, p.buffer.asUint8List());
  }

  /// Inject a virtual-key press (Win32 VK_* code).
  void sendKey(int vk) {
    final p = ByteData(2)..setUint16(0, vk, Endian.little);
    _send(_kKey, p.buffer.asUint8List());
  }

  /// Forward one ordinary [InputEvent] payload (the `{'k': ...}` map) to the
  /// SYSTEM agent so it can inject it into an elevated foreground window that
  /// our own Medium-integrity injector can't reach. Mirrors the binary layout
  /// the agent decodes in `InjectForwardedInput`.
  void sendInput(Map<String, dynamic> e) {
    final k = e['k'] as String?;
    final b = BytesBuilder();
    switch (k) {
      case 'mv':
        b.addByte(0x6D); // 'm'
        b.add((ByteData(8)
              ..setFloat32(0, (e['x'] as num?)?.toDouble() ?? 0, Endian.little)
              ..setFloat32(4, (e['y'] as num?)?.toDouble() ?? 0, Endian.little))
            .buffer
            .asUint8List());
        break;
      case 'btn':
        final x = (e['x'] as num?)?.toDouble();
        final y = (e['y'] as num?)?.toDouble();
        final hasPos = x != null && y != null;
        b.addByte(0x62); // 'b'
        b.add((ByteData(11)
              ..setUint8(0, (e['b'] as int?) ?? 0)
              ..setUint8(1, e['d'] == true ? 1 : 0)
              ..setUint8(2, hasPos ? 1 : 0)
              ..setFloat32(3, x ?? 0, Endian.little)
              ..setFloat32(7, y ?? 0, Endian.little))
            .buffer
            .asUint8List());
        break;
      case 'whl':
        b.addByte(0x77); // 'w'
        b.add((ByteData(8)
              ..setFloat32(0, (e['dx'] as num?)?.toDouble() ?? 0, Endian.little)
              ..setFloat32(4, (e['dy'] as num?)?.toDouble() ?? 0, Endian.little))
            .buffer
            .asUint8List());
        break;
      case 'key':
        b.addByte(0x6B); // 'k'
        b.add((ByteData(3)
              ..setUint16(0, (e['u'] as int?) ?? 0, Endian.little)
              ..setUint8(2, e['d'] == true ? 1 : 0))
            .buffer
            .asUint8List());
        break;
      default:
        return;
    }
    _send(_kInput, b.toBytes());
  }

  void dispose() {
    _stopped = true;
    _retry?.cancel();
    _sock?.destroy();
    _sock = null;
  }
}
