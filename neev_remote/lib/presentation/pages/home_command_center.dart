import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/constants/app_constants.dart';
import '../../core/theme/app_theme.dart';
import '../../data/services/audit_log.dart';
import '../../data/services/discovery_model.dart';
import '../../data/services/file_transfer_service.dart' show FileStatus;
import '../../data/services/remote_service.dart';
import '../../data/services/thumb_store.dart';
import '../providers/app_providers.dart';

/// Full-screen connection sequence shown while the viewer is connecting — a
/// glowing encrypted path between this device and the remote, with named stages
/// (locating → securing → verifying → negotiating) instead of a bare spinner.
/// The parent swaps to the live session the moment status flips to connected, so
/// this holds at the last pre-connect stage under a slow link (no fake looping).
class ConnectionSequence extends StatefulWidget {
  final String targetLabel;
  final VoidCallback onCancel;
  const ConnectionSequence({
    super.key,
    required this.targetLabel,
    required this.onCancel,
  });
  @override
  State<ConnectionSequence> createState() => _ConnectionSequenceState();
}

class _ConnectionSequenceState extends State<ConnectionSequence>
    with SingleTickerProviderStateMixin {
  static const _stages = [
    'Locating device',
    'Establishing secure channel',
    'Verifying identity',
    'Negotiating display quality',
  ];
  late final AnimationController _c;
  int _stage = 0;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _c = AnimationController(
        vsync: this, duration: const Duration(milliseconds: 1400))
      ..repeat();
    _timer = Timer.periodic(const Duration(milliseconds: 420), (_) {
      if (!mounted) return;
      // Advance through the stages, then hold on the last one until the real
      // connection completes (the parent replaces this screen on 'connected').
      if (_stage < _stages.length - 1) setState(() => _stage++);
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background,
      body: Center(
        child: Container(
          width: 520,
          padding: const EdgeInsets.fromLTRB(36, 40, 36, 30),
          decoration: BoxDecoration(
            color: AppColors.surface,
            borderRadius: BorderRadius.circular(AppRadii.panel),
            border: Border.all(color: AppColors.border),
            boxShadow: AppShadows.dock,
          ),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            SizedBox(
              height: 96,
              child: AnimatedBuilder(
                animation: _c,
                builder: (_, __) => CustomPaint(
                  painter: _PathPainter(_c.value),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      _EndNode(icon: Icons.laptop_windows_rounded, label: 'This PC'),
                      _EndNode(
                          icon: Icons.dns_rounded,
                          label: widget.targetLabel,
                          remote: true),
                    ],
                  ),
                ),
              ),
            ),
            const SizedBox(height: 30),
            Text(_stages[_stage],
                style: AppTypography.pageTitle.copyWith(fontSize: 18)),
            const SizedBox(height: 4),
            Text('Securing an end-to-end encrypted connection…',
                style: AppTypography.caption),
            const SizedBox(height: 22),
            ...List.generate(_stages.length, (i) {
              final done = i < _stage;
              final active = i == _stage;
              return Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  SizedBox(
                    width: 20,
                    height: 20,
                    child: done
                        ? Icon(Icons.check_circle_rounded,
                            size: 18, color: AppColors.success)
                        : active
                            ? CircularProgressIndicator(
                                strokeWidth: 2,
                                valueColor: AlwaysStoppedAnimation(
                                    AppColors.primary))
                            : Icon(Icons.circle_outlined,
                                size: 16, color: AppColors.textTertiary),
                  ),
                  const SizedBox(width: 12),
                  Text(_stages[i],
                      style: AppTypography.body.copyWith(
                          fontSize: 13.5,
                          color: (done || active)
                              ? AppColors.textPrimary
                              : AppColors.textTertiary,
                          fontWeight:
                              active ? FontWeight.w600 : FontWeight.w500)),
                ]),
              );
            }),
            const SizedBox(height: 26),
            TextButton(
              onPressed: widget.onCancel,
              child: Text('Cancel',
                  style: AppTypography.bodyStrong
                      .copyWith(color: AppColors.textSecondary)),
            ),
          ]),
        ),
      ),
    );
  }
}

class _EndNode extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool remote;
  const _EndNode({required this.icon, required this.label, this.remote = false});
  @override
  Widget build(BuildContext context) {
    return Column(mainAxisSize: MainAxisSize.min, children: [
      Container(
        width: 60,
        height: 60,
        decoration: BoxDecoration(
          color: remote ? AppColors.deviceNavy : AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(
              color: remote ? Colors.transparent : AppColors.borderStrong),
          boxShadow: [
            BoxShadow(
                color: AppColors.primary.withValues(alpha: 0.25),
                blurRadius: 18,
                spreadRadius: -4),
          ],
        ),
        child: Icon(icon,
            size: 26,
            color: remote ? Colors.white : AppColors.textSecondary),
      ),
      const SizedBox(height: 8),
      SizedBox(
        width: 90,
        child: Text(label,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            textAlign: TextAlign.center,
            style: AppTypography.caption
                .copyWith(fontSize: 11.5, fontWeight: FontWeight.w600)),
      ),
    ]);
  }
}

class _PathPainter extends CustomPainter {
  final double t; // 0..1 travelling position
  _PathPainter(this.t);
  @override
  void paint(Canvas canvas, Size size) {
    final y = 30.0; // centre of the 60px icon row
    final x0 = 66.0, x1 = size.width - 66.0;
    // base track
    canvas.drawLine(
        Offset(x0, y),
        Offset(x1, y),
        Paint()
          ..color = AppColors.border
          ..strokeWidth = 2);
    // travelling glow
    final px = x0 + (x1 - x0) * t;
    final grad = Paint()
      ..shader = LinearGradient(colors: [
        AppColors.primary.withValues(alpha: 0),
        AppColors.primary,
        AppColors.primary.withValues(alpha: 0),
      ]).createShader(Rect.fromLTWH(px - 40, y - 2, 80, 4))
      ..strokeWidth = 3;
    canvas.drawLine(Offset((px - 40).clamp(x0, x1), y),
        Offset((px + 40).clamp(x0, x1), y), grad);
    canvas.drawCircle(
        Offset(px, y),
        4,
        Paint()
          ..color = AppColors.primary
          ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 3));
  }

  @override
  bool shouldRepaint(_PathPainter old) => old.t != t;
}

/// One entry in the compact nav rail.
class NavRailItem {
  final IconData icon;
  final String label;
  const NavRailItem(this.icon, this.label);
}

/// Compact expandable navigation rail (88 → 240px on hover) — the left edge of
/// the Command Center shell. Icons always; labels + brand + device name fade in
/// when expanded. Active item: soft-orange fill + a coral indicator on the left.
// ============================================================ START CONNECTION
// Mockup hero: "Start a new connection" card with the animated orange globe.
class _StartConnectionCard extends StatelessWidget {
  final TextEditingController idController;
  final TextEditingController passwordController;
  final VoidCallback onConnect;
  final List<RecentConnection> recents;
  final void Function(String id) onPick;
  final VoidCallback onClear;
  const _StartConnectionCard({
    required this.idController,
    required this.passwordController,
    required this.onConnect,
    required this.recents,
    required this.onPick,
    required this.onClear,
  });

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(AppRadii.panel),
      child: Container(
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(AppRadii.panel),
          border: Border.all(color: AppColors.border),
          boxShadow: AppShadows.card,
        ),
        // Row (form | globe), NOT a Stack with a negative-Positioned globe: the
        // old Stack let the decorative globe overlap and displace the form
        // (title + Remote-ID field flung right, hint orphaned). As siblings the
        // form always owns its width and the globe can never collide with it.
        child: LayoutBuilder(builder: (context, c) {
          final wide = c.maxWidth >= 900;
          final globeW = wide ? (c.maxWidth * 0.22).clamp(180.0, 300.0) : 0.0;
          final form = Padding(
            padding: const EdgeInsets.fromLTRB(24, 20, 20, 18),
            child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text('Start a new connection',
                      style: AppTypography.pageTitle.copyWith(fontSize: 19)),
                  const SizedBox(height: 3),
                  Text('Connect to any device using ID, device name or alias.',
                      style: AppTypography.caption.copyWith(fontSize: 12.5)),
                  const SizedBox(height: 16),
                  Row(children: [
                    Expanded(
                      flex: 5,
                      child: _LabeledField(
                        controller: idController,
                        label: 'Remote ID',
                        hint: 'Enter Remote ID or Device Name',
                        icon: Icons.devices_rounded,
                        mono: true,
                        onSubmitted: (_) => onConnect(),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      flex: 4,
                      child: _LabeledField(
                        controller: passwordController,
                        label: 'Password',
                        hint: 'Enter Password',
                        icon: Icons.lock_outline_rounded,
                        obscure: true,
                        onSubmitted: (_) => onConnect(),
                      ),
                    ),
                    const SizedBox(width: 10),
                    const SizedBox(width: 148, child: _ModeSelector()),
                    const SizedBox(width: 10),
                    _WideConnectButton(onTap: onConnect),
                  ]),
                  if (recents.isNotEmpty) ...[
                    const SizedBox(height: 14),
                    Row(crossAxisAlignment: CrossAxisAlignment.center, children: [
                      Text('Recent IDs:',
                          style: AppTypography.caption.copyWith(
                              fontSize: 12.5, color: AppColors.textSecondary)),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Wrap(
                          spacing: 8,
                          runSpacing: 8,
                          crossAxisAlignment: WrapCrossAlignment.center,
                          children: [
                            for (final r in recents.take(4))
                              _RecentChip(
                                  name: r.name, onTap: () => onPick(r.id)),
                            InkWell(
                              onTap: onClear,
                              child: Text('Clear all',
                                  style: AppTypography.bodyStrong.copyWith(
                                      fontSize: 12.5,
                                      color: AppColors.primary)),
                            ),
                          ],
                        ),
                      ),
                    ]),
                  ],
                ]),
          );
          if (!wide) return form;
          // The night map is the card's BACKGROUND (CustomPaint paints before
          // its child), so it fills the whole panel edge-to-edge and the form
          // sits on top — no layout participation, so it can never displace the
          // fields the way the old Stack/Row versions did.
          return _AnimatedGlobe(child: form);
        }),
      ),
    );
  }
}

