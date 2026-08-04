// NeevVoice — the host's session controls on macOS.
//
// A macOS host running the daemon had no way to see that a session was live,
// no way to end it, and no way to speak to the viewer. Windows has a native
// session bar for exactly this; this is the macOS equivalent, as a menu-bar
// item rather than a floating window so it cannot cover the screen the viewer
// is looking at.
//
// It is an .app bundle rather than a bare tool for a second reason: macOS shows
// the microphone prompt on behalf of the process that opens the device, using
// that process's Info.plist. A bare executable produces a prompt with no
// explanation, which users decline.
//
// The helper holds NO state of its own. It asks the worker to toggle and
// renders what the worker reports back, so the menu can never claim the
// microphone is off while it is open.

import AppKit

final class VoiceBar: NSObject, NSApplicationDelegate {
    private var item: NSStatusItem!
    private var sock: Int32 = -1
    private var micOn = false
    private var recOn = false
    private var soundOn = false
    private var tap: AnyObject?
    private let socketPath: String

    init(socketPath: String) {
        self.socketPath = socketPath
        super.init()
    }

    func applicationDidFinishLaunching(_: Notification) {
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        item.button?.image = NSImage(
            systemSymbolName: "dot.radiowaves.left.and.right",
            accessibilityDescription: "Remote session active")
        item.button?.toolTip = "Neev Remote — session active"
        rebuildMenu()
        connect()
    }

