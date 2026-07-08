#include "clipboard_writer.h"
#include "remote_file_drop.h"

#include <windows.h>
#include <objidl.h>
#include <shlobj.h>

#include <flutter/encodable_value.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <chrono>
#include <condition_variable>
#include <cstdint>
#include <cstdlib>
#include <deque>
#include <map>
#include <memory>
#include <mutex>
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

// ---------------------------------------------------------------------------
// Delayed file-content fetch bridge.
//
// When the shell pastes a virtual file, RemoteFileDrop::MakeContents (on the
// shell's thread) calls FetchRemoteFileBytes below. That blocks on a per-request
// event while the Dart side — which owns the peer connection — pulls the bytes:
// Dart polls pendingFetches via "pollFileRequests", downloads each file over the
// data channel, and returns the bytes via "deliverRemoteFileBytes", which wakes
// the blocked shell thread. A timeout means paste fails cleanly instead of
// hanging Explorer forever.
// ---------------------------------------------------------------------------
struct FetchRequest {
  std::vector<BYTE> bytes;
  bool delivered = false;
  bool ok = false;
  bool dispatched = false;  // Dart has picked it up
};

std::mutex g_fetchMutex;
std::condition_variable g_fetchCv;
std::map<std::wstring, FetchRequest> g_fetches;  // key = token + L"\x1f" + index

std::wstring FetchKey(const std::wstring& token, uint32_t index) {
  return token + L"\x1f" + std::to_wstring(index);
}

}  // namespace

// Called on the shell's paste thread (see remote_file_drop.cpp).
bool FetchRemoteFileBytes(const std::wstring& token, uint32_t index,
                          std::vector<BYTE>& out) {
  const std::wstring key = FetchKey(token, index);
  {
    std::lock_guard<std::mutex> lock(g_fetchMutex);
    g_fetches[key] = FetchRequest{};  // fresh request, awaiting dispatch+delivery
  }
  g_fetchCv.notify_all();
  std::unique_lock<std::mutex> lock(g_fetchMutex);
  // Wait up to 60s for Dart to deliver the bytes.
  bool got = g_fetchCv.wait_for(lock, std::chrono::seconds(60), [&] {
    auto it = g_fetches.find(key);
    return it != g_fetches.end() && it->second.delivered;
  });
  auto it = g_fetches.find(key);
  bool ok = got && it != g_fetches.end() && it->second.ok;
  if (ok) out = std::move(it->second.bytes);
  if (it != g_fetches.end()) g_fetches.erase(it);
  return ok;
}