// A field with a small label above the input + a leading icon (mockup style).
class _LabeledField extends StatefulWidget {
  final TextEditingController controller;
  final String label;
  final String hint;
  final IconData icon;
  final bool mono;
  final bool obscure;
  final ValueChanged<String>? onSubmitted;
  const _LabeledField({
    required this.controller,
    required this.label,
    required this.hint,
    required this.icon,
    this.mono = false,
    this.obscure = false,
    this.onSubmitted,
  });
  @override
  State<_LabeledField> createState() => _LabeledFieldState();
}

class _LabeledFieldState extends State<_LabeledField> {
  late bool _hide = widget.obscure;
  @override
  Widget build(BuildContext context) {
    return Container(
      height: 56,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: AppColors.surfaceLight,
        borderRadius: BorderRadius.circular(AppRadii.md),
        border: Border.all(color: AppColors.borderStrong),
      ),
      child: Row(children: [
        Icon(widget.icon, size: 18, color: AppColors.textTertiary),
        const SizedBox(width: 11),
        Expanded(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(widget.label,
                  maxLines: 1,
                  softWrap: false,
                  overflow: TextOverflow.clip,
                  style: AppTypography.microLabel
                      .copyWith(fontSize: 9.5, letterSpacing: 0.8, height: 1.0)),
              const SizedBox(height: 4),
              SizedBox(
                height: 19,
                child: TextField(
                  controller: widget.controller,
                  obscureText: _hide,
                  onSubmitted: widget.onSubmitted,
                  style: widget.mono
                      ? AppTypography.mono.copyWith(fontSize: 14, height: 1.0)
                      : AppTypography.body.copyWith(fontSize: 14, height: 1.0),
                  // Clear EVERY border + the fill: `border:` alone is only a
                  // fallback — the theme's enabledBorder/focusedBorder and
                  // filled:true still merge in, drawing a nested inner pill
                  // inside the field (the cramped double-outline look).
                  decoration: InputDecoration(
                    isCollapsed: true,
                    filled: false,
                    border: InputBorder.none,
                    enabledBorder: InputBorder.none,
                    focusedBorder: InputBorder.none,
                    hintText: widget.hint,
                    hintStyle: AppTypography.body.copyWith(
                        fontSize: 13.5,
                        height: 1.0,
                        color: AppColors.textTertiary),
                  ),
                ),
              ),
            ],
          ),
        ),
        if (widget.obscure)
          InkWell(
            onTap: () => setState(() => _hide = !_hide),
            child: Icon(
                _hide
                    ? Icons.visibility_off_rounded
                    : Icons.visibility_rounded,
                size: 17,
                color: AppColors.textTertiary),
          ),
      ]),
    );
  }
}

// Full-width-ish ember/orange Connect button (reused by the hero card).
class _WideConnectButton extends StatefulWidget {
  final VoidCallback onTap;
  const _WideConnectButton({required this.onTap});
  @override
  State<_WideConnectButton> createState() => _WideConnectButtonState();
}

class _WideConnectButtonState extends State<_WideConnectButton> {
  bool _hover = false;
  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: widget.onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 160),
          height: 56,
          padding: const EdgeInsets.symmetric(horizontal: 22),
          decoration: BoxDecoration(
            color: _hover ? AppColors.primaryDark : AppColors.primary,
            borderRadius: BorderRadius.circular(AppRadii.md),
            boxShadow: [
              BoxShadow(
                  color: AppColors.primary.withValues(alpha: _hover ? 0.4 : 0.28),
                  blurRadius: _hover ? 20 : 12,
                  offset: Offset(0, _hover ? 8 : 5)),
            ],
          ),
          alignment: Alignment.center,
          child: const Row(mainAxisSize: MainAxisSize.min, children: [
            Text('Connect',
                style: TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.w700,
                    fontSize: 15)),
            SizedBox(width: 9),
            Icon(Icons.arrow_forward_rounded, color: Colors.white, size: 19),
          ]),
        ),
      ),
    );
  }
}

// Earth at night, seen from space: dark equirectangular world map with glowing
// city lights. Replaces the old wireframe globe (read as an empty cage).
class _AnimatedGlobe extends StatefulWidget {
  final Widget child;
  const _AnimatedGlobe({required this.child});
  @override
  State<_AnimatedGlobe> createState() => _AnimatedGlobeState();
}

class _AnimatedGlobeState extends State<_AnimatedGlobe>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c;
  @override
  void initState() {
    super.initState();
    _c = AnimationController(vsync: this, duration: const Duration(seconds: 12))
      ..repeat();
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final reduce = MediaQuery.maybeOf(context)?.disableAnimations ?? false;
    return AnimatedBuilder(
      animation: _c,
      builder: (_, __) => CustomPaint(
        painter: _NightEarthPainter(reduce ? 0 : _c.value),
        child: widget.child,
      ),
    );
  }
}

/// Real coastline geometry (simplified), as [lon, lat] rings — a vector world
/// map, NOT a dot grid. Equirectangular projection at paint time.
const List<List<List<double>>> _worldLand = [
  // North America
  [
    [-168, 66], [-165, 60], [-158, 57], [-152, 59], [-145, 60], [-138, 58],
    [-131, 54], [-125, 49], [-124, 43], [-121, 36], [-117, 32], [-114, 30],
    [-110, 24], [-106, 22], [-101, 19], [-97, 16], [-92, 15], [-88, 16],
    [-87, 21], [-91, 21], [-95, 19], [-97, 23], [-97, 27], [-94, 29],
    [-89, 29], [-85, 30], [-81, 25], [-80, 28], [-78, 33], [-75, 36],
    [-71, 41], [-67, 44], [-60, 47], [-56, 51], [-64, 60], [-71, 62],
    [-78, 63], [-80, 70], [-90, 72], [-100, 70], [-110, 69], [-120, 70],
    [-130, 70], [-141, 70], [-155, 71], [-165, 68],
  ],
  // Greenland
  [
    [-45, 60], [-52, 64], [-56, 70], [-58, 76], [-50, 82], [-30, 83],
    [-20, 80], [-22, 74], [-32, 68], [-42, 62],
  ],
  // South America
  [
    [-81, 8], [-77, 8], [-72, 12], [-63, 11], [-60, 7], [-52, 5], [-50, 0],
    [-44, -2], [-38, -6], [-35, -9], [-38, -13], [-39, -18], [-45, -23],
    [-48, -26], [-54, -34], [-58, -39], [-63, -41], [-65, -45], [-68, -51],
    [-71, -55], [-75, -52], [-74, -45], [-73, -38], [-71, -30], [-70, -23],
    [-70, -18], [-75, -15], [-78, -9], [-81, -5], [-80, 0], [-78, 4],
  ],
  // Africa
  [
    [-17, 15], [-16, 21], [-12, 26], [-9, 31], [-5, 35], [3, 37], [11, 34],
    [19, 31], [25, 32], [32, 31], [35, 24], [37, 18], [39, 15], [43, 12],
    [48, 12], [51, 11], [45, 4], [41, -2], [40, -10], [38, -16], [35, -21],
    [32, -26], [27, -34], [20, -35], [15, -27], [12, -17], [9, -6], [9, 2],
    [5, 5], [-2, 5], [-8, 4], [-13, 9],
  ],
  // Madagascar
  [[43, -12], [50, -15], [50, -25], [45, -25], [43, -18]],
  // Eurasia
  [
    [-10, 36], [-9, 43], [-2, 43], [0, 49], [2, 51], [8, 54], [10, 57],
    [7, 58], [5, 59], [7, 63], [12, 65], [15, 69], [21, 70], [28, 71],
    [35, 69], [44, 68], [52, 70], [60, 70], [70, 73], [78, 74], [86, 74],
    [95, 78], [105, 77], [113, 74], [125, 73], [135, 72], [145, 70],
    [155, 70], [165, 69], [175, 68], [180, 65], [175, 62], [165, 60],
    [160, 58], [155, 52], [145, 44], [140, 45], [135, 43], [130, 42],
    [127, 38], [122, 30], [118, 24], [110, 21], [108, 15], [105, 10],
    [103, 1], [100, 6], [98, 10], [95, 16], [91, 22], [88, 21], [80, 15],
    [77, 8], [73, 15], [70, 21], [65, 25], [60, 25], [57, 22], [53, 25],
    [50, 29], [48, 30], [43, 40], [40, 41], [36, 36], [32, 31], [28, 36],
    [24, 40], [19, 40], [14, 38], [12, 45], [8, 44], [3, 43], [-5, 36],
  ],
  // Great Britain
  [[-5, 50], [-2, 51], [0, 53], [-1, 55], [-3, 58], [-5, 58], [-6, 55], [-5, 52]],
  // Japan
  [[130, 31], [135, 34], [140, 38], [142, 42], [145, 44], [141, 45], [139, 40], [135, 35], [131, 33]],
  // Sumatra / Java / Borneo (indicative)
  [[95, 5], [103, -2], [106, -6], [113, -8], [117, -3], [117, 2], [110, 2], [100, 4]],
  // Australia
  [
    [113, -22], [114, -26], [116, -32], [119, -34], [125, -32], [131, -31],
    [137, -33], [140, -38], [146, -39], [150, -37], [153, -30], [153, -25],
    [149, -20], [146, -18], [142, -11], [136, -12], [132, -11], [130, -13],
    [125, -14], [122, -17], [117, -20],
  ],
  // New Zealand
  [[166, -46], [171, -44], [174, -41], [178, -38], [176, -37], [173, -40], [168, -44]],
];

