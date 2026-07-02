import 'dart:ui' show ImageFilter;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/app_theme.dart';
import '../../data/services/remote_service.dart';
import '../providers/app_providers.dart';
import '../widgets/file_transfer_panel.dart';
import '../widgets/remote_view_widget.dart';
import '../widgets/shortcuts_menu.dart';
import 'settings_page.dart';

/// Single-screen hub: "Share my screen" (host) on the left, "Connect to a
/// computer" (viewer) on the right. When a viewer session is active it takes
/// over the whole screen. Replaces the old Home/Agent/Viewer/Settings tabs.
class ConnectPage extends ConsumerStatefulWidget {
  const ConnectPage({super.key});

  @override
  ConsumerState<ConnectPage> createState() => _ConnectPageState();
}

class _ConnectPageState extends ConsumerState<ConnectPage> {
  final _idController = TextEditingController();
  final _passwordController = TextEditingController();
  bool _autoStarted = false;
  int _section = 0; // selected sidebar section

  @override
  void initState() {
    super.initState();
    // On desktop, start hosting automatically when the app opens so the
    // machine is immediately reachable (service-like). The browser web build
    // stays manual (each visitor shouldn't auto-share their screen).
    if (!kIsWeb) {
      Future.delayed(const Duration(milliseconds: 600), _autoStartHost);
    }
  }

  Future<void> _autoStartHost() async {
    if (_autoStarted || !mounted) return;
    final service = ref.read(remoteServiceProvider);
    if (service.isHosting) return;
    final settings = ref.read(settingsProvider);
    if (settings.relayUrl.isEmpty) return; // wait until the server is configured
    _autoStarted = true;
    try {
      await service.startHosting(
        relayUrl: settings.relayUrl,
        // Unattended: reuse the fixed password so the id+password stay stable
        // across restarts; otherwise a fresh one is generated.
        password: settings.unattendedPassword.isEmpty
            ? null
            : settings.unattendedPassword,
      );
    } catch (_) {
      // Surfaced on the Share card; user can fix the relay URL in Settings.
    }
  }

  @override
  void dispose() {
    _idController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final service = ref.watch(remoteServiceProvider);
    final relayUrl = ref.watch(settingsProvider).relayUrl;

    // Active remote session takes the whole window.
    if (service.viewerStatus == ViewerStatus.connected) {
      return _ConnectedSession(service: service);
    }

    // First run with no server baked in / saved: ask for the server once.
    if (relayUrl.isEmpty) {
      return Scaffold(
        appBar: AppBar(
          backgroundColor: AppColors.surface,
          elevation: 0,
          title: Text('Neev Remote', style: AppTypography.heading2),
        ),
        body: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(AppSpacing.xl),
            child: SizedBox(width: 420, child: _ServerSetupCard(onSaved: () {
              _autoStarted = false;
              _autoStartHost();
            })),
          ),
        ),
      );
    }

    return Scaffold(
      backgroundColor: AppColors.background,
      body: Row(
        children: [
          _Sidebar(
            selected: _section,
            online: service.hostStatus == HostStatus.online,
            onSelect: (i) => setState(() => _section = i),
          ),
          Expanded(
            child: Column(
              children: [
                _TopBar(
                  service: service,
                  onSettings: () => setState(() => _section = 6),
                ),
                Expanded(child: _sectionContent(service)),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _sectionContent(RemoteService service) {
    switch (_section) {
      case 6: // Settings
        return const SettingsPage();
      case 2: // Recent
      case 3: // Favorites
        return _RecentPage(onPick: (id) {
          _fillId(id);
          setState(() => _section = 0);
        });
      case 0: // Home
        return _HomeDashboard(
          service: service,
          idController: _idController,
          passwordController: _passwordController,
          onConnect: _connect,
          onPick: _fillId,
        );
      default: // Address book / Discovery / Chat — coming soon
        return _ComingSoon(item: _navItems[_section]);
    }
  }

  // Quick-connect from a recent: drop the id into the field and focus password.
  void _fillId(String id) {
    _idController.text = id;
    _passwordController.clear();
  }

  void _connect() {
    final id = _idController.text.trim();
    if (id.isEmpty) return;
    final relayUrl = ref.read(settingsProvider).relayUrl;
    // Remember this machine so it shows up under Recent connections.
    ref.read(recentConnectionsProvider.notifier).addConnection(
          RecentConnection(id: id, name: id, lastConnected: DateTime.now()),
        );
    ref.read(remoteServiceProvider).connectToHost(
          relayUrl: relayUrl,
          targetId: id,
          password: _passwordController.text,
        );
  }
}

// ---------------------------------------------------------------------------

class _Card extends StatelessWidget {
  final Widget child;
  final EdgeInsets? padding;
  const _Card({required this.child, this.padding});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: padding ?? const EdgeInsets.all(AppSpacing.xl),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadius.card),
        border: Border.all(color: AppColors.border),
        boxShadow: AppShadows.card,
      ),
      child: child,
    );
  }
}

/// Shared search text for filtering Recent connections (top bar -> list).
final _homeSearchProvider = StateProvider<String>((_) => '');

// --- App shell: left icon sidebar ---------------------------------------

class _NavItem {
  final IconData icon;
  final String label;
  const _NavItem(this.icon, this.label);
}

const List<_NavItem> _navItems = [
  _NavItem(Icons.home_rounded, 'Home'),
  _NavItem(Icons.contacts_outlined, 'Contacts'),
  _NavItem(Icons.history_rounded, 'Recent'),
  _NavItem(Icons.star_border_rounded, 'Favorites'),
  _NavItem(Icons.radar_rounded, 'Discovery'),
  _NavItem(Icons.chat_bubble_outline_rounded, 'Chat'),
  _NavItem(Icons.settings_outlined, 'Settings'),
];

class _Sidebar extends StatelessWidget {
  final int selected;
  final bool online;
  final ValueChanged<int> onSelect;
  const _Sidebar(
      {required this.selected, required this.online, required this.onSelect});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 76,
      decoration: const BoxDecoration(
        color: AppColors.surface,
        border: Border(right: BorderSide(color: AppColors.border)),
      ),
      child: Column(
        children: [
          const SizedBox(height: AppSpacing.md),
          for (var i = 0; i < _navItems.length; i++)
            _SidebarItem(
              item: _navItems[i],
              active: i == selected,
              onTap: () => onSelect(i),
            ),
          const Spacer(),
          Padding(
            padding: const EdgeInsets.only(bottom: AppSpacing.lg),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: online ? AppColors.success : AppColors.textTertiary,
                  ),
                ),
                const SizedBox(height: 5),
                Text(online ? 'Online' : 'Offline',
                    style: AppTypography.label.copyWith(fontSize: 10)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _SidebarItem extends StatefulWidget {
  final _NavItem item;
  final bool active;
  final VoidCallback onTap;
  const _SidebarItem(
      {required this.item, required this.active, required this.onTap});
  @override
  State<_SidebarItem> createState() => _SidebarItemState();
}

class _SidebarItemState extends State<_SidebarItem> {
  bool _hover = false;
  @override
  Widget build(BuildContext context) {
    final active = widget.active;
    final fg = active ? AppColors.accentDark : AppColors.textSecondary;
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
          message: widget.item.label,
          waitDuration: const Duration(milliseconds: 500),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 120),
            margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
            padding: const EdgeInsets.symmetric(vertical: 8),
            decoration: BoxDecoration(
              color: bg,
              borderRadius: BorderRadius.circular(AppRadius.md),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(widget.item.icon, size: 21, color: fg),
                const SizedBox(height: 3),
                Text(
                  widget.item.label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: AppTypography.label.copyWith(
                      fontSize: 10,
                      color: fg,
                      fontWeight:
                          active ? FontWeight.w600 : FontWeight.w500),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Home section: the connect + share + recents + security cards.
class _HomeDashboard extends StatelessWidget {
  final RemoteService service;
  final TextEditingController idController;
  final TextEditingController passwordController;
  final VoidCallback onConnect;
  final void Function(String id) onPick;
  const _HomeDashboard({
    required this.service,
    required this.idController,
    required this.passwordController,
    required this.onConnect,
    required this.onPick,
  });

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, c) {
      final wide = c.maxWidth > 860;
      final left = Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _ConnectOutCard(
            service: service,
            idController: idController,
            passwordController: passwordController,
            onConnect: onConnect,
          ),
          const SizedBox(height: AppSpacing.lg),
          _RecentConnectionsCard(onPick: onPick),
        ],
      );
      final right = Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _ThisComputerCard(service: service),
          const SizedBox(height: AppSpacing.lg),
          const _SecurityCard(),
        ],
      );
      return SingleChildScrollView(
        padding: const EdgeInsets.all(AppSpacing.xl),
        child: wide
            ? Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(flex: 6, child: left),
                  const SizedBox(width: AppSpacing.lg),
                  Expanded(flex: 5, child: right),
                ],
              )
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [right, const SizedBox(height: AppSpacing.lg), left],
              ),
      );
    });
  }
}

