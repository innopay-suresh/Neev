import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/theme/app_theme.dart';

/// The incoming-connection consent card (approved design, 2026-07-31).
///
/// Replaces a plain AlertDialog that showed only a sentence and two buttons.
/// This is the same layout the native Windows prompt draws
/// (agent/session/consentwin_windows.go), so a host user sees one design
/// whether they are on a macOS/attended host (this widget) or a Windows
/// SYSTEM-service host (the native window).
///
/// Returns a [ConsentChoice], or null only if the dialog is dismissed by a
/// route pop — callers must treat that as a refusal, never as an accept.
class ConsentChoice {
  const ConsentChoice({
    required this.accepted,
    required this.control,
    required this.remember,
  });
  final bool accepted;

  /// The ACCESS LEVEL the host granted. The host decides this — a viewer can
  /// never raise itself from view-only to control.
  final bool control;
  final bool remember;
}

class ConsentDialog extends StatefulWidget {
  const ConsentDialog({
    super.key,
    required this.deviceId,
    this.defaultControl = true,
  });

  /// The viewer's device id, already stripped of the internal "ctrl-" prefix.
  final String deviceId;

  /// Which access level the selector opens on — the host's own "View only
  /// mode" setting, so the prompt defaults to what this machine already asked
  /// for.
  final bool defaultControl;

  @override
  State<ConsentDialog> createState() => _ConsentDialogState();
}

class _ConsentDialogState extends State<ConsentDialog> {
  bool _remember = false;
  bool _copied = false;
  late bool _control = widget.defaultControl;

  /// Groups a 9-digit id as "926 941 775" — the form the user reads aloud.
  String get _prettyId {
    final digits = widget.deviceId.replaceAll(RegExp(r'[^0-9]'), '');
    if (digits.length != 9) return widget.deviceId;
    return '${digits.substring(0, 3)} ${digits.substring(3, 6)} '
        '${digits.substring(6, 9)}';
  }

  void _close(bool accepted) => Navigator.of(context).pop(ConsentChoice(
      accepted: accepted, control: _control, remember: _remember));

