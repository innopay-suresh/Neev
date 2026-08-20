// Conditional export: a real native prompt on desktop (dart:io), a no-op on web.
//
// Kept behind the same conditional-import pattern as file_store.dart so this
// file can be used from remote_service.dart, which must still compile for web.
export 'native_consent_web.dart' if (dart.library.io) 'native_consent_io.dart';