/// Internal country borders as [lon, lat] polylines — drawn thinner/dimmer than
/// the coastline so the map reads as countries, not just continents.
const List<List<List<double>>> _worldBorders = [
  // North America
  [[-123, 49], [-95, 49], [-89, 48], [-83, 42], [-79, 43], [-74, 45], [-69, 47]],
  [[-117, 32], [-111, 31], [-106, 31], [-103, 29], [-99, 27], [-97, 26]],
  [[-141, 70], [-141, 60]],
  // South America
  [[-70, -10], [-65, -10], [-60, -13], [-58, -20], [-55, -24], [-54, -26]],
  [[-70, -18], [-69, -24], [-70, -30], [-71, -37], [-72, -45], [-72, -52]],
  [[-73, 10], [-70, 7], [-67, 6], [-63, 4]],
  [[-69, -14], [-65, -18], [-58, -20]],
  // Europe
  [[-1, 43], [3, 42]],
  [[8, 49], [7, 47]],
  [[14, 54], [15, 51], [19, 49]],
  [[7, 45], [13, 46]],
  [[12, 59], [15, 66], [21, 69]],
  [[24, 50], [30, 52], [33, 52]],
  [[20, 45], [28, 44]],
  // Africa
  [[-12, 22], [5, 22], [16, 22], [25, 22], [34, 22]],
  [[0, 15], [1, 6]],
  [[15, 21], [14, 6]],
  [[12, -5], [22, -5], [29, -5]],
  [[20, -22], [26, -22], [32, -22]],
  [[33, 10], [37, 8]],
  [[25, 22], [25, 10]],
  [[30, -1], [30, -12]],
  // Asia
  [[80, 50], [95, 50], [110, 50], [120, 49], [130, 45]],
  [[76, 35], [81, 30], [88, 28], [95, 29]],
  [[70, 24], [72, 28], [74, 32]],
  [[90, 45], [105, 42], [118, 45]],
  [[50, 45], [62, 45], [72, 43], [80, 45]],
  [[61, 35], [61, 29]],
  [[44, 37], [46, 33], [48, 30]],
  [[40, 32], [48, 30]],
  [[126, 38], [130, 39]],
  [[100, 20], [105, 15], [107, 11]],
  // Australia
  [[129, -14], [129, -31]],
  [[141, -11], [141, -29]],
  [[138, -26], [129, -26]],
];

/// Metro clusters that actually glow on a night-side photo, as [lon, lat].
const List<List<double>> _cityLights = [
  [-122, 37], [-118, 34], [-112, 33], [-96, 33], [-87, 42], [-84, 34],
  [-80, 26], [-77, 39], [-74, 41], [-71, 42], [-79, 44], [-99, 19],
  [-70, -34], [-58, -35], [-47, -24], [-43, -23], [-74, 5],
  [-3, 40], [2, 49], [0, 52], [-6, 53], [7, 51], [13, 53], [16, 50],
  [12, 42], [23, 38], [28, 41], [30, 60], [37, 56], [31, 30], [3, 37],
  [35, 32], [44, 33], [51, 36], [55, 25], [47, 24],
  [77, 29], [72, 19], [80, 13], [88, 23], [67, 25],
  [100, 14], [104, 1], [107, -6], [121, 14], [117, 32], [121, 31],
  [113, 23], [114, 40], [126, 38], [127, 37], [135, 35], [140, 36],
  [151, -34], [145, -38], [153, -27], [175, -37],
  [31, -26], [3, 6], [7, 9], [39, -6],
];

class _NightEarthPainter extends CustomPainter {
  final double t;
  _NightEarthPainter(this.t);

  static const double _latN = 80, _latS = -58;

  @override
  void paint(Canvas canvas, Size size) {
    // The map keeps its NATURAL proportions (lon:lat ≈ 2.6:1). Stretching it to
    // this card's ~6:1 shape squashed latitude ~3x and smeared the continents
    // into one blob, so instead it is sized by HEIGHT (full 80N..58S visible)
    // and anchored right; the form occupies the left, over the scrim.
    const bleed = 8.0;
    final mapH = size.height + bleed * 2;
    final mapW = mapH * (360 / (_latN - _latS)); // no distortion
    final top = -bleed;
    // Anchor right, nudged so Asia/Australia don't fall off the edge.
    final left = size.width - mapW * 0.97;

    Offset proj(double lon, double lat) => Offset(
          left + (lon + 180) / 360 * mapW,
          top + (_latN - lat) / (_latN - _latS) * mapH,
        );

    canvas.save();
    canvas.clipRect(Offset.zero & size);

    // Deep-space wash behind the continents.
    canvas.drawRect(
      Offset.zero & size,
      Paint()
        ..shader = const RadialGradient(
          center: Alignment(0.45, 0),
          radius: 0.95,
          colors: [Color(0x2EFF7A28), Color(0x00FF7A28)],
        ).createShader(Offset.zero & size),
    );

    // Landmass outline path (shared by the extrusion + the top face).
    final land = Path();
    for (final ring in _worldLand) {
      if (ring.isEmpty) continue;
      final sub = Path()..moveTo(proj(ring[0][0], ring[0][1]).dx,
          proj(ring[0][0], ring[0][1]).dy);
      for (var i = 1; i < ring.length; i++) {
        final p = proj(ring[i][0], ring[i][1]);
        sub.lineTo(p.dx, p.dy);
      }
      sub.close();
      land.addPath(sub, Offset.zero);
    }

    // Landmasses GLOW like the night-from-space reference: a wide soft bloom
    // under the shape, then the body, then a hot lit coastline.
    canvas.drawPath(
        land,
        Paint()
          ..color = const Color(0x2EFF6B00)
          ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 14));
    canvas.drawPath(
        land,
        Paint()
          ..shader = const LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Color(0x5CFF8A3D), Color(0x47E05A14)],
          ).createShader(Offset.zero & size));
    canvas.drawPath(
        land,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.0
          ..color = const Color(0x8CFFC08A));

    // Internal country borders (thin, dim — reads as countries, not continents).
    final borderPaint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 0.7
      ..color = const Color(0x4DFFB889);
    for (final line in _worldBorders) {
      if (line.length < 2) continue;
      final path = Path();
      final p0 = proj(line[0][0], line[0][1]);
      path.moveTo(p0.dx, p0.dy);
      for (var i = 1; i < line.length; i++) {
        final p = proj(line[i][0], line[i][1]);
        path.lineTo(p.dx, p.dy);
      }
      canvas.drawPath(path, borderPaint);
    }

    // City lights sit ON the raised top face (lifted by the extrusion height so
    // they don't look sunk into the walls).
    for (var i = 0; i < _cityLights.length; i++) {
      final c = _cityLights[i];
      final p = proj(c[0], c[1]) - const Offset(0, 1.5);
      final tw = 0.7 + 0.3 * math.sin(t * 2 * math.pi + i * 0.8);
      canvas.drawCircle(
          p,
          4.2,
          Paint()
            ..color = Color.fromRGBO(255, 150, 60, 0.24 * tw)
            ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 3));
      canvas.drawCircle(
          p, 1.4, Paint()..color = Color.fromRGBO(255, 210, 160, 0.98 * tw));
    }

    // Readability scrim: the form sits on the left, so fade the card colour
    // across the left ~55% — the map stays vivid on the right, text stays sharp.
    canvas.drawRect(
      Offset.zero & size,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.centerLeft,
          end: Alignment.centerRight,
          colors: [
            AppColors.surface,
            AppColors.surface.withValues(alpha: 0.92),
            AppColors.surface.withValues(alpha: 0.35),
            AppColors.surface.withValues(alpha: 0.0),
          ],
          stops: const [0.0, 0.34, 0.56, 0.80],
        ).createShader(Offset.zero & size),
    );

    canvas.restore();
  }

  @override
  bool shouldRepaint(_NightEarthPainter old) => old.t != t;
}

class CommandNavRail extends StatefulWidget {
  final List<NavRailItem> items;
  final int selected;
  final bool online;
  final ValueChanged<int> onSelect;
  final RemoteService service;
  const CommandNavRail({
    super.key,
    required this.items,
    required this.selected,
    required this.online,
    required this.onSelect,
    required this.service,
  });

  @override
  State<CommandNavRail> createState() => _CommandNavRailState();
}

class _CommandNavRailState extends State<CommandNavRail> {
  static const bool _open = true; // always expanded (mockup)

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 236,
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border(right: BorderSide(color: AppColors.border)),
      ),
      clipBehavior: Clip.hardEdge,
      child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // brand
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 20, 18, 16),
              child: Row(children: [
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [AppColors.primary, AppColors.primaryDark],
                    ),
                    borderRadius: BorderRadius.circular(11),
                    boxShadow: [
                      BoxShadow(
                          color: AppColors.primary.withValues(alpha: 0.35),
                          blurRadius: 12,
                          offset: const Offset(0, 4)),
                    ],
                  ),
                  alignment: Alignment.center,
                  child: const Text('N',
                      style: TextStyle(
                          color: Colors.white,
                          fontWeight: FontWeight.w800,
                          fontSize: 17)),
                ),
                const SizedBox(width: 11),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('Neev Remote',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style:
                              AppTypography.sectionTitle.copyWith(fontSize: 15)),
                      Text('Global Remote Access',
                          style: AppTypography.meta.copyWith(fontSize: 10)),
                    ],
                  ),
                ),
              ]),
            ),
            // nav
            Expanded(
              child: ListView(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                children: [
                  for (var i = 0; i < widget.items.length; i++)
                    _RailItem(
                      item: widget.items[i],
                      active: i == widget.selected,
                      open: _open,
                      onTap: () => widget.onSelect(i),
                    ),
                ],
              ),
            ),
            // this-device (own id + password) — real data
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 4, 12, 12),
              child: _ThisDeviceCard(service: widget.service),
            ),
          ],
        ),
    );
  }
}

class _RailItem extends StatefulWidget {
  final NavRailItem item;
  final bool active;
  final bool open;
  final VoidCallback onTap;
  const _RailItem({
    required this.item,
    required this.active,
    required this.open,
    required this.onTap,
  });
  @override
  State<_RailItem> createState() => _RailItemState();
}

