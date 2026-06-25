#include "input_injector.h"

#include <windows.h>

#include <flutter/encodable_value.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <memory>
#include <string>

namespace {

WORD HidToVk(int usage) {
  if (usage >= 0x04 && usage <= 0x1D)
    return static_cast<WORD>('A' + (usage - 0x04));
  if (usage >= 0x1E && usage <= 0x26)
    return static_cast<WORD>('1' + (usage - 0x1E));
  if (usage == 0x27) return static_cast<WORD>('0');
  if (usage >= 0x3A && usage <= 0x45)
    return static_cast<WORD>(VK_F1 + (usage - 0x3A));

  switch (usage) {
    case 0x28: return VK_RETURN;
    case 0x29: return VK_ESCAPE;
    case 0x2A: return VK_BACK;
    case 0x2B: return VK_TAB;
    case 0x2C: return VK_SPACE;
    case 0x2D: return VK_OEM_MINUS;
    case 0x2E: return VK_OEM_PLUS;
    case 0x2F: return VK_OEM_4;
    case 0x30: return VK_OEM_6;
    case 0x31: return VK_OEM_5;
    case 0x33: return VK_OEM_1;
    case 0x34: return VK_OEM_7;
    case 0x35: return VK_OEM_3;
    case 0x36: return VK_OEM_COMMA;
    case 0x37: return VK_OEM_PERIOD;
    case 0x38: return VK_OEM_2;
    case 0x39: return VK_CAPITAL;
    case 0x49: return VK_INSERT;
    case 0x4A: return VK_HOME;
    case 0x4B: return VK_PRIOR;
    case 0x4C: return VK_DELETE;
    case 0x4D: return VK_END;
    case 0x4E: return VK_NEXT;
    case 0x4F: return VK_RIGHT;
    case 0x50: return VK_LEFT;
    case 0x51: return VK_DOWN;
    case 0x52: return VK_UP;
    case 0xE0: return VK_LCONTROL;
    case 0xE1: return VK_LSHIFT;
    case 0xE2: return VK_LMENU;
    case 0xE3: return VK_LWIN;
    case 0xE4: return VK_RCONTROL;
    case 0xE5: return VK_RSHIFT;
    case 0xE6: return VK_RMENU;
    case 0xE7: return VK_RWIN;
    default: return 0;
  }
}

bool IsExtendedVk(WORD vk) {
  switch (vk) {
    case VK_RIGHT: case VK_LEFT: case VK_UP: case VK_DOWN:
    case VK_HOME: case VK_END: case VK_PRIOR: case VK_NEXT:
    case VK_INSERT: case VK_DELETE:
    case VK_RCONTROL: case VK_RMENU:
    case VK_LWIN: case VK_RWIN:
      return true;
    default:
      return false;
  }
}

template <typename T>
const T* Find(const flutter::EncodableMap& map, const char* key) {
  auto it = map.find(flutter::EncodableValue(std::string(key)));
  if (it == map.end()) return nullptr;
  return std::get_if<T>(&it->second);
}

double GetNum(const flutter::EncodableMap& map, const char* key) {
  if (auto* d = Find<double>(map, key)) return *d;
  if (auto* i = Find<int>(map, key)) return static_cast<double>(*i);
  return 0.0;
}

double gLastNx = 0.0;
double gLastNy = 0.0;

void SendMouseAbsolute(double nx, double ny, DWORD flags, DWORD mouseData) {
  INPUT in = {};
  in.type = INPUT_MOUSE;
  in.mi.dx = static_cast<LONG>(nx * 65535.0);
  in.mi.dy = static_cast<LONG>(ny * 65535.0);
  in.mi.mouseData = mouseData;
  in.mi.dwFlags = flags;
  SendInput(1, &in, sizeof(INPUT));
}

void HandleInject(const flutter::EncodableMap& args) {
  const auto* kind = Find<std::string>(args, "k");
  if (!kind) return;

  if (*kind == "mv") {
    gLastNx = GetNum(args, "x");
    gLastNy = GetNum(args, "y");
    SendMouseAbsolute(gLastNx, gLastNy,
                      MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE, 0);
  } else if (*kind == "btn") {
    int button = Find<int>(args, "b") ? *Find<int>(args, "b") : 0;
    bool down = Find<bool>(args, "d") && *Find<bool>(args, "d");
    DWORD btnFlag = 0;
    if (button == 1) btnFlag = down ? MOUSEEVENTF_RIGHTDOWN : MOUSEEVENTF_RIGHTUP;
    else if (button == 2) btnFlag = down ? MOUSEEVENTF_MIDDLEDOWN : MOUSEEVENTF_MIDDLEUP;
    else btnFlag = down ? MOUSEEVENTF_LEFTDOWN : MOUSEEVENTF_LEFTUP;
    double nx = GetNum(args, "x");
    double ny = GetNum(args, "y");
    if (nx == 0.0 && ny == 0.0) { nx = gLastNx; ny = gLastNy; }
    SendMouseAbsolute(nx, ny, MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE | btnFlag, 0);
    gLastNx = nx; gLastNy = ny;
  } else if (*kind == "whl") {
    double dy = GetNum(args, "dy");
    if (dy != 0.0) {
      SendMouseAbsolute(0, 0, MOUSEEVENTF_WHEEL,
                        static_cast<DWORD>(static_cast<int>(-dy)));
    }
    double dx = GetNum(args, "dx");
    if (dx != 0.0) {
      SendMouseAbsolute(0, 0, MOUSEEVENTF_HWHEEL,
                        static_cast<DWORD>(static_cast<int>(dx)));
    }
  } else if (*kind == "key") {
    int usage = Find<int>(args, "u") ? *Find<int>(args, "u") : 0;
    bool down = Find<bool>(args, "d") && *Find<bool>(args, "d");
    WORD vk = HidToVk(usage);
    if (vk == 0) return;
    INPUT in = {};
    in.type = INPUT_KEYBOARD;
    in.ki.wVk = vk;
    in.ki.wScan = static_cast<WORD>(MapVirtualKey(vk, MAPVK_VK_TO_VSC));
    in.ki.dwFlags = (down ? 0 : KEYEVENTF_KEYUP);
    if (IsExtendedVk(vk)) in.ki.dwFlags |= KEYEVENTF_EXTENDEDKEY;
    SendInput(1, &in, sizeof(INPUT));
  }
}

// Struct passed to the thread pool work item.
struct InjectWork {
  std::shared_ptr<flutter::EncodableMap> args;
  std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result;
};

void CALLBACK InjectWorkCallback(PTP_CALLBACK_INSTANCE,
                                 void* param,
                                 PTP_WORK) {
  // This runs on a thread pool thread — not the Flutter platform thread.
  auto* work = static_cast<InjectWork*>(param);
  HandleInject(*work->args);
  // Signal completion via the platform thread using Flash's
  // non-blocking task posting.  FlutterWindowsEngine::PostTask threadsafe.
  // Since we can't safely call result->Success() from here, we use
  // SendMessage to a hidden window as a synchronous cross-thread dispatch.
  // Actually for simplicity, just delete the result as "fire-and-forget"
  // — the Flutter side doesn't require a response for input events.
  delete work;
}

}  // namespace

void RegisterInputInjector(flutter::FlutterEngine* engine) {
  auto channel =
      std::make_shared<flutter::MethodChannel<flutter::EncodableValue>>(
          engine->messenger(), "neev_remote/input",
          &flutter::StandardMethodCodec::GetInstance());

  channel->SetMethodCallHandler(
      [](const flutter::MethodCall<flutter::EncodableValue>& call,
         std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>>
             result) {
        if (call.method_name() == "inject") {
          const auto* args =
              std::get_if<flutter::EncodableMap>(call.arguments());
          if (args) {
            // Dispatch input injection to the Windows thread pool so
            // SendInput blocking cannot freeze the Flutter event loop.
            auto work = new InjectWork{
              std::make_shared<flutter::EncodableMap>(*args),
              std::move(result),
            };
            // Use CreateThreadpoolWork for reliable fire-and-forget async.
            static PTP_WORK g_work = CreateThreadpoolWork(
                InjectWorkCallback, work, nullptr);
            SubmitThreadpoolWork(g_work);
            return;
          }
          result->Success();
        } else {
          result->NotImplemented();
        }
      });

  static std::shared_ptr<flutter::MethodChannel<flutter::EncodableValue>>
      g_channel;
  g_channel = channel;
}