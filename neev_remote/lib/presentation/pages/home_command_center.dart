import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

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
                        ? const Icon(Icons.check_circle_rounded,
                            size: 18, color: AppColors.success)
                        : active
                            ? const CircularProgressIndicator(
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
class CommandNavRail extends StatefulWidget {
  final List<NavRailItem> items;
  final int selected;
  final bool online;
  final ValueChanged<int> onSelect;
  const CommandNavRail({
    super.key,
    required this.items,
    required this.selected,
    required this.online,
    required this.onSelect,
  });

  @override
  State<CommandNavRail> createState() => _CommandNavRailState();
}

class _CommandNavRailState extends State<CommandNavRail> {
  bool _open = false;

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      onEnter: (_) => setState(() => _open = true),
      onExit: (_) => setState(() => _open = false),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 280),
        curve: Curves.easeOutCubic,
        width: _open ? 240 : 88,
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: const Border(right: BorderSide(color: AppColors.border)),
          boxShadow: _open
              ? [
                  BoxShadow(
                      color: Colors.black.withValues(alpha: 0.06),
                      blurRadius: 40,
                      offset: const Offset(8, 0)),
                ]
              : null,
        ),
        clipBehavior: Clip.hardEdge,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // logo
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 20, 20, 18),
              child: Row(children: [
                Container(
                  width: 34,
                  height: 34,
                  decoration: BoxDecoration(
                    gradient: const LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [Color(0xFFFF8352), Color(0xFFE0491A)],
                    ),
                    borderRadius: BorderRadius.circular(10),
                    boxShadow: const [
                      BoxShadow(
                          color: Color(0x66FF6A32),
                          blurRadius: 18,
                          offset: Offset(0, 5)),
                    ],
                  ),
                  alignment: Alignment.center,
                  child: const Text('N',
                      style: TextStyle(
                          color: Colors.white,
                          fontWeight: FontWeight.w800,
                          fontSize: 16)),
                ),
                if (_open) ...[
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('Neev Remote',
                            maxLines: 1,
                            overflow: TextOverflow.clip,
                            style: AppTypography.cardTitle.copyWith(fontSize: 15)),
                        Text('COMMAND CENTER',
                            style: AppTypography.microLabel
                                .copyWith(fontSize: 9, letterSpacing: 0.8)),
                      ],
                    ),
                  ),
                ],
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
            const SizedBox(height: 12),
          ],
        ),
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
                  decoration: const BoxDecoration(
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

  const HomeCommandCenter({
    super.key,
    required this.service,
    required this.idController,
    required this.passwordController,
    required this.onConnect,
    required this.onPick,
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
        _IdleFloat(
          amplitude: 2.5,
          child: _ConnectionDock(
            idController: widget.idController,
            passwordController: widget.passwordController,
            onConnect: widget.onConnect,
            recents: ref.watch(recentConnectionsProvider).take(3).toList(),
            onPick: widget.onPick,
          ),
        ),
        const SizedBox(height: 26),
        _StatusStrip(
          onlineCount: onlineCount,
          knownCount: all.length,
          activeXfer: activeXfer,
          sharing: sharing,
          unattended: unattended,
          connectedViewers: service.connectedViewers,
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
        ),
        const SizedBox(height: 28),
        const _SectionHead(title: 'Recent activity', tabs: []),
        const SizedBox(height: 14),
        _ActivityTimeline(recents: ref.watch(recentConnectionsProvider)),
      ],
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
      padding: const EdgeInsets.fromLTRB(18, 15, 18, 15),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadii.lg),
        border: Border.all(color: AppColors.border),
        boxShadow: AppShadows.dock,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Container(
              width: 34,
              height: 34,
              decoration: BoxDecoration(
                color: AppColors.primarySoft,
                borderRadius: BorderRadius.circular(AppRadii.md),
              ),
              child: const Icon(Icons.cast_connected_rounded,
                  size: 18, color: AppColors.primary),
            ),
            const SizedBox(width: 11),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Connect to a device',
                      style: AppTypography.sectionTitle.copyWith(fontSize: 14.5)),
                  Text('Enter an ID and password to start a session',
                      style: AppTypography.meta.copyWith(fontSize: 11)),
                ],
              ),
            ),
          ]),
          const SizedBox(height: 12),
          _DockField(
            controller: idController,
            hint: 'Remote ID or device name',
            icon: Icons.devices_rounded,
            mono: true,
            onSubmitted: (_) => onConnect(),
          ),
          const SizedBox(height: 9),
          Row(
            children: [
              Expanded(
                flex: 5,
                child: _DockField(
                  controller: passwordController,
                  hint: 'Password',
                  icon: Icons.lock_outline_rounded,
                  obscure: true,
                  onSubmitted: (_) => onConnect(),
                ),
              ),
              const SizedBox(width: 9),
              const Expanded(flex: 3, child: _ModeSelector()),
              const SizedBox(width: 9),
              _ConnectButton(onTap: onConnect),
            ],
          ),
          if (recents.isNotEmpty) ...[
            const SizedBox(height: 12),
            Row(children: [
              Text('Recent',
                  style: AppTypography.label
                      .copyWith(fontSize: 11, color: AppColors.textTertiary)),
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
      height: 44,
      padding: const EdgeInsets.symmetric(horizontal: 13),
      decoration: BoxDecoration(
        color: AppColors.surfaceLight,
        borderRadius: BorderRadius.circular(AppRadii.md),
        border: Border.all(color: AppColors.borderStrong),
      ),
      child: Row(children: [
        Icon(icon, size: 17, color: AppColors.textTertiary),
        const SizedBox(width: 10),
        Expanded(
          child: TextField(
            controller: controller,
            obscureText: obscure,
            onSubmitted: onSubmitted,
            style: mono
                ? AppTypography.idLarge.copyWith(fontSize: 14.5, letterSpacing: 0.8)
                : AppTypography.body.copyWith(fontSize: 14),
            decoration: InputDecoration(
              hintText: hint,
              hintStyle:
                  AppTypography.body.copyWith(color: AppColors.textTertiary),
              border: InputBorder.none,
              isCollapsed: true,
              filled: false,
            ),
          ),
        ),
      ]),
    );
  }
}

