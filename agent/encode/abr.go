package encode

import (
	"sync"
	"time"
)

// ABRController implements adaptive bitrate control based on network
// feedback (RTT, packet loss). It adjusts the encoder bitrate to keep
// the stream smooth without overflowing network buffers.
//
// Algorithm: AIMD (Additive Increase / Multiplicative Decrease)
//   - On good conditions: increase bitrate by StepUp kbps every interval
//   - On congestion (loss > LossThreshold): cut bitrate by half (MD)
type ABRController struct {
	mu sync.Mutex

	encoder *Encoder

	// Bitrate bounds (kbps)
	MinBitrate int
	MaxBitrate int
	current    int

	// Tuning knobs
	StepUp        int     // kbps increase per good interval
	LossThreshold float64 // packet loss ratio that triggers decrease
	RTTThreshold  time.Duration

	// Measurement inputs (updated externally by WebRTC stats)
	lastLoss float64
	lastRTT  time.Duration

	stopCh chan struct{}
}

// NewABRController creates a controller for the given encoder.
func NewABRController(enc *Encoder, minKbps, maxKbps int) *ABRController {
	return &ABRController{
		encoder:       enc,
		MinBitrate:    minKbps,
		MaxBitrate:    maxKbps,
		current:       enc.Bitrate(),
		StepUp:        100,
		LossThreshold: 0.02, // 2% loss → decrease
		RTTThreshold:  150 * time.Millisecond,
		stopCh:        make(chan struct{}),
	}
}

// UpdateStats feeds latest RTT and packet loss measurements to the controller.
// Call this from your WebRTC stats reader goroutine.
func (a *ABRController) UpdateStats(rtt time.Duration, lossRatio float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastRTT = rtt
	a.lastLoss = lossRatio
}

// Run starts the adaptive bitrate control loop. Call in a goroutine.
func (a *ABRController) Run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.adjust()
		}
	}
}

// Stop shuts down the control loop.
func (a *ABRController) Stop() { close(a.stopCh) }

func (a *ABRController) adjust() {
	a.mu.Lock()
	loss := a.lastLoss
	rtt := a.lastRTT
	cur := a.current
	a.mu.Unlock()

	var next int
	congested := loss > a.LossThreshold || (rtt > 0 && rtt > a.RTTThreshold)
	if congested {
		// Multiplicative decrease — halve the bitrate.
		next = cur / 2
	} else {
		// Additive increase.
		next = cur + a.StepUp
	}

	// Clamp to [min, max].
	if next < a.MinBitrate {
		next = a.MinBitrate
	}
	if next > a.MaxBitrate {
		next = a.MaxBitrate
	}

	if next != cur {
		a.mu.Lock()
		a.current = next
		a.mu.Unlock()
		a.encoder.SetBitrate(next)
	}
}

// CurrentBitrate returns the current target bitrate in kbps.
func (a *ABRController) CurrentBitrate() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current
}
