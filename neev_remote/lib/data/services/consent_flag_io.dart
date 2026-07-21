import 'dart:io';

/// Writes the "Ask before allowing connections" flag where the SYSTEM-service
/// transport (session 0) reads it: %ProgramData%\NeevRemote\consent.txt
/// ("1" = ask, "0" = auto-accept). Windows only — that's the only platform that
/// runs the TransportMode service whose transport gates on this file. On a
/// Flutter-hosted box the in-app consent dialog is used instead (no file needed).
Future<void> writeConsentFlag(bool ask) async {
  if (!Platform.isWindows) return;
  try {
    final pd = Platform.environment['ProgramData'];
    if (pd == null || pd.isEmpty) return;
    final dir = Directory('$pd\\NeevRemote');
    // The service creates this dir; only write when it exists so we don't fight
    // ACLs by creating a user-owned dir the service can't read.
    if (!await dir.exists()) return;
    await File('${dir.path}\\consent.txt')
        .writeAsString(ask ? '1' : '0', flush: true);
  } catch (_) {}
}