class _ModeSelector extends StatefulWidget {
  const _ModeSelector();
  @override
  State<_ModeSelector> createState() => _ModeSelectorState();
}

class _ModeSelectorState extends State<_ModeSelector> {
  static const _modes = [
    'Full Control',
    'View Only',
    'File Transfer',
    'Privacy Mode',
    'Support Mode',
  ];
  String _mode = 'Full Control';

  @override
  Widget build(BuildContext context) {
    return PopupMenuButton<String>(
      initialValue: _mode,
      onSelected: (v) => setState(() => _mode = v),
      offset: const Offset(0, 46),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadii.md),
        side: const BorderSide(color: AppColors.border),
      ),
      color: AppColors.surface,
      itemBuilder: (_) => [
        for (final m in _modes)
          PopupMenuItem(
              value: m,
              height: 38,
              child: Text(m, style: AppTypography.body.copyWith(fontSize: 13))),
      ],
      child: Container(
        height: 44,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(
          color: AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(AppRadii.md),
          border: Border.all(color: AppColors.borderStrong),
        ),
        child: Row(children: [
          const Icon(Icons.tune_rounded, size: 15, color: AppColors.textTertiary),
          const SizedBox(width: 8),
          Expanded(
            child: Text(_mode,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: AppTypography.bodyStrong.copyWith(fontSize: 12.5)),
          ),
          const Icon(Icons.keyboard_arrow_down_rounded,
              size: 17, color: AppColors.textTertiary),
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
          height: 44,
          padding: const EdgeInsets.symmetric(horizontal: 20),
          transform: Matrix4.translationValues(
              0, _down ? 0 : (_hover ? -2 : 0), 0)
            ..scaleByDouble(
                _down ? 0.97 : 1.0, _down ? 0.97 : 1.0, 1.0, 1.0),
          transformAlignment: Alignment.center,
          decoration: BoxDecoration(
            color: _hover ? AppColors.primaryDark : AppColors.primary,
            borderRadius: BorderRadius.circular(AppRadii.md),
            boxShadow: [
              BoxShadow(
                color: AppColors.primary.withValues(alpha: _hover ? 0.5 : 0.34),
                blurRadius: _hover ? 22 : 14,
                offset: Offset(0, _hover ? 9 : 5),
              ),
            ],
          ),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            const Text('Connect',
                style: TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.w700,
                    fontSize: 14)),
            const SizedBox(width: 8),
            AnimatedSlide(
              duration: const Duration(milliseconds: 160),
              offset: Offset(_hover ? 0.25 : 0, 0),
              child: const Icon(Icons.arrow_forward_rounded,
                  color: Colors.white, size: 18),
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
              decoration: const BoxDecoration(
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
  const _DeviceGrid({required this.devices, required this.onPick});

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
          const Icon(Icons.devices_other_rounded,
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
            SizedBox(width: w, child: _DeviceCard(device: d, onPick: onPick)),
        ],
      );
    });
  }
}

const List<Color> _grounds = [
  AppColors.deviceNavy,
  AppColors.deviceForest,
  AppColors.devicePlum,
  AppColors.deviceWalnut,
];

