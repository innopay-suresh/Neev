import Cocoa
import FlutterMacOS

/// Watches macOS session state — screen lock/unlock, fast-user-switch, and wake
/// from sleep — and pushes "resume"/"suspend" events to Dart over a method
/// channel. The host uses "resume" to RE-ACQUIRE its screen-capture stream:
/// when the Mac locks (or the session is switched away), the ScreenCaptureKit /
/// getDisplayMedia stream freezes and never restarts on its own, so the remote
/// viewer is stuck on the last frame even after the user unlocks. Re-capturing
/// on resume makes the video recover.
///
/// This does NOT capture the login/lock window itself (that needs a privileged
/// pre-login LaunchAgent). It fixes the common same-user "video frozen after
/// unlock" case entirely in the user session — no elevated permissions needed.
class SessionWatcher {
  private let channel: FlutterMethodChannel
  private static var retained: SessionWatcher?

  init(messenger: FlutterBinaryMessenger) {
    channel = FlutterMethodChannel(
      name: "neev_remote/session", binaryMessenger: messenger)
  }

  static func register(messenger: FlutterBinaryMessenger) {
    let watcher = SessionWatcher(messenger: messenger)
    SessionWatcher.retained = watcher
    watcher.start()
    // Also handle inbound calls: the host activates itself before opening a file
    // picker so the panel is frontmost and visible to the controlling viewer
    // (otherwise a backgrounded host shows the picker behind its window).
    watcher.channel.setMethodCallHandler { call, result in
      if call.method == "activateApp" {
        NSApplication.shared.activate(ignoringOtherApps: true)
      }
      result(nil)
    }
  }

  private func start() {
    // Screen lock / unlock (com.apple.screenIsLocked / …Unlocked) are only
    // delivered on the *distributed* notification center.
    let dnc = DistributedNotificationCenter.default()
    dnc.addObserver(
      self, selector: #selector(onLocked),
      name: NSNotification.Name("com.apple.screenIsLocked"), object: nil)
    dnc.addObserver(
      self, selector: #selector(onUnlocked),
      name: NSNotification.Name("com.apple.screenIsUnlocked"), object: nil)

    // Fast user switch + wake-from-sleep come from the workspace center.
    let wnc = NSWorkspace.shared.notificationCenter
    wnc.addObserver(
      self, selector: #selector(onResignActive),
      name: NSWorkspace.sessionDidResignActiveNotification, object: nil)
    wnc.addObserver(
      self, selector: #selector(onBecomeActive),
      name: NSWorkspace.sessionDidBecomeActiveNotification, object: nil)
    wnc.addObserver(
      self, selector: #selector(onBecomeActive),
      name: NSWorkspace.didWakeNotification, object: nil)
  }

  @objc private func onLocked() { emit("suspend", "screenLocked") }
  @objc private func onUnlocked() { emit("resume", "screenUnlocked") }
  @objc private func onResignActive() { emit("suspend", "sessionResigned") }
  @objc private func onBecomeActive() { emit("resume", "sessionActive") }

  private func emit(_ event: String, _ reason: String) {
    // Method-channel calls must happen on the main thread.
    DispatchQueue.main.async { [weak self] in
      self?.channel.invokeMethod(
        "sessionEvent", arguments: ["event": event, "reason": reason])
    }
  }
}
