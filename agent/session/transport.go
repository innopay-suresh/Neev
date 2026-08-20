package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/rs/zerolog/log"

	"github.com/pion/webrtc/v3"

	"github.com/neev/remote-agent/agent/audio"
	"github.com/neev/remote-agent/agent/auth"
	"github.com/neev/remote-agent/agent/ipc"
	"github.com/neev/remote-agent/agent/network"
)

// vp8ClockRate is the RTP clock for VP8; samplesPerFrame drives the per-frame
// timestamp increment at the nominal capture FPS.
const (
	vp8ClockRate    = 90000
	samplesPerFrame = vp8ClockRate / workerFPS
)

// Transport is the persistent process that owns the WebRTC connection(s). It
// stays alive across capture-worker restarts (i.e. across user switches): worker
// frames flow in over IPC and are packetized onto each viewer's video track with
// a continuous RTP sequence, so a worker swap causes at most a brief freeze —
// never a disconnect.
type Transport struct {
	relayURL   string
	password   string
	sigClient  *network.Client
	iceServers []network.ICEServer

	mu    sync.Mutex
	peers map[string]*peerSession // by controller id

	workerMu sync.Mutex
	worker   *ipc.Conn // current capture worker (nil if none attached); writes are serialized

	// Consent gate (TransportMode "Ask before allowing connections"): pending
	// approvals keyed by viewer id, each answered by the worker's Accept/Deny.
	consentMu      sync.Mutex
	consentWaiters map[string]chan consentAnswer

	// Per-viewer CONTROL permission, decided by the HOST (consent choice, or the
	// host's view-only default when no prompt is shown). The host is the only
	// authority here: view-only used to be enforced solely on the viewer, which
	// made it an honour system — a viewer that ignored the flag, or an older
	// build, had full control no matter what the host wanted.
	controlMu      sync.Mutex
	controlAllowed map[string]bool
	// Full permission profile per viewer, so clipboard and file transfer are
	// governed by the same grant as control rather than being always-on.
	peerProfile map[string]AccessProfile

	bridge    *secureBridge // helper secure-desktop pipe (UAC/lock/login)
	secureWas atomic.Bool   // last worker-frame saw secure active (for keyframe on revert)

	// Throttle keyframe requests (unix nanos of the last one sent). A viewer under
	// video contention (e.g. during a big file transfer) floods PLI/FIR, which the
	// transport would otherwise turn into a flood of KindKeyframeReq on the hi IPC
	// lane — starving the bulk (file-transfer) lane so large uploads wedge. The
	// worker collapses them to a single wantKeyframe flag anyway, so ≤1 per 200ms
	// loses nothing and keeps the hi lane free for file data to flow.
	lastKeyframe atomic.Int64

	// Readiness heartbeat (unix nanos of the last time we refreshed the ready
	// file). Written to <dataDir>/transport.ready only while a worker is actually
	// producing frames — i.e. Screen Recording is granted. The macOS Flutter app
	// checks this file's freshness to decide the daemon is genuinely hosting
	// before it defers its own hosting; without it an installed-but-permission-
	// denied daemon strands the app "Offline" with no video anywhere.
	lastReadyNs atomic.Int64

	// Input-path observability (post-July-9 split diagnostics). Viewer input now
	// travels transport → worker over IPC instead of being injected in-process;
	// these sampled counters make a dead-input session visible in transport.log:
	// where each event was routed (worker vs secure/elevated bridge) and how many
	// were dropped for lack of an attached worker.
	inToWorker atomic.Uint64
	inToBridge atomic.Uint64
	inDropped  atomic.Uint64
	// Input refused because the HOST granted view-only (distinct from inDropped,
	// which counts input lost for lack of a worker).
	inViewOnlyDropped atomic.Uint64
}

type peerSession struct {
	peer *network.Peer
	pktz rtp.Packetizer
	// apktz packetizes voice. Separate from pktz because audio and video carry
	// their own sequence numbers and clock — sharing one would make every audio
	// packet look to the viewer like a gap in the video stream.
	apktz rtp.Packetizer
}

// RunTransport starts the persistent transport: registers with the relay, then
// serves worker frames to connected viewers until ctx is cancelled.
func RunTransport(ctx context.Context, port int) error {
	setupFileLog("transport.log")
	if port == 0 {
		port = ipc.DefaultPort
	}
	relayURL := os.Getenv("RELAY_URL")
	if relayURL == "" {
		relayURL = "ws://127.0.0.1:8080/ws"
	}

	t := &Transport{
		relayURL:       relayURL,
		peers:          make(map[string]*peerSession),
		consentWaiters: make(map[string]chan consentAnswer),
		controlAllowed: make(map[string]bool),
		peerProfile:    make(map[string]AccessProfile),
	}
	if err := t.setupSignaling(ctx); err != nil {
		return err
	}

	// Bridge the SYSTEM helper's secure desktop (UAC / lock / another user's
	// login screen) onto the SAME live track: while secure, the helper's frames
	// are used instead of the worker's, so a user-profile switch shows and
	// accepts the login password with no disconnect. Only distribute while
	// secure (a straggler frame after 'G' is ignored).
	t.bridge = newSecureBridge(func(vp8 []byte, keyframe bool) {
		if t.bridge != nil && t.bridge.SecureActive() {
			t.distributeFrame(vp8)
		}
	})

	// Accept the (single active) capture worker and pump its frames to peers.
	ln, err := ipc.Listen(port)
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() { <-ctx.Done(); ln.Close() }()
	log.Info().Int("port", port).Msg("transport listening for capture worker")

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				continue
			}
		}
		go t.handleWorker(ctx, ipc.NewConn(conn))
	}
}