    private func rebuildMenu() {
        let menu = NSMenu()

        let status = NSMenuItem(title: "Remote session active", action: nil, keyEquivalent: "")
        status.isEnabled = false
        menu.addItem(status)
        menu.addItem(.separator())

        // Label states the CURRENT state, not the action, so a glance answers
        // "am I being heard right now?" — the only question that matters here.
        let mic = NSMenuItem(
            title: micOn ? "Microphone on — click to mute" : "Microphone off",
            action: #selector(toggleMic), keyEquivalent: "")
        mic.target = self
        mic.state = micOn ? .on : .off
        menu.addItem(mic)

        // System sound, captured with ScreenCaptureKit — no virtual audio
        // device needed. Requires macOS 13; below that the item stays disabled
        // and says why rather than failing when clicked.
        if #available(macOS 13.0, *) {
            let sound = NSMenuItem(
                title: soundOn ? "Sharing this Mac's sound — click to stop"
                               : "Share this Mac's sound",
                action: #selector(toggleSound), keyEquivalent: "")
            sound.target = self
            sound.state = soundOn ? .on : .off
            menu.addItem(sound)
        } else {
            let sound = NSMenuItem(
                title: "Share this Mac's sound — needs macOS 13 or later",
                action: nil, keyEquivalent: "")
            sound.isEnabled = false
            menu.addItem(sound)
        }

        // Recording is offered to the HOST only. A viewer able to silently
        // record this machine's screen would make the product a surveillance
        // tool; the host records their own session, for their own record of it.
        let rec = NSMenuItem(
            title: recOn ? "Recording — click to stop" : "Record this session",
            action: #selector(toggleRec), keyEquivalent: "")
        rec.target = self
        rec.state = recOn ? .on : .off
        menu.addItem(rec)

        menu.addItem(.separator())
        let end = NSMenuItem(title: "End session", action: #selector(endSession), keyEquivalent: "")
        end.target = self
        menu.addItem(end)

        item.menu = menu
    }

    // MARK: - worker link

    private func connect() {
        sock = socket(AF_UNIX, SOCK_STREAM, 0)
        guard sock >= 0 else { return }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = Array(socketPath.utf8)
        // Truncating the path would connect to the WRONG socket, so refuse.
        guard pathBytes.count < MemoryLayout.size(ofValue: addr.sun_path) else {
            NSApp.terminate(nil)
            return
        }
        withUnsafeMutablePointer(to: &addr.sun_path) { raw in
            raw.withMemoryRebound(to: CChar.self, capacity: pathBytes.count + 1) { dst in
                for (i, b) in pathBytes.enumerated() { dst[i] = CChar(bitPattern: b) }
                dst[pathBytes.count] = 0
            }
        }
        let ok = withUnsafePointer(to: &addr) { p in
            p.withMemoryRebound(to: sockaddr.self, capacity: 1) { sa in
                Darwin.connect(sock, sa, socklen_t(MemoryLayout<sockaddr_un>.size)) == 0
            }
        }
        if !ok {
            // No worker means no session — a menu bar item for a session that
            // does not exist is worse than none.
            NSApp.terminate(nil)
            return
        }
        readReplies()
    }

    private func send(_ line: String) {
        guard sock >= 0 else { return }
        _ = (line + "\n").withCString { write(sock, $0, strlen($0)) }
    }

    /// Reads worker replies forever; if the link drops, the session is over.
    private func readReplies() {
        let fd = sock
        DispatchQueue.global(qos: .utility).async { [weak self] in
            var buf = [UInt8](repeating: 0, count: 256)
            while true {
                let n = read(fd, &buf, buf.count)
                if n <= 0 {
                    DispatchQueue.main.async { NSApp.terminate(nil) }
                    return
                }
                guard let text = String(bytes: buf[0..<n], encoding: .utf8) else { continue }
                for line in text.split(separator: "\n") {
                    let on = line.hasSuffix("true")
                    if line.hasPrefix("mic ") {
                        DispatchQueue.main.async {
                            self?.micOn = on
                            self?.rebuildMenu()
                        }
                    } else if line.hasPrefix("rec ") {
                        DispatchQueue.main.async {
                            self?.recOn = on
                            self?.rebuildMenu()
                        }
                    }
                }
            }
        }
    }

    // MARK: - actions

    @objc private func toggleMic() {
        // Ask, do not assume. The menu updates when the worker confirms, so a
        // failed device open cannot leave the menu showing "on".
        send(micOn ? "mic-off" : "mic-on")
    }

    @objc private func toggleSound() {
        guard #available(macOS 13.0, *) else { return }
        if soundOn {
            if let t = tap as? SystemAudioTap {
                Task { await t.stop() }
            }
            tap = nil
            soundOn = false
            rebuildMenu()
            return
        }
        let t = SystemAudioTap { [weak self] frame in
            // Base64 over the existing line-based socket. At 8 kHz mu-law this
            // is ~50 short lines a second — the encoding overhead is nothing
            // next to adding a second binary channel and its framing bugs.
            self?.send("a " + Data(frame).base64EncodedString())
        }
        tap = t
        Task { [weak self] in
            do {
                try await t.start()
                await MainActor.run {
                    self?.soundOn = true
                    self?.rebuildMenu()
                }
            } catch {
                // Almost always the Screen Recording permission: SCStream needs
                // it even for audio only. Say so instead of failing silently.
                NSLog("NeevVoice: could not start system audio: \(error.localizedDescription)")
                await MainActor.run {
                    self?.tap = nil
                    self?.soundOn = false
                    self?.showSoundPermissionHelp()
                    self?.rebuildMenu()
                }
            }
        }
    }

    /// Tells the host what to grant, since the failure is a permission and not
    /// something they can fix by clicking again.
    private func showSoundPermissionHelp() {
        let a = NSAlert()
        a.messageText = "Neev Remote needs Screen Recording to share this Mac's sound"
        a.informativeText = "macOS captures system audio through the same "
            + "permission as screen recording. Open System Settings → Privacy & "
            + "Security → Screen & System Audio Recording and enable NeevVoice, "
            + "then try again."
        a.alertStyle = .informational
        a.addButton(withTitle: "OK")
        a.runModal()
    }

    @objc private func toggleRec() {
        // Same rule as the microphone: ask, then render what the worker
        // confirms, so the menu cannot claim to be recording when it is not.
        send(recOn ? "rec-off" : "rec-on")
    }

    @objc private func endSession() {
        send("end-session")
    }
}

// --socket <path> is required: without it there is no worker to talk to.
var socketPath: String?
var args = CommandLine.arguments.dropFirst().makeIterator()
while let a = args.next() {
    if a == "--socket" { socketPath = args.next() }
}
guard let path = socketPath else {
    FileHandle.standardError.write(Data("NeevVoice: --socket <path> is required\n".utf8))
    exit(2)
}

let app = NSApplication.shared
// Accessory: menu-bar only. No Dock icon, no window — this is a status
// indicator, not an app the host has to manage.
app.setActivationPolicy(.accessory)
let delegate = VoiceBar(socketPath: path)
app.delegate = delegate
app.run()
