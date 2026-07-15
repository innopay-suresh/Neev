import Cocoa
import FlutterMacOS

/// Native macOS clipboard bridge over `neev_remote/clipboard`.
///
/// WHY NATIVE: macOS clipboard change-detection must use
/// `NSPasteboard.general.changeCount` — the only reliable signal. The old Dart
/// approach read the content every poll and hashed it; after the host wrote a
/// received image, the next read returned bytes it could not distinguish from a
/// fresh user copy (cheap-hash collisions + write/read round-trips), so sync
/// wedged after the first item ("works once then stops"). changeCount increments
/// exactly once per pasteboard mutation, so we detect real changes precisely and
/// suppress echo by remembering the count OUR OWN writes produced.
///
/// Also fixes file COPY semantics: writing file URLs via NSPasteboard.writeObjects
/// pastes as a COPY (the Dart `Pasteboard.writeFiles` fallback behaved like a move,
/// so staged temp files vanished).
///
/// macOS ONLY. Windows/Linux keep their existing Dart poller + native
/// ClipboardWriter untouched — this class is never constructed off macOS.
class ClipboardMonitor {
  private let channel: FlutterMethodChannel
  private static var retained: ClipboardMonitor?
  private let pb = NSPasteboard.general
  private var timer: Timer?
  private var lastSeen: Int = 0
  private var selfChange: Int = -1  // changeCount WE produced — never echo it

  init(messenger: FlutterBinaryMessenger) {
    channel = FlutterMethodChannel(
      name: "neev_remote/clipboard", binaryMessenger: messenger)
  }

  static func register(messenger: FlutterBinaryMessenger) {
    let m = ClipboardMonitor(messenger: messenger)
    ClipboardMonitor.retained = m
    m.channel.setMethodCallHandler { call, result in m.handle(call, result) }
  }

  private func handle(_ call: FlutterMethodCall, _ result: FlutterResult) {
    switch call.method {
    case "start":
      start()
      result(nil)
    case "stop":
      stop()
      result(nil)
    case "writeText":
      if let s = call.arguments as? String { writeText(s) }
      result(nil)
    case "writeImage":
      if let d = call.arguments as? FlutterStandardTypedData { writeImage(d.data) }
      result(nil)
    case "writeFiles":
      if let a = call.arguments as? [String] { writeFiles(a) }
      result(nil)
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  private func start() {
    if timer != nil { return }
    lastSeen = pb.changeCount  // don't fire for content already on the pasteboard
    let t = Timer(timeInterval: 0.3, repeats: true) { [weak self] _ in self?.poll() }
    RunLoop.main.add(t, forMode: .common)
    timer = t
  }

  private func stop() {
    timer?.invalidate()
    timer = nil
  }

  private func poll() {
    let cc = pb.changeCount
    if cc == lastSeen { return }
    lastSeen = cc
    if cc == selfChange { return }  // our own write — do not echo it back
    // Real user change. Prefer files > image > text (richest representation).
    if let files = readFiles() {
      channel.invokeMethod("changed", arguments: ["type": "files", "paths": files])
    } else if let png = readImagePNG() {
      channel.invokeMethod(
        "changed",
        arguments: ["type": "image", "data": FlutterStandardTypedData(bytes: png)])
    } else if let s = pb.string(forType: .string) {
      channel.invokeMethod("changed", arguments: ["type": "text", "text": s])
    }
  }

  private func readFiles() -> [String]? {
    let opts: [NSPasteboard.ReadingOptionKey: Any] = [.urlReadingFileURLsOnly: true]
    guard
      let objs = pb.readObjects(forClasses: [NSURL.self], options: opts) as? [URL]
    else { return nil }
    let paths = objs.filter { $0.isFileURL }.map { $0.path }
    return paths.isEmpty ? nil : paths
  }

  private func readImagePNG() -> Data? {
    if let png = pb.data(forType: .png) { return png }
    // Screenshots and many apps put TIFF — transcode to PNG (what the wire + the
    // Windows host's CF_DIB decoder expect).
    guard let tiff = pb.data(forType: .tiff),
      let rep = NSBitmapImageRep(data: tiff)
    else { return nil }
    return rep.representation(using: .png, properties: [:])
  }

  private func writeText(_ s: String) {
    pb.clearContents()
    pb.setString(s, forType: .string)
    markSelf()
  }

  private func writeImage(_ data: Data) {
    pb.clearContents()
    pb.setData(data, forType: .png)
    markSelf()
  }

  private func writeFiles(_ paths: [String]) {
    pb.clearContents()
    let urls = paths.map { URL(fileURLWithPath: $0) as NSURL }
    pb.writeObjects(urls)
    markSelf()
  }

  /// Record that WE just mutated the pasteboard, so the next poll ignores it.
  private func markSelf() {
    selfChange = pb.changeCount
    lastSeen = pb.changeCount
  }
}
