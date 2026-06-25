import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import '../../core/theme/app_theme.dart';
import '../../data/services/input_event.dart';

/// Flip to true to log the input event stream (down/up/cancel + a 1/s move
/// heartbeat) to the console / app log. Used to pinpoint whether the *viewer*
/// stops sending after a click vs. the host stops injecting.
const bool kLogRemoteInput = true;

/// Renders the remote video stream and, unless [viewOnly], captures local
/// mouse + keyboard input and forwards it as normalized [InputEvent]s via
/// [onInput].
class RemoteViewWidget extends StatefulWidget {
  final MediaStream? remoteStream;
  final bool isConnected;
  final bool viewOnly;
  final void Function(InputEvent event)? onInput;

  const RemoteViewWidget({
    super.key,
    this.remoteStream,
    this.isConnected = false,
    this.viewOnly = false,
    this.onInput,
  });

  @override
  State<RemoteViewWidget> createState() => _RemoteViewWidgetState();
}

class _RemoteViewWidgetState extends State<RemoteViewWidget> {
  final RTCVideoRenderer _renderer = RTCVideoRenderer();
  final FocusNode _focusNode = FocusNode();
  bool _initialized = false;
  int _activeButton = 0;
  bool _buttonHeld = false;
  Offset _lastLocal = Offset.zero;
  final Stopwatch _moveClock = Stopwatch()..start();
  int _lastMoveMs = -100;
  // Move-heartbeat logging.
  int _moveCount = 0;
  int _lastHeartbeatMs = 0;

  @override
  void initState() {
    super.initState();
    _initRenderer();
  }

  Future<void> _initRenderer() async {
    await _renderer.initialize();
    if (widget.remoteStream != null) {
      _renderer.srcObject = widget.remoteStream;
    }
    if (mounted) setState(() => _initialized = true);
  }

