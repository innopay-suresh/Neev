import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/app_theme.dart';
import '../../data/services/audit_log.dart';
import '../../data/services/discovery_model.dart';
import '../../data/services/file_transfer_service.dart' show FileStatus;
import '../../data/services/remote_service.dart';
import '../providers/app_providers.dart';

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
                      colors: [AppColors.primary, AppColors.primaryDark],
                    ),
                    borderRadius: BorderRadius.circular(10),
                    boxShadow: [
                      BoxShadow(
                          color: AppColors.primary.withValues(alpha: 0.4),
                          blurRadius: 12,
                          offset: const Offset(0, 4)),
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
            // bottom device
            Container(
              margin: const EdgeInsets.all(12),
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                border: Border.all(color: AppColors.border),
                borderRadius: BorderRadius.circular(AppRadii.md),
              ),
              child: Row(children: [
                Stack(clipBehavior: Clip.none, children: [
                  Container(
                    width: 34,
                    height: 34,
                    decoration: BoxDecoration(
                      color: AppColors.deviceNavy,
                      borderRadius: BorderRadius.circular(10),
                    ),
                    alignment: Alignment.center,
                    child: const Text('PC',
                        style: TextStyle(
                            color: Colors.white,
                            fontWeight: FontWeight.w700,
                            fontSize: 12)),
                  ),
                  Positioned(
                    right: -2,
                    bottom: -2,
                    child: Container(
                      width: 11,
                      height: 11,
                      decoration: BoxDecoration(
                        color: widget.online
                            ? AppColors.success
                            : AppColors.textTertiary,
                        shape: BoxShape.circle,
                        border: Border.all(color: AppColors.surface, width: 2.5),
                      ),
                    ),
                  ),
                ]),
                if (_open) ...[
                  const SizedBox(width: 11),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('This PC',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: AppTypography.caption.copyWith(
                                fontSize: 13,
                                fontWeight: FontWeight.w600,
                                color: AppColors.textPrimary)),
                        Text(widget.online ? 'Online · Sharing' : 'Offline',
                            style: AppTypography.caption.copyWith(
                                fontSize: 10.5,
                                color: widget.online
                                    ? AppColors.success
                                    : AppColors.textTertiary,
                                fontWeight: FontWeight.w600)),
                      ],
                    ),
                  ),
                ],
              ]),
            ),
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
  _HomeDevice(this.id, this.name, this.os, this.online, this.favorite,
      this.lastConnected);
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
        _ConnectionDock(
          idController: widget.idController,
          passwordController: widget.passwordController,
          onConnect: widget.onConnect,
          recents: ref.watch(recentConnectionsProvider).take(3).toList(),
          onPick: widget.onPick,
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
      offset: const Offset(0, 54),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadii.lg),
        side: const BorderSide(color: AppColors.border),
      ),
      color: AppColors.surface,
      itemBuilder: (_) => [
        for (final m in _modes)
          PopupMenuItem(
              value: m,
              height: 40,
              child: Text(m, style: AppTypography.body.copyWith(fontSize: 13.5))),
      ],
      child: Container(
        height: 52,
        padding: const EdgeInsets.symmetric(horizontal: 14),
        decoration: BoxDecoration(
          color: AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(AppRadii.lg),
          border: Border.all(color: AppColors.borderStrong),
        ),
        child: Row(children: [
          Expanded(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('MODE', style: AppTypography.microLabel),
                Text(_mode,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: AppTypography.bodyStrong.copyWith(fontSize: 13.5)),
              ],
            ),
          ),
          const Icon(Icons.keyboard_arrow_down_rounded,
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
      final cols = c.maxWidth > 1180 ? 4 : (c.maxWidth > 860 ? 3 : 2);
      const gap = 18.0;
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
        (local.dy / 150 - 0.5).clamp(-0.5, 0.5),
      );
    });
  }

  Color get _ground {
    final h = widget.device.id.codeUnits.fold<int>(0, (a, b) => a + b);
    return _grounds[h % _grounds.length];
  }

  @override
  Widget build(BuildContext context) {
    final d = widget.device;
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() {
        _hover = false;
        _tilt = Offset.zero;
      }),
      cursor: SystemMouseCursors.click,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        curve: Curves.easeOutCubic,
        transform: Matrix4.translationValues(0, _hover ? -6 : 0, 0),
        decoration: BoxDecoration(
          color: AppColors.surface,
          borderRadius: BorderRadius.circular(AppRadii.card),
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
            // stage
            Listener(
              child: MouseRegion(
                onHover: _onHover,
                child: SizedBox(
                  height: 150,
                  child: Stack(children: [
                    Positioned.fill(
                      child: DecoratedBox(
                        decoration: BoxDecoration(
                          gradient: LinearGradient(
                            begin: Alignment.topLeft,
                            end: Alignment.bottomRight,
                            colors: [
                              _ground,
                              Color.alphaBlend(
                                  Colors.black.withValues(alpha: 0.25), _ground),
                            ],
                          ),
                        ),
                      ),
                    ),
                    // glow
                    Positioned(
                      bottom: 18,
                      left: 0,
                      right: 0,
                      child: Center(
                        child: AnimatedOpacity(
                          duration: const Duration(milliseconds: 200),
                          opacity: _hover ? 0.8 : 0.45,
                          child: Container(
                            width: 120,
                            height: 22,
                            decoration: BoxDecoration(
                              borderRadius: BorderRadius.circular(999),
                              boxShadow: [
                                BoxShadow(
                                    color: AppColors.primary
                                        .withValues(alpha: 0.55),
                                    blurRadius: 22,
                                    spreadRadius: -6),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                    // tilting device glyph
                    Center(
                      child: Transform(
                        alignment: Alignment.center,
                        transform: Matrix4.identity()
                          ..setEntry(3, 2, 0.0014)
                          ..rotateY(_tilt.dx * 0.34)
                          ..rotateX(-_tilt.dy * 0.28),
                        child: Icon(_glyph(d.os), size: 60, color: Colors.white
                            .withValues(alpha: 0.92)),
                      ),
                    ),
                    // status badge
                    Positioned(
                      top: 12,
                      left: 12,
                      child: _StatusBadge(online: d.online),
                    ),
                    // favorite
                    Positioned(
                      top: 12,
                      right: 12,
                      child: Container(
                        width: 28,
                        height: 28,
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.14),
                          borderRadius: BorderRadius.circular(AppRadii.sm),
                        ),
                        child: Icon(
                            d.favorite ? Icons.star_rounded : Icons.star_outline_rounded,
                            size: 16,
                            color: d.favorite
                                ? const Color(0xFFF5C451)
                                : Colors.white),
                      ),
                    ),
                  ]),
                ),
              ),
            ),
            // body
            Padding(
              padding: const EdgeInsets.fromLTRB(15, 14, 15, 15),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(d.name.isEmpty ? d.id : d.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: AppTypography.cardTitle.copyWith(fontSize: 16)),
                  const SizedBox(height: 2),
                  Text(_osLabel(d.os),
                      style: AppTypography.caption.copyWith(fontSize: 12)),
                  const SizedBox(height: 3),
                  Text('ID ${_group(d.id)}',
                      style: AppTypography.mono.copyWith(
                          fontSize: 12.5, color: AppColors.textTertiary)),
                  const SizedBox(height: 12),
                  Row(children: [
                    Expanded(
                      child: Text(
                        d.lastConnected != null
                            ? _ago(d.lastConnected!)
                            : (d.online ? 'Online' : 'Never connected'),
                        style: AppTypography.caption.copyWith(fontSize: 11.5),
                      ),
                    ),
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

class _StatusBadge extends StatelessWidget {
  final bool online;
  const _StatusBadge({required this.online});
  @override
  Widget build(BuildContext context) {
    return Container(
      height: 26,
      padding: const EdgeInsets.symmetric(horizontal: 10),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: Colors.white.withValues(alpha: 0.18)),
      ),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        Container(
          width: 7,
          height: 7,
          decoration: BoxDecoration(
            color: online ? const Color(0xFF48D69A) : const Color(0xFF9A9385),
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 6),
        Text(online ? 'Online' : 'Offline',
            style: TextStyle(
                color: Colors.white.withValues(alpha: online ? 1 : 0.7),
                fontSize: 11,
                fontWeight: FontWeight.w600)),
      ]),
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