/// Recent / Favorites section — a full-width recent connections list.
class _RecentPage extends StatelessWidget {
  final void Function(String id) onPick;
  const _RecentPage({required this.onPick});
  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(AppSpacing.xl),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 720),
          child: _RecentConnectionsCard(onPick: onPick),
        ),
      ),
    );
  }
}

/// Placeholder for sections not yet built (Address book / Discovery / Chat).
class _ComingSoon extends StatelessWidget {
  final _NavItem item;
  const _ComingSoon({required this.item});
  @override
  Widget build(BuildContext context) {
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 360),
        child: _EmptyState(
          icon: item.icon,
          title: '${item.label} is coming soon',
          body: 'This section is on the roadmap and will light up in an '
              'upcoming update.',
        ),
      ),
    );
  }
}

/// Top application toolbar: logo, connection status, search, notifications,
/// settings, user chip.
class _TopBar extends ConsumerWidget {
  final RemoteService service;
  final VoidCallback onSettings;
  const _TopBar({required this.service, required this.onSettings});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final online = service.hostStatus == HostStatus.online;
    return Container(
      height: 60,
      decoration: const BoxDecoration(
        color: AppColors.surface,
        border: Border(bottom: BorderSide(color: AppColors.border)),
      ),
      padding: const EdgeInsets.symmetric(horizontal: AppSpacing.lg),
      child: Row(
        children: [
          Container(
            width: 34,
            height: 34,
            decoration: BoxDecoration(
              color: AppColors.primary,
              borderRadius: BorderRadius.circular(AppRadius.sm),
            ),
            child: const Icon(Icons.hub_rounded, color: Colors.white, size: 19),
          ),
          const SizedBox(width: 10),
          Text('Neev Remote', style: AppTypography.title),
          const SizedBox(width: AppSpacing.md),
          _StatusPill(online: online),
          const Spacer(),
          SizedBox(width: 260, height: 40, child: _TopSearchField()),
          const SizedBox(width: AppSpacing.sm),
          _TopIconButton(
            icon: Icons.notifications_none_rounded,
            tooltip: 'Notifications',
            onTap: () => ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(
                  content: Text('You’re all caught up — no new notifications'),
                  duration: Duration(seconds: 2)),
            ),
          ),
          _TopIconButton(
              icon: Icons.settings_outlined,
              tooltip: 'Settings',
              onTap: onSettings),
          const SizedBox(width: AppSpacing.sm),
          _UserChip(),
        ],
      ),
    );
  }
}

