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
          title: const Text('Neev Remote', style: AppTypography.heading2),
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
      appBar: AppBar(
        backgroundColor: AppColors.surface,
        elevation: 0,
        title: Row(
          children: [
            const Icon(Icons.desktop_windows, color: AppColors.primary),
            const SizedBox(width: AppSpacing.sm),
            const Text('Neev Remote', style: AppTypography.heading2),
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            tooltip: 'Settings',
            onPressed: () => Navigator.of(context).push(
              MaterialPageRoute(
                builder: (_) => Scaffold(
                  appBar: AppBar(
                    backgroundColor: AppColors.surface,
                    title: const Text('Settings'),
                  ),
                  body: const SettingsPage(),
                ),
              ),
            ),
          ),
        ],
      ),
      body: Container(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Color(0xFFEDF2FB), Color(0xFFF6F8FC)],
          ),
        ),
        child: LayoutBuilder(
          builder: (context, constraints) {
            final wide = constraints.maxWidth > 820;
            final share = _ShareCard(service: service);
            final connect = _ConnectCard(
              service: service,
              idController: _idController,
              passwordController: _passwordController,
              onConnect: _connect,
            );
            return SingleChildScrollView(
              padding: const EdgeInsets.symmetric(
                  horizontal: AppSpacing.xl, vertical: AppSpacing.xxl),
              child: Center(
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 900),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      _HeroBanner(service: service),
                      const SizedBox(height: AppSpacing.xl),
                      if (wide)
                        Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Expanded(child: share),
                            const SizedBox(width: AppSpacing.lg),
                            Expanded(child: connect),
                          ],
                        )
                      else ...[
                        share,
                        const SizedBox(height: AppSpacing.lg),
                        connect,
                      ],
                      const SizedBox(height: AppSpacing.xl),
                      const _HowItWorks(),
                    ],
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }

  void _connect() {
    final id = _idController.text.trim();
    if (id.isEmpty) return;
    final relayUrl = ref.read(settingsProvider).relayUrl;
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
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: const Color(0xFFE8EDF4)),
        boxShadow: const [
          BoxShadow(
            color: Color(0x14101828),
            blurRadius: 28,
            offset: Offset(0, 12),
          ),
          BoxShadow(
            color: Color(0x0A101828),
            blurRadius: 4,
            offset: Offset(0, 1),
          ),
        ],
      ),
      child: child,
    );
  }
}

/// Gradient welcome banner across the top — adds depth/brand and fills space.
class _HeroBanner extends StatelessWidget {
  final RemoteService service;
  const _HeroBanner({required this.service});

  @override
  Widget build(BuildContext context) {
    final online = service.hostStatus == HostStatus.online;
    return Container(
      padding: const EdgeInsets.symmetric(
          horizontal: AppSpacing.xl, vertical: AppSpacing.lg),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(18),
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFF2D6CFF), Color(0xFF1E40AF)],
        ),
        boxShadow: const [
          BoxShadow(
              color: Color(0x382D6CFF), blurRadius: 30, offset: Offset(0, 14)),
        ],
      ),
      child: Row(
        children: [
          Container(
            width: 46,
            height: 46,
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.18),
              borderRadius: BorderRadius.circular(12),
            ),
            child: const Icon(Icons.bolt_rounded, color: Colors.white, size: 26),
          ),
          const SizedBox(width: AppSpacing.lg),
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Welcome to Neev Remote',
                    style: TextStyle(
                        color: Colors.white,
                        fontSize: 21,
                        fontWeight: FontWeight.bold)),
                SizedBox(height: 3),
                Text('Securely view and control any computer, anywhere.',
                    style: TextStyle(color: Color(0xCCFFFFFF), fontSize: 13)),
              ],
            ),
          ),
          Container(
            padding:
                const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.16),
              borderRadius: BorderRadius.circular(999),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    color: online ? const Color(0xFF4ADE80) : Colors.white70,
                    shape: BoxShape.circle,
                  ),
                ),
                const SizedBox(width: 7),
                Text(online ? 'Ready to receive' : 'Starting…',
                    style: const TextStyle(
                        color: Colors.white,
                        fontSize: 12,
                        fontWeight: FontWeight.w600)),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// Three-step explainer row that fills the lower space tastefully.
class _HowItWorks extends StatelessWidget {
  const _HowItWorks();