class _RailItemState extends State<_RailItem> {
  bool _hover = false;
  @override
  Widget build(BuildContext context) {
    final active = widget.active;
    final fg = active ? AppColors.primaryDark : AppColors.textSecondary;
    final bg = active
        ? AppColors.primarySoft
        : (_hover ? AppColors.surfaceLight : Colors.transparent);
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: widget.onTap,
        child: Tooltip(
          message: widget.open ? '' : widget.item.label,
          waitDuration: const Duration(milliseconds: 400),
          child: Stack(children: [
            AnimatedContainer(
              duration: const Duration(milliseconds: 120),
              height: 44,
              margin: const EdgeInsets.symmetric(vertical: 2),
              padding: const EdgeInsets.symmetric(horizontal: 15),
              decoration: BoxDecoration(
                color: bg,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Row(children: [
                Icon(widget.item.icon, size: 20, color: fg),
                if (widget.open) ...[
                  const SizedBox(width: 14),
                  Expanded(
                    child: Text(widget.item.label,
                        maxLines: 1,
                        overflow: TextOverflow.clip,
                        softWrap: false,
                        style: AppTypography.caption.copyWith(
                            fontSize: 13.5,
                            color: fg,
                            fontWeight:
                                active ? FontWeight.w600 : FontWeight.w500)),
                  ),
                ],
              ]),
            ),
            if (active)
              Positioned(
                left: 0,
                top: 11,
                bottom: 11,
                child: Container(
                  width: 3,
                  decoration: BoxDecoration(
                    color: AppColors.primary,
                    borderRadius: BorderRadius.horizontal(right: Radius.circular(3)),
                  ),
                ),
              ),
          ]),
        ),
      ),
    );
  }
}

/// Command Center — Home workspace (DESIGN.md 2026-07-21 redesign).
/// Connection dock → status strip → Your Devices grid → recent activity.
/// Wired to the same providers as the old dashboard; renders only real data
/// (Data Honesty Rule): no fabricated latency/FPS/bandwidth.
class HomeCommandCenter extends ConsumerStatefulWidget {
  final RemoteService service;
  final TextEditingController idController;
  final TextEditingController passwordController;
  final VoidCallback onConnect;
  final void Function(String id) onPick;
  final VoidCallback onOpenSettings;

  const HomeCommandCenter({
    super.key,
    required this.service,
    required this.idController,
    required this.passwordController,
    required this.onConnect,
    required this.onPick,
    required this.onOpenSettings,
  });

  @override
  ConsumerState<HomeCommandCenter> createState() => _HomeCommandCenterState();
}

/// A device row unified from recents + relay peers + LAN discovery + favorites.
class _HomeDevice {
  final String id;
  final String name;
  final String os;
  final bool online;
  final bool favorite;
  final DateTime? lastConnected;
  final String? thumbPath; // last captured remote frame, if any
  _HomeDevice(this.id, this.name, this.os, this.online, this.favorite,
      this.lastConnected, this.thumbPath);
}

enum _Tab { pinned, online, recent, offline, all }

class _HomeCommandCenterState extends ConsumerState<HomeCommandCenter> {
  _Tab _tab = _Tab.all;

  String _norm(String s) => s.replaceAll(RegExp(r'[^0-9a-zA-Z]'), '');

  List<_HomeDevice> _devices() {
    final service = widget.service;
    final recents = ref.watch(recentConnectionsProvider);
    final book = ref.watch(addressBookProvider);
    final disc = ref.watch(discoveryProvider).devices;

    final favs = <String>{for (final e in book.where((e) => e.favorite)) _norm(e.id)};
    final online = <String, DiscoveredDevice>{};
    for (final d in service.serverPeers) {
      online[_norm(d.id)] = d;
    }
    for (final d in disc) {
      online.putIfAbsent(_norm(d.id), () => d);
    }

    final map = <String, _HomeDevice>{};
    void put(String id, String name, String os, DateTime? last) {
      final k = _norm(id);
      if (k.isEmpty) return;
      final existing = map[k];
      final on = online.containsKey(k);
      final o = online[k];
      map[k] = _HomeDevice(
        id,
        (name.isNotEmpty ? name : o?.name ?? id),
        (o?.os.isNotEmpty == true ? o!.os : os),
        on,
        favs.contains(k),
        last ?? existing?.lastConnected,
        service.thumbPathFor(id),
      );
    }

    for (final r in recents) {
      put(r.id, r.name, '', r.lastConnected);
    }
    for (final d in online.values) {
      put(d.id, d.name, d.os, map[_norm(d.id)]?.lastConnected);
    }
    for (final e in book) {
      put(e.id, e.name, '', map[_norm(e.id)]?.lastConnected);
    }

    final list = map.values.toList();
    list.sort((a, b) {
      if (a.online != b.online) return a.online ? -1 : 1;
      final la = a.lastConnected, lb = b.lastConnected;
      if (la != null && lb != null) return lb.compareTo(la);
      if (la != null) return -1;
      if (lb != null) return 1;
      return a.name.toLowerCase().compareTo(b.name.toLowerCase());
    });
    return list;
  }

  List<_HomeDevice> _filtered(List<_HomeDevice> all) {
    switch (_tab) {
      case _Tab.pinned:
        return all.where((d) => d.favorite).toList();
      case _Tab.online:
        return all.where((d) => d.online).toList();
      case _Tab.recent:
        return all.where((d) => d.lastConnected != null).toList();
      case _Tab.offline:
        return all.where((d) => !d.online).toList();
      case _Tab.all:
        return all;
    }
  }

  @override
  Widget build(BuildContext context) {
    final service = widget.service;
    final all = _devices();
    final onlineCount = all.where((d) => d.online).length;
    final activeXfer = service.fileTransfers
        .where((t) => t.status == FileStatus.active || t.status == FileStatus.sent)
        .length;
    final sharing = service.hostStatus == HostStatus.online;
    final unattended = ref.watch(settingsProvider).unattendedEnabled;

    return ListView(
      padding: const EdgeInsets.fromLTRB(30, 26, 30, 40),
      children: [
        _StartConnectionCard(
          idController: widget.idController,
          passwordController: widget.passwordController,
          onConnect: widget.onConnect,
          recents: ref.watch(recentConnectionsProvider).take(4).toList(),
          onPick: widget.onPick,
          onClear: () => ref.read(recentConnectionsProvider.notifier).clear(),
        ),
        const SizedBox(height: 26),
        _SectionHead(
          title: 'Your devices',
          tabs: [
            for (final t in _Tab.values)
              _TabSpec(_tabLabel(t), t == _tab, () => setState(() => _tab = t),
                  _tabCount(t, all)),
          ],
        ),
        const SizedBox(height: 16),
        _DeviceGrid(
          devices: _filtered(all),
          onPick: widget.onPick,
          onToggleFav: (id) =>
              ref.read(addressBookProvider.notifier).toggleFavorite(id),
        ),
        const SizedBox(height: 28),
        _BottomPanels(
          recents: ref.watch(recentConnectionsProvider),
          favorites: all.where((d) => d.favorite).toList(),
          onPick: widget.onPick,
          unattended: ref.watch(settingsProvider).unattendedEnabled,
          onOpenSettings: widget.onOpenSettings,
          onInvite: () => _shareInvite(context),
          onHelp: () => _showHelp(context),
        ),
      ],
    );
  }

  /// Invite = share THIS device's real credentials so someone can connect to
  /// it. Uses the live id/password from the transport (same values the sidebar
  /// shows) — nothing invented, no account system implied.
  Future<void> _shareInvite(BuildContext context) async {
    final id = widget.service.agentId;
    final pw = widget.service.password;
    if (id == null || id.isEmpty) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
          content: Text('This device has no ID yet — start hosting first.'),
          duration: Duration(seconds: 3)));
      return;
    }
    final text = 'Connect to my computer with Neev Remote\n'
        'Device ID: ${_group(id)}\n'
        '${(pw != null && pw.isNotEmpty) ? 'Password: $pw\n' : ''}'
        'Download: http://172.17.17.77:8080';
    await Clipboard.setData(ClipboardData(text: text));
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
        content: Text('Invite copied — paste it to share this device'),
        duration: Duration(seconds: 3)));
  }

  /// Help = the real facts we have (build stamp, relay, log location), not a
  /// dead link to a support site that does not exist.
  void _showHelp(BuildContext context) {
    final relay = ref.read(settingsProvider).relayUrl;
    showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: AppColors.surface,
        title: Text('Help & Support', style: AppTypography.heading2),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Build: ${AppConstants.buildTag}',
                style: AppTypography.caption.copyWith(fontSize: 12.5)),
            const SizedBox(height: 8),
            Text('Relay: $relay',
                style: AppTypography.caption.copyWith(fontSize: 12.5)),
            const SizedBox(height: 8),
            Text('Downloads: http://172.17.17.77:8080',
                style: AppTypography.caption.copyWith(fontSize: 12.5)),
            const SizedBox(height: 8),
            Text(
                'Logs: %ProgramData%\\NeevRemote (Windows) · '
                '~/.neev_remote (macOS)',
                style: AppTypography.caption.copyWith(fontSize: 12.5)),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () async {
              await Clipboard.setData(ClipboardData(
                  text: 'Neev Remote ${AppConstants.buildTag}\nRelay: $relay'));
              if (ctx.mounted) Navigator.pop(ctx);
            },
            child: const Text('Copy details'),
          ),
          TextButton(
              onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
        ],
      ),
    );
  }

  String _tabLabel(_Tab t) => switch (t) {
        _Tab.pinned => 'Pinned',
        _Tab.online => 'Online',
        _Tab.recent => 'Recent',
        _Tab.offline => 'Offline',
        _Tab.all => 'All',
      };

  int? _tabCount(_Tab t, List<_HomeDevice> all) => switch (t) {
        _Tab.pinned => all.where((d) => d.favorite).length,
        _Tab.online => all.where((d) => d.online).length,
        _Tab.offline => all.where((d) => !d.online).length,
        _Tab.all => all.length,
        _Tab.recent => null,
      };
}

