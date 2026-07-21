// Conditional: real file write on desktop (dart:io), no-op on web.
export 'consent_flag_web.dart' if (dart.library.io) 'consent_flag_io.dart';
