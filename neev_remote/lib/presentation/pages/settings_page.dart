import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../core/constants/app_constants.dart';
import '../../core/theme/app_theme.dart';
import 'audit_log_page.dart';
import '../../data/services/consent_store.dart';
import '../../data/services/mac_daemon.dart';
import '../../data/services/remote_service.dart';
import '../providers/app_providers.dart';

class _SettingsSection {
  final IconData icon;
  final String label;
  const _SettingsSection(this.icon, this.label);
}

const _settingsSections = [
  _SettingsSection(Icons.tune_rounded, 'General'),
  _SettingsSection(Icons.shield_outlined, 'Security'),
  _SettingsSection(Icons.desktop_windows_outlined, 'Display'),
  _SettingsSection(Icons.dns_outlined, 'Connection'),
  _SettingsSection(Icons.info_outline_rounded, 'About'),
];

/// AnyDesk-style settings: a left section list + a content pane on the right.
class SettingsPage extends ConsumerStatefulWidget {
  const SettingsPage({super.key});

  @override
  ConsumerState<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends ConsumerState<SettingsPage> {
  int _section = 0;
  bool _daemonBusy = false;
  String? _daemonMsg;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Section list
        Container(
          width: 190,
          decoration: BoxDecoration(
            border: Border(right: BorderSide(color: AppColors.border)),
          ),
          child: ListView(
            padding: const EdgeInsets.symmetric(vertical: AppSpacing.md),
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(AppSpacing.lg, AppSpacing.sm,
                    AppSpacing.lg, AppSpacing.md),
                child: Text('Settings', style: AppTypography.heading2),
              ),
              for (var i = 0; i < _settingsSections.length; i++)
                _navRow(i, _settingsSections[i]),
            ],
          ),
        ),
        // Content pane
        Expanded(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(AppSpacing.xl),
            child: Center(
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 620),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: _sectionContent(),
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _navRow(int i, _SettingsSection s) {
    final selected = i == _section;
    return InkWell(
      onTap: () => setState(() => _section = i),
      child: Container(
        padding: const EdgeInsets.symmetric(
            horizontal: AppSpacing.lg, vertical: 10),
        color: selected ? AppColors.primarySoft : Colors.transparent,
        child: Row(children: [
          Icon(s.icon,
              size: 18,
              color: selected ? AppColors.primary : AppColors.textSecondary),
          const SizedBox(width: 10),
          Text(s.label,
              style: AppTypography.body.copyWith(
                  color: selected ? AppColors.primary : AppColors.textPrimary,
                  fontWeight:
                      selected ? FontWeight.w600 : FontWeight.w400)),
        ]),
      ),
    );
  }

  List<Widget> _sectionContent() {
    final settings = ref.watch(settingsProvider);
    switch (_section) {
      case 1:
        return _securitySection(settings);
      case 2:
        return _displaySection(settings);
      case 3:
        return _connectionSection(settings);
      case 4:
        return _aboutSection();
      default:
        return _generalSection(settings);
    }
  }

  List<Widget> _generalSection(AppSettings settings) {
    return [
      _buildSectionHeader('Application'),
      _buildSettingsCard([
        _buildToggle(
          label: 'Auto answer',
          subtitle: 'Automatically accept incoming connections',
          value: settings.autoAnswer,
          onChanged: (_) =>
              ref.read(settingsProvider.notifier).toggleAutoAnswer(),
        ),
        const Divider(),
        _buildToggle(
          label: 'Start on boot',
          subtitle: 'Launch Neev Remote when the system starts',
          value: settings.startOnBoot,
          onChanged: (_) =>
              ref.read(settingsProvider.notifier).toggleStartOnBoot(),
        ),
      ]),
    ];
  }

  List<Widget> _connectionSection(AppSettings settings) {
    return [
      _buildSectionHeader('Connection'),
      _buildSettingsCard([
        const _RelayUrlField(),
        const Divider(),
        _buildToggle(
          label: 'View only mode',
          subtitle: 'Watch without sending keyboard or mouse input',
          value: settings.viewOnly,
          onChanged: (_) =>
              ref.read(settingsProvider.notifier).toggleViewOnly(),
        ),
      ]),
    ];
  }

  List<Widget> _displaySection(AppSettings settings) {
    return [
      _buildSectionHeader('Video'),
      _buildSettingsCard([
        _buildSlider(
          label: 'Bitrate',
          value: settings.videoBitrate.toDouble(),
          min: 500,
          max: 5000,
          divisions: 9,
          suffix: 'kbps',
          onChanged: (v) => ref
              .read(settingsProvider.notifier)
              .updateVideoBitrate(v.toInt()),
        ),
        const Divider(),
        _buildSlider(
          label: 'Frame rate',
          value: settings.videoFps.toDouble(),
          min: 15,
          max: 60,
          divisions: 3,
          suffix: 'fps',
          onChanged: (v) =>
              ref.read(settingsProvider.notifier).updateVideoFps(v.toInt()),
        ),
      ]),
    ];
  }

  List<Widget> _aboutSection() {
    return [
      _buildSectionHeader('About'),
      _buildSettingsCard([
        _buildInfoRow('Version', AppConstants.appVersion),
        const Divider(),
        _buildInfoRow('Build', AppConstants.buildTag),
        const Divider(),
        _buildInfoRow('Platform', 'Desktop'),
        const Divider(),
        _buildInfoRow('Engine', 'WebRTC (native)'),
      ]),
    ];
  }

  /// Interactive Access: what happens when someone connects with the SESSION
  /// password. The unattended password is a separate door and is unaffected.
  Widget _buildInteractiveAccess(AppSettings settings) {
    const modes = [
      ('always', 'Always allow requests', 'Anyone with the session password may ask'),
      ('when-open', 'Only while the app is open', 'Requests are ignored when the app is closed'),
      ('never', 'Disable interactive access',
          'Refuse session-password logins entirely — only the unattended password works'),
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(bottom: 6),
          child: Text('Interactive access',
              style: AppTypography.body.copyWith(fontWeight: FontWeight.w600)),
        ),
        Text(
          'Applies to the session password only. The unattended password always '
          'connects without a prompt — that is what it is for.',
          style: AppTypography.caption,
        ),
        const SizedBox(height: 8),
        for (final m in modes)
          RadioListTile<String>(
            contentPadding: EdgeInsets.zero,
            dense: true,
            value: m.$1,
            groupValue: settings.interactiveAccess,
            onChanged: (v) => ref
                .read(settingsProvider.notifier)
                .setInteractiveAccess(v ?? 'always'),
            title: Text(m.$2, style: AppTypography.body),
            subtitle: Text(m.$3, style: AppTypography.caption),
          ),
      ],
    );
  }

  /// Permissions for UNATTENDED sessions, kept separate from interactive ones:
  /// nobody is present to judge an unattended login, so it can be given less.
  Widget _buildUnattendedProfile(AppSettings settings) {
    final n = ref.read(settingsProvider.notifier);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(bottom: 6),
          child: Text('Unattended session permissions',
              style: AppTypography.body.copyWith(fontWeight: FontWeight.w600)),
        ),
        Text(
          'Granted when someone connects with the unattended password. '
          '"View only mode" still overrides control.',
          style: AppTypography.caption,
        ),
        const SizedBox(height: 4),
        _buildToggle(
          label: 'Allow control',
          subtitle: 'Keyboard and mouse, lock, restart',
          value: settings.unattendedAllowControl,
          onChanged: (v) => n.setUnattendedPerms(control: v),
        ),
        _buildToggle(
          label: 'Allow clipboard',
          subtitle: 'Share clipboard text and files',
          value: settings.unattendedAllowClipboard,
          onChanged: (v) => n.setUnattendedPerms(clipboard: v),
        ),
        _buildToggle(
          label: 'Allow file transfer',
          subtitle: 'Send and receive files',
          value: settings.unattendedAllowFiles,
          onChanged: (v) => n.setUnattendedPerms(files: v),
        ),
      ],
    );
  }

  List<Widget> _securitySection(AppSettings settings) {
    return [
              // Security — incoming access + default permissions (AnyDesk parity)
              _buildSectionHeader('Security'),
              _buildSettingsCard([
                _buildToggle(
                  label: 'Ask before allowing connections',
                  subtitle:
                      'Show an Accept / Decline prompt for interactive sessions',
                  value: settings.askOnConnect,
                  onChanged: (v) =>
                      ref.read(settingsProvider.notifier).setAskOnConnect(v),
                ),
                const Divider(),
                // Interactive Access, AnyDesk-style. The unattended password is
                // a separate door and is never governed by this — that is the
                // whole point of unattended access.
                _buildInteractiveAccess(settings),
                const Divider(),
                _buildUnattendedProfile(settings),
                const Divider(),
                // Undo for "Remember this decision" on the consent prompt. A
                // remembered DECLINE is otherwise a dead end: the device is
                // refused silently, with no prompt left to change the answer.
                const _ForgetRememberedDevices(),
                const Divider(),
                _buildToggle(
                  label: 'Sound on incoming connection',
                  subtitle: 'Play a sound when someone connects',
                  value: settings.soundOnConnect,
                  onChanged: (v) =>
                      ref.read(settingsProvider.notifier).setSoundOnConnect(v),
                ),
                const Divider(),
                // Roadmap Phase 2 — audit trail entry point.
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  leading: const Icon(Icons.receipt_long_rounded, size: 20),
                  title: Text('Audit log', style: AppTypography.body),
                  subtitle: Text(
                      'Every session: who connected, when, for how long',
                      style: AppTypography.caption),
                  trailing: const Icon(Icons.chevron_right_rounded, size: 20),
                  onTap: () => Navigator.of(context).push(MaterialPageRoute(
                      builder: (_) => const AuditLogPage())),
                ),
                const Divider(),
                // Roadmap Phase 3 — custom alias.
                const _AliasField(),
                const Divider(),
                _buildToggle(
                  label: 'Lock this device on session end',
                  subtitle: 'Lock the screen when the last viewer disconnects',
                  value: settings.lockOnSessionEnd,
                  onChanged: (v) => ref
                      .read(settingsProvider.notifier)
                      .setLockOnSessionEnd(v),
                ),
                const Divider(),
                _buildToggle(
                  label: 'Clipboard sync',
                  subtitle:
                      'Share copied text, images & files with the other side. '
                      'Copying never pastes on its own — press Ctrl+V to paste.',
                  value: settings.clipboardSync,
                  onChanged: (v) =>
                      ref.read(settingsProvider.notifier).setClipboardSync(v),
                ),
              ]),
              ..._macDaemonCard(),
    ];
  }

  /// macOS-only: install/remove the lock-screen + switch-user daemon. Only shown
  /// when the build actually bundled the daemon payload.
  List<Widget> _macDaemonCard() {
    if (!MacDaemon.canInstall) return const [];
    final installed = MacDaemon.isInstalled;
    return [
      const SizedBox(height: AppSpacing.lg),
      _buildSectionHeader('Unattended access (macOS)'),
      _buildSettingsCard([
        Padding(
          padding: const EdgeInsets.all(AppSpacing.sm),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                installed
                    ? 'Lock-screen daemon: installed'
                    : 'Lock-screen daemon: not installed',
                style: AppTypography.body.copyWith(
                    color: installed ? AppColors.success : AppColors.textPrimary),
              ),
              const SizedBox(height: AppSpacing.xs),
              Text(
                'Runs a background service so viewers keep seeing this Mac across '
                'lock, logout and fast-user-switch — including the login window. '
                'After installing, grant it Screen Recording and Accessibility in '
                'System Settings → Privacy & Security.',
                style: AppTypography.caption
                    .copyWith(color: AppColors.textSecondary),
              ),
              if (_daemonMsg != null) ...[
                const SizedBox(height: AppSpacing.xs),
                Text(_daemonMsg!,
                    style: AppTypography.caption
                        .copyWith(color: AppColors.warning)),
              ],
              const SizedBox(height: AppSpacing.sm),
              Row(
                children: [
                  ElevatedButton.icon(
                    onPressed: _daemonBusy
                        ? null
                        : () => _runDaemon(installed ? 'uninstall' : 'install'),
                    icon: _daemonBusy
                        ? const SizedBox(
                            width: 14,
                            height: 14,
                            child: CircularProgressIndicator(strokeWidth: 2))
                        : Icon(installed
                            ? Icons.delete_outline
                            : Icons.download_rounded),
                    label: Text(installed
                        ? 'Remove daemon'
                        : 'Install lock-screen daemon'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ]),
    ];
  }

  Future<void> _runDaemon(String action) async {
    setState(() {
      _daemonBusy = true;
      _daemonMsg = null;
    });
    final err = action == 'install'
        ? await MacDaemon.install()
        : await MacDaemon.uninstall();
    if (!mounted) return;
    setState(() {
      _daemonBusy = false;
      if (err == null) {
        _daemonMsg = action == 'install'
            ? 'Installed. Now grant Screen Recording + Accessibility in System '
                'Settings, then log out and back in once.'
            : 'Removed.';
      } else if (err == 'cancelled') {
        _daemonMsg = 'Cancelled.';
      } else {
        _daemonMsg = 'Failed: $err';
      }
    });
  }

  Widget _buildSectionHeader(String title) {
    return Padding(
      padding: const EdgeInsets.only(left: AppSpacing.sm, bottom: AppSpacing.sm),
      child: Text(
        title,
        style: AppTypography.heading2.copyWith(color: AppColors.primary),
      ),
    );
  }

  Widget _buildSettingsCard(List<Widget> children) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadius.md),
        border: Border.all(color: AppColors.border),
      ),
      child: Column(children: children),
    );
  }

  Widget _buildToggle({
    required String label,
    required String subtitle,
    required bool value,
    required ValueChanged<bool> onChanged,
  }) {
    return Padding(
      padding: const EdgeInsets.all(AppSpacing.md),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(label, style: AppTypography.body),
                const SizedBox(height: AppSpacing.xs),
                Text(subtitle, style: AppTypography.caption),
              ],
            ),
          ),
          Switch(
            value: value,
            onChanged: onChanged,
            activeColor: AppColors.primary,
          ),
        ],
      ),
    );
  }

  Widget _buildSlider({
    required String label,
    required double value,
    required double min,
    required double max,
    required int divisions,
    required String suffix,
    required ValueChanged<double> onChanged,
  }) {
    return Padding(
      padding: const EdgeInsets.all(AppSpacing.md),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(label, style: AppTypography.body),
              Text(
                '${value.toInt()} $suffix',
                style: AppTypography.caption.copyWith(color: AppColors.primary),
              ),
            ],
          ),
          Slider(
            value: value,
            min: min,
            max: max,
            divisions: divisions,
            onChanged: onChanged,
            // ignore: deprecated_member_use
            activeColor: AppColors.primary,
          ),
        ],
      ),
    );
  }

  Widget _buildInfoRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.all(AppSpacing.md),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: AppTypography.body),
          Text(value, style: AppTypography.body.copyWith(color: AppColors.textSecondary)),
        ],
      ),
    );
  }
}