  @override
  Widget build(BuildContext context) {
    return Dialog(
      backgroundColor: AppColors.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadii.panel),
        side: BorderSide(color: AppColors.border),
      ),
      insetPadding: const EdgeInsets.all(AppSpacing.lg),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 440),
        // The card grew when the access-level selector was added and can be
        // taller than a short window. Scroll the BODY, but keep the header and
        // the Allow/Decline row pinned: a security prompt whose actions have
        // scrolled out of sight is worse than one that scrolls.
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _header(),
            Flexible(
              child: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Padding(
                      padding: const EdgeInsets.fromLTRB(
                          AppSpacing.xl, 0, AppSpacing.xl, AppSpacing.lg),
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          _medallion(),
                          const SizedBox(width: AppSpacing.lg),
                          Expanded(child: _body()),
                        ],
                      ),
                    ),
                    Divider(height: 1, color: AppColors.border),
                    _accessLevel(),
                    Divider(height: 1, color: AppColors.border),
                    _securityNote(),
                  ],
                ),
              ),
            ),
            Divider(height: 1, color: AppColors.border),
            _footer(),
          ],
        ),
      ),
    );
  }

  Widget _header() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(
          AppSpacing.xl, AppSpacing.lg, AppSpacing.md, AppSpacing.md),
      child: Row(
        children: [
          Container(
            width: 24,
            height: 24,
            decoration: BoxDecoration(
              color: AppColors.primary,
              borderRadius: BorderRadius.circular(AppRadii.sm),
            ),
            alignment: Alignment.center,
            child: Text('N',
                style: AppTypography.title.copyWith(
                    color: Colors.white,
                    fontSize: 14,
                    fontWeight: FontWeight.w700)),
          ),
          const SizedBox(width: AppSpacing.sm),
          Text('Neev Remote',
              style: AppTypography.title.copyWith(fontSize: 17)),
          const Spacer(),
          IconButton(
            // Closing the prompt is a refusal, never an accept.
            onPressed: () => _close(false),
            icon: Icon(Icons.close,
                size: 18, color: AppColors.textSecondary),
            tooltip: 'Decline',
            splashRadius: 18,
          ),
        ],
      ),
    );
  }

  Widget _medallion() {
    return Container(
      width: 84,
      height: 84,
      decoration: BoxDecoration(
        color: AppColors.primarySoft,
        shape: BoxShape.circle,
      ),
      alignment: Alignment.center,
      child: Icon(Icons.desktop_windows_outlined,
          size: 38, color: AppColors.primary),
    );
  }

  Widget _body() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Connection Request',
            style: AppTypography.title.copyWith(fontSize: 20)),
        const SizedBox(height: AppSpacing.xs),
        Text(
          'A remote device is requesting to connect and control this computer.',
          style: AppTypography.caption.copyWith(height: 1.45),
        ),
        const SizedBox(height: AppSpacing.lg),
        Text('Device ID',
            style: AppTypography.body
                .copyWith(fontSize: 12, fontWeight: FontWeight.w600)),
        const SizedBox(height: 2),
        Row(
          children: [
            // The device id is a designed object: mono + tabular figures, so
            // digits line up and it reads correctly over the phone.
            Text(_prettyId,
                style: AppTypography.mono.copyWith(
                  fontSize: 21,
                  fontWeight: FontWeight.w700,
                  color: AppColors.primary,
                )),
            const SizedBox(width: AppSpacing.sm),
            IconButton(
              onPressed: () async {
                await Clipboard.setData(ClipboardData(text: _prettyId));
                if (mounted) setState(() => _copied = true);
              },
              icon: Icon(_copied ? Icons.check : Icons.copy_outlined,
                  size: 15, color: AppColors.textSecondary),
              tooltip: _copied ? 'Copied' : 'Copy device ID',
              splashRadius: 15,
              constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
              padding: EdgeInsets.zero,
            ),
          ],
        ),
      ],
    );
  }

  /// The access level being granted. This is the whole point of the prompt:
  /// the HOST chooses whether this viewer may control the machine or only watch.
  Widget _accessLevel() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(
          AppSpacing.xl, AppSpacing.lg, AppSpacing.xl, AppSpacing.lg),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(_control ? 'Full Control Access' : 'View Only Access',
              style: AppTypography.body
                  .copyWith(fontSize: 14, fontWeight: FontWeight.w700)),
          const SizedBox(height: 3),
          Text(
            _control
                ? 'The remote user will be able to see your screen and control '
                    'your computer.'
                : 'The remote user will be able to see your screen but NOT '
                    'control it.',
            style: AppTypography.caption.copyWith(height: 1.4),
          ),
          const SizedBox(height: AppSpacing.md),
          Row(children: [
            Expanded(child: _segment('Full control', true)),
            const SizedBox(width: AppSpacing.sm),
            Expanded(child: _segment('View only', false)),
          ]),
        ],
      ),
    );
  }

  Widget _segment(String label, bool control) {
    final on = _control == control;
    return InkWell(
      onTap: () => setState(() => _control = control),
      borderRadius: BorderRadius.circular(AppRadii.md),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 9),
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: on ? AppColors.primarySoft : Colors.transparent,
          border: Border.all(
              color: on ? AppColors.primary : AppColors.borderStrong),
          borderRadius: BorderRadius.circular(AppRadii.md),
        ),
        child: Text(label,
            style: AppTypography.caption.copyWith(
                color: on ? AppColors.primary : AppColors.textSecondary,
                fontWeight: on ? FontWeight.w600 : FontWeight.w400)),
      ),
    );
  }

  Widget _securityNote() {
    return Padding(
      padding: const EdgeInsets.all(AppSpacing.xl),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.shield_outlined, size: 19, color: AppColors.primary),
          const SizedBox(width: AppSpacing.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Only allow if you recognise this request.',
                    style: AppTypography.body
                        .copyWith(fontSize: 13, fontWeight: FontWeight.w600)),
                const SizedBox(height: 3),
                Text(
                  "If you don't recognise this device, do not allow the "
                  'connection.',
                  style: AppTypography.caption.copyWith(height: 1.4),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _footer() {
    return Padding(
      padding: const EdgeInsets.all(AppSpacing.xl),
      child: Row(
        children: [
          // The label toggles too, not just the 18px box. Expanded (not Spacer)
          // so the label yields space to the buttons on a narrow window instead
          // of overflowing the row.
          Expanded(
            child: InkWell(
              onTap: () => setState(() => _remember = !_remember),
              borderRadius: BorderRadius.circular(AppRadii.sm),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 4, horizontal: 2),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    SizedBox(
                      width: 18,
                      height: 18,
                      child: Checkbox(
                        value: _remember,
                        onChanged: (v) =>
                            setState(() => _remember = v ?? false),
                        activeColor: AppColors.primary,
                        side: BorderSide(color: AppColors.borderStrong),
                        materialTapTargetSize:
                            MaterialTapTargetSize.shrinkWrap,
                      ),
                    ),
                    const SizedBox(width: AppSpacing.sm),
                    Flexible(
                      child: Text('Remember this decision',
                          style: AppTypography.caption,
                          overflow: TextOverflow.ellipsis),
                    ),
                  ],
                ),
              ),
            ),
          ),
          const SizedBox(width: AppSpacing.sm),
          OutlinedButton(
            onPressed: () => _close(false),
            style: OutlinedButton.styleFrom(
              foregroundColor: AppColors.textPrimary,
              side: BorderSide(color: AppColors.borderStrong),
              padding: const EdgeInsets.symmetric(
                  horizontal: AppSpacing.md, vertical: AppSpacing.md),
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(AppRadii.md)),
            ),
            child: const Text('Decline'),
          ),
          const SizedBox(width: AppSpacing.sm),
          FilledButton(
            onPressed: () => _close(true),
            style: FilledButton.styleFrom(
              backgroundColor: AppColors.primary,
              padding: const EdgeInsets.symmetric(
                  horizontal: AppSpacing.lg, vertical: AppSpacing.md),
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(AppRadii.md)),
            ),
            child: const Text('Accept'),
          ),
        ],
      ),
    );
  }
}
