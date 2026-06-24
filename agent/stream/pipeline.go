// Package stream wires the capture → encode → WebRTC RTP pipeline.
//
// RTCP Feedback Collection:
// The pipeline runs a stats collector goroutine that periodically calls
// PeerConnection.GetStats() to extract RTT, packet loss, and bandwidth metrics.
// These are fed to the ABR controller and exposed via OnQualityChange callback.
//
// Flow:
//  1. Capturer grabs a screen frame (RGBA)
//  2. Encoder compresses it to H264
//  3. Packetizer splits the H264 frame into RTP packets
//  4. Packets are written to a pion WebRTC TrackLocalStaticRTP
//  5. Dirty rect detection: changed regions are encoded as JPEG and sent via DataChannel
//
// The pipeline runs entirely in Go goroutines; CGo calls are confined
// to the capture and encode packages.
package stream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog/log"

	"github.com/neev/remote-agent/agent/capture"
	"github.com/neev/remote-agent/agent/encode"
)

// DirtyRectPayload represents a single dirty rectangle with encoded JPEG data.
type DirtyRectPayload struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	Data string `json:"data"` // base64-encoded JPEG
}

// DirtyRectsMessage is the JSON payload sent over DataChannel for dirty rect updates.
type DirtyRectsMessage struct {
	Type     string              `json:"type"`
	Rects    []DirtyRectPayload  `json:"rects"`
	Keyframe bool                `json:"keyframe"`
}

const (
	// VP8 RTP payload type (96 is the standard dynamic type for VP8).
	vp8PayloadType  = 96
	vp8ClockRate    = 90000
	// Default streaming parameters.
	defaultFPS         = 30
	defaultBitrateKbps = 1500
	minBitrateKbps     = 500
	maxBitrateKbps     = 8000
	// defaultHwAccel unused for VP8 (software encoding only).
	defaultHwAccel = false
	// Stats collection interval
	statsInterval = 1 * time.Second
	// Sliding window size for quality samples
	statsWindowSize = 5
	// FPS ladder thresholds
	fpsNormal   = 30 // rtt < 100ms AND loss < 1%
	fpsModerate = 15 // rtt 100-200ms OR loss 1-5%
	fpsSevere   = 5  // rtt > 200ms OR loss > 5%
	rttModerate = 100.0
	rttSevere   = 200.0
	lossModerate = 0.01
	lossSevere   = 0.05
)

// QualityState captures current network conditions for adaptive bitrate control.
type QualityState struct {
	BitrateKbps    int     // current encoder bitrate target (kbps)
	FPS            int     // current FPS setting
	LossRate       float64 // packet loss rate (0.0-1.0)
	RTTMs          float64 // round-trip time (ms)
	BytesPerSecond int64   // estimated bytes per second sent
	JitterMs       float64 // RTP jitter (ms)
}

// statsSample holds a single stats measurement for sliding window.
type statsSample struct {
	rttMs    float64
	lossRate float64
}

// fpsControl manages adaptive frame rate based on network conditions.
type fpsControl struct {
	mu          sync.Mutex
	currentFPS  int
	targetFPS   int
	goodSamples int // consecutive good samples
	badSamples  int // consecutive bad samples
}

// computeTargetFPS returns target FPS based on network conditions.
func (fc *fpsControl) computeTargetFPS(rttMs, lossRate float64) int {
	// Severe: rtt > 200ms OR loss > 5%
	if rttMs > rttSevere || lossRate > lossSevere {
		return fpsSevere
	}
	// Moderate: rtt 100-200ms OR loss 1-5%
	if rttMs > rttModerate || lossRate > lossModerate {
		return fpsModerate
	}
	// Good: rtt < 100ms AND loss < 1%
	return fpsNormal
}