/// A barely-perceptible vertical breathing motion (premium idle depth). Honours
/// the OS reduce-motion setting.
class _IdleFloat extends StatefulWidget {
  final Widget child;
  final double amplitude;
  const _IdleFloat({required this.child, this.amplitude = 2.5});
  @override
  State<_IdleFloat> createState() => _IdleFloatState();
}

class _IdleFloatState extends State<_IdleFloat>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c;
  @override
  void initState() {
    super.initState();
    _c = AnimationController(vsync: this, duration: const Duration(seconds: 5))
      ..repeat(reverse: true);
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (MediaQuery.of(context).disableAnimations) return widget.child;
    return AnimatedBuilder(
      animation: _c,
      builder: (_, child) {
        final t = Curves.easeInOut.transform(_c.value);
        return Transform.translate(
            offset: Offset(0, (t - 0.5) * 2 * widget.amplitude), child: child);
      },
      child: widget.child,
    );
  }
}

// ---------------------------------------------------------------- dock

class _ConnectionDock extends StatelessWidget {
  final TextEditingController idController;
  final TextEditingController passwordController;
  final VoidCallback onConnect;
  final List<RecentConnection> recents;
  final void Function(String id) onPick;
  const _ConnectionDock({
    required this.idController,
    required this.passwordController,
    required this.onConnect,
    required this.recents,
    required this.onPick,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(30, 28, 30, 26),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadii.panel),
        border: Border.all(color: AppColors.border),
        boxShadow: AppShadows.dock,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Connect securely to another device',
              style: AppTypography.pageTitle.copyWith(fontSize: 22)),
          const SizedBox(height: 5),
          Text('Access, support, transfer files or collaborate in real time.',
              style: AppTypography.body.copyWith(color: AppColors.textSecondary)),
          const SizedBox(height: 20),
          _DockField(
            controller: idController,
            hint: 'Remote ID, device name or contact',
            icon: Icons.devices_rounded,
            mono: true,
            onSubmitted: (_) => onConnect(),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                flex: 3,
                child: _DockField(
                  controller: passwordController,
                  hint: 'Password / Access key',
                  icon: Icons.lock_outline_rounded,
                  obscure: true,
                  onSubmitted: (_) => onConnect(),
                ),
              ),
              const SizedBox(width: 12),
              const Expanded(flex: 2, child: _ModeSelector()),
              const SizedBox(width: 12),
              _ConnectButton(onTap: onConnect),
            ],
          ),
          if (recents.isNotEmpty) ...[
            const SizedBox(height: 16),
            Row(children: [
              Text('Recent',
                  style: AppTypography.label
                      .copyWith(fontSize: 11.5, color: AppColors.textTertiary)),
              const SizedBox(width: 8),
              ...recents.map((r) => Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: _RecentChip(name: r.name, onTap: () => onPick(r.id)),
                  )),
            ]),
          ],
        ],
      ),
    );
  }
}

class _DockField extends StatelessWidget {
  final TextEditingController controller;
  final String hint;
  final IconData icon;
  final bool mono;
  final bool obscure;
  final ValueChanged<String>? onSubmitted;
  const _DockField({
    required this.controller,
    required this.hint,
    required this.icon,
    this.mono = false,
    this.obscure = false,
    this.onSubmitted,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 52,
      padding: const EdgeInsets.symmetric(horizontal: 16),
      decoration: BoxDecoration(
        color: AppColors.surfaceLight,
        borderRadius: BorderRadius.circular(AppRadii.lg),
        border: Border.all(color: AppColors.borderStrong),
      ),
      child: Row(children: [
        Icon(icon, size: 18, color: AppColors.textTertiary),
        const SizedBox(width: 11),
        Expanded(
          child: TextField(
            controller: controller,
            obscureText: obscure,
            onSubmitted: onSubmitted,
            style: mono
                ? AppTypography.idLarge.copyWith(fontSize: 16, letterSpacing: 1.5)
                : AppTypography.body.copyWith(fontSize: 15),
            decoration: InputDecoration(
              hintText: hint,
              hintStyle:
                  AppTypography.body.copyWith(color: AppColors.textTertiary),
              border: InputBorder.none,
              enabledBorder: InputBorder.none,
              focusedBorder: InputBorder.none,
              isCollapsed: true,
              filled: false,
            ),
          ),
        ),
      ]),
    );
  }
}

/// Session mode. ONLY the two modes the backend actually implements:
/// Full Control and View Only (bound to the real `viewOnly` setting, which
/// gates input in remote_view_widget). 'File Transfer' / 'Privacy Mode' /
/// 'Support Mode' were removed — they had no backend and would have been a
/// dropdown that silently does nothing (Data Honesty Rule).
class _ModeSelector extends ConsumerWidget {
  const _ModeSelector();
  static const _full = 'Full Control';
  static const _view = 'View Only';

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final viewOnly = ref.watch(settingsProvider).viewOnly;
    final mode = viewOnly ? _view : _full;
    return PopupMenuButton<String>(
      initialValue: mode,
      onSelected: (v) {
        final wantViewOnly = v == _view;
        if (wantViewOnly != viewOnly) {
          ref.read(settingsProvider.notifier).toggleViewOnly();
        }
      },
      offset: const Offset(0, 54),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadii.lg),
        side: BorderSide(color: AppColors.border),
      ),
      color: AppColors.surface,
      itemBuilder: (_) => [
        for (final m in const [_full, _view])
          PopupMenuItem(
              value: m,
              height: 40,
              child: Row(children: [
                Icon(
                    m == _view
                        ? Icons.visibility_outlined
                        : Icons.mouse_outlined,
                    size: 15,
                    color: AppColors.textSecondary),
                const SizedBox(width: 9),
                Text(m,
                    style: AppTypography.body.copyWith(fontSize: 13.5)),
              ])),
      ],
      child: Container(
        height: 56,
        padding: const EdgeInsets.symmetric(horizontal: 13),
        decoration: BoxDecoration(
          color: AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(AppRadii.md),
          border: Border.all(color: AppColors.borderStrong),
        ),
        child: Row(children: [
          Expanded(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text('MODE',
                    style: AppTypography.microLabel.copyWith(
                        fontSize: 9.5, letterSpacing: 0.8, height: 1.0)),
                const SizedBox(height: 4),
                Text(mode,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AppTypography.bodyStrong
                        .copyWith(fontSize: 13.5, height: 1.0)),
              ],
            ),
          ),
          Icon(Icons.keyboard_arrow_down_rounded,
              size: 18, color: AppColors.textTertiary),
        ]),
      ),
    );
  }
}

class _ConnectButton extends StatefulWidget {
  final VoidCallback onTap;
  const _ConnectButton({required this.onTap});
  @override
  State<_ConnectButton> createState() => _ConnectButtonState();
}

class _ConnectButtonState extends State<_ConnectButton> {
  bool _hover = false;
  bool _down = false;
  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTapDown: (_) => setState(() => _down = true),
        onTapUp: (_) => setState(() => _down = false),
        onTapCancel: () => setState(() => _down = false),
        onTap: widget.onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 160),
          curve: Curves.easeOutCubic,
          height: 52,
          padding: const EdgeInsets.symmetric(horizontal: 24),
          transform: Matrix4.translationValues(
              0, _down ? 0 : (_hover ? -2 : 0), 0)
            ..scaleByDouble(
                _down ? 0.97 : 1.0, _down ? 0.97 : 1.0, 1.0, 1.0),
          transformAlignment: Alignment.center,
          decoration: BoxDecoration(
            color: _hover ? AppColors.primaryDark : AppColors.primary,
            borderRadius: BorderRadius.circular(AppRadii.lg),
            boxShadow: [
              BoxShadow(
                color: AppColors.primary.withValues(alpha: _hover ? 0.45 : 0.3),
                blurRadius: _hover ? 22 : 14,
                offset: Offset(0, _hover ? 10 : 6),
              ),
            ],
          ),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            const Text('Connect',
                style: TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.w700,
                    fontSize: 15)),
            const SizedBox(width: 9),
            AnimatedSlide(
              duration: const Duration(milliseconds: 160),
              offset: Offset(_hover ? 0.25 : 0, 0),
              child: const Icon(Icons.arrow_forward_rounded,
                  color: Colors.white, size: 19),
            ),
          ]),
        ),
      ),
    );
  }
}

class _RecentChip extends StatelessWidget {
  final String name;
  final VoidCallback onTap;
  const _RecentChip({required this.name, required this.onTap});
  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(AppRadii.xl),
      onTap: onTap,
      child: Container(
        height: 30,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(
          color: AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(999),
          border: Border.all(color: AppColors.border),
        ),
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          Container(
              width: 7,
              height: 7,
              decoration: BoxDecoration(
                  color: AppColors.success, shape: BoxShape.circle)),
          const SizedBox(width: 7),
          Text(name,
              style: AppTypography.caption.copyWith(
                  fontSize: 12.5, fontWeight: FontWeight.w600)),
        ]),
      ),
    );
  }
}

// ---------------------------------------------------------------- status strip