class _DeviceCard extends StatefulWidget {
  final _HomeDevice device;
  final void Function(String id) onPick;
  const _DeviceCard({required this.device, required this.onPick});
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
    // Subtle premium depth: the whole card tips a few degrees toward the pointer
    // (perspective), with the thumbnail glyph parallaxing a touch more. Small
    // angles + a soft settle on exit — no continuous spin.
    final tilt = Matrix4.identity()
      ..setEntry(3, 2, 0.0011)
      ..rotateY(_hover ? _tilt.dx * 0.12 : 0)
      ..rotateX(_hover ? -_tilt.dy * 0.10 : 0);
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onHover: _onHover,
      onExit: (_) => setState(() {
        _hover = false;
        _tilt = Offset.zero;
      }),
      cursor: SystemMouseCursors.click,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        curve: Curves.easeOutCubic,
        transformAlignment: Alignment.center,
        transform: tilt..translateByDouble(0.0, _hover ? -4.0 : 0.0, 0.0, 1.0),
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
              color: _hover
                  ? AppColors.primary.withValues(alpha: 0.4)
                  : AppColors.border),
          boxShadow: _hover ? AppShadows.cardHover : AppShadows.card,
        ),
        clipBehavior: Clip.antiAlias,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // thumbnail (real screen if captured, else a light placeholder),
            // treated as a "lit window": a faint top sheen + bottom shade give
            // depth, and a hairline separates it from the card body.
            SizedBox(
              height: 108,
              width: double.infinity,
              child: Stack(fit: StackFit.expand, children: [
                (d.thumbPath != null)
                    ? thumbImage(d.thumbPath!, fallback: _placeholder(d))
                    : _placeholder(d),
                const IgnorePointer(
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      gradient: LinearGradient(
                        begin: Alignment.topCenter,
                        end: Alignment.bottomCenter,
                        colors: [
                          Color(0x14FFFFFF),
                          Color(0x00000000),
                          Color(0x38000000),
                        ],
                        stops: [0.0, 0.55, 1.0],
                      ),
                    ),
                  ),
                ),
                Align(
                  alignment: Alignment.bottomCenter,
                  child: Container(height: 1, color: AppColors.border),
                ),
              ]),
            ),
            // body
            Padding(
              padding: const EdgeInsets.fromLTRB(13, 11, 12, 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(children: [
                    Container(
                      width: 8,
                      height: 8,
                      decoration: BoxDecoration(
                        color: d.online
                            ? AppColors.success
                            : AppColors.textTertiary,
                        shape: BoxShape.circle,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(d.name.isEmpty ? d.id : d.name,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style:
                              AppTypography.cardTitle.copyWith(fontSize: 14.5)),
                    ),
                    Icon(
                        d.favorite
                            ? Icons.star_rounded
                            : Icons.star_outline_rounded,
                        size: 16,
                        color: d.favorite
                            ? AppColors.warning
                            : AppColors.textTertiary),
                  ]),
                  const SizedBox(height: 3),
                  Text('${_osLabel(d.os)}  ·  ID ${_group(d.id)}',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: AppTypography.mono.copyWith(
                          fontSize: 11.5, color: AppColors.textTertiary)),
                  const SizedBox(height: 11),
                  Row(children: [
                    Expanded(
                      child: Text(
                        d.lastConnected != null
                            ? _ago(d.lastConnected!)
                            : (d.online ? 'Online now' : 'Not connected yet'),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: AppTypography.caption.copyWith(fontSize: 11),
                      ),
                    ),
                    const SizedBox(width: 8),
                    _CardConnect(onTap: () => widget.onPick(d.id)),
                  ]),
                ],
              ),
            ),
          ],
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
            : const Border(top: BorderSide(color: AppColors.border)),
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
          child: const Icon(Icons.link_rounded,
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
      decoration: const BoxDecoration(
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
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: AppColors.border)),
            ),
            child: Row(children: [
              Text('Live activity', style: AppTypography.sectionTitle),
              const Spacer(),
              Container(
                width: 7,
                height: 7,
                decoration: const BoxDecoration(
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
                      const Icon(Icons.bolt_rounded,
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
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF2A2118), Color(0xFF201C16)],
        ),
        borderRadius: BorderRadius.circular(AppRadii.card),
        border: Border.all(color: const Color(0x33FF6A32)),
        boxShadow: const [
          BoxShadow(
              color: Color(0x1FFF6A32), blurRadius: 22, offset: Offset(0, 6)),
          BoxShadow(color: Color(0x40000000), blurRadius: 14, offset: Offset(0, 6)),
        ],
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.wifi_tethering_rounded,
              size: 15, color: AppColors.primary),
          const SizedBox(width: 7),
          Text('THIS DEVICE — share to be controlled',
              style: AppTypography.microLabel.copyWith(
                  color: AppColors.textSecondary, fontSize: 8.5)),
        ]),
        const SizedBox(height: 14),
        _DarkRow(label: 'Your ID', value: id == '—' ? id : _group(id)),
        const Padding(
          padding: EdgeInsets.symmetric(vertical: 10),
          child: Divider(height: 1, color: Color(0x33FFFFFF)),
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
                  .copyWith(color: AppColors.textTertiary, fontSize: 8.5)),
          const SizedBox(height: 3),
          Text(value,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: AppTypography.idLarge.copyWith(
                  color: accent ? AppColors.primaryHover : AppColors.textPrimary,
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
        child: const Icon(Icons.swap_vert_rounded,
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
                  const AlwaysStoppedAnimation<Color>(AppColors.primary),
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
            const Icon(Icons.lock_rounded, size: 11, color: AppColors.success),
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
