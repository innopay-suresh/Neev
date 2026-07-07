// Platform-switched machine hostname: real name via dart:io on native,
// empty on web (callers fall back to a generic label).
export 'host_name_web.dart' if (dart.library.io) 'host_name_io.dart';
