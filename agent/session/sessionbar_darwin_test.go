package session

import (
	"bufio"
	"net"
	"testing"
	"time"
)

// The menu-bar helper drives the host's microphone over this socket, so the
// protocol is worth pinning: a mis-parse here would either strand the mic on or
// make the menu lie about it.

func TestVoiceControlTogglesMic(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	got := make(chan bool, 4)
	macBarMu.Lock()
	macOnTalk = func(on bool) { got <- on }
	macBarMu.Unlock()
	defer func() {
		macBarMu.Lock()
		macOnTalk = nil
		macBarMu.Unlock()
	}()

	go serveVoiceControl(b)

	r := bufio.NewReader(a)
	if _, err := a.Write([]byte("mic-on\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case on := <-got:
		if !on {
			t.Fatal("mic-on did not switch the microphone on")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker never acted on mic-on")
	}
	// The worker must ECHO the resulting state. The helper renders what it is
	// told rather than assuming its click worked, so a missing reply would
	// leave the menu showing the wrong thing.
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("no state echoed back: %v", err)
	}
	if line != "mic true\n" {
		t.Fatalf("echoed %q, want %q", line, "mic true\n")
	}

	if _, err := a.Write([]byte("mic-off\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case on := <-got:
		if on {
			t.Fatal("mic-off did not switch the microphone off")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker never acted on mic-off")
	}
	if line, _ := r.ReadString('\n'); line != "mic false\n" {
		t.Fatalf("echoed %q after mic-off, want %q", line, "mic false\n")
	}
}

func TestVoiceControlEndSession(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	ended := make(chan struct{}, 1)
	macBarMu.Lock()
	macOnHang = func() { ended <- struct{}{} }
	macBarMu.Unlock()
	defer func() {
		macBarMu.Lock()
		macOnHang = nil
		macBarMu.Unlock()
	}()

	go serveVoiceControl(b)
	if _, err := a.Write([]byte("end-session\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("host could not end the session from the menu bar")
	}
}

func TestVoiceControlIgnoresJunk(t *testing.T) {
	// The socket is user-owned but still a local endpoint; an unrecognised line
	// must be dropped, never treated as a toggle.
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	fired := make(chan bool, 2)
	macBarMu.Lock()
	macOnTalk = func(on bool) { fired <- on }
	macBarMu.Unlock()
	defer func() {
		macBarMu.Lock()
		macOnTalk = nil
		macBarMu.Unlock()
	}()

	go serveVoiceControl(b)
	if _, err := a.Write([]byte("MIC-ON\nmic on\nenable\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case on := <-fired:
		t.Fatalf("junk line toggled the microphone (on=%v)", on)
	case <-time.After(500 * time.Millisecond):
	}
}