// Pipeline manages the full capture → encode → RTP stream.
type Pipeline struct {
	capturer   capture.Capturer
	encoder    *encode.Encoder // VP8 encoder (all platforms via libvpx)
	abr        *capture.ABRController
	track      *webrtc.TrackLocalStaticRTP
	packetizer rtp.Packetizer

	fps         int
	fpsCtrl     fpsControl
	frameTick   *time.Ticker
	running     atomic.Bool
	ctx         context.Context
	cancel      context.CancelFunc
	frameCount  int // frames since last keyframe (keyframe every 60 frames)

	SendLog func(level, msg string)
	// SendCursorInfo sends cursor position and bitmap for overlay rendering.
	// Called at a lower frequency than frame capture to save bandwidth.
	SendCursorInfo func(ci capture.CursorInfo)
	// OnQualityChange is called when network quality metrics change significantly.
	// The callback receives the current QualityState.
	OnQualityChange func(QualityState)
	// SendDirtyRects sends encoded dirty rectangle data over the DataChannel.
	// The payload is a JSON message with base64-encoded JPEG data for each rect.
	SendDirtyRects func(data []byte)

	// Internals for stats collection
	peerConn    *webrtc.PeerConnection // WebRTC peer connection for stats
	statsMu     sync.Mutex
	lastQuality QualityState
	statsWindow []statsSample // sliding window of last N samples
}

// NewPipeline creates a pipeline that streams to the given WebRTC track.
// It creates the platform capturer, detects screen resolution, and
// initializes the H264 encoder at that resolution.
// The peerConn parameter is used for GetStats() to collect RTCP feedback.
func NewPipeline(track *webrtc.TrackLocalStaticRTP, peerConn *webrtc.PeerConnection, fps int, displayID uint32) (*Pipeline, error) {
	if fps <= 0 {
		fps = defaultFPS
	}

	// Platform screen capturer.
	log.Info().Uint32("displayID", displayID).Msg("creating platform capturer...")
	cap, err := capture.NewPlatformCapture(displayID)
	if err != nil {
		log.Error().Err(err).Msg("capturer creation failed")
		return nil, fmt.Errorf("capturer: %w", err)
	}
	log.Info().Msg("platform capturer created successfully")

	// Get screen resolution instantly using Bounds().
	width, height := cap.Bounds()
	log.Info().Int("width", width).Int("height", height).Msg("screen bounds")
	if width <= 0 || height <= 0 {
		cap.Close()
		return nil, fmt.Errorf("could not determine screen resolution: %dx%d", width, height)
	}
	log.Info().Int("width", width).Int("height", height).Int("fps", fps).Msg("screen resolution detected")

	// VP8 encoder — uses libvpx on all platforms (including Windows cross-compile).
	enc, err := encode.NewEncoder(width, height, fps, defaultBitrateKbps)
	if err != nil {
		cap.Close()
		log.Error().Err(err).Msg("VP8 encoder creation failed")
		return nil, fmt.Errorf("VP8 encoder: %w", err)
	}
	log.Info().Int("width", width).Int("height", height).Int("kbps", defaultBitrateKbps).Msg("VP8 encoder init OK")

	// Adaptive bitrate controller (synchronous — evaluated in collectStats goroutine).
	abr := capture.NewABRController()

	// RTP packetizer for VP8.
	ssrc := webrtc.SSRC(12345678)
	packetizer := rtp.NewPacketizer(
		1200,          // max RTP packet size (bytes)
		vp8PayloadType,
		uint32(ssrc),
		&codecs.VP8Payloader{},
		rtp.NewRandomSequencer(),
		vp8ClockRate,
	)

	p := &Pipeline{
		capturer:    cap,
		encoder:     enc,
		abr:         abr,
		track:       track,
		packetizer:  packetizer,
		fps:         fps,
		fpsCtrl: fpsControl{
			currentFPS: fps,
			targetFPS:  fps,
		},
		peerConn:    peerConn,
		statsWindow: make([]statsSample, 0, statsWindowSize),
	}
	p.frameTick = time.NewTicker(time.Second / time.Duration(fps))
	return p, nil
}