class _TopSearchField extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return TextField(
      onChanged: (v) => ref.read(_homeSearchProvider.notifier).state = v,
      textAlignVertical: TextAlignVertical.center,
      style: AppTypography.body,
      decoration: InputDecoration(
        isDense: true,
        hintText: 'Search recent connections',
        prefixIcon: const Icon(Icons.search, size: 18),
        prefixIconConstraints:
            const BoxConstraints(minWidth: 38, minHeight: 38),
        contentPadding: const EdgeInsets.symmetric(vertical: 8),
        fillColor: AppColors.surfaceLight,
      ),
    );
  }
}

class _TopIconButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback onTap;
  const _TopIconButton(
      {required this.icon, required this.tooltip, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return IconButton(
      icon: Icon(icon, size: 20),
      tooltip: tooltip,
      onPressed: onTap,
      style: IconButton.styleFrom(
        foregroundColor: AppColors.textSecondary,
        hoverColor: AppColors.surfaceLight,
        shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(AppRadius.sm)),
      ),
    );
  }
}

class _UserChip extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(6, 5, 12, 5),
      decoration: BoxDecoration(
        color: AppColors.surfaceLight,
        borderRadius: BorderRadius.circular(AppRadius.pill),
      ),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        Container(
          width: 24,
          height: 24,
          decoration: const BoxDecoration(
              color: AppColors.primary, shape: BoxShape.circle),
          alignment: Alignment.center,
          child: const Icon(Icons.person, color: Colors.white, size: 15),
        ),
        const SizedBox(width: 8),
        Text('This PC', style: AppTypography.caption),
      ]),
    );
  }
}

/// Rounded connection-status pill (Online / Offline).
class _StatusPill extends StatelessWidget {
  final bool online;
  const _StatusPill({required this.online});

  @override
  Widget build(BuildContext context) {
    final color = online ? AppColors.success : AppColors.textTertiary;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(AppRadius.pill),
      ),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle)),
        const SizedBox(width: 6),
        Text(online ? 'Online' : 'Offline',
            style: AppTypography.label.copyWith(color: color)),
      ]),
    );
  }
}

/// Left column: enter a partner ID + password to control another machine.
class _ConnectOutCard extends StatelessWidget {
  final RemoteService service;
  final TextEditingController idController;
  final TextEditingController passwordController;
  final VoidCallback onConnect;
  const _ConnectOutCard({
    required this.service,
    required this.idController,
    required this.passwordController,
    required this.onConnect,
  });

  @override
  Widget build(BuildContext context) {
    final connecting = service.viewerStatus == ViewerStatus.connecting;
    final failed = service.viewerStatus == ViewerStatus.failed;
    return _Card(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const _CardHeader(
            icon: Icons.cast_connected_rounded,
            title: 'Connect to a computer',
            subtitle: 'Enter the ID and password shared with you',
          ),
          const SizedBox(height: AppSpacing.lg),
          TextField(
            controller: idController,
            decoration: InputDecoration(
              labelText: 'Partner ID',
              hintText: '123 456 789',
              prefixIcon: const Icon(Icons.link, size: 20),
              errorText: failed ? service.viewerError : null,
            ),
            style: AppTypography.body.copyWith(
                fontSize: 16, letterSpacing: 1, fontWeight: FontWeight.w600),
            onSubmitted: (_) => onConnect(),
          ),
          const SizedBox(height: AppSpacing.md),
          TextField(
            controller: passwordController,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Password',
              prefixIcon: Icon(Icons.lock_outline, size: 20),
            ),
            onSubmitted: (_) => onConnect(),
          ),
          const SizedBox(height: AppSpacing.lg),
          ElevatedButton.icon(
            onPressed: connecting ? null : onConnect,
            icon: connecting
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: Colors.white))
                : const Icon(Icons.arrow_forward_rounded, size: 20),
            label: Text(connecting ? 'Connecting…' : 'Connect'),
          ),
        ],
      ),
    );
  }
}

/// Left column: recent machines, filterable from the top-bar search.
class _RecentConnectionsCard extends ConsumerWidget {
  final void Function(String id) onPick;
  const _RecentConnectionsCard({required this.onPick});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final query = ref.watch(_homeSearchProvider).trim().toLowerCase();
    final all = ref.watch(recentConnectionsProvider);
    final recents = query.isEmpty
        ? all
        : all
            .where((c) =>
                c.id.toLowerCase().contains(query) ||
                c.name.toLowerCase().contains(query))
            .toList();
    return _Card(
      padding: const EdgeInsets.fromLTRB(
          AppSpacing.xl, AppSpacing.lg, AppSpacing.lg, AppSpacing.md),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Text('Recent connections', style: AppTypography.title),
              const Spacer(),
              if (all.isNotEmpty)
                TextButton(
                  onPressed: () =>
                      ref.read(recentConnectionsProvider.notifier).clear(),
                  child: const Text('Clear'),
                ),
            ],
          ),
          const SizedBox(height: AppSpacing.sm),
          if (recents.isEmpty)
            _EmptyState(
              icon: Icons.history_rounded,
              title: query.isEmpty
                  ? 'No recent connections yet'
                  : 'No matches',
              body: query.isEmpty
                  ? 'Machines you connect to will appear here for one-click access.'
                  : 'Try a different ID or name.',
            )
          else
            for (final c in recents) _RecentRow(conn: c, onPick: onPick),
        ],
      ),
    );
  }
}

class _RecentRow extends StatefulWidget {
  final RecentConnection conn;
  final void Function(String id) onPick;
  const _RecentRow({required this.conn, required this.onPick});
  @override
  State<_RecentRow> createState() => _RecentRowState();
}