  @override
  Widget build(BuildContext context) {
    const steps = [
      (Icons.tag_rounded, 'Share your ID',
          'Give your ID + password to whoever should connect.'),
      (Icons.cast_connected_rounded, 'Or connect out',
          'Enter a partner ID + password to control their screen.'),
      (Icons.lock_rounded, 'Encrypted & direct',
          'Sessions are peer-to-peer and end-to-end encrypted.'),
    ];
    return LayoutBuilder(builder: (context, c) {
      final wide = c.maxWidth > 680;
      final cards = [
        for (final s in steps) _StepCard(icon: s.$1, title: s.$2, body: s.$3),
      ];
      if (!wide) {
        return Column(
          children: [
            for (var i = 0; i < cards.length; i++) ...[
              if (i > 0) const SizedBox(height: AppSpacing.md),
              cards[i],
            ],
          ],
        );
      }
      return Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (var i = 0; i < cards.length; i++) ...[
            if (i > 0) const SizedBox(width: AppSpacing.md),
            Expanded(child: cards[i]),
          ],
        ],
      );
    });
  }
}

class _StepCard extends StatelessWidget {
  final IconData icon;
  final String title;
  final String body;
  const _StepCard(
      {required this.icon, required this.title, required this.body});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(AppSpacing.lg),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.7),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: const Color(0xFFE8EDF4)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 36,
            height: 36,
            decoration: BoxDecoration(
              color: AppColors.primary.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(10),
            ),
            child: Icon(icon, color: AppColors.primary, size: 18),
          ),
          const SizedBox(height: AppSpacing.sm),
          Text(title,
              style: AppTypography.body
                  .copyWith(fontWeight: FontWeight.w600)),
          const SizedBox(height: 2),
          Text(body, style: AppTypography.caption),
        ],
      ),
    );
  }
}

class _ShareCard extends ConsumerWidget {
  final RemoteService service;
  const _ShareCard({required this.service});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final online = service.hostStatus == HostStatus.online;
    final busy = service.hostStatus == HostStatus.starting;
    return _Card(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const _CardHeader(
            icon: Icons.screen_share,
            title: 'Share my screen',
            subtitle: 'Let someone connect to this computer',
          ),
          const SizedBox(height: AppSpacing.lg),
          if (online) ...[
            _Credential(label: 'ID', value: service.agentId ?? '…', big: true),
            const SizedBox(height: AppSpacing.sm),
            _Credential(label: 'Password', value: service.password ?? '…'),
            const SizedBox(height: AppSpacing.xs),
            Text('${service.connectedViewers} connected',
                style: AppTypography.caption),
            const SizedBox(height: AppSpacing.lg),
            OutlinedButton.icon(
              onPressed: () => ref.read(remoteServiceProvider).stopHosting(),
              icon: const Icon(Icons.stop, size: 18),
              label: const Text('Stop sharing'),
              style: OutlinedButton.styleFrom(
                foregroundColor: AppColors.error,
                side: const BorderSide(color: AppColors.error),
              ),
            ),
            if (service.connectedViewers > 0) ...[
              const SizedBox(height: AppSpacing.md),
              Align(
                alignment: Alignment.centerLeft,
                child: FileShareButtons(service: service),
              ),
            ],
            if (service.fileTransfers.isNotEmpty) ...[
              const SizedBox(height: AppSpacing.md),
              Align(
                alignment: Alignment.centerLeft,
                child: FileTransferList(service: service),
              ),
            ],
            const SizedBox(height: AppSpacing.md),
            const Divider(height: 1),
            const _UnattendedControls(),
          ] else
            ElevatedButton.icon(
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
                  : const Icon(Icons.play_arrow),
              label: Text(busy ? 'Starting…' : 'Start sharing'),
              style: ElevatedButton.styleFrom(
                backgroundColor: AppColors.success,
                padding: const EdgeInsets.all(AppSpacing.md),
              ),
            ),
          if (service.hostError != null) ...[
            const SizedBox(height: AppSpacing.md),
            _ErrorText(service.hostError!),
          ],
        ],
      ),
    );
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

class _ConnectCard extends StatelessWidget {
  final RemoteService service;
  final TextEditingController idController;
  final TextEditingController passwordController;
  final VoidCallback onConnect;

