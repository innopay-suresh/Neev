import AppKit

/// The host's on-screen session bar for macOS.
///
/// A Windows host draws a real always-on-top bar (sessionbar_windows.go), so the
/// person being viewed can see a session is live and stop it. macOS had the same
/// four controls but only inside a menu-bar dropdown, which people simply do not
/// find: reported as "I get these dock buttons in win-win but not win-mac or
/// mac-mac". The controls existed; nothing on screen said so.
///
/// Deliberately a sibling of the menu-bar item rather than a replacement. The
/// menu stays the fallback while the bar is hidden, and removing it would be a
/// regression for anyone already used to it.
final class SessionBar {
    private var panel: NSPanel?
    private var revealStrip: NSPanel?
    private var hideTimer: Timer?
    private var buttons: [String: NSButton] = [:]

    /// Called when the host clicks a control. The caller maps these onto the
    /// socket commands the worker already understands, so this adds no protocol.
    var onAction: ((String) -> Void)?

    private let idleBeforeHiding: TimeInterval = 4

    func show() {
        guard panel == nil else { return }
        let width: CGFloat = 420, height: CGFloat = 44
        guard let screen = NSScreen.main else { return }
        let x = screen.frame.midX - width / 2
        let y = screen.visibleFrame.maxY - height - 8

        // .nonactivatingPanel: clicking a control must never steal focus from
        // whatever the host is doing. .statusBar level keeps it above ordinary
        // windows, and the collection behaviour keeps it visible across Spaces
        // and alongside a full-screen app — a session indicator that vanishes
        // when the host switches Space is not an indicator.
        let p = NSPanel(
            contentRect: NSRect(x: x, y: y, width: width, height: height),
            styleMask: [.nonactivatingPanel, .fullSizeContentView, .borderless],
            backing: .buffered, defer: false)
        p.level = .statusBar
        p.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .stationary]
        p.isMovableByWindowBackground = true
        p.hidesOnDeactivate = false
        p.backgroundColor = .clear
        p.isOpaque = false

        let content = NSVisualEffectView(frame: NSRect(x: 0, y: 0, width: width, height: height))
        content.material = .hudWindow
        content.blendingMode = .behindWindow
        content.state = .active
        content.wantsLayer = true
        content.layer?.cornerRadius = 10
        content.layer?.masksToBounds = true

        let label = NSTextField(labelWithString: "Remote session active")
        label.font = .systemFont(ofSize: 12, weight: .semibold)
        label.frame = NSRect(x: 14, y: 13, width: 150, height: 18)
        content.addSubview(label)

        var x0: CGFloat = 168
        for (key, title) in [("record", "Record"), ("sound", "Sound off"),
                             ("mic", "Voice off"), ("end", "Disconnect")] {
            let b = NSButton(title: title, target: self, action: #selector(tapped(_:)))
            b.bezelStyle = .rounded
            b.frame = NSRect(x: x0, y: 8, width: 60, height: 28)
            b.identifier = NSUserInterfaceItemIdentifier(key)
            content.addSubview(b)
            buttons[key] = b
            x0 += 62
        }

        p.contentView = content
        p.orderFrontRegardless()
        panel = p
        armAutoHide()
        installRevealStrip(screen: screen, width: width)
    }

    @objc private func tapped(_ sender: NSButton) {
        guard let key = sender.identifier?.rawValue else { return }
        onAction?(key)
        armAutoHide() // interacting with it means it should stay a while longer
    }

    /// Reflects worker-reported state, so a recording the VIEWER started still
    /// shows here. The bar holds no state of its own.
    func setState(mic: Bool, sound: Bool, rec: Bool) {
        buttons["mic"]?.title = mic ? "Voice on" : "Voice off"
        buttons["sound"]?.title = sound ? "Sound on" : "Sound off"
        buttons["record"]?.title = rec ? "Recording" : "Record"
    }

    private func armAutoHide() {
        hideTimer?.invalidate()
        hideTimer = Timer.scheduledTimer(withTimeInterval: idleBeforeHiding, repeats: false) { [weak self] _ in
            self?.hideToEdge()
        }
    }

    /// Hidden means ORDERED OUT, not merely transparent.
    ///
    /// A bar left on screen with ignoresMouseEvents still covers the menu bar
    /// area visually. The host's own desktop has to stay completely usable while
    /// nobody is interacting with the bar — it is an indicator, not an overlay.
    private func hideToEdge() {
        panel?.orderOut(nil)
        revealStrip?.orderFrontRegardless()
    }

    /// A 3px strip at the top edge that brings the bar back on mouse-enter.
    ///
    /// A tracking area rather than a global NSEvent monitor on purpose: a global
    /// monitor would add a permission dependency (and another thing to explain
    /// when it silently stops working), for behaviour a strip gives for free.
    private func installRevealStrip(screen: NSScreen, width: CGFloat) {
        guard revealStrip == nil else { return }
        let h: CGFloat = 3
        let rect = NSRect(x: screen.frame.midX - width / 2,
                          y: screen.visibleFrame.maxY - h,
                          width: width, height: h)
        let s = NSPanel(contentRect: rect,
                        styleMask: [.nonactivatingPanel, .borderless],
                        backing: .buffered, defer: false)
        s.level = .statusBar
        s.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .stationary]
        s.backgroundColor = NSColor.systemOrange.withAlphaComponent(0.55)
        s.isOpaque = false
        s.ignoresMouseEvents = false

        let v = HoverView(frame: NSRect(origin: .zero, size: rect.size))
        v.onEnter = { [weak self] in self?.reveal() }
        s.contentView = v
        s.orderOut(nil)
        revealStrip = s
    }

    private func reveal() {
        revealStrip?.orderOut(nil)
        panel?.orderFrontRegardless()
        armAutoHide()
    }

    func close() {
        hideTimer?.invalidate()
        panel?.orderOut(nil)
        revealStrip?.orderOut(nil)
        panel = nil
        revealStrip = nil
        buttons.removeAll()
    }
}

/// Reports mouse-enter, so the reveal strip needs no global event monitor.
final class HoverView: NSView {
    var onEnter: (() -> Void)?

    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        trackingAreas.forEach(removeTrackingArea)
        addTrackingArea(NSTrackingArea(
            rect: bounds,
            options: [.mouseEnteredAndExited, .activeAlways],
            owner: self, userInfo: nil))
    }

    override func mouseEntered(with _: NSEvent) { onEnter?() }
}
