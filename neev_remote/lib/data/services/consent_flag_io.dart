import 'dart:io';

/// Writes the "Ask before allowing connections" flag where the root/SYSTEM
/// transport reads it ("1" = ask, "0" = auto-accept).
///
/// Windows: %ProgramData%\NeevRemote\consent.txt — the app and the service share
/// that directory, so one machine-wide file is enough.
///
/// macOS: the daemon's /Library/Application Support/NeevRemote is root-owned
/// 0755, so this app cannot write there. It writes into its OWN
/// ~/Library/Application Support/NeevRemote/consent.txt instead, and the root
/// transport reads the console user's copy (see consentflag_darwin.go). Without
/// this the flag was never written on macOS at all, so a Mac daemon host
/// auto-accepted every connection no matter what the setting said.
Future<void> writeConsentFlag(bool ask) async {
  try {
    final Directory dir;
    if (Platform.isWindows) {
      final pd = Platform.environment['ProgramData'];
      if (pd == null || pd.isEmpty) return;
      dir = Directory('$pd\\NeevRemote');
      // The service creates this dir; only write when it exists so we don't
      // fight ACLs by creating a user-owned dir the service can't read.
      if (!await dir.exists()) return;
    } else if (Platform.isMacOS) {
      final home = Platform.environment['HOME'];
      if (home == null || home.isEmpty) return;
      dir = Directory('$home/Library/Application Support/NeevRemote');
      // Unlike Windows, this one is ours to create — root can still read it.
      if (!await dir.exists()) await dir.create(recursive: true);
    } else {
      return;
    }
    final sep = Platform.isWindows ? '\\' : '/';
    await File('${dir.path}${sep}consent.txt')
        .writeAsString(ask ? '1' : '0', flush: true);
  } catch (_) {}
}