/// Relay server URL field with an explicit Save button. Persists via
/// SharedPreferences and reconnects the host with the new address.
class _RelayUrlField extends ConsumerStatefulWidget {
  const _RelayUrlField();

  @override
  ConsumerState<_RelayUrlField> createState() => _RelayUrlFieldState();
}

class _RelayUrlFieldState extends ConsumerState<_RelayUrlField> {
  late final TextEditingController _controller;

  @override
  void initState() {
    super.initState();
    _controller =
        TextEditingController(text: ref.read(settingsProvider).relayUrl);
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final url = normalizeRelayUrl(_controller.text);
    if (url.isEmpty) return;
    _controller.text = url;
    ref.read(settingsProvider.notifier).updateRelayUrl(url);
    // Reconnect the host with the new server address so it takes effect now.
    final service = ref.read(remoteServiceProvider);
    if (service.isHosting || service.hostStatus == HostStatus.error) {
      try {
        await service.startHosting(relayUrl: url);
      } catch (_) {}
    }
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Relay URL saved'),
          duration: Duration(seconds: 2),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(AppSpacing.md),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Relay Server URL', style: AppTypography.body),
          const SizedBox(height: AppSpacing.xs),
          Text(
            'Address of your signaling server, e.g. ws://192.168.1.10:8080/ws',
            style: AppTypography.caption,
          ),
          const SizedBox(height: AppSpacing.sm),
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _controller,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: 'ws://server-ip:8080/ws',
                  ),
                  onSubmitted: (_) => _save(),
                ),
              ),
              const SizedBox(width: AppSpacing.sm),
              ElevatedButton.icon(
                onPressed: _save,
                icon: const Icon(Icons.save, size: 18),
                label: const Text('Save'),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

/// Phase 3: set a human-readable alias for this machine. Others can then dial
/// the alias instead of the numeric ID.
class _AliasField extends ConsumerStatefulWidget {
  const _AliasField();
  @override
  ConsumerState<_AliasField> createState() => _AliasFieldState();
}

class _AliasFieldState extends ConsumerState<_AliasField> {
  late final TextEditingController _c;

  @override
  void initState() {
    super.initState();
    _c = TextEditingController(text: ref.read(remoteServiceProvider).deviceAlias);
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final service = ref.watch(remoteServiceProvider);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('Custom alias', style: AppTypography.body),
        Text('Let people reach this machine by a name instead of its ID.',
            style: AppTypography.caption),
        const SizedBox(height: 8),
        Row(children: [
          Expanded(
            child: TextField(
              controller: _c,
              decoration: InputDecoration(
                hintText: 'e.g. reception-pc',
                isDense: true,
                errorText: service.aliasError,
                prefixIcon: const Icon(Icons.alternate_email_rounded, size: 18),
              ),
            ),
          ),
          const SizedBox(width: 10),
          FilledButton(
            onPressed: () => ref
                .read(remoteServiceProvider)
                .setDeviceAlias(_c.text.trim().toLowerCase()),
            child: const Text('Save'),
          ),
        ]),
        if (service.deviceAlias.isNotEmpty && service.aliasError == null)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text('Reachable as "${service.deviceAlias}"',
                style: AppTypography.caption.copyWith(color: AppColors.success)),
          ),
      ]),
    );
  }
}

