/// Web stub: no filesystem, and no TransportMode service to gate.
Future<void> writeConsentFlag(bool ask) async {}

/// Web stub: the host-side view-only default only matters to the native
/// transport, which does not exist on web.
Future<void> writeViewOnlyFlag(bool viewOnly) async {}

/// Web stub: interactive-access policy is a native-host concern.
Future<void> writeInteractiveAccess(String mode) async {}

/// Web stub: access profiles are a native-host concern.
Future<void> writeAccessProfile(
    {required bool unattended,
    required bool control,
    required bool clipboard,
    required bool files}) async {}