class _RecentRowState extends State<_RecentRow> {
  bool _hover = false;

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: () => widget.onPick(widget.conn.id),
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 150),
          padding:
              const EdgeInsets.symmetric(horizontal: 10, vertical: 9),
          decoration: BoxDecoration(
            color: _hover ? AppColors.surfaceLight : Colors.transparent,
            borderRadius: BorderRadius.circular(AppRadius.md),
          ),
          child: Row(
            children: [
              Container(
                width: 34,
                height: 34,
                decoration: BoxDecoration(
                    color: AppColors.primarySoft,
                    borderRadius: BorderRadius.circular(AppRadius.sm)),
                alignment: Alignment.center,
                child: const Icon(Icons.computer,
                    size: 18, color: AppColors.primary),
              ),
              const SizedBox(width: AppSpacing.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(widget.conn.id,
                        style: AppTypography.bodyStrong.copyWith(
                            fontFeatures: const [
                              FontFeature.tabularFigures()
                            ])),
                    Text('Last connected recently',
                        style: AppTypography.caption),
                  ],
                ),
              ),
              AnimatedOpacity(
                opacity: _hover ? 1 : 0,
                duration: const Duration(milliseconds: 150),
                child: FilledButton(
                  onPressed: () => widget.onPick(widget.conn.id),
                  style: FilledButton.styleFrom(
                    minimumSize: const Size(0, 34),
                    padding: const EdgeInsets.symmetric(horizontal: 14),
                    textStyle: AppTypography.caption
                        .copyWith(fontWeight: FontWeight.w600),
                  ),
                  child: const Text('Connect'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Right column: security + connection info.
class _SecurityCard extends ConsumerWidget {
  const _SecurityCard();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final relay = ref.watch(settingsProvider).relayUrl;
    final server = Uri.tryParse(relay)?.host ?? relay;
    return _Card(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('Security', style: AppTypography.title),
          const SizedBox(height: AppSpacing.lg),
          const _InfoRow(
            icon: Icons.lock_rounded,
            title: 'End-to-end encrypted',
            value: 'DTLS-SRTP',
            good: true,
          ),
          const SizedBox(height: AppSpacing.md),
          const _InfoRow(
            icon: Icons.hub_outlined,
            title: 'Peer-to-peer',
            value: 'Direct',
            good: true,
          ),
          const SizedBox(height: AppSpacing.md),
          _InfoRow(
            icon: Icons.dns_outlined,
            title: 'Signaling server',
            value: server.isEmpty ? '—' : server,
            good: server.isNotEmpty,
          ),
        ],
      ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  final IconData icon;
  final String title;
  final String value;
  final bool good;
  const _InfoRow(
      {required this.icon,
      required this.title,
      required this.value,
      this.good = false});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          width: 34,
          height: 34,
          decoration: BoxDecoration(
              color: AppColors.surfaceLight,
              borderRadius: BorderRadius.circular(AppRadius.sm)),
          alignment: Alignment.center,
          child: Icon(icon, size: 18, color: AppColors.textSecondary),
        ),
        const SizedBox(width: AppSpacing.md),
        Expanded(child: Text(title, style: AppTypography.body)),
        Text(value,
            style: AppTypography.caption.copyWith(
                color: good ? AppColors.success : AppColors.textSecondary,
                fontWeight: FontWeight.w600)),
      ],
    );
  }
}

class _EmptyState extends StatelessWidget {
  final IconData icon;
  final String title;
  final String body;
  const _EmptyState(
      {required this.icon, required this.title, required this.body});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: AppSpacing.xl),
      child: Column(
        children: [
          Container(
            width: 48,
            height: 48,
            decoration: BoxDecoration(
                color: AppColors.surfaceLight,
                borderRadius: BorderRadius.circular(AppRadius.md)),
            alignment: Alignment.center,
            child: Icon(icon, color: AppColors.textTertiary, size: 24),
          ),
          const SizedBox(height: AppSpacing.md),
          Text(title, style: AppTypography.bodyStrong),
          const SizedBox(height: 4),
          Text(body,
              textAlign: TextAlign.center, style: AppTypography.caption),
        ],
      ),
    );
  }
}

/// Right column: this machine's own ID + password for incoming connections,
/// share state, and unattended access.
class _ThisComputerCard extends ConsumerWidget {
  final RemoteService service;
  const _ThisComputerCard({required this.service});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final online = service.hostStatus == HostStatus.online;
    final busy = service.hostStatus == HostStatus.starting;
    final card = _Card(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              const _CardHeader(
                icon: Icons.desktop_windows_rounded,
                title: 'This computer',
                subtitle: 'Share these so someone can connect to you',
              ),
              const Spacer(),
              if (online)
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  decoration: BoxDecoration(
                      color: AppColors.success.withValues(alpha: 0.12),
                      borderRadius: BorderRadius.circular(AppRadius.pill)),
                  child: Text('${service.connectedViewers} connected',
                      style: AppTypography.label
                          .copyWith(color: AppColors.success)),
                ),
            ],
          ),
          const SizedBox(height: AppSpacing.lg),
          if (online) ...[
            _Credential(label: 'ID', value: service.agentId ?? '…', big: true),
            const SizedBox(height: AppSpacing.sm),
            _Credential(label: 'Password', value: service.password ?? '…'),
            if (service.connectedViewers > 0) ...[
              const SizedBox(height: AppSpacing.md),
              Align(
                alignment: Alignment.centerLeft,
                child: FileShareButtons(service: service),
              ),
              const SizedBox(height: AppSpacing.xs),
              Text('…or drag files onto this card to send them.',
                  style: AppTypography.caption),
            ],
            if (service.fileTransfers.isNotEmpty) ...[
              const SizedBox(height: AppSpacing.md),
              Align(
                alignment: Alignment.centerLeft,
                child: FileTransferList(service: service),
              ),
            ],
            const SizedBox(height: AppSpacing.md),
            Row(children: [
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: () =>
                      ref.read(remoteServiceProvider).stopHosting(),
                  icon: const Icon(Icons.stop_circle_outlined, size: 18),
                  label: const Text('Stop sharing'),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: AppColors.error,
                    side: const BorderSide(color: AppColors.error),
                  ),
                ),
              ),
            ]),
            const SizedBox(height: AppSpacing.md),
            const Divider(height: 1),
            const _UnattendedControls(),
          ] else
            SizedBox(
              width: double.infinity,
              child: ElevatedButton.icon(
                onPressed: busy
                    ? null
                    : () async {
                        final s = ref.read(settingsProvider);
                        try {
                          await ref.read(remoteServiceProvider).startHosting(
                                relayUrl: s.relayUrl,
                                password: s.unattendedPassword.isEmpty
                                    ? null
                                    : s.unattendedPassword,
                              );
                        } catch (_) {}
                      },
                icon: busy
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: Colors.white))
                    : const Icon(Icons.wifi_tethering_rounded, size: 20),
                label: Text(busy ? 'Starting…' : 'Start sharing'),
              ),
            ),
          if (service.hostError != null) ...[
            const SizedBox(height: AppSpacing.md),
            _ErrorText(service.hostError!),
          ],
        ],
      ),
    );
    // While sharing, let the host drag files onto the card to send them.
    return online ? DropToSend(service: service, child: card) : card;
  }
}

