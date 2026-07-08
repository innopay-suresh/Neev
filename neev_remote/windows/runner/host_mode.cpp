#include "host_mode.h"

#include <windows.h>

#include <flutter/encodable_value.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <memory>

namespace {

// HKLM\SOFTWARE\NeevRemote\ServiceHost=1 is written by the installer when the
// user opts into "keep reachable for every user" — the service then owns the
// host. Read live so the toggle takes effect without reinstalling.
bool ServiceHostModeEnabled() {
  DWORD val = 0, sz = sizeof(val);
  if (RegGetValueW(HKEY_LOCAL_MACHINE, L"SOFTWARE\\NeevRemote", L"ServiceHost",
                   RRF_RT_REG_DWORD, nullptr, &val, &sz) == ERROR_SUCCESS) {
    return val != 0;
  }
  return false;
}

}  // namespace

void RegisterHostMode(flutter::FlutterEngine* engine, bool is_service_instance) {
  auto channel =
      std::make_shared<flutter::MethodChannel<flutter::EncodableValue>>(
          engine->messenger(), "neev_remote/hostmode",
          &flutter::StandardMethodCodec::GetInstance());

  channel->SetMethodCallHandler(
      [is_service_instance](
          const flutter::MethodCall<flutter::EncodableValue>& call,
          std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>
              result) {
        if (call.method_name() != "query") {
          result->NotImplemented();
          return;
        }
        flutter::EncodableMap m;
        m[flutter::EncodableValue("serviceInstance")] =
            flutter::EncodableValue(is_service_instance);
        m[flutter::EncodableValue("serviceHostMode")] =
            flutter::EncodableValue(ServiceHostModeEnabled());
        result->Success(flutter::EncodableValue(m));
      });

  static std::shared_ptr<flutter::MethodChannel<flutter::EncodableValue>>
      g_channel;
  g_channel = channel;
}
