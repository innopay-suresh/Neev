# FPS Control Implementation (Step 4)

## Changes Made

### 1. Added FPS Ladder Constants (pipeline.go)
- `fpsNormal = 30` — rtt < 100ms AND loss < 1%
- `fpsModerate = 15` — rtt 100-200ms OR loss 1-5%
- `fpsSevere = 5` — rtt > 200ms OR loss > 5%

### 2. Added `fpsControl` Struct
```go
type fpsControl struct {
    mu          sync.Mutex
    currentFPS  int
    targetFPS   int
    goodSamples int  // consecutive good samples
    badSamples  int  // consecutive bad samples
}
```

### 3. Added `computeTargetFPS` Method
Implements the FPS ladder logic:
- Returns `fpsSevere` (5) when rtt > 200ms OR loss > 5%
- Returns `fpsModerate` (15) when rtt > 100ms OR loss > 1%
- Returns `fpsNormal` (30) otherwise (good conditions)

### 4. Pipeline Struct Updates
- Added `fpsCtrl fpsControl` field
- Added `frameTick *time.Ticker` field for dynamic frame timing
- Initialized in `NewPipeline`: `p.frameTick = time.NewTicker(time.Second / time.Duration(fps))`

### 5. `collectStats` Updates
- Added call to `p.fpsCtrl.computeTargetFPS(rttMs, lossRate)`
- When FPS changes: logs `"fps_control adjusting FPS"`, recreates ticker with new interval
- Updated `QualityState.FPS` to use `p.fpsCtrl.currentFPS`
- Updated quality log to use dynamic FPS

### 6. Capture Loop Updates
- Moved `currentFPS := p.fpsCtrl.currentFPS` to top of frame tick handler
- Keyframe interval now uses dynamic `currentFPS` instead of fixed `p.fps`
- RTP timestamp samples use dynamic `currentFPS`

## FPS Transition Log Format
When FPS changes, logs:
```
level=info target_fps=15 rtt=150.5 loss=2.5 msg="fps_control adjusting FPS"
```

## Verification
Build: `CGO_ENABLED=1 go build -o remote-agent-mac .` — SUCCESS

## Design Notes
- FPS control runs in `collectStats` goroutine, updates frame ticker when needed
- Frame capture loop reads `p.fpsCtrl.currentFPS` at each tick (no locking needed for read)
- Ticker is recreated on FPS change: `p.frameTick.Stop()` + `p.frameTick = time.NewTicker(...)`
- Thread safety via `fpsCtrl.mu` mutex during FPS updates