class _UnattendedControls extends ConsumerWidget {
  const _UnattendedControls();

  Future<void> _enable(BuildContext context, WidgetRef ref) async {
    final pw = await _askPassword(context);
    if (pw == null || pw.trim().isEmpty) return;
    final notifier = ref.read(settingsProvider.notifier);
    notifier.setUnattendedPassword(pw.trim());
    await notifier.setStartOnBoot(true);
    final service = ref.read(remoteServiceProvider);
    // Multi-user: store the password machine-wide (via the SYSTEM helper) so
    // every account on this PC shares it and it survives user-switching.
    service.setMachinePassword(pw.trim());
    // Re-share so the new fixed password takes effect immediately.
    final relay = ref.read(settingsProvider).relayUrl;
    if (service.isHosting) {
      await service.stopHosting();
      await service.startHosting(relayUrl: relay, password: pw.trim());
    }
  }

  Future<void> _disable(WidgetRef ref) async {
    final notifier = ref.read(settingsProvider.notifier);
    notifier.setUnattendedPassword('');
    await notifier.setStartOnBoot(false);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(settingsProvider);
    final enabled = s.unattendedEnabled;
    return Padding(
      padding: const EdgeInsets.only(top: AppSpacing.sm),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.lock_clock, size: 18),
              const SizedBox(width: 8),
              const Expanded(
                child: Text('Unattended access',
                    style: TextStyle(fontWeight: FontWeight.w600)),
              ),
              Switch(
                value: enabled,
                onChanged: (v) => v ? _enable(context, ref) : _disable(ref),
              ),
            ],
          ),
          Text(
            enabled
                ? (s.startOnBoot
                    ? 'Fixed password set · starts with Windows and re-shares automatically.'
                    : 'Fixed password set · turn on "Start with Windows" to reconnect after a reboot.')
                : 'Set a permanent password so you can reconnect any time — no one needs to re-share, and it survives restarts.',
            style: AppTypography.caption,
          ),
          if (enabled) ...[
            const SizedBox(height: AppSpacing.xs),
            Row(
              children: [
                TextButton.icon(
                  onPressed: () => _enable(context, ref),
                  icon: const Icon(Icons.key, size: 16),
                  label: const Text('Change password'),
                ),
                const Spacer(),
                const Text('Start with Windows',
                    style: TextStyle(fontSize: 12)),
                Switch(
                  value: s.startOnBoot,
                  onChanged: (_) =>
                      ref.read(settingsProvider.notifier).toggleStartOnBoot(),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

Future<String?> _askPassword(BuildContext context) {
  final ctrl = TextEditingController();
  return showDialog<String>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: const Text('Set permanent password'),
      content: TextField(
        controller: ctrl,
        autofocus: true,
        decoration: const InputDecoration(
            hintText: 'Password for unattended access'),
        onSubmitted: (v) => Navigator.pop(ctx, v),
      ),
      actions: [
        TextButton(
            onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
        FilledButton(
            onPressed: () => Navigator.pop(ctx, ctrl.text),
            child: const Text('Save')),
      ],
    ),
  );
}

/// (Ambient animated background removed — the redesigned home is minimal.)
class _CardHeader extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  const _CardHeader(
      {required this.icon, required this.title, required this.subtitle});

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 44,
          height: 44,
          decoration: BoxDecoration(
            gradient: const LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: [AppColors.accent, AppColors.accentDark],
            ),
            borderRadius: BorderRadius.circular(AppRadius.md),
            boxShadow: [
              BoxShadow(
                  color: AppColors.accent.withValues(alpha: 0.32),
                  blurRadius: 12,
                  offset: const Offset(0, 5)),
            ],
          ),
          child: Icon(icon, color: Colors.white, size: 22),
        ),
        const SizedBox(width: AppSpacing.md),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: AppTypography.heading2),
              const SizedBox(height: 2),
              Text(subtitle, style: AppTypography.caption),
            ],
          ),
        ),
      ],
    );
  }
}

