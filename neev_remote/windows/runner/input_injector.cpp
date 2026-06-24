#include "input_injector.h"

#include <windows.h>

#include <flutter/encodable_value.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <memory>
#include <string>

namespace {

// Maps a USB HID keyboard usage code to a Windows Virtual-Key code.
// Returns 0 when unmapped.
WORD HidToVk(int usage) {
  // a-z
  if (usage >= 0x04 && usage <= 0x1D)
    return static_cast<WORD>('A' + (usage - 0x04));
  // 1-9, 0
  if (usage >= 0x1E && usage <= 0x26)
    return static_cast<WORD>('1' + (usage - 0x1E));
  if (usage == 0x27) return static_cast<WORD>('0');
  // F1-F12
  if (usage >= 0x3A && usage <= 0x45)
    return static_cast<WORD>(VK_F1 + (usage - 0x3A));

  switch (usage) {
    case 0x28: return VK_RETURN;
    case 0x29: return VK_ESCAPE;
    case 0x2A: return VK_BACK;
    case 0x2B: return VK_TAB;
    case 0x2C: return VK_SPACE;
    case 0x2D: return VK_OEM_MINUS;
    case 0x2E: return VK_OEM_PLUS;   // '='
    case 0x2F: return VK_OEM_4;      // '['
    case 0x30: return VK_OEM_6;      // ']'
    case 0x31: return VK_OEM_5;      // '\'
    case 0x33: return VK_OEM_1;      // ';'
    case 0x34: return VK_OEM_7;      // '\''
    case 0x35: return VK_OEM_3;      // '`'
    case 0x36: return VK_OEM_COMMA;  // ','
    case 0x37: return VK_OEM_PERIOD; // '.'
    case 0x38: return VK_OEM_2;      // '/'
    case 0x39: return VK_CAPITAL;    // CapsLock
    case 0x49: return VK_INSERT;
    case 0x4A: return VK_HOME;
    case 0x4B: return VK_PRIOR;      // PageUp
    case 0x4C: return VK_DELETE;
    case 0x4D: return VK_END;
    case 0x4E: return VK_NEXT;       // PageDown
    case 0x4F: return VK_RIGHT;
    case 0x50: return VK_LEFT;
    case 0x51: return VK_DOWN;
    case 0x52: return VK_UP;
    case 0xE0: return VK_LCONTROL;
    case 0xE1: return VK_LSHIFT;
    case 0xE2: return VK_LMENU;      // LAlt
    case 0xE3: return VK_LWIN;
    case 0xE4: return VK_RCONTROL;
    case 0xE5: return VK_RSHIFT;
    case 0xE6: return VK_RMENU;      // RAlt
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

// Last pointer position (normalized) so clicks can reposition atomically.
double gLastNx = 0.0;
double gLastNy = 0.0;

void SendMouseAbsolute(double nx, double ny, DWORD flags, DWORD mouseData) {
  INPUT in = {};
  in.type = INPUT_MOUSE;
  // ABSOLUTE coordinates are 0..65535 over the primary monitor.
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
    // Reposition + click atomically so the click lands under the cursor.
    SendMouseAbsolute(gLastNx, gLastNy,
                      MOUSEEVENTF_MOVE | MOUSEEVENTF_ABSOLUTE | btnFlag, 0);
  } else if (*kind == "whl") {
    double dy = GetNum(args, "dy");
    if (dy != 0.0) {
      // Flutter dy>0 = scroll down; Windows wheel>0 = scroll up.
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
          if (args) HandleInject(*args);
          result->Success();
        } else {
          result->NotImplemented();
        }
      });

  // Keep the channel alive for the lifetime of the process.
  static std::shared_ptr<flutter::MethodChannel<flutter::EncodableValue>>
      g_channel;
  g_channel = channel;
}
