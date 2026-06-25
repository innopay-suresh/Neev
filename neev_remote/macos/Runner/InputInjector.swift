import ApplicationServices
import Cocoa
import FlutterMacOS

/// Injects remote mouse/keyboard events into macOS via CGEvent.
///
/// Requires the app to be granted **Accessibility** permission
/// (System Settings → Privacy & Security → Accessibility) to post events to
/// other applications.
class InputInjector {
  private let source = CGEventSource(stateID: .hidSystemState)
  private var leftDown = false
  private var rightDown = false
  private var otherDown = false
  private var modifierFlags: CGEventFlags = []

  static func register(messenger: FlutterBinaryMessenger) {
    let channel = FlutterMethodChannel(
      name: "neev_remote/input", binaryMessenger: messenger)
    let injector = InputInjector()
    channel.setMethodCallHandler { call, result in
      if call.method == "inject", let args = call.arguments as? [String: Any] {
        injector.inject(args)
        result(nil)
      } else {
        result(FlutterMethodNotImplemented)
      }
    }
    // Retain the channel/injector for the process lifetime.
    InputInjector.retained = (channel, injector)

    // Ask for Accessibility permission so injected clicks/keys reach other
    // apps. Prompts once if not yet granted; no-op once the user allows it.
    let promptKey = kAXTrustedCheckOptionPrompt.takeUnretainedValue() as NSString
    _ = AXIsProcessTrustedWithOptions([promptKey: true] as CFDictionary)
  }

  private static var retained: (FlutterMethodChannel, InputInjector)?

  private func screenSize() -> CGSize {
    let bounds = CGDisplayBounds(CGMainDisplayID())
    return bounds.size
  }

  private func point(_ args: [String: Any]) -> CGPoint {
    let size = screenSize()
    let nx = (args["x"] as? Double) ?? 0
    let ny = (args["y"] as? Double) ?? 0
    return CGPoint(x: nx * Double(size.width), y: ny * Double(size.height))
  }

  private var lastPos = CGPoint.zero

  func inject(_ args: [String: Any]) {
    guard let kind = args["k"] as? String else { return }
    switch kind {
    case "mv":
      lastPos = point(args)
      let type: CGEventType = leftDown ? .leftMouseDragged
        : (rightDown ? .rightMouseDragged
        : (otherDown ? .otherMouseDragged : .mouseMoved))
      let button: CGMouseButton = rightDown ? .right : (otherDown ? .center : .left)
      post(type: type, at: lastPos, button: button)
    case "btn":
      let b = (args["b"] as? Int) ?? 0
      let down = (args["d"] as? Bool) ?? false
      // Use the click position from args so the click lands correctly even if
      // mv/btn messages arrived out of order over the data channel.
      let pos = point(args)
      mouseButton(b, down, at: pos)
    case "whl":
      let dy = (args["dy"] as? Double) ?? 0
      let dx = (args["dx"] as? Double) ?? 0
      if let e = CGEvent(scrollWheelEvent2Source: source, units: .pixel,
                         wheelCount: 2, wheel1: Int32(-dy), wheel2: Int32(-dx),
                         wheel3: 0) {
        e.post(tap: .cghidEventTap)
      }
    case "key":
      let usage = (args["u"] as? Int) ?? 0
      let down = (args["d"] as? Bool) ?? false
      // Track modifier state so capitals, symbols and shortcuts (Cmd+C/V) work.
      if let flag = InputInjector.modifierFlag(usage) {
        if down { modifierFlags.insert(flag) } else { modifierFlags.remove(flag) }
      }
      if let vk = InputInjector.hidToKeyCode(usage),
         let e = CGEvent(keyboardEventSource: source, virtualKey: vk,
                         keyDown: down) {
        e.flags = modifierFlags
        e.post(tap: .cghidEventTap)
      }
    default:
      break
    }
  }

  private func mouseButton(_ b: Int, _ down: Bool, at pos: CGPoint) {
    let type: CGEventType
    let button: CGMouseButton
    switch b {
    case 1:
      rightDown = down
      type = down ? .rightMouseDown : .rightMouseUp
      button = .right
    case 2:
      otherDown = down
      type = down ? .otherMouseDown : .otherMouseUp
      button = .center
    default:
      leftDown = down
      type = down ? .leftMouseDown : .leftMouseUp
      button = .left
    }
    lastPos = pos
    post(type: type, at: pos, button: button)
  }

  private func post(type: CGEventType, at pos: CGPoint, button: CGMouseButton) {
    if let e = CGEvent(mouseEventSource: source, mouseType: type,
                       mouseCursorPosition: pos, mouseButton: button) {
      e.flags = modifierFlags
      e.post(tap: .cghidEventTap)
    }
  }

  /// Maps a USB HID modifier usage to its CGEventFlags bit.
  static func modifierFlag(_ usage: Int) -> CGEventFlags? {
    switch usage {
    case 0xE0, 0xE4: return .maskControl
    case 0xE1, 0xE5: return .maskShift
    case 0xE2, 0xE6: return .maskAlternate
    case 0xE3, 0xE7: return .maskCommand
    default: return nil
    }
  }

  /// USB HID usage → macOS CGKeyCode (ANSI layout). Returns nil when unmapped.
  static func hidToKeyCode(_ usage: Int) -> CGKeyCode? {
    let map: [Int: CGKeyCode] = [
      // Letters a-z
      0x04: 0, 0x05: 11, 0x06: 8, 0x07: 2, 0x08: 14, 0x09: 3, 0x0A: 5,
      0x0B: 4, 0x0C: 34, 0x0D: 38, 0x0E: 40, 0x0F: 37, 0x10: 46, 0x11: 45,
      0x12: 31, 0x13: 35, 0x14: 12, 0x15: 15, 0x16: 1, 0x17: 17, 0x18: 32,
      0x19: 9, 0x1A: 13, 0x1B: 7, 0x1C: 16, 0x1D: 6,
      // 1-9, 0
      0x1E: 18, 0x1F: 19, 0x20: 20, 0x21: 21, 0x22: 23, 0x23: 22, 0x24: 26,
      0x25: 28, 0x26: 25, 0x27: 29,
      // Control keys
      0x28: 36, 0x29: 53, 0x2A: 51, 0x2B: 48, 0x2C: 49,
      0x2D: 27, 0x2E: 24, 0x2F: 33, 0x30: 30, 0x31: 42, 0x33: 41, 0x34: 39,
      0x35: 50, 0x36: 43, 0x37: 47, 0x38: 44, 0x39: 57,
      // F1-F12
      0x3A: 122, 0x3B: 120, 0x3C: 99, 0x3D: 118, 0x3E: 96, 0x3F: 97,
      0x40: 98, 0x41: 100, 0x42: 101, 0x43: 109, 0x44: 103, 0x45: 111,
      // Navigation
      0x49: 114, 0x4A: 115, 0x4B: 116, 0x4C: 117, 0x4D: 119, 0x4E: 121,
      0x4F: 124, 0x50: 123, 0x51: 125, 0x52: 126,
      // Modifiers
      0xE0: 59, 0xE1: 56, 0xE2: 58, 0xE3: 55,
      0xE4: 62, 0xE5: 60, 0xE6: 61, 0xE7: 54,
    ]
    if let code = map[usage] { return code }
    return nil
  }
}