func (t *Transport) setupSignaling(ctx context.Context) error {
	// Identity + credential: prefer the machine-wide id + password minted by the
	// SYSTEM helper (machine.dat), so a viewer reaches the transport with the
	// SAME id+password it uses for the normal Flutter host — nothing new to
	// discover, and a switch into TransportMode is transparent. Fall back to a
	// fixed unattended password (env) or a fresh random one for non-helper/PoC
	// runs. id/password are also written to transport.txt on register.
	machineID, machinePw := loadMachineCreds()
	password := machinePw
	if password == "" {
		password = os.Getenv("UNATTENDED_PASSWORD")
	}
	if password == "" {
		if p, err := auth.GenerateRandomPassword(); err == nil {
			password = p
		}
	}
	t.password = password
	var passwordHash string
	if h, err := auth.HashPassword(password); err == nil {
		passwordHash = h
	}

	ice, err := network.FetchICEServers(ctx, t.relayURL)
	if err != nil || len(ice) == 0 {
		ice = []network.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
	}
	t.iceServers = ice

	t.sigClient = network.NewClient(t.relayURL, passwordHash, passwordHash,
		"transport", os.Getenv("ORG_ID"), os.Getenv("DEVICE_GROUP"),
		os.Getenv("ENROLLMENT_CODE"))
	// Register under the machine-wide id (the relay honors a requested id), so
	// the viewer's saved machine-id keeps working across the switch.
	if machineID != "" {
		t.sigClient.AgentID = machineID
	}

	go func() {
		if err := t.sigClient.Connect(ctx); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("transport: signaling failed")
		}
	}()

	go t.runAliveHeartbeat(ctx)

	t.sigClient.On(network.MsgRegistered, func(network.Message) {
		log.Info().Str("id", t.sigClient.AgentID).Msg("transport registered")
		t.writeCreds()
		t.announceHostCreds()
		// Freeze the password (and id) so it is STABLE across restarts. The
		// installer writes only the id to machine.dat, so line 2 is empty and the
		// fallback above mints a FRESH random password on every boot — a viewer's
		// saved password then silently goes stale and the relay rejects it as
		// "invalid password" (no useful error, just a hung "Connecting…"). Persist
		// only when machine.dat had no password, so a user-set password is never
		// overwritten.
		if machinePw == "" {
			persistMachineCreds(t.sigClient.AgentID, password)
		}
	})
	t.sigClient.On(network.MsgConnect, func(m network.Message) { t.onConnect(ctx, m) })
	// The transport is the OFFERER (like the Flutter host), so the viewer sends
	// an ANSWER, not an offer.
	t.sigClient.On(network.MsgAnswer, func(m network.Message) {
		if p := t.getPeer(m.From); p != nil {
			if err := p.peer.HandleAnswer(m.Payload); err != nil {
				log.Error().Err(err).Msg("transport: handle answer")
			}
		}
	})
	t.sigClient.On(network.MsgCandidate, func(m network.Message) {
		if p := t.getPeer(m.From); p != nil {
			_ = p.peer.HandleCandidate(m.Payload)
		}
	})
	t.sigClient.On(network.MsgBye, func(m network.Message) {
		// If a consent prompt is still up for this viewer, withdraw it now
		// instead of leaving the host to answer a question about someone who
		// has already left.
		t.consentMu.Lock()
		_, pending := t.consentWaiters[m.From]
		t.consentMu.Unlock()
		if pending {
			t.deliverConsent(m.From, false, false)
			t.cancelConsent(m.From)
			log.Info().Str("from", m.From).
				Msg("transport: viewer left while its consent prompt was open — withdrawn")
		}
		t.dropPeer(m.From)
	})
	return nil
}