// Start begins streaming. Blocks until ctx is cancelled.
func (p *Pipeline) Start(ctx context.Context) error {
	if !p.running.CompareAndSwap(false, true) {
		return fmt.Errorf("pipeline already running")
	}
	defer p.running.Store(false)

	// Start RTCP stats collection loop.
	go p.collectStats(ctx)

	// Channel for PLI (Picture Loss Indication) requests from viewer
	pliChan := make(chan struct{}, 1)

	// Start RTCP reader loop to catch PLI and FIR requests
	if p.peerConn != nil {
		for _, sender := range p.peerConn.GetSenders() {
			if sender.Track() != nil && sender.Track().ID() == p.track.ID() {
				go func(s *webrtc.RTPSender) {
					rtcpBuf := make([]byte, 1500)
					for {
						n, _, rtcpErr := s.Read(rtcpBuf)
						if rtcpErr != nil {
							return
						}
						pkts, err := rtcp.Unmarshal(rtcpBuf[:n])
						if err != nil {
							continue
						}
						for _, pkt := range pkts {
							switch pkt.(type) {
							case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
								// Notify pipeline to force a keyframe
								select {
								case pliChan <- struct{}{}:
								default:
								}
							}
						}
					}
				}(sender)
			}
		}
	}

	var lastHash uint64

	log.Info().Int("fps", p.fps).Msg("streaming pipeline started (H.264)")

	var consecutiveErrors int
	backOff := time.Second
	maxBackOff := 10 * time.Second

	var lastFrame *image.RGBA
	ticksSinceLastEncode := 0

	// Cursor info ticker: send at ~15Hz (every 2nd frame at 30fps)
	cursorTicker := time.NewTicker(66 * time.Millisecond)
	defer cursorTicker.Stop()
	var lastCursor capture.CursorInfo

	var forceKeyframeNext bool

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pliChan:
			// Viewer requested a keyframe due to packet loss
			forceKeyframeNext = true
			log.Info().Msg("received PLI request, forcing keyframe on next tick")
			continue
		case <-cursorTicker.C:
			// Send cursor info at lower frequency
			if p.SendCursorInfo != nil {
				ci := p.capturer.GetCursorInfo()
				// Only send if cursor changed position or visibility
				if ci.X != lastCursor.X || ci.Y != lastCursor.Y || ci.Visible != lastCursor.Visible {
					lastCursor = ci
					p.SendCursorInfo(ci)
				}
			}
		case t := <-p.frameTick.C:
			ticksSinceLastEncode++
			currentFPS := p.fpsCtrl.currentFPS
			frame, err := p.capturer.CaptureFrame()
			if err != nil {
				if err == capture.ErrNoNewFrame {
					consecutiveErrors = 0
					backOff = time.Second

					// Force a keyframe every 2 seconds even if screen is static
					if ticksSinceLastEncode >= currentFPS*2 {
						if lastFrame != nil {
							frame = lastFrame
						} else {
							// Generate a placeholder frame to kickstart WebRTC if the OS
							// hasn't given us a single frame yet (e.g. perfectly static headless desktop)
							w, h := p.capturer.Bounds()
							frame = image.NewRGBA(image.Rect(0, 0, w, h))
							// Fill with a dark gray so it's not pure black, indicating connection is active
							for i := 0; i < len(frame.Pix); i += 4 {
								frame.Pix[i+0] = 0   // R
								frame.Pix[i+1] = 0   // G
								frame.Pix[i+2] = 150 // B
								frame.Pix[i+3] = 255 // A
							}
						}
					} else {
						continue
					}
				} else if err == capture.ErrAccessDenied {
					return err
				} else {
					consecutiveErrors++
					if consecutiveErrors%5 == 0 {
						errStr := fmt.Sprintf("Capture error: %v", err)
						log.Warn().Err(err).Int("consecutiveErrors", consecutiveErrors).Msg("capture error")
						if p.SendLog != nil {
							p.SendLog("error", errStr)
						}
					}
					time.Sleep(backOff)
					if backOff < maxBackOff {
						backOff *= 2
					}
					continue
				}
			} else {
				// Reset error backoff on successful capture
				consecutiveErrors = 0
				backOff = time.Second
				lastFrame = frame
			}

			// Force keyframe every 60 frames, on first frame, or on PLI request
			p.frameCount++
			forceKF := p.frameCount >= 60 || p.frameCount == 1 || forceKeyframeNext
			forceKeyframeNext = false

			// We need a minimum keep-alive frame rate (e.g. 5 fps) so WebRTC doesn't get starved
			// and can initialize playback successfully even if the screen is static.
			forceKeepAlive := p.frameCount%(currentFPS/5) == 0

			// Skip unchanged frames unless forcing a keyframe or keep-alive frame.
			h := quickHash(frame)
			if h == lastHash && !forceKF && !forceKeepAlive {
				continue
			}
			lastHash = h

			// Dirty rect detection: only send via DataChannel for non-keyframe frames
			// Keyframes go through full H.264 for reliability
			var totalDirtyPixels int
			if p.SendDirtyRects != nil && !forceKF && lastFrame != nil {
				if rects := capture.DetectDirtyRects(frame, lastFrame, 16); len(rects) > 0 {
					totalDirtyPixels = capture.TotalPixels(rects)
					
					// Encode each rect as JPEG and build message
					msg := DirtyRectsMessage{
						Type:     "dirty_rects",
						Rects:    make([]DirtyRectPayload, 0, len(rects)),
						Keyframe: false,
					}

					var totalJpegBytes int
					for _, r := range rects {
						if enc := capture.EncodeJPEG(frame, r, 75); enc != nil {
							msg.Rects = append(msg.Rects, DirtyRectPayload{
								X:    r.X,
								Y:    r.Y,
								W:    r.W,
								H:    r.H,
								Data: base64.StdEncoding.EncodeToString(enc.Data),
							})
							totalJpegBytes += len(enc.Data)
						}
					}

					if len(msg.Rects) > 0 {
						if data, err := json.Marshal(msg); err == nil {
							p.SendDirtyRects(data)
							log.Debug().
								Int("rects", len(msg.Rects)).
								Int("pixels", totalDirtyPixels).
								Int("jpeg_bytes", totalJpegBytes).
								Msg("dirty_rects_sent")
						}
					}
				}
			} else if forceKF {
				// Send keyframe notification so viewer knows full frame is coming via H.264
				msg := DirtyRectsMessage{
					Type:     "dirty_rects",
					Keyframe: true,
				}
				if data, err := json.Marshal(msg); err == nil {
					p.SendDirtyRects(data)
				}
			}

			// Dynamically recreate encoder if resolution changes (or if Bounds() was scaled by Windows DPI)
			fw, fh := frame.Bounds().Dx(), frame.Bounds().Dy()
			if fw != p.encoder.Width() || fh != p.encoder.Height() {
				log.Info().Int("oldW", p.encoder.Width()).Int("oldH", p.encoder.Height()).
					Int("newW", fw).Int("newH", fh).Msg("resolution change detected, recreating encoder")

				newEnc, err := encode.NewEncoder(fw, fh, p.fps, p.encoder.Bitrate())
				if err != nil {
					log.Error().Err(err).Msg("failed to recreate encoder on resolution change")
					if p.SendLog != nil {
						p.SendLog("error", fmt.Sprintf("Encoder recreation failed: %v", err))
					}
					continue
				}
				p.encoder.Close()
				p.encoder = newEnc
				p.abr = capture.NewABRController()
				forceKF = true
				if p.SendLog != nil {
					p.SendLog("info", fmt.Sprintf("Screen resolution adjusted to %dx%d", fw, fh))
				}
			}

			encoded, err := p.encoder.Encode(frame, forceKF)
			if err != nil {
				log.Warn().Msgf("encode error: %v", err)
				if p.SendLog != nil {
					p.SendLog("error", fmt.Sprintf("Encode error: %v", err))
				}
				continue
			}
			if encoded == nil {
				continue // buffering
			}
			if forceKF {
				p.frameCount = 0
				log.Info().Int("frame", 0).Msg("keyframe_sent")
			}
			ticksSinceLastEncode = 0

			// Packetize H264 frame into RTP packets.
			samples := uint32(vp8ClockRate) / uint32(currentFPS)
			_ = t
			packets := p.packetizer.Packetize(encoded.Data, samples)
			if len(packets) > 0 {
				log.Info().Int("packets", len(packets)).Int("pt", int(packets[0].Header.PayloadType)).Msg("RTP packets ready to send")
			}
			pktCount := 0
			for _, pkt := range packets {
				log.Debug().Uint8("pt", pkt.Header.PayloadType).Uint32("ssrc", pkt.Header.SSRC).Msg("WriteRTP call")
				if err := p.track.WriteRTP(pkt); err != nil {
					log.Warn().Err(err).Msg("RTP write error")
					return err
				}
				pktCount++
			}
			log.Info().Int("written", pktCount).Msg("RTP write loop completed")
		}
	}
}

