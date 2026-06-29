import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/theme/app_theme.dart';
import '../../data/services/remote_service.dart';
import '../providers/app_providers.dart';
import '../widgets/remote_view_widget.dart';
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
    _autoStarted = true;
    final service = ref.read(remoteServiceProvider);
    if (service.isHosting) return;
    final relayUrl = ref.read(settingsProvider).relayUrl;
    try {
      await service.startHosting(relayUrl: relayUrl);
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

    // Active remote session takes the whole window.
    if (service.viewerStatus == ViewerStatus.connected) {
      return _ConnectedSession(service: service);
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
      body: LayoutBuilder(
        builder: (context, constraints) {
          final share = _ShareCard(service: service);
          final connect = _ConnectCard(
            service: service,
            idController: _idController,
            passwordController: _passwordController,
            onConnect: _connect,
          );
          final wide = constraints.maxWidth > 760;
          return SingleChildScrollView(
            padding: const EdgeInsets.all(AppSpacing.xl),
            child: Center(
              child: wide
                  ? Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        SizedBox(width: 380, child: share),
                        const SizedBox(width: AppSpacing.xl),
                        SizedBox(width: 380, child: connect),
                      ],
                    )
                  : Column(
                      children: [
                        share,
                        const SizedBox(height: AppSpacing.xl),
                        connect,
                      ],
                    ),
            ),
          );
        },
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
  const _Card({required this.child});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(AppSpacing.xl),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadius.lg),
        border: Border.all(color: AppColors.border),
      ),
      child: child,
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
          ] else
            ElevatedButton.icon(
              onPressed: busy
                  ? null
                  : () async {
                      final relayUrl = ref.read(settingsProvider).relayUrl;
                      try {
                        await ref
                            .read(remoteServiceProvider)
                            .startHosting(relayUrl: relayUrl);
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
            Icon(icon, color: AppColors.primary),
            const SizedBox(width: AppSpacing.sm),
            Expanded(child: Text(title, style: AppTypography.heading2)),
          ],
        ),
        const SizedBox(height: AppSpacing.xs),
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
    final viewOnly = ref.watch(settingsProvider).viewOnly;
    return Scaffold(
      body: Column(
        children: [
          Expanded(
            child: Padding(
              padding: const EdgeInsets.all(AppSpacing.sm),
              child: RemoteViewWidget(
                isConnected: true,
                remoteStream: service.remoteStream,
                viewOnly: viewOnly,
                hostOs: service.remoteHostOs,
                onInput: viewOnly
                    ? null
                    : (event) =>
                        ref.read(remoteServiceProvider).sendViewerInput(event),
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
                _StatChip(Icons.speed, '${stats.fps ?? 0} fps'),
                if (stats.latencyMs != null)
                  _StatChip(Icons.network_ping, '${stats.latencyMs} ms'),
                if (stats.bitrateKbps != null && stats.bitrateKbps! > 0)
                  _StatChip(Icons.bar_chart,
                      '${(stats.bitrateKbps! / 1000).toStringAsFixed(1)} Mbps'),
                if (stats.codec != null) _StatChip(Icons.movie, stats.codec!),
                const Spacer(),
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