  @override
  void didUpdateWidget(RemoteViewWidget oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.remoteStream != oldWidget.remoteStream) {
      _renderer.srcObject = widget.remoteStream;
    }
  }

  @override
  void dispose() {
    _renderer.srcObject = null;
    _renderer.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  bool get _controlEnabled => !widget.viewOnly && widget.onInput != null;

  /// Maps a pointer position within [size] to normalized 0..1 coordinates over
  /// the letterboxed ("contain") video rect. Returns null if outside the video.
  Offset? _normalize(Offset local, Size size) {
    final vw = _renderer.videoWidth.toDouble();
    final vh = _renderer.videoHeight.toDouble();
    if (vw <= 0 || vh <= 0 || size.width <= 0 || size.height <= 0) {
      // Fall back to the whole widget area.
      return Offset(local.dx / size.width, local.dy / size.height);
    }
    final ar = vw / vh;
    double dispW, dispH;
    if (size.width / size.height > ar) {
      dispH = size.height;
      dispW = dispH * ar;
    } else {
      dispW = size.width;
      dispH = dispW / ar;
    }
    final left = (size.width - dispW) / 2;
    final top = (size.height - dispH) / 2;
    final nx = (local.dx - left) / dispW;
    final ny = (local.dy - top) / dispH;
    if (nx < 0 || nx > 1 || ny < 0 || ny > 1) return null;
    return Offset(nx, ny);
  }

  int _buttonFrom(int buttons) {
    if (buttons & kSecondaryButton != 0) return 1;
    if (buttons & kMiddleMouseButton != 0) return 2;
    return 0;
  }

  void _emit(InputEvent e) => widget.onInput?.call(e);

  void _onPointerDown(PointerDownEvent e, Size size) {
    _lastLocal = e.localPosition;
    final pos = _normalize(e.localPosition, size);
    if (pos == null) return;
    // Only request focus when we don't already have it. Calling requestFocus on
    // every click rebuilds the Focus subtree, and on macOS that rebuild drops
    // the MouseRegion's hover tracking — so move/hover events silently stop
    // after the first click and the remote cursor appears frozen.
    if (!_focusNode.hasFocus) _focusNode.requestFocus();
    _activeButton = _buttonFrom(e.buttons);
    _buttonHeld = true;
    _emit(InputEvent.move(pos.dx, pos.dy));
    _emit(InputEvent.button(_activeButton, true, x: pos.dx, y: pos.dy));
    _log('DOWN btn=$_activeButton');
  }

  void _onPointerMove(Offset local, Size size) {
    _lastLocal = local;
    // Throttle to ~60/s so fast movement doesn't flood the data channel.
    final now = _moveClock.elapsedMilliseconds;
    if (now - _lastMoveMs < 16) return;
    _lastMoveMs = now;
    final pos = _normalize(local, size);
    if (pos != null) {
      _emit(InputEvent.move(pos.dx, pos.dy));
      _heartbeat(now);
    }
  }

  void _onPointerUp(Offset local, Size size) {
    _lastLocal = local;
    // Always release the active button, even if the pointer was lifted outside
    // the video rect — otherwise the host's button stays stuck down.
    if (!_buttonHeld) return;
    _buttonHeld = false;
    final pos = _normalize(local, size);
    _emit(InputEvent.button(_activeButton, false, x: pos?.dx, y: pos?.dy));
    _log('UP btn=$_activeButton');
  }

  // macOS can deliver a PointerCancel instead of a PointerUp after a click
  // (gesture arena / cursor changes). If we ignore it the host's button stays
  // pressed, turning every later move into a drag and freezing the cursor.
  void _onPointerCancel(Size size) {
    if (!_buttonHeld) return;
    _buttonHeld = false;
    final pos = _normalize(_lastLocal, size);
    _emit(InputEvent.button(_activeButton, false, x: pos?.dx, y: pos?.dy));
    _log('CANCEL btn=$_activeButton (released)');
  }

  void _heartbeat(int now) {
    _moveCount++;
    if (now - _lastHeartbeatMs >= 1000) {
      _log('moves sent in last ~1s: $_moveCount');
      _moveCount = 0;
      _lastHeartbeatMs = now;
    }
  }

  void _log(String msg) {
    if (kLogRemoteInput) debugPrint('[remote-input] $msg');
  }

  void _onPointerSignal(PointerSignalEvent e) {
    if (e is PointerScrollEvent) {
      _emit(InputEvent.wheel(e.scrollDelta.dx, e.scrollDelta.dy));
    }
  }

  KeyEventResult _onKey(FocusNode node, KeyEvent event) {
    final usage = event.physicalKey.usbHidUsage;
    if (usage == 0) return KeyEventResult.ignored;
    // Flutter packs the USB HID *usage page* into the upper 16 bits (the
    // keyboard page is 0x07, so e.g. KeyA == 0x00070004). Every native host
    // maps the bare usage code, so strip the page before sending — otherwise
    // no key ever matches and typing silently does nothing.
    final hid = usage & 0xFFFF;
    if (event is KeyDownEvent || event is KeyRepeatEvent) {
      // Forward auto-repeat as additional downs so holding a key repeats it.
      _emit(InputEvent.key(hid, true));
      return KeyEventResult.handled;
    } else if (event is KeyUpEvent) {
      _emit(InputEvent.key(hid, false));
      return KeyEventResult.handled;
    }
    return KeyEventResult.ignored;
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.isConnected) {
      return _buildStatus('Waiting for connection...', Icons.hourglass_empty);
    }
    if (widget.remoteStream == null) {
      return _buildStatus('No video stream', Icons.videocam_off);
    }

    return ClipRRect(
      borderRadius: BorderRadius.circular(AppRadius.md),
      child: LayoutBuilder(
        builder: (context, constraints) {
          final size = constraints.biggest;
          Widget video = _initialized
              ? RTCVideoView(
                  _renderer,
                  objectFit:
                      RTCVideoViewObjectFit.RTCVideoViewObjectFitContain,
                  mirror: false,
                )
              : const ColoredBox(color: Colors.black);

          if (_controlEnabled) {
            video = Focus(
              focusNode: _focusNode,
              autofocus: true,
              onKeyEvent: _onKey,
              child: Listener(
                // opaque so pointer down/up fire over the (non-hit-testable)
                // video texture — otherwise only hover worked and clicks were
                // silently dropped.
                behavior: HitTestBehavior.opaque,
                onPointerHover: (e) => _onPointerMove(e.localPosition, size),
                onPointerDown: (e) => _onPointerDown(e, size),
                onPointerMove: (e) => _onPointerMove(e.localPosition, size),
                onPointerUp: (e) => _onPointerUp(e.localPosition, size),
                onPointerCancel: (_) => _onPointerCancel(size),
                onPointerSignal: _onPointerSignal,
                child: MouseRegion(
                  cursor: SystemMouseCursors.none,
                  child: video,
                ),
              ),
            );
          }

          return Stack(
            fit: StackFit.expand,
            children: [
              video,
              if (widget.viewOnly)
                Positioned(
                  top: AppSpacing.md,
                  right: AppSpacing.md,
                  child: _viewOnlyBadge(),
                ),
            ],
          );
        },
      ),
    );
  }

  Widget _viewOnlyBadge() {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.md,
        vertical: AppSpacing.sm,
      ),
      decoration: BoxDecoration(
        color: AppColors.warning.withValues(alpha: 0.9),
        borderRadius: BorderRadius.circular(AppRadius.sm),
      ),
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.visibility, size: 16, color: Colors.white),
          SizedBox(width: AppSpacing.xs),
          Text('View Only', style: TextStyle(color: Colors.white, fontSize: 12)),
        ],
      ),
    );
  }

  Widget _buildStatus(String message, IconData icon) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadius.md),
        border: Border.all(color: AppColors.border),
      ),
      child: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, size: 64, color: AppColors.textSecondary),
            const SizedBox(height: AppSpacing.lg),
            Text(message,
                style:
                    AppTypography.body.copyWith(color: AppColors.textSecondary)),
          ],
        ),
      ),
    );
  }
}
