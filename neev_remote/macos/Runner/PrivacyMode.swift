import Cocoa
import FlutterMacOS

/// Host privacy mode on macOS (parity with the Windows privacy feature): covers
/// every screen with a black window that is EXCLUDED from screen capture (so the
/// remote viewer still sees the real desktop), and blocks the LOCAL user's
/// physical mouse/keyboard while letting remote-injected input through.
/// Requires Accessibility (for the input-blocking event tap).
class PrivacyMode {
  private var windows: [NSWindow] = []
  private var tap: CFMachPort?
  private var runLoopSource: CFRunLoopSource?

  static func register(messenger: FlutterBinaryMessenger) {
    let channel = FlutterMethodChannel(
      name: "neev_remote/privacy", binaryMessenger: messenger)
    let plugin = PrivacyMode()
    PrivacyMode.retained = (channel, plugin)
    channel.setMethodCallHandler { call, result in
      if call.method == "setPrivacy" {
        let on = (call.arguments as? Bool) ?? false
        DispatchQueue.main.async { on ? plugin.enable() : plugin.disable() }
        result(nil)
      } else {
        result(FlutterMethodNotImplemented)
      }
    }
  }
  private static var retained: (FlutterMethodChannel, PrivacyMode)?

  private func enable() {
    if windows.isEmpty {
      for screen in NSScreen.screens {
        let w = NSWindow(contentRect: screen.frame, styleMask: .borderless,
                         backing: .buffered, defer: false)
        w.backgroundColor = .black
        w.isOpaque = true
        w.level = .screenSaver
        w.ignoresMouseEvents = true
        w.collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary]
        w.sharingType = .none // exclude from screen capture — viewer sees the real screen
        w.setFrame(screen.frame, display: true)
        w.orderFrontRegardless()
        windows.append(w)
      }
    }
    installInputBlock()
  }

  private func disable() {
    for w in windows { w.orderOut(nil) }
    windows.removeAll()
    removeInputBlock()
  }

  private func installInputBlock() {
    if tap != nil { return }
    let types: [CGEventType] = [
      .keyDown, .keyUp, .flagsChanged,
      .leftMouseDown, .leftMouseUp, .rightMouseDown, .rightMouseUp,
      .otherMouseDown, .otherMouseUp, .mouseMoved,
      .leftMouseDragged, .rightMouseDragged, .scrollWheel,
    ]
    var mask: CGEventMask = 0
    for t in types { mask |= (UInt64(1) << UInt64(t.rawValue)) }
    let callback: CGEventTapCallBack = { _, _, event, refcon in
      // Let OUR injected events (tagged) through; consume the local user's input.
      if event.getIntegerValueField(.eventSourceUserData) == InputInjector.injectedTag {
        return Unmanaged.passUnretained(event)
      }
      return nil
    }
    guard let t = CGEvent.tapCreate(
      tap: .cgSessionEventTap, place: .headInsertEventTap, options: .defaultTap,
      eventsOfInterest: mask, callback: callback, userInfo: nil) else { return }
    tap = t
    let src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, t, 0)
    runLoopSource = src
    CFRunLoopAddSource(CFRunLoopGetMain(), src, .commonModes)
    CGEvent.tapEnable(tap: t, enable: true)
  }

  private func removeInputBlock() {
    if let src = runLoopSource {
      CFRunLoopRemoveSource(CFRunLoopGetMain(), src, .commonModes)
    }
    if let t = tap { CGEvent.tapEnable(tap: t, enable: false) }
    tap = nil
    runLoopSource = nil
  }
}
