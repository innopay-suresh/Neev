import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../core/theme/app_theme.dart';
import '../providers/app_providers.dart';

class SettingsPage extends ConsumerWidget {
  const SettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(settingsProvider);

    return SingleChildScrollView(
      padding: const EdgeInsets.all(AppSpacing.xl),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 600),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Settings', style: AppTypography.heading1),
              const SizedBox(height: AppSpacing.xl),

              // Connection Settings
              _buildSectionHeader('Connection'),
              _buildSettingsCard([
                _buildTextField(
                  context,
                  label: 'Relay Server URL',
                  value: settings.relayUrl,
                  onChanged: (v) => ref.read(settingsProvider.notifier).updateRelayUrl(v),
                ),
                const Divider(),
                _buildToggle(
                  label: 'View Only Mode',
                  subtitle: 'Disable keyboard and mouse input',
                  value: settings.viewOnly,
                  onChanged: (_) => ref.read(settingsProvider.notifier).toggleViewOnly(),
                ),
              ]),

              const SizedBox(height: AppSpacing.xl),

              // Video Settings
              _buildSectionHeader('Video'),
              _buildSettingsCard([
                _buildSlider(
                  label: 'Bitrate',
                  value: settings.videoBitrate.toDouble(),
                  min: 500,
                  max: 5000,
                  divisions: 9,
                  suffix: 'kbps',
                  onChanged: (v) => ref.read(settingsProvider.notifier).updateVideoBitrate(v.toInt()),
                ),
                const Divider(),
                _buildSlider(
                  label: 'Frame Rate',
                  value: settings.videoFps.toDouble(),
                  min: 15,
                  max: 60,
                  divisions: 3,
                  suffix: 'fps',
                  onChanged: (v) => ref.read(settingsProvider.notifier).updateVideoFps(v.toInt()),
                ),
              ]),

              const SizedBox(height: AppSpacing.xl),

              // Agent Settings
              _buildSectionHeader('Agent'),
              _buildSettingsCard([
                _buildToggle(
                  label: 'Auto Answer',
                  subtitle: 'Automatically accept incoming connections',
                  value: settings.autoAnswer,
                  onChanged: (_) => ref.read(settingsProvider.notifier).toggleAutoAnswer(),
                ),
                const Divider(),
                _buildToggle(
                  label: 'Start on Boot',
                  subtitle: 'Launch agent when system starts',
                  value: settings.startOnBoot,
                  onChanged: (_) => ref.read(settingsProvider.notifier).toggleStartOnBoot(),
                ),
              ]),

              const SizedBox(height: AppSpacing.xl),

              // About
              _buildSectionHeader('About'),
              _buildSettingsCard([
                _buildInfoRow('Version', '1.0.0'),
                const Divider(),
                _buildInfoRow('Flutter', 'Desktop'),
                const Divider(),
                _buildInfoRow('WebRTC', 'Native'),
              ]),
            ],
          ),
        ),
      ),
    );
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

  Widget _buildTextField(
    BuildContext context, {
    required String label,
    required String value,
    required ValueChanged<String> onChanged,
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
              ],
            ),
          ),
          SizedBox(
            width: 300,
            child: TextField(
              controller: TextEditingController(text: value),
              decoration: const InputDecoration(
                isDense: true,
                contentPadding: EdgeInsets.symmetric(
                  horizontal: AppSpacing.md,
                  vertical: AppSpacing.sm,
                ),
              ),
              onSubmitted: onChanged,
            ),
          ),
        ],
      ),
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