import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/theme/app_theme.dart';
import '../../data/services/audit_log.dart';

/// Roadmap Phase 2 — the local session audit trail, viewable and exportable.
/// Shows the hash-chain integrity result up front: a compliance reader's first
/// question is "has this been edited", so it is answered before the rows.
class AuditLogPage extends StatefulWidget {
  const AuditLogPage({super.key});

  @override
  State<AuditLogPage> createState() => _AuditLogPageState();
}

class _AuditLogPageState extends State<AuditLogPage> {
  List<Map<String, dynamic>> _rows = const [];
  String? _integrity;
  String _path = '';
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final rows = await AuditLog.instance.read();
    final bad = await AuditLog.instance.verify();
    final path = await AuditLog.instance.filePath();
    if (!mounted) return;
    setState(() {
      _rows = rows;
      _integrity = bad;
      _path = path;
      _loading = false;
    });
  }

  String _fmt(String? iso) {
    final t = DateTime.tryParse(iso ?? '')?.toLocal();
    if (t == null) return '—';
    String two(int v) => v.toString().padLeft(2, '0');
    return '${t.year}-${two(t.month)}-${two(t.day)} ${two(t.hour)}:${two(t.minute)}';
  }

  String _dur(dynamic s) {
    final v = (s is int) ? s : int.tryParse('$s') ?? 0;
    if (v < 60) return '${v}s';
    if (v < 3600) return '${v ~/ 60}m ${v % 60}s';
    return '${v ~/ 3600}h ${(v % 3600) ~/ 60}m';
  }

  @override
  Widget build(BuildContext context) {
    final intact = _integrity == null;
    return Scaffold(
      backgroundColor: AppColors.background,
      appBar: AppBar(
        backgroundColor: AppColors.surface,
        surfaceTintColor: Colors.transparent,
        title: Text('Audit log', style: AppTypography.pageTitle),
        actions: [
          IconButton(
            tooltip: 'Copy file path',
            icon: const Icon(Icons.folder_open_rounded, size: 19),
            onPressed: () {
              Clipboard.setData(ClipboardData(text: _path));
              ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
                  content: Text('Audit file path copied'),
                  duration: Duration(seconds: 2)));
            },
          ),
          IconButton(
            tooltip: 'Refresh',
            icon: const Icon(Icons.refresh_rounded, size: 19),
            onPressed: _load,
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(AppSpacing.xl),
              children: [
                // Integrity banner
                Container(
                  padding: const EdgeInsets.all(14),
                  decoration: BoxDecoration(
                    color: intact
                        ? AppColors.successSoft
                        : AppColors.error.withValues(alpha: 0.10),
                    borderRadius: BorderRadius.circular(AppRadii.lg),
                    border: Border.all(
                        color: intact
                            ? AppColors.success.withValues(alpha: 0.35)
                            : AppColors.error.withValues(alpha: 0.4)),
                  ),
                  child: Row(children: [
                    Icon(
                        intact
                            ? Icons.verified_user_rounded
                            : Icons.gpp_maybe_rounded,
                        size: 18,
                        color: intact ? AppColors.success : AppColors.error),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        intact
                            ? 'Hash chain intact — ${_rows.length} session(s) recorded, none altered.'
                            : _integrity!,
                        style: AppTypography.caption.copyWith(
                            color: intact
                                ? AppColors.success
                                : AppColors.error),
                      ),
                    ),
                  ]),
                ),
                const SizedBox(height: AppSpacing.md),
                Text(_path, style: AppTypography.meta),
                const SizedBox(height: AppSpacing.xl),
                if (_rows.isEmpty)
                  Container(
                    padding: const EdgeInsets.symmetric(vertical: 30),
                    alignment: Alignment.center,
                    child: Column(children: [
                      const Icon(Icons.receipt_long_rounded,
                          size: 26, color: AppColors.textTertiary),
                      const SizedBox(height: 10),
                      Text('No sessions recorded yet',
                          style: AppTypography.cardTitle),
                      const SizedBox(height: 4),
                      Text('Every connection will be logged here.',
                          style: AppTypography.meta),
                    ]),
                  )
                else
                  for (final r in _rows) _row(r),
              ],
            ),
    );
  }

  Widget _row(Map<String, dynamic> r) {
    final role = '${r['role']}';
    final host = role == 'host';
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.surface,
        borderRadius: BorderRadius.circular(AppRadii.lg),
        border: Border.all(color: AppColors.border),
        boxShadow: AppShadows.soft,
      ),
      child: Row(children: [
        Container(
          width: 32,
          height: 32,
          decoration: BoxDecoration(
            color: host ? AppColors.primarySoft : AppColors.secondarySoft,
            borderRadius: BorderRadius.circular(AppRadii.md),
          ),
          child: Icon(host ? Icons.call_received_rounded : Icons.call_made_rounded,
              size: 15,
              color: host ? AppColors.primaryDark : AppColors.secondary),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(
                host
                    ? 'Incoming from ${r['peer_id']}'
                    : 'Outgoing to ${r['peer_id']}',
                style: AppTypography.caption
                    .copyWith(fontWeight: FontWeight.w600)),
            const SizedBox(height: 2),
            Text(
                '${_fmt('${r['ts_start']}')} · ${_dur(r['duration_s'])} · '
                '${r['consent']} · ${r['end_reason']}',
                style: AppTypography.meta),
          ]),
        ),
      ]),
    );
  }
}