class _Credential extends StatelessWidget {
  final String label;
  final String value;
  final bool big;
  const _Credential(
      {required this.label, required this.value, this.big = false});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(AppSpacing.lg, 10, AppSpacing.sm, 10),
      decoration: BoxDecoration(
        color: big ? AppColors.accentSoft : AppColors.surfaceAlt,
        borderRadius: BorderRadius.circular(AppRadius.md),
        border: Border.all(
            color: big ? AppColors.accent.withValues(alpha: 0.45)
                       : AppColors.border),
      ),
      child: Row(
        children: [
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(label.toUpperCase(),
                  style: AppTypography.label
                      .copyWith(color: AppColors.textTertiary)),
              const SizedBox(height: 2),
              Text(
                value,
                style: TextStyle(
                  fontSize: big ? 26 : 18,
                  fontWeight: FontWeight.w700,
                  fontFeatures: const [FontFeature.tabularFigures()],
                  color: big ? AppColors.accentDark : AppColors.textPrimary,
                  letterSpacing: big ? 2 : 1,
                ),
              ),
            ],
          ),
          const Spacer(),
          IconButton(
            icon: const Icon(Icons.content_copy_rounded, size: 18),
            tooltip: 'Copy $label',
            style: IconButton.styleFrom(
              foregroundColor: AppColors.accentDark,
              backgroundColor: AppColors.surface,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(AppRadius.sm),
                side: const BorderSide(color: AppColors.border),
              ),
            ),
            onPressed: () {
              Clipboard.setData(ClipboardData(text: value));
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(
                    content: Text('$label copied'),
                    duration: const Duration(seconds: 1)),
              );
            },
          ),
        ],
      ),
    );
  }
}

class _ErrorText extends StatelessWidget {
  final String message;
  const _ErrorText(this.message);

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        const Icon(Icons.error_outline, color: AppColors.error, size: 18),
        const SizedBox(width: AppSpacing.sm),
        Expanded(
          child: Text(message,
              style: AppTypography.caption.copyWith(color: AppColors.error)),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------

class _ConnectedSession extends ConsumerWidget {
  final RemoteService service;
  const _ConnectedSession({required this.service});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final viewOnly =
        ref.watch(settingsProvider).viewOnly || service.viewerViewOnly;
    return Scaffold(
      backgroundColor: const Color(0xFF0B0F1A),
      body: DropToSend(
        service: service,
        child: Stack(
          children: [
            Positioned.fill(
              child: RemoteViewWidget(
                isConnected: true,
                remoteStream: service.remoteStream,
                viewOnly: viewOnly,
                hostOs: service.remoteHostOs,
                onInput: viewOnly
                    ? null
                    : (event) =>
                        ref.read(remoteServiceProvider).sendViewerInput(event),
                uacActive: service.uacActive,
                uacFrame: service.uacFrame,
                uacW: service.uacW,
                uacH: service.uacH,
                uacKind: service.uacKind,
                onUacClick: (b, x, y) =>
                    ref.read(remoteServiceProvider).sendUacClick(b, x, y),
                onUacApprove: () =>
                    ref.read(remoteServiceProvider).sendUacApprove(),
                onUacDecline: () =>
                    ref.read(remoteServiceProvider).sendUacDecline(),
              ),
            ),
            Positioned(
              right: AppSpacing.lg,
              bottom: 96,
              child: FileTransferList(service: service),
            ),
            // Floating command bar, centered along the bottom.
            Positioned(
              left: 0,
              right: 0,
              bottom: AppSpacing.lg,
              child: Center(child: _SessionToolbar(service: service)),
            ),
          ],
        ),
      ),
    );
  }
}

/// Premium in-session control bar: a status/stats cluster on the left and
/// clearly-labeled, grouped controls on the right so every action is trackable
/// (the old bar was icon-only and ambiguous).
class _SessionToolbar extends ConsumerWidget {
  final RemoteService service;
  const _SessionToolbar({required this.service});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final stats = service.stats;
    final win = service.remoteHostOs == 'windows';

