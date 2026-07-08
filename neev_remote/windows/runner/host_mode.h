#ifndef RUNNER_HOST_MODE_H_
#define RUNNER_HOST_MODE_H_

#include <flutter/flutter_engine.h>

// Registers the "neev_remote/hostmode" MethodChannel. Its "query" method returns
// {serviceInstance: bool, serviceHostMode: bool} so Dart can decide whether to
// auto-start hosting: a manually-opened window must NOT host when the SYSTEM
// service is already hosting (ServiceHost mode), or the two hosts compete for
// the machine-id and a user switch strands the visible one in the old session.
// [is_service_instance] is true when this process was launched with
// --service-host (i.e. it IS the service-managed host).
void RegisterHostMode(flutter::FlutterEngine* engine, bool is_service_instance);

#endif  // RUNNER_HOST_MODE_H_
