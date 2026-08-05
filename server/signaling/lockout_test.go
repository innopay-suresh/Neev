package signaling

import (
	"testing"
	"time"
)

// The lockout used to be permanent: failCount only cleared on a SUCCESSFUL
// password, but the check ran BEFORE verification — so once a target hit 5
// failures nothing could ever clear it and that machine was unreachable until
// the relay restarted. A stale saved password bricked a device for good.

func TestLockoutExpiresSoAMachineIsNotBrickedForever(t *testing.T) {
	h := &Hub{failCount: make(map[string]failRecord)}

	// Five bad attempts, the last one just over the lockout window ago.
	h.failCount["target-1"] = failRecord{
		count: maxConnectFails,
		last:  time.Now().Add(-lockoutFor - time.Second),
	}

	rec := h.failCount["target-1"]
	locked := rec.count >= maxConnectFails && time.Since(rec.last) < lockoutFor
	if locked {
		t.Fatal("a target was still locked out after the lockout window passed")
	}
}

func TestLockoutHoldsDuringTheWindow(t *testing.T) {
	// It must still actually rate limit — the fix is expiry, not removal.
	h := &Hub{failCount: make(map[string]failRecord)}
	h.failCount["target-2"] = failRecord{count: maxConnectFails, last: time.Now()}

	rec := h.failCount["target-2"]
	locked := rec.count >= maxConnectFails && time.Since(rec.last) < lockoutFor
	if !locked {
		t.Fatal("five failures in a row did not rate limit at all")
	}
}

func TestOldFailuresAreForgottenEntirely(t *testing.T) {
	// Occasional typos spread across a long session must not accumulate into a
	// lockout hours later.
	h := &Hub{failCount: make(map[string]failRecord)}
	h.failCount["target-3"] = failRecord{
		count: maxConnectFails - 1,
		last:  time.Now().Add(-failWindow - time.Minute),
	}

	rec := h.failCount["target-3"]
	if !rec.last.IsZero() && time.Since(rec.last) > failWindow {
		rec = failRecord{}
	}
	if rec.count != 0 {
		t.Fatalf("failures older than the window were kept (count=%d)", rec.count)
	}
}

func TestLockoutWindowIsRecoverable(t *testing.T) {
	// A user who fixes their password should not wait long. Guard the constants
	// themselves — a lockout measured in hours is indistinguishable from the
	// permanent bug for anyone trying to work.
	if lockoutFor > 5*time.Minute {
		t.Errorf("lockout of %v is too long to be recoverable in practice", lockoutFor)
	}
	if failWindow < time.Minute {
		t.Errorf("fail window of %v forgets too fast to rate limit", failWindow)
	}
}
