package network

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// The agent must receive the viewer's microphone.
//
// pc.OnTrack used to be registered only for RoleController, on the assumption
// that only the viewer receives media. Voice broke that assumption: with the
// handler gated behind the role, pion never delivered an incoming track to the
// agent, the transport's OnTrack callback was dead code, and viewer→host audio
// could not work whatever the SDP said. It presented as host→viewer working,
// the reverse silent, and a healthy sendrecv channel reported at both ends —
// which is why it survived two releases of "fixes" aimed at the wrong layer.
func TestAgentPeerReceivesAnIncomingAudioTrack(t *testing.T) {
	agent, err := NewPeer(nil, RoleAgent, nil, "test-viewer")
	if err != nil {
		t.Fatalf("create agent peer: %v", err)
	}
	defer agent.pc.Close()

	got := make(chan string, 4)
	agent.OnTrack = func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		got <- track.Kind().String()
	}

	// Stand in for the viewer: send one PCMU audio track.
	viewer, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("create viewer peer: %v", err)
	}
	defer viewer.Close()

	mic, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1},
		"viewer-voice", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.AddTrack(mic); err != nil {
		t.Fatal(err)
	}

	// The agent already carries a sendrecv audio transceiver from NewPeer, so
	// the viewer's track has a section to land in.
	offer, err := agent.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	agentGathering := webrtc.GatheringCompletePromise(agent.pc)
	if err := agent.pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-agentGathering
	if err := viewer.SetRemoteDescription(*agent.pc.LocalDescription()); err != nil {
		t.Fatal(err)
	}

	answer, err := viewer.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err)
	}
	viewerGathering := webrtc.GatheringCompletePromise(viewer)
	if err := viewer.SetLocalDescription(answer); err != nil {
		t.Fatal(err)
	}
	<-viewerGathering
	if err := agent.pc.SetRemoteDescription(*viewer.LocalDescription()); err != nil {
		t.Fatal(err)
	}

	// Feed audio until the agent reports the track (or we give up).
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		silence := make([]byte, 160)
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				_ = mic.WriteSample(media.Sample{
					Data:     silence,
					Duration: 20 * time.Millisecond,
				})
			}
		}
	}()

	select {
	case kind := <-got:
		if kind != "audio" {
			t.Fatalf("agent received a %q track, want audio", kind)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the agent never received the viewer's audio track — " +
			"viewer→host voice is silent")
	}
}