/// "Forget remembered devices" — clears every Accept/Decline decision saved by
/// the "Remember this decision" checkbox on the consent prompt.
///
/// This exists because a remembered DECLINE is otherwise unrecoverable from the
/// UI: the device is refused before any prompt is shown, so there is nothing
/// left to click to change the answer.
class _ForgetRememberedDevices extends StatefulWidget {
  const _ForgetRememberedDevices();

  @override
  State<_ForgetRememberedDevices> createState() =>
      _ForgetRememberedDevicesState();
}

class _ForgetRememberedDevicesState extends State<_ForgetRememberedDevices> {
  int _count = 0;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    final n = await ConsentStore.count();
    if (mounted) setState(() => _count = n);
  }

  Future<void> _forget() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('Forget remembered devices?', style: AppTypography.title),
        content: Text(
          'Every device with a remembered Accept or Decline will prompt again '
          'on the next connection. This cannot be undone.',
          style: AppTypography.caption,
        ),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Forget')),
        ],
      ),
    );
    if (ok != true) return;
    await ConsentStore.forgetAll();
    await _refresh();
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Remembered devices cleared')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    // Data Honesty: show the real count this app knows about, and say plainly
    // that it counts this app's own store (the host worker keeps its own).
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: const Icon(Icons.history_toggle_off_rounded, size: 20),
      title: Text('Forget remembered devices', style: AppTypography.body),
      subtitle: Text(
        _count == 0
            ? 'No remembered decisions in this app'
            : '$_count device${_count == 1 ? '' : 's'} will connect or be '
                'refused without prompting',
        style: AppTypography.caption,
      ),
      trailing: TextButton(
        onPressed: _forget,
        child: const Text('Forget'),
      ),
    );
  }
}