// Stop halts the pipeline and releases resources.
func (p *Pipeline) Stop() {
	p.encoder.Close()
	p.capturer.Close()
}

// UpdateNetworkStats feeds RTT and loss metrics for ABR.
func (p *Pipeline) UpdateNetworkStats(rtt time.Duration, lossRatio float64) {
	p.abr.UpdateStats(rtt, lossRatio)
}

// collectStats periodically extracts RTCP feedback from PeerConnection.GetStats()
// and feeds the metrics to the ABR controller and OnQualityChange callback.
func (p *Pipeline) collectStats(ctx context.Context) {
	ticker := time.NewTicker(statsInterval)
	defer ticker.Stop()

	var lastBytesSent uint64
	var lastTimestamp time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if p.peerConn == nil {
				continue
			}

			stats := p.peerConn.GetStats()

			var rttMs float64
			var lossRate float64
			var jitterMs float64
			var bytesSent uint64
			var packetsLost uint64
			var packetsReceived uint64

			for _, s := range stats {
				switch s := s.(type) {
				case webrtc.ICECandidatePairStats:
					if s.State == webrtc.StatsICECandidatePairStateSucceeded {
						rttMs = s.CurrentRoundTripTime * 1000
						bytesSent = s.BytesSent
					}
				case webrtc.InboundRTPStreamStats:
					if s.Kind == "video" {
						packetsLost = uint64(s.PacketsLost)
						packetsReceived = uint64(s.PacketsReceived)
						jitterMs = s.Jitter * 1000
					}
				}
			}

			// Calculate loss rate
			if packetsReceived+packetsLost > 0 {
				lossRate = float64(packetsLost) / float64(packetsReceived+packetsLost)
			}

			// Calculate bytes per second
			now := time.Now()
			var bytesPerSecond int64
			if !lastTimestamp.IsZero() && bytesSent > lastBytesSent {
				elapsed := now.Sub(lastTimestamp).Seconds()
				if elapsed > 0 {
					bytesPerSecond = int64(float64(bytesSent-lastBytesSent) / elapsed)
				}
			}
			lastBytesSent = bytesSent
			lastTimestamp = now

			// Update sliding window
			p.statsMu.Lock()
			p.statsWindow = append(p.statsWindow, statsSample{rttMs: rttMs, lossRate: lossRate})
			if len(p.statsWindow) > statsWindowSize {
				p.statsWindow = p.statsWindow[1:]
			}
			p.statsMu.Unlock()

			// Feed ABR and apply recommended bitrate
			rtt := time.Duration(rttMs) * time.Millisecond
			p.abr.UpdateStats(rtt, lossRate)
			p.encoder.SetBitrate(p.abr.GetTargetBitrate())

			// Update FPS based on network conditions
			targetFPS := p.fpsCtrl.computeTargetFPS(rttMs, lossRate)
			p.fpsCtrl.mu.Lock()
			changed := targetFPS != p.fpsCtrl.currentFPS
			if changed {
				log.Info().Int("target_fps", targetFPS).Float64("rtt", rttMs).Float64("loss", lossRate*100).Msg("fps_control adjusting FPS")
				p.fpsCtrl.currentFPS = targetFPS
				// Recreate ticker with new interval
				p.frameTick.Stop()
				p.frameTick = time.NewTicker(time.Second / time.Duration(targetFPS))
			}
			p.fpsCtrl.mu.Unlock()

			// Build quality state
			quality := QualityState{
				BitrateKbps:    p.encoder.Bitrate(),
				FPS:            p.fpsCtrl.currentFPS,
				LossRate:       lossRate,
				RTTMs:          rttMs,
				BytesPerSecond: bytesPerSecond,
				JitterMs:       jitterMs,
			}

			p.statsMu.Lock()
			p.lastQuality = quality
			p.statsMu.Unlock()

			// Log quality metrics
			bwKbps := float64(bytesPerSecond*8) / 1000
			lossPercent := lossRate * 100
			log.Info().
				Float64("rtt", rttMs).
				Float64("loss", lossPercent).
				Float64("bw", bwKbps).
				Int("fps", p.fpsCtrl.currentFPS).
				Msg("quality")

			// Notify callback if set
			if p.OnQualityChange != nil {
				p.OnQualityChange(quality)
			}
		}
	}
}

// GetLastQuality returns the most recent quality metrics.
func (p *Pipeline) GetLastQuality() QualityState {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	return p.lastQuality
}

// GetStatsWindow returns a copy of the sliding window of recent stats samples.
func (p *Pipeline) GetStatsWindow() []statsSample {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	window := make([]statsSample, len(p.statsWindow))
	copy(window, p.statsWindow)
	return window
}

// quickHash samples every 64th pixel for fast change detection.
func quickHash(img *image.RGBA) uint64 {
	var h uint64 = 14695981039346656037
	pix := img.Pix
	for i := 0; i < len(pix); i += 256 {
		h ^= uint64(pix[i])
		h *= 1099511628211
	}
	return h
}
