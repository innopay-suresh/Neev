/// Web stub: no filesystem, and no TransportMode service to gate.
Future<void> writeConsentFlag(bool ask) async {}

/// Web stub: the host-side view-only default only matters to the native
/// transport, which does not exist on web.
Future<void> writeViewOnlyFlag(bool viewOnly) async {}
