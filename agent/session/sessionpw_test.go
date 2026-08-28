package session

import "testing"

// The session password must actually differ each time. It used to be read once
// from machine.dat and reused forever, so the only thing that ever changed it
// was a reinstall — anyone who saw it once kept working access.
func TestSessionPasswordIsFreshEachTime(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		p := newSessionPassword()
		if p == "" {
			t.Fatal("generated an EMPTY password — the host would accept anyone")
		}
		if seen[p] {
			t.Fatalf("repeated password %q on iteration %d", p, i)
		}
		seen[p] = true
	}
}

// A password short enough to guess is not a rotation, it is a formality.
func TestSessionPasswordLength(t *testing.T) {
	if got := len(newSessionPassword()); got < 8 {
		t.Errorf("session password is %d chars; too short to resist guessing", got)
	}
}
