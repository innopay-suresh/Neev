#include "clipboard_writer.h"

#include <windows.h>
#include <objidl.h>
#include <shlobj.h>

#include <flutter/encodable_value.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <memory>
#include <string>
#include <vector>

namespace {

std::wstring Widen(const std::string& utf8) {
  if (utf8.empty()) return std::wstring();
  int n = MultiByteToWideChar(CP_UTF8, 0, utf8.c_str(),
                              static_cast<int>(utf8.size()), nullptr, 0);
  std::wstring w(n, L'\0');
  MultiByteToWideChar(CP_UTF8, 0, utf8.c_str(), static_cast<int>(utf8.size()),
                      &w[0], n);
  return w;
}

// Put the given paths on the clipboard as CF_HDROP + "Preferred DropEffect" =
// COPY. Returns true on success.
bool WriteFilesCopy(const std::vector<std::wstring>& paths) {
  if (paths.empty()) return false;

  // CF_HDROP payload: DROPFILES header, then each path, double-null terminated.
  size_t chars = 0;
  for (const auto& p : paths) chars += p.size() + 1;
  chars += 1;  // final extra null
  size_t bytes = sizeof(DROPFILES) + chars * sizeof(wchar_t);
  HGLOBAL hDrop = GlobalAlloc(GHND, bytes);
  if (!hDrop) return false;
  auto* df = static_cast<DROPFILES*>(GlobalLock(hDrop));
  df->pFiles = sizeof(DROPFILES);
  df->fWide = TRUE;
  auto* w = reinterpret_cast<wchar_t*>(reinterpret_cast<BYTE*>(df) +
                                       sizeof(DROPFILES));
  for (const auto& p : paths) {
    wcscpy_s(w, p.size() + 1, p.c_str());
    w += p.size() + 1;
  }
  *w = L'\0';
  GlobalUnlock(hDrop);

  HGLOBAL hEffect = GlobalAlloc(GHND, sizeof(DWORD));
  if (hEffect) {
    auto* e = static_cast<DWORD*>(GlobalLock(hEffect));
    *e = DROPEFFECT_COPY;
    GlobalUnlock(hEffect);
  }

  if (!OpenClipboard(nullptr)) {
    GlobalFree(hDrop);
    if (hEffect) GlobalFree(hEffect);
    return false;
  }
  EmptyClipboard();
  bool ok = SetClipboardData(CF_HDROP, hDrop) != nullptr;
  if (hEffect) {
    UINT cf = RegisterClipboardFormatW(L"Preferred DropEffect");
    if (cf) SetClipboardData(cf, hEffect);
  }
  CloseClipboard();
  // On success the clipboard owns hDrop/hEffect; don't free them.
  return ok;
}

}  // namespace

void RegisterClipboardWriter(flutter::FlutterEngine* engine) {
  auto channel =
      std::make_shared<flutter::MethodChannel<flutter::EncodableValue>>(
          engine->messenger(), "neev_remote/clipboard",
          &flutter::StandardMethodCodec::GetInstance());

  channel->SetMethodCallHandler(
      [](const flutter::MethodCall<flutter::EncodableValue>& call,
         std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>
             result) {
        if (call.method_name() != "writeFilesCopy") {
          result->NotImplemented();
          return;
        }
        const auto* args = std::get_if<flutter::EncodableList>(call.arguments());
        if (!args) {
          result->Success(flutter::EncodableValue(false));
          return;
        }
        std::vector<std::wstring> paths;
        for (const auto& v : *args) {
          if (const auto* s = std::get_if<std::string>(&v)) {
            if (!s->empty()) paths.push_back(Widen(*s));
          }
        }
        result->Success(flutter::EncodableValue(WriteFilesCopy(paths)));
      });

  static std::shared_ptr<flutter::MethodChannel<flutter::EncodableValue>>
      g_channel;
  g_channel = channel;
}
