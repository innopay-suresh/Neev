// Conditional export, matching file_store.dart: real checks on desktop
// (dart:io), no-ops on web.
export 'capture_status_web.dart'
    if (dart.library.io) 'capture_status_io.dart';