class _StatusStrip extends StatelessWidget {
  final int onlineCount;
  final int knownCount;
  final int activeXfer;
  final bool sharing;
  final bool unattended;
  final int connectedViewers;
  const _StatusStrip({
    required this.onlineCount,
    required this.knownCount,
    required this.activeXfer,
    required this.sharing,
    required this.unattended,
    required this.connectedViewers,
  });

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, c) {
      // Reflow instead of clipping: 6 across when wide, 3 when medium, 2 when
      // narrow (the activity panel can squeeze the workspace on smaller desktops).
      final perRow = c.maxWidth >= 900 ? 6 : (c.maxWidth >= 540 ? 3 : 2);
      const gap = 12.0;
      final w = (c.maxWidth - gap * (perRow - 1)) / perRow;
      return Wrap(
        spacing: gap,
        runSpacing: gap,
        children: [
          SizedBox(
            width: w,
            child: FutureBuilder<int>(
              future: AuditLog.instance.countToday(),
              builder: (_, snap) => _Stat(
                  icon: Icons.schedule_rounded,
                  tint: AppColors.primary,
                  value: '${snap.data ?? 0}',
                  label: 'Sessions today'),
            ),
          ),
          SizedBox(
              width: w,
              child: _Stat(
                  icon: Icons.circle,
                  tint: AppColors.success,
                  value: '$onlineCount',
                  label: 'Online devices',
                  valueColor: AppColors.success)),
          SizedBox(
              width: w,
              child: _Stat(
                  icon: Icons.dns_rounded,
                  tint: AppColors.secondary,
                  value: '$knownCount',
                  label: 'Known devices')),
          SizedBox(
              width: w,
              child: _Stat(
                  icon: Icons.swap_vert_rounded,
                  tint: AppColors.success,
                  value: '$activeXfer',
                  label: 'Active transfers')),
          SizedBox(
              width: w,
              child: _Stat(
                  icon: Icons.podcasts_rounded,
                  tint: AppColors.primary,
                  value: sharing ? 'On' : 'Off',
                  label: connectedViewers > 0
                      ? '$connectedViewers connected'
                      : 'Sharing')),
          SizedBox(
              width: w,
              child: _Stat(
                  icon: Icons.flag_rounded,
                  tint: AppColors.warning,
                  value: unattended ? 'On' : 'Off',
                  label: 'Unattended')),
        ],
      );
    });
  }
}

class _Stat extends StatelessWidget {
  final IconData icon;
  final Color tint;
  final String value;
  final String label;
  final Color? valueColor;
  const _Stat({
    required this.icon,
    required this.tint,
    required this.value,
    required this.label,
    this.valueColor,
  });
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadii.lg),
        border: Border.all(color: AppColors.border),
        boxShadow: AppShadows.card,
      ),
      child: Row(children: [
        Container(
          width: 36,
          height: 36,
          decoration: BoxDecoration(
            color: tint.withValues(alpha: 0.14),
            borderRadius: BorderRadius.circular(AppRadii.md),
          ),
          child: Icon(icon, size: 16, color: tint),
        ),
        const SizedBox(width: 11),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text(value,
                  style: AppTypography.mono.copyWith(
                      fontSize: 18,
                      fontWeight: FontWeight.w600,
                      color: valueColor ?? AppColors.textPrimary)),
              Text(label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: AppTypography.caption.copyWith(fontSize: 11)),
            ],
          ),
        ),
      ]),
    );
  }
}

// ---------------------------------------------------------------- section head

class _TabSpec {
  final String label;
  final bool active;
  final VoidCallback onTap;
  final int? count;
  _TabSpec(this.label, this.active, this.onTap, this.count);
}

class _SectionHead extends StatelessWidget {
  final String title;
  final List<_TabSpec> tabs;
  const _SectionHead({required this.title, required this.tabs});
  @override
  Widget build(BuildContext context) {
    return Row(children: [
      Text(title, style: AppTypography.sectionTitle),
      const SizedBox(width: 14),
      ...tabs.map((t) => Padding(
            padding: const EdgeInsets.only(right: 4),
            child: _TabPill(t),
          )),
    ]);
  }
}

class _TabPill extends StatelessWidget {
  final _TabSpec spec;
  const _TabPill(this.spec);
  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(999),
      onTap: spec.onTap,
      child: Container(
        height: 30,
        padding: const EdgeInsets.symmetric(horizontal: 13),
        decoration: BoxDecoration(
          color: spec.active ? AppColors.textPrimary : Colors.transparent,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          Text(spec.label,
              style: AppTypography.caption.copyWith(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w600,
                  color:
                      spec.active ? AppColors.surface : AppColors.textSecondary)),
          if (spec.count != null) ...[
            const SizedBox(width: 5),
            Text('${spec.count}',
                style: AppTypography.mono.copyWith(
                    fontSize: 11,
                    color: spec.active
                        ? AppColors.surface.withValues(alpha: 0.7)
                        : AppColors.textTertiary)),
          ],
        ]),
      ),
    );
  }
}

// ---------------------------------------------------------------- device grid

class _DeviceGrid extends StatelessWidget {
  final List<_HomeDevice> devices;
  final void Function(String id) onPick;
  final void Function(String id) onToggleFav;
  const _DeviceGrid(
      {required this.devices, required this.onPick, required this.onToggleFav});

  @override
  Widget build(BuildContext context) {
    if (devices.isEmpty) {
      return Container(
        height: 180,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(AppRadii.card),
          border: Border.all(color: AppColors.border),
        ),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          Icon(Icons.devices_other_rounded,
              size: 30, color: AppColors.textTertiary),
          const SizedBox(height: 10),
          Text('No devices here yet',
              style: AppTypography.body.copyWith(color: AppColors.textSecondary)),
          const SizedBox(height: 4),
          Text('Connect to a device or wait for one to come online.',
              style: AppTypography.caption),
        ]),
      );
    }
    return LayoutBuilder(builder: (context, c) {
      // Smaller cards, more per row: aim for ~240px wide, 2–6 columns.
      final cols = (c.maxWidth / 244).floor().clamp(2, 6);
      const gap = 16.0;
      final w = (c.maxWidth - gap * (cols - 1)) / cols;
      return Wrap(
        spacing: gap,
        runSpacing: gap,
        children: [
          for (final d in devices)
            SizedBox(
                width: w,
                child: _DeviceCard(
                    device: d, onPick: onPick, onToggleFav: onToggleFav)),
        ],
      );
    });
  }
}

List<Color> _grounds = [
  AppColors.deviceNavy,
  AppColors.deviceForest,
  AppColors.devicePlum,
  AppColors.deviceWalnut,
];

const List<Color> _deviceTints = [
  Color(0xFF4C9AFF),
  Color(0xFF36B37E),
  Color(0xFF9F7AEA),
  Color(0xFFFF8B3D),
  Color(0xFFF06A6A),
  Color(0xFF2DD4BF),
];

IconData _deviceGlyph(String os) {
  final o = os.toLowerCase();
  if (o.contains('mac') || o.contains('darwin')) return Icons.laptop_mac_rounded;
  if (o.contains('linux') || o.contains('server')) return Icons.dns_rounded;
  return Icons.desktop_windows_rounded;
}

class _DeviceCard extends StatefulWidget {
  final _HomeDevice device;
  final void Function(String id) onPick;
  final void Function(String id) onToggleFav;
  const _DeviceCard(
      {required this.device, required this.onPick, required this.onToggleFav});
  @override
  State<_DeviceCard> createState() => _DeviceCardState();
}

class _DeviceCardState extends State<_DeviceCard> {
  bool _hover = false;
  Offset _tilt = Offset.zero; // -0.5..0.5

  void _onHover(PointerHoverEvent e) {
    final box = context.findRenderObject() as RenderBox?;
    if (box == null) return;
    final local = box.globalToLocal(e.position);
    setState(() {
      _tilt = Offset(
        (local.dx / box.size.width - 0.5).clamp(-0.5, 0.5),
        (local.dy / box.size.height - 0.5).clamp(-0.5, 0.5),
      );
    });
  }

  Color get _ground {
    final h = widget.device.id.codeUnits.fold<int>(0, (a, b) => a + b);
    return _grounds[h % _grounds.length];
  }

  /// Placeholder when there's no screenshot yet: a LIGHT tinted panel (not a
  /// heavy dark ground) with a small, subtly-tilting device icon — keeps the
  /// grid calm so the real screenshots stand out.
  Widget _placeholder(_HomeDevice d) {
    final g = _ground;
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            Color.alphaBlend(g.withValues(alpha: 0.07), AppColors.surfaceLight),
            Color.alphaBlend(g.withValues(alpha: 0.18), AppColors.surfaceLight),
          ],
        ),
      ),
      child: Center(
        child: Transform(
          alignment: Alignment.center,
          transform: Matrix4.identity()
            ..setEntry(3, 2, 0.0012)
            ..rotateY(_tilt.dx * 0.3)
            ..rotateX(-_tilt.dy * 0.24),
          child: Icon(_glyph(d.os), size: 36, color: g.withValues(alpha: 0.55)),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final d = widget.device;
    final tint = _deviceTints[
        d.id.codeUnits.fold<int>(0, (a, b) => a + b) % _deviceTints.length];
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: () => widget.onPick(d.id), // CONNECT — audit: connectToHost
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 150),
          transformAlignment: Alignment.center,
          transform: Matrix4.translationValues(0, _hover ? -3 : 0, 0),
          padding: const EdgeInsets.fromLTRB(16, 14, 16, 15),
          decoration: BoxDecoration(
            color: AppColors.surface,
            borderRadius: BorderRadius.circular(AppRadii.card),
            border: Border.all(
                color: _hover
                    ? AppColors.primary.withValues(alpha: 0.5)
                    : AppColors.border),
            boxShadow: _hover ? AppShadows.cardHover : AppShadows.card,
          ),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Row(children: [
              Container(
                width: 8,
                height: 8,
                decoration: BoxDecoration(
                    color: d.online ? AppColors.success : AppColors.error,
                    shape: BoxShape.circle),
              ),
              const SizedBox(width: 7),
              Text(d.online ? 'Online' : 'Offline',
                  style: AppTypography.caption.copyWith(
                      fontSize: 11.5,
                      fontWeight: FontWeight.w600,
                      color: d.online ? AppColors.success : AppColors.error)),
              const Spacer(),
              InkWell(
                borderRadius: BorderRadius.circular(6),
                onTap: () =>
                    widget.onToggleFav(d.id), // FAVORITE — audit: toggleFavorite
                child: Padding(
                  padding: const EdgeInsets.all(2),
                  child: Icon(
                      d.favorite
                          ? Icons.star_rounded
                          : Icons.star_outline_rounded,
                      size: 18,
                      color: d.favorite
                          ? AppColors.warning
                          : AppColors.textTertiary),
                ),
              ),
            ]),
            const SizedBox(height: 14),
            Center(
              child: Container(
                width: 66,
                height: 66,
                decoration: BoxDecoration(
                    color: tint.withValues(alpha: 0.14),
                    borderRadius: BorderRadius.circular(18)),
                child: Icon(_deviceGlyph(d.os), size: 32, color: tint),
              ),
            ),
            const SizedBox(height: 14),
            Center(
                child: Text(_group(d.id),
                    style: AppTypography.idLarge.copyWith(fontSize: 16.5))),
            const SizedBox(height: 3),
            Center(
              child: Text(d.name.isEmpty ? _osLabel(d.os) : d.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: AppTypography.caption
                      .copyWith(fontSize: 12.5, color: AppColors.textSecondary)),
            ),
            const SizedBox(height: 14),
            Container(height: 1, color: AppColors.border),
            const SizedBox(height: 10),
            Row(children: [
              Expanded(
                child: Text('ID ${_group(d.id)}',
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AppTypography.mono.copyWith(
                        fontSize: 10.5, color: AppColors.textTertiary)),
              ),
              const SizedBox(width: 8),
              Text(
                  d.lastConnected != null
                      ? _ago(d.lastConnected!)
                      : (d.online ? 'online' : '—'),
                  style: AppTypography.meta.copyWith(fontSize: 10.5)),
            ]),
          ]),
        ),
      ),
    );
  }
}

