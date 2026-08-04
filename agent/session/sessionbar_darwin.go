package session

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// Host session controls on macOS.
//
// There is no Win32 window to draw here, and there is deliberately no attempt
// to fake one from the worker: a Go LaunchAgent has no menu bar of its own. The
// controls live in a tiny bundled app (NeevVoice.app) that the worker starts
// when a session begins and stops when it ends.
//
// The bundle is not decoration. macOS attributes microphone access to the
// process that opens the device and shows that process's usage string — a bare
// executable with no Info.plist gets a bare prompt, which users decline. A real
// app bundle carries NSMicrophoneUsageDescription, so the host is asked a
// question that explains itself.

var (
	macBarMu   sync.Mutex
	macBarCmd  *exec.Cmd
	macBarLn   net.Listener
	macOnHang  func()
	macOnTalk  func(string, bool)
	macBarLive bool
)

// voiceSockPath is per-user, in the user's own temp dir — NOT the machine-wide
// data dir, which is root-owned and not writable by the Aqua worker.
func voiceSockPath() string {
	return filepath.Join(os.TempDir(),
		fmt.Sprintf("neev-voice-%d.sock", os.Getuid()))
}

// helperAppPath locates NeevVoice.app next to the running agent binary.
func helperAppPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, cand := range []string{
		filepath.Join(dir, "NeevVoice.app", "Contents", "MacOS", "NeevVoice"),
		filepath.Join(dir, "..", "Resources", "NeevVoice.app", "Contents", "MacOS", "NeevVoice"),
	} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

func showHostSessionBar(onHangUp func()) {
	showHostSessionBarWithVoice(onHangUp, nil)
}

// showHostSessionBarWithVoice starts the menu-bar helper for this session.
func showHostSessionBarWithVoice(onHangUp func(), onTalk func(string, bool)) {
	macBarMu.Lock()
	macOnHang = onHangUp
	macOnTalk = onTalk
	if macBarLive {
		macBarMu.Unlock()
		return
	}
	macBarLive = true
	macBarMu.Unlock()

	helper := helperAppPath()
	if helper == "" {
		// No bundle shipped (older install, or a build without it). Say so once
		// rather than leaving the host wondering where the control went — and
		// leave the microphone shut, which is the safe direction to fail.
		log.Warn().Msg("worker: NeevVoice.app not found — no host session controls on this host")
		return
	}

	sock := voiceSockPath()
	_ = os.Remove(sock) // a stale socket from a crashed worker would block Listen
	ln, err := net.Listen("unix", sock)
	if err != nil {
		log.Warn().Err(err).Msg("worker: cannot open voice control socket")
		return
	}
	// The socket carries session control. Restrict it to this user so another
	// account on a shared Mac cannot toggle the host's microphone.
	_ = os.Chmod(sock, 0o600)

	macBarMu.Lock()
	macBarLn = ln
	macBarMu.Unlock()

	go acceptVoiceControl(ln)

	cmd := exec.Command(helper, "--socket", sock)
	if err := cmd.Start(); err != nil {
		log.Warn().Err(err).Msg("worker: cannot start NeevVoice.app")
		return
	}
	macBarMu.Lock()
	macBarCmd = cmd
	macBarMu.Unlock()
	log.Info().Str("helper", helper).Msg("worker: macOS session controls started")
}

// acceptVoiceControl serves the helper's commands until the listener closes.
func acceptVoiceControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serveVoiceControl(conn)
	}
}

func serveVoiceControl(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	// Audio frames are base64 lines; a 20 ms mu-law frame is ~216 chars, but
	// give the scanner room so a long line can never silently truncate a frame
	// into garbage.
	sc.Buffer(make([]byte, 0, 8192), 64*1024)
	micOn := false
	for sc.Scan() {
		line := sc.Text()
		// Host system sound, captured by the helper via ScreenCaptureKit.
		if strings.HasPrefix(line, "a ") {
			frame, err := base64.StdEncoding.DecodeString(line[2:])
			if err != nil {
				continue
			}
			feedHostSound(frame)
			continue
		}
		switch line {
		case "mic-on", "mic-off":
			on := sc.Text() == "mic-on"
			macBarMu.Lock()
			cb := macOnTalk
			macBarMu.Unlock()
			micOn = on
			if cb != nil {
				cb("mic", on)
			}
			log.Info().Bool("on", on).Msg("worker: host microphone toggled from menu bar")
			_, _ = conn.Write([]byte("mic " + strconv.FormatBool(micOn) + "\n"))
		case "rec-on", "rec-off":
			on := sc.Text() == "rec-on"
			macBarMu.Lock()
			cb := macOnTalk
			macBarMu.Unlock()
			if cb != nil {
				cb("record", on)
			}
			log.Info().Bool("on", on).Msg("worker: host toggled recording from menu bar")
			_, _ = conn.Write([]byte("rec " + strconv.FormatBool(recordingActive()) + "\n"))
		case "end-session":
			macBarMu.Lock()
			cb := macOnHang
			macBarMu.Unlock()
			if cb != nil {
				go cb()
			}
			log.Info().Msg("worker: host ended the session from the menu bar")
		}
	}
}

// hideHostSessionBar stops the helper when the last viewer leaves.
func hideHostSessionBar() {
	macBarMu.Lock()
	cmd := macBarCmd
	ln := macBarLn
	macBarCmd, macBarLn, macBarLive = nil, nil, false
	macBarMu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	_ = os.Remove(voiceSockPath())
	// The menu-bar item is gone, so the host can no longer see or change mic
	// state — the microphone must not be left open behind a control that no
	// longer exists.
	stopHostMic()
}
