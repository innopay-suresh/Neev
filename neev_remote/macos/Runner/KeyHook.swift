import Cocoa
import FlutterMacOS

/// Viewer-side keyboard capture (parity with the Windows key hook). While ON, a
/// session CGEventTap grabs ALL keys — including OS-reserved combos (Cmd+Tab,
/// Cmd+Space, Mission Control) that Flutter never receives — buffers them as USB
/// HID usages, and CONSUMES them so they go to the remote instead of acting on
/// this Mac. The Dart side drains the buffer every ~12ms. Requires Accessibility.
class KeyHook {
  private var tap: CFMachPort?
  private var runLoopSource: CFRunLoopSource?
  private var buffer: [[String: Any]] = []
  private let lock = NSLock()
  private var lastFlags: CGEventFlags = []

  static func register(messenger: FlutterBinaryMessenger) {
    let channel = FlutterMethodChannel(
      name: "neev_remote/keyhook", binaryMessenger: messenger)
    let hook = KeyHook()
    KeyHook.retained = (channel, hook)
    channel.setMethodCallHandler { call, result in
      switch call.method {
      case "setCapture":
        let on = (call.arguments as? Bool) ?? false
        DispatchQueue.main.async { on ? hook.enable() : hook.disable() }
        result(nil)
      case "drain":
        result(hook.drain())
      default:
        result(FlutterMethodNotImplemented)
      }
    }
  }
  private static var retained: (FlutterMethodChannel, KeyHook)?

  private func enable() {
    if tap != nil { return }
    let mask = (UInt64(1) << UInt64(CGEventType.keyDown.rawValue))
      | (UInt64(1) << UInt64(CGEventType.keyUp.rawValue))
      | (UInt64(1) << UInt64(CGEventType.flagsChanged.rawValue))
    let callback: CGEventTapCallBack = { _, type, event, refcon in
      let hook = Unmanaged<KeyHook>.fromOpaque(refcon!).takeUnretainedValue()
      return hook.handle(type: type, event: event)
    }
    guard let t = CGEvent.tapCreate(
      tap: .cgSessionEventTap, place: .headInsertEventTap, options: .defaultTap,
      eventsOfInterest: CGEventMask(mask), callback: callback,
      userInfo: Unmanaged.passUnretained(self).toOpaque()) else { return }
    tap = t
    let src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, t, 0)
    runLoopSource = src
    CFRunLoopAddSource(CFRunLoopGetMain(), src, .commonModes)
    CGEvent.tapEnable(tap: t, enable: true)
  }

  private func disable() {
    if let src = runLoopSource {
      CFRunLoopRemoveSource(CFRunLoopGetMain(), src, .commonModes)
    }
    if let t = tap { CGEvent.tapEnable(tap: t, enable: false) }
    tap = nil
    runLoopSource = nil
    lastFlags = []
  }

  private func handle(type: CGEventType, event: CGEvent) -> Unmanaged<CGEvent>? {
    // A disabled-by-timeout tap must be re-enabled or it stays dead.
    if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
      if let t = tap { CGEvent.tapEnable(tap: t, enable: true) }
      return Unmanaged.passUnretained(event)
    }
    let code = Int(event.getIntegerValueField(.keyboardEventKeycode))
    switch type {
    case .keyDown, .keyUp:
      if let usage = KeyHook.keyCodeToHid[code] {
        push(usage, type == .keyDown)
        return nil // consume so it goes to the remote, not locally
      }
      return Unmanaged.passUnretained(event)
    case .flagsChanged:
      let flags = event.flags
      if let usage = KeyHook.keyCodeToHid[code], let flag = KeyHook.flagFor[code] {
        let down = flags.contains(flag) && !lastFlags.contains(flag)
        push(usage, down)
      }
      lastFlags = flags
      return nil
    default:
      return Unmanaged.passUnretained(event)
    }
  }

  private func push(_ usage: Int, _ down: Bool) {
    lock.lock()
    buffer.append(["u": usage, "d": down])
    lock.unlock()
  }

  func drain() -> [[String: Any]] {
    lock.lock()
    defer { lock.unlock() }
    let out = buffer
    buffer.removeAll()
    return out
  }

  /// macOS virtual keyCode → USB HID usage (inverse of InputInjector.hidToKeyCode).
  static let keyCodeToHid: [Int: Int] = [
    0: 0x04, 11: 0x05, 8: 0x06, 2: 0x07, 14: 0x08, 3: 0x09, 5: 0x0A, 4: 0x0B,
    34: 0x0C, 38: 0x0D, 40: 0x0E, 37: 0x0F, 46: 0x10, 45: 0x11, 31: 0x12,
    35: 0x13, 12: 0x14, 15: 0x15, 1: 0x16, 17: 0x17, 32: 0x18, 9: 0x19, 13: 0x1A,
    7: 0x1B, 16: 0x1C, 6: 0x1D,
    18: 0x1E, 19: 0x1F, 20: 0x20, 21: 0x21, 23: 0x22, 22: 0x23, 26: 0x24,
    28: 0x25, 25: 0x26, 29: 0x27,
    36: 0x28, 53: 0x29, 51: 0x2A, 48: 0x2B, 49: 0x2C, 27: 0x2D, 24: 0x2E,
    33: 0x2F, 30: 0x30, 42: 0x31, 41: 0x33, 39: 0x34, 50: 0x35, 43: 0x36,
    47: 0x37, 44: 0x38, 57: 0x39,
    122: 0x3A, 120: 0x3B, 99: 0x3C, 118: 0x3D, 96: 0x3E, 97: 0x3F, 98: 0x40,
    100: 0x41, 101: 0x42, 109: 0x43, 103: 0x44, 111: 0x45,
    114: 0x49, 115: 0x4A, 116: 0x4B, 117: 0x4C, 119: 0x4D, 121: 0x4E, 124: 0x4F,
    123: 0x50, 125: 0x51, 126: 0x52,
    59: 0xE0, 56: 0xE1, 58: 0xE2, 55: 0xE3, 62: 0xE4, 60: 0xE5, 61: 0xE6, 54: 0xE7,
  ]

  /// Modifier keyCode → its CGEventFlags bit (for flagsChanged down/up detection).
  static let flagFor: [Int: CGEventFlags] = [
    55: .maskCommand, 54: .maskCommand,
    56: .maskShift, 60: .maskShift,
    58: .maskAlternate, 61: .maskAlternate,
    59: .maskControl, 62: .maskControl,
    57: .maskAlphaShift,
  ]
}
