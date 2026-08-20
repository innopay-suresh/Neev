import AppKit

/// The host session bar's palette, shared with the Windows bar.
///
/// The two bars sit side by side in cross-platform testing, so they must not be
/// different oranges. These are the exact values consentwin_windows.go uses
/// (cwColAccent / cwColInk / cwColCard); NSColor.systemOrange and .systemRed
/// were a fourth and fifth colour in a product that already had three.
enum BarPalette {
    static let accent = NSColor(srgbRed: 0xFF / 255, green: 0x6B / 255, blue: 0x00 / 255, alpha: 1)
    static let danger = NSColor(srgbRed: 0xD8 / 255, green: 0x49 / 255, blue: 0x3F / 255, alpha: 1)
    static let ink = NSColor(srgbRed: 0x17 / 255, green: 0x17 / 255, blue: 0x14 / 255, alpha: 1)
}

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
        guard let screen = NSScreen.main else { return }

        // Build the buttons FIRST and size the bar to fit them.
        //
        // Fixed 60pt widths truncated every label to "Rec...", "Sou...",
        // "Voic...", "Dis..." — a control bar whose controls cannot be read is
        // no better than the hidden menu it was meant to replace. Widths now
        // come from each button's own intrinsic size, so a longer state label
        // ("Recording", "Sound on") still fits.
        let label = NSTextField(labelWithString: "Remote session active")
        label.font = .systemFont(ofSize: 12, weight: .semibold)
        label.textColor = BarPalette.ink
        label.sizeToFit()

        var made: [(String, NSButton)] = []
        for (key, title) in [("record", "Record"), ("sound", "Sound off"),
                             ("mic", "Voice off"), ("end", "Disconnect")] {
            let b = NSButton(title: title, target: self, action: #selector(tapped(_:)))
            b.bezelStyle = .rounded
            b.identifier = NSUserInterfaceItemIdentifier(key)
            b.sizeToFit()
            // Reserve room for the LONGEST state each button can show, so the
            // layout does not jump when a label changes under the pointer.
            let longest = ["record": "Recording", "sound": "Sound off",
                           "mic": "Voice off", "end": "Disconnect"][key] ?? title
            let probe = NSButton(title: longest, target: nil, action: nil)
            probe.bezelStyle = .rounded
            probe.sizeToFit()
            b.setFrameSize(NSSize(width: max(b.frame.width, probe.frame.width) + 10,
                                  height: 30))
            made.append((key, b))
        }

        let pad: CGFloat = 14, gap: CGFloat = 8, height: CGFloat = 46
        var width = pad + label.frame.width + gap * 2
        for (_, b) in made { width += b.frame.width + gap }
        width += pad - gap

        let x = screen.frame.midX - width / 2
        let y = screen.visibleFrame.maxY - height - 8

        // .nonactivatingPanel: clicking a control must never steal focus from
        // whatever the host is doing. .statusBar level keeps it above ordinary
        // windows, and the collection behaviour keeps it visible across Spaces
        // and alongside a full-screen app — an indicator that vanishes when the
        // host switches Space is not an indicator.
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
        // .windowBackground on a light appearance, not .hudWindow: the Windows
        // bar is a light card, and a dark translucent HUD beside it reads as a
        // different product.
        content.material = .windowBackground
        content.appearance = NSAppearance(named: .aqua)
        content.blendingMode = .behindWindow
        content.state = .active
        content.wantsLayer = true
        content.layer?.cornerRadius = 10
        content.layer?.masksToBounds = true

        label.frame.origin = NSPoint(x: pad, y: (height - label.frame.height) / 2)
        content.addSubview(label)

        var x0 = pad + label.frame.width + gap * 2
        for (key, b) in made {
            b.frame.origin = NSPoint(x: x0, y: (height - b.frame.height) / 2)
            content.addSubview(b)
            buttons[key] = b
            x0 += b.frame.width + gap
        }

        p.contentView = content
        p.orderFrontRegardless()
        panel = p
        style(mic: false, sound: false, rec: false)
        armAutoHide()
        installRevealStrip(screen: screen, width: width)
    }

    /// Colour carries the state, because the label alone did not.
    ///
    /// "Sound off" and "Sound on" differ by two characters at a glance, so the
    /// host could not tell whether their microphone was live — the one thing a
    /// session bar exists to answer. An active control is filled; Disconnect is
    /// always tinted as destructive.
    private func style(mic: Bool, sound: Bool, rec: Bool) {
        func paint(_ key: String, active: Bool, activeColor: NSColor) {
            guard let b = buttons[key] else { return }
            b.bezelColor = active ? activeColor : nil
            b.contentTintColor = active ? .white : nil
        }
        paint("record", active: rec, activeColor: BarPalette.danger)
        paint("sound", active: sound, activeColor: BarPalette.accent)
        paint("mic", active: mic, activeColor: BarPalette.accent)
        buttons["end"]?.bezelColor = BarPalette.danger
        buttons["end"]?.contentTintColor = .white
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
        style(mic: mic, sound: sound, rec: rec)
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
        // The strip is the only always-visible part, so it carries the brand
        // accent rather than a system colour.
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
        s.backgroundColor = BarPalette.accent.withAlphaComponent(0.55)
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
