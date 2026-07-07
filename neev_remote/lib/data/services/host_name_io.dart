import 'dart:io';

/// The machine's hostname (e.g. "DESKTOP-AB12CD"), or '' if unavailable.
String localHostname() {
  try {
    return Platform.localHostname;
  } catch (_) {
    return '';
  }
}
