import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import '../../core/theme/app_theme.dart';
import '../../data/services/input_event.dart';

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
    final pos = _normalize(e.localPosition, size);
    if (pos == null) return;
    _focusNode.requestFocus();
    _activeButton = _buttonFrom(e.buttons);
    _emit(InputEvent.move(pos.dx, pos.dy));
    _emit(InputEvent.button(_activeButton, true));
  }

  void _onPointerMove(Offset local, Size size) {
    final pos = _normalize(local, size);
    if (pos != null) _emit(InputEvent.move(pos.dx, pos.dy));
  }

  void _onPointerUp() => _emit(InputEvent.button(_activeButton, false));

  void _onPointerSignal(PointerSignalEvent e) {
    if (e is PointerScrollEvent) {
      _emit(InputEvent.wheel(e.scrollDelta.dx, e.scrollDelta.dy));
    }
  }

  KeyEventResult _onKey(FocusNode node, KeyEvent event) {
    final usage = event.physicalKey.usbHidUsage;
    if (usage == 0) return KeyEventResult.ignored;
    if (event is KeyDownEvent) {
      _emit(InputEvent.key(usage, true));
      return KeyEventResult.handled;
    } else if (event is KeyUpEvent) {
      _emit(InputEvent.key(usage, false));
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
              child: MouseRegion(
                cursor: SystemMouseCursors.none,
                onHover: (e) => _onPointerMove(e.localPosition, size),
                child: Listener(
                  onPointerDown: (e) => _onPointerDown(e, size),
                  onPointerMove: (e) => _onPointerMove(e.localPosition, size),
                  onPointerUp: (_) => _onPointerUp(),
                  onPointerSignal: _onPointerSignal,
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