class _CardConnect extends StatefulWidget {
  final VoidCallback onTap;
  const _CardConnect({required this.onTap});
  @override
  State<_CardConnect> createState() => _CardConnectState();
}

class _CardConnectState extends State<_CardConnect> {
  bool _h = false;
  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      onEnter: (_) => setState(() => _h = true),
      onExit: (_) => setState(() => _h = false),
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: widget.onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 140),
          height: 34,
          padding: const EdgeInsets.symmetric(horizontal: 14),
          decoration: BoxDecoration(
            color: _h ? AppColors.primary : AppColors.primarySoft,
            borderRadius: BorderRadius.circular(AppRadii.md),
          ),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            Text('Connect',
                style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: _h ? Colors.white : AppColors.primaryDark)),
            const SizedBox(width: 6),
            Icon(Icons.arrow_forward_rounded,
                size: 14, color: _h ? Colors.white : AppColors.primaryDark),
          ]),
        ),
      ),
    );
  }
}

// ---------------------------------------------------- bottom panels (mockup)
class _BottomPanels extends StatelessWidget {
  final List<RecentConnection> recents;
  final List<_HomeDevice> favorites;
  final void Function(String id) onPick;
  final bool unattended;
  final VoidCallback onOpenSettings;
  final VoidCallback onInvite;
  final VoidCallback onHelp;
  const _BottomPanels({
    required this.recents,
    required this.favorites,
    required this.onPick,
    required this.unattended,
    required this.onOpenSettings,
    required this.onInvite,
    required this.onHelp,
  });

  Widget _card(String title, Widget child) => Container(
        padding: const EdgeInsets.fromLTRB(18, 16, 14, 12),
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(AppRadii.card),
          border: Border.all(color: AppColors.border),
          boxShadow: AppShadows.card,
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(title, style: AppTypography.sectionTitle),
          const SizedBox(height: 12),
          child,
        ]),
      );

  Widget _empty(String msg) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 22),
        child: Center(
            child: Text(msg,
                textAlign: TextAlign.center,
                style: AppTypography.caption.copyWith(fontSize: 12))),
      );

  Widget _deviceRow(String id, String name, String os, Widget trailing,
      VoidCallback onTap) {
    final tint =
        _deviceTints[id.codeUnits.fold<int>(0, (a, b) => a + b) % _deviceTints.length];
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(AppRadii.md),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 7, horizontal: 4),
        child: Row(children: [
          Container(
            width: 30,
            height: 30,
            decoration: BoxDecoration(
                color: tint.withValues(alpha: 0.14),
                borderRadius: BorderRadius.circular(9)),
            child: Icon(_deviceGlyph(os), size: 15, color: tint),
          ),
          const SizedBox(width: 11),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(_group(id),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: AppTypography.mono.copyWith(
                      fontSize: 12.5, fontWeight: FontWeight.w600)),
              if (name.isNotEmpty && name != id)
                Text(name,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AppTypography.meta.copyWith(fontSize: 10.5)),
            ]),
          ),
          const SizedBox(width: 8),
          trailing,
        ]),
      ),
    );
  }

  Widget _quickAction(
          IconData icon, String label, String state, VoidCallback onTap) =>
      InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(AppRadii.md),
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 4),
          child: Row(children: [
            Container(
              width: 30,
              height: 30,
              decoration: BoxDecoration(
                  color: AppColors.primarySoft,
                  borderRadius: BorderRadius.circular(9)),
              child: Icon(icon, size: 15, color: AppColors.primary),
            ),
            const SizedBox(width: 11),
            Expanded(
                child: Text(label,
                    style: AppTypography.body.copyWith(fontSize: 13))),
            Text(state,
                style: AppTypography.meta.copyWith(
                    fontSize: 10.5,
                    color: state == 'On'
                        ? AppColors.success
                        : AppColors.textTertiary)),
            const SizedBox(width: 6),
            Icon(Icons.chevron_right_rounded,
                size: 16, color: AppColors.textTertiary),
          ]),
        ),
      );

  @override
  Widget build(BuildContext context) {
    final sessions = _card(
      'Recent Sessions',
      recents.isEmpty
          ? _empty('No sessions yet')
          : Column(children: [
              for (final r in recents.take(5))
                _deviceRow(
                    r.id,
                    r.name,
                    '',
                    Text(r.lastConnected != null ? _ago(r.lastConnected!) : '',
                        style: AppTypography.meta.copyWith(fontSize: 10.5)),
                    () => onPick(r.id)),
            ]),
    );
    final favs = _card(
      'Favorites',
      favorites.isEmpty
          ? _empty("No favorites yet —\ntap a device's star")
          : Column(children: [
              for (final d in favorites.take(5))
                _deviceRow(
                    d.id,
                    d.name,
                    d.os,
                    Icon(Icons.star_rounded,
                        size: 16, color: AppColors.warning),
                    () => onPick(d.id)),
            ]),
    );
    final actions = _card(
      'Quick Actions',
      // Every row does something real. Wake-on-LAN was removed: it needs each
      // device's MAC address, which nothing in the app collects — a WoL button
      // could only ever no-op (Data Honesty Rule).
      Column(children: [
        _quickAction(Icons.podcasts_rounded, 'Unattended Access',
            unattended ? 'On' : 'Off', onOpenSettings),
        _quickAction(Icons.shield_outlined, 'Security & access', 'Settings',
            onOpenSettings),
        _quickAction(Icons.download_rounded, 'Install agent / daemon',
            'Settings', onOpenSettings),
        _quickAction(Icons.person_add_alt_1_rounded, 'Invite a friend',
            'Copy invite', onInvite),
        _quickAction(
            Icons.help_outline_rounded, 'Help & Support', 'Details', onHelp),
      ]),
    );

    return LayoutBuilder(builder: (context, c) {
      if (c.maxWidth >= 940) {
        return Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Expanded(child: sessions),
          const SizedBox(width: 16),
          Expanded(child: favs),
          const SizedBox(width: 16),
          Expanded(child: actions),
        ]);
      }
      return Column(children: [
        sessions,
        const SizedBox(height: 16),
        favs,
        const SizedBox(height: 16),
        actions,
      ]);
    });
  }
}

// ---------------------------------------------------------------- timeline

class _ActivityTimeline extends StatelessWidget {
  final List<RecentConnection> recents;
  const _ActivityTimeline({required this.recents});

  @override
  Widget build(BuildContext context) {
    if (recents.isEmpty) {
      return Container(
        padding: const EdgeInsets.all(24),
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(AppRadii.lg),
          border: Border.all(color: AppColors.border),
        ),
        child: Text('No recent activity yet.',
            style: AppTypography.caption),
      );
    }
    final items = recents.take(8).toList();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadii.lg),
        border: Border.all(color: AppColors.border),
        boxShadow: AppShadows.card,
      ),
      child: Column(
        children: [
          for (var i = 0; i < items.length; i++)
            _TimelineRow(item: items[i], first: i == 0),
        ],
      ),
    );
  }
}

class _TimelineRow extends StatelessWidget {
  final RecentConnection item;
  final bool first;
  const _TimelineRow({required this.item, required this.first});
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 11),
      decoration: BoxDecoration(
        border: first
            ? null
            : Border(top: BorderSide(color: AppColors.border)),
      ),
      child: Row(children: [
        SizedBox(
          width: 64,
          child: Text(_time(item.lastConnected),
              style: AppTypography.mono.copyWith(
                  fontSize: 12, color: AppColors.textTertiary)),
        ),
        Container(
          width: 22,
          height: 22,
          margin: const EdgeInsets.only(right: 12),
          decoration: BoxDecoration(
            color: AppColors.success.withValues(alpha: 0.14),
            borderRadius: BorderRadius.circular(999),
          ),
          child: Icon(Icons.link_rounded,
              size: 13, color: AppColors.success),
        ),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Connected to ${item.name}',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: AppTypography.bodyStrong.copyWith(fontSize: 13.5)),
              Text('ID ${_group(item.id)}',
                  style: AppTypography.caption.copyWith(fontSize: 11.5)),
            ],
          ),
        ),
        Text(_ago(item.lastConnected),
            style: AppTypography.mono.copyWith(
                fontSize: 12, color: AppColors.textTertiary)),
      ]),
    );
  }
}

// ---------------------------------------------------------------- helpers