namespace {

// Returns [{token, index}] for requests Dart hasn't picked up yet, marking them
// dispatched so they're handed out only once.
flutter::EncodableValue PollFileRequests() {
  flutter::EncodableList out;
  std::lock_guard<std::mutex> lock(g_fetchMutex);
  for (auto& kv : g_fetches) {
    if (kv.second.dispatched || kv.second.delivered) continue;
    kv.second.dispatched = true;
    const std::wstring& key = kv.first;
    size_t sep = key.find(L'\x1f');
    std::wstring token = key.substr(0, sep);
    int index = _wtoi(key.substr(sep + 1).c_str());
    // token is UTF-16 → return as UTF-8 string for Dart.
    int n = WideCharToMultiByte(CP_UTF8, 0, token.c_str(), (int)token.size(),
                                nullptr, 0, nullptr, nullptr);
    std::string t(n, '\0');
    WideCharToMultiByte(CP_UTF8, 0, token.c_str(), (int)token.size(), &t[0], n,
                        nullptr, nullptr);
    out.push_back(flutter::EncodableValue(flutter::EncodableMap{
        {flutter::EncodableValue("token"), flutter::EncodableValue(t)},
        {flutter::EncodableValue("index"), flutter::EncodableValue(index)},
    }));
  }
  return flutter::EncodableValue(std::move(out));
}

void DeliverFileBytes(const std::wstring& token, uint32_t index, bool ok,
                      std::vector<BYTE> bytes) {
  {
    std::lock_guard<std::mutex> lock(g_fetchMutex);
    auto& r = g_fetches[FetchKey(token, index)];
    r.bytes = std::move(bytes);
    r.ok = ok;
    r.delivered = true;
  }
  g_fetchCv.notify_all();
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
        const std::string& method = call.method_name();

        if (method == "writeFilesCopy") {
          const auto* args =
              std::get_if<flutter::EncodableList>(call.arguments());
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
          return;
        }

        // Announce a set of remote files as delayed-render virtual files.
        // args: {token: string, files: [{name: string, size: int}]}
        if (method == "announceRemoteFiles") {
          const auto* m = std::get_if<flutter::EncodableMap>(call.arguments());
          if (!m) {
            result->Success(flutter::EncodableValue(false));
            return;
          }
          std::wstring token;
          std::vector<RemoteFileEntry> files;
          for (const auto& kv : *m) {
            const auto* key = std::get_if<std::string>(&kv.first);
            if (!key) continue;
            if (*key == "token") {
              if (const auto* s = std::get_if<std::string>(&kv.second))
                token = Widen(*s);
            } else if (*key == "files") {
              const auto* list =
                  std::get_if<flutter::EncodableList>(&kv.second);
              if (!list) continue;
              for (const auto& fv : *list) {
                const auto* fm = std::get_if<flutter::EncodableMap>(&fv);
                if (!fm) continue;
                RemoteFileEntry e;
                for (const auto& fkv : *fm) {
                  const auto* fk = std::get_if<std::string>(&fkv.first);
                  if (!fk) continue;
                  if (*fk == "name") {
                    if (const auto* s = std::get_if<std::string>(&fkv.second))
                      e.name = Widen(*s);
                  } else if (*fk == "size") {
                    if (const auto* i = std::get_if<int>(&fkv.second))
                      e.size = static_cast<uint64_t>(*i);
                    else if (const auto* l =
                                 std::get_if<int64_t>(&fkv.second))
                      e.size = static_cast<uint64_t>(*l);
                  }
                }
                if (!e.name.empty()) files.push_back(std::move(e));
              }
            }
          }
          result->Success(
              flutter::EncodableValue(SetRemoteFileClipboard(token, files)));
          return;
        }

        // Return pending byte-fetch requests for Dart to service.
        if (method == "pollFileRequests") {
          result->Success(PollFileRequests());
          return;
        }

        // Deliver fetched bytes back to a blocked paste.
        // args: {token: string, index: int, ok: bool, bytes: Uint8List}
        if (method == "deliverRemoteFileBytes") {
          const auto* m = std::get_if<flutter::EncodableMap>(call.arguments());
          if (!m) {
            result->Success(flutter::EncodableValue(false));
            return;
          }
          std::wstring token;
          uint32_t index = 0;
          bool ok = false;
          std::vector<BYTE> bytes;
          for (const auto& kv : *m) {
            const auto* key = std::get_if<std::string>(&kv.first);
            if (!key) continue;
            if (*key == "token") {
              if (const auto* s = std::get_if<std::string>(&kv.second))
                token = Widen(*s);
            } else if (*key == "index") {
              if (const auto* i = std::get_if<int>(&kv.second))
                index = static_cast<uint32_t>(*i);
            } else if (*key == "ok") {
              if (const auto* b = std::get_if<bool>(&kv.second)) ok = *b;
            } else if (*key == "bytes") {
              if (const auto* v =
                      std::get_if<std::vector<uint8_t>>(&kv.second))
                bytes = *v;
            }
          }
          DeliverFileBytes(token, index, ok, std::move(bytes));
          result->Success(flutter::EncodableValue(true));
          return;
        }

        result->NotImplemented();
      });

  static std::shared_ptr<flutter::MethodChannel<flutter::EncodableValue>>
      g_channel;
  g_channel = channel;
}