  const _ConnectCard({
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
            icon: Icons.cast_connected,
            title: 'Connect to a computer',
            subtitle: 'Enter the ID and password shared with you',
          ),
          const SizedBox(height: AppSpacing.lg),
          TextField(
            controller: idController,
            decoration: InputDecoration(
              labelText: 'Partner ID',
              hintText: '123-456-789',
              prefixIcon: const Icon(Icons.link),
              errorText: failed ? service.viewerError : null,
            ),
            textAlign: TextAlign.center,
            style: const TextStyle(
                fontSize: 18, fontFamily: 'monospace', letterSpacing: 2),
            onSubmitted: (_) => onConnect(),
          ),
          const SizedBox(height: AppSpacing.md),
          TextField(
            controller: passwordController,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Password',
              prefixIcon: Icon(Icons.lock_outline),
            ),
            textAlign: TextAlign.center,
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
                : const Icon(Icons.connect_without_contact),
            label: Text(connecting ? 'Connecting…' : 'Connect'),
            style: ElevatedButton.styleFrom(
                padding: const EdgeInsets.all(AppSpacing.md)),
          ),
        ],
      ),
    );
  }
}

class _CardHeader extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  const _CardHeader(
      {required this.icon, required this.title, required this.subtitle});

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: AppColors.primary.withValues(alpha: 0.1),
                borderRadius: BorderRadius.circular(11),
              ),
              child: Icon(icon, color: AppColors.primary, size: 20),
            ),
            const SizedBox(width: AppSpacing.md),
            Expanded(child: Text(title, style: AppTypography.heading2)),
          ],
        ),
        const SizedBox(height: AppSpacing.md),
        Text(subtitle, style: AppTypography.caption),
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
      padding: const EdgeInsets.symmetric(
          horizontal: AppSpacing.md, vertical: AppSpacing.sm),
      decoration: BoxDecoration(
        color: AppColors.surfaceLight,
        borderRadius: BorderRadius.circular(AppRadius.md),
      ),
      child: Row(
        children: [
          SizedBox(
            width: 78,
            child: Text(label, style: AppTypography.caption),
          ),
          Expanded(
            child: Text(
              value,
              style: TextStyle(
                fontSize: big ? 24 : 18,
                fontWeight: FontWeight.bold,
                fontFamily: 'monospace',
                color: big ? AppColors.primary : AppColors.textPrimary,
                letterSpacing: 1.5,
              ),
            ),
          ),
          IconButton(
            icon: const Icon(Icons.copy, size: 18),
            tooltip: 'Copy',
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
    final stats = service.stats;
    final viewOnly =
        ref.watch(settingsProvider).viewOnly || service.viewerViewOnly;
    return Scaffold(
      body: Column(
        children: [
          Expanded(
            child: Padding(
              padding: const EdgeInsets.all(AppSpacing.sm),
              child: DropToSend(
                service: service,
                child: Stack(
                  children: [
                    RemoteViewWidget(
                      isConnected: true,
                      remoteStream: service.remoteStream,
                      viewOnly: viewOnly,
                      hostOs: service.remoteHostOs,
                      onInput: viewOnly
                          ? null
                          : (event) => ref
                              .read(remoteServiceProvider)
                              .sendViewerInput(event),
                      uacActive: service.uacActive,
                      uacFrame: service.uacFrame,
                      uacW: service.uacW,
                      uacH: service.uacH,
                      onUacClick: (b, x, y) =>
                          ref.read(remoteServiceProvider).sendUacClick(b, x, y),
                      onUacApprove: () =>
                          ref.read(remoteServiceProvider).sendUacApprove(),
                      onUacDecline: () =>
                          ref.read(remoteServiceProvider).sendUacDecline(),
                    ),
                    Positioned(
                      right: AppSpacing.md,
                      bottom: AppSpacing.md,
                      child: FileTransferList(service: service),
                    ),
                  ],
                ),
              ),
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(
                horizontal: AppSpacing.lg, vertical: AppSpacing.sm),
            color: AppColors.surface,
            child: Row(
              children: [
                const Icon(Icons.circle, color: AppColors.success, size: 10),
                const SizedBox(width: AppSpacing.sm),
                Text('Connected to ${service.targetId}',
                    style: AppTypography.body),
                const SizedBox(width: AppSpacing.md),
                // Full diagnostic line (always visible) so a blank session can
                // be pinpointed: kbps>0 + 0 frames = receiving but not decoding;
                // 0 kbps = no media arriving (path/host); codec shows what was
                // negotiated.
                _StatChip(Icons.speed, '${stats.fps ?? 0} fps'),
                _StatChip(Icons.network_ping, '${stats.latencyMs ?? 0} ms'),
                _StatChip(Icons.bar_chart, '${stats.bitrateKbps ?? 0} kbps'),
                _StatChip(Icons.movie, stats.codec ?? '—'),
                _StatChip(Icons.photo_library, '${stats.framesDecoded ?? 0} frames'),
                const Spacer(),
                if (service.hostMonitors.length > 1)
                  PopupMenuButton<String>(
                    tooltip: 'Switch monitor',
                    icon: const Icon(Icons.monitor, size: 20),
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
                            style: const TextStyle(fontSize: 13),
                          ),
                        ),
                    ],
                  ),
                IconButton(
                  tooltip: 'Restart the remote PC',
                  icon: const Icon(Icons.restart_alt, size: 20),
                  onPressed: () async {
                    final ok = await showDialog<bool>(
                      context: context,
                      builder: (ctx) => AlertDialog(
                        title: const Text('Restart remote PC?'),
                        content: const Text(
                            'The remote computer will reboot now. Neev Remote '
                            'will keep trying to reconnect for a few minutes once '
                            "it's back (the host must be set to start on boot)."),
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
                  },
                ),
                if (service.remoteHostOs == 'windows')
                  IconButton(
                    tooltip: service.privacyMode
                        ? 'Privacy ON — host screen blanked + its input blocked'
                        : 'Privacy mode — blank the host screen + block its '
                            'local input',
                    isSelected: service.privacyMode,
                    icon: const Icon(Icons.blur_on, size: 20),
                    selectedIcon: const Icon(Icons.blur_on,
                        size: 20, color: AppColors.primary),
                    onPressed: () =>
                        service.setPrivacyMode(!service.privacyMode),
                  ),
                IconButton(
                  tooltip: service.viewerViewOnly
                      ? 'View only — click to take control'
                      : 'Controlling — click for view only',
                  icon: Icon(
                      service.viewerViewOnly
                          ? Icons.visibility_outlined
                          : Icons.ads_click,
                      size: 20),
                  onPressed: () =>
                      service.setViewOnly(!service.viewerViewOnly),
                ),
                if (service.keyboardCaptureSupported)
                  IconButton(
                    tooltip: service.keyboardCapture
                        ? 'Keyboard capture ON — Win+R, Alt+Tab etc. go to the '
                            'remote. Click away to stop.'
                        : 'Capture keyboard — send Win+R, Alt+Tab etc. by '
                            'pressing them',
                    isSelected: service.keyboardCapture,
                    icon: const Icon(Icons.keyboard_alt_outlined, size: 20),
                    selectedIcon: const Icon(Icons.keyboard_alt,
                        size: 20, color: AppColors.primary),
                    onPressed: () =>
                        service.setKeyboardCapture(!service.keyboardCapture),
                  ),
                ShortcutsMenu(service: service),
                const SizedBox(width: AppSpacing.xs),
                FileShareButtons(service: service, dense: true),
                const SizedBox(width: AppSpacing.sm),
                OutlinedButton.icon(
                  onPressed: () =>
                      ref.read(remoteServiceProvider).disconnectViewer(),
                  icon: const Icon(Icons.close, size: 18),
                  label: const Text('Disconnect'),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: AppColors.error,
                    side: const BorderSide(color: AppColors.error),
                  ),
                ),
              ],
            ),
          ),
        ],
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
          const Icon(Icons.dns_outlined, color: AppColors.primary, size: 40),
          const SizedBox(height: AppSpacing.md),
          const Text('Connect to your server', style: AppTypography.heading1),
          const SizedBox(height: AppSpacing.xs),
          const Text(
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

/// A small icon + label pill used in the connected-session status bar.
class _StatChip extends StatelessWidget {
  final IconData icon;
  final String label;
  const _StatChip(this.icon, this.label);

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(right: AppSpacing.sm),
      child: Container(
        padding: const EdgeInsets.symmetric(
            horizontal: AppSpacing.sm, vertical: AppSpacing.xs),
        decoration: BoxDecoration(
          color: AppColors.surfaceLight,
          borderRadius: BorderRadius.circular(AppRadius.sm),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 12, color: AppColors.textSecondary),
            const SizedBox(width: AppSpacing.xs),
            Text(label, style: AppTypography.caption),
          ],
        ),
      ),
    );
  }
}