IconData _glyph(String os) {
  final o = os.toLowerCase();
  if (o.contains('mac') || o.contains('ios') || o.contains('ipad')) {
    return Icons.laptop_mac_rounded;
  }
  if (o.contains('android') || o.contains('phone')) {
    return Icons.smartphone_rounded;
  }
  if (o.contains('server')) return Icons.dns_rounded;
  if (o.contains('linux')) return Icons.terminal_rounded;
  return Icons.laptop_windows_rounded;
}

String _osLabel(String os) {
  if (os.isEmpty) return 'Device';
  final o = os.toLowerCase();
  if (o.contains('windows')) return 'Windows';
  if (o.contains('mac')) return 'macOS';
  if (o.contains('linux')) return 'Linux';
  return os;
}

String _group(String id) {
  final s = id.replaceAll(RegExp(r'[^0-9]'), '');
  if (s.length != 9) return id;
  return '${s.substring(0, 3)} ${s.substring(3, 6)} ${s.substring(6)}';
}

String _time(DateTime t) {
  final h = t.hour % 12 == 0 ? 12 : t.hour % 12;
  final m = t.minute.toString().padLeft(2, '0');
  return '$h:$m ${t.hour < 12 ? 'AM' : 'PM'}';
}

String _ago(DateTime t) {
  final d = DateTime.now().difference(t);
  if (d.inMinutes < 1) return 'just now';
  if (d.inMinutes < 60) return '${d.inMinutes}m ago';
  if (d.inHours < 24) return '${d.inHours}h ago';
  return '${d.inDays}d ago';
}

// ---------------------------------------------------------------- activity panel

/// Right column of the Command Center shell: this machine's own ID + password
/// (so it can be shared/dialled) followed by live activity — incoming consent
/// requests, active file transfers, connected viewers. Real state only.
class CommandActivityPanel extends StatelessWidget {
  final RemoteService service;
  const CommandActivityPanel({super.key, required this.service});

  @override
  Widget build(BuildContext context) {
    final consent = service.pendingConsent;
    final xfers = service.fileTransfers
        .where((t) =>
            t.status == FileStatus.active || t.status == FileStatus.sent)
        .toList();
    final viewers = service.connectedViewers;

    return Container(
      width: 328,
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border(left: BorderSide(color: AppColors.border)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // header
          Container(
            height: 74,
            padding: const EdgeInsets.symmetric(horizontal: 20),
            decoration: BoxDecoration(
              border: Border(bottom: BorderSide(color: AppColors.border)),
            ),
            child: Row(children: [
              Text('Live activity', style: AppTypography.sectionTitle),
              const Spacer(),
              Container(
                width: 7,
                height: 7,
                decoration: BoxDecoration(
                    color: AppColors.success, shape: BoxShape.circle),
              ),
              const SizedBox(width: 6),
              Text('Live',
                  style: AppTypography.caption.copyWith(
                      fontSize: 11,
                      fontWeight: FontWeight.w700,
                      color: AppColors.success)),
            ]),
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.all(16),
              children: [
                _ThisDeviceCard(service: service),
                const SizedBox(height: 14),
                if (consent != null) ...[
                  _ConsentRequestCard(controllerId: consent.controllerId),
                  const SizedBox(height: 12),
                ],
                if (viewers > 0) ...[
                  _ActivityRow(
                    icon: Icons.circle,
                    tint: AppColors.success,
                    title: '$viewers viewer${viewers == 1 ? '' : 's'} connected',
                    sub: 'Sharing your screen · encrypted',
                  ),
                  const SizedBox(height: 12),
                ],
                for (final t in xfers) ...[
                  _TransferRow(name: t.name, progress: t.progress),
                  const SizedBox(height: 12),
                ],
                if (consent == null && viewers == 0 && xfers.isEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 30),
                    child: Column(children: [
                      Icon(Icons.bolt_rounded,
                          size: 28, color: AppColors.textTertiary),
                      const SizedBox(height: 8),
                      Text('Nothing happening right now',
                          style: AppTypography.caption
                              .copyWith(color: AppColors.textSecondary)),
                      const SizedBox(height: 3),
                      Text('Incoming connections and transfers show here.',
                          textAlign: TextAlign.center,
                          style: AppTypography.caption.copyWith(fontSize: 11)),
                    ]),
                  ),
                const SizedBox(height: 16),
                const _SecurityBadges(),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ThisDeviceCard extends StatelessWidget {
  final RemoteService service;
  const _ThisDeviceCard({required this.service});

  @override
  Widget build(BuildContext context) {
    final id = service.agentId ?? '—';
    final pw = service.password ?? '—';

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [AppColors.deviceNavy, Color.alphaBlend(
              Colors.black.withValues(alpha: 0.15), AppColors.deviceNavy)],
        ),
        borderRadius: BorderRadius.circular(AppRadii.card),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.wifi_tethering_rounded,
              size: 15, color: Colors.white70),
          const SizedBox(width: 7),
          Text('THIS DEVICE — share to be controlled',
              style: AppTypography.microLabel
                  .copyWith(color: Colors.white70, fontSize: 8.5)),
        ]),
        const SizedBox(height: 14),
        _DarkRow(label: 'Your ID', value: id == '—' ? id : _group(id)),
        const Padding(
          padding: EdgeInsets.symmetric(vertical: 10),
          child: Divider(height: 1, color: Colors.white24),
        ),
        _DarkRow(label: 'Password', value: pw, accent: true),
      ]),
    );
  }
}

class _DarkRow extends StatelessWidget {
  final String label;
  final String value;
  final bool accent;
  const _DarkRow(
      {required this.label, required this.value, this.accent = false});
  @override
  Widget build(BuildContext context) {
    return Row(children: [
      Expanded(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(label.toUpperCase(),
              style: AppTypography.microLabel
                  .copyWith(color: Colors.white54, fontSize: 8.5)),
          const SizedBox(height: 3),
          Text(value,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: AppTypography.idLarge.copyWith(
                  color: accent ? const Color(0xFFFFB088) : Colors.white,
                  fontSize: 16,
                  letterSpacing: accent ? 1 : 2.5)),
        ]),
      ),
      _Copy(value: value, dark: true),
    ]);
  }
}

class _Copy extends StatelessWidget {
  final String value;
  final bool dark;
  const _Copy({required this.value, this.dark = false});
  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(AppRadii.sm),
      onTap: value == '—'
          ? null
          : () {
              Clipboard.setData(ClipboardData(text: value));
              ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
                  content: Text('Copied'), duration: Duration(seconds: 1)));
            },
      child: Container(
        width: 28,
        height: 28,
        decoration: BoxDecoration(
          color: dark ? Colors.white.withValues(alpha: 0.12) : AppColors.background,
          borderRadius: BorderRadius.circular(AppRadii.sm),
          border: dark ? null : Border.all(color: AppColors.borderStrong),
        ),
        child: Icon(Icons.copy_rounded,
            size: 13, color: dark ? Colors.white : AppColors.textSecondary),
      ),
    );
  }
}

class _ConsentRequestCard extends StatelessWidget {
  final String controllerId;
  const _ConsentRequestCard({required this.controllerId});
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.primarySoft,
        borderRadius: BorderRadius.circular(AppRadii.card),
        border: Border.all(color: AppColors.primary.withValues(alpha: 0.3)),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('Incoming connection',
            style: AppTypography.bodyStrong.copyWith(fontSize: 13.5)),
        const SizedBox(height: 2),
        Text('Device $controllerId wants to connect',
            style: AppTypography.caption.copyWith(fontSize: 12)),
        const SizedBox(height: 12),
        Text('Use the Accept / Dismiss prompt to decide.',
            style: AppTypography.caption.copyWith(fontSize: 11)),
      ]),
    );
  }
}

class _ActivityRow extends StatelessWidget {
  final IconData icon;
  final Color tint;
  final String title;
  final String sub;
  const _ActivityRow(
      {required this.icon,
      required this.tint,
      required this.title,
      required this.sub});
  @override
  Widget build(BuildContext context) {
    return Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          color: tint.withValues(alpha: 0.14),
          borderRadius: BorderRadius.circular(9),
        ),
        child: Icon(icon, size: 13, color: tint),
      ),
      const SizedBox(width: 12),
      Expanded(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(title,
              style: AppTypography.caption.copyWith(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w600,
                  color: AppColors.textPrimary)),
          Text(sub, style: AppTypography.caption.copyWith(fontSize: 11)),
        ]),
      ),
    ]);
  }
}

class _TransferRow extends StatelessWidget {
  final String name;
  final double progress;
  const _TransferRow({required this.name, required this.progress});
  @override
  Widget build(BuildContext context) {
    return Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          color: AppColors.primary.withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(9),
        ),
        child: Icon(Icons.swap_vert_rounded,
            size: 15, color: AppColors.primary),
      ),
      const SizedBox(width: 12),
      Expanded(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: AppTypography.caption.copyWith(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w600,
                  color: AppColors.textPrimary)),
          const SizedBox(height: 6),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: LinearProgressIndicator(
              value: progress > 0 ? progress : null,
              minHeight: 5,
              backgroundColor: AppColors.surfaceLight,
              valueColor:
                  AlwaysStoppedAnimation<Color>(AppColors.primary),
            ),
          ),
        ]),
      ),
    ]);
  }
}

class _SecurityBadges extends StatelessWidget {
  const _SecurityBadges();
  @override
  Widget build(BuildContext context) {
    Widget b(String t) => Container(
          padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 5),
          decoration: BoxDecoration(
            color: AppColors.successSoft,
            borderRadius: BorderRadius.circular(999),
          ),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            Icon(Icons.lock_rounded, size: 11, color: AppColors.success),
            const SizedBox(width: 5),
            Text(t,
                style: AppTypography.caption.copyWith(
                    fontSize: 10.5,
                    fontWeight: FontWeight.w700,
                    color: AppColors.success)),
          ]),
        );
    return Wrap(spacing: 6, runSpacing: 6, children: [
      b('Encrypted'),
      b('Pinned cert'),
      b('End-to-end'),
    ]);
  }
}