    return Container(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(AppRadius.xl),
        boxShadow: AppShadows.float,
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(AppRadius.xl),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 16, sigmaY: 16),
          child: Container(
            decoration: BoxDecoration(
              color: AppColors.surface.withValues(alpha: 0.94),
              borderRadius: BorderRadius.circular(AppRadius.xl),
              border: Border.all(color: AppColors.border),
            ),
            padding: const EdgeInsets.symmetric(
                horizontal: AppSpacing.md, vertical: AppSpacing.xs),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                _ConnectionBadge(id: service.targetId ?? '—'),
                const SizedBox(width: AppSpacing.sm),
                _StatsStrip(stats: stats),
                const _ToolDivider(),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                // --- Control group ---
                _ToolButton(
                  icon: service.viewerViewOnly
                      ? Icons.visibility_outlined
                      : Icons.ads_click,
                  label: service.viewerViewOnly ? 'View only' : 'Control',
                  tooltip: service.viewerViewOnly
                      ? 'View only — click to take control'
                      : 'Controlling — click for view only',
                  active: !service.viewerViewOnly,
                  onPressed: () => service.setViewOnly(!service.viewerViewOnly),
                ),
                if (service.keyboardCaptureSupported)
                  _ToolButton(
                    icon: service.keyboardCapture
                        ? Icons.keyboard_alt
                        : Icons.keyboard_alt_outlined,
                    label: 'Keyboard',
                    tooltip: service.keyboardCapture
                        ? 'Keyboard capture ON — Win+R, Alt+Tab etc. go to the '
                            'remote. Click away to stop.'
                        : 'Capture keyboard — send Win+R, Alt+Tab etc. by '
                            'pressing them',
                    active: service.keyboardCapture,
                    onPressed: () =>
                        service.setKeyboardCapture(!service.keyboardCapture),
                  ),
                ShortcutsMenu(service: service),
                if (service.hostMonitors.length > 1)
                  _MonitorButton(service: service),
                const _ToolDivider(),
                // --- Files group ---
                _ToolButton(
                  icon: Icons.upload_file,
                  label: 'Export',
                  tooltip: 'Send a file to the connected computer',
                  onPressed: () => pickAndSendFile(context, service),
                ),
                _ToolButton(
                  icon: Icons.download_for_offline_outlined,
                  label: 'Import',
                  tooltip: 'Ask the connected computer to send you a file',
                  onPressed: () {
                    service.requestFileFromPeer();
                    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
                      content: Text(
                          'Import requested — the other computer picks a file '
                          'to send'),
                      duration: Duration(seconds: 3),
                    ));
                  },
                ),
                const _ToolDivider(),
                // --- Session group ---
                if (win)
                  _ToolButton(
                    icon: Icons.password_rounded,
                    label: 'Login',
                    tooltip: 'Transmit a username + password to the remote '
                        'UAC / login prompt',
                    onPressed: () => _showTransmitCredentials(context, service),
                  ),
                if (win)
                  _ToolButton(
                    icon: Icons.blur_on,
                    label: 'Privacy',
                    tooltip: service.privacyMode
                        ? 'Privacy ON — host screen blanked + its input blocked'
                        : 'Privacy mode — blank the host screen + block its '
                            'local input',
                    active: service.privacyMode,
                    onPressed: () =>
                        service.setPrivacyMode(!service.privacyMode),
                  ),
                _ToolButton(
                  icon: Icons.restart_alt,
                  label: 'Restart',
                  tooltip: 'Restart the remote PC',
                  onPressed: () => _confirmRestart(context, service),
                ),
                const SizedBox(width: AppSpacing.sm),
                _DisconnectButton(
                  onPressed: () =>
                      ref.read(remoteServiceProvider).disconnectViewer(),
                ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Future<void> _confirmRestart(
      BuildContext context, RemoteService service) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Restart remote PC?'),
        content: const Text(
            'The remote computer will reboot now. Neev Remote will keep trying '
            'to reconnect for a few minutes once it\'s back (the host must be '
            'set to start on boot).'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Restart')),
        ],
      ),
    );
    if (ok == true) service.rebootHost();
  }

  Future<void> _showTransmitCredentials(
      BuildContext context, RemoteService service) async {
    final userCtrl = TextEditingController();
    final passCtrl = TextEditingController();
    void toast(String msg) => ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(msg), duration: const Duration(seconds: 1)));
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Row(children: [
          const Icon(Icons.password_rounded,
              color: AppColors.accentDark, size: 20),
          const SizedBox(width: AppSpacing.sm),
          const Text('Transmit login'),
        ]),
        content: SizedBox(
          width: 380,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                'Sends your typed username / password to the remote so you '
                'never reveal them on screen. Click the target field on the '
                'remote first, then type or send both.',
                style: AppTypography.caption,
              ),
              const SizedBox(height: AppSpacing.lg),
              TextField(
                controller: userCtrl,
                decoration: const InputDecoration(
                    labelText: 'Username',
                    prefixIcon: Icon(Icons.person_outline)),
              ),
              const SizedBox(height: AppSpacing.md),
              TextField(
                controller: passCtrl,
                obscureText: true,
                decoration: const InputDecoration(
                    labelText: 'Password',
                    prefixIcon: Icon(Icons.lock_outline)),
              ),
              const SizedBox(height: AppSpacing.sm),
              Row(
                children: [
                  Expanded(
                    child: OutlinedButton(
                      onPressed: () {
                        service.transmitText(userCtrl.text);
                        toast('Username sent');
                      },
                      child: const Text('Type username'),
                    ),
                  ),
                  const SizedBox(width: AppSpacing.sm),
                  Expanded(
                    child: OutlinedButton(
                      onPressed: () {
                        service.transmitText(passCtrl.text);
                        toast('Password sent');
                      },
                      child: const Text('Type password'),
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx), child: const Text('Close')),
          FilledButton.icon(
            onPressed: () {
              // Username ⇥ then password ⏎ — ordered/reliable channel keeps the
              // two type messages in sequence.
              service.transmitText(userCtrl.text, tab: true);
              service.transmitText(passCtrl.text, enter: true);
              Navigator.pop(ctx);
              toast('Login transmitted');
            },
            icon: const Icon(Icons.send_rounded, size: 18),
            label: const Text('Send user ⇥ pass ⏎'),
          ),
        ],
      ),
    );
  }
}

/// Green pulse dot + "Connected to <id>" pill.
class _ConnectionBadge extends StatelessWidget {
  final String id;
  const _ConnectionBadge({required this.id});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: AppColors.success.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(AppRadius.pill),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: const BoxDecoration(
                color: AppColors.success, shape: BoxShape.circle),
          ),
          const SizedBox(width: 8),
          Text('Connected', style: AppTypography.label.copyWith(
              color: AppColors.success, fontWeight: FontWeight.w700)),
          const SizedBox(width: 6),
          Text(id, style: AppTypography.caption.copyWith(
              fontFeatures: const [FontFeature.tabularFigures()],
              color: AppColors.textPrimary)),
        ],
      ),
    );
  }
}

/// Compact live-stats strip (fps · latency · bitrate). Codec + decoded frames
/// are surfaced on hover so a blank session can still be diagnosed.
class _StatsStrip extends StatelessWidget {
  final dynamic stats;
  const _StatsStrip({required this.stats});

  @override
  Widget build(BuildContext context) {
    Widget item(IconData ic, String v) => Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            Icon(ic, size: 13, color: AppColors.textTertiary),
            const SizedBox(width: 4),
            Text(v,
                style: AppTypography.caption.copyWith(
                    fontFeatures: const [FontFeature.tabularFigures()])),
          ]),
        );
    return Tooltip(
      message:
          'Codec ${stats.codec ?? '—'} · ${stats.framesDecoded ?? 0} frames decoded',
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 6),
        decoration: BoxDecoration(
          color: AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(AppRadius.sm),
        ),
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          item(Icons.speed, '${stats.fps ?? 0} fps'),
          item(Icons.network_ping, '${stats.latencyMs ?? 0} ms'),
          item(Icons.bar_chart, '${stats.bitrateKbps ?? 0} kbps'),
        ]),
      ),
    );
  }
}

