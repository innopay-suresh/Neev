// Conditional export: desktop gets certificate-pinned wss, web uses the plain
// browser WebSocket (the browser owns TLS validation there).
export 'ws_connect_web.dart' if (dart.library.io) 'ws_connect_io.dart';
