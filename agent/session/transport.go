package session

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/rtcp"
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
	sigClient  *network.Client
	iceServers []network.ICEServer

	mu    sync.Mutex
	peers map[string]*peerSession // by controller id

	workerMu sync.Mutex
	worker   net.Conn // current capture worker (nil if none attached)
}

type peerSession struct {
	peer *network.Peer
	pktz rtp.Packetizer
}

// RunTransport starts the persistent transport: registers with the relay, then
// serves worker frames to connected viewers until ctx is cancelled.
func RunTransport(ctx context.Context, port int) error {
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
	// Unattended password so the machine is reachable with a stable credential
	// (Phase 1 will source id+password from the SYSTEM helper's machine creds).
	unattended := os.Getenv("UNATTENDED_PASSWORD")
	var unattendedHash string
	if unattended != "" {
		if h, err := auth.HashPassword(unattended); err == nil {
			unattendedHash = h
		}
	}

	ice, err := network.FetchICEServers(ctx, t.relayURL)
	if err != nil || len(ice) == 0 {
		ice = []network.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
	}
	t.iceServers = ice

	t.sigClient = network.NewClient(t.relayURL, unattendedHash, unattendedHash,
		"transport", os.Getenv("ORG_ID"), os.Getenv("DEVICE_GROUP"),
		os.Getenv("ENROLLMENT_CODE"))

	go func() {
		if err := t.sigClient.Connect(ctx); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("transport: signaling failed")
		}
	}()

	t.sigClient.On(network.MsgRegistered, func(network.Message) {
		log.Info().Str("id", t.sigClient.AgentID).Msg("transport registered")
	})
	t.sigClient.On(network.MsgConnect, func(m network.Message) { t.onConnect(ctx, m) })
	t.sigClient.On(network.MsgOffer, func(m network.Message) {
		if p := t.getPeer(m.From); p != nil {
			if err := p.peer.HandleOffer(m.Payload); err != nil {
				log.Error().Err(err).Msg("transport: handle offer")
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
		t.requestKeyframe() // fresh keyframe for the new viewer
	}

	// Forward viewer PLI/FIR (keyframe requests) to the capture worker.
	go t.watchRTCP(ctx, peer)
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

// requestKeyframe asks the current capture worker for a keyframe.
func (t *Transport) requestKeyframe() {
	t.workerMu.Lock()
	conn := t.worker
	t.workerMu.Unlock()
	if conn != nil {
		_ = ipc.WriteMessage(conn, ipc.KindKeyframeReq, nil)
	}
}
