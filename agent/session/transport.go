package session

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/rs/zerolog/log"

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
	worker   net.Conn // current capture worker (nil if none attached)

	bridge    *secureBridge // helper secure-desktop pipe (UAC/lock/login)
	secureWas atomic.Bool   // last worker-frame saw secure active (for keyframe on revert)
}

type peerSession struct {
	peer *network.Peer
	pktz rtp.Packetizer
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

	t := &Transport{relayURL: relayURL, peers: make(map[string]*peerSession)}
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
		go t.handleWorker(ctx, conn)
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

	t.sigClient.On(network.MsgRegistered, func(network.Message) {
		log.Info().Str("id", t.sigClient.AgentID).Msg("transport registered")
		t.writeCreds()
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
	t.sigClient.On(network.MsgBye, func(m network.Message) { t.dropPeer(m.From) })
	return nil
}

func (t *Transport) onConnect(ctx context.Context, m network.Message) {
	log.Info().Str("from", m.From).Msg("transport: incoming connect")
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

	ps := &peerSession{peer: peer, pktz: pktz}
	t.mu.Lock()
	t.peers[m.From] = ps
	t.mu.Unlock()

	peer.OnConnected = func() {
		log.Info().Str("controller", m.From).Msg("transport: viewer connected")
		// Fresh keyframe for the new viewer from whichever source is live.
		t.requestKeyframe()
		if t.bridge != nil {
			t.bridge.requestKeyframe()
		}
	}

	// Viewer input (mouse/keyboard) arrives on the control + cursor channels.
	// Route it to whoever owns the CURRENT input path: the SYSTEM helper while a
	// secure or elevated desktop is up (only it can inject there), otherwise the
	// per-session worker. Exactly one owner at a time — no contention.
	peer.OnData = func(label string, data []byte, isString bool) {
		switch label {
		case "control", "cursor":
			if t.bridge != nil && (t.bridge.SecureActive() || t.bridge.ElevatedActive()) {
				t.bridge.SendInput(data)
			} else {
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

// sendInputToWorker forwards a raw viewer input event to the current capture
// worker over IPC. Dropped silently if no worker is attached (e.g. mid-swap).
func (t *Transport) sendInputToWorker(raw []byte) {
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn != nil {
		_ = ipc.WriteMessage(conn, ipc.KindInput, raw)
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
}

// handleWorker drains one capture worker's frame stream and distributes frames
// to all connected viewers. A new worker (after a session switch) simply
// replaces the old one; the peers/tracks are untouched.
func (t *Transport) handleWorker(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	t.workerMu.Lock()
	t.worker = conn
	t.workerMu.Unlock()
	log.Info().Msg("transport: capture worker attached")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		kind, payload, err := ipc.ReadMessage(conn)
		if err != nil {
			log.Info().Err(err).Msg("transport: capture worker detached")
			t.workerMu.Lock()
			if t.worker == conn {
				t.worker = nil
			}
			t.workerMu.Unlock()
			return
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
		t.distributeFrame(vp8)
	}
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

// writeCreds records the transport's id + password so a headless (session 0)
// transport is reachable during testing. Written to ProgramData\NeevRemote.
func (t *Transport) writeCreds() {
	dir := os.Getenv("ProgramData")
	if dir == "" {
		dir = os.TempDir()
	}
	dir = dir + string(os.PathSeparator) + "NeevRemote"
	_ = os.MkdirAll(dir, 0o755)
	path := dir + string(os.PathSeparator) + "transport.txt"
	content := "id=" + t.sigClient.AgentID + "\npassword=" + t.password + "\n"
	_ = os.WriteFile(path, []byte(content), 0o600)
	log.Info().Str("path", path).Msg("transport creds written")
}

// loadMachineCreds reads the SYSTEM helper's machine-wide id + password from
// C:\ProgramData\NeevRemote\machine.dat (line 1 = id, line 2 = password; the
// password may be empty until the user sets one). Returns ("","") if absent —
// the caller then falls back to env/random. Windows-only in practice; on other
// OSes ProgramData is unset so this returns empty.
func loadMachineCreds() (id, password string) {
	dir := os.Getenv("ProgramData")
	if dir == "" {
		return "", ""
	}
	data, err := os.ReadFile(dir + string(os.PathSeparator) + "NeevRemote" +
		string(os.PathSeparator) + "machine.dat")
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

// requestKeyframe asks the current capture worker for a keyframe.
func (t *Transport) requestKeyframe() {
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn != nil {
		_ = ipc.WriteMessage(conn, ipc.KindKeyframeReq, nil)
	}
}