func (t *Transport) onConnect(ctx context.Context, m network.Message) {
	log.Info().Str("from", m.From).Msg("transport: incoming connect")

	// How did this controller authenticate? The relay checks the session password
	// and the unattended password and now tells us which one matched. That
	// distinction is the whole basis of unattended access: an unattended login is
	// meant to proceed with nobody present, while a session-password login is an
	// interactive request a human should be able to refuse.
	authMode := connectAuthMode(m.Payload)
	unattended := authMode == "unattended"

	// Permissions come from the profile for THIS mode, so an unattended session
	// can be granted less (or more) than someone sitting at the keyboard.
	prof := hostProfile(unattended)
	control := prof.Control

	switch {
	case unattended:
		// The unattended password IS the authorisation. Prompting here would
		// defeat the feature: there may be nobody to answer.
		log.Info().Str("from", m.From).Bool("control", control).
			Msg("transport: unattended login — no prompt")
	case !t.interactiveAllowed():
		// Interactive access disabled: the unattended password is the only way
		// in. Refuse rather than prompt.
		log.Info().Str("from", m.From).
			Msg("transport: connection DENIED — interactive access is disabled")
		t.refuse(m.From, reasonInteractiveOff)
		return
	case t.consentRequired():
		// Interactive request → ask the logged-in user. The session-0 transport
		// can't draw UI, so it asks the per-session worker and waits.
		// Deny/timeout/no-user-session → refuse, matching the setting's intent
		// (nobody there to accept = not allowed).
		allow, granted := t.askConsent(ctx, m.From)
		if !allow {
			log.Info().Str("from", m.From).Msg("transport: connection DENIED (consent)")
			t.refuse(m.From, reasonConsentDenied)
			return
		}
		control = granted
		log.Info().Str("from", m.From).Bool("control", control).
			Msg("transport: connection approved (consent)")
	default:
		log.Info().Str("from", m.From).Bool("control", control).
			Msg("transport: interactive login auto-accepted (prompt disabled)")
	}
	defer t.announceSessionState()
	prof.Control = control
	t.controlMu.Lock()
	t.controlAllowed[m.From] = control
	t.peerProfile[m.From] = prof
	t.controlMu.Unlock()
	if !control {
		log.Info().Str("from", m.From).
			Msg("transport: viewer is VIEW-ONLY — input will be dropped host-side")
	}

	t.dropPeer(m.From) // replace any stale session

	peer, err := network.NewPeer(t.iceServers, network.RoleAgent, t.sigClient, m.From)
	if err != nil {
		log.Error().Err(err).Msg("transport: create peer")
		return
	}

	// One packetizer per peer track → continuous RTP sequence/timestamp across
	// capture-worker swaps (the whole point: the viewer never sees a disconnect).
	pktz := rtp.NewPacketizer(1200, 96, 0x1234ABCD,
		&codecs.VP8Payloader{}, rtp.NewRandomSequencer(), vp8ClockRate)
	// Opus: payload type 111 and a 48 kHz clock are the WebRTC convention.
	apktz := rtp.NewPacketizer(1200, 111, 0x1234ABCE,
		&codecs.OpusPayloader{}, rtp.NewRandomSequencer(), audio.OpusRate)

	ps := &peerSession{peer: peer, pktz: pktz, apktz: apktz}
	t.mu.Lock()
	t.peers[m.From] = ps
	t.mu.Unlock()

	// Viewer voice → host speakers. The RTP payload of a PCMU packet IS the
	// mu-law bytes, so no decode happens here; the transport just unwraps and
	// hands them down to the worker, which owns the audio device.
	peer.OnTrack = func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		log.Info().Str("codec", track.Codec().MimeType).
			Msg("transport: viewer opened a voice track")
		go t.pumpViewerVoice(ctx, track)
	}

	peer.OnConnected = func() {
		log.Info().Str("controller", m.From).Msg("transport: viewer connected")
		// Fresh keyframe for the new viewer from whichever source is live.
		t.requestKeyframe()
		if t.bridge != nil {
			t.bridge.requestKeyframe()
		}
		// Announce the host OS (like the Flutter host does) so the viewer enables
		// OS-specific controls — WITHOUT it `remoteHostOs` stays empty and the
		// viewer hides the Windows-only Privacy/Login buttons and skips ⌘↔Ctrl
		// mapping. The control DC may not be open at the instant OnConnected fires,
		// so retry briefly until it lands (idempotent on the viewer).
		go t.announceHostOS(peer)
		go t.announceGrant(peer, control)
	}

	// Viewer input (mouse/keyboard) arrives on the control + cursor channels.
	// Route it to whoever owns the CURRENT input path: the SYSTEM helper while a
	// secure or elevated desktop is up (only it can inject there), otherwise the
	// per-session worker. Exactly one owner at a time — no contention.
	viewerID := m.From
	peer.OnData = func(label string, data []byte, isString bool) {
		switch label {
		case "file":
			// Viewer→host file transfer rides its own reliable channel; hand it to
			// the worker (user session) to write into Downloads.
			t.sendToWorker(ipc.KindFileData, data)
			return
		case "control", "cursor":
			// Ctrl+Alt+Del must fire from a SYSTEM process (session 0) — the worker
			// (user session) can't, which is why it was a no-op. Intercept it here in
			// the transport and call SendSAS directly. Other commands still go to the
			// worker below.
			if label == "control" && isSASCommand(data) {
				if !t.viewerMayControl(viewerID) {
					log.Info().Str("from", viewerID).
						Msg("transport: Ctrl+Alt+Del DROPPED — host granted view-only")
					return
				}
				triggerSAS()
				return
			}
			// Only real mouse/keyboard INPUT ever needs the secure/elevated bridge
			// (only SYSTEM can inject into Winlogon/elevated windows). Clipboard,
			// chat, file and command messages are handled by the worker regardless
			// of which desktop is up, so they must NOT go to the bridge (it would
			// drop them) — this machine is elevated/secure often, which is why
			// chat + image clipboard were failing.
			// HOST-SIDE VIEW-ONLY ENFORCEMENT. The viewer-side gate is a courtesy
			// (it just stops sending); this is the one that actually holds, because
			// the host decides. Only real control attempts are dropped — clipboard,
			// chat, file transfer and view-related commands still work, so
			// view-only does not degrade into a half-dead session.
			if isClipboardMsg(data) && !t.viewerProfile(viewerID).Clipboard {
				return // clipboard not granted to this session
			}
			if isControlAttempt(data) && !t.viewerMayControl(viewerID) {
				if n := t.inViewOnlyDropped.Add(1); n == 1 || n%256 == 0 {
					log.Info().Uint64("n", n).Str("from", viewerID).
						Msg("transport: input DROPPED — host granted view-only")
				}
				return
			}
			bridgeUp := t.bridge != nil && (t.bridge.SecureActive() || t.bridge.ElevatedActive())
			if bridgeUp && label == "control" && workerOnlyMessage(data) {
				bridgeUp = false
			}
			if bridgeUp {
				if n := t.inToBridge.Add(1); n == 1 || n%256 == 0 {
					log.Info().Uint64("n", n).
						Bool("secure", t.bridge.SecureActive()).
						Bool("elevated", t.bridge.ElevatedActive()).
						Msg("transport: input → secure/elevated bridge")
				}
				t.bridge.SendInput(data)
			} else {
				if n := t.inToWorker.Add(1); n == 1 || n%256 == 0 {
					t.workerMu.Lock()
					has := t.worker != nil
					t.workerMu.Unlock()
					log.Info().Uint64("n", n).Bool("worker", has).
						Str("label", label).Msg("transport: input → capture worker")
				}
				t.sendInputToWorker(data)
			}
		}
	}

	// Send the offer now (viewer answers). Candidates trickle via OnICECandidate.
	if err := peer.CreateAgentOffer(ctx); err != nil {
		log.Error().Err(err).Msg("transport: create offer")
		t.dropPeer(m.From)
		return
	}

	// Forward viewer PLI/FIR (keyframe requests) to the capture worker.
	go t.watchRTCP(ctx, peer)
}

// broadcastClip relays a host clipboard text change to every viewer on the
// control channel as {"k":"clip","t":...} (as text — the viewer ignores binary
// there), matching the Flutter host's clipboard message so no viewer change is
// needed.
func (t *Transport) broadcastClip(text string) {
	msg, err := json.Marshal(map[string]string{"k": "clip", "t": text})
	if err != nil {
		return
	}
	t.mu.Lock()
	sessions := make([]*peerSession, 0, len(t.peers))
	for _, ps := range t.peers {
		sessions = append(sessions, ps)
	}
	t.mu.Unlock()
	for _, ps := range sessions {
		_ = ps.peer.SendControlText(string(msg))
	}
}

