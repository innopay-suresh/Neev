package capture

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Bitrate ladder for adaptive bitrate control (kbps).
// Ordered from highest to lowest.
var bitrateLadder = []int{2048, 1024, 512, 256}

// ABRController implements adaptive bitrate control based on network
// feedback (RTT, packet loss). It steps through a fixed bitrate ladder
// to keep the stream smooth without overflowing network buffers.
//
// Step-down trigger: lossRate > 5% OR rtt > 200ms for 3+ consecutive samples
// Step-up trigger:   lossRate < 1% AND rtt < 100ms for 10+ consecutive samples
type ABRController struct {
	mu sync.Mutex

	// Current position in the bitrate ladder (index into bitrateLadder).
	// 0 = highest (2048 kbps), len(ladder)-1 = lowest (256 kbps).
	step int

	// Consecutive samples meeting step-down condition.
	downConsecutive int
	// Consecutive samples meeting step-up condition.
	upConsecutive int

	// Measurement inputs (updated externally by WebRTC stats).
	lastLoss float64
	lastRTT  time.Duration
}

// NewABRController creates a fresh ABR controller starting at the top of the ladder.
func NewABRController() *ABRController {
	return &ABRController{step: 0}
}

// UpdateStats feeds latest RTT and packet loss measurements to the controller.
// Call this from your WebRTC stats reader goroutine on each stats interval.
func (a *ABRController) UpdateStats(rtt time.Duration, lossRatio float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastRTT = rtt
	a.lastLoss = lossRatio
}

// GetTargetBitrate returns the recommended encoder bitrate in kbps for the
// current network conditions. Call this after UpdateStats to apply the result.
func (a *ABRController) GetTargetBitrate() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.evaluate()

	return bitrateLadder[a.step]
}

// evaluate advances the internal ABR state machine. Caller must hold a.mu.
func (a *ABRController) evaluate() {
	loss := a.lastLoss
	rtt := a.lastRTT

	bad := loss > 0.05 || rtt > 200*time.Millisecond
	good := loss < 0.01 && rtt < 100*time.Millisecond

	if bad {
		a.upConsecutive = 0
		a.downConsecutive++
		if a.downConsecutive >= 3 && a.step < len(bitrateLadder)-1 {
			a.step++
			a.downConsecutive = 0
			log.Info().Int("bitrate_kbps", bitrateLadder[a.step]).Msg("abr")
		}
	} else if good {
		a.downConsecutive = 0
		a.upConsecutive++
		if a.upConsecutive >= 10 && a.step > 0 {
			a.step--
			a.upConsecutive = 0
			log.Info().Int("bitrate_kbps", bitrateLadder[a.step]).Msg("abr")
		}
	} else {
		// Neither clearly good nor bad — reset up counter, keep down counter
		// to allow recovery when conditions improve.
		a.upConsecutive = 0
	}
}

// CurrentBitrate returns the current bitrate in kbps (convenience wrapper).
func (a *ABRController) CurrentBitrate() int {
	return a.GetTargetBitrate()
}