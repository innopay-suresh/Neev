import CoreGraphics
import FlutterMacOS
import Foundation

/// Screen Recording (TCC) preflight for the capture path.
///
/// Without this the app CRASHED on a Mac that had not granted Screen Recording.
/// flutter_webrtc's desktop source enumeration does not return an error when the
/// grant is missing — ObjCDesktopMediaList::UpdateSourceList calls abort(), so
/// the whole process dies with SIGABRT the moment hosting starts. The observed
/// effect was worse than a crash dialog: the app relaunched, re-registered with
/// the relay, and a viewer dialing it sat at "Requesting the device" forever,
/// because the host was aborting before it could ever show the consent prompt.
/// Nothing pointed at a permission.
///
/// CGPreflightScreenCaptureAccess answers the question WITHOUT prompting and
/// without touching the capture stack, so the app can refuse to start hosting
/// and say why, instead of dying.
enum ScreenPermission {
  static func register(messenger: FlutterBinaryMessenger) {
    let channel = FlutterMethodChannel(
      name: "neev_remote/screenpermission", binaryMessenger: messenger)
    channel.setMethodCallHandler { call, result in
      switch call.method {
      case "check":
        // Never prompts. Safe to call on every hosting attempt.
        result(CGPreflightScreenCaptureAccess())
      case "request":
        // Prompts once per code identity, then returns immediately forever
        // after. macOS only applies the answer to a NEW process, so a first
        // grant still needs a restart — the caller says so rather than
        // pretending capture will work now.
        result(CGRequestScreenCaptureAccess())
      default:
        result(FlutterMethodNotImplemented)
      }
    }
    ScreenPermission.retained = channel
  }

  private static var retained: FlutterMethodChannel?
}