// broadcastClipImage relays a host clipboard IMAGE (PNG) to every viewer as
// chunked {"k":"clip","img":1,"i","n","d"} control-channel messages — the exact
// format the Flutter viewer reassembles (48 KB base64 chunks, in order).
func (t *Transport) broadcastClipImage(png []byte) {
	b64 := base64.StdEncoding.EncodeToString(png)
	const chunk = 48 * 1024
	total := (len(b64) + chunk - 1) / chunk
	if total < 1 {
		total = 1
	}
	t.mu.Lock()
	sessions := make([]*peerSession, 0, len(t.peers))
	for _, ps := range t.peers {
		sessions = append(sessions, ps)
	}
	t.mu.Unlock()
	for i := 0; i < total; i++ {
		start := i * chunk
		end := start + chunk
		if end > len(b64) {
			end = len(b64)
		}
		msg, err := json.Marshal(map[string]interface{}{
			"k": "clip", "img": 1, "i": i, "n": total, "d": b64[start:end],
		})
		if err != nil {
			return
		}
		for _, ps := range sessions {
			_ = ps.peer.SendControlText(string(msg))
		}
	}
}

// workerOnlyMessage reports whether a control-channel payload is a clipboard /
// chat / file / command message (handled by the worker in the user session) as
// opposed to real mouse/keyboard input (which may need the secure bridge). Such
// messages must always reach the worker, even while a secure/elevated desktop is
// up. Unparseable or input payloads return false (default to the input path).
func workerOnlyMessage(data []byte) bool {
	var m struct {
		K string `json:"k"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	switch m.K {
	case "clip", "chat", "ft", "cmd":
		return true
	}
	return false
}

// isControlAttempt reports whether a control-channel payload would actually
// CONTROL the host, as opposed to merely observing or exchanging data with it.
//
// Deliberately a denylist, not an allowlist: the four input kinds are defined in
// exactly one place (input_event.dart) and are stable, whereas new non-control
// message kinds get added regularly. An allowlist would silently break each new
// feature for view-only sessions.
//
// Allowed while view-only: clipboard, chat, file transfer, monitor switch,
// stream quality, keyframe requests — everything that lets a watcher be useful
// without touching the machine. Malformed payloads pass: they cannot be decoded
// as input downstream either, so they inject nothing.
func isControlAttempt(data []byte) bool {
	var m struct {
		K string `json:"k"`
		C string `json:"c"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	switch m.K {
	case "mv", "btn", "whl", "key":
		return true // mouse move / button / wheel / keystroke
	case "cmd":
		// Commands that change the machine's state. A watcher must not be able to
		// lock, log out, reboot, blank the screen, or raise the secure desktop.
		switch m.C {
		case "lock", "logoff", "reboot", "privacy", "sas":
			return true
		}
	}
	return false
}

// viewerMayControl reports whether the HOST granted this viewer control.
// Unknown viewers default to ALLOWED: the old host-side gate was removed
// precisely because defaulting to deny silently killed all input whenever the
// flag was unset, and that footgun must not come back. Every path that admits a
// viewer records an explicit decision, so "unknown" only happens if that
// bookkeeping is ever missed.
func (t *Transport) viewerMayControl(id string) bool {
	t.controlMu.Lock()
	defer t.controlMu.Unlock()
	allowed, ok := t.controlAllowed[id]
	return !ok || allowed
}

// viewerProfile returns the permission profile granted to a viewer. An unknown
// viewer gets full access, matching viewerMayControl: defaulting to deny would
// resurrect the footgun where an unset flag silently killed working features.
func (t *Transport) viewerProfile(id string) AccessProfile {
	t.controlMu.Lock()
	defer t.controlMu.Unlock()
	p, ok := t.peerProfile[id]
	if !ok {
		return AccessProfile{Control: true, Clipboard: true, Files: true}
	}
	return p
}

// isClipboardMsg reports whether a control-channel payload is clipboard traffic
// (text, or the announce/request/deliver messages for clipboard FILES).
func isClipboardMsg(data []byte) bool {
	var m struct {
		K string `json:"k"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	switch m.K {
	case "clip", "clipfann", "clipfreq", "clipfdat":
		return true
	}
	return false
}

// isSASCommand reports whether a payload is the viewer's Ctrl+Alt+Del request
// ({"k":"cmd","c":"sas"}) — handled by the transport (SYSTEM) via SendSAS.
func isSASCommand(data []byte) bool {
	var m struct {
		K string `json:"k"`
		C string `json:"c"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	return m.K == "cmd" && m.C == "sas"
}

// announceGrant tells the viewer what access the HOST granted it.
//
// Without this the viewer had no idea it was view-only: the host silently
// dropped its input while the viewer's own toolbar still said "Control", so a
// correctly-enforced restriction looked exactly like a broken app — clicks that
// did nothing, with no explanation anywhere.
//
// Retried like the OS announce: the control channel can open a beat after the
// peer connects, and a grant the viewer never receives is the same as no grant.
func (t *Transport) announceGrant(peer *network.Peer, control bool) {
	msg := `{"k":"grant","control":true}`
	if !control {
		msg = `{"k":"grant","control":false}`
	}
	for i := 0; i < 15; i++ {
		if err := peer.SendControlText(msg); err == nil {
			log.Info().Bool("control", control).Msg("transport: announced grant to viewer")
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Warn().Msg("transport: grant announce never landed (control DC not open)")
}

// announceHostOS tells the viewer this host is Windows so it un-hides the
// Windows-only Privacy/Login buttons and applies ⌘↔Ctrl mapping. The control DC
// can open a beat after the peer connects, so retry until a send succeeds. pion
// returns an error while the channel isn't open, so err==nil means it landed;
// sends are idempotent on the viewer regardless.
func (t *Transport) announceHostOS(peer *network.Peer) {
	// The machine's own name, so the viewer can label the session with
	// something a person recognises instead of a nine-digit id. Sent with the
	// OS announce because it rides the same channel and has the same "retry
	// until the data channel is actually open" problem.
	name, _ := os.Hostname()
	nameJSON, _ := json.Marshal(name)

	for i := 0; i < 15; i++ {
		// runtime.GOOS, not the literal "windows" this used to send.
		//
		// Every host announced itself as Windows, including macOS ones. The
		// viewer keys real behaviour off this value, so a Mac host produced:
		// no Ctrl->Command translation (viewer and host both looked
		// non-mac, so no swap happened and every shortcut reached macOS as
		// Control — copy, paste, save and quit all silently did nothing), the
		// Windows-only Login/UAC button offered for a machine that has no such
		// prompt, and any macOS-specific branch skipped. Reported as two
		// separate bugs, "Ctrl+C does not work" and "the sound/mic/record dock
		// is missing when I connect to a Mac"; both were this string.
		if err := peer.SendControlText(`{"k":"os","v":"` + viewerOSName() + `"}`); err == nil {
			log.Info().Msg("transport: announced host OS to viewer")
			if name != "" {
				_ = peer.SendControlText(`{"k":"hostname","v":` + string(nameJSON) + `}`)
				log.Info().Str("hostname", name).Msg("transport: announced hostname to viewer")
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Warn().Msg("transport: host-OS announce never landed (control DC not open)")
}

// viewerOSName is the host OS in the VIEWER's vocabulary, which is not
// runtime.GOOS.
//
// The viewer compares against "macos"; Go calls it "darwin". Sending GOOS
// straight through swapped one wrong value for another — the viewer would not
// recognise the host as a Mac and would still skip the Ctrl->Command
// translation, so this would have looked exactly like the bug it was meant to
// fix. The Flutter host has always sent "macos" (see _osName), and that is the
// contract both ends already share.
func viewerOSName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS // "windows", "linux" already match
	}
}

// sendInputToWorker forwards a raw viewer input event to the current capture
// worker over IPC. Dropped silently if no worker is attached (e.g. mid-swap).
func (t *Transport) sendInputToWorker(raw []byte) {
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn == nil {
		// No worker attached (mid-swap, or the per-session worker never spawned):
		// input is silently lost. Sample-log so this shows up as the cause of a
		// "video works, clicks dead" session instead of failing invisibly.
		if n := t.inDropped.Add(1); n <= 5 || n%256 == 0 {
			log.Warn().Uint64("dropped", n).
				Msg("transport: viewer input dropped — no capture worker attached")
		}
		return
	}
	if err := conn.WriteMessage(ipc.KindInput, raw); err != nil {
		log.Warn().Err(err).Msg("transport: forward input to worker failed")
	}
}

// sendToWorker forwards a raw viewer message of the given IPC kind to the current
// capture worker (file-transfer / clipboard-file data). Goes on the BULK lane so
// a large transfer paces itself (backpressure) and never blocks input on the hi
// lane. Dropped silently if no worker.
func (t *Transport) sendToWorker(kind byte, raw []byte) {
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn == nil {
		return
	}
	if err := conn.WriteBulk(kind, raw); err != nil {
		log.Warn().Err(err).Uint8("kind", kind).Msg("transport: forward to worker failed")
	}
}

// watchRTCP reads RTCP from the peer's video sender and asks the worker for a
// keyframe on PLI/FIR.
func (t *Transport) watchRTCP(ctx context.Context, peer *network.Peer) {
	pc := peer.PeerConnection()
	if pc == nil {
		return
	}
	for _, sender := range pc.GetSenders() {
		s := sender
		go func() {
			buf := make([]byte, 1500)
			for {
				n, _, err := s.Read(buf)
				if err != nil {
					return
				}
				pkts, err := rtcp.Unmarshal(buf[:n])
				if err != nil {
					continue
				}
				for _, pkt := range pkts {
					switch pkt.(type) {
					case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
						t.requestKeyframe()
					}
				}
			}
		}()
	}
}

func (t *Transport) getPeer(id string) *peerSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.peers[id]
}

// announceSessionState tells the worker how many viewers are connected so it
// can show or hide the host's "Remote session active / Disconnect" bar. Without
// this the host has no indication a session is live and no way to end it.
func (t *Transport) announceSessionState() {
	t.mu.Lock()
	n := len(t.peers)
	t.mu.Unlock()
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn == nil {
		return
	}
	_ = conn.WriteMessage(ipc.KindSessionState, []byte(strconv.Itoa(n)))
}

// announceHostCreds gives the worker this machine's id + password so the app
// can display them.
//
// The app has no other way to learn them. transport.txt is root-owned 0600
// because it carries the password, and the app runs as the logged-in user — so
// the macOS share card showed an empty id while the daemon was hosting
// perfectly well, leaving the host with nothing to hand out. Sent to the worker
// (which runs AS that user) rather than relaxing the file, so the credentials
// reach the person at the keyboard and no other local account.
//
// Sent on register and again whenever a worker attaches, because a worker
// spawned by a user switch has missed the register that came before it.
func (t *Transport) announceHostCreds() {
	id := t.sigClient.AgentID
	if id == "" {
		return // not registered yet; the register hook will send it
	}
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn == nil {
		return // no worker yet; handleWorker sends it on attach
	}
	body, err := json.Marshal(struct {
		ID       string `json:"id"`
		Password string `json:"password"`
	}{id, t.password})
	if err != nil {
		return
	}
	_ = conn.WriteMessage(ipc.KindHostCreds, body)
}

func (t *Transport) dropPeer(id string) {
	t.mu.Lock()
	ps, ok := t.peers[id]
	if ok {
		delete(t.peers, id)
	}
	t.mu.Unlock()
	if ok && ps.peer != nil {
		ps.peer.Close()
	}
	if ok {
		t.announceSessionState()
	}
}

// handleWorker drains one capture worker's frame stream and distributes frames
// to all connected viewers. A new worker (after a session switch) simply
// replaces the old one; the peers/tracks are untouched.
//
// It must also be TOLD the current session state. A worker spawned after a user
// switch starts with no idea a viewer is connected: it never showed its session
// bar and never registered its audio sink, so the microphone and system-sound
// controls were missing or dead until the next viewer connect/disconnect
// happened to re-announce it.
func (t *Transport) handleWorker(ctx context.Context, conn *ipc.Conn) {
	defer conn.Close()
	t.workerMu.Lock()
	t.worker = conn
	t.workerMu.Unlock()
	log.Info().Msg("transport: capture worker attached")

	// Tell the fresh worker what is already happening. Without this a worker
	// spawned by a user switch never learns a viewer is connected, so it never
	// shows its session bar and never registers its audio sink — the mic and
	// system-sound controls end up missing or inert until some later
	// connect/disconnect happens to re-announce.
	t.announceSessionState()
	// Same reasoning for the credentials: a worker started by a user switch
	// missed the register that carried them, so the new user's app would show a
	// blank id even though the machine is registered and reachable.
	t.announceHostCreds()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		kind, payload, err := conn.ReadMessage()
		if err != nil {
			log.Info().Err(err).Msg("transport: capture worker detached")
			t.workerMu.Lock()
			if t.worker == conn {
				t.worker = nil
			}
			t.workerMu.Unlock()
			return
		}
		// Host clipboard changed → relay to viewers on the control channel. Only
		// the current worker (single-producer guard) pushes, so an overlapping or
		// backgrounded worker can't inject stale clipboard.
		if kind == ipc.KindClipboard {
			t.workerMu.Lock()
			current := t.worker == conn
			t.workerMu.Unlock()
			if current {
				t.broadcastClip(string(payload))
			}
			continue
		}
		// Consent answer (worker's Accept/Deny dialog) → unblock askConsent.
		if kind == ipc.KindEndSession {
			// The HOST user hung up. Drop the named viewer, or every viewer when
			// no id is given, and tell each one so the viewer shows a clean
			// "session ended" rather than a silent freeze.
			id := strings.TrimSpace(string(payload))
			if id == "" {
				n := t.dropAllPeers()
				log.Info().Int("viewers", n).Msg("transport: host ended the session")
			} else {
				t.sendBye(id)
				t.dropPeer(id)
				log.Info().Str("viewer", id).Msg("transport: host disconnected a viewer")
			}
			continue
		}
		if kind == ipc.KindConsentReply {
			var r struct {
				ID    string `json:"id"`
				Allow bool   `json:"allow"`
				// Whether the host granted CONTROL or view-only. Absent (older
				// worker) means control, preserving the previous behaviour.
				Control *bool `json:"control"`
			}
			if json.Unmarshal(payload, &r) == nil {
				control := true
				if r.Control != nil {
					control = *r.Control
				}
				t.deliverConsent(r.ID, r.Allow, control)
			}
			continue
		}
		// Host chat reply (typed in the worker's chat window) → relay to viewers
		// on the control channel as TEXT.
		if kind == ipc.KindChat {
			t.workerMu.Lock()
			current := t.worker == conn
			t.workerMu.Unlock()
			if current {
				t.mu.Lock()
				sessions := make([]*peerSession, 0, len(t.peers))
				for _, ps := range t.peers {
					sessions = append(sessions, ps)
				}
				t.mu.Unlock()
				for _, ps := range sessions {
					_ = ps.peer.SendControlText(string(payload))
				}
			}
			continue
		}
		// Host→viewer file export (picker result) → relay onto each viewer's
		// 'file' channel as TEXT (the viewer ignores binary there).
		if kind == ipc.KindFileData {
			t.workerMu.Lock()
			current := t.worker == conn
			t.workerMu.Unlock()
			if current {
				t.mu.Lock()
				sessions := make([]*peerSession, 0, len(t.peers))
				for _, ps := range t.peers {
					sessions = append(sessions, ps)
				}
				t.mu.Unlock()
				for _, ps := range sessions {
					_ = ps.peer.SendFileTransferText(string(payload))
				}
			}
			continue
		}
		// Host clipboard image changed → chunk + relay to viewers.
		if kind == ipc.KindClipboardImage {
			t.workerMu.Lock()
			current := t.worker == conn
			t.workerMu.Unlock()
			if current {
				t.broadcastClipImage(payload)
			}
			continue
		}
		// Host microphone → viewers. Guarded by the same single-producer rule as
		// video: a superseded worker must not keep talking over the live one.
		if kind == ipc.KindAudioFrame {
			t.workerMu.Lock()
			current := t.worker == conn
			t.workerMu.Unlock()
			if current {
				t.distributeAudio(payload)
			}
			continue
		}
		if kind != ipc.KindVideoFrame {
			continue
		}
		_, vp8, ok := ipc.DecodeVideoFrame(payload)
		if !ok || len(vp8) == 0 {
			continue
		}
		// Single-producer guard: if a newer worker has attached (during a session
		// swap, the old and new worker can briefly overlap), only the current one
		// feeds the track — otherwise two sources would interleave on one decoder
		// and corrupt the picture. A superseded worker drains but stops emitting.
		t.workerMu.Lock()
		current := t.worker == conn
		t.workerMu.Unlock()
		if !current {
			continue
		}
		// While the secure desktop is showing, the bridge owns the track — drop
		// worker frames so the two sources never interleave on one decoder.
		if t.bridge != nil && t.bridge.SecureActive() {
			t.secureWas.Store(true)
			continue
		}
		// Just reverted from secure → ask the worker for a fresh keyframe so the
		// viewer's decoder re-syncs, and drop this (likely inter) frame.
		if t.secureWas.Swap(false) {
			t.requestKeyframe()
			continue
		}
		t.markProducing()
		t.distributeFrame(vp8)
	}
}

// markAlive refreshes <dataDir>/transport.alive while the transport is
// registered, whether or not anyone is connected.
//
// Distinct from transport.ready, which is only written while FRAMES are
// flowing. The app used ready to decide whether the daemon owns hosting — but
// frames only flow once a viewer connects, so with no session it always looked
// stale, the app registered its OWN id as a second host, and a viewer reaching
// that id got an app-hosted session with no recording and no system sound. The
// decision has to be answerable before any session exists.
func (t *Transport) markAlive() {
	path := filepath.Join(dataDir(), "transport.alive")
	_ = os.WriteFile(path,
		[]byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
}

// runAliveHeartbeat keeps transport.alive fresh for as long as the transport is
// running. Stops with the context, so a stopped daemon goes stale and the app
// takes hosting back rather than leaving the machine unreachable.
func (t *Transport) runAliveHeartbeat(ctx context.Context) {
	t.markAlive()
	tk := time.NewTicker(10 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = os.Remove(filepath.Join(dataDir(), "transport.alive"))
			return
		case <-tk.C:
			t.markAlive()
		}
	}
}

// markProducing refreshes <dataDir>/transport.ready, at most every 2s. Its
// presence + freshness proves the daemon is genuinely hosting (a worker is
// attached AND producing frames, so Screen Recording is granted) — the signal
// the macOS app uses to decide whether to defer its own hosting.
func (t *Transport) markProducing() {
	now := time.Now().UnixNano()
	last := t.lastReadyNs.Load()
	if now-last < int64(2*time.Second) {
		return
	}
	if !t.lastReadyNs.CompareAndSwap(last, now) {
		return
	}
	path := filepath.Join(dataDir(), "transport.ready")
	_ = os.WriteFile(path, []byte(strconv.FormatInt(now/int64(time.Second), 10)), 0o644)
}

// distributeFrame packetizes a VP8 frame onto every connected viewer's track.
func (t *Transport) distributeFrame(vp8 []byte) {
	t.mu.Lock()
	sessions := make([]*peerSession, 0, len(t.peers))
	for _, ps := range t.peers {
		sessions = append(sessions, ps)
	}
	t.mu.Unlock()

	for _, ps := range sessions {
		if ps.peer.VideoTrack == nil {
			continue
		}
		for _, pkt := range ps.pktz.Packetize(vp8, samplesPerFrame) {
			_ = ps.peer.VideoTrack.WriteRTP(pkt)
		}
	}
}

// Refusal reasons sent to the viewer. Without one, a refused viewer sat on a
// spinner until it timed out and then blamed the network — the host's decision
// never reached the person it was about.
const (
	reasonInteractiveOff = "interactive_disabled"
	reasonConsentDenied  = "consent_denied"
)

// refuse tells the viewer its connection was turned down, and why.
//
// Sent as a bye rather than left silent so the viewer stops waiting
// immediately. The reason is a stable token, not prose: the viewer decides the
// wording, and an older viewer that does not know the token still tears the
// attempt down instead of hanging.
func (t *Transport) refuse(viewer, reason string) {
	_ = t.sigClient.Send(network.Message{
		Type:  network.MsgBye,
		To:    viewer,
		Error: reason,
	})
}

// pumpViewerVoice reads the viewer's voice track and forwards each packet to
// the capture worker for playback.
//
// Runs until the track ends, which is how a disconnect stops playback: the read
// fails, the loop exits, and nothing further is handed to the worker. There is
// deliberately no jitter buffer here — that belongs next to the output device
// in the worker, where the playback clock actually lives.
func (t *Transport) pumpViewerVoice(ctx context.Context, track *webrtc.TrackRemote) {
	// Only packets in the NEGOTIATED voice codec are audio.
	//
	// A browser/libwebrtc sender negotiates more than PCMU on this track: it
	// also carries CN (comfort noise, PT 13) and telephone-event. When the
	// viewer's microphone is muted or simply silent, it STOPS sending PCMU and
	// sends CN instead — a couple of bytes describing a noise level, not audio
	// samples. Those arrive on the same SSRC and used to be forwarded to the
	// speakers and decoded as mu-law, which is noise by construction: audible
	// with the microphone off, on both machines, unrelated to anyone speaking.
	wantPT := uint8(track.PayloadType())

	buf := make([]byte, 1500)
	first := true
	warnedPT := false
	for {
		if ctx.Err() != nil {
			return
		}
		n, _, err := track.Read(buf)
		if err != nil {
			log.Info().Err(err).Msg("transport: viewer voice track ended")
			return
		}
		var pkt rtp.Packet
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}
		if pkt.PayloadType != wantPT {
			// Never decode it, never play it. Logged once so a codec the viewer
			// starts sending in future is visible rather than silent.
			if !warnedPT {
				warnedPT = true
				log.Info().Uint8("got", pkt.PayloadType).Uint8("want", wantPT).
					Msg("transport: ignoring non-voice packets on the audio track (comfort noise / events)")
			}
			continue
		}
		// Padding bytes are not samples either.
		if pkt.Padding || len(pkt.Payload) == 0 {
			continue
		}
		// Log the FIRST packet only. Viewer→host voice failing silently is hard
		// to tell apart from a muted microphone, so one line proving audio
		// reached the host turns a guessing game into a lookup.
		if first {
			first = false
			log.Info().Str("codec", track.Codec().MimeType).Uint8("pt", wantPT).
				Msg("transport: viewer voice is arriving — forwarding to the worker for playback")
		}
		t.sendToWorker(ipc.KindAudioPlay, pkt.Payload)
	}
}

// distributeAudio packetizes one 20 ms Opus packet onto every viewer's voice
// track.
//
// Unlike video there is no keyframe concept — every audio packet stands alone,
// so a viewer that joins mid-sentence simply starts hearing from that moment.
func (t *Transport) distributeAudio(pkt []byte) {
	if len(pkt) == 0 {
		return
	}
	t.mu.Lock()
	sessions := make([]*peerSession, 0, len(t.peers))
	for _, ps := range t.peers {
		sessions = append(sessions, ps)
	}
	t.mu.Unlock()

	for _, ps := range sessions {
		if ps.peer.AudioTrack == nil || ps.apktz == nil {
			continue
		}
		// Opus is compressed, so the byte count says nothing about duration:
		// the timestamp must advance by one FRAME of samples per packet, or the
		// receiver's jitter buffer drifts against real time.
		for _, p := range ps.apktz.Packetize(pkt, uint32(audio.OpusFrameSamples)) {
			_ = ps.peer.AudioTrack.WriteRTP(p)
		}
	}
}

// writeCreds records the transport's id + password so a headless (session 0)
// transport is reachable during testing. Written to ProgramData\NeevRemote.
func (t *Transport) writeCreds() {
	path := filepath.Join(dataDir(), "transport.txt")
	content := "id=" + t.sigClient.AgentID + "\npassword=" + t.password + "\n"
	_ = os.WriteFile(path, []byte(content), 0o600)
	log.Info().Str("path", path).Msg("transport creds written")

	// The id ALONE, world-readable.
	//
	// It is not a secret — it is the thing the host reads out over the phone —
	// and keeping it locked inside the 0600 file (which has to stay 0600
	// because the password lives there) is why the share card could render
	// nothing at all. This lets the app name the machine's identity even before
	// a worker exists to relay the password.
	idPath := filepath.Join(dataDir(), "transport.id")
	_ = os.WriteFile(idPath, []byte(t.sigClient.AgentID+"\n"), 0o644)
}

// persistMachineCreds writes id+password to machine.dat so the password stays
// STABLE across transport restarts/reinstalls. Without this a machine.dat that
// carries only the id (the common case — the installer never sets a password)
// makes the transport re-randomize the password every boot, so a viewer's saved
// password goes stale and the relay silently rejects it. Called ONLY when the
// loaded machine.dat had no password line, so a user-set password is never
// clobbered. machine.dat format matches loadMachineCreds: line 1 = id, line 2 =
// password.
func persistMachineCreds(id, password string) {
	if id == "" || password == "" {
		return
	}
	path := filepath.Join(dataDir(), "machine.dat")
	if err := os.WriteFile(path, []byte(id+"\n"+password+"\n"), 0o600); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("could not persist machine creds")
		return
	}
	log.Info().Str("path", path).Msg("machine creds persisted (stable password)")
}

// loadMachineCreds reads the privileged installer's machine-wide id + password
// from <dataDir>/machine.dat (line 1 = id, line 2 = password; the password may
// be empty until the user sets one). Returns ("","") if absent — the caller then
// falls back to env/random. dataDir() resolves to ProgramData on Windows and
// /Library/Application Support/NeevRemote on macOS, so the same machine identity
// is shared between the root transport and per-session workers on both.
func loadMachineCreds() (id, password string) {
	data, err := os.ReadFile(filepath.Join(dataDir(), "machine.dat"))
	if err != nil {
		return "", ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 {
		id = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		password = strings.TrimSpace(lines[1])
	}
	return id, password
}

// consentRequired reports whether the "Ask before allowing connections" gate is
// on. The Flutter app writes <dataDir>/consent.txt ("1"/"0") when the user
// toggles the setting; absent/"0" means auto-accept (the default, unchanged).
func (t *Transport) consentRequired() bool {
	for _, p := range consentFlagPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return strings.TrimSpace(string(data)) == "1"
	}
	return false
}

// hostViewOnlyDefault reports whether this host grants VIEW-ONLY by default —
// the host user's "View only mode" setting, mirrored to viewonly.txt by the app
// exactly like consent.txt.
//
// This is the host's own wish about its own machine, so it is the default for
// every incoming viewer. The consent prompt can still override it per
// connection. Absent/"0" means full control, which preserves existing
// behaviour for every host that has never touched the setting.
func hostViewOnlyDefault() bool {
	for _, p := range hostFlagPaths("viewonly.txt") {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return strings.TrimSpace(string(data)) == "1"
	}
	return false
}

// askConsent asks the current worker to show an Accept/Deny dialog for viewer id
// and blocks (up to 30 s) for the answer. Returns false on deny, timeout, or no
// worker attached (e.g. lock screen / no interactive session — nobody to accept).
func (t *Transport) askConsent(ctx context.Context, id string) (allow bool, control bool) {
	ch := make(chan consentAnswer, 1)
	t.consentMu.Lock()
	t.consentWaiters[id] = ch
	t.consentMu.Unlock()
	answered := false
	defer func() {
		t.consentMu.Lock()
		delete(t.consentWaiters, id)
		t.consentMu.Unlock()
		// Stopped waiting without the user answering — cancelled, timed out or
		// shutting down. Take the dialog down with the request; it used to stay
		// up until dismissed by hand, asking about a viewer that had gone.
		if !answered {
			t.cancelConsent(id)
		}
	}()

	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn == nil {
		return false, false // no interactive session to ask → deny
	}
	if err := conn.WriteMessage(ipc.KindConsentRequest, []byte(id)); err != nil {
		return false, false
	}
	select {
	case a := <-ch:
		answered = true
		return a.allow, a.control
	case <-time.After(30 * time.Second):
		return false, false // no answer → deny
	case <-ctx.Done():
		return false, false
	}
}

// consentAnswer is the host user's decision: whether to admit the viewer at all,
// and whether that viewer may CONTROL the machine or only watch it.
type consentAnswer struct {
	allow   bool
	control bool
}

// sendBye tells a viewer the session is over, so it can show a clean ended
// state instead of appearing to freeze until its own timeout.
func (t *Transport) sendBye(id string) {
	if t.sigClient == nil {
		return
	}
	_ = t.sigClient.Send(network.Message{Type: network.MsgBye, To: id})
}

// dropAllPeers ends every live viewer session and reports how many were
// dropped. Each viewer is sent a bye first so it can show "the host ended the
// session" instead of appearing to freeze.
func (t *Transport) dropAllPeers() int {
	t.mu.Lock()
	ids := make([]string, 0, len(t.peers))
	for id := range t.peers {
		ids = append(ids, id)
	}
	t.mu.Unlock()
	for _, id := range ids {
		t.sendBye(id)
		t.dropPeer(id)
	}
	return len(ids)
}

// cancelConsent tells the worker to withdraw a consent prompt still on screen
// for this viewer. Safe to call when no prompt is up — the worker just finds
// nothing to close.
func (t *Transport) cancelConsent(id string) {
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn == nil {
		return
	}
	_ = conn.WriteMessage(ipc.KindConsentCancel, []byte(id))
}

// deliverConsent routes a worker's Accept/Deny answer to the waiting askConsent.
func (t *Transport) deliverConsent(id string, allow, control bool) {
	t.consentMu.Lock()
	ch := t.consentWaiters[id]
	t.consentMu.Unlock()
	if ch != nil {
		select {
		case ch <- consentAnswer{allow: allow, control: control}:
		default:
		}
	}
}

// requestKeyframe asks the current capture worker for a keyframe. Throttled to
// ≤1 per 200ms: a PLI/FIR flood (common while a big file transfer starves video
// bandwidth) must NOT flood the hi IPC lane and starve the bulk file lane. The
// worker only needs one — it re-encodes a keyframe on the next frame regardless
// of how many requests it got.
func (t *Transport) requestKeyframe() {
	now := time.Now().UnixNano()
	last := t.lastKeyframe.Load()
	if now-last < int64(200*time.Millisecond) {
		return
	}
	if !t.lastKeyframe.CompareAndSwap(last, now) {
		return // another goroutine just sent one
	}
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn != nil {
		_ = conn.WriteMessage(ipc.KindKeyframeReq, nil)
	}
}