/// A single labeled toolbar action: icon over a small caption, hover + active
/// states. Labels make every control immediately recognisable.
class _ToolButton extends StatefulWidget {
  final IconData icon;
  final String label;
  final String tooltip;
  final bool active;
  final VoidCallback onPressed;
  const _ToolButton({
    required this.icon,
    required this.label,
    required this.tooltip,
    required this.onPressed,
    this.active = false,
  });

  @override
  State<_ToolButton> createState() => _ToolButtonState();
}

class _ToolButtonState extends State<_ToolButton> {
  bool _hover = false;

  @override
  Widget build(BuildContext context) {
    final active = widget.active;
    final fg = active ? AppColors.accentDark : AppColors.textSecondary;
    final bg = active
        ? AppColors.accentSoft
        : (_hover ? AppColors.surfaceLight : Colors.transparent);
    return Tooltip(
      message: widget.tooltip,
      waitDuration: const Duration(milliseconds: 400),
      child: MouseRegion(
        onEnter: (_) => setState(() => _hover = true),
        onExit: (_) => setState(() => _hover = false),
        cursor: SystemMouseCursors.click,
        child: GestureDetector(
          onTap: widget.onPressed,
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 120),
            width: 62,
            padding: const EdgeInsets.symmetric(vertical: 7),
            margin: const EdgeInsets.symmetric(horizontal: 2),
            decoration: BoxDecoration(
              color: bg,
              borderRadius: BorderRadius.circular(AppRadius.md),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(widget.icon, size: 20, color: fg),
                const SizedBox(height: 4),
                Text(
                  widget.label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: AppTypography.label.copyWith(
                      color: fg,
                      fontWeight:
                          active ? FontWeight.w700 : FontWeight.w600),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Monitor switcher styled as a [_ToolButton] with a dropdown.
class _MonitorButton extends StatelessWidget {
  final RemoteService service;
  const _MonitorButton({required this.service});

  @override
  Widget build(BuildContext context) {
    return PopupMenuButton<String>(
      tooltip: 'Switch monitor',
      position: PopupMenuPosition.under,
      onSelected: service.setMonitor,
      itemBuilder: (_) => [
        for (var i = 0; i < service.hostMonitors.length; i++)
          PopupMenuItem<String>(
            value: service.hostMonitors[i]['id'],
            child: Text(
              (service.hostMonitors[i]['n'] ?? '').isNotEmpty
                  ? service.hostMonitors[i]['n']!
                  : 'Monitor ${i + 1}',
              style: AppTypography.body,
            ),
          ),
      ],
      child: Container(
        width: 62,
        padding: const EdgeInsets.symmetric(vertical: 7),
        margin: const EdgeInsets.symmetric(horizontal: 2),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.monitor, size: 20, color: AppColors.textSecondary),
            const SizedBox(height: 4),
            Text('Monitor',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: AppTypography.label),
          ],
        ),
      ),
    );
  }
}

class _ToolDivider extends StatelessWidget {
  const _ToolDivider();
  @override
  Widget build(BuildContext context) => Container(
        width: 1,
        height: 30,
        margin: const EdgeInsets.symmetric(horizontal: AppSpacing.sm),
        color: AppColors.border,
      );
}

/// Prominent red pill for the one destructive action.
class _DisconnectButton extends StatelessWidget {
  final VoidCallback onPressed;
  const _DisconnectButton({required this.onPressed});

  @override
  Widget build(BuildContext context) {
    return FilledButton.icon(
      onPressed: onPressed,
      icon: const Icon(Icons.call_end_rounded, size: 18),
      label: const Text('Disconnect'),
      style: FilledButton.styleFrom(
        backgroundColor: AppColors.error,
        foregroundColor: Colors.white,
        padding:
            const EdgeInsets.symmetric(horizontal: AppSpacing.lg, vertical: 12),
      ),
    );
  }
}

/// First-run card: ask for the server address so the same installer works
/// against any deployment. Shown only when no server is baked in or saved.
class _ServerSetupCard extends ConsumerStatefulWidget {
  final VoidCallback onSaved;
  const _ServerSetupCard({required this.onSaved});

  @override
  ConsumerState<_ServerSetupCard> createState() => _ServerSetupCardState();
}

class _ServerSetupCardState extends ConsumerState<_ServerSetupCard> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _save() {
    final url = normalizeRelayUrl(_controller.text);
    if (url.isEmpty) return;
    ref.read(settingsProvider.notifier).updateRelayUrl(url);
    widget.onSaved();
  }

  @override
  Widget build(BuildContext context) {
    return _Card(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Icon(Icons.dns_outlined, color: AppColors.accent, size: 40),
          const SizedBox(height: AppSpacing.md),
          Text('Connect to your server', style: AppTypography.heading1),
          const SizedBox(height: AppSpacing.xs),
          Text(
            'Enter the address of your Neev Remote server (the one you '
            'downloaded this app from).',
            style: AppTypography.caption,
          ),
          const SizedBox(height: AppSpacing.lg),
          TextField(
            controller: _controller,
            autofocus: true,
            decoration: const InputDecoration(
              labelText: 'Server address',
              hintText: 'e.g. 192.168.1.10  or  remote.company.com',
              prefixIcon: Icon(Icons.public),
            ),
            onSubmitted: (_) => _save(),
          ),
          const SizedBox(height: AppSpacing.lg),
          ElevatedButton.icon(
            onPressed: _save,
            icon: const Icon(Icons.arrow_forward),
            label: const Text('Save & Continue'),
            style: ElevatedButton.styleFrom(
                padding: const EdgeInsets.all(AppSpacing.md)),
          ),
        ],
      ),
    );
  }
}

