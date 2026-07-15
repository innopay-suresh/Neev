import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.title = "Neev Remote"
    self.setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)
    InputInjector.register(messenger: flutterViewController.engine.binaryMessenger)
    PrivacyMode.register(messenger: flutterViewController.engine.binaryMessenger)
    KeyHook.register(messenger: flutterViewController.engine.binaryMessenger)
    SessionWatcher.register(messenger: flutterViewController.engine.binaryMessenger)

    super.awakeFromNib()
  }
}
