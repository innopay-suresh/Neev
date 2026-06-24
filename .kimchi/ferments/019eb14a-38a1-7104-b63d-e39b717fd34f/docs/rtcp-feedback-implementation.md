# RTCP Feedback Implementation - Step 2 of Phase 1

## Summary
Implemented RTCP TWCC-style feedback collection via `PeerConnection.GetStats()` to enable adaptive bitrate/FPS control in the remote desktop agent.

## Changes Made

### 1. `agent/stream/pipeline.go`
- **Added `QualityState` struct**: Captures bitrate, FPS, loss rate, RTT, bytes/second, and jitter metrics
- **Added `statsSample` struct**: Sliding window entry for RTT and loss rate
- **Added constants**: `statsInterval` (1s), `statsWindowSize` (5 samples)
- **Extended `Pipeline` struct**:
  - `peerConn *webrtc.PeerConnection` - WebRTC connection for stats access
  - `statsMu sync.Mutex` - Thread-safe access to stats
  - `lastQuality QualityState` - Most recent quality metrics
  - `statsWindow []statsSample` - Sliding window of recent samples
  - `OnQualityChange func(QualityState)` - Callback for quality changes
- **Modified `NewPipeline`**: Now accepts `*webrtc.PeerConnection` parameter
- **Added `collectStats()` goroutine**: Runs every 1 second, extracts:
  - RTT from `StatsICECandidatePair.CurrentRoundTripTime`
  - Packet loss from `StatsInboundRTP.PacketsLost/PacketsReceived`
  - Bandwidth from `StatsICEAgent.BytesSent` delta calculation
  - Jitter from `StatsInboundRTP.Jitter`
- **Added `GetLastQuality()` and `GetStatsWindow()` methods**: For external access to stats
- **Logs quality metrics** at info level: `quality: rtt=Xms loss=X% bw=Xkbps fps=X`

### 2. `agent/network/peer.go`
- **Added `PeerConnection()` getter**: Returns the underlying `*webrtc.PeerConnection` for stats access

### 3. `agent/core/agent.go`
- **Updated `NewPipeline` calls**: Now pass `peer.PeerConnection()` as second argument
- **Added `OnQualityChange` callbacks**: Log quality updates at info level with format `quality update: rtt=Xms loss=X% bw=Xkbps fps=X`

## How It Works
1. Stats collection runs in a goroutine every 1 second
2. `PeerConnection.GetStats()` returns stats including ICE candidate pair state, inbound RTP stats
3. Metrics are extracted and fed to the existing ABR controller
4. Quality state is exposed via callback for future adaptation logic
5. A sliding window of 5 samples tracks recent network conditions

## Log Output Format
```
quality: rtt=45ms loss=0.2% bw=1840kbps fps=30
quality update: rtt=45ms loss=0.2% bw=1840kbps fps=30
```

## Dependencies
- Uses pion/webrtc v3.3.4 (already in go.mod)
- No new dependencies added

## Limitations
- TWCC (RFC 8888) feedback is not explicitly parsed; uses GetStats() which aggregates RTCP info
- Build environment has libx264 dependency issue (unrelated to this change)

## Next Steps (Phase 1 Steps 3-4)
- Implement adaptation logic based on sliding window (e.g., reduce quality if RTT > 200ms for 3+ samples)
- Wire up actual bitrate/FPS adjustment when thresholds are crossed