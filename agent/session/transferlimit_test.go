package session

import (
	"os"
	"testing"
)

// The limit must be configurable per deployment: a support desk moving
// installers and a kiosk fleet that should never receive a file have very
// different answers, and a hardcoded constant serves neither.
func TestTransferLimitEnvOverride(t *testing.T) {
	orig := maxTransferBytes
	t.Cleanup(func() { maxTransferBytes = orig })

	t.Setenv("NEEV_MAX_TRANSFER_MB", "50")
	applyTransferLimitFromEnv()
	if want := int64(50 * 1024 * 1024); maxTransferBytes != want {
		t.Fatalf("limit = %d, want %d", maxTransferBytes, want)
	}
}

// A malformed or hostile value must leave the default in place rather than
// disabling the limit — "0" or "-1" turning the cap off would be worse than
// having no override at all.
func TestTransferLimitRejectsBadValues(t *testing.T) {
	orig := maxTransferBytes
	t.Cleanup(func() { maxTransferBytes = orig })

	for _, bad := range []string{"0", "-5", "abc", ""} {
		maxTransferBytes = 2 << 30
		if bad == "" {
			os.Unsetenv("NEEV_MAX_TRANSFER_MB")
		} else {
			t.Setenv("NEEV_MAX_TRANSFER_MB", bad)
		}
		applyTransferLimitFromEnv()
		if maxTransferBytes != 2<<30 {
			t.Errorf("value %q changed the limit to %d; the default must hold", bad, maxTransferBytes)
		}
	}
}
